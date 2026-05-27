package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/monzim/muxc/internal/state"
)

// ── filterByIdle ─────────────────────────────────────────────────────────────

func TestIdleFilter(t *testing.T) {
	t.Parallel()

	rows := []SessionRow{
		{Name: "muxc-a", IdleSeconds: 10},
		{Name: "muxc-b", IdleSeconds: 3600},  // exactly 1h
		{Name: "muxc-c", IdleSeconds: 7200},  // 2h
		{Name: "muxc-d", IdleSeconds: 59},    // <1m
		{Name: "muxc-e", IdleSeconds: 14400}, // 4h
	}

	tests := []struct {
		name      string
		threshold time.Duration
		wantNames []string
	}{
		{
			name:      "1h threshold: 1h, 2h, 4h sessions",
			threshold: time.Hour,
			wantNames: []string{"muxc-b", "muxc-c", "muxc-e"},
		},
		{
			name:      "2h threshold: only 2h and 4h",
			threshold: 2 * time.Hour,
			wantNames: []string{"muxc-c", "muxc-e"},
		},
		{
			name:      "5h threshold: none",
			threshold: 5 * time.Hour,
			wantNames: []string{},
		},
		{
			name:      "0 threshold: all sessions",
			threshold: 0,
			wantNames: []string{"muxc-a", "muxc-b", "muxc-c", "muxc-d", "muxc-e"},
		},
		{
			name:      "30s threshold: excludes only muxc-a(10s) and muxc-d(59s>30s keeps it)",
			threshold: 30 * time.Second,
			wantNames: []string{"muxc-b", "muxc-c", "muxc-d", "muxc-e"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterByIdle(rows, tt.threshold)
			if len(got) != len(tt.wantNames) {
				t.Fatalf("filterByIdle(%v) returned %d rows, want %d: got %v",
					tt.threshold, len(got), len(tt.wantNames), rowNames(got))
			}
			for i, r := range got {
				if r.Name != tt.wantNames[i] {
					t.Errorf("filterByIdle()[%d] = %q, want %q", i, r.Name, tt.wantNames[i])
				}
			}
		})
	}
}

// ── runStaleKill ─────────────────────────────────────────────────────────────

func TestStaleKill(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		stateNames    []string
		liveNames     []string
		wantRemoved   []string
		wantRemaining []string
	}{
		{
			name:          "one stale entry",
			stateNames:    []string{"muxc-a", "muxc-b", "muxc-c"},
			liveNames:     []string{"muxc-b", "muxc-c"},
			wantRemoved:   []string{"muxc-a"},
			wantRemaining: []string{"muxc-b", "muxc-c"},
		},
		{
			name:          "all live, nothing removed",
			stateNames:    []string{"muxc-a", "muxc-b"},
			liveNames:     []string{"muxc-a", "muxc-b"},
			wantRemoved:   []string{},
			wantRemaining: []string{"muxc-a", "muxc-b"},
		},
		{
			name:          "all stale",
			stateNames:    []string{"muxc-a", "muxc-b"},
			liveNames:     []string{},
			wantRemoved:   []string{"muxc-a", "muxc-b"},
			wantRemaining: []string{},
		},
		{
			name:          "empty state",
			stateNames:    []string{},
			liveNames:     []string{"muxc-a"},
			wantRemoved:   []string{},
			wantRemaining: []string{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st := &state.State{
				Version:  1,
				Sessions: map[string]state.SessionEntry{},
			}
			for _, n := range tt.stateNames {
				st.Sessions[n] = state.SessionEntry{ProjectPath: "/tmp/" + n}
			}

			removed := runStaleKill(st, tt.liveNames)

			// Check removed count (order may vary; use a set comparison).
			removedSet := make(map[string]struct{}, len(removed))
			for _, r := range removed {
				removedSet[r] = struct{}{}
			}
			wantSet := make(map[string]struct{}, len(tt.wantRemoved))
			for _, r := range tt.wantRemoved {
				wantSet[r] = struct{}{}
			}

			if len(removedSet) != len(wantSet) {
				t.Errorf("runStaleKill removed %v, want %v", removed, tt.wantRemoved)
			}
			for r := range wantSet {
				if _, ok := removedSet[r]; !ok {
					t.Errorf("runStaleKill: expected %q to be removed but it was not", r)
				}
			}

			// Check remaining sessions in state.
			if len(st.Sessions) != len(tt.wantRemaining) {
				t.Errorf("state has %d sessions after prune, want %d", len(st.Sessions), len(tt.wantRemaining))
			}
			for _, r := range tt.wantRemaining {
				if _, ok := st.Sessions[r]; !ok {
					t.Errorf("expected session %q to remain in state but it was removed", r)
				}
			}
		})
	}
}

