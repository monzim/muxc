// Package render — JSON output.
// Fully implemented in Wave 0: all --json outputs are valid, indented JSON
// per spec §15.2. Timestamps are RFC3339 UTC; sizes are bytes (integers).
package render

import (
	"encoding/json"
	"io"
)

// JSON encodes v as indented JSON and writes it to w.
// Uses two-space indentation. Suitable for both array and object top-level values.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
