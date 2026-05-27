// Package tmux wraps every tmux CLI invocation for muxc.
//
// All exec.Command("tmux", ...) calls in the codebase MUST go through this
// package. Callers never invoke tmux directly (CLAUDE.md architecture invariant).
//
// Error classification (spec §12):
//   - "no server running on …" exit → ErrNoServer (treat as zero sessions, exit 0)
//   - "session not found" on targeted op → ErrSessionNotFound (exit 2)
//   - other failures → wrapped original error (exit 1, propagate stderr)
//
// Test-only socket override:
// When the environment variable MUXC_TMUX_SOCKET is set, every tmux invocation
// prepends "-L <socket-name>" so integration tests can target a private server
// without touching the user's real tmux sessions. When the variable is unset
// the behaviour is identical to before this change.
package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ErrNoServer is returned when tmux is not running (no server on the socket).
var ErrNoServer = errors.New("tmux: no server running")

// ErrSessionNotFound is returned when a targeted session does not exist.
var ErrSessionNotFound = errors.New("tmux: session not found")

// Session holds the parsed output of one tmux list-sessions entry.
// Fields map to the format string documented in parse.go and spec §12.
type Session struct {
	Name     string
	Created  time.Time
	Activity time.Time
	Attached int
	ID       string
	Windows  int
}

// classifyError inspects stderr output from a failed tmux command and returns
// the appropriate sentinel error or a wrapped error with the stderr text.
// subcommand is the tmux subcommand name (e.g. "list-sessions") used in the
// fallback error message format.
//
// ErrNoServer patterns (spec §12: treat as zero sessions):
//   - "no server running on …"  — default socket not started
//   - "error connecting to …"   — named socket (via -L) not started
func classifyError(subcommand string, stderr []byte) error {
	lower := strings.ToLower(strings.TrimSpace(string(stderr)))
	if strings.Contains(lower, "no server running") ||
		strings.Contains(lower, "error connecting to") {
		return ErrNoServer
	}
	if strings.Contains(lower, "can't find session") ||
		strings.Contains(lower, "session not found") ||
		strings.Contains(lower, "no such session") {
		return ErrSessionNotFound
	}
	return fmt.Errorf("tmux %s: %s", subcommand, strings.TrimSpace(string(stderr)))
}

// tmuxArgs builds the complete argument list for a tmux invocation.
//
// When the MUXC_TMUX_SOCKET environment variable is set, the returned slice
// starts with ["-L", <socket>] so that every tmux call targets the private
// test server rather than the default one. When the variable is unset the
// slice begins directly with subcommand, preserving the original behaviour.
//
// This helper is the single point of change for socket selection; all other
// call sites (runTmux, Version, Attach) must use it.
func tmuxArgs(subcommand string, args ...string) []string {
	var result []string
	if sock := os.Getenv("MUXC_TMUX_SOCKET"); sock != "" {
		result = append(result, "-L", sock)
	}
	result = append(result, subcommand)
	result = append(result, args...)
	return result
}

// runTmux executes tmux with args, captures stdout, and returns (stdout, error).
// On failure the error is classified via classifyError using subcommand.
func runTmux(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
	allArgs := tmuxArgs(subcommand, args...)
	cmd := exec.CommandContext(ctx, "tmux", allArgs...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, classifyError(subcommand, stderr.Bytes())
	}
	return out, nil
}

// ListSessions returns all tmux sessions on the default server.
// Returns an empty slice (not an error) when ErrNoServer is detected, per spec §12.
//
// Format string used: see ParseSessions in parse.go / spec §12.
func ListSessions(ctx context.Context) ([]Session, error) {
	out, err := runTmux(ctx, "list-sessions", "-F", SessionFormat)
	if err != nil {
		if errors.Is(err, ErrNoServer) {
			// Spec §12: "no server running" → treat as empty list, not an error.
			return []Session{}, nil
		}
		return nil, err
	}
	return ParseSessions(strings.TrimRight(string(out), "\n"))
}

