package tui

import (
	"time"

	"github.com/monzim/muxc/internal/session"
)

// gatherCompleteMsg carries the result of a background Gather call back to
// the sessions screen. err is non-nil when Gather failed; rows is the new
// slice (possibly nil) when err is nil.
type gatherCompleteMsg struct {
	rows []session.Row
	err  error
	when time.Time
}

// tickMsg fires every refreshInterval and triggers an auto-refresh. The
// sessions screen swallows or schedules the next tick depending on state.
type tickMsg time.Time

// errMsg surfaces a non-fatal error to the app for display in a banner.
type errMsg struct{ err error }

// attachRequestMsg signals the app to set its attachTarget and Quit.
// (Wave 4 dispatches this.)
type attachRequestMsg struct{ name string }
