// Package cli — doctor subcommand.
// See spec §11.7 for the full behavioral specification.
//
// Each check is an independently callable function (checkXxx) so tests can
// exercise individual checks in isolation without running the full command.
// The package-level tmuxVersioner and sessionLister vars allow tests to inject
// fakes for checks that would otherwise shell out to tmux.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/fatih/color"
	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/state"
	"github.com/monzim/muxc/internal/sysinfo"
	"github.com/monzim/muxc/internal/tmux"
	"github.com/spf13/cobra"
)

// CheckStatus represents the result level for a single doctor check.
// Spec §11.7: each check is OK, WARN, or FAIL.
type CheckStatus string

const (
	StatusOK   CheckStatus = "OK"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
)

// CheckResult holds the outcome of one doctor check.
// JSON tags match spec §11.7 JSON output schema.
type CheckResult struct {
	Name    string      `json:"name"`
	Status  CheckStatus `json:"status"`
	Message string      `json:"message,omitempty"`
}

// doctorVersioner is the interface that check #1 uses to get the tmux version.
// Tests replace this with a fake to avoid calling tmux.
type doctorVersioner interface {
	TmuxVersion(ctx context.Context) (string, error)
}

type realVersioner struct{}

func (realVersioner) TmuxVersion(ctx context.Context) (string, error) {
	return sysinfo.TmuxVersion(ctx)
}

// doctorSessionLister is the interface that check #7 (orphan entries) uses to
// list live sessions. Tests inject a fake.
type doctorSessionLister interface {
	ListSessions(ctx context.Context) ([]tmux.Session, error)
}

type realSessionLister struct{}

func (realSessionLister) ListSessions(ctx context.Context) ([]tmux.Session, error) {
	return tmux.ListSessions(ctx)
}

// Package-level vars swapped by tests.
var (
	doctorTmuxVersioner  doctorVersioner     = realVersioner{}
	doctorTmuxSessLister doctorSessionLister = realSessionLister{}
)

// doctorCmd runs a set of environment checks and reports OK/WARN/FAIL per check.
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check environment prerequisites",
	Long: `Run a set of environment checks and report OK, WARN, or FAIL for each.

Exit code is 0 when all checks are OK or WARN; 1 when any check FAILs.`,
	RunE: runDoctor,
}

