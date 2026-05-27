# muxc — Claude Code Session Manager

**Version:** 1.0.0 (spec)
**Status:** Implementation-ready
**Target:** Linux x86_64 / arm64 (macOS as v2 stretch)
**Language:** Go 1.22+
**License:** MIT

---

## 1. Problem Statement

A developer running multiple Claude Code sessions on a headless remote workspace via tmux faces three concrete problems:

1. **Discovery.** No fast way to see which sessions exist, what project each is tied to, and which Claude conversation each is running.
2. **Resource pressure.** Each Claude Code session holds significant RAM (Node runtime + MCP servers + language servers). Idle sessions accumulate. No easy way to identify and reclaim them.
3. **Lifecycle friction.** Creating, attaching to, and tearing down sessions is a mix of ad-hoc tmux commands, shell aliases, and manual `cd && claude` invocations. Inconsistent across projects.

`muxc` solves these by being a thin, single-binary CLI layered over tmux and Claude Code's existing session persistence.

## 2. Design Philosophy

These are non-negotiable. Every implementation decision must respect them.

1. **tmux is the source of truth for liveness.** A session exists iff tmux says it does. Never trust local state files for "is this running."
2. **Claude Code owns its own persistence.** Conversation transcripts live at `~/.claude/projects/<project>/<session-id>.jsonl`. `muxc` reads these but never writes to them. Ever.
3. **No daemon, no background process, no socket.** Every command is a fresh one-shot. State is reconstructed from tmux + procfs + Claude's session directory on every invocation.
4. **Stateless against catastrophe.** If the local state file is deleted, `muxc` still works — it just loses ancillary metadata like the original launch command. Recovery is automatic on next `muxc new`.
5. **Single static binary.** No runtime dependencies beyond `tmux` itself. Distribute via `scp`.
6. **Read-only filesystem boundaries.** `~/.claude/` is read-only. `~/.config/muxc/` is the only write target.
7. **No Claude API calls.** `muxc` does not make HTTP requests. No tokens, no network, no rate limits.
8. **No automation of `claude -p`.** Headless mode has terms-of-service implications on subscription accounts. `muxc` orchestrates interactive sessions only.
9. **`--json` on every read command.** Output structs designed first, table rendering second.
10. **Fail loudly, recover silently.** Errors print to stderr with actionable messages. Missing optional state never blocks operation.

## 3. Out of Scope (Will Not Build)

These have been considered and rejected. Do not add them in v1 or propose them as "small extensions."

| Feature | Reason rejected |
|---|---|
| Background daemon / supervisor | Adds crash-recovery complexity. Stateless one-shots are more robust. |
| Auto-restart of crashed Claude processes | Hides errors. User should see and decide. |
| Remote-host management (SSH from laptop) | Adds auth, transport, state-sync complexity. SSH to box, run `muxc`. |
| Headless mode (`claude -p`) orchestration | ToS risk on subscription accounts; out of scope domain. |
| Conversation editing / transcript modification | `~/.claude/` is read-only by contract. |
| Multi-user mode | Single-user tool. State lives in `$HOME`. |
| TUI dashboard (ncurses) | `watch -n 2 muxc ls` covers 95% of value. Revisit only if real usage demands it. |
| Web UI / mobile app | It is a CLI. |
| Project scaffolding / templates | Claude already has `CLAUDE.md`, settings, skills. Wrong layer. |
| Support for screen, zellij, wezterm | tmux only. Forks welcome for other multiplexers. |
| Telemetry / analytics / auto-update | None. |
| Notification system (idle alerts, etc.) | `muxc ls --json \| jq` + cron solves this externally. |
| Writes to `~/.claude/` | Hard boundary. Claude owns that directory. |
| Hardcoded Claude CLI flags | All flags live in config. `muxc` never knows Claude's flag semantics. |

## 4. Glossary

| Term | Definition |
|---|---|
| **tmux session** | A tmux session named `muxc-<slug>` containing one Claude Code process. |
| **Claude session** | A persisted Claude Code conversation, stored as JSONL in `~/.claude/projects/`. Has either a UUID session ID or a user-set name. |
| **Project path** | Absolute filesystem path the Claude session is rooted at (its `cwd`). |
| **Pane PID** | The PID of the shell tmux spawned in the session's first pane. Parent of the Claude process. |
| **Claude process** | The `node` process whose `cmdline` references the `claude` entry script. |
| **Process tree** | Claude process plus all descendants (MCP servers, ripgrep, language servers). |
| **Idle time** | Seconds since tmux last recorded any pane activity (input or output). |
| **State file** | `~/.config/muxc/state.json`. Side-table for metadata tmux doesn't track. |
| **Stale entry** | An entry in `state.json` whose corresponding tmux session no longer exists. |

