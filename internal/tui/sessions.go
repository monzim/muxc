package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
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
	help   help.Model
}

func newSessionsModel(cfg *config.Config, st *state.State, styles Styles, keys KeyMap) sessionsModel {
	return sessionsModel{
		cfg:    cfg,
		st:     st,
		styles: styles,
		keys:   keys,
		help:   newHelp(),
	}
}

// Init kicks off the initial gather and the auto-refresh tick. The first
// gather typically returns within ~50 ms; the View shows "refreshing…" only
// when a manual `r` is in flight (which the Update handler toggles).
func (m sessionsModel) Init() tea.Cmd {
	return tea.Batch(gatherCmd(m.cfg, m.st), tickCmd())
}

// Update handles messages targeted at the sessions screen. Returns an updated
// model plus optional commands. The root App is responsible for passing
// WindowSizeMsg and dispatching screen-specific messages here.
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
		// Clamp cursor in case the list shrank.
		if m.cursor >= len(m.rows) {
			m.cursor = len(m.rows) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, nil

	case tickMsg:
		// Schedule the next tick and request fresh data.
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

// View renders the sessions screen — header, table, optional banner, footer.
func (m sessionsModel) View() string {
	var b strings.Builder

	header := m.styles.Title.Render("muxc") +
		m.styles.Subtitle.Render(fmt.Sprintf("  ·  sessions (%d)  sort=%s", len(m.rows), sortKeys[m.sortIdx]))
	if m.loading {
		header += m.styles.Muted.Render("  · refreshing…")
	}
	b.WriteString(header)
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(m.styles.StatusBad.Render("error: ") + m.err.Error() + "\n\n")
	}

	if len(m.rows) == 0 {
		b.WriteString(m.styles.Muted.Render("no sessions yet — press 'n' to create one"))
		b.WriteString("\n")
	} else {
		b.WriteString(m.renderTable())
		b.WriteString("\n")
	}

	footerKeys := m.help.View(m.keys)
	b.WriteString(m.styles.Help.Render(footerKeys))

	return b.String()
}

// renderTable draws the columnar session list with a cursor row.
func (m sessionsModel) renderTable() string {
	headers := []string{"NAME", "PROJECT", "CLAUDE", "UPTIME", "IDLE", "MEM", "ATTACHED"}
	colWidths := []int{0, 0, 0, 0, 0, 0, 0}

	// Pre-compute column widths.
	rows := make([][]string, len(m.rows))
	home, _ := os.UserHomeDir()
	for i, r := range m.rows {
		name := r.Name
		if r.IsExternal {
			name = "★ " + name
		}
		project := homeRel(r.ProjectPath, home)
		truncLimit := m.cfg.Display.TruncatePath
		if truncLimit > 0 {
			project = render.LeftTruncate(project, truncLimit)
		}
		claudeCol := claudeDisplay(r)
		row := []string{
			name,
			project,
			claudeCol,
			render.Duration(time.Duration(r.UptimeSeconds) * time.Second),
			render.Duration(time.Duration(r.IdleSeconds) * time.Second),
			render.Bytes(r.RSSPlusChildrenBytes),
			ifelse(r.Attached, "yes", "no"),
		}
		rows[i] = row
		for j, cell := range row {
			if w := lipgloss.Width(cell); w > colWidths[j] {
				colWidths[j] = w
			}
		}
	}
	for j, h := range headers {
		if w := lipgloss.Width(h); w > colWidths[j] {
			colWidths[j] = w
		}
	}

	// Render header.
	var b strings.Builder
	for j, h := range headers {
		b.WriteString(m.styles.TableHeader.Render(padRight(h, colWidths[j])))
	}
	b.WriteString("\n")

	// Render rows.
	for i, row := range rows {
		var cells []string
		for j, cell := range row {
			cells = append(cells, padRight(cell, colWidths[j]))
		}
		line := strings.Join(cells, " ")
		if i == m.cursor {
			b.WriteString(m.styles.TableRowSel.Render(line))
		} else {
			// Apply ExternalBadge tint to the first column of external rows.
			if m.rows[i].IsExternal {
				// Just colour the ★ prefix; lipgloss can't apply mid-string
				// styles per-column, so we re-render the row with no border.
				b.WriteString(m.styles.TableRow.Render(line))
			} else {
				b.WriteString(m.styles.TableRow.Render(line))
			}
		}
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// padRight pads s with spaces to width, accounting for terminal-cell width.
func padRight(s string, width int) string {
	pad := width - lipgloss.Width(s)
	if pad <= 0 {
		return s
	}
	return s + strings.Repeat(" ", pad)
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