// ListPanePIDs returns the PID of each pane in session name.
// The first element is the pane PID used for process-tree walking (spec §13.1).
func ListPanePIDs(ctx context.Context, name string) ([]int, error) {
	out, err := runTmux(ctx, "list-panes", "-t", name, "-F", "#{pane_pid}")
	if err != nil {
		return nil, err
	}

	raw := strings.TrimRight(string(out), "\n")
	if raw == "" {
		return []int{}, nil
	}

	lines := strings.Split(raw, "\n")
	pids := make([]int, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("tmux list-panes: non-numeric pane_pid %q", line)
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// NewSession creates a new detached tmux session named name with its working
// directory set to cwd. Equivalent to: tmux new-session -d -s <name> -c <cwd>.
func NewSession(ctx context.Context, name, cwd string) error {
	_, err := runTmux(ctx, "new-session", "-d", "-s", name, "-c", cwd)
	return err
}

// SendKeys sends cmd followed by Enter to the first pane of session name.
// Equivalent to: tmux send-keys -t <name> '<cmd>' Enter.
// cmd and "Enter" are separate argv entries to send-keys per spec §12.
func SendKeys(ctx context.Context, name, cmd string) error {
	_, err := runTmux(ctx, "send-keys", "-t", name, cmd, "Enter")
	return err
}

// KillSession kills the tmux session name.
// Equivalent to: tmux kill-session -t <name>.
// Returns ErrSessionNotFound if the session does not exist.
func KillSession(ctx context.Context, name string) error {
	_, err := runTmux(ctx, "kill-session", "-t", name)
	return err
}

// HasSession reports whether a tmux session named name currently exists.
// Uses exit-code inspection of: tmux has-session -t <name>.
// Exit 0 → true, "can't find session" stderr → false (no error), other error → error.
func HasSession(ctx context.Context, name string) (bool, error) {
	_, err := runTmux(ctx, "has-session", "-t", name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrSessionNotFound) {
		return false, nil
	}
	return false, err
}

// Attach replaces the current process with tmux attach via syscall.Exec.
// If detach is true, passes -d so other clients are detached (spec §11.3).
//
// If $TMUX is set (we're already inside tmux), Attach uses `switch-client -t
// <name>` instead — nesting tmux refuses with "sessions should be nested with
// care, unset $TMUX to force". switch-client moves the existing client to
// the target session cleanly, which is what users actually want when running
// muxc from inside another tmux pane.
//
// This function never returns on success. Update state BEFORE calling it.
// Not suitable for unit tests — inject a fake via an interface in CLI commands.
func Attach(name string, detach bool) error {
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found in PATH: %w", err)
	}

	// Nested case: we're already inside tmux. Use switch-client so the
	// current client jumps to the target session without trying to nest.
	if os.Getenv("TMUX") != "" {
		fullArgs := tmuxArgs("switch-client", "-t", name)
		argv := append([]string{"tmux"}, fullArgs...)
		return syscall.Exec(tmuxBin, argv, syscall.Environ())
	}

	// Build args via tmuxArgs so the private socket is honoured when set.
	attachArgs := []string{"-t", name}
	if detach {
		attachArgs = append(attachArgs, "-d")
	}
	fullArgs := tmuxArgs("attach", attachArgs...)
	argv := append([]string{"tmux"}, fullArgs...)

	// syscall.Exec replaces the current process; does not return on success.
	return syscall.Exec(tmuxBin, argv, syscall.Environ())
}

// Version returns the tmux version string (e.g. "tmux 3.4").
// Equivalent to: tmux -V.
func Version(ctx context.Context) (string, error) {
	// Note: -V is a top-level flag, not a subcommand. tmuxArgs is called with
	// an empty subcommand so it only prepends the socket flags when set.
	allArgs := tmuxArgs("-V")
	cmd := exec.CommandContext(ctx, "tmux", allArgs...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return "", classifyError("-V", stderr.Bytes())
	}
	return strings.TrimSpace(string(out)), nil
}