## 5. Naming

The binary, command name, and module are all `muxc`. Pronounced "mux-see." The name reads as "mux Claude" / "multiplex Claude," which is what it does.

Go module: `github.com/<user>/muxc` (replace `<user>` at init).
Binary install path: `~/.local/bin/muxc` (recommended; user can override).
Config dir: `~/.config/muxc/`.

## 6. Dependencies

### Runtime (external)

- **tmux** ≥ 3.0 (required, hard dependency)
- **claude** (Claude Code CLI) — invoked but not statically required
- **fzf** — optional, used by `muxc attach` if present
- **/proc** — Linux procfs (required for memory measurement)

### Build (Go modules)

Recommended set. Implementer may substitute equivalents but should justify in PR.

```
github.com/spf13/cobra        // CLI framework
github.com/spf13/viper        // config (alternative: BurntSushi/toml directly)
github.com/BurntSushi/toml    // TOML parser
github.com/olekukonko/tablewriter  // table rendering
github.com/dustin/go-humanize // memory/time formatting
```

No CGo. Pure Go. Static binary.

## 7. File Layout (on user's machine)

```
~/.config/muxc/
  config.toml          # user-editable, see §9
  state.json           # see §10
  muxc.log             # optional, append-only; controlled by config

~/.claude/projects/    # READ-ONLY for muxc
  <encoded-project-path>/
    <session-id>.jsonl
    ...

~/.local/bin/muxc      # the binary
```

Project-local override (optional, read on `muxc new` only):

```
<project>/.muxc.toml   # overrides keys from global config.toml for this project
```

## 8. Source Code Layout

```
muxc/
  cmd/
    muxc/
      main.go              # entry point, cobra setup
  internal/
    cli/
      root.go              # root command, global flags
      ls.go                # muxc ls
      new.go               # muxc new
      attach.go            # muxc attach
      kill.go              # muxc kill
      mem.go               # muxc mem
      info.go              # muxc info
      doctor.go            # muxc doctor
      completion.go        # muxc completion
    config/
      config.go            # TOML load, merge, validation
      defaults.go          # default values
    state/
      state.go             # state.json read/write/migrate
    tmux/
      tmux.go              # tmux command wrappers (list, new, kill, attach, send-keys)
      parse.go             # parsing -F format output
    proc/
      tree.go              # PID tree walk via /proc
      memory.go            # /proc/<pid>/status VmRSS reader
      claude.go            # identify the Claude process in a tree
    claude/
      sessions.go          # read ~/.claude/projects/ for session metadata
      transcript.go        # parse JSONL transcripts
    render/
      table.go             # table output
      json.go              # JSON output
      humanize.go          # mem/time formatting
    sysinfo/
      sysinfo.go           # OS detection, tmux version check
  go.mod
  go.sum
  README.md
  LICENSE
  Makefile                 # build, test, install, lint
  .goreleaser.yml          # release packaging (optional v1)
```

## 9. Configuration Schema

File: `~/.config/muxc/config.toml`. All keys optional; defaults applied for missing.

```toml
[defaults]
prefix          = "muxc-"                          # tmux session name prefix
claude_bin      = "claude"                         # claude executable name (PATH-resolved)
launch_args     = ["--dangerously-skip-permissions"]  # args passed after claude_bin
name_sessions   = true                             # if true, append -n <derived-name>
idle_threshold  = "4h"                             # default for `kill --idle` with no value
confirm_kill    = true                             # prompt y/N before kill unless --yes

[attach]
force_detach    = true                             # tmux attach -d (steal from other clients)
fzf_picker      = true                             # use fzf if present, else numbered prompt

[display]
mem_unit        = "auto"                           # auto | mb | gb
time_format     = "relative"                       # relative | absolute (RFC3339)
show_claude_id  = true                             # include Claude session name/id column
truncate_path   = 40                               # max project path width in table; 0 = no truncate
color           = "auto"                           # auto | always | never

[paths]
claude_projects = "~/.claude/projects"             # override if CLAUDE_CONFIG_DIR set
state_file      = "~/.config/muxc/state.json"
log_file        = ""                               # empty = no logging

[logging]
level           = "warn"                           # debug | info | warn | error
```

