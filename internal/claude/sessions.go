// Package claude provides read-only access to Claude Code's session persistence
// under ~/.claude/projects/. muxc never writes to this directory (spec §2).
package claude

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Session holds metadata about a single Claude Code conversation transcript.
type Session struct {
	// ID is the UUID-based session identifier (from the .jsonl filename).
	ID string
	// Name is the human-readable session name, if found in the transcript
	// (spec §14.2). Falls back to first 8 chars of ID + "…" if absent.
	Name string
	// TranscriptPath is the absolute path to the .jsonl transcript file.
	TranscriptPath string
	// Size is the file size in bytes.
	Size int64
	// ModTime is the last modification time of the transcript file.
	ModTime time.Time
}

// EncodeProjectPath encodes an absolute project path to the directory name
// Claude uses under ~/.claude/projects/. The encoding replaces every "/"
// with "-" (spec §14):
//
//	/home/monzim/code/my145  →  -home-monzim-code-my145
//	/                        →  -
//
// Trailing slashes are trimmed before encoding (except "/" itself).
func EncodeProjectPath(path string) string {
	// Trim trailing slashes, but preserve the root "/" case.
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	// Replace every "/" with "-". Because the path starts with "/" the
	// result starts with "-".
	return strings.ReplaceAll(path, "/", "-")
}

// LatestSession returns the most recently modified Claude session for the
// given projectPath under projectsRoot.
//
// projectsRoot is typically ~/.claude/projects.
// projectPath is encoded via EncodeProjectPath before directory lookup.
//
// Returns nil, nil if the project directory does not exist or has no .jsonl files.
// Returns an error only for unexpected I/O failures.
func LatestSession(projectsRoot, projectPath string) (*Session, error) {
	dir := filepath.Join(projectsRoot, EncodeProjectPath(projectPath))

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Collect .jsonl files with their mtimes.
	type candidate struct {
		name    string
		modTime time.Time
		size    int64
	}
	var files []candidate

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, candidate{
			name:    e.Name(),
			modTime: info.ModTime(),
			size:    info.Size(),
		})
	}

	if len(files) == 0 {
		return nil, nil
	}

	// Sort descending by mtime; pick the first (most recent).
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	latest := files[0]
	transcriptPath := filepath.Join(dir, latest.name)

	// Session ID is the filename without ".jsonl".
	id := strings.TrimSuffix(latest.name, ".jsonl")

	// Derive the display name from the transcript.
	name, _ := ExtractSessionName(transcriptPath)
	if name == "" {
		// Fall back to truncated UUID (spec §14.2).
		if len(id) >= 8 {
			name = id[:8] + "…"
		} else {
			name = id
		}
	}

	return &Session{
		ID:             id,
		Name:           name,
		TranscriptPath: transcriptPath,
		Size:           latest.size,
		ModTime:        latest.modTime,
	}, nil
}
