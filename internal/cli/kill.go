// Package cli — kill subcommand.
// See spec §11.4 for the full behavioral specification.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/monzim/muxc/internal/state"
	"github.com/monzim/muxc/internal/tmux"
)

// killCmd kills one or more muxc-managed tmux sessions.
var killCmd = &cobra.Command{
	Use:   "kill [name]",
	Short: "Kill one or more Claude Code sessions",
	Long: `Kill one or more muxc-managed tmux sessions.

Exactly one mode must be given:
  kill <name>           kill the named session
  kill --idle <dur>     kill sessions idle longer than <dur> (e.g. 2h, 30m)
  kill --all            kill every muxc session
  kill --stale          remove state.json entries with no matching tmux session`,
	RunE: runKill,
}

func init() {
	killCmd.Flags().String("idle", "", "kill sessions idle longer than this duration (e.g. 2h, 30m)")
	killCmd.Flags().Bool("all", false, "kill every muxc session")
	killCmd.Flags().Bool("stale", false, "remove state.json entries with no matching tmux session")
	killCmd.Flags().BoolP("yes", "y", false, "skip confirmation prompt")
	killCmd.Flags().Bool("dry-run", false, "print what would be killed without acting")
}

// runKill is the RunE implementation for killCmd.
func runKill(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// ── Flag extraction ───────────────────────────────────────────────────────
	idleStr, _ := cmd.Flags().GetString("idle")
	doAll, _ := cmd.Flags().GetBool("all")
	doStale, _ := cmd.Flags().GetBool("stale")
	yes, _ := cmd.Flags().GetBool("yes")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	hasName := len(args) > 0

	// ── Mode validation: exactly one mode must be active ─────────────────────
	modeCount := 0
	if hasName {
		modeCount++
	}
	if idleStr != "" {
		modeCount++
	}
	if doAll {
		modeCount++
	}
	if doStale {
		modeCount++
	}

	if modeCount == 0 {
		return &ExitError{
			Code:    2,
			Message: "muxc: kill requires exactly one mode: <name>, --idle <dur>, --all, or --stale",
		}
	}
	if modeCount > 1 {
		return &ExitError{
			Code:    2,
			Message: "muxc: kill: only one of <name>, --idle, --all, --stale may be given at a time",
		}
	}

	// ── Load context ──────────────────────────────────────────────────────────
	cfg, st, err := LoadContext(cmd)
	if err != nil {
		return err
	}

	// ── --stale: special case, no tmux kill involved ──────────────────────────
	if doStale {
		liveSessions, lsErr := tmux.ListSessions(ctx)
		if lsErr != nil && !errors.Is(lsErr, tmux.ErrNoServer) {
			return fmt.Errorf("muxc: list tmux sessions: %w", lsErr)
		}
		liveNames := make([]string, 0, len(liveSessions))
		for _, s := range liveSessions {
			liveNames = append(liveNames, s.Name)
		}

		removed := runStaleKill(st, liveNames)
		if len(removed) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no stale state entries found")
			return nil
		}
		if err := st.Write(cfg.Paths.StateFile); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "muxc: write state: %v\n", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "removed %d stale state %s: %s\n",
			len(removed),
			pluralise("entry", "entries", len(removed)),
			strings.Join(removed, ", "),
		)
		return nil
	}

	// ── Build target list ─────────────────────────────────────────────────────
	type target struct {
		name string
		// rss is the pre-kill RSS (for summary); 0 if unknown.
		rss uint64
	}

	var targets []target

	switch {
	case hasName:
		// hasName == len(args) > 0 (set above), so args[0] is safe. gosec G602
		// can't follow that across the switch, hence the nolint.
		name := args[0] //nolint:gosec // G602: hasName guard ensures len(args) > 0
		resolved, rErr := ResolveSessionName(ctx, cfg, name)
		if rErr != nil {
			if errors.Is(rErr, ErrNotFound) {
				return &ExitError{
					Code:    2,
					Message: fmt.Sprintf("muxc: tmux session %q not found\n  list available sessions with `muxc ls`", name),
				}
			}
			return rErr
		}
		targets = []target{{name: resolved}}

	case doAll:
		sessions, lsErr := tmux.ListSessions(ctx)
		if lsErr != nil && !errors.Is(lsErr, tmux.ErrNoServer) {
			return fmt.Errorf("muxc: list tmux sessions: %w", lsErr)
		}
		for _, s := range sessions {
			if strings.HasPrefix(s.Name, cfg.Defaults.Prefix) {
				targets = append(targets, target{name: s.Name})
			}
		}

	case idleStr != "":
		idleDur, parseErr := time.ParseDuration(idleStr)
		if parseErr != nil {
			return &ExitError{
				Code: 2,
				Message: fmt.Sprintf(
					"muxc: --idle: %q is not a valid duration\n  use Go duration syntax, e.g. \"2h\", \"30m\", \"1h30m\"",
					idleStr,
				),
			}
		}

		// ExternalNone: never kill external (non-muxc) sessions via --idle.
		rows, gErr := Gather(ctx, cfg, st, ExternalNone)
		if gErr != nil {
			return fmt.Errorf("muxc: gather sessions: %w", gErr)
		}

		idle := filterByIdle(rows, idleDur)
		for _, r := range idle {
			targets = append(targets, target{name: r.Name, rss: r.RSSPlusChildrenBytes})
		}
	}

	// ── Empty target set ──────────────────────────────────────────────────────
	if len(targets) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no sessions matched")
		return nil
	}

	// ── Confirmation prompt ───────────────────────────────────────────────────
	if cfg.Defaults.ConfirmKill && !yes && !dryRun {
		// Build a SessionRow slice for the targets so we can use formatKillTargets.
		// For name-mode the RSS might be 0; that's acceptable. idle-mode populates
		// t.rss from the Gather() rows above; name/all modes leave it as 0.
		confirmRows := make([]SessionRow, 0, len(targets))
		for _, t := range targets {
			confirmRows = append(confirmRows, SessionRow{Name: t.name, RSSPlusChildrenBytes: t.rss})
		}

		prompt := fmt.Sprintf("Kill %d %s?\n%s",
			len(targets),
			pluralise("session", "sessions", len(targets)),
			formatKillTargets(confirmRows),
		)
		fmt.Fprint(cmd.OutOrStdout(), prompt)

		ok, cErr := Confirm(os.Stdin, cmd.OutOrStdout(), "")
		if cErr != nil {
			return fmt.Errorf("muxc: read confirmation: %w", cErr)
		}
		if !ok {
			fmt.Fprintln(cmd.OutOrStdout(), "aborted")
			return nil
		}
	}

	// ── Dry-run ───────────────────────────────────────────────────────────────
	if dryRun {
		names := make([]string, len(targets))
		for i, t := range targets {
			names[i] = t.name
		}
		fmt.Fprintf(cmd.OutOrStdout(), "would kill: %s\n", strings.Join(names, ", "))
		return nil
	}

	// ── Kill each target ──────────────────────────────────────────────────────
	var killed []string
	var totalFreed uint64

	for _, t := range targets {
		if kErr := tmux.KillSession(ctx, t.name); kErr != nil {
			if errors.Is(kErr, tmux.ErrSessionNotFound) {
				// Session already gone — just clean state.
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "muxc: kill %s: %v\n", t.name, kErr)
				continue // don't remove from state on unexpected errors
			}
		}
		st.Remove(t.name)
		killed = append(killed, t.name)
		totalFreed += t.rss
	}

	// ── Persist state ─────────────────────────────────────────────────────────
	if len(killed) > 0 {
		if wErr := st.Write(cfg.Paths.StateFile); wErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "muxc: write state: %v\n", wErr)
		}
	}

	// ── Print summary ─────────────────────────────────────────────────────────
	fmt.Fprintln(cmd.OutOrStdout(), summarizeKill(killed, totalFreed))
	return nil
}

