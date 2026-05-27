// Package render — human-readable formatting helpers.
// Wraps go-humanize for bytes and durations, with muxc-specific conventions.
package render

import (
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
)

// Bytes formats a byte count as a human-readable SI string using go-humanize.
// Examples: 0 → "0 B", 1024 → "1.0 kB", 1048576 → "1.0 MB", 1073741824 → "1.1 GB".
//
// Uses humanize.Bytes (SI, base-1000) rather than IBytes (IEC, base-1024) so
// that output matches spec sample "412 MB" / "1.84 GB" (no "iB" suffix).
// §25: colour-library choice is fatih/color; imported in table.go.
func Bytes(n uint64) string {
	return humanize.Bytes(n)
}

// Duration formats a time.Duration as a compact human string with at most two
// units (days/hours/minutes/seconds) and no spaces.
//
// Examples:
//
//	5*time.Second    → "5s"
//	90*time.Second   → "1m30s"
//	90*time.Minute   → "1h30m"
//	27*time.Hour     → "1d3h"
//	25*time.Hour     → "1d1h"
//	24*time.Hour     → "1d"
//	0               → "0s"
//
// Spec §11.1 / §15.1: UPTIME and IDLE columns use this format.
func Duration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	total := int64(d.Seconds())

	days := total / 86400
	rem := total % 86400
	hours := rem / 3600
	rem %= 3600
	minutes := rem / 60
	seconds := rem % 60

	// Build compact string with at most two non-zero leading units.
	type unit struct {
		val    int64
		suffix string
	}
	units := []unit{
		{days, "d"},
		{hours, "h"},
		{minutes, "m"},
		{seconds, "s"},
	}

	result := ""
	count := 0
	for _, u := range units {
		if count == 2 {
			break
		}
		if u.val > 0 || (count == 0 && u.val == 0 && u.suffix == "s") {
			// Only emit if non-zero, OR if we've hit the last unit with still nothing emitted.
			if u.val > 0 {
				result += fmt.Sprintf("%d%s", u.val, u.suffix)
				count++
			}
		}
	}

	if result == "" {
		// All zeros (d == 0) → show "0s".
		return "0s"
	}
	return result
}

// RelTime formats a time.Time as a relative human string (e.g. "5 minutes ago",
// "2 hours ago"). Uses go-humanize.Time internally.
//
// Spec §15.1: used in the UPTIME column when display.time_format = "relative".
func RelTime(t time.Time) string {
	return humanize.Time(t)
}

// LeftTruncate shortens s to at most max runes, replacing the leading characters
// with the Unicode ellipsis "…" (1 rune) when truncation is needed.
//
// An empty s or max <= 0 is returned unchanged. The ellipsis itself occupies one
// rune in the returned string so the visible width is always ≤ max runes.
//
// Example: LeftTruncate("/home/monzim/code/deep/nested/project", 20)
//
//	→ "…eep/nested/project"
//
// Spec §25: left-truncate with "…" prefix is the preferred strategy.
func LeftTruncate(s string, max int) string {
	if s == "" || max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	// Keep the last (max-1) runes and prefix with the ellipsis.
	keep := runes[len(runes)-(max-1):]
	return "…" + string(keep)
}
