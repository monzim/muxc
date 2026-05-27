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

// newFormModel is the screen for `muxc new`. Three fields:
//
//	project path  (text input, default = cwd)
//	name override (text input, optional)
//	skip launch?  (bool, toggle with space)
//
// Submit (Enter on the last field, or Ctrl+S anywhere) builds the launch
// command and creates the tmux session via the internal/tmux helpers
// directly. On success the model returns a sessionCreatedMsg so the App can
// switch back to the sessions screen and refresh.
type newFormModel struct {
	cfg *config.Config
	st  *state.State

	pathInput textinput.Model
	nameInput textinput.Model
	skipBox   bool
	focusIdx  int // 0=path, 1=name, 2=skip toggle, 3=submit button

	err     error
	success string

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
	pathIn.Placeholder = "/path/to/project (default: current directory)"
	if cwd, err := os.Getwd(); err == nil {
		pathIn.SetValue(cwd)
	}
	pathIn.CharLimit = 1024
	pathIn.Width = 60
	pathIn.Focus()

	nameIn := textinput.New()
	nameIn.Placeholder = "optional override (default: dir basename)"
	nameIn.CharLimit = 64
	nameIn.Width = 60

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

// reset clears errors / success state when re-entering the screen.
func (m *newFormModel) reset() {
	m.err = nil
	m.success = ""
	m.focusIdx = newFormFocusPath
	m.pathInput.Focus()
	m.nameInput.Blur()
}

func (m newFormModel) Init() tea.Cmd { return textinput.Blink }

// sessionCreatedMsg signals the App to switch back to the sessions screen
// and trigger an immediate refresh.
type sessionCreatedMsg struct {
	name string
}

func (m newFormModel) Update(msg tea.Msg) (newFormModel, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionCreatedMsg:
		m.success = fmt.Sprintf("started %s", msg.name)
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
			// Enter on a text input — advance focus.
			m.focusIdx = (m.focusIdx + 1) % newFormFocusCount
			m.refocus()
			return m, nil
		}
	}

	// Forward to the focused text input.
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
	var b strings.Builder

	b.WriteString(m.styles.Title.Render("muxc") + m.styles.Subtitle.Render("  ·  new session"))
	b.WriteString("\n\n")

	field := func(label string, idx int, body string) string {
		focused := m.focusIdx == idx
		prefix := "  "
		if focused {
			prefix = m.styles.Accent.Render("▸ ")
		}
		return prefix + m.styles.Subtitle.Render(label) + "\n  " + body + "\n"
	}

	b.WriteString(field("project path", newFormFocusPath, m.pathInput.View()))
	b.WriteString(field("name override", newFormFocusName, m.nameInput.View()))

	skipMark := "[ ]"
	if m.skipBox {
		skipMark = m.styles.Accent.Render("[x]")
	}
	b.WriteString(field("skip launching claude", newFormFocusSkip, skipMark+" toggle with space"))

	btnLabel := "[ submit ]"
	if m.focusIdx == newFormFocusSubmit {
		btnLabel = m.styles.Accent.Render("[ submit ]")
	}
	b.WriteString("  " + btnLabel + "\n")

	if m.err != nil {
		b.WriteString("\n" + m.styles.StatusBad.Render("error: ") + m.err.Error() + "\n")
	}
	if m.success != "" {
		b.WriteString("\n" + m.styles.StatusOK.Render(m.success) + "\n")
		b.WriteString(m.styles.Muted.Render("press esc to return to sessions"))
		b.WriteString("\n")
	}

	footer := m.styles.Help.Render("tab/↑↓ field · enter submit · space toggle · esc back · ^C quit")
	b.WriteString("\n" + footer)
	return b.String()
}

// submit returns a tea.Cmd that creates the tmux session and emits a result
// message. Runs the side effect off the UI thread.
func (m newFormModel) submit() tea.Cmd {
	pathRaw := strings.TrimSpace(m.pathInput.Value())
	nameOverride := strings.TrimSpace(m.nameInput.Value())
	skip := m.skipBox
	cfg := m.cfg
	st := m.st
	return func() tea.Msg {
		// Resolve path: expand ~ and make absolute.
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
			return errMsg{err: fmt.Errorf("project path is not a directory: %s", absPath)}
		}

		// Derive base name.
		base := nameOverride
		if base == "" {
			base = filepath.Base(absPath)
		}
		base = sanitizeNameTUI(base)
		if base == "" {
			return errMsg{err: fmt.Errorf("could not derive a usable session name from %q", nameOverride)}
		}
		tmuxName := cfg.Defaults.Prefix + base

		// Dedup against live sessions (lightweight — TUI doesn't try as hard
		// as the cli new path, which adds -2/-3 suffixes; here we just
		// surface the conflict so the user can pick a different name).
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
			// Build launch command from defaults.
			args := append([]string{cfg.Defaults.ClaudeBin}, cfg.Defaults.LaunchArgs...)
			if cfg.Defaults.NameSessions {
				args = append(args, "-n", base)
			}
			cmdline := strings.Join(args, " ")
			if err := tmux.SendKeys(ctx, tmuxName, cmdline); err != nil {
				return errMsg{err: fmt.Errorf("send-keys: %w", err)}
			}
		}

		// Persist state.
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

// sanitizeNameTUI mirrors the cli's sanitizeName logic without re-importing
// cli (which would re-introduce the cycle). Lowercase, replace
// non-[A-Za-z0-9_-] with '-', collapse repeats, trim hyphens.
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
	out := strings.Trim(b.String(), "-")
	return out
}

// Ensure unused-import safety: lipgloss is used via Styles only above; this
// var keeps the import explicit even if some style is removed later.
var _ = lipgloss.NewStyle