**Project-local override** (`<project>/.muxc.toml`):

Only `launch_args`, `name_sessions`, and `claude_bin` may be overridden per-project. Other keys are ignored if present.

**Environment variables** (override config; documented in README):

- `MUXC_CONFIG_DIR` — override `~/.config/muxc/`
- `MUXC_NO_COLOR` — force `display.color = "never"`
- `CLAUDE_CONFIG_DIR` — respected automatically for `paths.claude_projects` if set

## 10. State File Schema

File: `~/.config/muxc/state.json`. Written atomically (write to `.tmp`, fsync, rename).

```json
{
  "version": 1,
  "sessions": {
    "muxc-my145": {
      "project_path": "/home/monzim/code/my145",
      "claude_session_name": "auth-refactor",
      "launch_args": ["--dangerously-skip-permissions"],
      "created_at": "2026-05-27T10:14:22Z",
      "last_attached_at": "2026-05-27T14:33:08Z"
    }
  }
}
```

**Rules:**

- `version` is an integer. On read, if `version > 1`, abort with a clear error. Future versions should add migrations.
- Stale entries (no matching tmux session) are pruned on every `muxc ls` invocation, silently.
- If the file is missing, treat as empty. Do not create it until first write.
- If parsing fails, log a warning, treat as empty, do not crash.
- All timestamps are RFC3339 UTC.

## 11. Command Surface — Detailed Spec

Every command supports `--help`. Every read command supports `--json`. Exit codes per §16.

### 11.1 `muxc ls`

**Synopsis:** `muxc ls [--json] [--all] [--sort <key>] [--filter <pattern>]`

**Flags:**
- `--json` — emit JSON array instead of table
- `--all` — include tmux sessions not matching the `muxc-` prefix (marked as `external` in output)
- `--sort <key>` — `name` (default) | `mem` | `idle` | `created`
- `--filter <pattern>` — glob pattern matched against session name

**Behavior:**

1. Run `tmux list-sessions -F '#{session_name}|#{session_created}|#{session_activity}|#{session_attached}|#{session_id}'`. If tmux not running or no sessions, output empty (table with header, or `[]` JSON).
2. Filter by prefix unless `--all`.
3. For each session:
   - Get pane PID: `tmux list-panes -t <name> -F '#{pane_pid}'` (take first pane).
   - Walk process tree from pane PID (§13).
   - Identify Claude process in tree (§13.3).
   - Sum RSS for Claude process + its descendants.
   - Read `state.json` entry if present.
   - If `display.show_claude_id`, look up the most recent Claude session for this project path in `~/.claude/projects/` (§14).
4. Prune stale state.json entries (sessions in state but not in tmux).
5. Render.

**Table columns (default order):**

```
NAME         PROJECT              CLAUDE     UPTIME   IDLE     MEM      ATTACHED
muxc-my145   ~/code/my145         auth-refactor  2h14m    5m       412 MB   yes
muxc-iar     ~/code/iar-website   8a3c-…     1d3h     2h11m    287 MB   no
```

**JSON output:** array of objects with keys: `name`, `project_path`, `claude_session_name`, `claude_session_id`, `tmux_session_id`, `created_at` (RFC3339), `activity_at`, `idle_seconds`, `uptime_seconds`, `attached` (bool), `pane_pid`, `claude_pid`, `rss_bytes`, `rss_plus_children_bytes`, `is_external` (bool).

**Empty case:** table prints header only. JSON prints `[]`. Exit 0.

### 11.2 `muxc new`

**Synopsis:** `muxc new <project-path> [--name <n>] [--no-claude-name] [--args "..."] [--no-launch]`

**Flags:**
- `--name <n>` — override derived tmux session name (without prefix)
- `--no-claude-name` — do not pass `-n` to claude; let Claude assign a UUID
- `--args "..."` — append extra args to the launch command for this session only
- `--no-launch` — create the tmux session and cd, but do not start Claude (useful for testing)

**Behavior:**

1. Resolve `<project-path>` to absolute. Error if it doesn't exist or isn't a directory.
2. Derive tmux session name:
   - If `--name` given: `<prefix><sanitized-name>`
   - Else: `<prefix><sanitize(basename(path))>`
   - Sanitize: `[^a-zA-Z0-9_-]` → `-`, collapse repeats, trim hyphens, lowercase.
   - Dedup against existing tmux sessions: append `-2`, `-3`, … until unique.
