package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/state"
	"github.com/monzim/muxc/internal/tmux"
)

// newFormModel is the screen for `muxc new`: project path, name override,
// skip-launch toggle, submit button. Tab/↑↓ between fields; Enter submits.
type newFormModel struct {
	cfg *config.Config
	st  *state.State

	pathInput textinput.Model
	nameInput textinput.Model
	skipBox   bool
	focusIdx  int

	err     error
	success string

	width  int
	height int

	styles Styles
	keys   KeyMap
}

const (
	newFormFocusPath = iota
	newFormFocusName
	newFormFocusSkip
	newFormFocusSubmit
	newFormFocusCount
)

func newNewFormModel(cfg *config.Config, st *state.State, styles Styles, keys KeyMap) newFormModel {
	pathIn := textinput.New()
	pathIn.Placeholder = "/path/to/project"
	if cwd, err := os.Getwd(); err == nil {
		pathIn.SetValue(cwd)
	}
	pathIn.CharLimit = 1024
	pathIn.Width = 60
	pathIn.Prompt = "› "
	pathIn.Focus()

	nameIn := textinput.New()
	nameIn.Placeholder = "(optional) override session name"
	nameIn.CharLimit = 64
	nameIn.Width = 60
	nameIn.Prompt = "› "

	return newFormModel{
		cfg:       cfg,
		st:        st,
		pathInput: pathIn,
		nameInput: nameIn,
		focusIdx:  newFormFocusPath,
		styles:    styles,
		keys:      keys,
	}
}

func (m *newFormModel) reset() {
	m.err = nil
	m.success = ""
	m.focusIdx = newFormFocusPath
	m.pathInput.Focus()
	m.nameInput.Blur()
}

func (m newFormModel) Init() tea.Cmd { return textinput.Blink }

type sessionCreatedMsg struct{ name string }

func (m newFormModel) Update(msg tea.Msg) (newFormModel, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionCreatedMsg:
		m.success = fmt.Sprintf("✓ session %s created", msg.name)
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			m.focusIdx = (m.focusIdx + 1) % newFormFocusCount
			m.refocus()
			return m, nil
		case "shift+tab", "up":
			m.focusIdx = (m.focusIdx - 1 + newFormFocusCount) % newFormFocusCount
			m.refocus()
			return m, nil
		case " ":
			if m.focusIdx == newFormFocusSkip {
				m.skipBox = !m.skipBox
				return m, nil
			}
		case "enter":
			if m.focusIdx == newFormFocusSubmit ||
				(m.focusIdx != newFormFocusPath && m.focusIdx != newFormFocusName) {
				return m, m.submit()
			}
			m.focusIdx = (m.focusIdx + 1) % newFormFocusCount
			m.refocus()
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.focusIdx {
	case newFormFocusPath:
		m.pathInput, cmd = m.pathInput.Update(msg)
	case newFormFocusName:
		m.nameInput, cmd = m.nameInput.Update(msg)
	}
	return m, cmd
}

func (m *newFormModel) refocus() {
	m.pathInput.Blur()
	m.nameInput.Blur()
	switch m.focusIdx {
	case newFormFocusPath:
		m.pathInput.Focus()
	case newFormFocusName:
		m.nameInput.Focus()
	}
}

