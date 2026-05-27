// Package cli — type/function aliases that re-export the shared session
// package. The original implementation moved to internal/session/ to break
// the cli<->tui import cycle introduced by the post-v1.0 TUI.
//
// All callers in the cli package continue to use the SessionRow / Gather /
// ExternalMode names they always have; the symbols just live one level down
// now. New code should import internal/session directly.
package cli

import (
	"github.com/monzim/muxc/internal/session"
)

// SessionRow is an alias for session.Row preserving the historic name.
type SessionRow = session.Row

// ExternalMode is an alias for session.ExternalMode preserving the historic name.
type ExternalMode = session.ExternalMode

const (
	ExternalNone       = session.ExternalNone
	ExternalWithClaude = session.ExternalWithClaude
	ExternalAll        = session.ExternalAll
)

// Re-export the gather + sort + filter API so existing call sites compile
// unchanged. Implementation lives in internal/session/gather.go.
var (
	Gather       = session.Gather
	SortRows     = session.SortRows
	FilterByGlob = session.FilterByGlob
	FilterByMode = session.FilterByMode
)
