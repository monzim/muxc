// Package render provides output formatting for muxc commands.
// Supports table rendering (via tablewriter), JSON, and humanize helpers.
// Color is controlled by the display.color config setting (spec §15.3).
package render

import (
	"io"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
)

// TableOpts configures table rendering behaviour per spec §15.1.
type TableOpts struct {
	// Color enables ANSI color for headers, "yes"/"no" values, and the top row.
	Color bool
	// Headers is the ordered list of column header strings.
	Headers []string
	// RightAlignCols lists column indices (0-based) that should be right-aligned.
	// Memory columns use right-alignment per spec §15.1.
	RightAlignCols []int
}

// Table writes a formatted table to w using the tablewriter v1 library.
//
// Spec §15.1 styling:
//   - No row separators between data rows.
//   - Headers in bold when opts.Color is true.
//   - Right-align columns listed in opts.RightAlignCols.
//   - "yes" rendered green, "no" rendered dim when color is enabled.
func Table(w io.Writer, headers []string, rows [][]string, opts TableOpts) error {
	// Build per-column alignment: default left, override right for specified columns.
	colCount := len(headers)
	alignment := make(tw.Alignment, colCount)
	for i := range alignment {
		alignment[i] = tw.AlignLeft
	}
	for _, idx := range opts.RightAlignCols {
		if idx >= 0 && idx < colCount {
			alignment[idx] = tw.AlignRight
		}
	}

	t := tablewriter.NewTable(w,
		// No row separators between data rows (spec §15.1).
		tablewriter.WithRendition(tw.Rendition{
			Settings: tw.Settings{
				Separators: tw.Separators{
					BetweenRows: tw.Off,
				},
			},
		}),
		// Per-column alignment for header, rows, and footer.
		tablewriter.WithAlignment(alignment),
		// Disable auto-formatting of headers (preserves our casing).
		tablewriter.WithHeaderAutoFormat(tw.Off),
	)

	// Set headers, applying bold if color is enabled.
	headerRow := make([]string, len(headers))
	copy(headerRow, headers)
	if opts.Color {
		bold := color.New(color.Bold)
		for i, h := range headerRow {
			headerRow[i] = bold.Sprint(h)
		}
	}
	t.Header(toAnySlice(headerRow)...)

	// Process rows, colorising "yes"/"no" values when color is enabled.
	green := color.New(color.FgGreen)
	dim := color.New(color.Faint)
	for _, row := range rows {
		processed := make([]string, len(row))
		copy(processed, row)
		if opts.Color {
			for i, cell := range processed {
				switch cell {
				case "yes":
					processed[i] = green.Sprint(cell)
				case "no":
					processed[i] = dim.Sprint(cell)
				}
			}
		}
		if err := t.Append(toAnySlice(processed)...); err != nil {
			return err
		}
	}

	return t.Render()
}

// toAnySlice converts a []string to []any for tablewriter's variadic Append/Header.
func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
