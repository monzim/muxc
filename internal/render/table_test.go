package render

import (
	"bytes"
	"strings"
	"testing"
)

// TestTable verifies that Table writes output containing the headers and row values.
// We avoid asserting on exact formatting since tablewriter's column spacing may
// vary; instead we check that essential content is present.
func TestTable(t *testing.T) {
	headers := []string{"NAME", "PROJECT", "MEM", "ATTACHED"}
	rows := [][]string{
		{"muxc-my145", "~/code/my145", "412 MB", "yes"},
		{"muxc-iar", "~/code/iar", "287 MB", "no"},
	}

	t.Run("content present in output no color", func(t *testing.T) {
		var buf bytes.Buffer
		opts := TableOpts{Color: false, RightAlignCols: []int{2}}
		if err := Table(&buf, headers, rows, opts); err != nil {
			t.Fatalf("Table() error: %v", err)
		}
		out := buf.String()

		for _, h := range headers {
			if !strings.Contains(out, h) {
				t.Errorf("output missing header %q:\n%s", h, out)
			}
		}
		for _, row := range rows {
			for _, cell := range row {
				if !strings.Contains(out, cell) {
					t.Errorf("output missing cell %q:\n%s", cell, out)
				}
			}
		}
	})

	t.Run("empty rows produces only header line", func(t *testing.T) {
		var buf bytes.Buffer
		opts := TableOpts{Color: false}
		if err := Table(&buf, headers, [][]string{}, opts); err != nil {
			t.Fatalf("Table() error: %v", err)
		}
		out := buf.String()

		// Headers must still appear.
		for _, h := range headers {
			if !strings.Contains(out, h) {
				t.Errorf("output missing header %q:\n%s", h, out)
			}
		}
		// No row data should appear.
		if strings.Contains(out, "muxc-") {
			t.Errorf("output should not contain row data with empty rows:\n%s", out)
		}
	})

	t.Run("with color enabled does not error", func(t *testing.T) {
		var buf bytes.Buffer
		opts := TableOpts{Color: true, RightAlignCols: []int{2}}
		if err := Table(&buf, headers, rows, opts); err != nil {
			t.Fatalf("Table() with color error: %v", err)
		}
		if buf.Len() == 0 {
			t.Fatal("Table() with color produced empty output")
		}
	})

	t.Run("single row renders correctly", func(t *testing.T) {
		var buf bytes.Buffer
		singleRow := [][]string{{"session-1", "/path/to/proj", "100 MB", "yes"}}
		opts := TableOpts{Color: false}
		if err := Table(&buf, headers, singleRow, opts); err != nil {
			t.Fatalf("Table() error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "session-1") {
			t.Errorf("output missing 'session-1':\n%s", out)
		}
		if !strings.Contains(out, "/path/to/proj") {
			t.Errorf("output missing '/path/to/proj':\n%s", out)
		}
	})

	t.Run("right align col index out of range is safe", func(t *testing.T) {
		var buf bytes.Buffer
		opts := TableOpts{Color: false, RightAlignCols: []int{99, -1}}
		if err := Table(&buf, headers, rows, opts); err != nil {
			t.Fatalf("Table() error with out-of-range align: %v", err)
		}
	})
}
