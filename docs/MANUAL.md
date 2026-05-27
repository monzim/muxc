# muxc user manual

This is the authoritative reference for `muxc`. The [README](../README.md) is
the quick tour; this file documents every command, flag, configuration key,
exit code, and failure mode.

> muxc manages [Claude Code](https://docs.anthropic.com/claude/docs/claude-code)
> sessions running inside tmux: list them, create new ones, attach, and kill
> the idle ones. It is a thin, no-daemon, no-network Go binary that uses tmux
> as the source of truth for liveness and `/proc` for memory accounting.

---

## Table of contents

1. [Installation](#installation)
2. [Quick start](#quick-start)
3. [Command reference](#command-reference)
4. [The TUI](#the-tui)
5. [Configuration](#configuration)
6. [Environment variables](#environment-variables)
7. [JSON output](#json-output)
8. [Exit codes](#exit-codes)
9. [Troubleshooting](#troubleshooting)
10. [Architecture at a glance](#architecture-at-a-glance)

---

## Installation

muxc is a single static Go binary. The only runtime dependency is `tmux ≥ 3.0`.
Linux is required for full memory accounting (`/proc`); macOS works for
session management but memory columns are empty.

### Option 1 — release binary (recommended)

The release workflow publishes a tarball per `linux/amd64` and `linux/arm64`,
plus a SHA-256 checksums file.

```bash
# Pick a version
VERSION=1.0.0
ARCH=amd64   # or arm64

# Download
curl -fL -o muxc.tar.gz \
  "https://github.com/monzim/muxc/releases/download/v${VERSION}/muxc_${VERSION}_linux_${ARCH}.tar.gz"
curl -fL -o checksums.txt \
  "https://github.com/monzim/muxc/releases/download/v${VERSION}/muxc_${VERSION}_checksums.txt"

# Verify
grep "muxc_${VERSION}_linux_${ARCH}.tar.gz" checksums.txt | sha256sum --check

# Extract + install
tar -xzf muxc.tar.gz muxc
install -m 0755 muxc ~/.local/bin/muxc

# Confirm
~/.local/bin/muxc version
~/.local/bin/muxc doctor
```

### Option 2 — `.deb` / `.rpm` package

The same release builds ship distro packages. They depend on `tmux` and
install to `/usr/local/bin/muxc`.

```bash
# Debian / Ubuntu
sudo dpkg -i muxc_1.0.0_linux_amd64.deb

# Fedora / RHEL
sudo rpm -i muxc_1.0.0_linux_amd64.rpm
```

### Option 3 — Homebrew (macOS / Linuxbrew)

```bash
brew install monzim/tap/muxc
```

### Option 4 — `go install`

Requires Go 1.22+. The resulting binary lacks the injected version metadata
(`muxc version` will report `dev` / `unknown`).

```bash
go install github.com/monzim/muxc/cmd/muxc@latest
```

### Option 5 — build from source

```bash
git clone https://github.com/monzim/muxc.git
cd muxc
make build               # produces bin/muxc with full ldflags metadata
make install             # copies bin/muxc to ~/.local/bin/muxc
```

### Shell completions

```bash
# bash (system-wide)
muxc completion bash | sudo tee /etc/bash_completion.d/muxc >/dev/null

# bash (user-local)
mkdir -p ~/.local/share/bash-completion/completions
muxc completion bash > ~/.local/share/bash-completion/completions/muxc

# zsh — anywhere on $fpath
muxc completion zsh > "${fpath[1]}/_muxc"

# fish
muxc completion fish > ~/.config/fish/completions/muxc.fish
```

Re-source your shell rc file or open a new terminal for completions to take
effect.

### Uninstall

```bash
rm -f ~/.local/bin/muxc           # or /usr/local/bin/muxc for package installs
rm -rf ~/.config/muxc             # remove state + config (optional)
# brew uninstall monzim/tap/muxc  # if installed via Homebrew
```

`~/.claude/` is owned by Claude Code and not touched by uninstall.

---

## Quick start

```bash
muxc new ~/code/my-project         # launch a Claude session
muxc ls                            # see all sessions
muxc attach my-project             # jump in (detach with Ctrl-b d)
muxc info my-project               # full details including transcript path
muxc kill my-project --yes         # tear down
muxc kill --idle 4h --dry-run      # preview which sessions to reap
muxc                               # no subcommand → opens the TUI
```

---

## Command reference

### Global flags

These persistent flags work on every subcommand:

| Flag | Default | Description |
|---|---|---|
| `--json` | off | Emit machine-readable JSON instead of a table (read commands only). |
| `--config <path>` | `~/.config/muxc` | Override the config directory. Equivalent to `MUXC_CONFIG_DIR`. |

---

### `muxc ls` — list sessions

List all muxc-managed tmux sessions plus any external tmux session that
contains a detected Claude process.

```
muxc ls [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--all` | false | Include every tmux session, even those without a Claude process. |
| `--sort <key>` | `name` | Sort key: `name`, `mem`, `idle`, or `created`. |
| `--filter <glob>` | empty | Glob pattern matched against session name (e.g. `--filter 'muxc-*api*'`). |
| `--json` | off | Emit JSON instead of a table. |

**Columns:** `NAME`, `PROJECT`, `CLAUDE`, `UPTIME`, `IDLE`, `MEM`, `ATTACHED`.

External (non-`muxc-` prefix) sessions are shown with a trailing `*` on the
name and `"is_external": true` in JSON.

**Example:**

```bash
$ muxc ls
NAME             PROJECT             CLAUDE      UPTIME   IDLE   MEM     ATTACHED
muxc-api         ~/code/api          api         2h14m    3m     412 MB  no
muxc-website     ~/code/website      website     1d2h     1h22m  287 MB  yes
review-456*      ~/code/api          review-…    18m      4m     180 MB  no
```

JSON form yields a top-level array; an empty array (`[]`) is emitted when
nothing matches.

---

### `muxc new <project-path>` — create a session

Resolve `<project-path>`, derive a tmux session name (basename + `muxc-`
prefix), create a fresh tmux session rooted at the directory, then send a
`send-keys` line that launches Claude inside it.

```
muxc new <project-path> [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--name <n>` | derived | Override the session base name (the configured prefix is still applied). |
| `--no-claude-name` | false | Do not pass `-n <name>` to `claude`; let Claude assign its own UUID. |
| `--args "<str>"` | empty | Extra space-separated args appended to the claude launch command for this session only. |
| `--no-launch` | false | Create the tmux session and `cd` but do not start Claude (handy for manual launches). |

Project-local `.muxc.toml` overrides — when present at the project root, the
file may set `claude_bin`, `launch_args`, and `name_sessions` for this session
only. Other keys are ignored. See [Configuration](#configuration).

**Naming rules** (sanitisation):

1. Input is lowercased.
2. Any character outside `[a-z0-9_-]` becomes `-`.
3. Repeated `-` are collapsed.
4. Leading and trailing `-` are stripped.
5. If a tmux session with the same final name already exists, muxc appends
   `-2`, `-3`, … up to `-10`. Beyond that you must pass `--name`.

**Output:**

```
started muxc-my-project in /home/me/code/my-project
attach with: muxc attach my-project
```

---

### `muxc attach [name]` — attach to a session

Replaces the muxc process with `tmux attach` via `syscall.Exec`, so you land
directly inside tmux with no extra shell layer. `last_attached_at` is updated
in `state.json` *before* the exec.

```
muxc attach [name] [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--no-detach` | false | Do not pass `-d`; share the session with other clients. |

With no `name` argument:

- If exactly one matching session exists, attach to it.
- If `fzf` is installed and `attach.fzf_picker` is true (default), launch
  the fzf picker.
- Otherwise present a numbered prompt on stdin.

Cancel (Esc in fzf, empty/invalid input on the prompt) exits 0 silently.

---

### `muxc kill [name]` — kill sessions

Exactly one of `name`, `--idle`, `--all`, `--stale` must be specified.

```
muxc kill [name] [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--idle <duration>` | — | Kill sessions whose idle time ≥ this duration (Go duration: `30m`, `2h`, `1h30m`). |
| `--all` | false | Kill every muxc-managed session. |
| `--stale` | false | Remove `state.json` entries with no live tmux counterpart. Does not invoke `tmux kill-session`. |
| `-y`, `--yes` | false | Skip the confirmation prompt. |
| `--dry-run` | false | Print which sessions *would* be killed, then exit 0. |

**External sessions are never killed by `--all` or `--idle`** — only sessions
matching the configured prefix (`muxc-` by default) are affected. To kill an
external session by name, use `tmux kill-session -t <name>` directly.

**Confirmation:** by default `kill` prompts `Kill N session(s)?` listing the
targets with their idle time and memory. Disable globally with
`defaults.confirm_kill = false`, or per-call with `-y`.

---

### `muxc mem` — memory-sorted view

Like `ls`, but always sorted by RSS descending and with a totals footer.
Useful for "which session is hogging RAM?".

```
muxc mem [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--json` | off | Emit JSON: `{ "sessions": [...], "totals": { "count": N, "rss_bytes": N } }`. |

The top consumer is bolded when color is enabled. Memory is RSS of the Claude
process and all its descendants (MCP servers, language servers, helpers).
RSS over-counts shared pages — PSS would be more accurate but is deferred to
a future release.

---

### `muxc info <name>` — detailed view

Render the full picture for a single session: pane PID, process tree with
per-PID RSS, Claude session ID and name, transcript path with size and mtime,
and optionally the tail of the JSONL transcript.

```
muxc info <name> [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--transcript-lines <N>` | 0 | Tail the last *N* JSONL entries (capped at 100). |
| `--json` | off | Emit JSON with `process_tree`, `transcript`, and `recent` fields. |

**Sample output:**

```
muxc-api
  project:   ~/code/api
  created:   2026-05-27 14:02:11 (1d3h ago)
  activity:  2026-05-28 16:48:09 (3m idle)
  attached:  yes (1 client)
  tmux id:   $4

claude session:
  name:      api
  id:        9b1c…
  transcript: ~/.claude/projects/-home-me-code-api/9b1c…jsonl
  size:      812 KB, modified 3m ago

processes (rss):
  ├─ 18421 -zsh                                       4.1 MB
  └─ 18422 node /usr/bin/claude --dangerously…       408 MB
  total: 412 MB
```

---

### `muxc doctor` — environment checks

Runs ten environment checks and reports `OK`, `WARN`, or `FAIL` for each.

```
muxc doctor [--json]
```

The checks (defined in `internal/doctor/doctor.go`):

| # | Check | What it verifies |
|---|---|---|
| 1 | tmux installed (≥ 3.0) | `tmux -V` succeeds and version satisfies the floor. |
| 2 | claude on PATH | `defaults.claude_bin` is resolvable via `$PATH`. |
| 3 | `~/.claude/projects/` exists | The Claude project directory is readable. |
| 4 | `~/.config/muxc/` writable | Config dir exists (created if not) and is writable. |
| 5 | `config.toml` parses | If present, it loads and validates. |
| 6 | `state.json` parses | If present, it is valid JSON. |
| 7 | no orphan state entries | Every `state.json` entry maps to a live tmux session. |
| 8 | fzf installed | Optional; without it the numbered picker is used. |
| 9 | `/proc` accessible | Required on Linux for memory readings. |
| 10 | running as real user (UID > 0) | Warns when run as root. |

**Exit code:** 0 unless any check is `FAIL`. `WARN` alone is *not* a failure.

---

### `muxc completion <shell>`

Emit a shell-completion script for `bash`, `zsh`, or `fish` to stdout. See
[Shell completions](#shell-completions) above for installation paths.

---

### `muxc version`

Print the version, commit SHA, build date, and Go runtime:

```
$ muxc version
muxc v1.0.0 (commit a1b2c3d, built 2026-05-28T12:00:00Z, go go1.24.2)
```

Version, commit, and date are injected at build time via `-ldflags`. The
`go install` path leaves them as `dev` / `unknown`.

---

### `muxc` (no subcommand)

Launches the Bubble Tea TUI when stdin/stdout are TTYs and `MUXC_NO_TUI` is
unset. Otherwise falls back to a plain-text numbered REPL — useful for CI,
piped input, or dumb terminals.

---

## The TUI

The TUI is the default landing experience when you run `muxc` bare. Five
screens are stacked behind the sessions list; press `esc` to return to it.

### Sessions screen (default)

Auto-refreshes every 2 seconds.

| Key | Action |
|---|---|
| `↑`/`↓` or `j`/`k` | Move cursor in the sessions table. |
| `g` / `G` | Jump to top / bottom. |
| `enter` or `i` | Open the **info** screen for the selected session. |
| `a` | **Attach** — replaces muxc with `tmux attach`. Detaching from tmux returns you to your shell, not to the TUI. |
| `n` | Open the **new session** form. |
| `K` | Open the **kill picker** (capital K so it doesn't clash with `k` navigation). |
| `s` | Cycle sort key: name → mem → idle → created → name. |
| `r` | Force an immediate refresh. |
| `d` | Open the **doctor** screen. |
| `?` | Toggle help overlay. |
| `q` or `Ctrl-C` | Quit. |

External tmux sessions running Claude appear with a `★` prefix on the name.

### New session form (`n`)

Form fields:

- **Project path** — defaults to the current working directory.
- **Name** — optional override; the configured prefix is still applied.
- **Skip Claude name (-n)** — toggle whether `-n <name>` is passed.
- **Extra args** — appended to the launch command for this session only.
- **No launch** — create the tmux session without starting Claude.

Submit with `enter`; cancel with `esc`. After creation, the TUI returns to
the sessions list with the new row already populated.

### Kill picker (`K`)

A filterable list of muxc-managed sessions. Multi-select with `space`,
confirm with `enter`. The confirm modal shows total memory that will be
freed.

### Info screen (`enter` / `i`)

The same content `muxc info` renders, with process tree, transcript metadata,
and (optionally) recent transcript entries. Press `esc` to return.

### Doctor screen (`d`)

The same 10 checks `muxc doctor` runs, displayed live and re-runnable with
`r`.

### Non-TTY fallback

When stdout is not a terminal, or when `MUXC_NO_TUI=1` is set, muxc falls
back to a plain numbered REPL. The REPL exposes the same operations: list,
new, attach, kill, info, doctor.

---

## Configuration

Global config lives at `~/.config/muxc/config.toml`. All keys are optional —
defaults are used for anything absent. A project-local `.muxc.toml` at the
root of a project can override a small subset of keys for sessions created
from that directory.

### Full schema with defaults

```toml
[defaults]
# tmux session name prefix. Every session created with `muxc new` starts with
# this. `kill --all` and `kill --idle` only ever touch sessions matching it.
prefix         = "muxc-"

# Path or name of the claude executable. Resolved via $PATH.
claude_bin     = "claude"

# Args appended after the binary name when launching Claude. Opaque to muxc
# — it does not understand Claude's flag semantics.
launch_args    = ["--dangerously-skip-permissions"]

# When true, muxc passes `-n <derived-name>` to claude so the session has a
# human-readable name. Toggle off to let Claude auto-generate a UUID name.
name_sessions  = true

# Default duration for `kill --idle` when no value is passed. Go duration
# syntax: "2h", "30m", "1h30m".
idle_threshold = "4h"

# Prompt for y/N before killing. Disable for non-interactive scripts (or
# pass `-y` per call).
confirm_kill   = true

[attach]
# Pass -d to `tmux attach` (steals the session from other clients). Most
# users want this; turn off if you regularly attach from multiple terminals.
force_detach   = true

# Use fzf for session selection when no name is given. Falls back to the
# numbered prompt if fzf is not installed.
fzf_picker     = true

[display]
# "auto" | "mb" | "gb". Memory column unit.
mem_unit       = "auto"

# "relative" (e.g. "3m ago") or "absolute" (e.g. "2026-05-28 14:22").
time_format    = "relative"

# Include the Claude session name/id column in the ls table.
show_claude_id = true

# Maximum width of the project-path column; 0 disables truncation.
truncate_path  = 40

# "auto" | "always" | "never". ANSI color in tables.
color          = "auto"

[paths]
# Where Claude Code stores per-project transcript directories.
claude_projects = "~/.claude/projects"

# Where muxc keeps its side-table of session metadata.
state_file      = "~/.config/muxc/state.json"

# Append-only log file. Empty disables logging.
log_file        = ""

[logging]
# "debug" | "info" | "warn" | "error". Default is warn — only warnings and
# above hit stderr.
level           = "warn"
```

### Project-local `.muxc.toml`

Only three keys are honoured from a project-local file:

```toml
[defaults]
claude_bin    = "/opt/claude/bin/claude"   # alternate binary for this project
launch_args   = ["--strict-mode"]           # override the global launch args
name_sessions = false                       # let Claude pick its own UUID
```

All other keys are silently ignored. Parse errors are logged at debug level
and the file is skipped — they do not abort `muxc new`.

### Where the config directory lives

Resolution order:

1. `--config <path>` flag, if given.
2. `MUXC_CONFIG_DIR` environment variable, if set.
3. `~/.config/muxc/` (default).

---

## Environment variables

| Variable | Effect |
|---|---|
| `MUXC_CONFIG_DIR` | Override the config directory (same as `--config`). |
| `MUXC_NO_COLOR` | Force color off — equivalent to `display.color = "never"`. |
| `MUXC_NO_TUI` | Force the plain-text REPL even on a real terminal (skip Bubble Tea). |
| `MUXC_TMUX_SOCKET` | Target a private tmux socket via `tmux -L <name>`. Primarily for tests and sandboxed setups. |
| `CLAUDE_CONFIG_DIR` | Respected when `paths.claude_projects` is the default; muxc looks in `<value>/projects`. |
| `NO_COLOR` | Standard. Disables color in `auto` mode. |

---

## JSON output

Every read command supports `--json`. Output is valid JSON, timestamps are
RFC 3339, durations are integers in seconds, sizes are integers in bytes.

### `muxc ls --json`

Top-level array of session rows. Empty result is `[]`, not `null`.

```jsonc
[
  {
    "name": "muxc-api",
    "project_path": "/home/me/code/api",
    "claude_session_name": "api",
    "claude_session_id": "9b1c4f7e-…",
    "uptime_seconds": 8054,
    "idle_seconds": 180,
    "rss_plus_children_bytes": 432013312,
    "attached": false,
    "attached_clients": 0,
    "is_external": false,
    "tmux_session_id": "$4",
    "pane_pid": 18421,
    "created_at": "2026-05-27T14:02:11Z",
    "activity_at": "2026-05-28T16:48:09Z"
  }
]
```

### `muxc mem --json`

```jsonc
{
  "sessions": [ /* same shape as ls */ ],
  "totals":   { "count": 3, "rss_bytes": 879267840 }
}
```

### `muxc info --json`

Extends a single `ls` row with:

```jsonc
{
  /* ...all ls fields... */
  "process_tree": [
    { "pid": 18421, "comm": "-zsh",                              "rss_bytes":   4300800 },
    { "pid": 18422, "comm": "node /usr/bin/claude --dangerously", "rss_bytes": 427712512 }
  ],
  "total_processes": 2,
  "transcript": {
    "path": "/home/me/.claude/projects/-home-me-code-api/9b1c….jsonl",
    "size_bytes": 832000,
    "mod_time": "2026-05-28T16:48:09Z"
  },
  "recent": [
    { "role": "user",      "content": "…" },
    { "role": "assistant", "content": "…" }
  ]
}
```

### `muxc doctor --json`

Array of check results:

```jsonc
[
  { "name": "tmux installed (≥ 3.0)", "status": "OK",   "message": "3.4" },
  { "name": "fzf installed",          "status": "WARN", "message": "fzf not found; numbered prompt will be used instead" }
]
```

Useful pipelines:

```bash
muxc ls --json | jq '.[] | select(.idle_seconds > 14400) | .name'
muxc mem --json | jq '.totals.rss_bytes / 1024 / 1024 | floor'
muxc doctor --json | jq -r '.[] | select(.status=="FAIL") | .name'
```

---

## Exit codes

The exit code is part of the contract — assert against it in scripts.

| Code | Meaning | Examples |
|---|---|---|
| 0   | Success | Normal completion; doctor with no FAILs; user-cancelled picker. |
| 1   | General failure | Subprocess error (tmux exec failed unexpectedly), unwritable state. |
| 2   | Bad usage / target not found | Session name doesn't exist, multiple kill modes passed at once, project path missing. |
| 3   | Config error | `config.toml` fails to parse or validate. |
| 4   | Precondition failure | A doctor-style fatal precondition (rare in v1; reserved). |
| 130 | SIGINT | User pressed Ctrl-C. |

---

## Troubleshooting

### `muxc doctor` says `tmux` is too old

Upgrade to tmux ≥ 3.0. muxc uses format strings (`#{session_activity}`) and
behaviours introduced in 3.0.

```bash
# Debian/Ubuntu
sudo apt-get install --reinstall tmux

# From source (when distro is stuck on an old version)
# see https://github.com/tmux/tmux/wiki/Installing
```

### `muxc doctor` warns that `/proc` is unavailable

You are on macOS (or another non-Linux platform). Session management works,
but the `MEM` column will be empty and `mem`/`info` give no memory data.
Native macOS support (via `libproc`) is on the roadmap for v2.

### `muxc new` says the project path doesn't exist

The path is resolved with `~` expansion and `filepath.Abs`. Pass an existing
directory. muxc will not create the directory for you.

### A session in `state.json` no longer exists in tmux

Run `muxc kill --stale`. It removes orphaned entries without invoking
`tmux kill-session`. `muxc ls` also prunes silently on every invocation, so
the orphan list shouldn't grow unbounded.

### `state.json` is corrupted

muxc logs a warning to stderr and continues with empty state. tmux is still
the source of truth for what is alive. The only thing lost is the ancillary
metadata (original launch args, `last_attached_at` timestamps), which is
reconstructed on the next `muxc new`.

```bash
# nuke the side-table if you want to start fresh
rm ~/.config/muxc/state.json
```

### `muxc kill --idle` doesn't touch an external session I expected to reap

By design. `--idle` and `--all` only act on sessions whose names start with
the configured prefix (`muxc-`). Kill external sessions with
`tmux kill-session -t <name>`.

### fzf is installed but the picker doesn't appear

Two possible causes:

1. `attach.fzf_picker = false` in your config. Flip it back to `true`.
2. Only one session matched — muxc skips the picker and attaches directly.

### `claude` runs but the TUI looks wrong

Force the plain REPL fallback:

```bash
MUXC_NO_TUI=1 muxc
```

If `TERM` is unset or `dumb`, the TUI also refuses to start. Set
`TERM=xterm-256color` and try again.

### How do I confirm my conversation is intact after `muxc kill`?

Claude writes the transcript continuously to
`~/.claude/projects/<encoded-path>/<session-id>.jsonl`. After kill:

```bash
ls -la ~/.claude/projects/<encoded-path>/
claude --resume <name>
```

muxc never touches `~/.claude/`.

### Memory numbers disagree with `htop`

muxc reports RSS from `/proc/<pid>/status` (`VmRSS`) summed across the
Claude process and its descendants. `htop` defaults to RES (also RSS) so
numbers should match within ±5%; differences usually come from:

- htop's "Hide kernel threads" / "Hide userland process threads" filters.
- A pane shell process that htop attributes to a different user when listing
  by tree.
- Brief drift because muxc samples instantaneously while htop refreshes.

PSS (proportional set size) would discount shared pages and is on the
roadmap behind a `--pss` flag.

---

## Architecture at a glance

```
cmd/muxc/                  entry point, ldflags, signal handling
internal/cli/              cobra commands + REPL fallback
internal/tui/              Bubble Tea TUI screens
internal/session/          live session gather + row schema
internal/doctor/           the 10 environment checks
internal/tmux/             single point of contact for tmux subprocesses
internal/proc/             /proc walker, Claude process identification, RSS
internal/claude/           read-only access to ~/.claude/projects/ JSONLs
internal/config/           TOML loader + defaults
internal/state/            ~/.config/muxc/state.json read/write
internal/render/           table + JSON + humanize helpers
internal/sysinfo/          OS / tmux version detection
```

**Invariants** (full list in [`spec.md`](../spec.md) §2):

- tmux is the source of truth for liveness. muxc never infers "is this
  running" from `state.json`.
- `~/.claude/` is read-only.
- No daemon, no socket, no background process. Each invocation reconstructs
  state from tmux + `/proc` + `~/.claude/projects/`.
- No network I/O. No Claude API calls.
- Pure Go, no CGo. Single static binary, < 10 MB stripped.
- `--json` exists on every read command. The output struct is designed first;
  the table is a renderer over it.

For the full design rationale, including features deliberately rejected,
see [`spec.md`](../spec.md) (§3 and §23 in particular).