3. Determine launch command:
   - Start with global `defaults.claude_bin` + `defaults.launch_args`.
   - If `<project>/.muxc.toml` exists, overlay its keys.
   - If `--args` given, append.
   - If `name_sessions=true` and `--no-claude-name` not set, append `-n <claude-name>` where `<claude-name>` is the tmux name minus prefix.
4. Create tmux session: `tmux new-session -d -s <name> -c <abs-project-path>`.
5. If not `--no-launch`: `tmux send-keys -t <name> '<full-launch-cmd>' Enter`.
6. Write to state.json.
7. Print:
   ```
   started muxc-my145 in /home/monzim/code/my145
   attach with: muxc attach my145
   ```

**Errors:**
- Project path invalid → exit 2, stderr message.
- tmux session name collision (after dedup attempts > 10) → exit 1.
- tmux command failure → exit 1, propagate tmux stderr.

### 11.3 `muxc attach`

**Synopsis:** `muxc attach [name] [--no-detach]`

**Flags:**
- `--no-detach` — do not pass `-d` to tmux attach (share with other clients)

**Behavior:**

1. If `name` given:
   - Resolve: try `<name>` then `<prefix><name>` as tmux session names. Error if neither exists.
   - Run `tmux attach -t <full-name>` (with `-d` unless `--no-detach`).
2. If no name:
   - List muxc sessions (as in `ls`).
   - If zero: error "no muxc sessions found, create one with `muxc new <path>`". Exit 2.
   - If one: attach to it directly.
   - If many and `fzf` installed and `attach.fzf_picker=true`: pipe `name\tproject\tidle` lines to `fzf` and attach to chosen.
   - Else: numbered prompt:
     ```
     1) muxc-my145    ~/code/my145         idle 5m
     2) muxc-iar      ~/code/iar-website   idle 2h11m
     Select [1-2]:
     ```
3. On successful attach, update `last_attached_at` in state.json **before** exec'ing tmux (since exec replaces the process).

**Implementation note:** Use `syscall.Exec` to replace muxc with tmux, so the user lands directly in tmux without an extra shell layer.

### 11.4 `muxc kill`

**Synopsis:**
```
muxc kill <name>
muxc kill --idle <duration>
muxc kill --all
muxc kill --stale
```

**Flags:**
- `--idle <dur>` — kill sessions idle longer than duration (Go duration syntax: `2h`, `30m`, `1h30m`)
- `--all` — kill every muxc session
- `--stale` — remove state.json entries whose tmux session doesn't exist (no tmux kill needed)
- `--yes` / `-y` — skip confirmation prompt
- `--dry-run` — print what would be killed, do not act

**Behavior:**

1. Build target list per flag mode (exactly one mode must be given; `<name>` is its own mode).
2. If `defaults.confirm_kill=true` and `--yes` not set and target count > 0:
   ```
   Kill 3 sessions?
     muxc-my145    (idle 5h12m, 412 MB)
     muxc-iar      (idle 6h03m, 287 MB)
     muxc-research (idle 4h41m, 198 MB)
   [y/N]:
   ```
   Default is No. Empty input = No.
3. For each target:
   - `tmux kill-session -t <name>`. This SIGHUPs the shell, which SIGTERMs Claude. Claude flushes its transcript on shutdown.
   - Remove entry from state.json.
4. Print summary: `killed 3 sessions, freed ~897 MB (estimated from last ls)`.

**`--stale` special case:** does not call tmux, only cleans state.json. Reports count of removed entries.

### 11.5 `muxc mem`

**Synopsis:** `muxc mem [--json]`

**Behavior:** Same as `muxc ls --sort mem`, but with:
- Bold/colored highlight on the top consumer (table mode only).
- Footer row showing totals:
  ```
  TOTAL: 5 sessions, 1.84 GB RSS (claude+children)
  ```
- JSON mode: same as `ls --json` but adds a top-level wrapper: `{"sessions": [...], "totals": {"count": N, "rss_bytes": ...}}`.

### 11.6 `muxc info`

**Synopsis:** `muxc info <name> [--json] [--transcript-lines <n>]`

**Flags:**
- `--transcript-lines <n>` — tail N entries from the JSONL transcript (default 0, max 100)

