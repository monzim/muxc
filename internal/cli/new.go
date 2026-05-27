// Package cli — new subcommand.
// See spec §11.2 for the full behavioral specification.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/state"
	"github.com/monzim/muxc/internal/tmux"
	"github.com/spf13/cobra"
)

// newCmd creates a new tmux session and optionally launches Claude Code inside it.
var newCmd = &cobra.Command{
	Use:   "new <project-path>",
	Short: "Create a new Claude Code session in a tmux window",
	Long: `Create a new tmux session rooted at <project-path> and launch Claude Code inside it.

The tmux session name is derived from the basename of the project path,
prefixed with the configured prefix (default: muxc-). Use --name to override.`,
	Args: cobra.ExactArgs(1),
	RunE: runNew,
}

func init() {
	newCmd.Flags().String("name", "", "override derived tmux session name (without prefix)")
	newCmd.Flags().Bool("no-claude-name", false, "do not pass -n to claude; let Claude assign a UUID")
	newCmd.Flags().String("args", "", "extra args appended to the claude launch command for this session only")
	newCmd.Flags().Bool("no-launch", false, "create the tmux session and cd but do not start Claude")
}

// runNew is the RunE implementation for newCmd. Separated for testability.
func runNew(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// ── Step 1: resolve project path ─────────────────────────────────────────
	rawPath := args[0]
	absPath, err := filepath.Abs(PathExpand(rawPath))
	if err != nil {
		return &ExitError{
			Code: 2,
			Message: fmt.Sprintf(
				"muxc: cannot resolve project path: %v\n  pass an existing directory, e.g. `muxc new ~/code/my145`",
				err,
			),
		}
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return &ExitError{
			Code: 2,
			Message: fmt.Sprintf(
				"muxc: cannot resolve project path: %v\n  pass an existing directory, e.g. `muxc new ~/code/my145`",
				err,
			),
		}
	}
	if !info.IsDir() {
		return &ExitError{
			Code: 2,
			Message: fmt.Sprintf(
				"muxc: cannot resolve project path: %q is not a directory\n  pass an existing directory, e.g. `muxc new ~/code/my145`",
				absPath,
			),
		}
	}

	// ── Step 2: load base context (config + state) ────────────────────────────
	// LoadContext loads config without the project overlay (empty project path).
	// We load state from it, then re-load config with the project overlay below.
	cfgDir := ConfigDir(cmd)
	_, st, err := LoadContext(cmd)
	if err != nil {
		return err
	}

	// ── Step 3: re-load config with project overlay (spec §11.2 step 3) ──────
	// Only launch_args, name_sessions, claude_bin may be overridden per-project.
	cfg, err := config.Load(cfgDir, absPath)
	if err != nil {
		return err
	}

	// ── Step 4: derive tmux session name ─────────────────────────────────────
	nameFlag, _ := cmd.Flags().GetString("name")
	var baseName string
	if nameFlag != "" {
		baseName = sanitizeName(nameFlag)
	} else {
		baseName = sanitizeName(filepath.Base(absPath))
	}

	// Prepend the configured prefix.
	sessionName := cfg.Defaults.Prefix + baseName

	// Dedup against live tmux sessions.
	liveSessions, err := tmux.ListSessions(ctx)
	if err != nil && !errors.Is(err, tmux.ErrNoServer) {
		return fmt.Errorf("muxc: list tmux sessions: %w", err)
	}
	existing := make([]string, 0, len(liveSessions))
	for _, s := range liveSessions {
		existing = append(existing, s.Name)
	}
	sessionName, err = dedupName(sessionName, existing)
	if err != nil {
		return &ExitError{Code: 1, Message: fmt.Sprintf("muxc: %v", err)}
	}

	// ── Step 5: build launch command ─────────────────────────────────────────
	noClaudeName, _ := cmd.Flags().GetBool("no-claude-name")
	extraArgsStr, _ := cmd.Flags().GetString("args")
	var extraArgs []string
	if extraArgsStr != "" {
		extraArgs = strings.Fields(extraArgsStr)
	}

	// The claude-name is the session name minus the prefix (spec §11.2 step 3).
	claudeName := strings.TrimPrefix(sessionName, cfg.Defaults.Prefix)
	includeClaudeName := cfg.Defaults.NameSessions && !noClaudeName

	launchArgs := buildLaunchCmd(cfg, claudeName, extraArgs, includeClaudeName)

	// ── Step 6: create tmux session ───────────────────────────────────────────
	if err := tmux.NewSession(ctx, sessionName, absPath); err != nil {
		return &ExitError{Code: 1, Message: fmt.Sprintf("muxc: create tmux session: %v", err)}
	}

	// ── Step 7: send launch command unless --no-launch ────────────────────────
	noLaunch, _ := cmd.Flags().GetBool("no-launch")
	if !noLaunch {
		// Build the shell command string. Each arg is shell-escaped.
		escapedParts := make([]string, len(launchArgs))
		for i, a := range launchArgs {
			escapedParts[i] = shellEscape(a)
		}
		fullCmd := strings.Join(escapedParts, " ")
		if err := tmux.SendKeys(ctx, sessionName, fullCmd); err != nil {
			// Session was created; log but don't abort.
			fmt.Fprintf(cmd.ErrOrStderr(), "muxc: send-keys failed: %v\n", err)
		}
	}

	// ── Step 8: write to state ────────────────────────────────────────────────
	storedClaudeName := ""
	if includeClaudeName {
		storedClaudeName = claudeName
	}

	// Effective launch args stored in state = base args + extra args (no -n flag —
	// that is tracked separately via ClaudeSessionName). Store the actual invocation
	// args except the -n portion to match spec §10 shape.
	effectiveArgs := buildLaunchCmd(cfg, claudeName, extraArgs, false)

	st.Upsert(sessionName, state.SessionEntry{
		ProjectPath:       absPath,
		ClaudeSessionName: storedClaudeName,
		LaunchArgs:        effectiveArgs,
		CreatedAt:         time.Now().UTC(),
	})
	if err := st.Write(cfg.Paths.StateFile); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "muxc: write state: %v\n", err)
	}

	// ── Step 9: print success ─────────────────────────────────────────────────
	// "attach with" shows the name without prefix.
	fmt.Fprintf(cmd.OutOrStdout(), "started %s in %s\n", sessionName, absPath)
	fmt.Fprintf(cmd.OutOrStdout(), "attach with: muxc attach %s\n", claudeName)

	return nil
}

