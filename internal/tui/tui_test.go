package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/session"
	"github.com/monzim/muxc/internal/state"
)

func TestClaudeDisplay(t *testing.T) {
	cases := []struct {
		row  session.Row
		want string
	}{
		{session.Row{ClaudeSessionName: "auth"}, "auth"},
		{session.Row{ClaudeSessionID: "abcd1234efgh"}, "abcd1234…"},
		{session.Row{ClaudeSessionID: "short"}, "short"},
		{session.Row{}, "-"},
	}
	for _, c := range cases {
		got := claudeDisplay(c.row)
		if got != c.want {
			t.Errorf("claudeDisplay(%+v) = %q, want %q", c.row, got, c.want)
		}
	}
}

func TestHomeRel(t *testing.T) {
	cases := []struct{ in, home, want string }{
		{"/home/u/code/x", "/home/u", "~/code/x"},
		{"/home/u", "/home/u", "~"},
		{"/etc/foo", "/home/u", "/etc/foo"},
		{"", "/home/u", ""},
		{"/anything", "", "/anything"},
	}
	for _, c := range cases {
		if got := homeRel(c.in, c.home); got != c.want {
			t.Errorf("homeRel(%q, %q) = %q, want %q", c.in, c.home, got, c.want)
		}
	}
}

func TestPadRight(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"abc", 5, "abc  "},
		{"abc", 3, "abc"},
		{"abc", 1, "abc"}, // never truncates
		{"", 3, "   "},
	}
	for _, c := range cases {
		if got := padRight(c.in, c.width); got != c.want {
			t.Errorf("padRight(%q, %d) = %q, want %q", c.in, c.width, got, c.want)
		}
	}
}

func TestIfelse(t *testing.T) {
	if got := ifelse(true, "a", "b"); got != "a" {
		t.Errorf("ifelse(true, a, b) = %q", got)
	}
	if got := ifelse(false, "a", "b"); got != "b" {
		t.Errorf("ifelse(false, a, b) = %q", got)
	}
}

// TestSessionsModel_GatherCompleteSortsAndClamps verifies the core Update
// behaviour: a gather result sorts the rows by the current key and clamps
// the cursor to the new length.
func TestSessionsModel_GatherCompleteSortsAndClamps(t *testing.T) {
	cfg := config.DefaultConfig()
	st := &state.State{Version: 1, Sessions: map[string]state.SessionEntry{}}
	m := newSessionsModel(cfg, st, DefaultStyles(), DefaultKeyMap())

	// Pre-set cursor past the upcoming list length to exercise the clamp.
	m.cursor = 99
	m.sortIdx = 0 // "name"

	m2, _ := m.Update(gatherCompleteMsg{
		rows: []session.Row{
			{Name: "muxc-z"},
			{Name: "muxc-a"},
			{Name: "muxc-m"},
		},
		when: time.Now(),
	})

	if len(m2.rows) != 3 {
		t.Fatalf("rows: want 3, got %d", len(m2.rows))
	}
	if m2.rows[0].Name != "muxc-a" {
		t.Errorf("sort name: first row = %q, want muxc-a", m2.rows[0].Name)
	}
	if m2.cursor != 2 {
		t.Errorf("cursor clamp: got %d, want 2 (last index)", m2.cursor)
	}
	if m2.loading {
		t.Error("loading should be cleared after gatherCompleteMsg")
	}
}

func TestSessionsModel_SortKeyCycles(t *testing.T) {
	cfg := config.DefaultConfig()
	st := &state.State{Version: 1, Sessions: map[string]state.SessionEntry{}}
	m := newSessionsModel(cfg, st, DefaultStyles(), DefaultKeyMap())

	if got := sortKeys[m.sortIdx]; got != "name" {
		t.Fatalf("initial sort key: got %q, want name", got)
	}

	// Simulate pressing 's' four times — should cycle through all keys and
	// return to "name".
	want := []string{"mem", "idle", "created", "name"}
	for i, w := range want {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
		m = next
		if got := sortKeys[m.sortIdx]; got != w {
			t.Errorf("after %d presses of s: got %q, want %q", i+1, got, w)
		}
	}
}

func TestSanitizeNameTUI(t *testing.T) {
	cases := []struct{ in, want string }{
		{"my-project", "my-project"},
		{"My Project", "my-project"},
		{"  trim  ", "trim"},
		{"....", ""},
		{"snake_case", "snake_case"},
		{"a/b/c", "a-b-c"},
		{"weird!!chars", "weird-chars"},
		{"---hello---", "hello"},
	}
	for _, c := range cases {
		if got := sanitizeNameTUI(c.in); got != c.want {
			t.Errorf("sanitizeNameTUI(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestKillPickerAdvanceSelected(t *testing.T) {
	cfg := config.DefaultConfig()
	st := &state.State{Version: 1, Sessions: map[string]state.SessionEntry{}}
	m := newKillpickerModel(cfg, st, DefaultStyles(), DefaultKeyMap())
	m.reset("muxc-foo")
	m.modeIdx = 0 // "selected"

	next, cmd := m.advance()
	if cmd != nil {
		t.Errorf("selected mode should not return a cmd (no-op transition)")
	}
	if next.stage != killStageConfirm {
		t.Errorf("stage: got %v, want killStageConfirm", next.stage)
	}
	if len(next.victims) != 1 || next.victims[0].Name != "muxc-foo" {
		t.Errorf("victims: got %v, want [{muxc-foo}]", next.victims)
	}
}

func TestKillPickerAdvanceIdleInvalid(t *testing.T) {
	cfg := config.DefaultConfig()
	st := &state.State{Version: 1, Sessions: map[string]state.SessionEntry{}}
	m := newKillpickerModel(cfg, st, DefaultStyles(), DefaultKeyMap())
	m.reset("")
	m.modeIdx = 1 // "--idle"
	m.idleStr = "garbage"

	next, _ := m.advance()
	if next.err == nil {
		t.Error("expected an err for unparseable duration")
	}
	if next.stage != killStageMode {
		t.Error("stage should stay on mode picker after invalid duration")
	}
}

// TestNewApp_AttachTargetInitiallyEmpty guards the contract used by tui.Run.
func TestNewApp_AttachTargetInitiallyEmpty(t *testing.T) {
	cfg := config.DefaultConfig()
	st := &state.State{Version: 1, Sessions: map[string]state.SessionEntry{}}
	a := NewApp(cfg, st)
	if a.AttachTarget() != "" {
		t.Errorf("AttachTarget on fresh App = %q, want empty", a.AttachTarget())
	}
}
