package tui

import (
	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/state"
)

// App is the root tea.Model. It owns shared state (config, terminal size,
// attach handoff target) and dispatches messages to the currently-focused
// screen's model.
type App struct {
	screen       screenID
	cfg          *config.Config
	st           *state.State
	width        int
	height       int
	styles       Styles
	keys         KeyMap
	help         help.Model
	showHelp     bool
	attachTarget string // non-empty if user picked attach; tui.Run reads after program.Run()
	bannerErr    error  // non-nil → top-of-screen banner (cleared on next key)

	// Per-screen models.
	sessions   sessionsModel
	newForm    newFormModel
	killPicker killpickerModel
}

// NewApp constructs the initial root model.
func NewApp(cfg *config.Config, st *state.State) App {
	styles := DefaultStyles()
	keys := DefaultKeyMap()
	return App{
		screen:     screenSessions,
		cfg:        cfg,
		st:         st,
		styles:     styles,
		keys:       keys,
		help:       newHelp(),
		sessions:   newSessionsModel(cfg, st, styles, keys),
		newForm:    newNewFormModel(cfg, st, styles, keys),
		killPicker: newKillpickerModel(cfg, st, styles, keys),
	}
}

// AttachTarget returns the session name the user picked for attach (or "" if
// they exited some other way). Read after program.Run() so tui.Run can do the
// post-Quit syscall.Exec into tmux.
func (a App) AttachTarget() string { return a.attachTarget }

// Init dispatches to the active screen's Init.
func (a App) Init() tea.Cmd {
	return a.sessions.Init()
}

// Update handles global keys, resize, and cross-screen messages, then forwards
// to the active screen's Update.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// --- Cross-screen messages (handled regardless of screen) ---
	switch msg := msg.(type) {

	case attachRequestMsg:
		// Sessions screen requested attach. Set target, quit; tui.Run will
		// syscall.Exec into tmux post-Run.
		a.attachTarget = msg.name
		return a, tea.Quit

	case sessionCreatedMsg:
		// NewForm succeeded. Switch back to sessions and force a refresh.
		a.screen = screenSessions
		return a, gatherCmd(a.cfg, a.st)

	case killCompleteMsg:
		// KillPicker finished. Switch back to sessions and force a refresh.
		a.screen = screenSessions
		return a, gatherCmd(a.cfg, a.st)

	case killVictimsReadyMsg:
		// Gather command for kill mode finished — hand off to the picker.
		a.killPicker = a.killPicker.SetVictims(msg.rows)
		return a, nil

	case errMsg:
		a.bannerErr = msg.err
		return a, nil

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.help.Width = msg.Width
		a.sessions.width = msg.Width
		a.sessions.height = msg.Height
		return a, nil

	case tea.KeyMsg:
		// Global keys — handled before screen-specific dispatch.
		switch {
		case keyHit(msg, a.keys.Quit):
			return a, tea.Quit
		case keyHit(msg, a.keys.Help):
			a.showHelp = !a.showHelp
			a.help.ShowAll = a.showHelp
			a.sessions.help.ShowAll = a.showHelp
			return a, nil
		case keyHit(msg, a.keys.Back):
			// Esc on any sub-screen → back to sessions.
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
			}
		}

		// Any non-handled key clears the banner so it doesn't linger.
		a.bannerErr = nil
	}

	// --- Forward to the active screen ---
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
	}
	return a, nil
}

// View renders the active screen and a global help overlay if enabled.
func (a App) View() string {
	var body string
	switch a.screen {
	case screenSessions:
		body = a.sessions.View()
	case screenNewForm:
		body = a.newForm.View()
	case screenKillPicker:
		body = a.killPicker.View()
	default:
		body = a.styles.Muted.Render("(unknown screen)")
	}

	if a.bannerErr != nil {
		body = a.styles.StatusBad.Render("⚠ "+a.bannerErr.Error()) + "\n\n" + body
	}

	if a.showHelp {
		overlay := a.styles.Frame.Render(a.help.View(a.keys))
		body = lipgloss.JoinVertical(lipgloss.Left, body, overlay)
	}
	return body
}
