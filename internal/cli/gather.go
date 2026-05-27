// Package cli — session data gathering shared by ls, mem, info, kill, attach.
//
// Gather is the central function that walks tmux sessions, procfs, and the
// Claude projects directory to produce a []SessionRow — one per live session.
// All CLI commands that need per-session data call Gather rather than re-
// implementing the tmux+proc+claude dance themselves.
//
// Spec references: §11.1 (ls), §11.5 (mem), §11.6 (info), §13, §14.
package cli

import (
	"context"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/monzim/muxc/internal/claude"
	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/proc"
	"github.com/monzim/muxc/internal/state"
	"github.com/monzim/muxc/internal/tmux"
)

// SessionRow is the fully-enriched record for one tmux session.
// It is the canonical data type passed between Gather and all rendering code.
// JSON tags reflect the spec §11.1 JSON output schema.
type SessionRow struct {
	Name              string    `json:"name"`
	ProjectPath       string    `json:"project_path"`
	ClaudeSessionName string    `json:"claude_session_name"`
	ClaudeSessionID   string    `json:"claude_session_id"`
	TmuxSessionID     string    `json:"tmux_session_id"`
	CreatedAt         time.Time `json:"created_at"`
	ActivityAt        time.Time `json:"activity_at"`
	IdleSeconds       int64     `json:"idle_seconds"`
	UptimeSeconds     int64     `json:"uptime_seconds"`
	Attached          bool      `json:"attached"`
	AttachedClients   int       `json:"attached_clients"`
	PanePID           int       `json:"pane_pid"`
	ClaudePID         int       `json:"claude_pid"`
	// RSSBytes is the RSS of the Claude process alone (bytes). Zero if not found.
	RSSBytes uint64 `json:"rss_bytes"`
	// RSSPlusChildrenBytes is the sum of RSS for Claude + all its descendants.
	// If no Claude process was found, it is the RSS of the entire pane subtree.
	RSSPlusChildrenBytes uint64 `json:"rss_plus_children_bytes"`
	IsExternal           bool   `json:"is_external"`

	// StateEntry is the matching state.json entry for this session; not serialised.
	StateEntry *state.SessionEntry `json:"-"`
}

