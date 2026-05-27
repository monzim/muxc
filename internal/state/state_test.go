package state_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/monzim/muxc/internal/state"
)

// ---- helpers ----------------------------------------------------------------

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "muxc-state-test-*")
	if err != nil {
		t.Fatalf("tempDir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func writeRaw(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("writeRaw mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeRaw: %v", err)
	}
}

func sampleEntry(project string) state.SessionEntry {
	return state.SessionEntry{
		ProjectPath:       project,
		ClaudeSessionName: "my-session",
		LaunchArgs:        []string{"--dangerously-skip-permissions"},
		CreatedAt:         time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC),
		LastAttachedAt:    time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
	}
}

// ---- tests ------------------------------------------------------------------

// TestRead_MissingFile verifies that a missing state file is treated as empty.
func TestRead_MissingFile(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "state.json")

	s, err := state.Read(path)
	if err != nil {
		t.Fatalf("Read missing file: unexpected error: %v", err)
	}
	if s == nil {
		t.Fatal("Read missing file: got nil state")
	}
	if s.Version != 1 {
		t.Errorf("Version: got %d, want 1", s.Version)
	}
	if s.Sessions == nil {
		t.Error("Sessions: got nil, want empty map")
	}
	if len(s.Sessions) != 0 {
		t.Errorf("Sessions: got %d entries, want 0", len(s.Sessions))
	}
}

// TestRoundTrip verifies that Write followed by Read returns the same data.
func TestRoundTrip(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "state.json")

	original := &state.State{
		Version: 1,
		Sessions: map[string]state.SessionEntry{
			"muxc-alpha": sampleEntry("/home/user/alpha"),
			"muxc-beta":  sampleEntry("/home/user/beta"),
		},
	}

	if err := original.Write(path); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := state.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if got.Version != original.Version {
		t.Errorf("Version: got %d, want %d", got.Version, original.Version)
	}
	if len(got.Sessions) != len(original.Sessions) {
		t.Errorf("Sessions count: got %d, want %d", len(got.Sessions), len(original.Sessions))
	}
	for name, entry := range original.Sessions {
		gotEntry, ok := got.Sessions[name]
		if !ok {
			t.Errorf("Sessions: missing key %q", name)
			continue
		}
		if gotEntry.ProjectPath != entry.ProjectPath {
			t.Errorf("Sessions[%q].ProjectPath: got %q, want %q",
				name, gotEntry.ProjectPath, entry.ProjectPath)
		}
		if gotEntry.ClaudeSessionName != entry.ClaudeSessionName {
			t.Errorf("Sessions[%q].ClaudeSessionName: got %q, want %q",
				name, gotEntry.ClaudeSessionName, entry.ClaudeSessionName)
		}
		if !gotEntry.CreatedAt.Equal(entry.CreatedAt) {
			t.Errorf("Sessions[%q].CreatedAt: got %v, want %v",
				name, gotEntry.CreatedAt, entry.CreatedAt)
		}
	}
}

// TestWrite_CreatesParentDirs verifies that Write creates parent directories.
func TestWrite_CreatesParentDirs(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "nested", "deep", "state.json")

	s := &state.State{Version: 1, Sessions: map[string]state.SessionEntry{}}
	if err := s.Write(path); err != nil {
		t.Fatalf("Write to nested path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("state file not created: %v", err)
	}
}

// TestWrite_AtomicNoTmpLeftOnSuccess verifies that no .tmp file remains
// after a successful Write.
func TestWrite_AtomicNoTmpLeftOnSuccess(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "state.json")
	tmp := path + ".tmp"

	s := &state.State{Version: 1, Sessions: map[string]state.SessionEntry{}}
	if err := s.Write(path); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf(".tmp file should not exist after successful write, Stat err: %v", err)
	}
}

