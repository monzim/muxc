package tui

import "github.com/charmbracelet/lipgloss"

// Styles bundles every Lip Gloss style used by the TUI so the look-and-feel
// lives in one place. Wave 2 expands these.
type Styles struct {
	Title     lipgloss.Style
	Subtitle  lipgloss.Style
	StatusOK  lipgloss.Style
	StatusBad lipgloss.Style
}

// DefaultStyles returns the default "modern dark" palette using Lip Gloss
// adaptive colors so the same set looks reasonable on both light and dark
// terminal backgrounds.
func DefaultStyles() Styles {
	return Styles{
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#1F2937", Dark: "#E5E7EB"}),
		Subtitle: lipgloss.NewStyle().
			Faint(true),
		StatusOK: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#22C55E"}),
		StatusBad: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#EF4444"}),
	}
}
