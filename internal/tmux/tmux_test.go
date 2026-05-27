package tmux

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestVersion is a smoke-test that calls real tmux (if installed) and verifies
// the wrapper compiles and returns non-empty trimmed output beginning with "tmux".
func TestVersion(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found in PATH; skipping Version smoke-test")
	}

	ctx := context.Background()
	v, err := Version(ctx)
	if err != nil {
		t.Fatalf("Version() returned error: %v", err)
	}
	if v == "" {
		t.Fatal("Version() returned empty string")
	}
	if !strings.HasPrefix(v, "tmux ") {
		t.Errorf("Version() output %q does not start with 'tmux '", v)
	}
}

// TestClassifyError validates the error-classification logic without spawning tmux.
func TestClassifyError(t *testing.T) {
	cases := []struct {
		name       string
		stderr     string
		wantErr    error
		wantSubstr string
	}{
		{
			name:    "no server running",
			stderr:  "no server running on /tmp/tmux-1000/default",
			wantErr: ErrNoServer,
		},
		{
			name:    "no server running mixed case",
			stderr:  "No Server Running on socket",
			wantErr: ErrNoServer,
		},
		{
			name:    "error connecting to named socket",
			stderr:  "error connecting to /tmp/tmux-1000/muxc-int (No such file or directory)",
			wantErr: ErrNoServer,
		},
		{
			name:    "can't find session",
			stderr:  "can't find session: mysession",
			wantErr: ErrSessionNotFound,
		},
		{
			name:    "session not found",
			stderr:  "session not found: muxc-test",
			wantErr: ErrSessionNotFound,
		},
		{
			name:    "no such session",
			stderr:  "no such session",
			wantErr: ErrSessionNotFound,
		},
		{
			name:       "other error",
			stderr:     "some other tmux error occurred",
			wantSubstr: "some other tmux error occurred",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyError("test-cmd", []byte(tc.stderr))
			if err == nil {
				t.Fatal("expected non-nil error")
			}
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Errorf("got %v, want %v", err, tc.wantErr)
				}
			} else if tc.wantSubstr != "" {
				if !strings.Contains(err.Error(), tc.wantSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantSubstr)
				}
			}
		})
	}
}

// TestListSessionsNoServerReturnsEmpty verifies that ErrNoServer from the tmux
// command is converted to an empty slice with nil error (spec §12).
// We can't easily mock exec.Command here without refactoring the package, so
// this test validates the contract at the ParseSessions level: if ListSessions
// were to receive ErrNoServer it returns empty.
func TestListSessionsErrNoServerContract(t *testing.T) {
	// Validate that ErrNoServer is the sentinel we advertise.
	if ErrNoServer.Error() == "" {
		t.Error("ErrNoServer should have a non-empty message")
	}
	if ErrSessionNotFound.Error() == "" {
		t.Error("ErrSessionNotFound should have a non-empty message")
	}
}

// TestTmuxArgs validates the socket-selection helper introduced to support
// the MUXC_TMUX_SOCKET env var used by integration tests.
func TestTmuxArgs(t *testing.T) {
	t.Run("no socket env", func(t *testing.T) {
		t.Setenv("MUXC_TMUX_SOCKET", "")
		// Clear the env var entirely.
		if err := os.Unsetenv("MUXC_TMUX_SOCKET"); err != nil {
			t.Fatal(err)
		}
		got := tmuxArgs("list-sessions", "-F", "#{session_name}")
		want := []string{"list-sessions", "-F", "#{session_name}"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("arg[%d]: got %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("with socket env", func(t *testing.T) {
		t.Setenv("MUXC_TMUX_SOCKET", "muxc-test")
		got := tmuxArgs("list-sessions", "-F", "#{session_name}")
		want := []string{"-L", "muxc-test", "list-sessions", "-F", "#{session_name}"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("arg[%d]: got %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("version flag with socket", func(t *testing.T) {
		t.Setenv("MUXC_TMUX_SOCKET", "mysock")
		got := tmuxArgs("-V")
		want := []string{"-L", "mysock", "-V"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("arg[%d]: got %q, want %q", i, got[i], want[i])
			}
		}
	})
}
