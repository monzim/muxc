// Package state manages the muxc state file (~/.config/muxc/state.json).
//
// The state file is a side-table for metadata that tmux does not track
// (project path, launch args, timestamps). Losing it does not break
// functionality — muxc recovers silently on the next `muxc new`.
//
// Writes are atomic: write to <path>.tmp, fsync, rename. See spec §10.
package state

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// SessionEntry records metadata for one muxc-managed tmux session.
// The map key in State.Sessions is the full tmux session name (e.g. "muxc-myproj").
type SessionEntry struct {
	ProjectPath       string    `json:"project_path"`
	ClaudeSessionName string    `json:"claude_session_name,omitempty"`
	LaunchArgs        []string  `json:"launch_args"`
	CreatedAt         time.Time `json:"created_at"`
	LastAttachedAt    time.Time `json:"last_attached_at,omitempty"`
}

// State is the top-level state file structure.
//
// Version is an integer schema version. If Version > 1, Read returns an error
// rather than silently misinterpreting future schema changes. See spec §10.
type State struct {
	Version  int                     `json:"version"`
	Sessions map[string]SessionEntry `json:"sessions"`
}

// Read loads the state file at path and returns the parsed *State.
//
// If the file is missing, returns an empty *State (Version: 1, Sessions: {}).
// If the file exists but cannot be parsed, logs a warning and returns an empty
// *State (never crashes — see spec §10 robustness rules).
// If Version > 1, returns an error so the caller can abort with a clear message.
func Read(path string) (*State, error) {
	emptyState := func() *State {
		return &State{
			Version:  1,
			Sessions: map[string]SessionEntry{},
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Missing file is not an error — treat as empty (spec §10).
			return emptyState(), nil
		}
		// Other I/O errors: log and return empty state rather than crashing.
		slog.Warn("state: cannot read state file — treating as empty",
			"path", path, "err", err)
		return emptyState(), nil
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		// Corrupt file: log warning, return empty state (spec §10).
		slog.Warn("state: state file is corrupt — treating as empty",
			"path", path, "err", err)
		return emptyState(), nil
	}

	// Version gate: if a future binary wrote a higher-version file, refuse to
	// silently misinterpret it (spec §10).
	if s.Version > 1 {
		return nil, fmt.Errorf("state.json: unsupported version %d (this binary supports v1)", s.Version)
	}

	// Ensure the map is non-nil even when the file contained "sessions": null.
	if s.Sessions == nil {
		s.Sessions = map[string]SessionEntry{}
	}

	return &s, nil
}

// Write serialises s to path atomically: write to <path>.tmp, fsync, rename.
// Creates parent directories if they do not exist.
func (s *State) Write(path string) error {
	// 1. Ensure parent directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("state: create dir %s: %w", dir, err)
	}

	tmp := path + ".tmp"

	// cleanup is called on any failure path to remove partial writes.
	cleanup := func() {
		_ = os.Remove(tmp)
	}

	// 2. Open the temp file.
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("state: open temp file %s: %w", tmp, err)
	}

	// 3. Marshal to indented JSON for human readability.
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		f.Close()
		cleanup()
		return fmt.Errorf("state: marshal: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		cleanup()
		return fmt.Errorf("state: write temp file: %w", err)
	}

	// 4. Fsync so data is durable before rename.
	if err := f.Sync(); err != nil {
		f.Close()
		cleanup()
		return fmt.Errorf("state: sync temp file: %w", err)
	}

	// 5. Close before rename (required on some platforms; good practice everywhere).
	if err := f.Close(); err != nil {
		cleanup()
		return fmt.Errorf("state: close temp file: %w", err)
	}

	// 6. Atomic rename: replaces any existing file at path.
	if err := os.Rename(tmp, path); err != nil {
		cleanup()
		return fmt.Errorf("state: rename %s → %s: %w", tmp, path, err)
	}

	return nil
}

// Prune removes entries from s.Sessions whose name is not in liveSessions.
// Returns the list of removed session names. Called on every `muxc ls` invocation.
func (s *State) Prune(liveSessions []string) (removed []string) {
	live := make(map[string]struct{}, len(liveSessions))
	for _, name := range liveSessions {
		live[name] = struct{}{}
	}

	for name := range s.Sessions {
		if _, ok := live[name]; !ok {
			removed = append(removed, name)
			delete(s.Sessions, name)
		}
	}
	return removed
}

// Upsert inserts or updates the entry for name in s.Sessions.
func (s *State) Upsert(name string, entry SessionEntry) {
	if s.Sessions == nil {
		s.Sessions = map[string]SessionEntry{}
	}
	s.Sessions[name] = entry
}

// Remove deletes the entry for name from s.Sessions. No-op if name is absent.
func (s *State) Remove(name string) {
	delete(s.Sessions, name)
}
