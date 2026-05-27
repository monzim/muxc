package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap centralises every Bubble Tea key binding the TUI uses. All bindings
// are exposed so bubbles/help can render context-sensitive footers.
type KeyMap struct {
	// Movement (vim + arrows).
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding

	// Common actions.
	Enter   key.Binding
	Back    key.Binding
	Quit    key.Binding
	Help    key.Binding
	Refresh key.Binding

	// Sessions screen actions.
	Attach key.Binding
	New    key.Binding
	Kill   key.Binding
	Info   key.Binding
	Sort   key.Binding

	// Global screen switching.
	Doctor key.Binding
	Mem    key.Binding
}

// DefaultKeyMap returns the canonical bindings: arrow keys, vim hjkl, plus
// single-letter shortcuts for the actions on the sessions screen.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:  key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
		Right: key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),

		Enter:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Back:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),

		Attach: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "attach")),
		New:    key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		Kill:   key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "kill")), // capital K to avoid clash with k/j navigation
		Info:   key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "info")),
		Sort:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort")),

		Doctor: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "doctor")),
		Mem:    key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mem")),
	}
}

// keyHit is a short helper so screen code reads naturally:
//
//	if keyHit(msg, a.keys.Quit) { ... }
//
// instead of `if key.Matches(msg, a.keys.Quit)` everywhere. Lowercase to
// signal it's internal.
func keyHit(msg interface{ String() string }, b key.Binding) bool {
	for _, k := range b.Keys() {
		if msg.String() == k {
			return true
		}
	}
	return false
}

// ShortHelp returns the single-row help shown by bubbles/help in compact mode.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Help, k.Quit}
}

// FullHelp returns multi-column help shown when the user presses '?'.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right, k.Enter, k.Back},
		{k.Attach, k.New, k.Kill, k.Info, k.Sort, k.Refresh},
		{k.Doctor, k.Mem, k.Help, k.Quit},
	}
}
