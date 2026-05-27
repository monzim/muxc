package proc

import (
	"os"
	"path/filepath"
	"testing"
)

// procFixture returns the path to the basic proc fixture (PIDs 100→200→300).
func procFixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join("testdata", "proc")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("proc fixture missing: %v", err)
	}
	return p
}

// claudeTreeFixture returns the path to the claude-identification fixture tree.
func claudeTreeFixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join("testdata", "claude_tree")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("claude_tree fixture missing: %v", err)
	}
	return p
}

// ---- BuildTreeFromRoot + Descendants ----------------------------------------

func TestBuildTreeFromRoot_BasicChain(t *testing.T) {
	tree, err := BuildTreeFromRoot(procFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}

	// PID 100 → children should be [200], then 200 → [300].
	desc := tree.Descendants(100)
	if len(desc) != 2 {
		t.Fatalf("Descendants(100): want 2, got %d: %v", len(desc), desc)
	}

	// Check both 200 and 300 are present (order not guaranteed but BFS puts 200 first).
	pidSet := make(map[int]bool)
	for _, p := range desc {
		pidSet[p] = true
	}
	for _, want := range []int{200, 300} {
		if !pidSet[want] {
			t.Errorf("Descendants(100): missing PID %d in %v", want, desc)
		}
	}
}

