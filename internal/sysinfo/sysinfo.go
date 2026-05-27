// Package sysinfo provides OS detection and runtime prerequisite checks
// used by `muxc doctor` and other commands that need to verify the environment.
package sysinfo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"unicode"
)

// TmuxVersion returns the tmux version string by running `tmux -V`.
// The "tmux " prefix is stripped so "tmux 3.4" becomes "3.4".
// Returns ("", error) if the command fails.
func TmuxVersion(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "tmux", "-V").Output()
	if err != nil {
		return "", fmt.Errorf("tmux -V: %w", err)
	}
	// Output is typically "tmux 3.4\n"; take first line and strip prefix.
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	version := strings.TrimPrefix(line, "tmux ")
	return strings.TrimSpace(version), nil
}

// TmuxMeetsMin reports whether version >= min, comparing numerically
// by major.minor components.
//
// Both strings may be plain "X.Y" or include a non-numeric suffix like "3.0a".
// The suffix is stripped before comparison: "3.0a" → "3.0". Strings may also
// be in the form "tmux X.Y" — the "tmux " prefix is stripped first.
//
// §6 requires tmux ≥ 3.0; "3.0a" (a point-release letter) passes that bar.
func TmuxMeetsMin(version, min string) (bool, error) {
	vParts, err := parseTmuxVersion(version)
	if err != nil {
		return false, fmt.Errorf("parse version %q: %w", version, err)
	}
	mParts, err := parseTmuxVersion(min)
	if err != nil {
		return false, fmt.Errorf("parse min version %q: %w", min, err)
	}

	// Compare component by component, extending the shorter slice with zeros.
	maxLen := len(vParts)
	if len(mParts) > maxLen {
		maxLen = len(mParts)
	}
	for i := 0; i < maxLen; i++ {
		v, m := 0, 0
		if i < len(vParts) {
			v = vParts[i]
		}
		if i < len(mParts) {
			m = mParts[i]
		}
		if v > m {
			return true, nil
		}
		if v < m {
			return false, nil
		}
	}
	// All components equal.
	return true, nil
}

// parseTmuxVersion strips a "tmux " prefix and any trailing non-numeric suffix
// (e.g. "3.0a" → "3.0") then splits on "." and converts each part to int.
func parseTmuxVersion(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	// Strip "tmux " prefix if present.
	s = strings.TrimPrefix(s, "tmux ")

	// Strip any trailing alphabetic suffix from the last component.
	// "3.0a" → "3.0",  "3.6" → "3.6",  "10.0" → "10.0".
	s = strings.TrimRightFunc(s, func(r rune) bool {
		return unicode.IsLetter(r)
	})

	if s == "" {
		return nil, fmt.Errorf("empty version string")
	}

	parts := strings.Split(s, ".")
	result := make([]int, 0, len(parts))
	for _, p := range parts {
		// Strip any letter suffix that may appear mid-string (defensive).
		p = strings.TrimRightFunc(p, unicode.IsLetter)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("non-numeric version component %q", p)
		}
		result = append(result, n)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no numeric components in version string")
	}
	return result, nil
}

// OnLinux reports whether the current runtime OS is Linux.
// Implemented directly — no stub needed.
func OnLinux() bool {
	return runtime.GOOS == "linux"
}

// ProcfsAvailable reports whether /proc is accessible on this system.
// Implemented directly via os.Stat — no stub needed.
func ProcfsAvailable() bool {
	_, err := os.Stat("/proc")
	return err == nil
}
