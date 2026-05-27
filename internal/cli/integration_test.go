//go:build integration

// Package cli — integration test suite.
//
// These tests exercise the full muxc lifecycle against a real tmux server on a
// private socket ("muxc-int") so they never touch the user's real sessions.
//
// Prerequisites: tmux ≥ 3.0 must be installed. Run with:
//
//	make test-int
//	# or directly:
//	go test -tags integration ./internal/cli/... -v -count=1
//
// The tests are run serially (no t.Parallel()) because they share a single
// tmux server. Each test creates uniquely-named sessions to avoid collisions.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── Package-level test binary ──────────────────────────────────────────────

const intSocket = "muxc-int"

// muxcBin is the path to the compiled muxc binary, built once per test run.
var (
	muxcBin     string
	muxcBinOnce sync.Once
	muxcBinErr  error
)

// buildMuxc compiles the muxc binary into a temp directory and returns the path.
// It is called at most once per test binary execution (sync.Once guard).
func buildMuxc(t *testing.T) string {
	t.Helper()
	muxcBinOnce.Do(func() {
		// If MUXC_COV_BIN is set (Makefile coverage target), reuse the
		// caller-built coverage-instrumented binary instead of compiling.
		if cov := os.Getenv("MUXC_COV_BIN"); cov != "" {
			if _, err := os.Stat(cov); err == nil {
				muxcBin = cov
				return
			}
		}

		// Find the module root by walking up from this test file's directory.
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			muxcBinErr = fmt.Errorf("cannot determine test file path")
			return
		}
		// internal/cli/integration_test.go → walk up two directories.
		moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))

		tmpDir, err := os.MkdirTemp("", "muxc-int-bin-*")
		if err != nil {
			muxcBinErr = fmt.Errorf("create tmpdir for muxc binary: %w", err)
			return
		}

		binPath := filepath.Join(tmpDir, "muxc")
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/muxc")
		cmd.Dir = moduleRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			muxcBinErr = fmt.Errorf("go build muxc: %w\n%s", err, out)
			return
		}
		muxcBin = binPath
	})
	if muxcBinErr != nil {
		t.Fatalf("buildMuxc: %v", muxcBinErr)
	}
	return muxcBin
}

// ── Test environment helpers ────────────────────────────────────────────────

// testEnv holds paths for a single test's isolated environment.
type testEnv struct {
	homeDir     string // tmpdir serving as $HOME
	configDir   string // $MUXC_CONFIG_DIR — ~/.config/muxc equivalent
	claudeDir   string // $CLAUDE_CONFIG_DIR — ~/.claude equivalent
	projectsDir string // claudeDir/projects
	testDataBin string // path to internal/cli/testdata/bin (fake claude)
	origPATH    string // saved $PATH for restoring
}

// setupTestEnv creates isolated temporary directories for one test and
// returns a testEnv. The returned cleanup function kills the private tmux
// server and removes all temp dirs; register it with t.Cleanup.
func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	// Build the muxc binary (cached after first call).
	buildMuxc(t)

	// Locate the testdata/bin directory (fake claude).
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	cliDir := filepath.Dir(thisFile)
	testDataBin := filepath.Join(cliDir, "testdata", "bin")

	// Verify the fake claude is executable before any test runs.
	if _, err := os.Stat(filepath.Join(testDataBin, "claude")); err != nil {
		t.Fatalf("fake claude binary not found at %s: %v", testDataBin, err)
	}

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, "muxc-config")
	claudeDir := filepath.Join(homeDir, "claude")
	projectsDir := filepath.Join(claudeDir, "projects")

	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("create projects dir: %v", err)
	}

	env := &testEnv{
		homeDir:     homeDir,
		configDir:   configDir,
		claudeDir:   claudeDir,
		projectsDir: projectsDir,
		testDataBin: testDataBin,
		origPATH:    os.Getenv("PATH"),
	}

	// Kill any stale server on the private socket before the test starts.
	killPrivateServer(t)

	t.Cleanup(func() {
		killPrivateServer(t)
	})

	return env
}

// killPrivateServer kills the private tmux server on intSocket. Does not fail
// the test if the server is not running — that is the normal initial state.
func killPrivateServer(t *testing.T) {
	t.Helper()
	cmd := exec.Command("tmux", "-L", intSocket, "kill-server")
	// Ignore errors (server may not be running).
	_ = cmd.Run()
	// Small sleep to let tmux clean up its socket file.
	time.Sleep(50 * time.Millisecond)
}

