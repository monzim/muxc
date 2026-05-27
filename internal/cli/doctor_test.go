package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monzim/muxc/internal/doctor"
	"github.com/monzim/muxc/internal/state"
	"github.com/monzim/muxc/internal/tmux"
)

// ── doctor.CheckClaudeBinary ─────────────────────────────────────────────────────

func TestCheckClaudeBinary_Missing(t *testing.T) {
	result := doctor.CheckClaudeBinary("__definitely_not_a_real_binary_xyz__")
	if result.Status != StatusFail {
		t.Errorf("expected FAIL for missing binary, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "not found") {
		t.Errorf("expected 'not found' in message, got: %q", result.Message)
	}
}

func TestCheckClaudeBinary_Exists(t *testing.T) {
	// "true" is available on every POSIX system and is guaranteed to be in PATH.
	result := doctor.CheckClaudeBinary("true")
	if result.Status != StatusOK {
		t.Errorf("expected OK for 'true' binary, got %s: %s", result.Status, result.Message)
	}
}

func TestCheckClaudeBinary_ExistsByPath(t *testing.T) {
	// Create a temporary executable to verify the check works with a real binary.
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Prepend dir to PATH so exec.LookPath finds it.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+origPath)

	result := doctor.CheckClaudeBinary("fake-claude")
	if result.Status != StatusOK {
		t.Errorf("expected OK for fake binary in PATH, got %s: %s", result.Status, result.Message)
	}
}

// ── doctor.CheckClaudeProjects ───────────────────────────────────────────────────

func TestCheckClaudeProjects_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	result := doctor.CheckClaudeProjects(dir)
	if result.Status != StatusOK {
		t.Errorf("expected OK for existing dir, got %s: %s", result.Status, result.Message)
	}
}

func TestCheckClaudeProjects_NonExistent(t *testing.T) {
	result := doctor.CheckClaudeProjects("/tmp/__muxc_does_not_exist_xyz__")
	if result.Status != StatusWarn {
		t.Errorf("expected WARN for non-existent dir, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "not found") {
		t.Errorf("expected 'not found' in WARN message, got: %q", result.Message)
	}
}

func TestCheckClaudeProjects_WithFakeStructure(t *testing.T) {
	// Build a fake ~/.claude/projects structure.
	dir := t.TempDir()
	projDir := filepath.Join(dir, "-home-user-code-proj1")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	result := doctor.CheckClaudeProjects(dir)
	if result.Status != StatusOK {
		t.Errorf("expected OK for fake structure, got %s: %s", result.Status, result.Message)
	}
}

// ── doctor.CheckConfigDir ────────────────────────────────────────────────────────

func TestCheckConfigDir_ExistingWritableDir(t *testing.T) {
	dir := t.TempDir()
	result := doctor.CheckConfigDir(dir)
	if result.Status != StatusOK {
		t.Errorf("expected OK for writable dir, got %s: %s", result.Status, result.Message)
	}
}

func TestCheckConfigDir_NonExistentCreatable(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "muxc-new")
	result := doctor.CheckConfigDir(dir)
	if result.Status != StatusOK {
		t.Errorf("expected OK after creating dir, got %s: %s", result.Status, result.Message)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected dir to be created, got stat error: %v", err)
	}
}

func TestCheckConfigDir_ReadOnlyParent(t *testing.T) {
	// Create a read-only parent so MkdirAll fails.
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o444); err != nil {
		t.Skip("cannot chmod parent dir, skipping")
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	dir := filepath.Join(parent, "sub", "muxc")
	result := doctor.CheckConfigDir(dir)
	if result.Status != StatusFail {
		// On some systems (e.g. running as root) this may succeed.
		if os.Getuid() == 0 {
			t.Skip("running as root, permission test not meaningful")
		}
		t.Errorf("expected FAIL for read-only parent, got %s: %s", result.Status, result.Message)
	}
}

// ── doctor.CheckConfigToml ───────────────────────────────────────────────────────

func TestCheckConfigToml_Absent(t *testing.T) {
	// An empty temp dir (no config.toml) is fine — defaults apply.
	dir := t.TempDir()
	result := doctor.CheckConfigToml(dir)
	if result.Status != StatusOK {
		t.Errorf("expected OK when config.toml is absent, got %s: %s", result.Status, result.Message)
	}
}

