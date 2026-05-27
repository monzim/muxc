package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/session"
	"github.com/monzim/muxc/internal/state"
	"github.com/monzim/muxc/internal/tmux"
)

// killpickerModel handles the kill flow inside the TUI.
//
// Two stages:
//
//  1. Mode picker — by selected session, --idle <dur>, --all, --stale.
//  2. Confirmation — list of victims with idle/mem, y/n.
//
// ExternalNone is enforced for --all and --idle so the TUI mirrors the CLI's
// post-v1.0 invariant: external sessions are never auto-killed.
type killpickerModel struct {
	cfg *config.Config
	st  *state.State

	stage    killStage
	modeIdx  int
	idleStr  string
	selected string // name of the session selected on the sessions screen (passed in)

	// Stage-2 state: the list of victims we'll actually kill.
	victims []session.Row
	err     error
	success string

	styles Styles
	keys   KeyMap
}

type killStage int

const (
	killStageMode killStage = iota
	killStageConfirm
	killStageDone
)

type killModeChoice struct {
	label string
	help  string
}

var killModes = []killModeChoice{
	{"selected", "kill the session highlighted on the previous screen"},
	{"--idle", "kill every muxc session idle longer than a duration (e.g. 2h)"},
	{"--all", "kill every muxc session"},
	{"--stale", "remove stale state.json entries with no matching tmux session"},
}

func newKillpickerModel(cfg *config.Config, st *state.State, styles Styles, keys KeyMap) killpickerModel {
	return killpickerModel{
		cfg:    cfg,
		st:     st,
		styles: styles,
		keys:   keys,
	}
}

// reset prepares the screen for a fresh entry. `selected` is the name of the
// session currently highlighted on the sessions screen (may be empty).
func (m *killpickerModel) reset(selected string) {
	m.stage = killStageMode
	m.modeIdx = 0
	m.idleStr = ""
	m.selected = selected
	m.victims = nil
	m.err = nil
	m.success = ""
}

// killCompleteMsg signals the App to switch back to sessions and refresh.
type killCompleteMsg struct {
	count int
}

func (m killpickerModel) Init() tea.Cmd { return nil }

func (m killpickerModel) Update(msg tea.Msg) (killpickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case killCompleteMsg:
		m.success = fmt.Sprintf("killed %d", msg.count)
		m.stage = killStageDone
		return m, nil
	case errMsg:
		m.err = msg.err
		return m, nil
	case tea.KeyMsg:
		switch m.stage {
		case killStageMode:
			return m.updateModeStage(msg)
		case killStageConfirm:
			return m.updateConfirmStage(msg)
		case killStageDone:
			// any key returns
			return m, nil
		}
	}
	return m, nil
}

func (m killpickerModel) updateModeStage(msg tea.KeyMsg) (killpickerModel, tea.Cmd) {
	switch {
	case keyHit(msg, m.keys.Up):
		if m.modeIdx > 0 {
			m.modeIdx--
		}
	case keyHit(msg, m.keys.Down):
		if m.modeIdx < len(killModes)-1 {
			m.modeIdx++
		}
	case keyHit(msg, m.keys.Enter):
		return m.advance()
	}
	// For --idle, accept duration typing inline.
	if killModes[m.modeIdx].label == "--idle" {
		switch msg.String() {
		case "backspace":
			if len(m.idleStr) > 0 {
				m.idleStr = m.idleStr[:len(m.idleStr)-1]
			}
		default:
			r := msg.String()
			if len(r) == 1 && isDurationChar(r[0]) {
				m.idleStr += r
			}
		}
	}
	return m, nil
}

func (m killpickerModel) updateConfirmStage(msg tea.KeyMsg) (killpickerModel, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		return m, m.executeKill()
	case "n", "N", "esc":
		// abort, back to mode
		m.stage = killStageMode
		m.victims = nil
		return m, nil
	}
	return m, nil
}

// advance moves from mode → confirm by building the victim list. For --stale
// it executes immediately (no confirmation needed; only state changes).
//
// Returns the updated model so the caller (Update) can persist any field
// mutations — Bubble Tea models are value types and naive method calls drop
// those changes silently.
func (m killpickerModel) advance() (killpickerModel, tea.Cmd) {
	mode := killModes[m.modeIdx].label
	switch mode {
	case "selected":
		if m.selected == "" {
			m.err = fmt.Errorf("no session was selected on the previous screen")
			return m, nil
		}
		m.victims = []session.Row{{Name: m.selected}}
		m.stage = killStageConfirm
		return m, nil
	case "--all":
		return m, m.gatherVictimsCmd(false)
	case "--idle":
		dur, err := time.ParseDuration(m.idleStr)
		if err != nil || dur <= 0 {
			m.err = fmt.Errorf("invalid duration %q — try 2h, 30m, 1h30m", m.idleStr)
			return m, nil
		}
		return m, m.gatherIdleVictimsCmd(dur)
	case "--stale":
		return m, m.executeStaleKill()
	}
	return m, nil
}

