// Package doctor implements the environment checks reported by `muxc doctor`.
// It lives in its own package so both the cli command and the post-v1.0 TUI
// can share the implementation without an import cycle.
//
// Spec §11.7 defines ten checks; each is an independently callable function
// (CheckXxx) so tests can exercise checks in isolation. Public package
// variables (TmuxVersioner, SessionLister) let tests swap in fakes that
// don't shell out to tmux.
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/state"
	"github.com/monzim/muxc/internal/sysinfo"
	"github.com/monzim/muxc/internal/tmux"
)

// CheckStatus represents the result level for a single doctor check.
type CheckStatus string

const (
	StatusOK   CheckStatus = "OK"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
)

// CheckResult holds the outcome of one check. JSON tags match spec §11.7.
type CheckResult struct {
	Name    string      `json:"name"`
	Status  CheckStatus `json:"status"`
	Message string      `json:"message,omitempty"`
}

// Versioner provides the tmux version string. Wrapped behind an interface so
// tests can avoid shelling out.
type Versioner interface {
	TmuxVersion(ctx context.Context) (string, error)
}

type realVersioner struct{}

func (realVersioner) TmuxVersion(ctx context.Context) (string, error) {
	return sysinfo.TmuxVersion(ctx)
}

// SessionLister lists live tmux sessions. Same indirection rationale as
// Versioner.
type SessionLister interface {
	ListSessions(ctx context.Context) ([]tmux.Session, error)
}

type realSessionLister struct{}

func (realSessionLister) ListSessions(ctx context.Context) ([]tmux.Session, error) {
	return tmux.ListSessions(ctx)
}

// Package-level vars swappable by tests.
var (
	TmuxVersioner  Versioner     = realVersioner{}
	SessionsLister SessionLister = realSessionLister{}
)

// Run executes all 10 checks (spec §11.7) and returns the slice of results.
// Pure orchestration; rendering and exit-code mapping belong to the caller.
func Run(ctx context.Context, cfg *config.Config, st *state.State, configDir string) []CheckResult {
	return []CheckResult{
		CheckTmuxVersion(ctx),
		CheckClaudeBinary(cfg.Defaults.ClaudeBin),
		CheckClaudeProjects(cfg.Paths.ClaudeProjects),
		CheckConfigDir(configDir),
		CheckConfigToml(configDir),
		CheckStateJSON(cfg.Paths.StateFile),
		CheckOrphanEntries(ctx, st),
		CheckFzf(),
		CheckProcfs(),
		CheckUID(),
	}
}

// HasFailure reports whether any check has StatusFail. WARN alone does NOT
// count (spec §11.7). Used by callers to determine exit code.
func HasFailure(checks []CheckResult) bool {
	for _, c := range checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}

// ── Individual checks ─────────────────────────────────────────────────────

func CheckTmuxVersion(ctx context.Context) CheckResult {
	ver, err := TmuxVersioner.TmuxVersion(ctx)
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
	return CheckResult{Name: "tmux installed (≥ 3.0)", Status: StatusOK, Message: ver}
}

func CheckClaudeBinary(claudeBin string) CheckResult {
	if _, err := exec.LookPath(claudeBin); err != nil {
		return CheckResult{
			Name:    "claude on PATH",
			Status:  StatusFail,
			Message: fmt.Sprintf("%s: executable not found in $PATH", claudeBin),
		}
	}
	return CheckResult{Name: "claude on PATH", Status: StatusOK}
}

func CheckClaudeProjects(projectsPath string) CheckResult {
	if _, err := os.Stat(projectsPath); err != nil {
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
	if _, err := os.ReadDir(projectsPath); err != nil {
		return CheckResult{
			Name:    "~/.claude/projects/ exists",
			Status:  StatusFail,
			Message: fmt.Sprintf("permission denied or unreadable: %s", err),
		}
	}
	return CheckResult{Name: "~/.claude/projects/ exists", Status: StatusOK}
}

func CheckConfigDir(configDir string) CheckResult {
	if _, err := os.Stat(configDir); err != nil {
		if os.IsNotExist(err) {
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
	testFile := filepath.Join(configDir, ".muxc-write-test")
	f, err := os.Create(testFile)
	if err != nil {
		return CheckResult{
			Name:    "~/.config/muxc/ writable",
			Status:  StatusFail,
			Message: fmt.Sprintf("not writable: %s", err),
		}
	}
	_ = f.Close()
	_ = os.Remove(testFile)
	return CheckResult{Name: "~/.config/muxc/ writable", Status: StatusOK}
}

func CheckConfigToml(configDir string) CheckResult {
	if _, err := config.Load(configDir, ""); err != nil {
		return CheckResult{
			Name:    "config.toml parses",
			Status:  StatusFail,
			Message: err.Error(),
		}
	}
	return CheckResult{Name: "config.toml parses", Status: StatusOK}
}

func CheckStateJSON(stateFile string) CheckResult {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{Name: "state.json parses", Status: StatusOK}
		}
		return CheckResult{
			Name:    "state.json parses",
			Status:  StatusFail,
			Message: fmt.Sprintf("cannot read: %s", err),
		}
	}
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
	return CheckResult{Name: "state.json parses", Status: StatusOK}
}

func CheckOrphanEntries(ctx context.Context, st *state.State) CheckResult {
	if len(st.Sessions) == 0 {
		return CheckResult{Name: "no orphan state entries", Status: StatusOK}
	}
	sessions, err := SessionsLister.ListSessions(ctx)
	if err != nil {
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
	return CheckResult{Name: "no orphan state entries", Status: StatusOK}
}

func CheckFzf() CheckResult {
	if _, err := exec.LookPath("fzf"); err != nil {
		return CheckResult{
			Name:    "fzf installed",
			Status:  StatusWarn,
			Message: "fzf not found; numbered prompt will be used instead",
		}
	}
	return CheckResult{Name: "fzf installed", Status: StatusOK}
}

func CheckProcfs() CheckResult {
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
	return CheckResult{Name: "/proc accessible", Status: StatusOK}
}

func CheckUID() CheckResult {
	if os.Getuid() == 0 {
		return CheckResult{
			Name:    "running as real user (UID > 0)",
			Status:  StatusWarn,
			Message: "running as root is unusual",
		}
	}
	return CheckResult{Name: "running as real user (UID > 0)", Status: StatusOK}
}