func runDoctor(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, st, err := LoadContext(cmd)
	if err != nil {
		// Even if config fails to load, we can still run most checks.
		// Use a bare default config for the checks that need it.
		cfg = config.DefaultConfig()
		st = &state.State{Version: 1, Sessions: map[string]state.SessionEntry{}}
	}

	configDir := ConfigDir(cmd)
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = home + "/.config/muxc"
	}

	checks := []CheckResult{
		checkTmuxVersion(ctx),
		checkClaudeBinary(cfg.Defaults.ClaudeBin),
		checkClaudeProjects(cfg.Paths.ClaudeProjects),
		checkConfigDir(configDir),
		checkConfigToml(configDir),
		checkStateJSON(cfg.Paths.StateFile),
		checkOrphanEntries(ctx, st),
		checkFzf(),
		checkProcfs(),
		checkUID(),
	}

	if IsJSON(cmd) {
		data, err := json.MarshalIndent(checks, "", "  ")
		if err != nil {
			return exitErr(1, fmt.Sprintf("muxc: marshal checks: %s", err))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", data)
	} else {
		useColor := shouldColor(cfg)
		renderCheckText(cmd.OutOrStdout(), checks, useColor)
	}

	if hasFailure(checks) {
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
		// Respect NO_COLOR and MUXC_NO_COLOR env vars (fatih/color handles NO_COLOR).
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

// ── Individual checks ──────────────────────────────────────────────────────

// checkTmuxVersion checks that tmux is installed and meets the minimum version.
// Spec §11.7 check #1.
func checkTmuxVersion(ctx context.Context) CheckResult {
	ver, err := doctorTmuxVersioner.TmuxVersion(ctx)
	if err != nil {
		return CheckResult{
			Name:    "tmux installed (≥ 3.0)",
			Status:  StatusFail,
			Message: fmt.Sprintf("tmux not found or failed: %s", err),
		}
	}
	ok, err := sysinfo.TmuxMeetsMin(ver, "3.0")
	if err != nil {
		return CheckResult{
			Name:    "tmux installed (≥ 3.0)",
			Status:  StatusFail,
			Message: fmt.Sprintf("cannot parse version %q: %s", ver, err),
		}
	}
	if !ok {
		return CheckResult{
			Name:    "tmux installed (≥ 3.0)",
			Status:  StatusFail,
			Message: fmt.Sprintf("tmux %s < 3.0", ver),
		}
	}
	return CheckResult{
		Name:    "tmux installed (≥ 3.0)",
		Status:  StatusOK,
		Message: ver,
	}
}

// checkClaudeBinary checks that the configured claude binary is on PATH.
// Spec §11.7 check #2.
func checkClaudeBinary(claudeBin string) CheckResult {
	_, err := exec.LookPath(claudeBin)
	if err != nil {
		return CheckResult{
			Name:    "claude on PATH",
			Status:  StatusFail,
			Message: fmt.Sprintf("%s: executable not found in $PATH", claudeBin),
		}
	}
	return CheckResult{
		Name:   "claude on PATH",
		Status: StatusOK,
	}
}

// checkClaudeProjects checks that ~/.claude/projects/ exists and is readable.
// Missing → WARN (user may not have used Claude Code yet). Permission denied → FAIL.
// Spec §11.7 check #3.
func checkClaudeProjects(projectsPath string) CheckResult {
	_, err := os.Stat(projectsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:    "~/.claude/projects/ exists",
				Status:  StatusWarn,
				Message: "directory not found; create a Claude Code session first",
			}
		}
		return CheckResult{
			Name:    "~/.claude/projects/ exists",
			Status:  StatusFail,
			Message: fmt.Sprintf("cannot stat: %s", err),
		}
	}
	// Try ReadDir to confirm readable.
	_, err = os.ReadDir(projectsPath)
	if err != nil {
		return CheckResult{
			Name:    "~/.claude/projects/ exists",
			Status:  StatusFail,
			Message: fmt.Sprintf("permission denied or unreadable: %s", err),
		}
	}
	return CheckResult{
		Name:   "~/.claude/projects/ exists",
		Status: StatusOK,
	}
}

// checkConfigDir checks that the muxc config directory exists and is writable.
// If missing, attempts to create it. Spec §11.7 check #4.
func checkConfigDir(configDir string) CheckResult {
	_, err := os.Stat(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Attempt to create.
			if mkErr := os.MkdirAll(configDir, 0o755); mkErr != nil {
				return CheckResult{
					Name:    "~/.config/muxc/ writable",
					Status:  StatusFail,
					Message: fmt.Sprintf("cannot create config dir: %s", mkErr),
				}
			}
		} else {
			return CheckResult{
				Name:    "~/.config/muxc/ writable",
				Status:  StatusFail,
				Message: fmt.Sprintf("cannot stat config dir: %s", err),
			}
		}
	}

	// Verify writability by creating and removing a test file.
	testFile := configDir + "/.muxc-write-test"
	f, err := os.Create(testFile)
	if err != nil {
		return CheckResult{
			Name:    "~/.config/muxc/ writable",
			Status:  StatusFail,
			Message: fmt.Sprintf("not writable: %s", err),
		}
	}
	f.Close()
	_ = os.Remove(testFile)

	return CheckResult{
		Name:   "~/.config/muxc/ writable",
		Status: StatusOK,
	}
}

// checkConfigToml checks that config.toml parses (or is absent).
// Spec §11.7 check #5.
func checkConfigToml(configDir string) CheckResult {
	_, err := config.Load(configDir, "")
	if err != nil {
		return CheckResult{
			Name:    "config.toml parses",
			Status:  StatusFail,
			Message: err.Error(),
		}
	}
	return CheckResult{
		Name:   "config.toml parses",
		Status: StatusOK,
	}
}

