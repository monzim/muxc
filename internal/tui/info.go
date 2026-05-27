package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/monzim/muxc/internal/claude"
	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/proc"
	"github.com/monzim/muxc/internal/render"
	"github.com/monzim/muxc/internal/session"
)

// infoModel shows the full details of a single session: project, claude id,
// transcript path, process tree, and a scrollable tail of the JSONL
// transcript. ↑↓/PgUp/PgDn scroll the viewport; +/- adjust how many entries
// the transcript shows (capped at 100, spec §11.6).
type infoModel struct {
	cfg *config.Config

	row            session.Row
	procLines      []string
	transcriptPath string
	transcript     []claude.Entry
	tailLines      int

	viewport viewport.Model

	width  int
	height int

	styles Styles
	keys   KeyMap
}

func newInfoModel(cfg *config.Config, styles Styles, keys KeyMap) infoModel {
	vp := viewport.New(0, 0)
	return infoModel{
		cfg:       cfg,
		viewport:  vp,
		tailLines: 20,
		styles:    styles,
		keys:      keys,
	}
}

// setRow primes the model for a freshly-selected session. Called by the App
// when transitioning to the info screen.
func (m *infoModel) setRow(row session.Row) {
	m.row = row
	m.procLines = nil
	m.transcript = nil
	m.transcriptPath = ""
}

func (m infoModel) Init() tea.Cmd {
	return tea.Batch(buildProcTreeCmd(m.row), loadTranscriptCmd(m.cfg, m.row, m.tailLines))
}

// procTreeMsg carries the rendered process tree lines.
type procTreeMsg struct{ lines []string }

// transcriptMsg carries the transcript tail + the resolved file path.
type transcriptMsg struct {
	path    string
	entries []claude.Entry
}

func buildProcTreeCmd(row session.Row) tea.Cmd {
	return func() tea.Msg {
		tree, err := proc.BuildTree()
		if err != nil || row.PanePID == 0 {
			return procTreeMsg{lines: []string{"(process tree unavailable)"}}
		}
		all := append([]int{row.PanePID}, tree.Descendants(row.PanePID)...)
		lines := make([]string, 0, len(all))
		for _, pid := range all {
			cmdline, _ := tree.Cmdline(pid)
			cmdStr := strings.Join(cmdline, " ")
			if cmdStr == "" {
				cmdStr = "(no cmdline)"
			}
			rss, _ := tree.RSS(pid)
			lines = append(lines, fmt.Sprintf("%6d  %10s  %s", pid, render.Bytes(rss), cmdStr))
		}
		return procTreeMsg{lines: lines}
	}
}

func loadTranscriptCmd(cfg *config.Config, row session.Row, n int) tea.Cmd {
	return func() tea.Msg {
		if row.ProjectPath == "" {
			return transcriptMsg{}
		}
		sess, err := claude.LatestSession(cfg.Paths.ClaudeProjects, row.ProjectPath)
		if err != nil || sess == nil {
			return transcriptMsg{}
		}
		entries, err := claude.TailTranscript(sess.TranscriptPath, n)
		if err != nil {
			return transcriptMsg{path: sess.TranscriptPath}
		}
		return transcriptMsg{path: sess.TranscriptPath, entries: entries}
	}
}

// suppress: avoid unused-import warning when context is only referenced in
// nested closures during certain rebuild paths.
var _ = context.Background

