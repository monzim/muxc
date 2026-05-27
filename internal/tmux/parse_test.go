package tmux

import (
	"testing"
	"time"
)

func TestParseSessions(t *testing.T) {
	t.Run("empty string returns empty slice", func(t *testing.T) {
		sessions, err := ParseSessions("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions) != 0 {
			t.Fatalf("expected empty slice, got %d sessions", len(sessions))
		}
	})

	t.Run("whitespace only returns empty slice", func(t *testing.T) {
		sessions, err := ParseSessions("\n\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions) != 0 {
			t.Fatalf("expected empty slice, got %d sessions", len(sessions))
		}
	})

	t.Run("single session", func(t *testing.T) {
		// session_name | created | activity | attached | id | windows
		raw := "my-session|1716800000|1716803600|0|$1|2"
		sessions, err := ParseSessions(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
		s := sessions[0]
		if s.Name != "my-session" {
			t.Errorf("Name: got %q, want %q", s.Name, "my-session")
		}
		if s.Created != time.Unix(1716800000, 0).UTC() {
			t.Errorf("Created: got %v, want %v", s.Created, time.Unix(1716800000, 0).UTC())
		}
		if s.Activity != time.Unix(1716803600, 0).UTC() {
			t.Errorf("Activity: got %v, want %v", s.Activity, time.Unix(1716803600, 0).UTC())
		}
		if s.Attached != 0 {
			t.Errorf("Attached: got %d, want 0", s.Attached)
		}
		if s.ID != "$1" {
			t.Errorf("ID: got %q, want %q", s.ID, "$1")
		}
		if s.Windows != 2 {
			t.Errorf("Windows: got %d, want 2", s.Windows)
		}
	})

	t.Run("multi-session with various activity timestamps", func(t *testing.T) {
		raw := "session-a|1716800000|1716800100|0|$1|1\n" +
			"session-b|1716810000|1716820000|1|$2|3\n" +
			"session-c|1716830000|1716840000|2|$3|5"
		sessions, err := ParseSessions(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions) != 3 {
			t.Fatalf("expected 3 sessions, got %d", len(sessions))
		}

		cases := []struct {
			name     string
			attached int
			id       string
			windows  int
		}{
			{"session-a", 0, "$1", 1},
			{"session-b", 1, "$2", 3},
			{"session-c", 2, "$3", 5},
		}
		for i, tc := range cases {
			if sessions[i].Name != tc.name {
				t.Errorf("[%d] Name: got %q, want %q", i, sessions[i].Name, tc.name)
			}
			if sessions[i].Attached != tc.attached {
				t.Errorf("[%d] Attached: got %d, want %d", i, sessions[i].Attached, tc.attached)
			}
			if sessions[i].ID != tc.id {
				t.Errorf("[%d] ID: got %q, want %q", i, sessions[i].ID, tc.id)
			}
			if sessions[i].Windows != tc.windows {
				t.Errorf("[%d] Windows: got %d, want %d", i, sessions[i].Windows, tc.windows)
			}
		}
	})

	t.Run("session name with hyphens muxc-my145", func(t *testing.T) {
		raw := "muxc-my145|1716800000|1716803600|1|$5|1"
		sessions, err := ParseSessions(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
		if sessions[0].Name != "muxc-my145" {
			t.Errorf("Name: got %q, want %q", sessions[0].Name, "muxc-my145")
		}
	})

	t.Run("session name with underscores", func(t *testing.T) {
		raw := "muxc_project_123|1716800000|1716803600|0|$7|1"
		sessions, err := ParseSessions(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sessions[0].Name != "muxc_project_123" {
			t.Errorf("Name: got %q, want %q", sessions[0].Name, "muxc_project_123")
		}
	})

	t.Run("attached count greater than 1", func(t *testing.T) {
		raw := "multi-attach|1716800000|1716803600|3|$9|1"
		sessions, err := ParseSessions(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sessions[0].Attached != 3 {
			t.Errorf("Attached: got %d, want 3", sessions[0].Attached)
		}
	})

	t.Run("malformed line too few fields returns error", func(t *testing.T) {
		raw := "session-a|1716800000|1716800100"
		_, err := ParseSessions(raw)
		if err == nil {
			t.Fatal("expected error for malformed line, got nil")
		}
	})

	t.Run("malformed line too many fields returns error", func(t *testing.T) {
		raw := "session-a|1716800000|1716800100|0|$1|1|extra"
		_, err := ParseSessions(raw)
		if err == nil {
			t.Fatal("expected error for malformed line, got nil")
		}
	})

	t.Run("non-numeric created timestamp returns error", func(t *testing.T) {
		raw := "session-a|notanumber|1716800100|0|$1|1"
		_, err := ParseSessions(raw)
		if err == nil {
			t.Fatal("expected error for non-numeric timestamp, got nil")
		}
	})

	t.Run("non-numeric activity timestamp returns error", func(t *testing.T) {
		raw := "session-a|1716800000|notanumber|0|$1|1"
		_, err := ParseSessions(raw)
		if err == nil {
			t.Fatal("expected error for non-numeric activity timestamp, got nil")
		}
	})

	t.Run("non-numeric attached returns error", func(t *testing.T) {
		raw := "session-a|1716800000|1716800100|notanumber|$1|1"
		_, err := ParseSessions(raw)
		if err == nil {
			t.Fatal("expected error for non-numeric attached, got nil")
		}
	})

	t.Run("non-numeric windows returns error", func(t *testing.T) {
		raw := "session-a|1716800000|1716800100|0|$1|notanumber"
		_, err := ParseSessions(raw)
		if err == nil {
			t.Fatal("expected error for non-numeric windows, got nil")
		}
	})

	t.Run("trailing newline is handled gracefully", func(t *testing.T) {
		raw := "my-session|1716800000|1716803600|0|$1|2\n"
		sessions, err := ParseSessions(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
	})

	t.Run("timestamps are stored as UTC", func(t *testing.T) {
		raw := "sess|1716800000|1716803600|0|$1|1"
		sessions, err := ParseSessions(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sessions[0].Created.Location() != time.UTC {
			t.Errorf("Created not UTC: got %v", sessions[0].Created.Location())
		}
		if sessions[0].Activity.Location() != time.UTC {
			t.Errorf("Activity not UTC: got %v", sessions[0].Activity.Location())
		}
	})
}