// Gather walks tmux + state + procfs + claude projects and returns one
// SessionRow per live session.
//
// If includeExternal is true, sessions whose names do NOT start with
// cfg.Defaults.Prefix are included with IsExternal=true.
//
// Gather prunes stale state entries (names in state but not in tmux) from the
// in-memory State. It does NOT persist; the caller decides when to call
// st.Write.
//
// On tmux-not-running (ErrNoServer) ListSessions returns empty slice + nil,
// so Gather returns an empty slice + nil per spec §12.
func Gather(ctx context.Context, cfg *config.Config, st *state.State, includeExternal bool) ([]SessionRow, error) {
	// 1. List live tmux sessions. ErrNoServer → empty list, nil error (spec §12).
	sessions, err := tmux.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	// 2. Filter by prefix unless includeExternal is requested.
	prefix := cfg.Defaults.Prefix
	var filtered []tmux.Session
	for _, s := range sessions {
		if strings.HasPrefix(s.Name, prefix) {
			filtered = append(filtered, s)
		} else if includeExternal {
			filtered = append(filtered, s)
		}
	}

	// 3. Build the process tree ONCE for all sessions (spec §13.1 invariant).
	tree, err := proc.BuildTree()
	if err != nil {
		// /proc unavailable — log and continue; all RSS values will be 0.
		slog.Warn("gather: cannot build process tree", "err", err)
		tree = nil
	}

	now := time.Now()

	// 4. Build one SessionRow per session.
	rows := make([]SessionRow, 0, len(filtered))
	liveNames := make([]string, 0, len(filtered))

	for _, s := range filtered {
		liveNames = append(liveNames, s.Name)

		row := SessionRow{
			Name:            s.Name,
			TmuxSessionID:   s.ID,
			CreatedAt:       s.Created,
			ActivityAt:      s.Activity,
			IdleSeconds:     int64(now.Sub(s.Activity).Seconds()),
			UptimeSeconds:   int64(now.Sub(s.Created).Seconds()),
			Attached:        s.Attached > 0,
			AttachedClients: s.Attached,
			IsExternal:      !strings.HasPrefix(s.Name, prefix),
		}

		// 4a. Get the first pane PID.
		panePIDs, err := tmux.ListPanePIDs(ctx, s.Name)
		if err != nil {
			slog.Warn("gather: cannot get pane PIDs", "session", s.Name, "err", err)
		} else if len(panePIDs) > 0 {
			row.PanePID = panePIDs[0]
		}

		// 4b. Walk process tree for this session.
		if tree != nil && row.PanePID > 0 {
			claudePID, found := tree.IdentifyClaude(row.PanePID, cfg.Defaults.ClaudeBin)

			if found {
				row.ClaudePID = claudePID

				// RSS for Claude alone.
				rss, err := tree.RSS(claudePID)
				if err == nil {
					row.RSSBytes = rss
				}

				// RSSPlusChildren: Claude PID + all its descendants.
				descendants := tree.Descendants(claudePID)
				allPIDs := append([]int{claudePID}, descendants...)
				row.RSSPlusChildrenBytes = proc.SumRSS(tree, allPIDs)
			} else {
				// No Claude found — sum the whole pane subtree (spec §13.4).
				row.ClaudePID = 0
				allPIDs := append([]int{row.PanePID}, tree.Descendants(row.PanePID)...)
				row.RSSPlusChildrenBytes = proc.SumRSS(tree, allPIDs)
			}
		}

		// 4c. Copy project path and Claude session name from state.
		if entry, ok := st.Sessions[s.Name]; ok {
			row.ProjectPath = entry.ProjectPath
			row.ClaudeSessionName = entry.ClaudeSessionName
			entryCopy := entry
			row.StateEntry = &entryCopy
		}

		// 4d. If ShowClaudeID is set and we have a project path, look up the
		//     latest Claude session from ~/.claude/projects/ (spec §11.1 step 3).
		if cfg.Display.ShowClaudeID && row.ProjectPath != "" {
			cs, err := claude.LatestSession(cfg.Paths.ClaudeProjects, row.ProjectPath)
			if err != nil {
				slog.Warn("gather: cannot look up claude session",
					"session", s.Name, "project", row.ProjectPath, "err", err)
			} else if cs != nil {
				row.ClaudeSessionID = cs.ID
				// Override ClaudeSessionName from state only when state has no name.
				if row.ClaudeSessionName == "" {
					row.ClaudeSessionName = cs.Name
				}
			}
		}

		rows = append(rows, row)
	}

	// 5. Prune stale state entries (in-memory only; caller persists if desired).
	st.Prune(liveNames)

	return rows, nil
}

// SortRows sorts rows in-place by the given key.
//
// Supported keys:
//
//	"name"    — ascending by Name (default)
//	"mem"     — descending by RSSPlusChildrenBytes (top consumer first)
//	"idle"    — descending by IdleSeconds (most idle first)
//	"created" — ascending by CreatedAt (oldest first)
//
// Unknown keys fall back to "name" sorting.
func SortRows(rows []SessionRow, key string) {
	switch key {
	case "mem":
		sort.Slice(rows, func(i, j int) bool {
			// Descending: larger memory first.
			return rows[i].RSSPlusChildrenBytes > rows[j].RSSPlusChildrenBytes
		})
	case "idle":
		sort.Slice(rows, func(i, j int) bool {
			// Descending: most idle first.
			return rows[i].IdleSeconds > rows[j].IdleSeconds
		})
	case "created":
		sort.Slice(rows, func(i, j int) bool {
			// Ascending: oldest first.
			return rows[i].CreatedAt.Before(rows[j].CreatedAt)
		})
	default:
		// "name" and any unknown key → ascending by Name.
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].Name < rows[j].Name
		})
	}
}

// FilterByGlob keeps only rows whose Name matches the shell glob pattern.
// An empty pattern returns rows unchanged. Rows that fail filepath.Match
// (pattern error or no match) are dropped.
func FilterByGlob(rows []SessionRow, pattern string) []SessionRow {
	if pattern == "" {
		return rows
	}
	result := rows[:0:0] // zero-len, same backing array avoided to prevent mutation
	for _, r := range rows {
		matched, err := filepath.Match(pattern, r.Name)
		if err != nil {
			// Invalid pattern: skip row (same behaviour as no match).
			continue
		}
		if matched {
			result = append(result, r)
		}
	}
	return result
}
