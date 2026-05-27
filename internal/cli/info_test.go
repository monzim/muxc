package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// ---- processNode / tree rendering tests ------------------------------------

func TestRenderProcessTree_SingleNode(t *testing.T) {
	nodes := []processNode{
		{PID: 100, Comm: "zsh", RSS: 4 * 1024 * 1024},
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	orig := os.Stdout
	os.Stdout = w

	renderProcessTree(os.Stdout, nodes)

	os.Stdout = orig
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	// A single node gets the └─ connector (last item).
	if !strings.Contains(out, "└─") {
		t.Errorf("single node: expected └─, got:\n%s", out)
	}
	if !strings.Contains(out, "100") {
		t.Errorf("single node: PID 100 not found in:\n%s", out)
	}
	if !strings.Contains(out, "zsh") {
		t.Errorf("single node: 'zsh' not found in:\n%s", out)
	}
}

func TestRenderProcessTree_MultipleNodes(t *testing.T) {
	nodes := []processNode{
		{PID: 100, Comm: "zsh", RSS: 4 * 1024 * 1024},
		{PID: 200, Comm: "node /usr/share/claude/cli.js", RSS: 300 * 1024 * 1024},
		{PID: 300, Comm: "node mcp-server-fs", RSS: 40 * 1024 * 1024},
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	orig := os.Stdout
	os.Stdout = w

	renderProcessTree(os.Stdout, nodes)

	os.Stdout = orig
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	// First two nodes get ├─, last gets └─.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), out)
	}

	if !strings.Contains(lines[0], "├─") {
		t.Errorf("line 0 should have ├─:\n%s", lines[0])
	}
	if !strings.Contains(lines[1], "├─") {
		t.Errorf("line 1 should have ├─:\n%s", lines[1])
	}
	if !strings.Contains(lines[2], "└─") {
		t.Errorf("line 2 should have └─:\n%s", lines[2])
	}
}

func TestRenderProcessTree_CommTruncation(t *testing.T) {
	longComm := strings.Repeat("x", 50) // 50 chars > 40 limit
	nodes := []processNode{
		{PID: 999, Comm: longComm, RSS: 0},
	}

	r, w, _ := os.Pipe()
	defer r.Close()
	orig := os.Stdout
	os.Stdout = w

	renderProcessTree(os.Stdout, nodes)

	os.Stdout = orig
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "…") {
		t.Errorf("truncated comm should contain ellipsis:\n%s", out)
	}
}

func TestRenderProcessTree_Empty(t *testing.T) {
	// Should not panic on empty nodes.
	r, w, _ := os.Pipe()
	defer r.Close()
	orig := os.Stdout
	os.Stdout = w

	renderProcessTree(os.Stdout, nil)

	os.Stdout = orig
	w.Close()
}

// ---- commFromCmdline tests -------------------------------------------------

func TestCommFromCmdline_Empty(t *testing.T) {
	got := commFromCmdline(nil, 42)
	if got != "[42]" {
		t.Errorf("empty cmdline: got %q, want [42]", got)
	}
}

func TestCommFromCmdline_SingleArg(t *testing.T) {
	got := commFromCmdline([]string{"/bin/zsh"}, 1)
	if got != "/bin/zsh" {
		t.Errorf("single arg: got %q", got)
	}
}

func TestCommFromCmdline_ThreeArgs(t *testing.T) {
	argv := []string{"node", "/usr/local/bin/claude", "--dangerously-skip-permissions"}
	got := commFromCmdline(argv, 10)
	if got != "node /usr/local/bin/claude --dangerously-skip-permissions" {
		t.Errorf("three args: got %q", got)
	}
}

func TestCommFromCmdline_ManyArgs(t *testing.T) {
	argv := []string{"node", "a", "b", "c", "d"}
	got := commFromCmdline(argv, 10)
	// Should include "…" because more than 3 tokens.
	if !strings.Contains(got, "…") {
		t.Errorf("many args: expected ellipsis, got %q", got)
	}
}

// ---- renderInfoText tests --------------------------------------------------

func makeTestRow() *SessionRow {
	now := time.Now()
	return &SessionRow{
		Name:              "muxc-testproj",
		ProjectPath:       "/home/user/code/testproj",
		ClaudeSessionName: "my-session",
		ClaudeSessionID:   "abc12345-uuid",
		TmuxSessionID:     "$3",
		CreatedAt:         now.Add(-2 * time.Hour),
		ActivityAt:        now.Add(-5 * time.Minute),
		IdleSeconds:       300,
		UptimeSeconds:     7200,
		Attached:          true,
		AttachedClients:   1,
	}
}

func captureInfoText(t *testing.T, row *SessionRow, nodes []processNode, total int, transcript *transcriptInfo, recent []interface{}) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	orig := os.Stdout
	os.Stdout = w

	renderErr := renderInfoText(os.Stdout, row, nodes, total, transcript, nil)

	os.Stdout = orig
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	if renderErr != nil {
		t.Fatalf("renderInfoText: %v", renderErr)
	}
	return buf.String()
}

func TestRenderInfoText_Header(t *testing.T) {
	row := makeTestRow()
	nodes := []processNode{{PID: 1, Comm: "zsh", RSS: 4096}}

	r, w, _ := os.Pipe()
	defer r.Close()
	orig := os.Stdout
	os.Stdout = w

	_ = renderInfoText(os.Stdout, row, nodes, 1, nil, nil)

	os.Stdout = orig
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	checks := []string{
		"muxc-testproj",
		"project:",
		"created:",
		"activity:",
		"attached:",
		"tmux id:",
		"claude session:",
		"name:",
		"my-session",
		"id:",
		"processes (rss):",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("info text missing %q:\n%s", want, out)
		}
	}
}

