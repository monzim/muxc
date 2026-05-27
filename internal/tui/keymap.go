package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap centralises every Bubble Tea key binding the TUI uses. Wave 2
// expands this; for now it carries a minimal set so bubbles is a direct dep.
type KeyMap struct {
	Up    key.Binding
	Down  key.Binding
	Quit  key.Binding
	Help  key.Binding
	Enter key.Binding
}

// DefaultKeyMap returns the canonical bindings: arrow keys, vim hjkl, and the
// conventional q/?/enter triad.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
	}
}