func TestCheckConfigToml_ValidFile(t *testing.T) {
	dir := t.TempDir()
	// Write a minimal valid TOML.
	content := "[defaults]\nprefix = \"muxc-\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result := doctor.CheckConfigToml(dir)
	if result.Status != StatusOK {
		t.Errorf("expected OK for valid config.toml, got %s: %s", result.Status, result.Message)
	}
}

func TestCheckConfigToml_InvalidToml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("{not toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := doctor.CheckConfigToml(dir)
	if result.Status != StatusFail {
		t.Errorf("expected FAIL for invalid TOML, got %s", result.Status)
	}
}

// ── doctor.CheckStateJSON ────────────────────────────────────────────────────────

func TestCheckStateJSON_Absent(t *testing.T) {
	result := doctor.CheckStateJSON("/tmp/__muxc_no_state_xyz__.json")
	if result.Status != StatusOK {
		t.Errorf("expected OK for absent state file, got %s", result.Status)
	}
}

func TestCheckStateJSON_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	content := `{"version":1,"sessions":{}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result := doctor.CheckStateJSON(path)
	if result.Status != StatusOK {
		t.Errorf("expected OK for valid state file, got %s: %s", result.Status, result.Message)
	}
}

func TestCheckStateJSON_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := doctor.CheckStateJSON(path)
	if result.Status != StatusFail {
		t.Errorf("expected FAIL for corrupt state file, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "malformed") {
		t.Errorf("expected 'malformed' in message, got: %q", result.Message)
	}
}

// ── doctor.CheckOrphanEntries ────────────────────────────────────────────────────

// fakeSessionLister implements doctorSessionLister.
type fakeSessionLister struct {
	sessions []tmux.Session
	err      error
}

func (f fakeSessionLister) ListSessions(_ context.Context) ([]tmux.Session, error) {
	return f.sessions, f.err
}

func TestCheckOrphanEntries_EmptyState(t *testing.T) {
	st := &state.State{Version: 1, Sessions: map[string]state.SessionEntry{}}
	orig := doctor.SessionsLister
	doctor.SessionsLister = fakeSessionLister{}
	defer func() { doctor.SessionsLister = orig }()

	result := doctor.CheckOrphanEntries(context.Background(), st)
	if result.Status != StatusOK {
		t.Errorf("expected OK for empty state, got %s: %s", result.Status, result.Message)
	}
}

func TestCheckOrphanEntries_AllLive(t *testing.T) {
	st := &state.State{
		Version: 1,
		Sessions: map[string]state.SessionEntry{
			"muxc-a": {ProjectPath: "/proj/a"},
			"muxc-b": {ProjectPath: "/proj/b"},
		},
	}
	orig := doctor.SessionsLister
	doctor.SessionsLister = fakeSessionLister{
		sessions: []tmux.Session{
			{Name: "muxc-a"},
			{Name: "muxc-b"},
		},
	}
	defer func() { doctor.SessionsLister = orig }()

	result := doctor.CheckOrphanEntries(context.Background(), st)
	if result.Status != StatusOK {
		t.Errorf("expected OK when all entries have live sessions, got %s: %s", result.Status, result.Message)
	}
}

func TestCheckOrphanEntries_HasOrphans(t *testing.T) {
	st := &state.State{
		Version: 1,
		Sessions: map[string]state.SessionEntry{
			"muxc-a": {ProjectPath: "/proj/a"},
			"muxc-b": {ProjectPath: "/proj/b"},
			"muxc-c": {ProjectPath: "/proj/c"}, // orphan
		},
	}
	orig := doctor.SessionsLister
	doctor.SessionsLister = fakeSessionLister{
		sessions: []tmux.Session{
			{Name: "muxc-a"},
			{Name: "muxc-b"},
		},
	}
	defer func() { doctor.SessionsLister = orig }()

	result := doctor.CheckOrphanEntries(context.Background(), st)
	if result.Status != StatusWarn {
		t.Errorf("expected WARN for orphan entry, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "1 orphan") {
		t.Errorf("expected '1 orphan' in message, got: %q", result.Message)
	}
}

func TestCheckOrphanEntries_ListSessionsError(t *testing.T) {
	st := &state.State{
		Version: 1,
		Sessions: map[string]state.SessionEntry{
			"muxc-x": {ProjectPath: "/proj/x"},
		},
	}
	orig := doctor.SessionsLister
	doctor.SessionsLister = fakeSessionLister{err: errors.New("tmux not running")}
	defer func() { doctor.SessionsLister = orig }()

	result := doctor.CheckOrphanEntries(context.Background(), st)
	// When listing fails we return WARN (cannot determine), not FAIL.
	if result.Status != StatusWarn {
		t.Errorf("expected WARN when session listing fails, got %s", result.Status)
	}
}

// ── hasFailure ────────────────────────────────────────────────────────────

func TestHasFailure_AllOK(t *testing.T) {
	checks := []CheckResult{
		{Status: StatusOK},
		{Status: StatusOK},
		{Status: StatusOK},
	}
	if hasFailure(checks) {
		t.Error("expected false for all-OK checks")
	}
}

func TestHasFailure_WithOneFail(t *testing.T) {
	checks := []CheckResult{
		{Status: StatusOK},
		{Status: StatusFail, Message: "something broke"},
		{Status: StatusOK},
	}
	if !hasFailure(checks) {
		t.Error("expected true when one check is FAIL")
	}
}

func TestHasFailure_OnlyWarn(t *testing.T) {
	checks := []CheckResult{
		{Status: StatusOK},
		{Status: StatusWarn, Message: "advisory"},
	}
	if hasFailure(checks) {
		t.Error("expected false for WARN-only (no FAIL)")
	}
}

func TestHasFailure_Empty(t *testing.T) {
	if hasFailure(nil) {
		t.Error("expected false for empty check list")
	}
}

// ── renderCheckText ───────────────────────────────────────────────────────

func TestRenderCheckText_ContainsAllStatuses(t *testing.T) {
	checks := []CheckResult{
		{Name: "check-ok", Status: StatusOK, Message: "version 3.4"},
		{Name: "check-warn", Status: StatusWarn, Message: "advisory note"},
		{Name: "check-fail", Status: StatusFail, Message: "broken thing"},
	}
	var w strings.Builder
	renderCheckText(&w, checks, false /* no color */)
	out := w.String()

	if !strings.Contains(out, "[OK]") {
		t.Errorf("output should contain '[OK]', got:\n%s", out)
	}
	if !strings.Contains(out, "[WARN]") {
		t.Errorf("output should contain '[WARN]', got:\n%s", out)
	}
	if !strings.Contains(out, "[FAIL]") {
		t.Errorf("output should contain '[FAIL]', got:\n%s", out)
	}
}

func TestRenderCheckText_CheckNameInOutput(t *testing.T) {
	checks := []CheckResult{
		{Name: "tmux installed (≥ 3.0)", Status: StatusOK, Message: "3.4"},
	}
	var w strings.Builder
	renderCheckText(&w, checks, false)
	out := w.String()
	if !strings.Contains(out, "tmux installed (≥ 3.0)") {
		t.Errorf("output should contain check name, got:\n%s", out)
	}
	if !strings.Contains(out, "3.4") {
		t.Errorf("output should contain message '3.4', got:\n%s", out)
	}
}

func TestRenderCheckText_NoMessageOmitsColon(t *testing.T) {
	checks := []CheckResult{
		{Name: "fzf installed", Status: StatusOK},
	}
	var w strings.Builder
	renderCheckText(&w, checks, false)
	out := w.String()
	// When message is empty no ": " separator should appear.
	if strings.Contains(out, "fzf installed:") {
		t.Errorf("empty message should not produce trailing colon, got:\n%s", out)
	}
}

func TestRenderCheckText_OneLinePerCheck(t *testing.T) {
	checks := []CheckResult{
		{Name: "a", Status: StatusOK},
		{Name: "b", Status: StatusWarn},
		{Name: "c", Status: StatusFail},
	}
	var w strings.Builder
	renderCheckText(&w, checks, false)
	lines := strings.Split(strings.TrimRight(w.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines for 3 checks, got %d:\n%s", len(lines), w.String())
	}
}

// ── doctor.CheckTmuxVersion (with fake versioner) ────────────────────────────────

type fakeTmuxVersioner struct {
	version string
	err     error
}

func (f fakeTmuxVersioner) TmuxVersion(_ context.Context) (string, error) {
	return f.version, f.err
}

func TestCheckTmuxVersion_ErrorReturnsFail(t *testing.T) {
	orig := doctor.TmuxVersioner
	doctor.TmuxVersioner = fakeTmuxVersioner{err: errors.New("not found")}
	defer func() { doctor.TmuxVersioner = orig }()

	result := doctor.CheckTmuxVersion(context.Background())
	if result.Status != StatusFail {
		t.Errorf("expected FAIL when tmux errors, got %s", result.Status)
	}
}

func TestCheckTmuxVersion_OldVersionFails(t *testing.T) {
	orig := doctor.TmuxVersioner
	doctor.TmuxVersioner = fakeTmuxVersioner{version: "2.9"}
	defer func() { doctor.TmuxVersioner = orig }()

	result := doctor.CheckTmuxVersion(context.Background())
	if result.Status != StatusFail {
		t.Errorf("expected FAIL for tmux 2.9, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "2.9") {
		t.Errorf("expected version '2.9' in message, got: %q", result.Message)
	}
}

func TestCheckTmuxVersion_GoodVersionOK(t *testing.T) {
	orig := doctor.TmuxVersioner
	doctor.TmuxVersioner = fakeTmuxVersioner{version: "3.4"}
	defer func() { doctor.TmuxVersioner = orig }()

	result := doctor.CheckTmuxVersion(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected OK for tmux 3.4, got %s: %s", result.Status, result.Message)
	}
	if result.Message != "3.4" {
		t.Errorf("expected message to be the version string, got: %q", result.Message)
	}
}

func TestCheckTmuxVersion_ExactMinimumOK(t *testing.T) {
	orig := doctor.TmuxVersioner
	doctor.TmuxVersioner = fakeTmuxVersioner{version: "3.0"}
	defer func() { doctor.TmuxVersioner = orig }()

	result := doctor.CheckTmuxVersion(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected OK for exactly tmux 3.0, got %s: %s", result.Status, result.Message)
	}
}

// ── doctor.CheckFzf ──────────────────────────────────────────────────────────────

func TestCheckFzf_Missing(t *testing.T) {
	// Temporarily override PATH so fzf is not found.
	t.Setenv("PATH", "/dev/null")
	result := doctor.CheckFzf()
	if result.Status != StatusWarn {
		t.Errorf("expected WARN when fzf is not on PATH, got %s", result.Status)
	}
}

// ── doctor.CheckUID ──────────────────────────────────────────────────────────────

func TestCheckUID_NonRoot(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test non-root check when running as root")
	}
	result := doctor.CheckUID()
	if result.Status != StatusOK {
		t.Errorf("expected OK for non-root user, got %s: %s", result.Status, result.Message)
	}
}

// ── doctor.CheckProcfs ───────────────────────────────────────────────────────────

func TestCheckProcfs_Linux(t *testing.T) {
	// Skip on non-Linux; the check itself handles that case.
	result := doctor.CheckProcfs()
	// On the CI host (Linux), /proc is always available → OK.
	// On macOS/Windows it would be WARN. Either is valid; just ensure no panic.
	if result.Status == "" {
		t.Error("doctor.CheckProcfs returned empty status")
	}
}

// ── Optional: real tmux check (skip if tmux not installed) ────────────────

func TestCheckTmuxVersion_Real(t *testing.T) {
	// Use the real versioner (don't override doctor.TmuxVersioner).
	// Skip if tmux is not installed.
	ctx := context.Background()
	result := doctor.CheckTmuxVersion(ctx)
	if result.Status == StatusFail && strings.Contains(result.Message, "not found") {
		t.Skip("tmux not installed, skipping real version check")
	}
	// Either OK or FAIL (old version) — both are valid outcomes. Just ensure no panic.
	if result.Name == "" {
		t.Error("doctor.CheckTmuxVersion returned empty Name")
	}
}
