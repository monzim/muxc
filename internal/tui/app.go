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

	// Per-screen models.
	sessions sessionsModel
}

// NewApp constructs the initial root model.
func NewApp(cfg *config.Config, st *state.State) App {
	styles := DefaultStyles()
	keys := DefaultKeyMap()
	return App{
		screen:   screenSessions,
		cfg:      cfg,
		st:       st,
		styles:   styles,
		keys:     keys,
		help:     newHelp(),
		sessions: newSessionsModel(cfg, st, styles, keys),
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

// Update handles global keys and resize, then forwards to the active screen.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.help.Width = msg.Width
		a.sessions.width = msg.Width
		a.sessions.height = msg.Height
		return a, nil

	case tea.KeyMsg:
		// Global keys first.
		switch {
		case keyHit(msg, a.keys.Quit):
			return a, tea.Quit
		case keyHit(msg, a.keys.Help):
			a.showHelp = !a.showHelp
			a.help.ShowAll = a.showHelp
			a.sessions.help.ShowAll = a.showHelp
			return a, nil
		}
	}

	// Forward to active screen.
	switch a.screen {
	case screenSessions:
		next, cmd := a.sessions.Update(msg)
		a.sessions = next
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
	default:
		body = a.styles.Muted.Render("(unknown screen)")
	}

	if a.showHelp {
		overlay := a.styles.Frame.Render(a.help.View(a.keys))
		body = lipgloss.JoinVertical(lipgloss.Left, body, overlay)
	}
	return body
}