// gatherVictimsCmd lists every muxc-prefixed session (ExternalNone — external
// sessions are never killed) and stages them as victims.
func (m killpickerModel) gatherVictimsCmd(_ bool) tea.Cmd {
	cfg := m.cfg
	st := m.st
	return func() tea.Msg {
		ctx := context.Background()
		rows, err := session.Gather(ctx, cfg, st, session.ExternalNone)
		if err != nil {
			return errMsg{err: fmt.Errorf("list sessions: %w", err)}
		}
		return killVictimsReadyMsg{rows: rows}
	}
}

func (m killpickerModel) gatherIdleVictimsCmd(min time.Duration) tea.Cmd {
	cfg := m.cfg
	st := m.st
	return func() tea.Msg {
		ctx := context.Background()
		rows, err := session.Gather(ctx, cfg, st, session.ExternalNone)
		if err != nil {
			return errMsg{err: fmt.Errorf("list sessions: %w", err)}
		}
		var filtered []session.Row
		for _, r := range rows {
			if time.Duration(r.IdleSeconds)*time.Second >= min {
				filtered = append(filtered, r)
			}
		}
		return killVictimsReadyMsg{rows: filtered}
	}
}

// killVictimsReadyMsg carries the staged victim list from a gather Cmd.
type killVictimsReadyMsg struct{ rows []session.Row }

// SetVictims processes a killVictimsReadyMsg from the App-level dispatch.
// The App calls this when it receives the message and is showing the kill
// screen.
func (m killpickerModel) SetVictims(rows []session.Row) killpickerModel {
	m.victims = rows
	if len(rows) == 0 {
		m.success = "nothing matched — no sessions killed"
		m.stage = killStageDone
		return m
	}
	m.stage = killStageConfirm
	return m
}

// executeKill kills every staged victim and persists state.
func (m killpickerModel) executeKill() tea.Cmd {
	victims := append([]session.Row{}, m.victims...)
	cfg := m.cfg
	st := m.st
	return func() tea.Msg {
		ctx := context.Background()
		killed := 0
		for _, v := range victims {
			if err := tmux.KillSession(ctx, v.Name); err != nil {
				// Tolerate — log to err message but continue.
				continue
			}
			st.Remove(v.Name)
			killed++
		}
		_ = st.Write(cfg.Paths.StateFile)
		return killCompleteMsg{count: killed}
	}
}

// executeStaleKill prunes state entries with no matching tmux session.
func (m killpickerModel) executeStaleKill() tea.Cmd {
	cfg := m.cfg
	st := m.st
	return func() tea.Msg {
		ctx := context.Background()
		sessions, _ := tmux.ListSessions(ctx)
		live := make([]string, 0, len(sessions))
		for _, s := range sessions {
			live = append(live, s.Name)
		}
		removed := st.Prune(live)
		_ = st.Write(cfg.Paths.StateFile)
		return killCompleteMsg{count: len(removed)}
	}
}

func (m killpickerModel) View() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("muxc") + m.styles.Subtitle.Render("  ·  kill"))
	b.WriteString("\n\n")

	switch m.stage {
	case killStageMode:
		b.WriteString(m.styles.Subtitle.Render("Pick a kill mode:") + "\n\n")
		for i, mode := range killModes {
			marker := "  "
			if i == m.modeIdx {
				marker = m.styles.Accent.Render("▸ ")
			}
			label := mode.label
			if mode.label == "--idle" && i == m.modeIdx {
				label = fmt.Sprintf("--idle %s", m.idleStr+"_")
			}
			b.WriteString(marker + label + "  " + m.styles.Muted.Render(mode.help) + "\n")
		}
		b.WriteString("\n" + m.styles.Help.Render("↑↓ select · type duration for --idle · enter confirm · esc back · ^C quit"))

	case killStageConfirm:
		b.WriteString(m.styles.StatusWarn.Render(fmt.Sprintf("Confirm: kill %d session(s)?", len(m.victims))) + "\n\n")
		for _, v := range m.victims {
			b.WriteString("  " + v.Name + "\n")
		}
		b.WriteString("\n" + m.styles.Help.Render("y to confirm · n/esc to abort"))

	case killStageDone:
		if m.err != nil {
			b.WriteString(m.styles.StatusBad.Render("error: ") + m.err.Error() + "\n")
		}
		if m.success != "" {
			b.WriteString(m.styles.StatusOK.Render(m.success) + "\n")
		}
		b.WriteString("\n" + m.styles.Help.Render("press esc to return to sessions"))
	}

	if m.err != nil && m.stage != killStageDone {
		b.WriteString("\n" + m.styles.StatusBad.Render(m.err.Error()))
	}
	return b.String()
}

// isDurationChar accepts characters that may appear in a Go duration string
// (digits + 'h', 'm', 's', 'd' — the last for `time.ParseDuration` doesn't
// support but we include it so typos are caught at parse-time rather than
// silently dropped from the input box).
func isDurationChar(b byte) bool {
	return (b >= '0' && b <= '9') || b == 'h' || b == 'm' || b == 's' || b == 'd'
}
