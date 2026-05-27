package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/doctor"
	"github.com/monzim/muxc/internal/state"
)

// doctorModel renders the 10 environment checks. ↑↓ navigates rows; Enter on
// a row toggles its expanded message panel (useful when a FAIL has a long
// remediation string).
type doctorModel struct {
	cfg       *config.Config
	st        *state.State
	configDir string

	checks    []doctor.CheckResult
	cursor    int
	expanded  bool
	loading   bool

	styles Styles
	keys   KeyMap
}

func newDoctorModel(cfg *config.Config, st *state.State, configDir string, styles Styles, keys KeyMap) doctorModel {
	return doctorModel{
		cfg:       cfg,
		st:        st,
		configDir: configDir,
		styles:    styles,
		keys:      keys,
		loading:   true,
	}
}

// doctorResultMsg carries the result of a doctor.Run call into the model.
type doctorResultMsg struct{ checks []doctor.CheckResult }

func runDoctorCmd(cfg *config.Config, st *state.State, configDir string) tea.Cmd {
	return func() tea.Msg {
		return doctorResultMsg{checks: doctor.Run(context.Background(), cfg, st, configDir)}
	}
}

func (m doctorModel) Init() tea.Cmd {
	return runDoctorCmd(m.cfg, m.st, m.configDir)
}

func (m doctorModel) Update(msg tea.Msg) (doctorModel, tea.Cmd) {
	switch msg := msg.(type) {
	case doctorResultMsg:
		m.checks = msg.checks
		m.loading = false
		return m, nil
	case tea.KeyMsg:
		switch {
		case keyHit(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case keyHit(msg, m.keys.Down):
			if m.cursor < len(m.checks)-1 {
				m.cursor++
			}
		case keyHit(msg, m.keys.Refresh):
			m.loading = true
			return m, runDoctorCmd(m.cfg, m.st, m.configDir)
		case keyHit(msg, m.keys.Enter):
			m.expanded = !m.expanded
		}
	}
	return m, nil
}

func (m doctorModel) View() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("muxc") + m.styles.Subtitle.Render("  ·  doctor"))
	if m.loading {
		b.WriteString(m.styles.Muted.Render("  · running checks…"))
	}
	b.WriteString("\n\n")

	for i, c := range m.checks {
		badge := m.styledBadge(c.Status)
		row := badge + " " + c.Name
		if i == m.cursor {
			b.WriteString(m.styles.TableRowSel.Render("▸ " + row))
		} else {
			b.WriteString("  " + row)
		}
		// Inline message for OK rows; expanded panel handled below.
		if c.Status == doctor.StatusOK && c.Message != "" {
			b.WriteString(m.styles.Muted.Render("  " + c.Message))
		}
		b.WriteString("\n")
	}

	// Expanded panel: show the message of the selected row (especially useful
	// for FAIL/WARN where guidance lives in the Message field).
	if m.expanded && m.cursor < len(m.checks) {
		sel := m.checks[m.cursor]
		if sel.Message != "" {
			panel := m.styles.Frame.Render(m.styles.Subtitle.Render(sel.Name) + "\n" + sel.Message)
			b.WriteString("\n" + panel + "\n")
		}
	}

	footer := m.styles.Help.Render("↑↓ navigate · enter expand · r re-run · esc back · ^C quit")
	b.WriteString("\n" + footer)
	return b.String()
}

// styledBadge returns the colored "[OK]" / "[WARN]" / "[FAIL]" badge with
// adaptive Lip Gloss colours.
func (m doctorModel) styledBadge(s doctor.CheckStatus) string {
	switch s {
	case doctor.StatusOK:
		return m.styles.StatusOK.Render("[OK]  ")
	case doctor.StatusWarn:
		return m.styles.StatusWarn.Render("[WARN]")
	case doctor.StatusFail:
		return m.styles.StatusBad.Render("[FAIL]")
	default:
		return "[????]"
	}
}