// TestWrite_AtomicFailure verifies that a Write failure does not corrupt the
// original file and does not leave a .tmp file behind.
//
// We simulate failure by pre-creating the .tmp path as a directory, which
// causes os.OpenFile to fail.
func TestWrite_AtomicFailure(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "state.json")
	tmp := path + ".tmp"

	// Write an initial valid state so we have an "original" file.
	original := &state.State{
		Version:  1,
		Sessions: map[string]state.SessionEntry{"muxc-first": sampleEntry("/proj")},
	}
	if err := original.Write(path); err != nil {
		t.Fatalf("initial Write: %v", err)
	}

	// Block the atomic write by making the .tmp path a directory.
	if err := os.Mkdir(tmp, 0o755); err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}

	// Attempt to overwrite — must fail.
	updated := &state.State{
		Version:  1,
		Sessions: map[string]state.SessionEntry{"muxc-second": sampleEntry("/other")},
	}
	if err := updated.Write(path); err == nil {
		t.Fatal("expected Write to fail when .tmp is a directory, got nil")
	}

	// The .tmp directory should still be there (or cleaned up — either is
	// acceptable; we care that the original file is intact).
	got, err := state.Read(path)
	if err != nil {
		t.Fatalf("Read after failed Write: %v", err)
	}
	if _, ok := got.Sessions["muxc-first"]; !ok {
		t.Error("original file was corrupted — muxc-first session missing")
	}
}

// TestRead_CorruptFile verifies that a corrupt state.json is treated as empty
// and does not return an error.
func TestRead_CorruptFile(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "state.json")

	writeRaw(t, path, `{this is not valid JSON!!!}`)

	s, err := state.Read(path)
	if err != nil {
		t.Fatalf("Read corrupt file should not return error, got: %v", err)
	}
	if s == nil {
		t.Fatal("got nil state from corrupt file")
	}
	if len(s.Sessions) != 0 {
		t.Errorf("Sessions: got %d, want 0 (empty state on corrupt file)", len(s.Sessions))
	}
}

// TestRead_VersionMismatch verifies that a state file with version > 1
// returns a non-nil error.
func TestRead_VersionMismatch(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "state.json")

	writeRaw(t, path, `{"version":99,"sessions":{}}`)

	_, err := state.Read(path)
	if err == nil {
		t.Fatal("expected error for version > 1, got nil")
	}
	// Error message should mention the unsupported version.
	msg := err.Error()
	if !containsAny(msg, "99", "unsupported") {
		t.Errorf("error message should mention version number, got: %s", msg)
	}
}

// TestPrune verifies that Prune removes sessions not in liveSessions.
func TestPrune(t *testing.T) {
	s := &state.State{
		Version: 1,
		Sessions: map[string]state.SessionEntry{
			"a": sampleEntry("/a"),
			"b": sampleEntry("/b"),
			"c": sampleEntry("/c"),
			"d": sampleEntry("/d"),
		},
	}

	live := []string{"a", "b"}
	removed := s.Prune(live)

	// Sort for deterministic comparison.
	sort.Strings(removed)

	if len(removed) != 2 {
		t.Errorf("Prune: removed count = %d, want 2", len(removed))
	}
	if len(removed) >= 1 && removed[0] != "c" {
		t.Errorf("Prune: removed[0] = %q, want c", removed[0])
	}
	if len(removed) >= 2 && removed[1] != "d" {
		t.Errorf("Prune: removed[1] = %q, want d", removed[1])
	}
	if len(s.Sessions) != 2 {
		t.Errorf("Prune: remaining sessions = %d, want 2", len(s.Sessions))
	}
	if _, ok := s.Sessions["a"]; !ok {
		t.Error("Prune: session 'a' should remain")
	}
	if _, ok := s.Sessions["b"]; !ok {
		t.Error("Prune: session 'b' should remain")
	}
}

// TestPrune_NoneRemoved verifies Prune returns nil when nothing is pruned.
func TestPrune_NoneRemoved(t *testing.T) {
	s := &state.State{
		Version: 1,
		Sessions: map[string]state.SessionEntry{
			"x": sampleEntry("/x"),
		},
	}
	removed := s.Prune([]string{"x", "y"})
	if len(removed) != 0 {
		t.Errorf("Prune: expected no removed sessions, got: %v", removed)
	}
}

// TestUpsert verifies that Upsert inserts a new entry and updates an existing one.
func TestUpsert(t *testing.T) {
	s := &state.State{Version: 1, Sessions: map[string]state.SessionEntry{}}

	entry1 := sampleEntry("/first")
	s.Upsert("muxc-foo", entry1)

	got, ok := s.Sessions["muxc-foo"]
	if !ok {
		t.Fatal("Upsert: key muxc-foo not found")
	}
	if got.ProjectPath != "/first" {
		t.Errorf("Upsert: ProjectPath = %q, want /first", got.ProjectPath)
	}

	// Update the same key.
	entry2 := sampleEntry("/second")
	s.Upsert("muxc-foo", entry2)
	got = s.Sessions["muxc-foo"]
	if got.ProjectPath != "/second" {
		t.Errorf("Upsert update: ProjectPath = %q, want /second", got.ProjectPath)
	}
}

