package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/monzim/muxc/internal/config"
)

// ---- helpers ----------------------------------------------------------------

// writeFile creates a file at dir/name with the given content.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile %s: %v", p, err)
	}
	return p
}

// tempDir creates a temporary directory that is cleaned up at end of test.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "muxc-config-test-*")
	if err != nil {
		t.Fatalf("tempDir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// clearEnv clears the relevant env vars and restores them after the test.
func clearEnv(t *testing.T) {
	t.Helper()
	restore := func(key, val string, ok bool) {
		if ok {
			os.Setenv(key, val)
		} else {
			os.Unsetenv(key)
		}
	}
	v1, ok1 := os.LookupEnv("MUXC_NO_COLOR")
	v2, ok2 := os.LookupEnv("CLAUDE_CONFIG_DIR")
	os.Unsetenv("MUXC_NO_COLOR")
	os.Unsetenv("CLAUDE_CONFIG_DIR")
	t.Cleanup(func() {
		restore("MUXC_NO_COLOR", v1, ok1)
		restore("CLAUDE_CONFIG_DIR", v2, ok2)
	})
}

// ---- tests ------------------------------------------------------------------

// TestLoad_NoFile verifies that Load returns DefaultConfig() values when no
// config file is present.
func TestLoad_NoFile(t *testing.T) {
	clearEnv(t)
	cfgDir := tempDir(t) // empty — no config.toml

	got, err := config.Load(cfgDir, "")
	if err != nil {
		t.Fatalf("Load with no file: %v", err)
	}
	want := config.DefaultConfig()

	// Spot-check the key defaults.
	if got.Defaults.Prefix != want.Defaults.Prefix {
		t.Errorf("Prefix: got %q, want %q", got.Defaults.Prefix, want.Defaults.Prefix)
	}
	if got.Defaults.IdleThreshold != want.Defaults.IdleThreshold {
		t.Errorf("IdleThreshold: got %q, want %q", got.Defaults.IdleThreshold, want.Defaults.IdleThreshold)
	}
	if got.Display.Color != want.Display.Color {
		t.Errorf("Color: got %q, want %q", got.Display.Color, want.Display.Color)
	}
	if got.Logging.Level != want.Logging.Level {
		t.Errorf("Level: got %q, want %q", got.Logging.Level, want.Logging.Level)
	}
	if got.Attach.ForceDetach != want.Attach.ForceDetach {
		t.Errorf("ForceDetach: got %v, want %v", got.Attach.ForceDetach, want.Attach.ForceDetach)
	}
}

// TestLoad_GlobalOverlay verifies that keys present in config.toml override
// defaults while unset keys keep their default values.
func TestLoad_GlobalOverlay(t *testing.T) {
	clearEnv(t)
	cfgDir := tempDir(t)

	writeFile(t, cfgDir, "config.toml", `
[defaults]
idle_threshold = "2h"
claude_bin     = "myclaude"

[logging]
level = "debug"
`)

	got, err := config.Load(cfgDir, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Overridden values.
	if got.Defaults.IdleThreshold != "2h" {
		t.Errorf("IdleThreshold: got %q, want %q", got.Defaults.IdleThreshold, "2h")
	}
	if got.Defaults.ClaudeBin != "myclaude" {
		t.Errorf("ClaudeBin: got %q, want %q", got.Defaults.ClaudeBin, "myclaude")
	}
	if got.Logging.Level != "debug" {
		t.Errorf("Level: got %q, want %q", got.Logging.Level, "debug")
	}

	// Unset keys should keep their defaults.
	if got.Defaults.Prefix != "muxc-" {
		t.Errorf("Prefix (should be default): got %q, want %q", got.Defaults.Prefix, "muxc-")
	}
	if got.Display.MemUnit != "auto" {
		t.Errorf("MemUnit (should be default): got %q, want %q", got.Display.MemUnit, "auto")
	}

	// IdleThresholdParsed should reflect the overridden "2h".
	if got.IdleThresholdParsed() != 2*time.Hour {
		t.Errorf("IdleThresholdParsed: got %v, want %v", got.IdleThresholdParsed(), 2*time.Hour)
	}
}

// TestLoad_ProjectLocalOverlay verifies that .muxc.toml in the project directory
// only applies the whitelisted keys (claude_bin, launch_args, name_sessions)
// and ignores everything else.
func TestLoad_ProjectLocalOverlay(t *testing.T) {
	clearEnv(t)
	cfgDir := tempDir(t)
	projDir := tempDir(t)

	// Global config sets a baseline.
	writeFile(t, cfgDir, "config.toml", `
[defaults]
claude_bin = "global-claude"
prefix     = "pfx-"
`)

	// Project-local config overrides only allowed keys plus a disallowed key.
	writeFile(t, projDir, ".muxc.toml", `
[defaults]
claude_bin    = "proj-claude"
launch_args   = ["--no-auth"]
name_sessions = false
prefix        = "should-be-ignored-"
`)

	got, err := config.Load(cfgDir, projDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Whitelisted keys should be overridden.
	if got.Defaults.ClaudeBin != "proj-claude" {
		t.Errorf("ClaudeBin: got %q, want proj-claude", got.Defaults.ClaudeBin)
	}
	if len(got.Defaults.LaunchArgs) != 1 || got.Defaults.LaunchArgs[0] != "--no-auth" {
		t.Errorf("LaunchArgs: got %v, want [--no-auth]", got.Defaults.LaunchArgs)
	}
	if got.Defaults.NameSessions != false {
		t.Errorf("NameSessions: got %v, want false", got.Defaults.NameSessions)
	}

	// Non-whitelisted key (prefix) must not be applied.
	if got.Defaults.Prefix != "pfx-" {
		t.Errorf("Prefix should remain from global config %q, got %q", "pfx-", got.Defaults.Prefix)
	}
}

// TestLoad_ProjectLocalOverlay_OnlyWhitelisted verifies that project-local
// config keys outside the whitelist (e.g. [display], [attach]) are ignored.
func TestLoad_ProjectLocalOverlay_OnlyWhitelisted(t *testing.T) {
	clearEnv(t)
	cfgDir := tempDir(t)
	projDir := tempDir(t)

	writeFile(t, projDir, ".muxc.toml", `
[defaults]
claude_bin = "special-claude"

[display]
color = "always"

[logging]
level = "debug"
`)

	got, err := config.Load(cfgDir, projDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Whitelisted key applied.
	if got.Defaults.ClaudeBin != "special-claude" {
		t.Errorf("ClaudeBin: got %q, want special-claude", got.Defaults.ClaudeBin)
	}
	// Non-whitelisted sections ignored.
	if got.Display.Color != "auto" {
		t.Errorf("Color should remain default 'auto', got %q", got.Display.Color)
	}
	if got.Logging.Level != "warn" {
		t.Errorf("Level should remain default 'warn', got %q", got.Logging.Level)
	}
}

// TestLoad_InvalidDuration verifies the §19-formatted error for a bad
// idle_threshold value.
func TestLoad_InvalidDuration(t *testing.T) {
	clearEnv(t)
	cfgDir := tempDir(t)

	writeFile(t, cfgDir, "config.toml", `
[defaults]
idle_threshold = "forever"
`)

	_, err := config.Load(cfgDir, "")
	if err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}

	// Must wrap ErrConfig.
	if !errors.Is(err, config.ErrConfig) {
		t.Errorf("error should wrap ErrConfig, got: %v", err)
	}

	// Error message must contain the §19 shape.
	msg := err.Error()
	if !strings.Contains(msg, `idle_threshold "forever" is not a valid duration`) {
		t.Errorf("error message missing expected text, got: %s", msg)
	}
	if !strings.Contains(msg, `use Go duration syntax`) {
		t.Errorf("error message missing hint, got: %s", msg)
	}
}

// TestLoad_NoColor_EnvVar verifies that MUXC_NO_COLOR=1 forces color=never.
func TestLoad_NoColor_EnvVar(t *testing.T) {
	clearEnv(t)
	cfgDir := tempDir(t)

	os.Setenv("MUXC_NO_COLOR", "1")

	got, err := config.Load(cfgDir, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Display.Color != "never" {
		t.Errorf("Color: got %q, want never", got.Display.Color)
	}
}

// TestLoad_NoColor_EnvVar_Zero verifies that MUXC_NO_COLOR=0 does NOT override.
func TestLoad_NoColor_EnvVar_Zero(t *testing.T) {
	clearEnv(t)
	cfgDir := tempDir(t)

	os.Setenv("MUXC_NO_COLOR", "0")

	got, err := config.Load(cfgDir, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// "0" is treated as "not set", so the default "auto" should remain.
	if got.Display.Color != "auto" {
		t.Errorf("Color: got %q, want auto (MUXC_NO_COLOR=0 should not override)", got.Display.Color)
	}
}

// TestLoad_ClaudeConfigDir_EnvVar verifies that CLAUDE_CONFIG_DIR sets
// paths.claude_projects to <dir>/projects when not explicitly overridden.
func TestLoad_ClaudeConfigDir_EnvVar(t *testing.T) {
	clearEnv(t)
	cfgDir := tempDir(t)

	os.Setenv("CLAUDE_CONFIG_DIR", "/custom/claude")

	got, err := config.Load(cfgDir, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Paths.ClaudeProjects != "/custom/claude/projects" {
		t.Errorf("ClaudeProjects: got %q, want /custom/claude/projects", got.Paths.ClaudeProjects)
	}
}

// TestLoad_ClaudeConfigDir_EnvVar_Ignored verifies that CLAUDE_CONFIG_DIR does
// NOT override when the user has set paths.claude_projects explicitly.
func TestLoad_ClaudeConfigDir_EnvVar_Ignored(t *testing.T) {
	clearEnv(t)
	cfgDir := tempDir(t)

	writeFile(t, cfgDir, "config.toml", `
[paths]
claude_projects = "/my/explicit/projects"
`)
	os.Setenv("CLAUDE_CONFIG_DIR", "/should-not-be-used")

	got, err := config.Load(cfgDir, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Paths.ClaudeProjects != "/my/explicit/projects" {
		t.Errorf("ClaudeProjects: got %q, want /my/explicit/projects", got.Paths.ClaudeProjects)
	}
}

// TestLoad_MalformedTOML verifies that a syntactically broken TOML file
// returns an error rather than silently producing defaults.
func TestLoad_MalformedTOML(t *testing.T) {
	clearEnv(t)
	cfgDir := tempDir(t)

	writeFile(t, cfgDir, "config.toml", `
[defaults
idle_threshold = "4h"   # unclosed bracket — intentionally malformed
`)

	_, err := config.Load(cfgDir, "")
	if err == nil {
		t.Fatal("expected error for malformed TOML, got nil")
	}
	if !errors.Is(err, config.ErrConfig) {
		t.Errorf("error should wrap ErrConfig, got: %v", err)
	}
}

// TestValidate_AllInvalid exercises Validate with multiple bad values at once
// to confirm all failures are reported rather than stopping at the first.
func TestValidate_AllInvalid(t *testing.T) {
	clearEnv(t)
	c := config.DefaultConfig()
	c.Defaults.IdleThreshold = "not-a-duration"
	c.Display.MemUnit = "kb"
	c.Display.TimeFormat = "unix"
	c.Display.Color = "maybe"
	c.Logging.Level = "verbose"

	err := c.Validate()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	msg := err.Error()
	checks := []string{
		"idle_threshold",
		"mem_unit",
		"time_format",
		"color",
		"log level",
	}
	for _, needle := range checks {
		if !strings.Contains(msg, needle) {
			t.Errorf("expected %q in error message, got: %s", needle, msg)
		}
	}
}

// TestValidate_ValidDuration verifies that IdleThresholdParsed is set on success.
func TestValidate_ValidDuration(t *testing.T) {
	clearEnv(t)
	cfgDir := tempDir(t)

	writeFile(t, cfgDir, "config.toml", `
[defaults]
idle_threshold = "1h30m"
`)
	got, err := config.Load(cfgDir, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := 90 * time.Minute
	if got.IdleThresholdParsed() != want {
		t.Errorf("IdleThresholdParsed: got %v, want %v", got.IdleThresholdParsed(), want)
	}
}

// TestLoad_ProjectLocal_MalformedToml verifies that a malformed project-local
// .muxc.toml is silently ignored (logged at debug, not returned as error).
func TestLoad_ProjectLocal_MalformedToml(t *testing.T) {
	clearEnv(t)
	cfgDir := tempDir(t)
	projDir := tempDir(t)

	writeFile(t, projDir, ".muxc.toml", `
[defaults
claude_bin = "broken"
`)

	// Should succeed and use defaults (not crash).
	got, err := config.Load(cfgDir, projDir)
	if err != nil {
		t.Fatalf("Load with malformed project TOML should succeed, got: %v", err)
	}
	// claude_bin should remain the global default.
	if got.Defaults.ClaudeBin != "claude" {
		t.Errorf("ClaudeBin: got %q, want claude", got.Defaults.ClaudeBin)
	}
}