**Behavior:**

1. Resolve name (same logic as `attach`).
2. Gather everything `ls` gathers, plus:
   - Full process tree, indented, with PID, command, RSS each.
   - Claude session ID (UUID) and name (if set).
   - Absolute path to the JSONL transcript file in `~/.claude/projects/`.
   - File size and last modified time of transcript.
   - If `--transcript-lines > 0`: tail N entries, pretty-printed (role + first 200 chars of content; skip tool_use/tool_result unless content empty).
3. Render as a structured text block (table-of-tables) or one big JSON object.

**Sample text output:**
```
muxc-my145
  project:   /home/monzim/code/my145
  created:   2026-05-27 10:14:22 (2h14m ago)
  activity:  2026-05-27 12:23:45 (5m idle)
  attached:  yes (1 client)
  tmux id:   $3

claude session:
  name:      auth-refactor
  id:        8a3c1e2f-7d4b-4f1c-9a2e-3b5c6d7e8f9a
  transcript: ~/.claude/projects/-home-monzim-code-my145/8a3c1e2f-….jsonl
  size:      127 KB, modified 5m ago

processes (rss):
  ├─ 12345 zsh                            4 MB
  └─ 12389 node /usr/.../claude/cli.js  328 MB
     ├─ 12410 node mcp-server-fs         42 MB
     ├─ 12422 rg --json …                14 MB
     └─ 12431 node tsserver              28 MB
  total: 416 MB

recent transcript (last 3 entries):
  [user]      can we refactor the auth middleware to use JWT…
  [assistant] I'll start by reading the current middleware…
  [user]      good, now add refresh token support
```

### 11.7 `muxc doctor`

**Synopsis:** `muxc doctor [--json]`

**Checks (each reports OK / WARN / FAIL with a message):**

1. tmux installed (`tmux -V`) and version ≥ 3.0
2. `claude` (per `defaults.claude_bin`) on PATH
3. `~/.claude/projects/` exists and is readable
4. `~/.config/muxc/` exists, writable
5. `config.toml` parses (or is absent, which is fine — defaults apply)
6. `state.json` parses (or is absent)
7. No orphan state entries (sessions in state.json without matching tmux session)
8. fzf installed (informational, not a failure)
9. `/proc` accessible (Linux)
10. Running as a real user (UID > 0); not root unless config explicitly allows

**Exit code:** 0 if all OK or only WARNs; 1 if any FAIL.

### 11.8 `muxc completion`

**Synopsis:** `muxc completion <bash|zsh|fish>`

Emits the completion script to stdout (cobra provides this; just wire it up). Document install instructions in README.

### 11.9 `muxc version`

Prints: `muxc <version> (commit <sha>, built <date>, go <go-version>)`. Version injected at build time via `-ldflags`.

## 12. tmux Integration

### Commands used

All tmux calls go through a single wrapper in `internal/tmux/tmux.go`. Each wraps `exec.Command("tmux", …)` with stdout capture, stderr propagation, and clear error wrapping.

| Operation | Command |
|---|---|
| List sessions | `tmux list-sessions -F '<format>'` |
| List panes | `tmux list-panes -t <session> -F '#{pane_pid}'` |
| New session | `tmux new-session -d -s <name> -c <path>` |
| Send keys | `tmux send-keys -t <name> '<cmd>' Enter` |
| Kill session | `tmux kill-session -t <name>` |
| Attach | `tmux attach -d -t <name>` (via syscall.Exec) |
| Has session | `tmux has-session -t <name>` (exit code only) |
| Version | `tmux -V` |

### Format string for `list-sessions`

```
#{session_name}|#{session_created}|#{session_activity}|#{session_attached}|#{session_id}|#{session_windows}
```

Pipe-delimited. Parse in `internal/tmux/parse.go`. Fields:
- `session_name`: string
- `session_created`: Unix timestamp (int64)
- `session_activity`: Unix timestamp (int64) — last activity, used for idle
- `session_attached`: "0" or "1"+ (number of clients)
- `session_id`: tmux's internal id like `$3`
- `session_windows`: count

### Error handling

- "no server running on …" → tmux not started, treat as zero sessions, exit 0.
- "session not found" on a targeted operation → exit 2 with clear message.
- Other tmux errors → exit 1, propagate stderr.

## 13. Process Tree & Memory

