package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/render"
	"github.com/monzim/muxc/internal/session"
	"github.com/monzim/muxc/internal/state"
)

// sortKeys is the cycle order for the `s` key on the sessions screen.
var sortKeys = []string{"name", "mem", "idle", "created"}

// sessionsModel is the live session list — the default landing screen.
type sessionsModel struct {
	cfg *config.Config
	st  *state.State

	rows        []session.Row
	cursor      int
	sortIdx     int
	loading     bool
	err         error
	lastRefresh time.Time

	width  int
	height int

	styles Styles
	keys   KeyMap
}

func newSessionsModel(cfg *config.Config, st *state.State, styles Styles, keys KeyMap) sessionsModel {
	return sessionsModel{
		cfg:    cfg,
		st:     st,
		styles: styles,
		keys:   keys,
	}
}

func (m sessionsModel) Init() tea.Cmd {
	return tea.Batch(gatherCmd(m.cfg, m.st), tickCmd())
}

func (m sessionsModel) Update(msg tea.Msg) (sessionsModel, tea.Cmd) {
	switch msg := msg.(type) {

	case gatherCompleteMsg:
		m.loading = false
		m.lastRefresh = msg.when
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.rows = msg.rows
		session.SortRows(m.rows, sortKeys[m.sortIdx])
		if m.cursor >= len(m.rows) {
			m.cursor = len(m.rows) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(gatherCmd(m.cfg, m.st), tickCmd())

	case tea.KeyMsg:
		switch {
		case keyHit(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case keyHit(msg, m.keys.Down):
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
		case keyHit(msg, m.keys.Refresh):
			m.loading = true
			return m, gatherCmd(m.cfg, m.st)
		case keyHit(msg, m.keys.Sort):
			m.sortIdx = (m.sortIdx + 1) % len(sortKeys)
			session.SortRows(m.rows, sortKeys[m.sortIdx])
		}
	}
	return m, nil
}

// View renders the modern sessions screen: header bar, big bordered card with
// the table inside, and a single status bar at the bottom.
func (m sessionsModel) View() string {
	l := newLayout(m.styles, m.width)

	// ── Header ──
	crumb := fmt.Sprintf("sessions (%d)", len(m.rows))
	// Keep the right side single-line by using compact tokens. The Header bar
	// has Width set so anything wider than the row wraps; "↻ 2 minutes ago"
	// is too long, so we shorten to "↻ <compact dur>".
	rightBits := []string{m.styles.Muted.Render("sort=") + m.styles.Strong.Render(sortKeys[m.sortIdx])}
	if m.loading {
		rightBits = append(rightBits, m.styles.HeaderRefresh.Render("◌ refreshing"))
	} else if !m.lastRefresh.IsZero() {
		age := time.Since(m.lastRefresh)
		rightBits = append(rightBits, m.styles.Muted.Render("↻ "+render.Duration(age)))
	}
	header := l.header(crumb, strings.Join(rightBits, "  "))

	// ── Body card ──
	inner := cardInnerWidth(m.width)

	var body string
	if m.err != nil {
		body = m.styles.StatusBad.Render("⚠ error: ") + m.err.Error()
	} else if len(m.rows) == 0 {
		body = m.styles.Muted.Render("no sessions yet")
		body += "\n\n"
		body += m.styles.Faint.Render("press ") + m.styles.Accent.Render("n") +
			m.styles.Faint.Render(" to create one, or run a tmux session running ") +
			m.styles.Strong.Render("claude") +
			m.styles.Faint.Render(" — it will appear here automatically")
	} else {
		body = m.renderTable(inner)
	}

	card := m.styles.Card.Width(cardWidth(m.width)).Render(body)

	// ── Status bar ──
	status := []statusSeg{
		{"↑↓/jk", "select"},
		{"enter", "info"},
		{"a", "attach"},
		{"n", "new"},
		{"K", "kill"},
		{"s", "sort"},
		{"r", "refresh"},
		{"d", "doctor"},
		{"?", "help"},
		{"q", "quit"},
	}

	return l.compose(header, card, status)
}

// renderTable lays out the sessions table inside the body card. innerWidth
// is the available width inside the card (after borders + padding) — used
// to stretch the selection bar across the entire row.
func (m sessionsModel) renderTable(innerWidth int) string {
	headers := []string{"NAME", "PROJECT", "CLAUDE", "UPTIME", "IDLE", "MEM", "ATTACHED"}
	rights := map[int]bool{5: true} // MEM right-aligned

	colWidths := make([]int, len(headers))
	for j, h := range headers {
		colWidths[j] = lipgloss.Width(h)
	}

	// Pre-compute styled cells and widths.
	home, _ := os.UserHomeDir()
	type styledRow struct {
		cells    []string
		external bool
	}
	rows := make([]styledRow, len(m.rows))

	for i, r := range m.rows {
		// NAME: star prefix for external, name in primary color when selected.
		nameCell := r.Name
		if r.IsExternal {
			nameCell = m.styles.BadgeExternal.Render("★ ") + r.Name
		} else {
			nameCell = "  " + r.Name
		}

		project := homeRel(r.ProjectPath, home)
		truncLimit := m.cfg.Display.TruncatePath
		if truncLimit > 0 {
			project = render.LeftTruncate(project, truncLimit)
		}
		if project == "" {
			project = m.styles.Faint.Render("(unknown)")
		}

		claudeCol := claudeDisplay(r)
		if claudeCol == "-" {
			claudeCol = m.styles.Faint.Render("—")
		} else {
			claudeCol = m.styles.Secondary.Render(claudeCol)
		}

		uptime := render.Duration(time.Duration(r.UptimeSeconds) * time.Second)
		idle := render.Duration(time.Duration(r.IdleSeconds) * time.Second)
		mem := render.Bytes(r.RSSPlusChildrenBytes)

		attached := m.styles.Faint.Render("no")
		if r.Attached {
			attached = m.styles.BadgeAttached.Render("● yes")
		}

		cells := []string{nameCell, project, claudeCol, uptime, idle, mem, attached}
		rows[i] = styledRow{cells: cells, external: r.IsExternal}

		for j, c := range cells {
			if w := lipgloss.Width(c); w > colWidths[j] {
				colWidths[j] = w
			}
		}
	}

	// ── Header line ──
	var b strings.Builder
	headerCells := make([]string, len(headers))
	for j, h := range headers {
		headerCells[j] = padCol(h, colWidths[j], rights[j])
	}
	b.WriteString(m.styles.TableHeader.Render(strings.Join(headerCells, "  ")))
	b.WriteString("\n")
	b.WriteString(m.styles.Faint.Render(strings.Repeat("─", sumWidths(colWidths)+2*(len(colWidths)-1))))
	b.WriteString("\n")

	// ── Body rows ──
	// Each row gets padded to innerWidth so the selection highlight stretches
	// across the entire visible band, not just the cells.
	for i, row := range rows {
		cells := make([]string, len(row.cells))
		for j, c := range row.cells {
			cells[j] = padCol(c, colWidths[j], rights[j])
		}
		line := padRow(strings.Join(cells, "  "), innerWidth)
		if i == m.cursor {
			line = m.styles.TableRowSel.Render(line)
		}
		b.WriteString(line)
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// sumWidths totals an int slice.
func sumWidths(ws []int) int {
	total := 0
	for _, w := range ws {
		total += w
	}
	return total
}

// padCol pads s to width with spaces. If right, pads on the left so values
// hug the right edge (useful for MEM).
func padCol(s string, width int, right bool) string {
	pad := width - lipgloss.Width(s)
	if pad <= 0 {
		return s
	}
	gap := strings.Repeat(" ", pad)
	if right {
		return gap + s
	}
	return s + gap
}

// padRight pads s with spaces to width, accounting for terminal-cell width.
// Kept as an exported helper since tui_test.go still uses it.
func padRight(s string, width int) string {
	return padCol(s, width, false)
}

// claudeDisplay returns the CLAUDE column value with the same precedence as
// the CLI table (name > truncated id > "-").
func claudeDisplay(r session.Row) string {
	if r.ClaudeSessionName != "" {
		return r.ClaudeSessionName
	}
	if r.ClaudeSessionID != "" {
		id := r.ClaudeSessionID
		if len(id) > 8 {
			return id[:8] + "…"
		}
		return id
	}
	return "-"
}

// homeRel mirrors internal/cli's homeRelative without reaching back into the
// cli package (which would re-introduce the import cycle).
func homeRel(path, home string) string {
	if home == "" || path == "" {
		return path
	}
	if strings.HasPrefix(path, home+"/") {
		return "~/" + path[len(home)+1:]
	}
	if path == home {
		return "~"
	}
	return path
}

func ifelse(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
