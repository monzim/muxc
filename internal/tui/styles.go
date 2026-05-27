package tui

import "github.com/charmbracelet/lipgloss"

// Palette — Claude-inspired warm tones, adaptive across light and dark.
// Anchored on Claude's signature terracotta/orange. The palette is intentional
// about contrast: primary is bold enough to ride on top of dark backgrounds
// without straining readability, and the selection background is a warm
// inverse of the primary so the row reads as "lit up" rather than greyed out.
var (
	// Primary — Claude's signature warm orange ("clay").
	colorPrimary = lipgloss.AdaptiveColor{Light: "#CC785C", Dark: "#E89A85"}
	// PrimarySoft — a lighter shade used for less assertive accents.
	colorPrimarySoft = lipgloss.AdaptiveColor{Light: "#D49581", Dark: "#F0B8A0"}
	// Secondary — amber, used for headings and section labels.
	colorSecondary = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FDBA74"}
	// Selection background — warm tinted band that runs the full row width.
	colorSelBG = lipgloss.AdaptiveColor{Light: "#FCEDE4", Dark: "#3A241B"}
	// Selection foreground — high-contrast warm cream on dark, deep brown on light.
	colorSelFG = lipgloss.AdaptiveColor{Light: "#7C2D12", Dark: "#FFE4D3"}

	// Foregrounds.
	colorFG      = lipgloss.AdaptiveColor{Light: "#1C1917", Dark: "#F5F1E8"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#78716C", Dark: "#A8A29E"}
	colorFaint   = lipgloss.AdaptiveColor{Light: "#A8A29E", Dark: "#78716C"}
	colorInverse = lipgloss.AdaptiveColor{Light: "#FFFBF5", Dark: "#1C1917"}

	// Status colours — kept tonally consistent with the warm palette but
	// distinct enough to read as semantic signals.
	colorOK   = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#86EFAC"}
	colorWarn = lipgloss.AdaptiveColor{Light: "#A16207", Dark: "#FCD34D"}
	colorBad  = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#FCA5A5"}
	// External marker — slightly more saturated than primary so it stands out
	// without competing with the selection band.
	colorExt = lipgloss.AdaptiveColor{Light: "#9A3412", Dark: "#FB923C"}

	// Structural lines — warm beige border so it harmonises with the palette.
	colorBorder      = lipgloss.AdaptiveColor{Light: "#E7DDD0", Dark: "#44403C"}
	colorBorderFocus = colorPrimary

	// Header background — paper-ish on light, charcoal on dark.
	colorHeaderBG = lipgloss.AdaptiveColor{Light: "#F8F3EC", Dark: "#1A1612"}
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
			Foreground(colorMuted),
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
			Foreground(colorSecondary),
		TableRow: lipgloss.NewStyle().
			Foreground(colorFG),
		// TableRowSel: full-row tinted band. Width is applied at render time
		// in sessions.go so the highlight stretches the entire card width
		// rather than only the text it contains.
		TableRowSel: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSelFG).
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

// keep colorPrimarySoft / colorInverse exported via package so they can be
// referenced by future screens without needing to import them.
var _ = colorPrimarySoft
