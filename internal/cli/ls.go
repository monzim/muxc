// Package cli — ls subcommand.
// See spec §11.1 for the full behavioral specification.
package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/render"
	"github.com/spf13/cobra"
)

// lsCmd lists all muxc-managed tmux sessions with memory and idle stats.
//
// Spec §11.1: list sessions, filter by prefix (unless --all), gather proc+claude
// data, prune stale state, render table or JSON.
var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List Claude Code sessions",
	Long: `List all muxc-managed tmux sessions with project, memory, and idle information.

By default only sessions matching the configured prefix (muxc-) are shown.
Use --all to include all tmux sessions.`,
	RunE: runLs,
}

func init() {
	lsCmd.Flags().Bool("all", false, "include tmux sessions not matching the muxc- prefix")
	lsCmd.Flags().String("sort", "name", "sort key: name | mem | idle | created")
	lsCmd.Flags().String("filter", "", "glob pattern matched against session name")
	// --json is inherited from the root persistent flag.
}

func runLs(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cfg, st, err := LoadContext(cmd)
	if err != nil {
		return err
	}

	includeAll, _ := cmd.Flags().GetBool("all")
	sortKey, _ := cmd.Flags().GetString("sort")
	filterPat, _ := cmd.Flags().GetString("filter")

	// Default: muxc-managed sessions + external tmux sessions that have a
	// Claude process detected. --all expands to every tmux session.
	mode := ExternalWithClaude
	if includeAll {
		mode = ExternalAll
	}
	rows, err := Gather(ctx, cfg, st, mode)
	if err != nil {
		return fmt.Errorf("muxc: ls: %w", err)
	}

	// Persist pruned state silently (spec §11.1 step 4). Non-fatal on error.
	if err := st.Write(cfg.Paths.StateFile); err != nil {
		fmt.Fprintf(os.Stderr, "muxc: warning: cannot persist state: %v\n", err)
	}

	// Apply filter and sort.
	rows = FilterByGlob(rows, filterPat)
	SortRows(rows, sortKey)

	if IsJSON(cmd) {
		// Empty → emit [] not null (spec §11.1 empty case).
		if rows == nil {
			rows = []SessionRow{}
		}
		return render.JSON(os.Stdout, rows)
	}

	return RenderLsTable(os.Stdout, cfg, rows)
}

// RenderLsTable writes the ls table to w per spec §11.1 / §15.1.
// Exported so ls_test.go can call it with hand-constructed rows.
func RenderLsTable(w *os.File, cfg *config.Config, rows []SessionRow) error {
	headers := []string{"NAME", "PROJECT", "CLAUDE", "UPTIME", "IDLE", "MEM", "ATTACHED"}

	colorOn := ColorEnabled(cfg)
	truncLimit := cfg.Display.TruncatePath

	home, _ := os.UserHomeDir()

	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		project := homeRelative(r.ProjectPath, home)
		if truncLimit > 0 {
			project = render.LeftTruncate(project, truncLimit)
		}

		claudeCol := claudeDisplay(r)
		uptime := render.Duration(time.Duration(r.UptimeSeconds) * time.Second)
		idle := render.Duration(time.Duration(r.IdleSeconds) * time.Second)
		mem := render.Bytes(r.RSSPlusChildrenBytes)
		attached := "no"
		if r.Attached {
			attached = "yes"
		}
		tableRows = append(tableRows, []string{
			nameCell(r), project, claudeCol, uptime, idle, mem, attached,
		})
	}

	// MEM is column index 5; right-align it (spec §15.1).
	return render.Table(w, headers, tableRows, render.TableOpts{
		Color:          colorOn,
		Headers:        headers,
		RightAlignCols: []int{5},
	})
}

// ColorEnabled reports whether ANSI color should be emitted based on
// cfg.Display.Color and the terminal / env-var state (spec §15.3).
//
//   - "always" → true
//   - "never"  → false
//   - "auto"   → true only when stdout is a TTY and NO_COLOR / MUXC_NO_COLOR are unset
func ColorEnabled(cfg *config.Config) bool {
	switch cfg.Display.Color {
	case "always":
		return true
	case "never":
		return false
	default: // "auto"
		if os.Getenv("NO_COLOR") != "" || os.Getenv("MUXC_NO_COLOR") != "" {
			return false
		}
		// Check whether stdout is a terminal.
		fi, err := os.Stdout.Stat()
		if err != nil {
			return false
		}
		return (fi.Mode() & os.ModeCharDevice) != 0
	}
}

// homeRelative replaces the home-directory prefix in path with "~/".
func homeRelative(path, home string) string {
	if home == "" || path == "" {
		return path
	}
	if strings.HasPrefix(path, home+"/") {
		return "~/" + path[len(home)+1:]
	}
	if path == home {
		return "~"
	}
	return path
}

// nameCell formats the NAME column. External (non-muxc) sessions get a "*"
// suffix so users can spot them in the list at a glance. The JSON output
// keeps `is_external` as a typed boolean — the marker is for human eyes only.
func nameCell(r SessionRow) string {
	if r.IsExternal {
		return r.Name + "*"
	}
	return r.Name
}

// claudeDisplay returns the CLAUDE column value for a row.
//
// Priority (spec §11.1):
//  1. ClaudeSessionName if non-empty.
//  2. First 8 chars of ClaudeSessionID + "…" if non-empty.
//  3. "-"
func claudeDisplay(r SessionRow) string {
	if r.ClaudeSessionName != "" {
		return r.ClaudeSessionName
	}
	if r.ClaudeSessionID != "" {
		id := r.ClaudeSessionID
		if len(id) > 8 {
			return id[:8] + "…"
		}
		return id
	}
	return "-"
}
