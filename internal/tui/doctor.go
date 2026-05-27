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
// a row toggles its expanded message panel.
type doctorModel struct {
	cfg       *config.Config
	st        *state.State
	configDir string

	checks   []doctor.CheckResult
	cursor   int
	expanded bool
	loading  bool

	width  int
	height int

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
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
	l := newLayout(m.styles, m.width)

	right := ""
	if m.loading {
		right = m.styles.HeaderRefresh.Render("◌ running")
	}
	header := l.header("doctor", right)

	var b strings.Builder
	if len(m.checks) == 0 && m.loading {
		b.WriteString(m.styles.Muted.Render("running 10 environment checks…"))
	} else {
		// Find max name width for alignment.
		nameW := 0
		for _, c := range m.checks {
			if w := len(c.Name); w > nameW {
				nameW = w
			}
		}
		for i, c := range m.checks {
			badge := m.styledBadge(c.Status)
			name := c.Name + strings.Repeat(" ", nameW-len(c.Name))
			line := badge + "  " + name
			if c.Message != "" && c.Status == doctor.StatusOK {
				line += "  " + m.styles.Faint.Render(c.Message)
			}
			if i == m.cursor {
				line = m.styles.TableRowSel.Render("▸ " + line)
			} else {
				line = "  " + line
			}
			b.WriteString(line + "\n")

			// Expanded detail panel under the selected row, for WARN/FAIL.
			if m.expanded && i == m.cursor && c.Message != "" && c.Status != doctor.StatusOK {
				detail := m.styles.Card.
					BorderForeground(colorBorderFocus).
					Render(m.styles.Strong.Render(c.Name) + "\n" + c.Message)
				b.WriteString(detail + "\n")
			}
		}
	}

	cardWidth := m.width - 2
	if cardWidth < 40 {
		cardWidth = 40
	}
	card := m.styles.Card.Width(cardWidth).Render(b.String())

	status := []statusSeg{
		{"↑↓/jk", "select"},
		{"enter", "toggle detail"},
		{"r", "re-run"},
		{"esc", "back"},
		{"q", "quit"},
	}
	return l.compose(header, card, status)
}

// styledBadge renders a colored circle + label per status.
func (m doctorModel) styledBadge(s doctor.CheckStatus) string {
	switch s {
	case doctor.StatusOK:
		return m.styles.StatusOK.Render("● OK  ")
	case doctor.StatusWarn:
		return m.styles.StatusWarn.Render("● WARN")
	case doctor.StatusFail:
		return m.styles.StatusBad.Render("● FAIL")
	default:
		return "● ????"
	}
}