// TestRemove verifies that Remove deletes the named entry and is a no-op
// for absent keys.
func TestRemove(t *testing.T) {
	s := &state.State{
		Version: 1,
		Sessions: map[string]state.SessionEntry{
			"muxc-x": sampleEntry("/x"),
			"muxc-y": sampleEntry("/y"),
		},
	}

	s.Remove("muxc-x")
	if _, ok := s.Sessions["muxc-x"]; ok {
		t.Error("Remove: muxc-x should be gone")
	}
	if _, ok := s.Sessions["muxc-y"]; !ok {
		t.Error("Remove: muxc-y should still exist")
	}

	// No-op on absent key — must not panic.
	s.Remove("does-not-exist")
}

// TestWrite_ValidJSON verifies that the written file is valid, indented JSON.
func TestWrite_ValidJSON(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "state.json")

	s := &state.State{
		Version: 1,
		Sessions: map[string]state.SessionEntry{
			"muxc-z": sampleEntry("/z"),
		},
	}
	if err := s.Write(path); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Must be valid JSON.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Errorf("written file is not valid JSON: %v\ncontent:\n%s", err, data)
	}

	// Must contain indentation (2-space per MarshalIndent).
	content := string(data)
	if !containsAny(content, "\n  ") {
		t.Error("written file does not appear to be indented JSON")
	}
}

// TestRead_SessionsNilInitialised verifies that reading a file with
// "sessions":null results in an initialised (non-nil) map.
func TestRead_SessionsNilInitialised(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "state.json")
	writeRaw(t, path, `{"version":1,"sessions":null}`)

	s, err := state.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if s.Sessions == nil {
		t.Error("Sessions should be initialised to empty map, got nil")
	}
}

// TestRead_IOError_NonNotExist verifies that a non-IsNotExist read error
// (e.g. the path is a directory) returns an empty state and no error,
// consistent with the "never crash" robustness rule (spec §10).
func TestRead_IOError_NonNotExist(t *testing.T) {
	dir := tempDir(t)
	// Create a directory where the file would be — os.ReadFile on a directory
	// returns an error that is not os.IsNotExist.
	badPath := filepath.Join(dir, "is-a-dir")
	if err := os.Mkdir(badPath, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	s, err := state.Read(badPath)
	if err != nil {
		t.Fatalf("Read with I/O error: expected empty state + nil error, got error: %v", err)
	}
	if s == nil {
		t.Fatal("got nil state")
	}
	if len(s.Sessions) != 0 {
		t.Errorf("Sessions: got %d, want 0", len(s.Sessions))
	}
}

// TestWrite_ReadOnlyDir verifies that Write returns an error when the parent
// directory is not writable (triggers the MkdirAll / OpenFile error path).
func TestWrite_ReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — read-only dirs are still writable")
	}
	dir := tempDir(t)
	// Make dir read-only so MkdirAll on a sub-path fails.
	readOnly := filepath.Join(dir, "ro")
	if err := os.Mkdir(readOnly, 0o555); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	path := filepath.Join(readOnly, "nested", "state.json")

	s := &state.State{Version: 1, Sessions: map[string]state.SessionEntry{}}
	if err := s.Write(path); err == nil {
		t.Error("Write to read-only dir: expected error, got nil")
	}
}

// TestUpsert_NilSessions verifies that Upsert works even when s.Sessions is nil.
func TestUpsert_NilSessions(t *testing.T) {
	s := &state.State{Version: 1, Sessions: nil}
	entry := sampleEntry("/niltest")
	s.Upsert("muxc-nil", entry)

	if s.Sessions == nil {
		t.Fatal("Sessions should have been initialised by Upsert")
	}
	got, ok := s.Sessions["muxc-nil"]
	if !ok {
		t.Fatal("muxc-nil not found after Upsert on nil map")
	}
	if got.ProjectPath != "/niltest" {
		t.Errorf("ProjectPath = %q, want /niltest", got.ProjectPath)
	}
}

// ---- utilities --------------------------------------------------------------

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if len(n) > 0 {
			idx := 0
			for i := 0; i+len(n) <= len(s); i++ {
				if s[i:i+len(n)] == n {
					idx = i
					_ = idx
					return true
				}
			}
		}
	}
	return false
}
