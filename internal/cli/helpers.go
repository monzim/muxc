// Package cli — shared helper functions used by multiple subcommands.
//
// LoadContext, IsJSON, ResolveSessionName, PathExpand, and ConfigDir are the
// "first line" helpers that every command calls before doing real work.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/state"
	"github.com/monzim/muxc/internal/tmux"
	"github.com/spf13/cobra"
)

// ErrNotFound is the sentinel returned by ResolveSessionName when neither the
// bare name nor the prefixed name matches any live tmux session.
var ErrNotFound = errors.New("session not found")

// LoadContext is the first call every CLI command makes.
//
// It loads config from the directory resolved via --config flag or the
// MUXC_CONFIG_DIR env var (falling back to ~/.config/muxc/), then reads
// state.json from the path in the loaded config, and returns both.
func LoadContext(cmd *cobra.Command) (*config.Config, *state.State, error) {
	dir := ConfigDir(cmd)
	cfg, err := config.Load(dir, "")
	if err != nil {
		return nil, nil, fmt.Errorf("muxc: load config: %w", err)
	}
	st, err := state.Read(cfg.Paths.StateFile)
	if err != nil {
		return nil, nil, fmt.Errorf("muxc: read state: %w", err)
	}
	return cfg, st, nil
}

// IsJSON returns true if the --json flag is set on cmd or any of its ancestors.
// The flag is defined as a persistent flag on the root command (root.go).
func IsJSON(cmd *cobra.Command) bool {
	// Walk up the command chain so this works even when called from a subcommand.
	for c := cmd; c != nil; c = c.Parent() {
		f := c.Flags().Lookup("json")
		if f == nil {
			f = c.PersistentFlags().Lookup("json")
		}
		if f != nil && f.Value.String() == "true" {
			return true
		}
	}
	return false
}

// sessionChecker is a small interface that wraps tmux.HasSession so tests can
// substitute a fake without calling the real tmux binary.
//
// The real implementation is provided by tmuxSessionChecker below.
type sessionChecker interface {
	HasSession(ctx context.Context, name string) (bool, error)
}

// tmuxSessionChecker is the production implementation of sessionChecker.
type tmuxSessionChecker struct{}

func (tmuxSessionChecker) HasSession(ctx context.Context, name string) (bool, error) {
	return tmux.HasSession(ctx, name)
}

// defaultChecker is the checker used by ResolveSessionName in production code.
// Tests replace resolveChecker to avoid spawning real tmux processes.
var defaultChecker sessionChecker = tmuxSessionChecker{}

// ResolveSessionName tries to find a live tmux session matching the user-supplied
// name. It attempts, in order:
//  1. name as-is (the user may have typed the full tmux name).
//  2. cfg.Defaults.Prefix + name (the common shorthand).
//
// Returns the full tmux session name on success, or ErrNotFound if neither
// variant exists.
func ResolveSessionName(ctx context.Context, cfg *config.Config, name string) (string, error) {
	return resolveSessionNameWith(ctx, cfg, name, defaultChecker)
}

// resolveSessionNameWith is the testable implementation of ResolveSessionName.
func resolveSessionNameWith(ctx context.Context, cfg *config.Config, name string, checker sessionChecker) (string, error) {
	// Try name verbatim first.
	ok, err := checker.HasSession(ctx, name)
	if err != nil {
		return "", fmt.Errorf("muxc: check session %q: %w", name, err)
	}
	if ok {
		return name, nil
	}

	// Try with the configured prefix.
	prefixed := cfg.Defaults.Prefix + name
	if prefixed == name {
		// Prefix is empty or name already starts with it and we've already checked.
		return "", fmt.Errorf("muxc: tmux session %q not found\n  list available sessions with `muxc ls`", name)
	}

	ok, err = checker.HasSession(ctx, prefixed)
	if err != nil {
		return "", fmt.Errorf("muxc: check session %q: %w", prefixed, err)
	}
	if ok {
		return prefixed, nil
	}

	return "", fmt.Errorf("%w: %q\n  list available sessions with `muxc ls`", ErrNotFound, name)
}

// PathExpand expands a leading "~" to the current user's home directory.
// Other paths are returned unchanged. Errors expanding home are returned as-is.
func PathExpand(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}

// ConfigDir resolves the configuration directory to use, in priority order:
//  1. The --config flag value on cmd (or any ancestor).
//  2. The MUXC_CONFIG_DIR environment variable.
//  3. The empty string (config.Load will default to ~/.config/muxc/).
func ConfigDir(cmd *cobra.Command) string {
	// Walk command and its parents looking for --config.
	for c := cmd; c != nil; c = c.Parent() {
		f := c.Flags().Lookup("config")
		if f == nil {
			f = c.PersistentFlags().Lookup("config")
		}
		if f != nil && f.Value.String() != "" {
			return f.Value.String()
		}
	}
	// Fall back to environment variable.
	if v := os.Getenv("MUXC_CONFIG_DIR"); v != "" {
		return v
	}
	return ""
}
