package tui

import "github.com/charmbracelet/lipgloss"

// Palette — adaptive so the same set looks right on both light and dark
// terminal backgrounds. Inspired by gh dash, lazygit, k9s.
var (
	// Primary accent for selections, focus, active borders.
	colorPrimary = lipgloss.AdaptiveColor{Light: "#5B6CFF", Dark: "#7C8CFF"}
	// Secondary accent for headings and badges.
	colorSecondary = lipgloss.AdaptiveColor{Light: "#A855F7", Dark: "#C084FC"}
	// Subtle background tint for selected rows / cards.
	colorSelBG = lipgloss.AdaptiveColor{Light: "#E0E7FF", Dark: "#1E2240"}

	// Foregrounds.
	colorFG      = lipgloss.AdaptiveColor{Light: "#0F172A", Dark: "#E2E8F0"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"}
	colorFaint   = lipgloss.AdaptiveColor{Light: "#94A3B8", Dark: "#64748B"}
	colorInverse = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#0F172A"}

	// Status colours.
	colorOK   = lipgloss.AdaptiveColor{Light: "#059669", Dark: "#10B981"}
	colorWarn = lipgloss.AdaptiveColor{Light: "#D97706", Dark: "#F59E0B"}
	colorBad  = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F87171"}
	colorExt  = lipgloss.AdaptiveColor{Light: "#EA580C", Dark: "#FB923C"}

	// Structural lines.
	colorBorder      = lipgloss.AdaptiveColor{Light: "#CBD5E1", Dark: "#334155"}
	colorBorderFocus = colorPrimary

	// Header background.
	colorHeaderBG = lipgloss.AdaptiveColor{Light: "#F1F5F9", Dark: "#0B1224"}
)

// Styles bundles every Lip Gloss style the TUI uses so theming lives in one
// place and screens stay consistent.
type Styles struct {
	// Header bar (top of every screen).
	Header        lipgloss.Style
	HeaderTitle   lipgloss.Style
	HeaderCrumb   lipgloss.Style
	HeaderRight   lipgloss.Style
	HeaderRefresh lipgloss.Style

	// Status bar (bottom of every screen). Pre-styled segments.
	StatusBar lipgloss.Style
	StatusKey lipgloss.Style
	StatusSep lipgloss.Style

	// Content area — large bordered card holding the screen body.
	Card        lipgloss.Style
	CardFocused lipgloss.Style

	// Table cells.
	TableHeader lipgloss.Style
	TableRow    lipgloss.Style
	TableRowSel lipgloss.Style

	// Inline badges.
	BadgeExternal lipgloss.Style
	BadgeAttached lipgloss.Style
	BadgeIdle     lipgloss.Style

	// Status labels.
	StatusOK   lipgloss.Style
	StatusWarn lipgloss.Style
	StatusBad  lipgloss.Style

	// Accents.
	Accent    lipgloss.Style
	Secondary lipgloss.Style
	Muted     lipgloss.Style
	Faint     lipgloss.Style
	Strong    lipgloss.Style

	// Key/value blocks (info screen).
	KvKey   lipgloss.Style
	KvValue lipgloss.Style

	// Form pieces.
	FormLabel      lipgloss.Style
	FormInputFocus lipgloss.Style
	FormBtn        lipgloss.Style
	FormBtnFocus   lipgloss.Style

	// Modal overlay for the help screen.
	Modal       lipgloss.Style
	ModalHeader lipgloss.Style

	// Logo styling used in the header.
	Logo lipgloss.Style
}

// DefaultStyles returns the project palette. Adaptive colours mean the same
// instance looks right on both light and dark terminals.
func DefaultStyles() Styles {
	return Styles{
		Header: lipgloss.NewStyle().
			Padding(0, 1).
			Background(colorHeaderBG).
			Foreground(colorFG),
		HeaderTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary),
		HeaderCrumb: lipgloss.NewStyle().
			Foreground(colorMuted),
		HeaderRight: lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1),
		HeaderRefresh: lipgloss.NewStyle().
			Foreground(colorSecondary).
			Bold(true),

		StatusBar: lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(colorMuted),
		StatusKey: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary),
		StatusSep: lipgloss.NewStyle().
			Foreground(colorFaint),

		Card: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1),
		CardFocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorderFocus).
			Padding(0, 1),

		TableHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSecondary).
			Padding(0, 0),
		TableRow: lipgloss.NewStyle().
			Padding(0, 0).
			Foreground(colorFG),
		TableRowSel: lipgloss.NewStyle().
			Padding(0, 0).
			Bold(true).
			Foreground(colorPrimary).
			Background(colorSelBG),

		BadgeExternal: lipgloss.NewStyle().
			Foreground(colorExt).
			Bold(true),
		BadgeAttached: lipgloss.NewStyle().
			Foreground(colorOK).
			Bold(true),
		BadgeIdle: lipgloss.NewStyle().
			Foreground(colorFaint),

		StatusOK:   lipgloss.NewStyle().Foreground(colorOK).Bold(true),
		StatusWarn: lipgloss.NewStyle().Foreground(colorWarn).Bold(true),
		StatusBad:  lipgloss.NewStyle().Foreground(colorBad).Bold(true),

		Accent:    lipgloss.NewStyle().Foreground(colorPrimary).Bold(true),
		Secondary: lipgloss.NewStyle().Foreground(colorSecondary),
		Muted:     lipgloss.NewStyle().Foreground(colorMuted),
		Faint:     lipgloss.NewStyle().Foreground(colorFaint),
		Strong:    lipgloss.NewStyle().Foreground(colorFG).Bold(true),

		KvKey: lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(14),
		KvValue: lipgloss.NewStyle().
			Foreground(colorFG),

		FormLabel: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSecondary),
		FormInputFocus: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(colorPrimary),
		FormBtn: lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(colorMuted).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder),
		FormBtnFocus: lipgloss.NewStyle().
			Padding(0, 2).
			Bold(true).
			Foreground(colorInverse).
			Background(colorPrimary).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary),

		Modal: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2),
		ModalHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			MarginBottom(1),

		Logo: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary),
	}
}
