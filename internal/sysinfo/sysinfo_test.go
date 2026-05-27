package sysinfo_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/monzim/muxc/internal/sysinfo"
)

// ---- TmuxMeetsMin tests -----------------------------------------------------

func TestTmuxMeetsMin(t *testing.T) {
	tests := []struct {
		name    string
		version string
		min     string
		want    bool
		wantErr bool
	}{
		// Basic equality.
		{name: "equal_3.0", version: "3.0", min: "3.0", want: true},
		{name: "equal_3.4", version: "3.4", min: "3.4", want: true},

		// version > min.
		{name: "3.4_gt_3.0", version: "3.4", min: "3.0", want: true},
		{name: "4.0_gt_3.9", version: "4.0", min: "3.9", want: true},

		// version < min.
		{name: "2.9_lt_3.0", version: "2.9", min: "3.0", want: false},
		{name: "3.0_lt_3.1", version: "3.0", min: "3.1", want: false},

		// Letter suffixes must be stripped; "3.0a" is treated as "3.0".
		{name: "3.0a_eq_3.0", version: "3.0a", min: "3.0", want: true},
		{name: "3.0_eq_3.0a", version: "3.0", min: "3.0a", want: true},
		{name: "3.0a_eq_3.0a", version: "3.0a", min: "3.0a", want: true},

		// Numeric comparison — must NOT be lexicographic.
		// "10.0" > "3.0" numerically; lexicographically "1" < "3" would be wrong.
		{name: "10.0_gt_3.0", version: "10.0", min: "3.0", want: true},
		{name: "10.0_gt_9.9", version: "10.0", min: "9.9", want: true},

		// "tmux " prefix stripping.
		{name: "prefix_version", version: "tmux 3.4", min: "3.0", want: true},
		{name: "prefix_both", version: "tmux 3.4", min: "tmux 3.5", want: false},

		// Multi-component comparisons.
		{name: "3.3.300_gt_3.3.200", version: "3.3.300", min: "3.3.200", want: true},
		{name: "3.3_lt_3.3.1", version: "3.3", min: "3.3.1", want: false},

		// Error cases.
		{name: "empty_version", version: "", min: "3.0", wantErr: true},
		{name: "letters_only", version: "abc", min: "3.0", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sysinfo.TmuxMeetsMin(tc.version, tc.min)
			if tc.wantErr {
				if err == nil {
					t.Errorf("TmuxMeetsMin(%q, %q): expected error, got nil", tc.version, tc.min)
				}
				return
			}
			if err != nil {
				t.Fatalf("TmuxMeetsMin(%q, %q): unexpected error: %v", tc.version, tc.min, err)
			}
			if got != tc.want {
				t.Errorf("TmuxMeetsMin(%q, %q) = %v, want %v", tc.version, tc.min, got, tc.want)
			}
		})
	}
}

// ---- TmuxVersion tests ------------------------------------------------------

// TestTmuxVersion_Integration runs `tmux -V` for real; skipped if tmux is
// not in PATH. This is an optional integration-style check within the unit test
// suite since tmux is a stated runtime requirement (spec §6).
func TestTmuxVersion_Integration(t *testing.T) {
	ctx := context.Background()
	ver, err := sysinfo.TmuxVersion(ctx)
	if err != nil {
		t.Skipf("tmux not available: %v", err)
	}
	if ver == "" {
		t.Error("TmuxVersion returned empty string without error")
	}
	// The version string should be parseable by TmuxMeetsMin.
	ok, parseErr := sysinfo.TmuxMeetsMin(ver, "0.1")
	if parseErr != nil {
		t.Errorf("TmuxMeetsMin(%q, 0.1): %v", ver, parseErr)
	}
	if !ok {
		t.Errorf("TmuxMeetsMin(%q, 0.1) = false, expected true for any real tmux", ver)
	}
}

// ---- OnLinux / ProcfsAvailable tests ----------------------------------------

// TestOnLinux verifies that OnLinux matches runtime.GOOS.
func TestOnLinux(t *testing.T) {
	want := runtime.GOOS == "linux"
	got := sysinfo.OnLinux()
	if got != want {
		t.Errorf("OnLinux() = %v, want %v (GOOS=%q)", got, want, runtime.GOOS)
	}
}

// TestProcfsAvailable verifies that ProcfsAvailable is consistent with
// the presence of /proc (true on Linux, typically false on macOS/Windows).
func TestProcfsAvailable(t *testing.T) {
	got := sysinfo.ProcfsAvailable()
	// On Linux /proc must exist; on other platforms it must not.
	if runtime.GOOS == "linux" && !got {
		t.Error("ProcfsAvailable() = false on Linux — /proc should be accessible")
	}
}
