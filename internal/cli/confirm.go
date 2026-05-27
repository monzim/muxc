// Package cli — interactive confirmation prompt helper.
//
// Confirm provides a minimal y/N prompt with default-No semantics.
// It is intentionally extracted so callers (kill, etc.) can inject
// a fake io.Reader in tests without touching stdin.
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Confirm writes prompt followed by " [y/N]: " to out, then reads one line
// from in. It returns true only when the trimmed, lowercased input is "y" or
// "yes". Any other input — including empty (just Enter) — returns false.
//
// EOF on the reader is treated as the default answer (No), returning
// false + nil so callers don't need to special-case a closed pipe.
//
// The prompt string should not already contain a trailing space or "[y/N]"
// suffix; Confirm appends that itself.
func Confirm(in io.Reader, out io.Writer, prompt string) (bool, error) {
	fmt.Fprintf(out, "%s [y/N]: ", prompt)

	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		// EOF or closed reader → default No, no error (spec §11.4 semantics).
		if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}
		return false, nil
	}

	line := strings.TrimSpace(strings.ToLower(sc.Text()))
	return line == "y" || line == "yes", nil
}
