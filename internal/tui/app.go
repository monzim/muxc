package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/state"
)

// App is the root tea.Model. Owns shared state (config, terminal size,
// attach handoff target) and dispatches messages to the active screen.
type App struct {
	screen       screenID
	cfg          *config.Config
	st           *state.State
	width        int
	height       int
	styles       Styles
	keys         KeyMap
	showHelp     bool
	attachTarget string // non-empty if user picked attach; tui.Run reads after Quit
	bannerErr    error  // banner shown above the active screen, cleared on next key

	// Per-screen models.
	sessions   sessionsModel
	newForm    newFormModel
	killPicker killpickerModel
	doctor     doctorModel
	info       infoModel
}

// NewApp constructs the initial root model.
func NewApp(cfg *config.Config, st *state.State) App {
	styles := DefaultStyles()
	keys := DefaultKeyMap()
	configDir := ""
	return App{
		screen:     screenSessions,
		cfg:        cfg,
		st:         st,
		styles:     styles,
		keys:       keys,
		sessions:   newSessionsModel(cfg, st, styles, keys),
		newForm:    newNewFormModel(cfg, st, styles, keys),
		killPicker: newKillpickerModel(cfg, st, styles, keys),
		doctor:     newDoctorModel(cfg, st, configDir, styles, keys),
		info:       newInfoModel(cfg, styles, keys),
	}
}

// AttachTarget returns the session name the user picked for attach (or "" if
// they exited some other way). Read after program.Run().
func (a App) AttachTarget() string { return a.attachTarget }

func (a App) Init() tea.Cmd {
	return a.sessions.Init()
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Cross-screen messages, handled regardless of which screen is active.
	switch msg := msg.(type) {

	case attachRequestMsg:
		a.attachTarget = msg.name
		return a, tea.Quit

	case sessionCreatedMsg:
		a.screen = screenSessions
		return a, gatherCmd(a.cfg, a.st)

	case killCompleteMsg:
		a.screen = screenSessions
		return a, gatherCmd(a.cfg, a.st)

	case killVictimsReadyMsg:
		a.killPicker = a.killPicker.SetVictims(msg.rows)
		return a, nil

	case errMsg:
		a.bannerErr = msg.err
		return a, nil

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Fan window-size out to every screen — Bubble Tea otherwise only
		// delivers it to the active model, which means re-entering a
		// previously-unseen screen has stale dimensions.
		a.sessions.width, a.sessions.height = msg.Width, msg.Height
		a.newForm.width, a.newForm.height = msg.Width, msg.Height
		a.killPicker.width, a.killPicker.height = msg.Width, msg.Height
		a.doctor.width, a.doctor.height = msg.Width, msg.Height
		// Forward to info so its viewport resizes too.
		var infoCmd tea.Cmd
		a.info, infoCmd = a.info.Update(msg)
		return a, infoCmd

	case tea.KeyMsg:
		// Help modal closes on any key.
		if a.showHelp {
			a.showHelp = false
			return a, nil
		}

		// Global keys.
		switch {
		case keyHit(msg, a.keys.Quit):
			return a, tea.Quit
		case keyHit(msg, a.keys.Help):
			a.showHelp = true
			return a, nil
		case keyHit(msg, a.keys.Back):
			if a.screen != screenSessions {
				a.screen = screenSessions
				return a, nil
			}
		}

		// Sessions-screen action keys that switch screens.
		if a.screen == screenSessions {
			switch {
			case keyHit(msg, a.keys.New):
				a.newForm.reset()
				a.screen = screenNewForm
				return a, a.newForm.Init()
			case keyHit(msg, a.keys.Kill):
				sel := ""
				if a.sessions.cursor < len(a.sessions.rows) {
					sel = a.sessions.rows[a.sessions.cursor].Name
				}
				a.killPicker.reset(sel)
				a.screen = screenKillPicker
				return a, nil
			case keyHit(msg, a.keys.Attach):
				if a.sessions.cursor >= 0 && a.sessions.cursor < len(a.sessions.rows) {
					name := a.sessions.rows[a.sessions.cursor].Name
					return a, func() tea.Msg { return attachRequestMsg{name: name} }
				}
			case keyHit(msg, a.keys.Info), keyHit(msg, a.keys.Enter):
				if a.sessions.cursor >= 0 && a.sessions.cursor < len(a.sessions.rows) {
					a.info.setRow(a.sessions.rows[a.sessions.cursor])
					a.screen = screenInfo
					return a, a.info.Init()
				}
			case keyHit(msg, a.keys.Doctor):
				a.screen = screenDoctor
				return a, a.doctor.Init()
			}
		}

		// Info screen 'a' = attach to the viewed session.
		if a.screen == screenInfo {
			if keyHit(msg, a.keys.Attach) && a.info.row.Name != "" {
				name := a.info.row.Name
				return a, func() tea.Msg { return attachRequestMsg{name: name} }
			}
		}

		// Banner clears on the next non-help key.
		a.bannerErr = nil
	}

	// Forward to the active screen.
	switch a.screen {
	case screenSessions:
		next, cmd := a.sessions.Update(msg)
		a.sessions = next
		return a, cmd
	case screenNewForm:
		next, cmd := a.newForm.Update(msg)
		a.newForm = next
		return a, cmd
	case screenKillPicker:
		next, cmd := a.killPicker.Update(msg)
		a.killPicker = next
		return a, cmd
	case screenDoctor:
		next, cmd := a.doctor.Update(msg)
		a.doctor = next
		return a, cmd
	case screenInfo:
		next, cmd := a.info.Update(msg)
		a.info = next
		return a, cmd
	}
	return a, nil
}