// ── Pure helper functions (exported for testing) ──────────────────────────────

// filterByIdle returns only those rows whose IdleSeconds >= min.Seconds().
func filterByIdle(rows []SessionRow, min time.Duration) []SessionRow {
	threshold := int64(min.Seconds())
	result := make([]SessionRow, 0, len(rows))
	for _, r := range rows {
		if r.IdleSeconds >= threshold {
			result = append(result, r)
		}
	}
	return result
}

// runStaleKill prunes state entries not present in liveTmuxNames.
// It modifies st in-place and returns the names that were removed.
// The caller is responsible for persisting st afterward.
func runStaleKill(st *state.State, liveTmuxNames []string) (removed []string) {
	return st.Prune(liveTmuxNames)
}

// formatKillTargets produces the indented session list shown in the
// confirmation prompt. Rows with zero RSS omit the memory column.
func formatKillTargets(rows []SessionRow) string {
	var sb strings.Builder
	for _, r := range rows {
		if r.RSSPlusChildrenBytes > 0 {
			mb := float64(r.RSSPlusChildrenBytes) / (1024 * 1024)
			idle := time.Duration(r.IdleSeconds) * time.Second
			fmt.Fprintf(&sb, "  %s\t(idle %s, %.0f MB)\n", r.Name, fmtDuration(idle), mb)
		} else {
			fmt.Fprintf(&sb, "  %s\n", r.Name)
		}
	}
	return sb.String()
}

// summarizeKill formats the post-kill summary line.
// If freed is 0 (RSS was not measured) the "freed" clause is omitted.
func summarizeKill(killed []string, freed uint64) string {
	n := len(killed)
	if n == 0 {
		return "no sessions killed"
	}
	base := fmt.Sprintf("killed %d %s", n, pluralise("session", "sessions", n))
	if freed > 0 {
		mb := float64(freed) / (1024 * 1024)
		return fmt.Sprintf("%s, freed ~%.0f MB (estimated)", base, mb)
	}
	return base
}

// pluralise returns singular when count==1, otherwise plural.
func pluralise(singular, plural string, count int) string {
	if count == 1 {
		return singular
	}
	return plural
}

// fmtDuration formats a duration as a short human-readable string (e.g. "5h12m").
// Sub-minute durations are shown as seconds.
func fmtDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 && m > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh", h)
	}
	if m > 0 {
		return fmt.Sprintf("%dm", m)
	}
	// Less than a minute: show seconds of the original unrounded duration.
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
