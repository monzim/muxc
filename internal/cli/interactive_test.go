package cli

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestParseInteractiveSelection(t *testing.T) {
	cases := []struct{ in, want string }{
		{"1\n", "1"},
		{"  2  \n", "2"},
		{"q\n", "q"},
		{"QUIT\n", "quit"},
		{"Ls\n", "ls"},
		{"\n", ""},
		{"   \n", ""},
		{"info  ", "info"},
	}
	for _, c := range cases {
		got := parseInteractiveSelection(c.in)
		if got != c.want {
			t.Errorf("parseInteractiveSelection(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsQuitWord(t *testing.T) {
	for _, w := range []string{"q", "quit", "exit"} {
		if !isQuitWord(w) {
			t.Errorf("isQuitWord(%q): expected true", w)
		}
	}
	for _, w := range []string{"", "qq", "QUIT", "ls", "x"} {
		// Note: parseInteractiveSelection already lowercases, so isQuitWord
		// only needs to handle lowercase canonical forms.
		if isQuitWord(w) {
			t.Errorf("isQuitWord(%q): expected false", w)
		}
	}
}

func TestFindMenuItem(t *testing.T) {
	items := defaultMenuItems()

	// Numeric key.
	got := findMenuItem(items, "1")
	if got == nil || got.name != "ls" {
		t.Errorf("findMenuItem(\"1\"): got %v, want ls", got)
	}

	// By name.
	got = findMenuItem(items, "doctor")
	if got == nil || got.key != "7" {
		t.Errorf("findMenuItem(\"doctor\"): got %v, want key=7", got)
	}

	// Miss.
	if got := findMenuItem(items, "bogus"); got != nil {
		t.Errorf("findMenuItem(\"bogus\"): got %v, want nil", got)
	}
}

func TestFormatMenu_ContainsAllItems(t *testing.T) {
	items := defaultMenuItems()
	out := formatMenu(items)
	for _, it := range items {
		if !strings.Contains(out, it.name) {
			t.Errorf("formatMenu missing command name %q", it.name)
		}
		if !strings.Contains(out, it.desc) {
			t.Errorf("formatMenu missing description for %q", it.name)
		}
		if !strings.Contains(out, it.key+")") {
			t.Errorf("formatMenu missing key marker for %q", it.name)
		}
	}
}

func TestFormatMenu_OneLinePerItem(t *testing.T) {
	items := defaultMenuItems()
	out := formatMenu(items)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != len(items) {
		t.Errorf("formatMenu line count: got %d, want %d", len(lines), len(items))
	}
}

// fakeREPL builds a replEnv whose reader is a fixed input string and whose
// writers are buffers. The cmd field is left nil because most tests exercise
// the pure-logic helpers (parseInteractiveSelection, findMenuItem, etc.) that
// do not touch the cobra command.
func fakeREPL(input string) (*replEnv, *bytes.Buffer, *bytes.Buffer) {
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	return &replEnv{
		reader: bufio.NewReader(strings.NewReader(input)),
		out:    out,
		err:    errBuf,
	}, out, errBuf
}

func TestPromptLine_DefaultOnEmptyInput(t *testing.T) {
	env, _, _ := fakeREPL("\n")
	got := promptLine(env, "x: ", "default-value")
	if got != "default-value" {
		t.Errorf("empty input: got %q, want %q", got, "default-value")
	}
}

func TestPromptLine_Trim(t *testing.T) {
	env, _, _ := fakeREPL("  hello  \n")
	got := promptLine(env, "x: ", "")
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestPromptLine_EOFReturnsEmpty(t *testing.T) {
	env, _, _ := fakeREPL("")
	got := promptLine(env, "x: ", "default")
	if got != "" {
		t.Errorf("EOF: got %q, want empty string", got)
	}
}

func TestPromptYesNo(t *testing.T) {
	cases := []struct {
		in   string
		def  bool
		want bool
	}{
		{"y\n", false, true},
		{"Y\n", false, true},
		{"yes\n", false, true},
		{"n\n", true, false},
		{"\n", true, true},   // empty → default true
		{"\n", false, false}, // empty → default false
		{"maybe\n", true, false},
	}
	for _, c := range cases {
		env, _, _ := fakeREPL(c.in)
		got := promptYesNo(env, "ok? ", c.def)
		if got != c.want {
			t.Errorf("input=%q default=%v: got %v, want %v", c.in, c.def, got, c.want)
		}
	}
}

func TestPauseForEnter_QuitWord(t *testing.T) {
	env, _, _ := fakeREPL("q\n")
	if pauseForEnter(env) {
		t.Error("expected false on 'q'")
	}
}

func TestPauseForEnter_EnterContinues(t *testing.T) {
	env, _, _ := fakeREPL("\n")
	if !pauseForEnter(env) {
		t.Error("expected true on plain Enter")
	}
}

func TestPauseForEnter_EOFTerminatesCleanly(t *testing.T) {
	env, _, _ := fakeREPL("")
	if pauseForEnter(env) {
		t.Error("expected false on EOF")
	}
}

func TestDefaultMenuItems_HasAllSubcommands(t *testing.T) {
	items := defaultMenuItems()
	wantNames := []string{"ls", "new", "attach", "kill", "mem", "info", "doctor", "version"}
	got := map[string]bool{}
	for _, it := range items {
		got[it.name] = true
	}
	for _, n := range wantNames {
		if !got[n] {
			t.Errorf("default menu missing %q", n)
		}
	}
}