func (m newFormModel) View() string {
	l := newLayout(m.styles, m.width)
	header := l.header("new session", "")

	var b strings.Builder

	// Path input.
	b.WriteString(m.styles.FormLabel.Render("PROJECT PATH"))
	b.WriteString("\n")
	pv := m.pathInput.View()
	if m.focusIdx == newFormFocusPath {
		pv = m.styles.Accent.Render(pv)
	}
	b.WriteString(pv + "\n\n")

	// Name input.
	b.WriteString(m.styles.FormLabel.Render("NAME OVERRIDE") + "  " + m.styles.Faint.Render("(blank = use directory name)"))
	b.WriteString("\n")
	nv := m.nameInput.View()
	if m.focusIdx == newFormFocusName {
		nv = m.styles.Accent.Render(nv)
	}
	b.WriteString(nv + "\n\n")

	// Skip checkbox.
	mark := "[ ]"
	if m.skipBox {
		mark = m.styles.Accent.Render("[x]")
	}
	skipPrefix := "  "
	if m.focusIdx == newFormFocusSkip {
		skipPrefix = m.styles.Accent.Render("▸ ")
	}
	b.WriteString(skipPrefix + mark + "  skip launching claude  " + m.styles.Faint.Render("(--no-launch · space to toggle)"))
	b.WriteString("\n\n")

	// Submit button.
	btn := "  submit  "
	if m.focusIdx == newFormFocusSubmit {
		b.WriteString("  " + m.styles.FormBtnFocus.Render(btn))
	} else {
		b.WriteString("  " + m.styles.FormBtn.Render(btn))
	}
	b.WriteString("\n")

	// Result feedback.
	if m.err != nil {
		b.WriteString("\n" + m.styles.StatusBad.Render("⚠ "+m.err.Error()))
	}
	if m.success != "" {
		b.WriteString("\n" + m.styles.StatusOK.Render(m.success))
		b.WriteString("\n" + m.styles.Faint.Render("returning to sessions…"))
	}

	cardWidth := m.width - 2
	if cardWidth < 40 {
		cardWidth = 40
	}
	card := m.styles.CardFocused.Width(cardWidth).Render(b.String())

	status := []statusSeg{
		{"tab/↑↓", "field"},
		{"space", "toggle"},
		{"enter", "submit"},
		{"esc", "cancel"},
		{"q", "quit"},
	}
	return l.compose(header, card, status)
}

func (m newFormModel) submit() tea.Cmd {
	pathRaw := strings.TrimSpace(m.pathInput.Value())
	nameOverride := strings.TrimSpace(m.nameInput.Value())
	skip := m.skipBox
	cfg := m.cfg
	st := m.st
	return func() tea.Msg {
		if strings.HasPrefix(pathRaw, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				pathRaw = filepath.Join(home, strings.TrimPrefix(pathRaw, "~"))
			}
		}
		absPath, err := filepath.Abs(pathRaw)
		if err != nil {
			return errMsg{err: fmt.Errorf("resolve path: %w", err)}
		}
		info, err := os.Stat(absPath)
		if err != nil {
			return errMsg{err: fmt.Errorf("project path: %w", err)}
		}
		if !info.IsDir() {
			return errMsg{err: fmt.Errorf("not a directory: %s", absPath)}
		}

		base := nameOverride
		if base == "" {
			base = filepath.Base(absPath)
		}
		base = sanitizeNameTUI(base)
		if base == "" {
			return errMsg{err: fmt.Errorf("could not derive a usable session name")}
		}
		tmuxName := cfg.Defaults.Prefix + base

		ctx := context.Background()
		sessions, _ := tmux.ListSessions(ctx)
		for _, s := range sessions {
			if s.Name == tmuxName {
				return errMsg{err: fmt.Errorf("session %q already exists — pick a different name", tmuxName)}
			}
		}

		if err := tmux.NewSession(ctx, tmuxName, absPath); err != nil {
			return errMsg{err: fmt.Errorf("tmux new-session: %w", err)}
		}

		if !skip {
			args := append([]string{cfg.Defaults.ClaudeBin}, cfg.Defaults.LaunchArgs...)
			if cfg.Defaults.NameSessions {
				args = append(args, "-n", base)
			}
			cmdline := strings.Join(args, " ")
			if err := tmux.SendKeys(ctx, tmuxName, cmdline); err != nil {
				return errMsg{err: fmt.Errorf("send-keys: %w", err)}
			}
		}

		st.Upsert(tmuxName, state.SessionEntry{
			ProjectPath:       absPath,
			ClaudeSessionName: base,
			LaunchArgs:        append([]string{}, cfg.Defaults.LaunchArgs...),
			CreatedAt:         time.Now().UTC(),
		})
		_ = st.Write(cfg.Paths.StateFile)

		return sessionCreatedMsg{name: tmuxName}
	}
}

// sanitizeNameTUI mirrors the cli's sanitizeName logic.
func sanitizeNameTUI(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevDash := true
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
			prevDash = r == '-'
		} else {
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// _ keeps lipgloss imported for future styling tweaks.
var _ = lipgloss.NewStyle
