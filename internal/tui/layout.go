package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// layout composes the canonical screen shell: header bar + body card +
// status bar. Every screen uses this so the chrome stays consistent.
//
// Caller passes the screen-specific pieces:
//
//	crumb   short breadcrumb after "muxc ·" (e.g. "sessions (8)")
//	right   right-aligned header text (e.g. sort key, refresh indicator)
//	body    pre-rendered screen body (the table, form, etc.)
//	status  status-bar segments to render at the bottom
//	width   terminal width — used to right-align the header right segment
type layout struct {
	styles Styles
	width  int
}

func newLayout(s Styles, width int) layout {
	return layout{styles: s, width: width}
}

// header renders the top bar as a single line, full terminal width:
//
//	muxc · sessions (8)                                 sort=name · ↻ 2s
//
// If the right side doesn't fit, it's dropped rather than wrapped — keeps
// the header to exactly one row at all terminal widths.
func (l layout) header(crumb, right string) string {
	logo := l.styles.Logo.Render("muxc")
	sep := l.styles.Faint.Render(" · ")
	left := logo + sep + l.styles.HeaderCrumb.Render(crumb)

	if l.width <= 0 {
		if right != "" {
			return left + "  " + l.styles.HeaderRight.Render(right)
		}
		return left
	}

	leftWidth := lipgloss.Width(left)
	// 2 = sum of left/right padding (1 each) on the Header style.
	available := l.width - leftWidth - 2

	rightRendered := l.styles.HeaderRight.Render(right)
	rightWidth := lipgloss.Width(rightRendered)

	// Drop the right side entirely if it would force a wrap; we'd rather
	// hide the refresh indicator than break the header into two rows.
	if rightWidth+1 > available {
		rightRendered = ""
		rightWidth = 0
	}

	gap := available - rightWidth
	if gap < 0 {
		gap = 0
	}

	body := left + strings.Repeat(" ", gap) + rightRendered

	return l.styles.Header.
		Width(l.width).
		MaxHeight(1).
		Render(body)
}

// statusBar renders the bottom strip of context-sensitive shortcuts.
// segments is a slice of (key, label) pairs.
//
//	{"↑↓", "select"} {"enter", "info"} {"a", "attach"} ...
//
// Returns a single line that's rendered straight, no border.
func (l layout) statusBar(segments []statusSeg) string {
	if len(segments) == 0 {
		return ""
	}
	var parts []string
	for _, s := range segments {
		parts = append(parts,
			l.styles.StatusKey.Render(s.key)+" "+l.styles.StatusBar.Render(s.label))
	}
	sep := l.styles.StatusSep.Render(" • ")
	body := strings.Join(parts, sep)
	if l.width > 0 {
		return l.styles.StatusBar.Width(l.width).Render(body)
	}
	return l.styles.StatusBar.Render(body)
}

// compose stacks header + body + status with a blank line between body and
// status (visual breathing room).
func (l layout) compose(header, body string, status []statusSeg) string {
	statusLine := l.statusBar(status)
	if statusLine == "" {
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, "", statusLine)
}

// statusSeg is one (key, label) entry for the status bar.
type statusSeg struct {
	key   string
	label string
}

// padRow pads a pre-rendered line with trailing spaces so its visual width
// matches `width`. Returns line unchanged if it's already at or beyond width.
// Used by every screen that renders selectable rows so the selection
// background fills the entire band, not just the text it contains.
func padRow(line string, width int) string {
	have := lipgloss.Width(line)
	if have >= width {
		return line
	}
	return line + strings.Repeat(" ", width-have)
}

// cardInnerWidth returns the width available inside a Card style for a
// terminal of width `total`. Accounts for the rounded border (2 cols) plus
// the Card's horizontal padding (2 cols).
func cardInnerWidth(total int) int {
	w := total - 6 // -2 outer margin (compose), -2 border, -2 padding
	if w < 30 {
		w = 30
	}
	return w
}

// cardWidth returns the outer width of the body card given a total terminal
// width — used as the .Width() argument when rendering the Card.
func cardWidth(total int) int {
	w := total - 2
	if w < 40 {
		w = 40
	}
	return w
}
