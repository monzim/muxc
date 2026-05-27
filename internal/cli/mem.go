// Package cli — mem subcommand.
// See spec §11.5 for the full behavioral specification.
//
// mem is "ls sorted by memory" with an extra totals row and optional bold on
// the top consumer. The data-gathering path is identical to ls; only the
// presentation differs.
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/render"
	"github.com/spf13/cobra"
)

// memTotals is the JSON wrapper added in --json mode (spec §11.5).
type memTotals struct {
	Count    int    `json:"count"`
	RSSBytes uint64 `json:"rss_bytes"`
}

// memOutput is the top-level --json object for mem (spec §11.5).
type memOutput struct {
	Sessions []SessionRow `json:"sessions"`
	Totals   memTotals    `json:"totals"`
}

// memCmd shows a memory-sorted session list with totals.
var memCmd = &cobra.Command{
	Use:   "mem",
	Short: "Show memory usage of Claude Code sessions",
	Long: `Show all muxc-managed sessions sorted by memory consumption (highest first).

Includes a totals row at the bottom. RSS (Resident Set Size) covers the Claude
process and all its descendants (MCP servers, language servers, etc.).

Note: RSS overstates shared memory. PSS would be more accurate but is not
implemented in v1. See spec §13.2.`,
	RunE: runMem,
}

// No additional flags; --json is inherited from root.

func runMem(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cfg, st, err := LoadContext(cmd)
	if err != nil {
		return err
	}

	rows, err := Gather(ctx, cfg, st, false /* muxc sessions only */)
	if err != nil {
		return fmt.Errorf("muxc: mem: %w", err)
	}

	// Persist pruned state (same as ls).
	if err := st.Write(cfg.Paths.StateFile); err != nil {
		fmt.Fprintf(os.Stderr, "muxc: warning: cannot persist state: %v\n", err)
	}

	// Always sort by memory descending (spec §11.5).
	SortRows(rows, "mem")

	// Compute totals.
	totals := ComputeMemTotals(rows)

	if IsJSON(cmd) {
		if rows == nil {
			rows = []SessionRow{}
		}
		out := memOutput{
			Sessions: rows,
			Totals:   totals,
		}
		return render.JSON(os.Stdout, out)
	}

	return RenderMemTable(os.Stdout, cfg, rows, totals)
}

// ComputeMemTotals sums RSS across all rows and returns the totals struct.
// Exported so mem_test.go can call it without constructing the full command.
func ComputeMemTotals(rows []SessionRow) memTotals {
	var total uint64
	for _, r := range rows {
		total += r.RSSPlusChildrenBytes
	}
	return memTotals{Count: len(rows), RSSBytes: total}
}

// RenderMemTable writes the mem table to w.
// If colorOn and there are rows, the first (highest-memory) row is bolded.
// A totals footer is appended after the table (spec §11.5).
// Exported so mem_test.go can call it with hand-constructed rows.
func RenderMemTable(w *os.File, cfg *config.Config, rows []SessionRow, totals memTotals) error {
	headers := []string{"NAME", "PROJECT", "CLAUDE", "UPTIME", "IDLE", "MEM", "ATTACHED"}

	colorOn := ColorEnabled(cfg)
	truncLimit := cfg.Display.TruncatePath
	home, _ := os.UserHomeDir()

	bold := color.New(color.Bold)

	tableRows := make([][]string, 0, len(rows))
	for i, r := range rows {
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

		row := []string{r.Name, project, claudeCol, uptime, idle, mem, attached}

		// Bold the top consumer when color is enabled (spec §11.5).
		if colorOn && i == 0 {
			for j, cell := range row {
				row[j] = bold.Sprint(cell)
			}
		}

		tableRows = append(tableRows, row)
	}

	if err := render.Table(w, headers, tableRows, render.TableOpts{
		Color:          colorOn,
		Headers:        headers,
		RightAlignCols: []int{5},
	}); err != nil {
		return err
	}

	// Totals footer (spec §11.5).
	_, err := fmt.Fprintf(w, "TOTAL: %d session%s, %s RSS (claude+children)\n",
		totals.Count,
		sessionPlural(totals.Count),
		render.Bytes(totals.RSSBytes),
	)
	return err
}

// sessionPlural returns "s" when n != 1, else "".
func sessionPlural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
