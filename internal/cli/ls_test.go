package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/monzim/muxc/internal/config"
	"github.com/monzim/muxc/internal/render"
)

// ---- table rendering smoke tests -------------------------------------------

// captureTable calls RenderLsTable writing to a temp file and captures output.
func captureTable(t *testing.T, cfg *config.Config, rows []SessionRow) string {
	t.Helper()

	// RenderLsTable takes *os.File; we use a pipe to capture output.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()

	origStdout := os.Stdout
	os.Stdout = w

	renderErr := RenderLsTable(os.Stdout, cfg, rows)

	os.Stdout = origStdout
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if renderErr != nil {
		t.Fatalf("RenderLsTable: %v", renderErr)
	}
	return buf.String()
}

func TestLsTable_EmptyRows(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Display.Color = "never"

	// Empty rows → header row only; must not panic.
	r, w, _ := os.Pipe()
	defer r.Close()
	orig := os.Stdout
	os.Stdout = w

	err := RenderLsTable(os.Stdout, cfg, []SessionRow{})
	os.Stdout = orig
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if err != nil {
		t.Fatalf("RenderLsTable empty: %v", err)
	}
	// Header should still be present.
	if !strings.Contains(out, "NAME") {
		t.Errorf("header missing in empty table output:\n%s", out)
	}
}

func TestLsTable_OneRow(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Display.Color = "never"

	now := time.Now()
	rows := []SessionRow{
		{
			Name:                 "muxc-myproj",
			ProjectPath:          "/home/user/code/myproj",
			ClaudeSessionName:    "auth-refactor",
			TmuxSessionID:        "$1",
			CreatedAt:            now.Add(-2 * time.Hour),
			ActivityAt:           now.Add(-5 * time.Minute),
			IdleSeconds:          300,
			UptimeSeconds:        7200,
			Attached:             true,
			AttachedClients:      1,
			RSSPlusChildrenBytes: 432 * 1024 * 1024,
		},
	}

	r, w, _ := os.Pipe()
	defer r.Close()
	orig := os.Stdout
	os.Stdout = w

	err := RenderLsTable(os.Stdout, cfg, rows)
	os.Stdout = orig
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if err != nil {
		t.Fatalf("RenderLsTable one row: %v", err)
	}

	// Verify key cells appear in the output.
	checks := []struct {
		field string
		want  string
	}{
		{"name", "muxc-myproj"},
		{"claude", "auth-refactor"},
		{"attached", "yes"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("RenderLsTable %s: %q not found in:\n%s", c.field, c.want, out)
		}
	}
}

func TestLsTable_Headers(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Display.Color = "never"

	r, w, _ := os.Pipe()
	defer r.Close()
	orig := os.Stdout
	os.Stdout = w

	err := RenderLsTable(os.Stdout, cfg, []SessionRow{})
	os.Stdout = orig
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if err != nil {
		t.Fatalf("RenderLsTable headers: %v", err)
	}

	for _, h := range []string{"NAME", "PROJECT", "CLAUDE", "UPTIME", "IDLE", "MEM", "ATTACHED"} {
		if !strings.Contains(out, h) {
			t.Errorf("header %q missing in output:\n%s", h, out)
		}
	}
}

// ---- claudeDisplay tests ---------------------------------------------------

func TestClaudeDisplay_Name(t *testing.T) {
	r := SessionRow{ClaudeSessionName: "my-session"}
	got := claudeDisplay(r)
	if got != "my-session" {
		t.Errorf("claudeDisplay name: got %q", got)
	}
}

func TestClaudeDisplay_ID(t *testing.T) {
	r := SessionRow{ClaudeSessionID: "abcdef1234567890"}
	got := claudeDisplay(r)
	if got != "abcdef12…" {
		t.Errorf("claudeDisplay id: got %q, want %q", got, "abcdef12…")
	}
}

func TestClaudeDisplay_ShortID(t *testing.T) {
	r := SessionRow{ClaudeSessionID: "abc"}
	got := claudeDisplay(r)
	if got != "abc" {
		t.Errorf("claudeDisplay short id: got %q", got)
	}
}

func TestClaudeDisplay_Empty(t *testing.T) {
	r := SessionRow{}
	got := claudeDisplay(r)
	if got != "-" {
		t.Errorf("claudeDisplay empty: got %q", got)
	}
}

func TestClaudeDisplay_NameTakesPrecedence(t *testing.T) {
	r := SessionRow{ClaudeSessionName: "auth", ClaudeSessionID: "uuid-1234"}
	got := claudeDisplay(r)
	if got != "auth" {
		t.Errorf("claudeDisplay name precedence: got %q, want %q", got, "auth")
	}
}

// ---- homeRelative tests ----------------------------------------------------

func TestHomeRelative_WithHome(t *testing.T) {
	got := homeRelative("/home/user/code/proj", "/home/user")
	if got != "~/code/proj" {
		t.Errorf("homeRelative: got %q, want ~/code/proj", got)
	}
}

func TestHomeRelative_ExactHome(t *testing.T) {
	got := homeRelative("/home/user", "/home/user")
	if got != "~" {
		t.Errorf("homeRelative exact: got %q, want ~", got)
	}
}

func TestHomeRelative_NoPrefix(t *testing.T) {
	got := homeRelative("/other/path", "/home/user")
	if got != "/other/path" {
		t.Errorf("homeRelative no prefix: got %q", got)
	}
}

func TestHomeRelative_EmptyPath(t *testing.T) {
	got := homeRelative("", "/home/user")
	if got != "" {
		t.Errorf("homeRelative empty: got %q", got)
	}
}

// ---- ColorEnabled tests ----------------------------------------------------

func TestColorEnabled_Never(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Display.Color = "never"
	if ColorEnabled(cfg) {
		t.Error("ColorEnabled never: expected false")
	}
}

func TestColorEnabled_Always(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Display.Color = "always"
	if !ColorEnabled(cfg) {
		t.Error("ColorEnabled always: expected true")
	}
}

func TestColorEnabled_AutoNoColor(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Display.Color = "auto"
	t.Setenv("NO_COLOR", "1")
	if ColorEnabled(cfg) {
		t.Error("ColorEnabled auto with NO_COLOR: expected false")
	}
}

func TestColorEnabled_AutoMuxcNoColor(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Display.Color = "auto"
	t.Setenv("MUXC_NO_COLOR", "1")
	if ColorEnabled(cfg) {
		t.Error("ColorEnabled auto with MUXC_NO_COLOR: expected false")
	}
}

// ---- JSON output tests -----------------------------------------------------

func TestLsJSON_EmptyRows(t *testing.T) {
	var buf bytes.Buffer
	err := render.JSON(&buf, []SessionRow{})
	if err != nil {
		t.Fatalf("render.JSON: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	// Should be an empty array, not null.
	if out != "[]" {
		t.Errorf("JSON empty: got %q, want []", out)
	}
}

func TestLsJSON_RowFields(t *testing.T) {
	now := time.Now().Truncate(time.Second).UTC()
	rows := []SessionRow{
		{
			Name:          "muxc-test",
			TmuxSessionID: "$5",
			CreatedAt:     now,
			ActivityAt:    now,
			Attached:      false,
		},
	}

	var buf bytes.Buffer
	if err := render.JSON(&buf, rows); err != nil {
		t.Fatalf("render.JSON: %v", err)
	}
	out := buf.String()

	for _, want := range []string{`"name"`, `"muxc-test"`, `"tmux_session_id"`, `"$5"`} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON output missing %q:\n%s", want, out)
		}
	}
}