func (a App) View() string {
	var body string
	switch a.screen {
	case screenSessions:
		body = a.sessions.View()
	case screenNewForm:
		body = a.newForm.View()
	case screenKillPicker:
		body = a.killPicker.View()
	case screenDoctor:
		body = a.doctor.View()
	case screenInfo:
		body = a.info.View()
	default:
		body = a.styles.Muted.Render("(unknown screen)")
	}

	if a.bannerErr != nil {
		banner := a.styles.StatusBad.Render("⚠ " + a.bannerErr.Error())
		body = banner + "\n" + body
	}

	if a.showHelp {
		return a.renderHelpModal(body)
	}
	return body
}

// renderHelpModal paints a centered help card over the active screen.
func (a App) renderHelpModal(body string) string {
	rows := []string{
		a.styles.ModalHeader.Render("muxc — keyboard shortcuts"),
		"",
		section("Navigation",
			row("↑ k", "up"),
			row("↓ j", "down"),
			row("← h", "left"),
			row("→ l", "right"),
			row("enter", "select / open"),
			row("esc", "back"),
		),
		"",
		section("Sessions screen",
			row("a", "attach to selected session"),
			row("n", "create new session"),
			row("K", "open kill picker"),
			row("i", "open info"),
			row("s", "cycle sort key"),
			row("r", "force refresh now"),
			row("d", "open doctor"),
			row("m", "switch to mem view (sort by RSS)"),
		),
		"",
		section("Info screen",
			row("+/-", "increase/decrease transcript lines"),
			row("a", "attach to this session"),
			row("PgUp/PgDn", "scroll transcript"),
		),
		"",
		section("Global",
			row("?", "toggle this help"),
			row("q  ^C", "quit"),
		),
		"",
		a.styles.Faint.Render("press any key to dismiss"),
	}
	modal := a.styles.Modal.Render(strings.Join(rows, "\n"))
	if a.width > 0 && a.height > 0 {
		return lipgloss.Place(a.width, a.height,
			lipgloss.Center, lipgloss.Center, modal,
			lipgloss.WithWhitespaceChars(" "))
	}
	return modal
}

// section formats one help-modal section: a bold heading plus indented rows.
func section(title string, lines ...string) string {
	parts := append([]string{lipgloss.NewStyle().Bold(true).Render(title)}, lines...)
	return strings.Join(parts, "\n")
}

// row formats one (key, label) line inside a help-modal section.
func row(key, label string) string {
	keyCol := lipgloss.NewStyle().Width(12).Foreground(colorPrimary).Bold(true).Render(key)
	return "  " + keyCol + label
}