// baseEnv returns the minimal environment slice for running muxc in tests.
// It puts the fake claude binary first in PATH and sets all required vars.
// When configDir is empty, MUXC_CONFIG_DIR is omitted so muxc uses
// $HOME/.config/muxc/ (the HOME-derived default).
func (e *testEnv) baseEnv() []string {
	newPATH := e.testDataBin + string(os.PathListSeparator) + e.origPATH
	env := []string{
		"PATH=" + newPATH,
		"MUXC_TMUX_SOCKET=" + intSocket,
		"CLAUDE_CONFIG_DIR=" + e.claudeDir,
		"HOME=" + e.homeDir,
		// Disable color so table output is predictable.
		"MUXC_NO_COLOR=1",
		"NO_COLOR=1",
		// Propagate TMPDIR so tmux can find its socket directory.
		"TMPDIR=" + os.TempDir(),
	}
	if e.configDir != "" {
		env = append(env, "MUXC_CONFIG_DIR="+e.configDir)
	}
	return env
}

// runMuxc executes the compiled muxc binary with the given args and the test
// environment. Returns stdout, stderr, and the exit code.
func runMuxc(t *testing.T, env *testEnv, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(muxcBin, args...)
	cmd.Env = env.baseEnv()

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Unexpected error (binary not found, etc.).
			t.Fatalf("exec muxc %v: %v", args, err)
		}
	}
	return stdout, stderr, exitCode
}

