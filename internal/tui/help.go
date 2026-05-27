package tui

import "github.com/charmbracelet/bubbles/help"

// newHelp returns a bubbles/help.Model preconfigured for the TUI's look.
// Screens hold their own Model so they can toggle expanded mode independently.
func newHelp() help.Model {
	h := help.New()
	h.ShowAll = false
	return h
}