### 13.1 Walking the tree

For each tmux session, start from `pane_pid`. BFS through `/proc`:

```
for each pid in /proc/*:
  read /proc/<pid>/stat → field 4 is PPID
  build map[ppid] → []childpid
```

Then DFS from `pane_pid`. Cache the full PPID map once per `muxc` invocation (rebuilding is cheap but doing it N times for N sessions is wasteful).

### 13.2 Reading RSS

`/proc/<pid>/status` line `VmRSS:` gives KB. Multiply by 1024 for bytes.

If `/proc/<pid>/status` is missing (process died between listing and reading), skip that PID. Do not error.

For a more accurate "memory" number, use PSS from `/proc/<pid>/smaps_rollup` (`Pss:` line) — accounts for shared memory. Optional; v1 ships RSS, v2 may add `--pss` flag. **Decision for v1: RSS only.** Document the limitation in README.

### 13.3 Identifying the Claude process

Walk descendants of the pane PID. For each, read `/proc/<pid>/cmdline` (null-separated args). A process is "Claude" if **any** of:

1. `argv[0]` basename is `claude`
2. `argv[0]` basename is `node` AND any `argv[i]` contains the substring `/claude/` or ends with `claude.js` / `cli.js` in a path that contains `claude`
3. Process executable (`/proc/<pid>/exe` symlink target) basename matches the configured `claude_bin`

The first match wins. If multiple processes match, pick the one with the lowest PID (closest to pane PID by spawn order, usually).

If no match: report `claude_pid: 0` and `rss_bytes: 0`, but still show pane PID and total tree memory under a separate "shell+tree" field.

### 13.4 Sum for `rss_plus_children_bytes`