// listTmuxSessions returns the names of all sessions on the private tmux server.
func listTmuxSessions(t *testing.T) []string {
	t.Helper()
	cmd := exec.Command("tmux", "-L", intSocket, "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		// "no server running" means zero sessions.
		if strings.Contains(strings.ToLower(err.Error()), "exit status") {
			return nil
		}
		return nil
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

// makeProjectDir creates a temporary project directory and returns its path.
func makeProjectDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "muxc-proj-*")
	if err != nil {
		t.Fatalf("create project dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// writeFakeTranscript writes a minimal fake JSONL transcript for projectPath
// under claudeDir/projects/<encoded>/. Returns the transcript file path.
func writeFakeTranscript(t *testing.T, claudeDir, projectPath, sessionName string) string {
	t.Helper()

	// Encode the project path the same way Claude does: replace "/" with "-".
	encoded := strings.ReplaceAll(projectPath, "/", "-")
	projectDirName := encoded // starts with "-" because path starts with "/"

	dir := filepath.Join(claudeDir, "projects", projectDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create claude project dir: %v", err)
	}

	transcriptPath := filepath.Join(dir, "aaaabbbb-cccc-dddd-eeee-ffffffffffff.jsonl")
	content := fmt.Sprintf(`{"type":"summary","name":%q}
{"type":"user","role":"user","content":"hello"}
{"type":"assistant","role":"assistant","content":"hi there"}
`, sessionName)
	if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write fake transcript: %v", err)
	}
	return transcriptPath
}

// ── Tests ─────────────────────────────────────────────────────────────────

// TestLifecycle exercises the full session lifecycle: new → ls shows it →
// kill → ls shows it gone. Covers §24 acceptance criterion #3.
func TestLifecycle(t *testing.T) {
	env := setupTestEnv(t)
	projDir := makeProjectDir(t)

	// Step 1: create a session with --no-launch (no real claude needed for ls test).
	stdout, stderr, code := runMuxc(t, env, "new", projDir, "--no-launch")
	if code != 0 {
		t.Fatalf("muxc new: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "started muxc-") {
		t.Errorf("muxc new: expected 'started muxc-' in stdout; got %q", stdout)
	}

	// Extract the created session name from the output line "started muxc-xxx in /path".
	var sessionName string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "started ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				sessionName = parts[1]
			}
		}
	}
	if sessionName == "" {
		t.Fatalf("could not parse session name from muxc new output: %q", stdout)
	}

	// Step 2: ls should show the session.
	stdout, stderr, code = runMuxc(t, env, "ls")
	if code != 0 {
		t.Fatalf("muxc ls: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, sessionName) {
		t.Errorf("muxc ls: expected session %q in output; got:\n%s", sessionName, stdout)
	}

	// Step 3: kill the session with --yes to skip confirmation.
	// Pass the name without the "muxc-" prefix (muxc resolves it).
	shortName := strings.TrimPrefix(sessionName, "muxc-")
	stdout, stderr, code = runMuxc(t, env, "kill", shortName, "--yes")
	if code != 0 {
		t.Fatalf("muxc kill: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "killed") {
		t.Errorf("muxc kill: expected 'killed' in output; got %q", stdout)
	}

	// Step 4: ls should now be empty.
	stdout, stderr, code = runMuxc(t, env, "ls")
	if code != 0 {
		t.Fatalf("muxc ls after kill: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if strings.Contains(stdout, sessionName) {
		t.Errorf("muxc ls after kill: session %q still appears in output:\n%s", sessionName, stdout)
	}

	// Verify the tmux server also has no session with that name.
	sessions := listTmuxSessions(t)
	for _, s := range sessions {
		if s == sessionName {
			t.Errorf("tmux still has session %q after muxc kill", sessionName)
		}
	}
}

// TestLsJSON verifies that `muxc ls --json` emits valid JSON with the
// expected field names. Covers §24 acceptance criterion #4.
func TestLsJSON(t *testing.T) {
	env := setupTestEnv(t)
	projDir := makeProjectDir(t)

	// Create a session first.
	_, _, code := runMuxc(t, env, "new", projDir, "--no-launch")
	if code != 0 {
		t.Fatalf("muxc new: exit %d", code)
	}

	stdout, stderr, code := runMuxc(t, env, "ls", "--json")
	if code != 0 {
		t.Fatalf("muxc ls --json: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// Must be valid JSON.
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("muxc ls --json: invalid JSON: %v\noutput: %s", err, stdout)
	}

	if len(rows) == 0 {
		t.Fatal("muxc ls --json: expected at least one session row, got empty array")
	}

	// Check that required fields are present (§11.1 JSON schema).
	required := []string{
		"name", "project_path", "tmux_session_id",
		"created_at", "activity_at", "idle_seconds",
		"uptime_seconds", "attached", "pane_pid",
		"claude_pid", "rss_bytes", "rss_plus_children_bytes",
	}
	row := rows[0]
	for _, field := range required {
		if _, ok := row[field]; !ok {
			t.Errorf("muxc ls --json: missing required field %q in row: %v", field, row)
		}
	}

	// name must start with "muxc-".
	if name, ok := row["name"].(string); !ok || !strings.HasPrefix(name, "muxc-") {
		t.Errorf("muxc ls --json: row name %v does not start with 'muxc-'", row["name"])
	}

	// project_path must be non-empty and equal the project dir.
	if pp, ok := row["project_path"].(string); !ok || pp == "" {
		t.Errorf("muxc ls --json: project_path is empty or wrong type: %v", row["project_path"])
	}
}

// TestKillIdleDryRun verifies that `muxc kill --idle 0s --dry-run` lists
// targets without actually killing them. Covers §24 acceptance criterion #5.
func TestKillIdleDryRun(t *testing.T) {
	env := setupTestEnv(t)
	proj1 := makeProjectDir(t)
	proj2 := makeProjectDir(t)

	// Create two sessions.
	for _, proj := range []string{proj1, proj2} {
		_, _, code := runMuxc(t, env, "new", proj, "--no-launch")
		if code != 0 {
			t.Fatalf("muxc new %s: exit %d", proj, code)
		}
	}

	// Collect session names before the dry-run.
	beforeSessions := listTmuxSessions(t)
	if len(beforeSessions) < 2 {
		t.Fatalf("expected at least 2 sessions before dry-run, got: %v", beforeSessions)
	}

	// A 0s idle threshold means "all sessions are idle" — safe for testing.
	stdout, stderr, code := runMuxc(t, env, "kill", "--idle", "0s", "--dry-run")
	if code != 0 {
		t.Fatalf("muxc kill --idle 0s --dry-run: exit %d\nstdout: %s\nstderr: %s",
			code, stdout, stderr)
	}

	// Dry-run output must say "would kill".
	if !strings.Contains(stdout, "would kill") {
		t.Errorf("muxc kill --dry-run: expected 'would kill' in output; got %q", stdout)
	}

	// Sessions must still exist after dry-run.
	afterSessions := listTmuxSessions(t)
	if len(afterSessions) < len(beforeSessions) {
		t.Errorf("dry-run killed sessions: before=%v after=%v", beforeSessions, afterSessions)
	}
}

// TestInfoTranscriptPath verifies that `muxc info <name>` shows the transcript
// path from ~/.claude/projects/. Covers §24 acceptance criterion #6.
func TestInfoTranscriptPath(t *testing.T) {
	env := setupTestEnv(t)
	projDir := makeProjectDir(t)

	// Write a fake transcript for this project.
	transcriptPath := writeFakeTranscript(t, env.claudeDir, projDir, "info-test-session")

	// Create the session.
	stdout, _, code := runMuxc(t, env, "new", projDir, "--no-launch")
	if code != 0 {
		t.Fatalf("muxc new: exit %d\nstdout: %s", code, stdout)
	}

	// Parse the session name.
	var sessionName string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "started ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				sessionName = parts[1]
			}
		}
	}
	if sessionName == "" {
		t.Fatalf("could not parse session name from: %q", stdout)
	}

	shortName := strings.TrimPrefix(sessionName, "muxc-")
	infoOut, infoErr, code := runMuxc(t, env, "info", shortName)
	if code != 0 {
		t.Fatalf("muxc info %s: exit %d\nstdout: %s\nstderr: %s",
			shortName, code, infoOut, infoErr)
	}

	// The info output should contain the transcript path (or its home-relative
	// equivalent since the transcript is under claudeDir which is a temp path).
	if !strings.Contains(infoOut, transcriptPath) &&
		!strings.Contains(infoOut, filepath.Base(transcriptPath)) {
		t.Errorf("muxc info: transcript path %q not found in output:\n%s",
			transcriptPath, infoOut)
	}
}

// TestKillPreservesTranscript verifies that killing a session leaves the
// Claude transcript file on disk. Covers §24 acceptance criterion #8.
func TestKillPreservesTranscript(t *testing.T) {
	env := setupTestEnv(t)
	projDir := makeProjectDir(t)

	// Write a fake transcript.
	transcriptPath := writeFakeTranscript(t, env.claudeDir, projDir, "preserve-test")

	// Create session.
	stdout, _, code := runMuxc(t, env, "new", projDir, "--no-launch")
	if code != 0 {
		t.Fatalf("muxc new: exit %d", code)
	}

	var sessionName string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "started ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				sessionName = parts[1]
			}
		}
	}

	// Kill the session.
	shortName := strings.TrimPrefix(sessionName, "muxc-")
	_, _, code = runMuxc(t, env, "kill", shortName, "--yes")
	if code != 0 {
		t.Fatalf("muxc kill: exit %d", code)
	}

	// Transcript file must still exist.
	if _, err := os.Stat(transcriptPath); err != nil {
		t.Errorf("transcript %q was removed after muxc kill: %v", transcriptPath, err)
	}
}

