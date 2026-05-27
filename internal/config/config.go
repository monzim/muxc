// Package config — loading, merging, and validation.
// See spec §9 for the schema and overlay rules.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// ErrConfig is the sentinel error for configuration problems.
// The CLI maps this to exit code 3 (spec §16).
var ErrConfig = errors.New("config error")

// idleThresholdCache stores the last successfully parsed idle threshold.
// It is populated by Validate() so callers can use IdleThresholdParsed()
// without re-parsing. The cache is stored per Config pointer using a
// package-level map protected by package init ordering (single goroutine
// at startup).
//
// We cannot add a field to Config because the struct is defined in
// defaults.go (the frozen file). Using a method+package map is the
// idiomatic workaround in Go for extending an immutable struct definition.
var idleThresholdCache = map[*Config]time.Duration{}

// IdleThresholdParsed returns the parsed time.Duration for
// Defaults.IdleThreshold. It is only valid after Validate() has been called
// (Load always calls Validate before returning).
func (c *Config) IdleThresholdParsed() time.Duration {
	return idleThresholdCache[c]
}

// projectLocalToml is the subset of Config that may be read from
// <project>/.muxc.toml. Only launch_args, name_sessions, and claude_bin
// are honoured (spec §9). All other keys in the file are ignored.
type projectLocalToml struct {
	Defaults struct {
		ClaudeBin    *string  `toml:"claude_bin"`
		LaunchArgs   []string `toml:"launch_args"`
		NameSessions *bool    `toml:"name_sessions"`
	} `toml:"defaults"`
}

// expandHome replaces a leading "~" with the user's home directory.
func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand home dir: %w", err)
	}
	return filepath.Join(home, path[1:]), nil
}