Once Claude PID is identified, walk *its* descendants (not pane PID's) and sum RSS of Claude + all descendants.

## 14. Claude Sessions Directory

`~/.claude/projects/` contains one directory per project. The directory name is the project path with `/` replaced by `-` and a leading `-` (e.g., `/home/monzim/code/my145` → `-home-monzim-code-my145`).

Each directory contains `<session-id>.jsonl` files. Each file is a JSONL transcript.

### 14.1 Finding sessions for a project

Given a project path, encode it (replace `/` with `-`, prefix `-`), look in `~/.claude/projects/<encoded>/`, list `*.jsonl`, sort by mtime desc. The most recent is the "current" session for that project.

### 14.2 Extracting session name

Each JSONL line is a JSON object. The first few lines typically contain metadata. Look for an object with `"type": "summary"` or a field named `sessionName` / `name`. **Exact schema may vary across Claude versions — do not hardcode.** Implementation should:

1. Read up to the first 50 lines of the file.
2. Look for known name-bearing fields in order: `name`, `sessionName`, `session_name`, `title`.
3. If none found, fall back to displaying the session ID (UUID portion of the filename) truncated to 8 chars + `…`.

### 14.3 Robustness

- File may be actively written; do not lock, do not assume completeness.
- Corrupt JSON lines: skip silently, continue scanning.
- Missing project directory: return empty list, no error.
- Permission denied on `~/.claude/`: warn once, continue without Claude integration.

## 15. Output Formats

### 15.1 Tables

`tablewriter` defaults with these tweaks:
- No row separators.
- Headers in bold (when color enabled).
- Right-align memory column.
- Truncate project path per `display.truncate_path`.
- "yes" rendered green, "no" rendered dim (when color enabled).

### 15.2 JSON

All `--json` outputs are valid JSON, never NDJSON. Top-level is either an array (ls, mem.sessions, doctor.checks) or an object (info, mem with totals wrapper).

All timestamps RFC3339 UTC. All durations in seconds (integers). All sizes in bytes (integers). Let the caller format.

### 15.3 Color

Use `display.color`:
- `auto` — color if stdout is a TTY AND `NO_COLOR` env var not set AND `MUXC_NO_COLOR` not set
- `always` — color
- `never` — no color

Implement via simple ANSI escapes or `fatih/color` if added to deps.

## 16. Exit Codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | General failure (tmux error, IO error, etc.) |
| 2 | Bad usage (invalid flags, missing required arg, target not found) |
| 3 | Config error (parse failure, invalid value) |
| 4 | Precondition failure (`doctor` would flag this) |
| 130 | Interrupted (SIGINT) |

## 17. Logging

Use stdlib `log/slog`.

- Default: WARN to stderr.
- If `paths.log_file` set: append all logs at configured level.
- Each log line includes: time (RFC3339), level, command, message, optional key-value attrs.

No log rotation. User can `logrotate` externally if they care.

## 18. Testing Strategy

### Unit tests

- `internal/tmux/parse.go` — parse representative `list-sessions` outputs including edge cases (empty, single session, special chars in names).
- `internal/proc/tree.go` — given a synthetic /proc fixture directory, walk and identify correctly.
- `internal/proc/memory.go` — parse VmRSS from sample `/proc/<pid>/status` content.
- `internal/proc/claude.go` — identify Claude process from sample cmdlines (node-based, direct binary, edge cases).
- `internal/claude/transcript.go` — extract session names from sample JSONL files.
- `internal/state/state.go` — round-trip, atomic write, corrupted file recovery, version mismatch.
- `internal/config/config.go` — defaults, project-local merge, env override, validation errors.

### Integration tests

Build a test harness that:
1. Starts a temporary tmux server on a custom socket (`tmux -L muxc-test`).
2. Creates sessions via the muxc binary against that socket (via `TMUX_SOCKET` env var — needs to be added to config or wired through).
3. Asserts ls/info/kill behavior.
4. Tears down the test tmux server.

Mark as `//go:build integration` and run separately. Require tmux installed on CI.

### What NOT to test

- Real Claude Code launches. Mock via a fake `claude` script in test PATH that sleeps and writes a fake transcript.
- Network calls. There are none.

### Coverage target

≥ 75% for `internal/*`. Don't chase 100%; some main.go and command glue is integration-tested or not worth unit-testing.

## 19. Error Messages

Every error printed to the user follows this shape:

```
muxc: <verb that failed>: <reason>
  <optional hint or next action>
```

Examples:
```
muxc: cannot resolve project path: stat /home/monzim/code/missing: no such file or directory
  pass an existing directory, e.g. `muxc new ~/code/my145`

muxc: tmux session "muxc-my145" not found
  list available sessions with `muxc ls`

muxc: config error: idle_threshold "forever" is not a valid duration
  use Go duration syntax, e.g. "2h", "30m", "1h30m"
```

No stack traces unless `MUXC_DEBUG=1` is set.

## 20. README Requirements

The README must include:

1. **One-paragraph what/why.**
2. **Install:** `go install`, `scp` from release, build-from-source.
3. **Quickstart:** 5 commands that show the full lifecycle.
4. **Configuration:** link to config schema, show a minimal example.
5. **Commands:** one-line description each, link to `--help` for details.
6. **Limitations:** Linux-only in v1, tmux required, no headless mode, RSS not PSS, etc.
7. **How it works:** brief — tmux for runtime, `~/.claude/projects/` for persistence, no daemon.
8. **FAQ:** "will killing a session lose my conversation?" (no, Claude saves continuously), "can I use this with screen?" (no, tmux only), etc.
9. **License (MIT).**

Keep under 400 lines.

## 21. Build & Release

### Makefile targets

```
make build       # go build -o bin/muxc ./cmd/muxc
make install     # cp bin/muxc ~/.local/bin/muxc
make test        # go test ./...
make test-int    # go test -tags integration ./...
make lint        # golangci-lint run
make clean       # rm -rf bin/
make release     # goreleaser (v2 stretch)
```

### Build flags

```
go build \
  -ldflags "-s -w \
    -X main.Version=$(VERSION) \
    -X main.Commit=$(shell git rev-parse --short HEAD) \
    -X main.Date=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -trimpath \
  -o bin/muxc \
  ./cmd/muxc
```

### Binary size target

< 10 MB stripped. Verify with `ls -lh bin/muxc` after each release.

## 22. Implementation Order (for the agent)

Build in this order. Each step is a self-contained piece that should compile, test, and be committable.

### Step 1: Skeleton

- `go.mod`, `cmd/muxc/main.go` with cobra root command, `internal/config/` with TOML loading and defaults, `muxc version`, `muxc doctor` (minimal — just tmux check and config parse).
- Tests: config defaults, config merge.
- Verify: `muxc version` prints, `muxc doctor` reports tmux status.

### Step 2: tmux layer + `muxc ls`

- `internal/tmux/tmux.go`, `internal/tmux/parse.go`.
- `internal/state/state.go` (read/write/prune).
- `internal/render/table.go`, `internal/render/json.go`.
- `cli/ls.go`.
- Tests: parse fixtures, state round-trip.
- Verify: create a tmux session manually named `muxc-test`, `muxc ls` shows it.

### Step 3: `muxc new` + `muxc kill`

- `cli/new.go`, `cli/kill.go`.
- Project-local config overlay in `internal/config/`.
- Tests: name sanitization, dedup logic.
- Verify: full round-trip create → ls → kill on a real Claude binary (or fake script).

### Step 4: Memory & process tree

- `internal/proc/tree.go`, `internal/proc/memory.go`, `internal/proc/claude.go`.
- Wire into `ls`.
- Add `cli/mem.go`.
- Tests: fixture-based procfs walk, Claude identification across cmdline variants.
- Verify: `muxc mem` shows actual memory of running sessions.

### Step 5: `muxc attach` + `muxc info`

- `cli/attach.go` with fzf detection and numbered prompt fallback.
- `internal/claude/sessions.go`, `internal/claude/transcript.go`.
- `cli/info.go`.
- Tests: transcript parsing across sample JSONL.
- Verify: `muxc attach` lands you in tmux; `muxc info` shows the right transcript path.

### Step 6: Polish

- `cli/kill.go` `--idle` filter, `--stale`, `--dry-run`.
- `cli/doctor.go` full check list.
- `cli/completion.go`.
- README.
- `.goreleaser.yml` (optional).
- Tag v1.0.0.

## 23. Non-Goals That Implementers Will Be Tempted to Add

Reject these in code review:

- "What if we add a `muxc exec <name> <cmd>` to send a prompt to a Claude session?" — No. That's tmux send-keys with extra steps; user can do it directly.
- "Let's add a `muxc snapshot` to back up all transcripts." — No. `cp -r ~/.claude/projects ~/backup` does it.
- "We could detect when Claude is mid-response and refuse to kill." — No. Use tmux activity; trust the user.
- "Let's auto-launch `muxc new` on `cd` via a shell hook." — No. Out of scope; user can write their own zsh function.
- "Should we support remote tmux via SSH config?" — No. SSH to the box.
- "Can we add a `--detach-after <duration>` to auto-kill new sessions?" — No. Use cron + `muxc kill --idle`.

## 24. Acceptance Criteria for v1.0.0

The release is ready when **all** of these are true:

1. Single static binary, < 10 MB, builds on Linux amd64 and arm64.
2. `muxc doctor` passes on a clean Ubuntu 24.04 install with tmux 3.4 and Claude Code installed.
3. The full lifecycle works end-to-end: `new` → `ls` shows it → `attach` enters it → `kill` removes it → `ls` shows it gone.
4. `muxc ls --json` output parses with `jq` without errors.
5. `muxc kill --idle 1h --dry-run` correctly identifies idle sessions without killing.
6. `muxc info <name>` shows the correct transcript path in `~/.claude/projects/`.
7. Memory readings match `top`/`htop` for the Claude process within ±5%.
8. Killing a tmux session preserves the Claude transcript (verify by `claude --resume <name>` after `muxc kill`).
9. README covers install, quickstart, all commands, limitations.
10. Test coverage ≥ 75% on `internal/*`.
11. `golangci-lint run` passes with zero issues.
12. Binary works when `~/.config/muxc/` does not exist (creates on first write).
13. Binary works when `state.json` is corrupted (warns, treats as empty).
14. Binary works when tmux is not running (reports no sessions, exits 0).

## 25. Open Questions (to resolve during build, not blocking)

These can be decided by the implementer with judgment; they don't change the spec.

1. **Color library** — `fatih/color` vs raw ANSI vs `charmbracelet/lipgloss`. Pick one, justify in PR.
2. **fzf invocation details** — what columns to pipe, how to format. Try `name\tproject\tidle` first.
3. **Numbered prompt UI** — readline, bufio.Scanner, or `survey` library. Bufio is simplest.
4. **Truncation strategy** for long project paths — left-truncate with `…` prefix vs middle-truncate. Left-truncate preferred.
5. **Whether to include MCP server names** in the `info` process tree — yes if cmdline reveals them clearly, no parsing of MCP config files.

---

**End of spec.**

Hand this document to Claude Code. It should be sufficient to build v1.0.0 without further clarification. If the implementer finds genuine ambiguity, prefer the simpler interpretation and document the choice in a code comment referencing the section number.
