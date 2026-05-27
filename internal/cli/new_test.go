package cli

import (
	"testing"

	"github.com/monzim/muxc/internal/config"
)

// ── sanitizeName ─────────────────────────────────────────────────────────────

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Basic cases.
		{name: "all lowercase unchanged", input: "hello", want: "hello"},
		{name: "uppercase lowercased", input: "HELLO", want: "hello"},
		{name: "mixed case", input: "MyProject", want: "myproject"},

		// Dot replacement.
		{name: "dot replaced", input: "v1.5", want: "v1-5"},
		{name: "version with multiple dots", input: "My-Project_v1.5", want: "my-project_v1-5"},

		// Space handling.
		{name: "leading/trailing spaces", input: "  hello  ", want: "hello"},
		{name: "internal spaces", input: "hello world", want: "hello-world"},

		// All-invalid input.
		{name: "all dots becomes empty", input: "....", want: ""},
		{name: "only special chars", input: "!@#$", want: ""},

		// Unicode — only ASCII alphanumeric + _ + - survive.
		{name: "unicode cafe", input: "café", want: "caf"},
		{name: "unicode smiley", input: "hello😀world", want: "hello-world"},

		// Hyphen collapsing.
		{name: "consecutive hyphens collapsed", input: "a---b", want: "a-b"},
		{name: "leading hyphen trimmed", input: "-hello", want: "hello"},
		{name: "trailing hyphen trimmed", input: "hello-", want: "hello"},
		{name: "both ends trimmed", input: "-hello-", want: "hello"},

		// Underscore is preserved.
		{name: "underscore preserved", input: "my_project", want: "my_project"},

		// Numbers.
		{name: "numbers preserved", input: "project42", want: "project42"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── dedupName ────────────────────────────────────────────────────────────────

func TestDedupName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		base     string
		existing []string
		want     string
		wantErr  bool
	}{
		{
			name:     "no collision",
			base:     "muxc-foo",
			existing: []string{"muxc-bar"},
			want:     "muxc-foo",
		},
		{
			name:     "first collision picks -2",
			base:     "muxc-foo",
			existing: []string{"muxc-foo"},
			want:     "muxc-foo-2",
		},
		{
			name:     "first two collide, picks -3",
			base:     "muxc-foo",
			existing: []string{"muxc-foo", "muxc-foo-2"},
			want:     "muxc-foo-3",
		},
		{
			name:     "all collide returns error",
			base:     "muxc-foo",
			existing: []string{"muxc-foo", "muxc-foo-2", "muxc-foo-3", "muxc-foo-4", "muxc-foo-5", "muxc-foo-6", "muxc-foo-7", "muxc-foo-8", "muxc-foo-9", "muxc-foo-10"},
			wantErr:  true,
		},
		{
			name:     "empty existing list",
			base:     "muxc-new",
			existing: []string{},
			want:     "muxc-new",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := dedupName(tt.base, tt.existing)
			if tt.wantErr {
				if err == nil {
					t.Errorf("dedupName(%q, ...) returned nil error, want error", tt.base)
				}
				return
			}
			if err != nil {
				t.Fatalf("dedupName(%q, ...) returned unexpected error: %v", tt.base, err)
			}
			if got != tt.want {
				t.Errorf("dedupName(%q, ...) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}

// ── buildLaunchCmd ────────────────────────────────────────────────────────────

func TestBuildLaunchCmd(t *testing.T) {
	t.Parallel()

	baseCfg := func() *config.Config {
		cfg := config.DefaultConfig()
		cfg.Defaults.ClaudeBin = "claude"
		cfg.Defaults.LaunchArgs = []string{"--dangerously-skip-permissions"}
		cfg.Defaults.NameSessions = true
		return cfg
	}

	tests := []struct {
		name              string
		cfg               *config.Config
		claudeName        string
		extraArgs         []string
		includeClaudeName bool
		want              []string
	}{
		{
			name:              "basic with name",
			cfg:               baseCfg(),
			claudeName:        "my-project",
			extraArgs:         nil,
			includeClaudeName: true,
			want:              []string{"claude", "--dangerously-skip-permissions", "-n", "my-project"},
		},
		{
			name:              "no claude name flag",
			cfg:               baseCfg(),
			claudeName:        "my-project",
			extraArgs:         nil,
			includeClaudeName: false,
			want:              []string{"claude", "--dangerously-skip-permissions"},
		},
		{
			name:              "with extra args",
			cfg:               baseCfg(),
			claudeName:        "proj",
			extraArgs:         []string{"--verbose"},
			includeClaudeName: true,
			want:              []string{"claude", "--dangerously-skip-permissions", "--verbose", "-n", "proj"},
		},
		{
			name:              "include name true but empty claude name",
			cfg:               baseCfg(),
			claudeName:        "",
			extraArgs:         nil,
			includeClaudeName: true,
			// Empty claudeName → -n is NOT appended (guard in buildLaunchCmd).
			want: []string{"claude", "--dangerously-skip-permissions"},
		},
		{
			name: "custom claude bin",
			cfg: func() *config.Config {
				cfg := baseCfg()
				cfg.Defaults.ClaudeBin = "/usr/local/bin/claude"
				return cfg
			}(),
			claudeName:        "p",
			extraArgs:         nil,
			includeClaudeName: true,
			want:              []string{"/usr/local/bin/claude", "--dangerously-skip-permissions", "-n", "p"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildLaunchCmd(tt.cfg, tt.claudeName, tt.extraArgs, tt.includeClaudeName)
			if len(got) != len(tt.want) {
				t.Fatalf("buildLaunchCmd() = %v (%d), want %v (%d)", got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("buildLaunchCmd()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ── shellEscape ───────────────────────────────────────────────────────────────

func TestShellEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Safe words — returned unchanged.
		{name: "simple word", input: "claude", want: "claude"},
		{name: "path-like", input: "/usr/local/bin/claude", want: "/usr/local/bin/claude"},
		{name: "flag with hyphen", input: "--dangerously-skip-permissions", want: "--dangerously-skip-permissions"},
		{name: "alphanumeric", input: "abc123", want: "abc123"},
		{name: "underscore", input: "my_project", want: "my_project"},
		{name: "dot in path", input: "file.txt", want: "file.txt"},

		// Unsafe — must be single-quoted.
		{name: "space", input: "with space", want: "'with space'"},
		{name: "double space", input: "a  b", want: "'a  b'"},
		{name: "embedded single quote", input: "it's", want: `'it'\''s'`},
		{name: "double quote inside", input: `say "hello"`, want: `'say "hello"'`},
		{name: "dollar sign", input: "$HOME", want: "'$HOME'"},
		{name: "backtick", input: "`cmd`", want: "'`cmd`'"},
		{name: "empty string", input: "", want: "''"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := shellEscape(tt.input)
			if got != tt.want {
				t.Errorf("shellEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
