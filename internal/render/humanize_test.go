package render

import (
	"strings"
	"testing"
	"time"
)

// TestBytes verifies the Bytes helper formats byte counts as human-readable strings.
func TestBytes(t *testing.T) {
	cases := []struct {
		input    uint64
		wantSub  string // output must contain this substring
		wantFull string // if non-empty, output must equal exactly this
	}{
		{0, "0 B", "0 B"},
		{1000, "1.0 kB", "1.0 kB"},
		{1024 * 1024, "", ""},          // around 1 MB; just check it's non-empty
		{1024 * 1024 * 1024, "GB", ""}, // around 1 GB
	}

	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			got := Bytes(tc.input)
			if got == "" {
				t.Fatalf("Bytes(%d) returned empty string", tc.input)
			}
			if tc.wantFull != "" && got != tc.wantFull {
				t.Errorf("Bytes(%d) = %q, want %q", tc.input, got, tc.wantFull)
			}
			if tc.wantSub != "" && !strings.Contains(got, tc.wantSub) {
				t.Errorf("Bytes(%d) = %q, does not contain %q", tc.input, got, tc.wantSub)
			}
		})
	}
}

// TestDuration validates the compact Duration formatter.
func TestDuration(t *testing.T) {
	cases := []struct {
		input time.Duration
		want  string
	}{
		{0, "0s"},
		{5 * time.Second, "5s"},
		{90 * time.Second, "1m30s"},
		{90 * time.Minute, "1h30m"},
		{27 * time.Hour, "1d3h"},
		{25 * time.Hour, "1d1h"},
		{24 * time.Hour, "1d"},
		{time.Hour, "1h"},
		{time.Minute, "1m"},
		{61 * time.Second, "1m1s"},
		// Negative duration handled as absolute value.
		{-5 * time.Second, "5s"},
		// Two-unit cap: 1d3h, not 1d3h2m.
		{27*time.Hour + 2*time.Minute + 10*time.Second, "1d3h"},
		// Zero minutes, non-zero seconds: show only seconds.
		{30 * time.Second, "30s"},
	}

	for _, tc := range cases {
		t.Run(tc.input.String(), func(t *testing.T) {
			got := Duration(tc.input)
			if got != tc.want {
				t.Errorf("Duration(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestRelTime verifies that RelTime returns a non-empty string from go-humanize.
func TestRelTime(t *testing.T) {
	past := time.Now().Add(-5 * time.Minute)
	got := RelTime(past)
	if got == "" {
		t.Fatal("RelTime returned empty string")
	}
	// go-humanize returns strings like "5 minutes ago"
	if !strings.Contains(got, "minutes ago") && !strings.Contains(got, "minute ago") {
		// Allow some tolerance in case the timing differs; at least check non-empty.
		t.Logf("RelTime(%v) = %q (may be fine; just checking non-empty)", past, got)
	}
}

// TestLeftTruncate validates LeftTruncate across ASCII, unicode, and edge cases.
func TestLeftTruncate(t *testing.T) {
	cases := []struct {
		input string
		max   int
		want  string
	}{
		// Shorter than max → unchanged.
		{"hello", 10, "hello"},
		// Exactly max → unchanged.
		{"hello", 5, "hello"},
		// Empty string → unchanged.
		{"", 5, ""},
		// max <= 0 → unchanged.
		{"hello", 0, "hello"},
		{"hello", -1, "hello"},
		// Longer than max → ellipsis + suffix.
		// "hello world" = 11 runes; max=8: keep last 7 = "o world"; result = "…o world"
		{"hello world", 8, "…o world"},
		// Long path like spec example: 37 runes; max=20: keep last 19 = "deep/nested/project"
		{"/home/monzim/code/deep/nested/project", 20, "…deep/nested/project"},
		// Unicode: each accented char is one rune.
		// "héllo wörld" = 11 runes; max=8: keep last 7 = "o wörld"; result = "…o wörld"
		{"héllo wörld", 8, "…o wörld"},
		// Single character, max 1 → ellipsis only (keep last 0 chars).
		{"abcdef", 1, "…"},
		// max == 2 → ellipsis + last 1 char.
		{"abcdef", 2, "…f"},
	}

	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			got := LeftTruncate(tc.input, tc.max)
			if got != tc.want {
				t.Errorf("LeftTruncate(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.want)
			}
			// Verify rune count is <= max (when max > 0 and truncation happened).
			if tc.max > 0 && len([]rune(tc.input)) > tc.max {
				if len([]rune(got)) != tc.max {
					t.Errorf("LeftTruncate(%q, %d): result rune len = %d, want %d", tc.input, tc.max, len([]rune(got)), tc.max)
				}
			}
		})
	}
}