// Load reads the muxc configuration and returns the effective *Config.
//
// Resolution order (later values win):
//  1. DefaultConfig() baseline
//  2. configDir/config.toml (global user config)
//  3. Environment variables: MUXC_NO_COLOR, CLAUDE_CONFIG_DIR
//  4. projectPath/.muxc.toml — only launch_args, name_sessions, and claude_bin
//     are accepted from this overlay; other keys are ignored (spec §9).
//
// If configDir is empty, defaults to ~/.config/muxc.
// If projectPath is empty, the project-local overlay step is skipped.
// A missing config file is not an error — DefaultConfig() is returned as-is.
// A parse failure returns a config error (exit code 3 per spec §16).
func Load(configDir string, projectPath string) (*Config, error) {
	// Step 1: start from defaults.
	cfg := DefaultConfig()

	// Resolve configDir (expand ~ if needed).
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("%w: resolve home dir: %v", ErrConfig, err)
		}
		configDir = filepath.Join(home, ".config", "muxc")
	} else {
		expanded, err := expandHome(configDir)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrConfig, err)
		}
		configDir = expanded
	}

	// Step 2: read <configDir>/config.toml if present.
	// Decode directly into the default-populated struct; TOML only overwrites
	// keys that are present in the file, so unset keys keep their defaults.
	globalToml := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(globalToml); err == nil {
		if _, err := toml.DecodeFile(globalToml, cfg); err != nil {
			return nil, fmt.Errorf("%w: parse %s: %v", ErrConfig, globalToml, err)
		}
	}

	// Expand ~ in path fields that the user may have left as "~/.something".
	expandedSF, err := expandHome(cfg.Paths.StateFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfig, err)
	}
	cfg.Paths.StateFile = expandedSF

	expandedCP, err := expandHome(cfg.Paths.ClaudeProjects)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfig, err)
	}
	cfg.Paths.ClaudeProjects = expandedCP

	if cfg.Paths.LogFile != "" {
		expandedLF, err := expandHome(cfg.Paths.LogFile)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrConfig, err)
		}
		cfg.Paths.LogFile = expandedLF
	}

	// Step 3: apply environment variable overrides.
	//
	// MUXC_NO_COLOR → force display.color = "never" (spec §9, §15.3).
	if v := os.Getenv("MUXC_NO_COLOR"); v != "" && v != "0" {
		cfg.Display.Color = "never"
	}

	// CLAUDE_CONFIG_DIR → update paths.claude_projects if the user hasn't
	// explicitly overridden it from the default (spec §9).
	if claudeConfigDir := os.Getenv("CLAUDE_CONFIG_DIR"); claudeConfigDir != "" {
		// Only override if the current value is still the default (after home expansion).
		home, _ := os.UserHomeDir()
		defaultCP := filepath.Join(home, ".claude", "projects")
		if cfg.Paths.ClaudeProjects == defaultCP {
			cfg.Paths.ClaudeProjects = filepath.Join(claudeConfigDir, "projects")
		}
	}

	// Step 4: project-local overlay (if projectPath given).
	// Only launch_args, name_sessions, claude_bin are honoured (spec §9).
	if projectPath != "" {
		localToml := filepath.Join(projectPath, ".muxc.toml")
		if _, err := os.Stat(localToml); err == nil {
			var local projectLocalToml
			if _, err := toml.DecodeFile(localToml, &local); err != nil {
				// A bad project-level file is noteworthy but not fatal; log and skip.
				slog.Debug("project-local .muxc.toml parse error — ignoring",
					"path", localToml, "err", err)
			} else {
				if local.Defaults.ClaudeBin != nil {
					cfg.Defaults.ClaudeBin = *local.Defaults.ClaudeBin
				}
				if local.Defaults.LaunchArgs != nil {
					cfg.Defaults.LaunchArgs = local.Defaults.LaunchArgs
				}
				if local.Defaults.NameSessions != nil {
					cfg.Defaults.NameSessions = *local.Defaults.NameSessions
				}
			}
		}
	}

	// Step 5: validate (also populates idleThresholdCache).
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that all config values are semantically valid.
//
// Validated fields:
//   - Defaults.IdleThreshold must be a valid Go duration string
//   - Display.MemUnit must be one of "auto", "mb", "gb"
//   - Display.TimeFormat must be "relative" or "absolute"
//   - Display.Color must be "auto", "always", or "never"
//   - Logging.Level must be one of "debug", "info", "warn", "error"
//
// On success, IdleThresholdParsed() returns the parsed duration.
// Multiple validation failures are accumulated into a single error per spec §19.
func (c *Config) Validate() error {
	var errs []string

	// Validate and parse idle_threshold duration (spec §19 error shape).
	d, err := time.ParseDuration(c.Defaults.IdleThreshold)
	if err != nil {
		errs = append(errs,
			fmt.Sprintf("config error: idle_threshold %q is not a valid duration\n  use Go duration syntax, e.g. \"2h\", \"30m\", \"1h30m\"",
				c.Defaults.IdleThreshold))
	} else {
		idleThresholdCache[c] = d
	}

	// Validate display.mem_unit.
	switch c.Display.MemUnit {
	case "auto", "mb", "gb":
		// ok
	default:
		errs = append(errs,
			fmt.Sprintf("config error: mem_unit %q is invalid; must be one of: auto, mb, gb",
				c.Display.MemUnit))
	}

	// Validate display.time_format.
	switch c.Display.TimeFormat {
	case "relative", "absolute":
		// ok
	default:
		errs = append(errs,
			fmt.Sprintf("config error: time_format %q is invalid; must be one of: relative, absolute",
				c.Display.TimeFormat))
	}

	// Validate display.color.
	switch c.Display.Color {
	case "auto", "always", "never":
		// ok
	default:
		errs = append(errs,
			fmt.Sprintf("config error: color %q is invalid; must be one of: auto, always, never",
				c.Display.Color))
	}

	// Validate logging.level.
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
		// ok
	default:
		errs = append(errs,
			fmt.Sprintf("config error: log level %q is invalid; must be one of: debug, info, warn, error",
				c.Logging.Level))
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %s", ErrConfig, strings.Join(errs, "\n"))
	}
	return nil
}
