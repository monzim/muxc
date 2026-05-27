// Package tmux — parsing tmux list-sessions -F output.
//
// Format string used for all session listing (spec §12):
//
//	#{session_name}|#{session_created}|#{session_activity}|#{session_attached}|#{session_id}|#{session_windows}
//
// Fields, in order:
//   - session_name:     string (may contain hyphens, dots, unicode — not pipes)
//   - session_created:  Unix timestamp int64
//   - session_activity: Unix timestamp int64 — last pane activity; used for idle time
//   - session_attached: integer count of clients currently attached ("0" = detached)
//   - session_id:       tmux internal id like "$3"
//   - session_windows:  integer count of windows in the session
package tmux

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SessionFormat is the -F format string passed to tmux list-sessions.
// Field order must match ParseSessions's parsing logic.
const SessionFormat = "#{session_name}|#{session_created}|#{session_activity}|#{session_attached}|#{session_id}|#{session_windows}"

// ParseSessions parses the raw stdout of:
//
//	tmux list-sessions -F '#{session_name}|#{session_created}|#{session_activity}|#{session_attached}|#{session_id}|#{session_windows}'
//
// Each line maps to one Session. Empty input returns (nil, nil). A line that
// cannot be parsed returns an error immediately so callers can detect malformed
// output early.
func ParseSessions(raw string) ([]Session, error) {
	raw = strings.TrimRight(raw, "\n")
	if raw == "" {
		return []Session{}, nil
	}

	lines := strings.Split(raw, "\n")
	sessions := make([]Session, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) != 6 {
			return nil, fmt.Errorf("parse: malformed session line: %s", line)
		}

		name := parts[0]
		createdRaw := parts[1]
		activityRaw := parts[2]
		attachedRaw := parts[3]
		id := parts[4]
		windowsRaw := parts[5]

		createdUnix, err := strconv.ParseInt(createdRaw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse: malformed session line: %s", line)
		}

		activityUnix, err := strconv.ParseInt(activityRaw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse: malformed session line: %s", line)
		}

		attached, err := strconv.Atoi(attachedRaw)
		if err != nil {
			return nil, fmt.Errorf("parse: malformed session line: %s", line)
		}

		windows, err := strconv.Atoi(windowsRaw)
		if err != nil {
			return nil, fmt.Errorf("parse: malformed session line: %s", line)
		}

		sessions = append(sessions, Session{
			Name:     name,
			Created:  time.Unix(createdUnix, 0).UTC(),
			Activity: time.Unix(activityUnix, 0).UTC(),
			Attached: attached,
			ID:       id,
			Windows:  windows,
		})
	}

	return sessions, nil
}
