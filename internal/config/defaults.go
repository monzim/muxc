// Package config loads and validates the muxc configuration from
// ~/.config/muxc/config.toml, with optional project-local overlay from
// <project>/.muxc.toml. See spec §9 for the full TOML schema.
package config

// Config is the top-level configuration structure. All sub-structs map
// directly to TOML sections from spec §9. Fields are exported with TOML
// struct tags so BurntSushi/toml can decode them without reflection tricks.
type Config struct {
	Defaults Defaults `toml:"defaults"`
	Attach   Attach   `toml:"attach"`
	Display  Display  `toml:"display"`
	Paths    Paths    `toml:"paths"`
	Logging  Logging  `toml:"logging"`
}

// Defaults holds the core session-creation settings from [defaults].
type Defaults struct {
	// Prefix is the tmux session name prefix. Default: "muxc-".
	Prefix string `toml:"prefix"`
	// ClaudeBin is the claude executable name (PATH-resolved). Default: "claude".
	ClaudeBin string `toml:"claude_bin"`
	// LaunchArgs are args passed to claude after the binary name.
	LaunchArgs []string `toml:"launch_args"`
	// NameSessions controls whether -n <derived-name> is appended to claude.
	NameSessions bool `toml:"name_sessions"`
	// IdleThreshold is the default duration string for `kill --idle`. Default: "4h".
	IdleThreshold string `toml:"idle_threshold"`
	// ConfirmKill requires a y/N prompt before killing unless --yes is passed.
	ConfirmKill bool `toml:"confirm_kill"`
}

// Attach holds tmux-attach behaviour settings from [attach].
type Attach struct {
	// ForceDetach passes -d to tmux attach (steal session from other clients).
	ForceDetach bool `toml:"force_detach"`
	// FzfPicker enables fzf-based session picker when fzf is in PATH.
	FzfPicker bool `toml:"fzf_picker"`
}

// Display holds rendering and formatting settings from [display].
type Display struct {
	// MemUnit controls memory unit: "auto" | "mb" | "gb". Default: "auto".
	MemUnit string `toml:"mem_unit"`
	// TimeFormat controls time rendering: "relative" | "absolute". Default: "relative".
	TimeFormat string `toml:"time_format"`
	// ShowClaudeID includes the Claude session name/id column in tables.
	ShowClaudeID bool `toml:"show_claude_id"`
	// TruncatePath is the max project-path width in table output; 0 = no truncate.
	TruncatePath int `toml:"truncate_path"`
	// Color controls ANSI color: "auto" | "always" | "never". Default: "auto".
	Color string `toml:"color"`
}

// Paths holds filesystem path overrides from [paths].
type Paths struct {
	// ClaudeProjects is the root of Claude's project directory. Default: "~/.claude/projects".
	ClaudeProjects string `toml:"claude_projects"`
	// StateFile is the muxc state JSON file path. Default: "~/.config/muxc/state.json".
	StateFile string `toml:"state_file"`
	// LogFile is the append-only log path. Empty = no logging.
	LogFile string `toml:"log_file"`
}

// Logging holds log-level settings from [logging].
type Logging struct {
	// Level is one of: debug | info | warn | error. Default: "warn".
	Level string `toml:"level"`
}

// DefaultConfig returns a fully populated *Config with all spec §9 defaults.
// This is the baseline before any TOML file is loaded. Callers merge file
// values on top of these defaults.
func DefaultConfig() *Config {
	return &Config{
		Defaults: Defaults{
			Prefix:        "muxc-",
			ClaudeBin:     "claude",
			LaunchArgs:    []string{"--dangerously-skip-permissions"},
			NameSessions:  true,
			IdleThreshold: "4h",
			ConfirmKill:   true,
		},
		Attach: Attach{
			ForceDetach: true,
			FzfPicker:   true,
		},
		Display: Display{
			MemUnit:      "auto",
			TimeFormat:   "relative",
			ShowClaudeID: true,
			TruncatePath: 40,
			Color:        "auto",
		},
		Paths: Paths{
			ClaudeProjects: "~/.claude/projects",
			StateFile:      "~/.config/muxc/state.json",
			LogFile:        "",
		},
		Logging: Logging{
			Level: "warn",
		},
	}
}
