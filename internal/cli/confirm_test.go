package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		prompt string
		want   bool
	}{
		// Affirmative inputs.
		{name: "y lower", input: "y\n", want: true},
		{name: "Y upper", input: "Y\n", want: true},
		{name: "yes lower", input: "yes\n", want: true},
		{name: "Yes mixed", input: "Yes\n", want: true},
		{name: "YES upper", input: "YES\n", want: true},

		// Negative inputs.
		{name: "n lower", input: "n\n", want: false},
		{name: "N upper", input: "N\n", want: false},
		{name: "no lower", input: "no\n", want: false},
		{name: "empty enter", input: "\n", want: false},
		{name: "arbitrary text", input: "anything\n", want: false},
		{name: "space only", input: "   \n", want: false},

		// Whitespace around affirmative still accepted.
		{name: "y with leading space", input: "  y  \n", want: true},
		{name: "yes with trailing space", input: "yes  \n", want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := strings.NewReader(tt.input)
			var out bytes.Buffer
			got, err := Confirm(in, &out, tt.prompt)
			if err != nil {
				t.Fatalf("Confirm returned unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Confirm(%q) = %v, want %v", tt.input, got, tt.want)
			}
			// Verify the prompt was written.
			if !strings.Contains(out.String(), "[y/N]") {
				t.Errorf("expected [y/N] in output, got %q", out.String())
			}
		})
	}
}

// TestConfirmEOF verifies that a closed reader (EOF) returns false + nil.
func TestConfirmEOF(t *testing.T) {
	t.Parallel()
	// A reader that immediately returns EOF.
	got, err := Confirm(io.NopCloser(strings.NewReader("")), &bytes.Buffer{}, "prompt")
	if err != nil {
		t.Fatalf("Confirm on EOF reader returned error: %v", err)
	}
	if got {
		t.Error("Confirm on EOF reader returned true, want false (default No)")
	}
}

// TestConfirmCustomPrompt verifies the full prompt string appears in output.
func TestConfirmCustomPrompt(t *testing.T) {
	t.Parallel()
	in := strings.NewReader("y\n")
	var out bytes.Buffer
	_, _ = Confirm(in, &out, "Kill 3 sessions?")
	if !strings.Contains(out.String(), "Kill 3 sessions?") {
		t.Errorf("custom prompt not found in output: %q", out.String())
	}
}
