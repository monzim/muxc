package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/session"
	"github.com/monzim/muxc/internal/state"
)

// refreshInterval is the cadence for the sessions screen's auto-refresh.
// Hard-coded to match the spec's `watch -n 2 muxc ls` semantics; configurable
// surface can be added later if anyone asks.
const refreshInterval = 2 * time.Second

// gatherCmd produces a tea.Cmd that runs cli.Gather in the Bubble Tea worker
// goroutine and returns a gatherCompleteMsg. The mode is fixed to
// ExternalWithClaude — the TUI mirrors `muxc ls` defaults so external Claude
// sessions are surfaced. Callers can sort or filter the returned rows
// afterwards.
func gatherCmd(cfg *config.Config, st *state.State) tea.Cmd {
	return func() tea.Msg {
		// Use Background context — tea controls the goroutine lifecycle.
		ctx := context.Background()
		rows, err := session.Gather(ctx, cfg, st, session.ExternalWithClaude)
		return gatherCompleteMsg{
			rows: rows,
			err:  err,
			when: time.Now(),
		}
	}
}

// tickCmd schedules a tick after refreshInterval. The sessions screen's
// Update handler issues both a gatherCmd and another tickCmd whenever it
// receives a tickMsg, so the loop continues until the program quits.
func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
