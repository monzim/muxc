package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/state"
)

// App is the root tea.Model. It owns the active screen and shared state
// (config, last gather result, attach handoff target) and dispatches messages
// to the currently-focused screen's model.
//
// Implementation lands in Wave 3. This stub keeps the package compiling so
// other waves can build on it incrementally.
type App struct {
	screen       screenID
	cfg          *config.Config
	st           *state.State
	width        int
	height       int
	attachTarget string // non-empty if user chose attach; main reads after Quit
}

// NewApp constructs the initial root model.
func NewApp(cfg *config.Config, st *state.State) App {
	return App{
		screen: screenSessions,
		cfg:    cfg,
		st:     st,
	}
}

// AttachTarget returns the session name the user picked for attach (or "" if
// they exited some other way). Read after program.Run() so tui.Run can do the
// post-Quit syscall.Exec into tmux.
func (a App) AttachTarget() string { return a.attachTarget }

func (a App) Init() tea.Cmd { return nil }

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Wave 3 wires the real state machine here.
	if _, ok := msg.(tea.KeyMsg); ok {
		return a, tea.Quit
	}
	return a, nil
}

func (a App) View() string {
	return "muxc tui — Wave 1 skeleton (press any key to quit)\n"
}
