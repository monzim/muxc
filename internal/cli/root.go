// Package cli implements the muxc command-line interface using cobra.
// Each subcommand has its own file; this file owns the root command,
// persistent flags, build-info injection, and the Execute entry point.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// buildInfo holds values injected at link time via cmd/muxc/main.go.
var buildInfo struct {
	version string
	commit  string
	date    string
}

// SetBuildInfo is called from cmd/muxc/main.go before Execute so that
// subcommands (version, doctor) can access the build metadata.
func SetBuildInfo(version, commit, date string) {
	buildInfo.version = version
	buildInfo.commit = commit
	buildInfo.date = date
}

// ExitError carries an explicit exit code for the error-to-exit-code mapping
// in cmd/muxc/main.go. Exit codes per spec §16:
//
//	0 — success
//	1 — general failure
//	2 — bad usage / target not found
//	3 — config error
//	4 — precondition failure (doctor would flag this)
//	130 — SIGINT
type ExitError struct {
	Code    int
	Message string
}

func (e *ExitError) Error() string { return e.Message }

// exitErr is a helper that wraps a message and exit code.
func exitErr(code int, msg string) *ExitError {
	return &ExitError{Code: code, Message: msg}
}

// exitErrf formats a message and wraps it with the given exit code.
func exitErrf(code int, format string, args ...any) *ExitError {
	return &ExitError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// rootCmd is the base cobra command for the muxc binary.
var rootCmd = &cobra.Command{
	Use:   "muxc",
	Short: "Manage Claude Code sessions inside tmux",
	Long: `muxc is a thin CLI layered over tmux and Claude Code's session persistence.

It lets you list, create, attach to, and kill Claude Code sessions running
inside tmux, with memory and idle-time information sourced directly from
/proc and the Claude project directory (~/.claude/projects/).

tmux is the source of truth for liveness. muxc never writes to ~/.claude/.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command and returns any error. Called from main.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Persistent flags available to all subcommands.
	rootCmd.PersistentFlags().Bool("json", false, "emit JSON output instead of a table")
	rootCmd.PersistentFlags().String("config", "", "path to config directory (default ~/.config/muxc)")

	// Register all subcommands.
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(newCmd)
	rootCmd.AddCommand(attachCmd)
	rootCmd.AddCommand(killCmd)
	rootCmd.AddCommand(memCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(versionCmd)
}