// checkStateJSON checks that state.json parses (or is absent).
// state.Read silently swallows corruption and returns empty state, so we need
// to do manual JSON decode to detect corruption. Spec §11.7 check #6.
func checkStateJSON(stateFile string) CheckResult {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Absent is fine.
			return CheckResult{
				Name:   "state.json parses",
				Status: StatusOK,
			}
		}
		return CheckResult{
			Name:    "state.json parses",
			Status:  StatusFail,
			Message: fmt.Sprintf("cannot read: %s", err),
		}
	}

	// File exists and has content — validate JSON.
	if len(data) > 0 {
		var s state.State
		if err := json.Unmarshal(data, &s); err != nil {
			return CheckResult{
				Name:    "state.json parses",
				Status:  StatusFail,
				Message: fmt.Sprintf("malformed JSON: %s", err),
			}
		}
	}

	return CheckResult{
		Name:   "state.json parses",
		Status: StatusOK,
	}
}

// checkOrphanEntries checks for state entries with no matching live tmux session.
// Spec §11.7 check #7.
func checkOrphanEntries(ctx context.Context, st *state.State) CheckResult {
	if len(st.Sessions) == 0 {
		return CheckResult{
			Name:   "no orphan state entries",
			Status: StatusOK,
		}
	}

	sessions, err := doctorTmuxSessLister.ListSessions(ctx)
	if err != nil {
		// If tmux is not running treat all state entries as orphans is wrong —
		// we simply can't know. Report OK to avoid spurious WARNs.
		return CheckResult{
			Name:    "no orphan state entries",
			Status:  StatusWarn,
			Message: fmt.Sprintf("cannot list tmux sessions: %s", err),
		}
	}

	live := make(map[string]struct{}, len(sessions))
	for _, s := range sessions {
		live[s.Name] = struct{}{}
	}

	orphans := 0
	for name := range st.Sessions {
		if _, ok := live[name]; !ok {
			orphans++
		}
	}

	if orphans > 0 {
		return CheckResult{
			Name:    "no orphan state entries",
			Status:  StatusWarn,
			Message: fmt.Sprintf("%d orphan entries — clean with `muxc kill --stale`", orphans),
		}
	}
	return CheckResult{
		Name:   "no orphan state entries",
		Status: StatusOK,
	}
}

// checkFzf checks whether fzf is available. Informational only — WARN if absent.
// Spec §11.7 check #8.
func checkFzf() CheckResult {
	_, err := exec.LookPath("fzf")
	if err != nil {
		return CheckResult{
			Name:    "fzf installed",
			Status:  StatusWarn,
			Message: "fzf not found; numbered prompt will be used instead",
		}
	}
	return CheckResult{
		Name:   "fzf installed",
		Status: StatusOK,
	}
}

// checkProcfs checks whether /proc is accessible.
// Non-Linux → WARN. Linux without /proc → FAIL. Spec §11.7 check #9.
func checkProcfs() CheckResult {
	if !sysinfo.OnLinux() {
		return CheckResult{
			Name:    "/proc accessible",
			Status:  StatusWarn,
			Message: "only Linux is fully supported; memory readings unavailable",
		}
	}
	if !sysinfo.ProcfsAvailable() {
		return CheckResult{
			Name:    "/proc accessible",
			Status:  StatusFail,
			Message: "/proc is not accessible; memory readings will not work",
		}
	}
	return CheckResult{
		Name:   "/proc accessible",
		Status: StatusOK,
	}
}

// checkUID checks that muxc is not running as root.
// UID 0 → WARN. Spec §11.7 check #10.
func checkUID() CheckResult {
	if os.Getuid() == 0 {
		return CheckResult{
			Name:    "running as real user (UID > 0)",
			Status:  StatusWarn,
			Message: "running as root is unusual",
		}
	}
	return CheckResult{
		Name:   "running as real user (UID > 0)",
		Status: StatusOK,
	}
}

// ── Rendering ─────────────────────────────────────────────────────────────

// renderCheckText writes one line per check to w using the format:
//
//	[OK]   <name>: <message>
//	[WARN] <name>: <message>
//	[FAIL] <name>: <message>
//
// Color is applied when color is true: OK → green, WARN → yellow, FAIL → red.
func renderCheckText(w io.Writer, checks []CheckResult, useColor bool) {
	okStr := "[OK]  "
	warnStr := "[WARN]"
	failStr := "[FAIL]"

	if useColor {
		// fatih/color: disable its global color-detection if we're asked to use color.
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

// hasFailure returns true if any check in the slice has StatusFail.
// WARN alone does not constitute failure per spec §11.7.
func hasFailure(checks []CheckResult) bool {
	for _, c := range checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}