func (m infoModel) Update(msg tea.Msg) (infoModel, tea.Cmd) {
	switch msg := msg.(type) {
	case procTreeMsg:
		m.procLines = msg.lines
		m.refreshViewport()
		return m, nil
	case transcriptMsg:
		m.transcriptPath = msg.path
		m.transcript = msg.entries
		m.refreshViewport()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "+", "=":
			if m.tailLines < 100 {
				m.tailLines += 10
				if m.tailLines > 100 {
					m.tailLines = 100
				}
				return m, loadTranscriptCmd(m.cfg, m.row, m.tailLines)
			}
		case "-", "_":
			if m.tailLines > 0 {
				m.tailLines -= 10
				if m.tailLines < 0 {
					m.tailLines = 0
				}
				return m, loadTranscriptCmd(m.cfg, m.row, m.tailLines)
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Card subtracts: borders (2) + padding (2). Viewport sits inside,
		// minus header (1) + status (2) + breathing rows.
		m.viewport.Width = msg.Width - 6
		m.viewport.Height = msg.Height - 14
		if m.viewport.Height < 6 {
			m.viewport.Height = 6
		}
		m.refreshViewport()
		return m, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// refreshViewport rebuilds the scrollable content (key/value block + process
// tree + transcript) and pushes it into the viewport.
func (m *infoModel) refreshViewport() {
	r := m.row
	var b strings.Builder

	// ── Identity card ──
	b.WriteString(m.styles.Secondary.Render("┐ SESSION") + "\n")
	kv := func(k, v string) {
		if v == "" {
			v = m.styles.Faint.Render("—")
		}
		b.WriteString("  " + m.styles.KvKey.Render(k) + "  " + m.styles.KvValue.Render(v) + "\n")
	}
	nameDisplay := r.Name
	if r.IsExternal {
		nameDisplay = m.styles.BadgeExternal.Render("★ ") + r.Name + " " + m.styles.Faint.Render("(external)")
	}
	kv("name", nameDisplay)
	kv("project", r.ProjectPath)
	kv("created", r.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	kv("uptime", render.Duration(time.Duration(r.UptimeSeconds)*time.Second))
	kv("idle", render.Duration(time.Duration(r.IdleSeconds)*time.Second))
	attached := m.styles.Faint.Render("no")
	if r.Attached {
		attached = m.styles.BadgeAttached.Render(fmt.Sprintf("● yes (%d client(s))", r.AttachedClients))
	}
	kv("attached", attached)
	kv("tmux id", r.TmuxSessionID)

	b.WriteString("\n")

	// ── Claude block ──
	b.WriteString(m.styles.Secondary.Render("┐ CLAUDE") + "\n")
	claudeName := r.ClaudeSessionName
	if claudeName == "" {
		claudeName = m.styles.Faint.Render("(no name)")
	}
	kv("name", claudeName)
	kv("session id", r.ClaudeSessionID)
	if r.ClaudePID > 0 {
		kv("pid", fmt.Sprintf("%d", r.ClaudePID))
	} else {
		kv("pid", m.styles.Faint.Render("(not detected)"))
	}
	kv("rss (claude)", render.Bytes(r.RSSBytes))
	kv("rss (+kids)", render.Bytes(r.RSSPlusChildrenBytes))
	kv("transcript", m.transcriptPath)

	b.WriteString("\n")

	// ── Process tree ──
	if len(m.procLines) > 0 {
		b.WriteString(m.styles.Secondary.Render("┐ PROCESS TREE") + "\n")
		b.WriteString("  " + m.styles.Faint.Render(fmt.Sprintf("%6s  %10s  %s", "pid", "rss", "cmdline")) + "\n")
		for _, line := range m.procLines {
			b.WriteString("  " + line + "\n")
		}
		b.WriteString("\n")
	}

	// ── Transcript tail ──
	b.WriteString(m.styles.Secondary.Render(fmt.Sprintf("┐ TRANSCRIPT TAIL (%d lines, +/- to adjust)", m.tailLines)) + "\n")
	if len(m.transcript) == 0 {
		b.WriteString("  " + m.styles.Faint.Render("(no transcript or session never written to)") + "\n")
	} else {
		for _, e := range m.transcript {
			tag := m.styles.Accent.Render("[" + e.Role + "]")
			b.WriteString("  " + tag + " " + e.Content + "\n")
		}
	}

	m.viewport.SetContent(b.String())
}

func (m infoModel) View() string {
	l := newLayout(m.styles, m.width)

	header := l.header("info · "+m.row.Name, "")
	card := m.styles.Card.Width(cardWidth(m.width)).Render(m.viewport.View())

	status := []statusSeg{
		{"↑↓/jk", "scroll"},
		{"+/-", "transcript lines"},
		{"a", "attach"},
		{"esc", "back"},
		{"q", "quit"},
	}
	return l.compose(header, card, status)
}

// unused (kept for future per-row styling)
var _ = lipgloss.NormalBorder