// ── Pure helper functions (exported for testing) ──────────────────────────────

// sanitizeName converts a raw string into a valid tmux session name component.
//
// Rules (spec §11.2 step 2):
//  1. Lowercase the input.
//  2. Replace any character not in [A-Za-z0-9_-] with a hyphen.
//  3. Collapse consecutive hyphens into a single hyphen.
//  4. Trim leading and trailing hyphens.
//
// The result may be empty if all characters were stripped (e.g. "....").
func sanitizeName(s string) string {
	s = strings.ToLower(s)

	// Replace disallowed chars.
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			sb.WriteRune(r)
		} else {
			sb.WriteByte('-')
		}
	}
	s = sb.String()

	// Collapse consecutive hyphens.
	s = collapseHyphens.ReplaceAllString(s, "-")

	// Trim leading/trailing hyphens.
	s = strings.Trim(s, "-")

	return s
}

// collapseHyphens matches two or more consecutive hyphens.
var collapseHyphens = regexp.MustCompile(`-{2,}`)

// dedupName ensures base is unique relative to existing session names.
//
// If base is not in existing it is returned as-is. Otherwise the function
// tries base-2, base-3, … up to base-10. Returns an error if all suffixes
// collide (spec §11.2 step 2).
func dedupName(base string, existing []string) (string, error) {
	liveSet := make(map[string]struct{}, len(existing))
	for _, n := range existing {
		liveSet[n] = struct{}{}
	}

	if _, collision := liveSet[base]; !collision {
		return base, nil
	}

	for i := 2; i <= 10; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, collision := liveSet[candidate]; !collision {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("session name %q already exists (tried suffixes -2 through -10); rename with --name", base)
}

// buildLaunchCmd constructs the ordered list of arguments that form the Claude
// launch command for a session.
//
// The result is:
//
//	[claude_bin] [cfg.Defaults.LaunchArgs...] [extraArgs...] [-n claudeName (if includeClaudeName)]
//
// This slice is suitable for shell-escaping and joining into a send-keys string.
func buildLaunchCmd(cfg *config.Config, claudeName string, extraArgs []string, includeClaudeName bool) []string {
	args := make([]string, 0, 4+len(cfg.Defaults.LaunchArgs)+len(extraArgs))
	args = append(args, cfg.Defaults.ClaudeBin)
	args = append(args, cfg.Defaults.LaunchArgs...)
	args = append(args, extraArgs...)
	if includeClaudeName && claudeName != "" {
		args = append(args, "-n", claudeName)
	}
	return args
}

// shellEscape returns s in a form safe for use as a shell word.
//
// Safe characters (no quoting needed): [A-Za-z0-9_./-]
// Everything else is single-quoted; internal single quotes are handled with
// the classic '\” trick.
//
// §25 open Q3 resolution: bufio approach, simplest shell-escape possible.
func shellEscape(s string) string {
	if isSafeShellWord(s) {
		return s
	}
	// Wrap in single quotes; replace any embedded single-quote with '\''
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// safeShellChars matches a string that needs no shell quoting.
var safeShellChars = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

func isSafeShellWord(s string) bool {
	return s != "" && safeShellChars.MatchString(s)
}
