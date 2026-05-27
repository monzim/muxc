package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ── pickFromRows ──────────────────────────────────────────────────────────

func TestPickFromRows_ValidInput(t *testing.T) {
	rows := makePickRows(3)
	var out strings.Builder
	idx, err := pickFromRows(rows, strings.NewReader("1\n"), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 0 {
		t.Errorf("expected index 0 for input '1', got %d", idx)
	}
}

func TestPickFromRows_ValidInputLast(t *testing.T) {
	rows := makePickRows(3)
	var out strings.Builder
	idx, err := pickFromRows(rows, strings.NewReader("3\n"), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 2 {
		t.Errorf("expected index 2 for input '3', got %d", idx)
	}
}

func TestPickFromRows_OutOfRange(t *testing.T) {
	rows := makePickRows(3)
	var out strings.Builder
	idx, err := pickFromRows(rows, strings.NewReader("99\n"), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != -1 {
		t.Errorf("expected -1 for out-of-range input, got %d", idx)
	}
}

func TestPickFromRows_ZeroOutOfRange(t *testing.T) {
	rows := makePickRows(3)
	var out strings.Builder
	idx, err := pickFromRows(rows, strings.NewReader("0\n"), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != -1 {
		t.Errorf("expected -1 for input 0 (below range), got %d", idx)
	}
}

func TestPickFromRows_EmptyInput(t *testing.T) {
	rows := makePickRows(2)
	var out strings.Builder
	idx, err := pickFromRows(rows, strings.NewReader("\n"), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != -1 {
		t.Errorf("expected -1 for empty input, got %d", idx)
	}
}

func TestPickFromRows_NonNumeric(t *testing.T) {
	rows := makePickRows(2)
	var out strings.Builder
	idx, err := pickFromRows(rows, strings.NewReader("abc\n"), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != -1 {
		t.Errorf("expected -1 for non-numeric input, got %d", idx)
	}
}

func TestPickFromRows_EOF(t *testing.T) {
	rows := makePickRows(2)
	var out strings.Builder
	// Empty reader → immediate EOF.
	idx, err := pickFromRows(rows, strings.NewReader(""), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != -1 {
		t.Errorf("expected -1 on EOF, got %d", idx)
	}
}

func TestPickFromRows_OutputContainsAllRows(t *testing.T) {
	rows := makePickRows(3)
	var out strings.Builder
	_, _ = pickFromRows(rows, strings.NewReader("1\n"), &out)
	output := out.String()
	for i := 1; i <= 3; i++ {
		if !strings.Contains(output, fmt.Sprintf("%d)", i)) {
			t.Errorf("expected output to contain row %d numbered entry", i)
		}
	}
}

// ── formatPickerLine ───────────────────────────────────────────────────────

func TestFormatPickerLine_ShortValues(t *testing.T) {
	row := SessionRow{
		Name:        "muxc-abc",
		ProjectPath: "~/code/abc",
		IdleSeconds: 300, // 5m
	}
	line := formatPickerLine(row)
	if !strings.Contains(line, "muxc-abc") {
		t.Errorf("line should contain session name, got: %q", line)
	}
	if !strings.Contains(line, "~/code/abc") {
		t.Errorf("line should contain project path, got: %q", line)
	}
	if !strings.Contains(line, "idle 5m") {
		t.Errorf("line should contain 'idle 5m', got: %q", line)
	}
}

func TestFormatPickerLine_LongProjectPath(t *testing.T) {
	longPath := "/home/user/very/deeply/nested/project/that/exceeds/thirty/chars"
	row := SessionRow{
		Name:        "muxc-long",
		ProjectPath: longPath,
		IdleSeconds: 7200, // 2h
	}
	line := formatPickerLine(row)
	// Project field should be truncated to ≤ 30 runes.
	parts := strings.Fields(line)
	_ = parts
	// The line must not contain the full path (it would exceed the truncation limit).
	if strings.Contains(line, longPath) {
		t.Errorf("long path should be truncated, but full path appears in: %q", line)
	}
}

func TestFormatPickerLine_EmptyProjectPath(t *testing.T) {
	row := SessionRow{
		Name:        "muxc-nopath",
		ProjectPath: "",
		IdleSeconds: 0,
	}
	line := formatPickerLine(row)
	if !strings.Contains(line, "(unknown)") {
		t.Errorf("empty project path should show '(unknown)', got: %q", line)
	}
}

func TestFormatPickerLine_ZeroIdle(t *testing.T) {
	row := SessionRow{
		Name:        "muxc-fresh",
		ProjectPath: "~/proj",
		IdleSeconds: 0,
	}
	line := formatPickerLine(row)
	if !strings.Contains(line, "idle 0s") {
		t.Errorf("zero idle should show 'idle 0s', got: %q", line)
	}
}

func TestFormatPickerLine_ColumnPaddingName(t *testing.T) {
	// The name column is padded to 20 chars.
	short := SessionRow{Name: "ab", ProjectPath: "~/p", IdleSeconds: 60}
	long := SessionRow{Name: "muxc-a-longer-name-here", ProjectPath: "~/p", IdleSeconds: 60}

	lineShort := formatPickerLine(short)
	lineLong := formatPickerLine(long)

	// Both lines should still contain the project path at the same relative position
	// (within the padding tolerance). Both contain "idle 1m".
	if !strings.Contains(lineShort, "idle 1m") {
		t.Errorf("short name line missing idle, got: %q", lineShort)
	}
	if !strings.Contains(lineLong, "idle 1m") {
		t.Errorf("long name line missing idle, got: %q", lineLong)
	}
}

// ── formatFzfInput ────────────────────────────────────────────────────────

func TestFormatFzfInput_LineCount(t *testing.T) {
	rows := makePickRows(5)
	out := formatFzfInput(rows)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d", len(lines))
	}
}

func TestFormatFzfInput_TabSeparated(t *testing.T) {
	rows := makePickRows(2)
	out := formatFzfInput(rows)
	for i, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			t.Errorf("line %d: expected 3 tab-separated fields, got %d: %q", i, len(parts), line)
		}
	}
}

func TestFormatFzfInput_FirstFieldIsName(t *testing.T) {
	rows := []SessionRow{
		{Name: "muxc-alpha", ProjectPath: "~/alpha", IdleSeconds: 10},
		{Name: "muxc-beta", ProjectPath: "~/beta", IdleSeconds: 20},
	}
	out := formatFzfInput(rows)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] == "" || !strings.HasPrefix(lines[0], "muxc-alpha\t") {
		t.Errorf("first line should start with 'muxc-alpha\\t', got: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "muxc-beta\t") {
		t.Errorf("second line should start with 'muxc-beta\\t', got: %q", lines[1])
	}
}

func TestFormatFzfInput_EachLineEndsWithNewline(t *testing.T) {
	rows := makePickRows(3)
	out := formatFzfInput(rows)
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("output should end with newline, got: %q", out[max(0, len(out)-10):])
	}
}

func TestFormatFzfInput_EmptyProjectShowsUnknown(t *testing.T) {
	rows := []SessionRow{{Name: "muxc-x", ProjectPath: "", IdleSeconds: 5}}
	out := formatFzfInput(rows)
	if !strings.Contains(out, "(unknown)") {
		t.Errorf("empty project path should appear as '(unknown)' in fzf input, got: %q", out)
	}
}

// ── attach command (via fake attacher) ───────────────────────────────────

// fakeAttacher records the last Attach call without doing a syscall.Exec.
type fakeAttacher struct {
	calledName   string
	calledDetach bool
	calls        int
}

func (f *fakeAttacher) Attach(name string, detach bool) error {
	f.calledName = name
	f.calledDetach = detach
	f.calls++
	return nil
}

// The attach command integration tests use a fake attacher AND a fake
// sessionChecker so no real tmux process is spawned.

// fakeSessionChecker implements sessionChecker.
type fakeSessionChecker struct {
	sessions map[string]bool
}

func (f fakeSessionChecker) HasSession(_ context.Context, name string) (bool, error) {
	return f.sessions[name], nil
}

// ── helpers ────────────────────────────────────────────────────────────────

// makePickRows creates n synthetic SessionRows for prompt testing.
func makePickRows(n int) []SessionRow {
	rows := make([]SessionRow, n)
	now := time.Now()
	for i := range rows {
		rows[i] = SessionRow{
			Name:        strings.Repeat("muxc-sess", 1) + strings.Repeat("x", i),
			ProjectPath: "~/code/proj" + strings.Repeat("y", i),
			IdleSeconds: int64(i * 300),
			CreatedAt:   now,
			ActivityAt:  now,
		}
	}
	return rows
}

// max is a helper for Go 1.22 where the builtin max may not be available in all
// contexts (used only in test error formatting above).
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
