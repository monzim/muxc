// Package tui implements the Bubble Tea terminal UI for muxc.
//
// This is a deliberate deviation from spec §3 (which rejected "TUI dashboard").
// See CLAUDE.md "Post-v1.0 additions" for the rationale. The package is
// kept self-contained under internal/tui so the non-TTY plain REPL in
// internal/cli/interactive.go remains the fallback for CI, piped input, and
// dumb terminals.
package tui

import (
	"errors"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/state"
)

// ErrNoTTY indicates muxc was invoked in a non-interactive context. The CLI
// layer treats this as a signal to fall back to the plain-text REPL.
var ErrNoTTY = errors.New("tui: no interactive terminal detected")

// Run launches the Bubble Tea program against the user's terminal. It returns
// ErrNoTTY when stdin or stdout is not a terminal so the caller can choose a
// fallback. Any other non-nil error indicates a real failure to render.
//
// On a successful attach picked from the TUI, Run returns nil AFTER the
// terminal has been restored and tmux has taken over via syscall.Exec — so in
// practice control never returns from such a Run() call.
func Run(cfg *config.Config, st *state.State) error {
	// Wave 1: skeleton only. Real implementation arrives in Wave 2+.
	return errors.New("tui: not implemented")
}
