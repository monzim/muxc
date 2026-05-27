package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

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
	procLines      []string // pre-rendered process tree lines
	transcriptPath string
	transcript     []claude.Entry
	tailLines      int

	viewport viewport.Model

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
			lines = append(lines, fmt.Sprintf("  %d  %s  %s", pid, render.Bytes(rss), cmdStr))
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
		// Leave room for header + footer (~6 lines).
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 6
		if m.viewport.Height < 5 {
			m.viewport.Height = 5
		}
		m.refreshViewport()
		return m, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *infoModel) refreshViewport() {
	var b strings.Builder

	// Session block.
	r := m.row
	b.WriteString(m.styles.Subtitle.Render("session") + "\n")
	kv := func(k, v string) {
		b.WriteString(fmt.Sprintf("  %s  %s\n", m.styles.Muted.Render(k+":"), v))
	}
	kv("name       ", r.Name)
	kv("project    ", r.ProjectPath)
	kv("created    ", r.CreatedAt.Local().Format(time.RFC3339))
	kv("idle       ", render.Duration(time.Duration(r.IdleSeconds)*time.Second))
	kv("mem        ", render.Bytes(r.RSSPlusChildrenBytes))
	kv("pane pid   ", fmt.Sprintf("%d", r.PanePID))
	kv("claude pid ", fmt.Sprintf("%d", r.ClaudePID))
	kv("claude name", r.ClaudeSessionName)
	kv("claude id  ", r.ClaudeSessionID)

	// Process tree.
	if len(m.procLines) > 0 {
		b.WriteString("\n" + m.styles.Subtitle.Render("process tree") + "\n")
		for _, line := range m.procLines {
			b.WriteString(line + "\n")
		}
	}

	// Transcript.
	b.WriteString("\n" + m.styles.Subtitle.Render(fmt.Sprintf("transcript (tail %d, +/- to adjust)", m.tailLines)) + "\n")
	if m.transcriptPath != "" {
		b.WriteString(m.styles.Muted.Render("  "+m.transcriptPath) + "\n\n")
	}
	if len(m.transcript) == 0 {
		b.WriteString(m.styles.Muted.Render("  (no transcript or session never written)") + "\n")
	} else {
		for _, e := range m.transcript {
			b.WriteString("  " + m.styles.Accent.Render("["+e.Role+"]") + " " + e.Content + "\n")
		}
	}

	m.viewport.SetContent(b.String())
}

func (m infoModel) View() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("muxc") +
		m.styles.Subtitle.Render("  ·  info  ·  "+m.row.Name))
	b.WriteString("\n\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n" + m.styles.Help.Render("↑↓/PgUp/PgDn scroll · +/- transcript lines · esc back · ^C quit"))
	return b.String()
}
