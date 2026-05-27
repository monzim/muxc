package tui

import (
	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/state"
)

// App is the root tea.Model. Wave 3 will fan this out into per-screen models;
// Wave 2 wires the title bar, help footer, and the global keymap so we can
// verify the TUI launches, intercepts q/ctrl+c cleanly, and restores the
// terminal on exit.
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
}

// NewApp constructs the initial root model.
func NewApp(cfg *config.Config, st *state.State) App {
	return App{
		screen: screenSessions,
		cfg:    cfg,
		st:     st,
		styles: DefaultStyles(),
		keys:   DefaultKeyMap(),
		help:   newHelp(),
	}
}

// AttachTarget returns the session name the user picked for attach (or "" if
// they exited some other way). Read after program.Run() so tui.Run can do the
// post-Quit syscall.Exec into tmux.
func (a App) AttachTarget() string { return a.attachTarget }

func (a App) Init() tea.Cmd {
	// Wave 3 returns sessions.Init() here.
	return nil
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.help.Width = msg.Width
		return a, nil

	case tea.KeyMsg:
		switch {
		case keyHit(msg, a.keys.Quit):
			return a, tea.Quit
		case keyHit(msg, a.keys.Help):
			a.showHelp = !a.showHelp
			a.help.ShowAll = a.showHelp
			return a, nil
		}
	}
	return a, nil
}

func (a App) View() string {
	header := a.styles.Title.Render("muxc") + a.styles.Subtitle.Render("  ·  sessions")
	body := a.styles.Frame.Render(
		"Wave 2 skeleton — full sessions screen lands in Wave 3.\n\n" +
			"Press '?' for help, 'q' to quit.",
	)
	footer := a.styles.Help.Render(a.help.View(a.keys))
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}
