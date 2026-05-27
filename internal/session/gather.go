// Package session is the canonical data layer that the CLI and TUI both
// consume. It owns SessionRow (the enriched per-session record), the
// ExternalMode filter, and the Gather function that walks tmux + procfs +
// the Claude projects directory.
//
// Splitting this out of internal/cli broke the cli<->tui import cycle that
// arose when the post-v1.0 TUI also needed live session data.
//
// Spec references: §11.1 (ls), §11.5 (mem), §11.6 (info), §13, §14.
package session

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

// Row is the fully-enriched record for one tmux session.
// It is the canonical data type passed between Gather and all rendering code.
// JSON tags reflect the spec §11.1 JSON output schema.
//
// (Named Row to keep call sites short — the CLI re-exports it as SessionRow
// for backward compatibility.)
type Row struct {
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

// ExternalMode controls how Gather treats tmux sessions whose names do NOT
// start with cfg.Defaults.Prefix (i.e., sessions muxc didn't create).
//
// Spec §11.1 originally exposed only a `--all` flag (ExternalAll), but in
// post-v1.0 muxc surfaces external sessions that are running Claude by default
// so users can see all their Claude sessions in one place. See CLAUDE.md for
// the rationale on this divergence.
type ExternalMode int

const (
	// ExternalNone returns only muxc-prefixed sessions. Used by `kill --idle`
	// and `kill --all` so muxc never kills sessions it didn't create.
	ExternalNone ExternalMode = iota
	// ExternalWithClaude returns muxc-prefixed sessions plus non-muxc sessions
	// that have a Claude process detected. This is the default for `ls`, `mem`,
	// `attach`, and the interactive REPL/TUI.
	ExternalWithClaude
	// ExternalAll returns every tmux session, including non-muxc ones without
	// Claude. Triggered by the `--all` flag on `ls` and `mem`.
	ExternalAll
)

// Gather walks tmux + state + procfs + claude projects and returns one Row per
// live session, filtered according to mode.
//
// Gather always evaluates every tmux session through the Claude-process
// detection step (so ClaudePID is known for every row), then drops rows that
// don't match `mode`. Filtering happens AFTER row enrichment to keep the
// detection logic in one place.
//
// Gather prunes stale state entries (names in state but not in tmux) from the
// in-memory State. It does NOT persist; the caller decides when to call
// st.Write.
//
// On tmux-not-running (ErrNoServer) ListSessions returns empty slice + nil,
// so Gather returns an empty slice + nil per spec §12.
func Gather(ctx context.Context, cfg *config.Config, st *state.State, mode ExternalMode) ([]Row, error) {
	sessions, err := tmux.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	prefix := cfg.Defaults.Prefix

	tree, err := proc.BuildTree()
	if err != nil {
		slog.Warn("gather: cannot build process tree", "err", err)
		tree = nil
	}

	now := time.Now()

	rows := make([]Row, 0, len(sessions))
	liveNames := make([]string, 0, len(sessions))

	for _, s := range sessions {
		liveNames = append(liveNames, s.Name)

		row := Row{
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

		panePIDs, err := tmux.ListPanePIDs(ctx, s.Name)
		if err != nil {
			slog.Warn("gather: cannot get pane PIDs", "session", s.Name, "err", err)
		} else if len(panePIDs) > 0 {
			row.PanePID = panePIDs[0]
		}

		if tree != nil && row.PanePID > 0 {
			claudePID, found := tree.IdentifyClaude(row.PanePID, cfg.Defaults.ClaudeBin)
			if found {
				row.ClaudePID = claudePID

				if rss, err := tree.RSS(claudePID); err == nil {
					row.RSSBytes = rss
				}

				descendants := tree.Descendants(claudePID)
				allPIDs := append([]int{claudePID}, descendants...)
				row.RSSPlusChildrenBytes = proc.SumRSS(tree, allPIDs)
			} else {
				row.ClaudePID = 0
				allPIDs := append([]int{row.PanePID}, tree.Descendants(row.PanePID)...)
				row.RSSPlusChildrenBytes = proc.SumRSS(tree, allPIDs)
			}
		}

		if entry, ok := st.Sessions[s.Name]; ok {
			row.ProjectPath = entry.ProjectPath
			row.ClaudeSessionName = entry.ClaudeSessionName
			entryCopy := entry
			row.StateEntry = &entryCopy
		}

		if row.ProjectPath == "" && row.ClaudePID > 0 && tree != nil {
			if cwd, err := tree.CWD(row.ClaudePID); err == nil {
				row.ProjectPath = cwd
			}
		}

		if cfg.Display.ShowClaudeID && row.ProjectPath != "" {
			cs, err := claude.LatestSession(cfg.Paths.ClaudeProjects, row.ProjectPath)
			if err != nil {
				slog.Warn("gather: cannot look up claude session",
					"session", s.Name, "project", row.ProjectPath, "err", err)
			} else if cs != nil {
				row.ClaudeSessionID = cs.ID
				if row.ClaudeSessionName == "" {
					row.ClaudeSessionName = cs.Name
				}
			}
		}

		rows = append(rows, row)
	}

	st.Prune(liveNames)

	return filterByMode(rows, mode), nil
}

// filterByMode drops external rows according to the requested mode.
func filterByMode(rows []Row, mode ExternalMode) []Row {
	if mode == ExternalAll {
		return rows
	}
	out := rows[:0]
	for _, r := range rows {
		switch {
		case !r.IsExternal:
			out = append(out, r)
		case mode == ExternalWithClaude && r.ClaudePID > 0:
			out = append(out, r)
		}
	}
	return out
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
func SortRows(rows []Row, key string) {
	switch key {
	case "mem":
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].RSSPlusChildrenBytes > rows[j].RSSPlusChildrenBytes
		})
	case "idle":
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].IdleSeconds > rows[j].IdleSeconds
		})
	case "created":
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].CreatedAt.Before(rows[j].CreatedAt)
		})
	default:
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].Name < rows[j].Name
		})
	}
}

// FilterByGlob keeps only rows whose Name matches the shell glob pattern.
// An empty pattern returns rows unchanged. Rows that fail filepath.Match
// (pattern error or no match) are dropped.
func FilterByGlob(rows []Row, pattern string) []Row {
	if pattern == "" {
		return rows
	}
	result := rows[:0:0]
	for _, r := range rows {
		matched, err := filepath.Match(pattern, r.Name)
		if err != nil {
			continue
		}
		if matched {
			result = append(result, r)
		}
	}
	return result
}

// FilterByMode is the exported wrapper around filterByMode so callers outside
// the package (TUI, kill --idle) can apply the same filter without re-running
// Gather. Used when a screen wants to re-filter cached rows on a UI toggle.
func FilterByMode(rows []Row, mode ExternalMode) []Row {
	return filterByMode(rows, mode)
}
