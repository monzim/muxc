package tui

import "github.com/charmbracelet/lipgloss"

// Color palette — adaptive so the same set looks reasonable on both light
// and dark terminal backgrounds.
var (
	colorAccent  = lipgloss.AdaptiveColor{Light: "#3B82F6", Dark: "#60A5FA"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
	colorTitleFG = lipgloss.AdaptiveColor{Light: "#1F2937", Dark: "#E5E7EB"}
	colorOK      = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#22C55E"}
	colorWarn    = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#F59E0B"}
	colorBad     = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#EF4444"}
	colorExt     = lipgloss.AdaptiveColor{Light: "#C2410C", Dark: "#FB923C"} // external session marker
	colorBorder  = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#374151"}
)

// Styles bundles every Lip Gloss style the TUI uses so theming lives in one
// place. Wave 2+ subscreens read from this struct.
type Styles struct {
	// Top header bar.
	Title    lipgloss.Style
	Subtitle lipgloss.Style

	// Status colours for doctor + log messages.
	StatusOK   lipgloss.Style
	StatusWarn lipgloss.Style
	StatusBad  lipgloss.Style

	// Sessions table.
	TableHeader   lipgloss.Style
	TableRow      lipgloss.Style
	TableRowSel   lipgloss.Style
	ExternalBadge lipgloss.Style // "★" prefix on external rows

	// Generic accents.
	Accent lipgloss.Style
	Muted  lipgloss.Style

	// Frame around screens with content boundaries.
	Frame lipgloss.Style

	// Help footer.
	Help lipgloss.Style
}

// DefaultStyles returns the project's "modern dark" palette.
func DefaultStyles() Styles {
	return Styles{
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTitleFG).
			Padding(0, 1),
		Subtitle: lipgloss.NewStyle().
			Foreground(colorMuted),
		StatusOK:   lipgloss.NewStyle().Foreground(colorOK).Bold(true),
		StatusWarn: lipgloss.NewStyle().Foreground(colorWarn).Bold(true),
		StatusBad:  lipgloss.NewStyle().Foreground(colorBad).Bold(true),
		TableHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			Padding(0, 1),
		TableRow: lipgloss.NewStyle().
			Padding(0, 1),
		TableRowSel: lipgloss.NewStyle().
			Padding(0, 1).
			Bold(true).
			Foreground(colorAccent).
			Background(lipgloss.AdaptiveColor{Light: "#E5E7EB", Dark: "#1F2937"}),
		ExternalBadge: lipgloss.NewStyle().Foreground(colorExt).Bold(true),
		Accent:        lipgloss.NewStyle().Foreground(colorAccent),
		Muted:         lipgloss.NewStyle().Foreground(colorMuted),
		Frame: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1),
		Help: lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1),
	}
}