// TestNoTmuxServer verifies that `muxc ls` exits 0 and emits an empty result
// when no tmux server is running. Covers §24 acceptance criterion #14.
func TestNoTmuxServer(t *testing.T) {
	env := setupTestEnv(t)

	// Ensure no private server is running (setupTestEnv already did this, but
	// be explicit for test clarity).
	killPrivateServer(t)

	stdout, stderr, code := runMuxc(t, env, "ls")
	if code != 0 {
		t.Fatalf("muxc ls with no server: exit %d\nstdout: %s\nstderr: %s",
			code, stdout, stderr)
	}
	// The output should be an empty table header or empty — no error.
	// It must not contain session names.
	if strings.Contains(stdout, "muxc-") {
		t.Errorf("unexpected session names in ls output with no server: %q", stdout)
	}
}

// TestNoTmuxServerJSON verifies that `muxc ls --json` returns "[]" when no
// tmux server is running.
func TestNoTmuxServerJSON(t *testing.T) {
	env := setupTestEnv(t)
	killPrivateServer(t)

	stdout, stderr, code := runMuxc(t, env, "ls", "--json")
	if code != 0 {
		t.Fatalf("muxc ls --json with no server: exit %d\nstdout: %s\nstderr: %s",
			code, stdout, stderr)
	}

	var rows []any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("muxc ls --json with no server: invalid JSON: %v\noutput: %q", err, stdout)
	}
	if len(rows) != 0 {
		t.Errorf("muxc ls --json with no server: expected [], got %d rows", len(rows))
	}
}