func TestBuildTreeFromRoot_RootNotIncluded(t *testing.T) {
	tree, err := BuildTreeFromRoot(procFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	desc := tree.Descendants(100)
	for _, p := range desc {
		if p == 100 {
			t.Error("Descendants must NOT include rootPID itself")
		}
	}
}

func TestBuildTreeFromRoot_BFSOrder(t *testing.T) {
	tree, err := BuildTreeFromRoot(procFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	desc := tree.Descendants(100)
	// BFS: 200 must come before 300.
	if len(desc) < 2 {
		t.Fatalf("expected at least 2 descendants, got %v", desc)
	}
	if desc[0] != 200 || desc[1] != 300 {
		t.Errorf("BFS order wrong: want [200 300], got %v", desc)
	}
}

func TestDescendants_UnknownPID(t *testing.T) {
	tree, err := BuildTreeFromRoot(procFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	// PID 99999 is not in the fixture — should return nil, not panic.
	desc := tree.Descendants(99999)
	if desc != nil {
		t.Errorf("expected nil descendants for unknown PID, got %v", desc)
	}
}

func TestBuildTreeFromRoot_MissingStat(t *testing.T) {
	// If a stat file doesn't exist the PID should be skipped silently.
	// The basic fixture has no stat file for PID 400 (only status), but we can
	// also verify that our tree builds successfully even when a PID's stat is absent.
	tree, err := BuildTreeFromRoot(procFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot must not error on partially missing files: %v", err)
	}
	// PID 400 has no stat file in the basic fixture — tree should still be built.
	_ = tree
}

// ---- Cmdline ----------------------------------------------------------------

func TestCmdline_NullSeparated(t *testing.T) {
	tree, err := BuildTreeFromRoot(procFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	argv, err := tree.Cmdline(200)
	if err != nil {
		t.Fatalf("Cmdline(200): %v", err)
	}
	want := []string{"bash", "-c", "echo hello"}
	if len(argv) != len(want) {
		t.Fatalf("Cmdline(200): want %v, got %v", want, argv)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Errorf("Cmdline(200)[%d]: want %q, got %q", i, want[i], argv[i])
		}
	}
}

func TestCmdline_TrailingNullDropped(t *testing.T) {
	tree, err := BuildTreeFromRoot(procFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	// PID 100 cmdline is "zsh\x00" — should return ["zsh"], not ["zsh", ""].
	argv, err := tree.Cmdline(100)
	if err != nil {
		t.Fatalf("Cmdline(100): %v", err)
	}
	if len(argv) != 1 || argv[0] != "zsh" {
		t.Errorf("Cmdline(100): want [zsh], got %v", argv)
	}
}

func TestCmdline_NotExist(t *testing.T) {
	tree, err := BuildTreeFromRoot(procFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	_, err = tree.Cmdline(99999)
	if err != os.ErrNotExist {
		t.Errorf("Cmdline for missing PID: want os.ErrNotExist, got %v", err)
	}
}

// ---- RSS --------------------------------------------------------------------

func TestRSS_ParseVmRSS(t *testing.T) {
	tree, err := BuildTreeFromRoot(procFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	// PID 100 has VmRSS: 4096 kB → 4096 * 1024 = 4194304 bytes.
	rss, err := tree.RSS(100)
	if err != nil {
		t.Fatalf("RSS(100): %v", err)
	}
	const want = 4096 * 1024
	if rss != want {
		t.Errorf("RSS(100): want %d, got %d", want, rss)
	}
}

func TestRSS_KernelThread_NoVmRSS(t *testing.T) {
	tree, err := BuildTreeFromRoot(procFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	// PID 400 has a status file with no VmRSS line.
	rss, err := tree.RSS(400)
	if err != nil {
		t.Errorf("RSS(400): kernel thread without VmRSS should return 0,nil; got err=%v", err)
	}
	if rss != 0 {
		t.Errorf("RSS(400): want 0, got %d", rss)
	}
}

func TestRSS_MissingFile(t *testing.T) {
	tree, err := BuildTreeFromRoot(procFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	rss, err := tree.RSS(99999)
	if err != os.ErrNotExist {
		t.Errorf("RSS(99999): want os.ErrNotExist, got %v", err)
	}
	if rss != 0 {
		t.Errorf("RSS(99999): want 0, got %d", rss)
	}
}

func TestSumRSS(t *testing.T) {
	tree, err := BuildTreeFromRoot(procFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	// PIDs 100(4096kB), 200(2048kB), 300(512kB) → (4096+2048+512)*1024
	total := SumRSS(tree, []int{100, 200, 300})
	const want = (4096 + 2048 + 512) * 1024
	if total != want {
		t.Errorf("SumRSS: want %d, got %d", want, total)
	}
}

func TestSumRSS_SkipsErrors(t *testing.T) {
	tree, err := BuildTreeFromRoot(procFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	// Include a non-existent PID; it should be silently skipped.
	total := SumRSS(tree, []int{100, 99999})
	const want = 4096 * 1024
	if total != want {
		t.Errorf("SumRSS with missing PID: want %d, got %d", want, total)
	}
}

// ---- IdentifyClaude ---------------------------------------------------------

func TestIdentifyClaude_DirectBinary(t *testing.T) {
	tree, err := BuildTreeFromRoot(claudeTreeFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	// PID 1001 has argv[0] = /usr/local/bin/claude → Rule 1 match.
	// Build a subtree containing only PID 1001 as a child of 1000.
	pid, found := tree.IdentifyClaude(1000, "claude")
	if !found {
		t.Fatal("IdentifyClaude: expected to find claude, got false")
	}
	// Among 1001, 1002, 1003 all matching — lowest wins.
	if pid != 1001 {
		t.Errorf("IdentifyClaude: want lowest PID 1001, got %d", pid)
	}
}

func TestIdentifyClaude_NodeWithSlashClaudeSlash(t *testing.T) {
	// Build a tree that has only PID 1002 as a child of a fresh root,
	// so we can test Rule 2 in isolation using a temp fixture.
	dir := t.TempDir()

	writeStat := func(pid, ppid int, comm string) {
		if err := os.MkdirAll(filepath.Join(dir, itoa(pid)), 0o755); err != nil {
			t.Fatal(err)
		}
		content := itoa(pid) + " (" + comm + ") S " + itoa(ppid) + " 0 0 0 0 0 0 0 0 0 0 0 0 0 20 0 1 0 0 0 0\n"
		if err := os.WriteFile(filepath.Join(dir, itoa(pid), "stat"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeCmdline := func(pid int, args ...string) {
		var buf []byte
		for _, a := range args {
			buf = append(buf, []byte(a)...)
			buf = append(buf, 0)
		}
		if err := os.WriteFile(filepath.Join(dir, itoa(pid), "cmdline"), buf, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Root PID 2000, child PID 2001: node /opt/claude/cli.js
	writeStat(2000, 1, "zsh")
	writeCmdline(2000, "zsh")
	writeStat(2001, 2000, "node")
	writeCmdline(2001, "node", "/opt/claude/cli.js")

	tree, err := BuildTreeFromRoot(dir)
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	pid, found := tree.IdentifyClaude(2000, "claude")
	if !found {
		t.Fatal("expected to find claude via node /opt/claude/cli.js")
	}
	if pid != 2001 {
		t.Errorf("want 2001, got %d", pid)
	}
}

func TestIdentifyClaude_NodeWithHomeDotClaude(t *testing.T) {
	dir := t.TempDir()

	writeStat := func(pid, ppid int, comm string) {
		if err := os.MkdirAll(filepath.Join(dir, itoa(pid)), 0o755); err != nil {
			t.Fatal(err)
		}
		content := itoa(pid) + " (" + comm + ") S " + itoa(ppid) + " 0 0 0 0 0 0 0 0 0 0 0 0 0 20 0 1 0 0 0 0\n"
		if err := os.WriteFile(filepath.Join(dir, itoa(pid), "stat"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeCmdline := func(pid int, args ...string) {
		var buf []byte
		for _, a := range args {
			buf = append(buf, []byte(a)...)
			buf = append(buf, 0)
		}
		if err := os.WriteFile(filepath.Join(dir, itoa(pid), "cmdline"), buf, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Root PID 3000, child 3001: node /home/user/.claude/cli.js
	writeStat(3000, 1, "zsh")
	writeCmdline(3000, "zsh")
	writeStat(3001, 3000, "node")
	writeCmdline(3001, "node", "/home/user/.claude/cli.js")

	tree, err := BuildTreeFromRoot(dir)
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	pid, found := tree.IdentifyClaude(3000, "claude")
	if !found {
		t.Fatal("expected to find claude via node /home/user/.claude/cli.js")
	}
	if pid != 3001 {
		t.Errorf("want 3001, got %d", pid)
	}
}

func TestIdentifyClaude_NoMatch(t *testing.T) {
	dir := t.TempDir()

	writeStat := func(pid, ppid int, comm string) {
		if err := os.MkdirAll(filepath.Join(dir, itoa(pid)), 0o755); err != nil {
			t.Fatal(err)
		}
		content := itoa(pid) + " (" + comm + ") S " + itoa(ppid) + " 0 0 0 0 0 0 0 0 0 0 0 0 0 20 0 1 0 0 0 0\n"
		if err := os.WriteFile(filepath.Join(dir, itoa(pid), "stat"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeCmdline := func(pid int, args ...string) {
		var buf []byte
		for _, a := range args {
			buf = append(buf, []byte(a)...)
			buf = append(buf, 0)
		}
		if err := os.WriteFile(filepath.Join(dir, itoa(pid), "cmdline"), buf, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeStat(4000, 1, "zsh")
	writeCmdline(4000, "zsh")
	writeStat(4001, 4000, "python3")
	writeCmdline(4001, "python3", "server.py")

	tree, err := BuildTreeFromRoot(dir)
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	pid, found := tree.IdentifyClaude(4000, "claude")
	if found {
		t.Errorf("expected no match, got PID %d", pid)
	}
	if pid != 0 {
		t.Errorf("expected pid=0, got %d", pid)
	}
}

func TestIdentifyClaude_LowestPIDWins(t *testing.T) {
	dir := t.TempDir()

	writeStat := func(pid, ppid int, comm string) {
		if err := os.MkdirAll(filepath.Join(dir, itoa(pid)), 0o755); err != nil {
			t.Fatal(err)
		}
		content := itoa(pid) + " (" + comm + ") S " + itoa(ppid) + " 0 0 0 0 0 0 0 0 0 0 0 0 0 20 0 1 0 0 0 0\n"
		if err := os.WriteFile(filepath.Join(dir, itoa(pid), "stat"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeCmdline := func(pid int, args ...string) {
		var buf []byte
		for _, a := range args {
			buf = append(buf, []byte(a)...)
			buf = append(buf, 0)
		}
		if err := os.WriteFile(filepath.Join(dir, itoa(pid), "cmdline"), buf, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeStat(5000, 1, "zsh")
	writeCmdline(5000, "zsh")
	// Two processes that both match — higher PID and lower PID.
	writeStat(5010, 5000, "claude")
	writeCmdline(5010, "claude", "--dangerously-skip-permissions")
	writeStat(5020, 5000, "node")
	writeCmdline(5020, "node", "/opt/claude/cli.js")

	tree, err := BuildTreeFromRoot(dir)
	if err != nil {
		t.Fatalf("BuildTreeFromRoot: %v", err)
	}

	pid, found := tree.IdentifyClaude(5000, "claude")
	if !found {
		t.Fatal("expected to find claude")
	}
	if pid != 5010 {
		t.Errorf("lowest PID should win: want 5010, got %d", pid)
	}
}

func TestIdentifyClaude_MissingPIDSkippedSilently(t *testing.T) {
	// The claude_tree fixture has PID 1004 (no match) and all others matching.
	// Ensure no error even if some stat files were removed during walk.
	tree, err := BuildTreeFromRoot(claudeTreeFixture(t))
	if err != nil {
		t.Fatalf("BuildTreeFromRoot must not fail: %v", err)
	}

	// Just confirm we get a result and don't panic on any missing files.
	_, _ = tree.IdentifyClaude(1000, "claude")
}

// itoa is a local helper because strconv is already imported in the package.
func itoa(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = digits[n%10]
		n /= 10
	}
	return string(buf[pos:])
}
