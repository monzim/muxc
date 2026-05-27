// Package tui implements the Bubble Tea terminal UI for muxc.
//
// This is a deliberate deviation from spec §3 (which rejected "TUI dashboard").
// See CLAUDE.md "Post-v1.0 additions" for the rationale. The package is
// kept self-contained under internal/tui so the non-TTY plain REPL in
// internal/cli/interactive.go remains the fallback for CI, piped input, and
// dumb terminals.
package tui

import (
	"context"
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/state"
	"github.com/monzim/muxc/internal/tmux"
)

// ErrNoTTY indicates muxc was invoked in a non-interactive context. The CLI
// layer treats this as a signal to fall back to the plain-text REPL.
var ErrNoTTY = errors.New("tui: no interactive terminal detected")

// Run launches the Bubble Tea program against the user's terminal. It returns
// ErrNoTTY when stdin or stdout is not a terminal so the caller can choose a
// fallback. Any other non-nil error indicates a real failure to render.
//
// If the user selects "attach" inside the TUI, Run returns nil AFTER calling
// tmux.Attach (which uses syscall.Exec). In that path control never returns —
// the muxc process is replaced by tmux.
func Run(cfg *config.Config, st *state.State) error {
	// Defensive TTY check — the cli layer should have screened already, but
	// we don't want to corrupt a non-terminal stream if it didn't.
	if !isatty.IsTerminal(os.Stdin.Fd()) || !isatty.IsTerminal(os.Stdout.Fd()) {
		return ErrNoTTY
	}

	app := NewApp(cfg, st)

	prog := tea.NewProgram(
		app,
		tea.WithAltScreen(),
		tea.WithContext(context.Background()),
	)

	finalModel, err := prog.Run()
	if err != nil {
		return fmt.Errorf("tui: program: %w", err)
	}

	// Post-quit handoff: if the user picked attach, the App stored the target
	// session name. tmux.Attach uses syscall.Exec — terminal is already
	// restored by Bubble Tea on Quit.
	if final, ok := finalModel.(App); ok {
		if target := final.AttachTarget(); target != "" {
			return tmux.Attach(target, true)
		}
	}
	return nil
}