// ── formatKillTargets ────────────────────────────────────────────────────────

func TestFormatKillTargets(t *testing.T) {
	t.Parallel()

	t.Run("with rss shows MB", func(t *testing.T) {
		t.Parallel()
		rows := []SessionRow{
			{Name: "muxc-a", IdleSeconds: 1800, RSSPlusChildrenBytes: 200 * 1024 * 1024},
		}
		out := formatKillTargets(rows)
		if !strings.Contains(out, "muxc-a") {
			t.Error("expected session name in output")
		}
		if !strings.Contains(out, "MB") {
			t.Error("expected MB in output for non-zero RSS")
		}
	})

	t.Run("zero rss omits MB", func(t *testing.T) {
		t.Parallel()
		rows := []SessionRow{
			{Name: "muxc-b", IdleSeconds: 0, RSSPlusChildrenBytes: 0},
		}
		out := formatKillTargets(rows)
		if !strings.Contains(out, "muxc-b") {
			t.Error("expected session name in output")
		}
		if strings.Contains(out, "MB") {
			t.Error("MB should not appear when RSS is 0")
		}
	})

	t.Run("empty rows returns empty string", func(t *testing.T) {
		t.Parallel()
		out := formatKillTargets(nil)
		if out != "" {
			t.Errorf("expected empty string for nil rows, got %q", out)
		}
	})
}

// ── summarizeKill ────────────────────────────────────────────────────────────

func TestSummarizeKill(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		killed      []string
		freed       uint64
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:        "none killed",
			killed:      nil,
			freed:       0,
			wantContain: []string{"no sessions killed"},
		},
		{
			name:        "one killed, no RSS",
			killed:      []string{"muxc-a"},
			freed:       0,
			wantContain: []string{"killed 1 session"},
			wantAbsent:  []string{"freed", "MB"},
		},
		{
			name:        "multiple killed with RSS",
			killed:      []string{"muxc-a", "muxc-b", "muxc-c"},
			freed:       900 * 1024 * 1024,
			wantContain: []string{"killed 3 sessions", "freed", "MB", "estimated"},
		},
		{
			name:        "singular session wording",
			killed:      []string{"muxc-x"},
			freed:       0,
			wantContain: []string{"1 session"},
			wantAbsent:  []string{"1 sessions"},
		},
		{
			name:        "plural session wording",
			killed:      []string{"muxc-x", "muxc-y"},
			freed:       0,
			wantContain: []string{"2 sessions"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := summarizeKill(tt.killed, tt.freed)
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("summarizeKill() = %q: missing %q", got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("summarizeKill() = %q: should NOT contain %q", got, absent)
				}
			}
		})
	}
}

// ── Mode validation ───────────────────────────────────────────────────────────

func TestKillModeValidation(t *testing.T) {
	t.Parallel()

	t.Run("zero modes returns exit code 2", func(t *testing.T) {
		t.Parallel()
		err := runKill(killCmd, []string{})
		if err == nil {
			t.Fatal("expected error for zero modes, got nil")
		}
		var exitErr *ExitError
		if !asExitError(err, &exitErr) {
			t.Fatalf("expected *ExitError, got %T: %v", err, err)
		}
		if exitErr.Code != 2 {
			t.Errorf("exit code = %d, want 2", exitErr.Code)
		}
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func rowNames(rows []SessionRow) []string {
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Name
	}
	return names
}

// asExitError is a helper that does a type assertion to *ExitError.
func asExitError(err error, target **ExitError) bool {
	if e, ok := err.(*ExitError); ok {
		*target = e
		return true
	}
	return false
}
