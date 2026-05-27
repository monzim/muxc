// Package cli — doctor subcommand.
// See spec §11.7 for the full behavioral specification.
//
// The check functions live in internal/doctor so the post-v1.0 TUI can call
// them without an import cycle. cli/doctor.go keeps just the cobra command
// surface, text rendering, and exit-code mapping.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/doctor"
)

// Aliases so existing call sites and tests in this package keep their
// original names.
type CheckStatus = doctor.CheckStatus
type CheckResult = doctor.CheckResult

const (
	StatusOK   = doctor.StatusOK
	StatusWarn = doctor.StatusWarn
	StatusFail = doctor.StatusFail
)

// doctorCmd runs the environment checks and reports OK/WARN/FAIL per check.
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check environment prerequisites",
	Long: `Run a set of environment checks and report OK, WARN, or FAIL for each.

Exit code is 0 when all checks are OK or WARN; 1 when any check FAILs.`,
	RunE: runDoctor,
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	cfg, st, err := LoadContext(cmd)
	if err != nil {
		return err
	}
	configDir := ConfigDir(cmd)

	checks := doctor.Run(ctx, cfg, st, configDir)

	if IsJSON(cmd) {
		data, err := json.MarshalIndent(checks, "", "  ")
		if err != nil {
			return exitErr(1, fmt.Sprintf("muxc: marshal checks: %s", err))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", data)
	} else {
		renderCheckText(cmd.OutOrStdout(), checks, shouldColor(cfg))
	}

	if doctor.HasFailure(checks) {
		return exitErr(1, "")
	}
	return nil
}

// shouldColor reports whether to emit ANSI color based on display.color config
// and whether stdout is a TTY.
func shouldColor(cfg *config.Config) bool {
	switch cfg.Display.Color {
	case "always":
		return true
	case "never":
		return false
	default: // "auto"
		if os.Getenv("MUXC_NO_COLOR") != "" {
			return false
		}
		fi, err := os.Stdout.Stat()
		if err != nil {
			return false
		}
		return (fi.Mode() & os.ModeCharDevice) != 0
	}
}

// renderCheckText writes one line per check to w. Color is applied when
// useColor is true: OK → green, WARN → yellow, FAIL → red.
func renderCheckText(w io.Writer, checks []CheckResult, useColor bool) {
	okStr := "[OK]  "
	warnStr := "[WARN]"
	failStr := "[FAIL]"

	if useColor {
		color.NoColor = false
		okStr = color.GreenString("[OK]  ")
		warnStr = color.YellowString("[WARN]")
		failStr = color.RedString("[FAIL]")
	}

	for _, c := range checks {
		var badge string
		switch c.Status {
		case StatusOK:
			badge = okStr
		case StatusWarn:
			badge = warnStr
		case StatusFail:
			badge = failStr
		default:
			badge = "[????]"
		}
		if c.Message != "" {
			fmt.Fprintf(w, "%s %s: %s\n", badge, c.Name, c.Message)
		} else {
			fmt.Fprintf(w, "%s %s\n", badge, c.Name)
		}
	}
}

// hasFailure delegates to doctor.HasFailure — kept as a local alias since
// existing tests in this package reference the unexported name.
func hasFailure(checks []CheckResult) bool { return doctor.HasFailure(checks) }