// TestCorruptedState verifies that muxc handles a corrupted state.json
// gracefully: exit 0 and treat state as empty. Covers §24 criterion #13.
func TestCorruptedState(t *testing.T) {
	env := setupTestEnv(t)

	// Write garbage to the state file.
	if err := os.MkdirAll(env.configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	stateFile := filepath.Join(env.configDir, "state.json")
	if err := os.WriteFile(stateFile, []byte("this is not valid JSON {{{{"), 0o644); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	stdout, stderr, code := runMuxc(t, env, "ls")
	if code != 0 {
		t.Fatalf("muxc ls with corrupt state: exit %d\nstdout: %s\nstderr: %s",
			code, stdout, stderr)
	}
	// Must not crash; output may be an empty table or warning.
	// Specifically must NOT contain an unhandled panic or stack trace.
	if strings.Contains(stderr, "panic:") {
		t.Errorf("muxc ls with corrupt state: unexpected panic in stderr: %s", stderr)
	}
}

// TestNoConfigDir verifies that muxc creates the config directory on first
// write when ~/.config/muxc/ does not exist.
// Covers §24 acceptance criterion #12.
//
// Note: MUXC_CONFIG_DIR controls where config.toml lives; the state.json
// path defaults to $HOME/.config/muxc/state.json (expanded from HOME env var).
// We set HOME to a fresh temp dir so we control the entire config landscape.
func TestNoConfigDir(t *testing.T) {
	env := setupTestEnv(t)
	projDir := makeProjectDir(t)

	// Use a fresh HOME that has no .config/muxc/ at all.
	freshHome := t.TempDir()
	env.homeDir = freshHome
	// Clear MUXC_CONFIG_DIR so the code falls back to HOME-based path.
	env.configDir = ""

	// muxc new should create ~/.config/muxc/ under freshHome and write state.json.
	stdout, stderr, code := runMuxc(t, env, "new", projDir, "--no-launch")
	if code != 0 {
		t.Fatalf("muxc new with missing config dir: exit %d\nstdout: %s\nstderr: %s",
			code, stdout, stderr)
	}

	// Verify the state.json was created under freshHome.
	stateFile := filepath.Join(freshHome, ".config", "muxc", "state.json")
	if _, err := os.Stat(stateFile); err != nil {
		t.Errorf("muxc new did not create state.json at %s: %v", stateFile, err)
	}

	_ = stdout
}

// TestDoctor verifies that `muxc doctor` runs end-to-end and produces output
// for all 10 checks without crashing. Some checks may WARN or FAIL in CI
// (e.g. no real ~/.claude/projects). Covers §24 criterion #2.
func TestDoctor(t *testing.T) {
	env := setupTestEnv(t)

	stdout, stderr, code := runMuxc(t, env, "doctor")
	// Exit code 0 (all OK/WARN) or 1 (any FAIL) are both acceptable — we only
	// require that the command does not crash and produces check output.
	if code > 1 {
		t.Fatalf("muxc doctor: unexpected exit code %d\nstdout: %s\nstderr: %s",
			code, stdout, stderr)
	}

	// The ten spec §11.7 check names must all appear in the output.
	expectedChecks := []string{
		"tmux installed",
		"claude on PATH",
		"~/.claude/projects/",
		"~/.config/muxc/",
		"config.toml",
		"state.json",
		"orphan state entries",
		"fzf installed",
		"/proc accessible",
		"running as real user",
	}
	for _, check := range expectedChecks {
		if !strings.Contains(stdout, check) {
			t.Errorf("muxc doctor: expected check %q in output; stdout:\n%s", check, stdout)
		}
	}

	// Must not panic.
	if strings.Contains(stderr, "panic:") {
		t.Errorf("muxc doctor: unexpected panic:\n%s", stderr)
	}
}

// TestDoctorJSON verifies `muxc doctor --json` produces a parseable JSON array
// of exactly 10 check results, each with name/status fields.
func TestDoctorJSON(t *testing.T) {
	env := setupTestEnv(t)

	stdout, stderr, code := runMuxc(t, env, "doctor", "--json")
	if code > 1 {
		t.Fatalf("muxc doctor --json: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	var checks []map[string]any
	if err := json.Unmarshal([]byte(stdout), &checks); err != nil {
		t.Fatalf("muxc doctor --json: invalid JSON: %v\noutput: %s", err, stdout)
	}

	if len(checks) != 10 {
		t.Errorf("muxc doctor --json: expected 10 checks, got %d", len(checks))
	}

	for i, c := range checks {
		if _, ok := c["name"]; !ok {
			t.Errorf("check[%d]: missing 'name' field", i)
		}
		if status, ok := c["status"].(string); !ok || status == "" {
			t.Errorf("check[%d]: missing or empty 'status' field", i)
		}
	}
}

// TestJSONJqCompatibility pipes `muxc ls --json` through jq and asserts exit 0.
// Skips if jq is not available. Covers §24 criterion #4 follow-up.
func TestJSONJqCompatibility(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not found in PATH; skipping JSON/jq compatibility test")
	}

	env := setupTestEnv(t)
	projDir := makeProjectDir(t)

	// Create a session so there is something to list.
	_, _, code := runMuxc(t, env, "new", projDir, "--no-launch")
	if code != 0 {
		t.Fatalf("muxc new: exit %d", code)
	}

	// Run muxc ls --json and capture the output.
	stdout, stderr, code := runMuxc(t, env, "ls", "--json")
	if code != 0 {
		t.Fatalf("muxc ls --json: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// Pipe into jq.
	jqCmd := exec.Command("jq", ".")
	jqCmd.Stdin = strings.NewReader(stdout)
	jqOut, err := jqCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jq failed on muxc ls --json output: %v\njq output: %s\nls output: %s",
			err, jqOut, stdout)
	}
}

// TestProjectLocalOverride verifies that a .muxc.toml in the project directory
// with custom launch_args is picked up and stored in state.json.
// Covers the project-local override feature from spec §9.
func TestProjectLocalOverride(t *testing.T) {
	env := setupTestEnv(t)
	projDir := makeProjectDir(t)

	// Write a .muxc.toml with a custom launch_args.
	muxcToml := filepath.Join(projDir, ".muxc.toml")
	tomlContent := `[defaults]
launch_args = ["--custom-flag", "--another-flag"]
`
	if err := os.WriteFile(muxcToml, []byte(tomlContent), 0o644); err != nil {
		t.Fatalf("write .muxc.toml: %v", err)
	}

	// Create the session; --no-claude-name so we get predictable args.
	stdout, stderr, code := runMuxc(t, env, "new", projDir, "--no-launch", "--no-claude-name")
	if code != 0 {
		t.Fatalf("muxc new with .muxc.toml: exit %d\nstdout: %s\nstderr: %s",
			code, stdout, stderr)
	}

	// Parse session name.
	var sessionName string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "started ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				sessionName = parts[1]
			}
		}
	}
	if sessionName == "" {
		t.Fatalf("could not parse session name from: %q", stdout)
	}

	// Read state.json and check launch_args.
	// state.json is stored at $HOME/.config/muxc/state.json (HOME-expanded default).
	// The MUXC_CONFIG_DIR env var controls where config.toml is read, but
	// the state file path defaults to ~/.config/muxc/state.json under HOME.
	stateFile := filepath.Join(env.homeDir, ".config", "muxc", "state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state.json: %v", err)
	}

	var stateDoc struct {
		Sessions map[string]struct {
			LaunchArgs []string `json:"launch_args"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(data, &stateDoc); err != nil {
		t.Fatalf("parse state.json: %v\ncontent: %s", err, data)
	}

	entry, ok := stateDoc.Sessions[sessionName]
	if !ok {
		t.Fatalf("session %q not found in state.json\ncontent: %s", sessionName, data)
	}

	// The launch args must contain the custom flags from .muxc.toml.
	var hasCustomFlag, hasAnotherFlag bool
	for _, arg := range entry.LaunchArgs {
		if arg == "--custom-flag" {
			hasCustomFlag = true
		}
		if arg == "--another-flag" {
			hasAnotherFlag = true
		}
	}
	if !hasCustomFlag || !hasAnotherFlag {
		t.Errorf("state.json launch_args %v missing custom flags from .muxc.toml", entry.LaunchArgs)
	}
}

// TestSanitize verifies that a session name with uppercase and spaces is
// sanitized to a lowercase tmux-safe name. Covers spec §11.2 step 2.
func TestSanitize(t *testing.T) {
	env := setupTestEnv(t)
	projDir := makeProjectDir(t)

	// Create a session with an ugly --name value.
	stdout, stderr, code := runMuxc(t, env, "new", projDir, "--no-launch", "--name", "My Test Session")
	if code != 0 {
		t.Fatalf("muxc new --name 'My Test Session': exit %d\nstdout: %s\nstderr: %s",
			code, stdout, stderr)
	}

	// Extract the created session name.
	var sessionName string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "started ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				sessionName = parts[1]
			}
		}
	}
	if sessionName == "" {
		t.Fatalf("could not parse session name from: %q", stdout)
	}

	// Must be all-lowercase and contain no spaces.
	if strings.ToLower(sessionName) != sessionName {
		t.Errorf("session name %q is not lowercase", sessionName)
	}
	if strings.Contains(sessionName, " ") {
		t.Errorf("session name %q contains spaces", sessionName)
	}

	// Must start with the default prefix.
	if !strings.HasPrefix(sessionName, "muxc-") {
		t.Errorf("session name %q does not start with 'muxc-'", sessionName)
	}

	// Verify the session exists in tmux with the sanitized name.
	sessions := listTmuxSessions(t)
	found := false
	for _, s := range sessions {
		if s == sessionName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("sanitized session %q not found in tmux sessions: %v", sessionName, sessions)
	}
}

// TestLsEmptyJSON verifies that `muxc ls --json` returns "[]" (not null)
// when there are no sessions. Covers the empty-case spec §11.1.
func TestLsEmptyJSON(t *testing.T) {
	env := setupTestEnv(t)
	// No sessions created.

	stdout, stderr, code := runMuxc(t, env, "ls", "--json")
	if code != 0 {
		t.Fatalf("muxc ls --json empty: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	trimmed := strings.TrimSpace(stdout)
	if trimmed != "[]" {
		// Also accept a valid empty JSON array that is merely formatted differently.
		var rows []any
		if err := json.Unmarshal([]byte(trimmed), &rows); err != nil || len(rows) != 0 {
			t.Errorf("muxc ls --json with no sessions: want '[]', got %q", trimmed)
		}
	}
}

// TestKillByFullName verifies that `muxc kill muxc-<name>` works when the
// full session name (including prefix) is supplied.
func TestKillByFullName(t *testing.T) {
	env := setupTestEnv(t)
	projDir := makeProjectDir(t)

	stdout, _, code := runMuxc(t, env, "new", projDir, "--no-launch")
	if code != 0 {
		t.Fatalf("muxc new: exit %d", code)
	}

	var sessionName string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "started ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				sessionName = parts[1]
			}
		}
	}

	// Kill using the full name (with "muxc-" prefix).
	killOut, killErr, code := runMuxc(t, env, "kill", sessionName, "--yes")
	if code != 0 {
		t.Fatalf("muxc kill %s: exit %d\nstdout: %s\nstderr: %s",
			sessionName, code, killOut, killErr)
	}

	// Session must be gone.
	sessions := listTmuxSessions(t)
	for _, s := range sessions {
		if s == sessionName {
			t.Errorf("session %q still exists after kill by full name", sessionName)
		}
	}
}

// TestNewInvalidPath verifies that `muxc new` on a non-existent directory
// exits 2. The root command has SilenceErrors=true, so the error text is not
// printed to stderr by cobra; we only assert the exit code.
func TestNewInvalidPath(t *testing.T) {
	env := setupTestEnv(t)

	_, _, code := runMuxc(t, env, "new", "/tmp/this-path-does-not-exist-muxc-test-xyz")
	if code != 2 {
		t.Errorf("muxc new invalid path: expected exit 2, got %d", code)
	}
}

// TestKillAllFlag verifies `muxc kill --all --yes` removes every muxc session.
func TestKillAllFlag(t *testing.T) {
	env := setupTestEnv(t)
	proj1 := makeProjectDir(t)
	proj2 := makeProjectDir(t)

	for _, p := range []string{proj1, proj2} {
		_, _, code := runMuxc(t, env, "new", p, "--no-launch")
		if code != 0 {
			t.Fatalf("muxc new: exit %d", code)
		}
	}

	before := listTmuxSessions(t)
	if len(before) < 2 {
		t.Fatalf("expected >= 2 sessions before kill --all, got %v", before)
	}

	_, _, code := runMuxc(t, env, "kill", "--all", "--yes")
	if code != 0 {
		t.Fatalf("muxc kill --all --yes: exit %d", code)
	}

	// All muxc- sessions must be gone.
	after := listTmuxSessions(t)
	for _, s := range after {
		if strings.HasPrefix(s, "muxc-") {
			t.Errorf("session %q still exists after kill --all", s)
		}
	}
}