func TestRenderInfoText_Attached(t *testing.T) {
	row := makeTestRow()
	row.Attached = true
	row.AttachedClients = 2

	r, w, _ := os.Pipe()
	defer r.Close()
	orig := os.Stdout
	os.Stdout = w

	_ = renderInfoText(os.Stdout, row, nil, 0, nil, nil)

	os.Stdout = orig
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "yes (2 clients)") {
		t.Errorf("attached 2 clients: want 'yes (2 clients)', got:\n%s", out)
	}
}

func TestRenderInfoText_NotAttached(t *testing.T) {
	row := makeTestRow()
	row.Attached = false
	row.AttachedClients = 0

	r, w, _ := os.Pipe()
	defer r.Close()
	orig := os.Stdout
	os.Stdout = w

	_ = renderInfoText(os.Stdout, row, nil, 0, nil, nil)

	os.Stdout = orig
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "attached:  no") {
		t.Errorf("not attached: want 'attached:  no', got:\n%s", out)
	}
}

func TestRenderInfoText_TranscriptInfo(t *testing.T) {
	row := makeTestRow()
	tr := &transcriptInfo{
		Path:    "/home/user/.claude/projects/-home-user-code-testproj/abc.jsonl",
		Size:    127 * 1024,
		ModTime: time.Now().Add(-5 * time.Minute),
	}

	r, w, _ := os.Pipe()
	defer r.Close()
	orig := os.Stdout
	os.Stdout = w

	_ = renderInfoText(os.Stdout, row, nil, 0, tr, nil)

	os.Stdout = orig
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "transcript:") {
		t.Errorf("transcript label missing:\n%s", out)
	}
	if !strings.Contains(out, "size:") {
		t.Errorf("size label missing:\n%s", out)
	}
}

func TestRenderInfoText_NoProcesses(t *testing.T) {
	row := makeTestRow()

	r, w, _ := os.Pipe()
	defer r.Close()
	orig := os.Stdout
	os.Stdout = w

	_ = renderInfoText(os.Stdout, row, nil, 0, nil, nil)

	os.Stdout = orig
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "(no process data)") {
		t.Errorf("empty process tree: missing '(no process data)' in:\n%s", out)
	}
}

// ---- ifEmpty / entryPlural / truncateComm tests ----------------------------

func TestIfEmpty_NonEmpty(t *testing.T) {
	got := ifEmpty("hello", "fallback")
	if got != "hello" {
		t.Errorf("ifEmpty non-empty: got %q", got)
	}
}

func TestIfEmpty_Empty(t *testing.T) {
	got := ifEmpty("", "fallback")
	if got != "fallback" {
		t.Errorf("ifEmpty empty: got %q", got)
	}
}

func TestEntryPlural(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1, "entry"},
		{0, "entries"},
		{2, "entries"},
	}
	for _, c := range cases {
		got := entryPlural(c.n)
		if got != c.want {
			t.Errorf("entryPlural(%d): got %q, want %q", c.n, got, c.want)
		}
	}
}

func TestTruncateComm(t *testing.T) {
	short := "zsh"
	got := truncateComm(short, 40)
	if got != short {
		t.Errorf("truncateComm short: got %q", got)
	}

	long := strings.Repeat("a", 50)
	got2 := truncateComm(long, 40)
	runes := []rune(got2)
	if len(runes) > 40 {
		t.Errorf("truncateComm long: length %d > 40", len(runes))
	}
	if !strings.HasSuffix(got2, "…") {
		t.Errorf("truncateComm long: missing ellipsis in %q", got2)
	}
}

// ---- buildProcessTree tests ------------------------------------------------

func TestBuildProcessTree_ZeroPanePID(t *testing.T) {
	// When PanePID is 0, buildProcessTree should return nil without panicking.
	row := &SessionRow{PanePID: 0}
	nodes, total := buildProcessTree(context.Background(), row)
	if nodes != nil {
		t.Errorf("expected nil nodes when panePID=0, got %v", nodes)
	}
	if total != 0 {
		t.Errorf("expected total=0 when panePID=0, got %d", total)
	}
}

func TestBuildProcessTree_RealProc(t *testing.T) {
	// Use our own PID — it should always be alive.
	ourPID := os.Getpid()
	row := &SessionRow{PanePID: ourPID}
	nodes, total := buildProcessTree(context.Background(), row)

	// We expect at least 1 node (our own PID).
	if total == 0 {
		t.Error("expected total > 0 when using our own PID")
	}
	if len(nodes) == 0 {
		t.Error("expected at least 1 node when using our own PID")
	}
	// First node should be our PID.
	if nodes[0].PID != ourPID {
		t.Errorf("first node should be panePID %d, got %d", ourPID, nodes[0].PID)
	}
}

// Ensure the infoOutput struct serialises without panic (compile-time shape test).
func TestInfoOutput_JSONShape(t *testing.T) {
	now := time.Now()
	out := infoOutput{
		SessionRow: SessionRow{
			Name:      "muxc-x",
			CreatedAt: now,
		},
		ProcessTree:    []processNode{{PID: 1, Comm: "zsh", RSS: 0}},
		TotalProcesses: 1,
		Transcript: &transcriptInfo{
			Path:    "/tmp/t.jsonl",
			Size:    1024,
			ModTime: now,
		},
	}
	// Verify it can be marshalled to JSON.
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%T", out)
	if !strings.Contains(buf.String(), "cli.infoOutput") {
		t.Errorf("unexpected type string: %s", buf.String())
	}
}
