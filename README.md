# muxc

[![CI](https://github.com/monzim/muxc/actions/workflows/ci.yml/badge.svg)](https://github.com/monzim/muxc/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/monzim/muxc?logo=github)](https://github.com/monzim/muxc/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/monzim/muxc.svg)](https://pkg.go.dev/github.com/monzim/muxc)
[![Go Report Card](https://goreportcard.com/badge/github.com/monzim/muxc)](https://goreportcard.com/report/github.com/monzim/muxc)
[![Go version](https://img.shields.io/github/go-mod/go-version/monzim/muxc)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Manage Claude Code sessions running inside tmux.

`muxc` is a single static Go binary that lists, creates, attaches to, and kills
tmux sessions running Claude Code. It tracks per-session memory by walking the
Linux process tree via procfs, reads Claude's existing conversation transcripts
from `~/.claude/projects/` (never writes there), and reconstructs all state on
every invocation — no daemon, no background process, no API calls.

---

## Install

> Requires `tmux ≥ 3.0`. Linux for full memory accounting; macOS works for
> session management (memory columns are empty). See
> [`docs/MANUAL.md`](docs/MANUAL.md#installation) for the full installation guide.

**Download a release binary (recommended)**

```bash
VERSION=1.0.0
ARCH=amd64   # or arm64
curl -fL -o muxc.tar.gz \
  "https://github.com/monzim/muxc/releases/download/v${VERSION}/muxc_${VERSION}_linux_${ARCH}.tar.gz"
curl -fL "https://github.com/monzim/muxc/releases/download/v${VERSION}/muxc_${VERSION}_checksums.txt" \
  | grep "muxc_${VERSION}_linux_${ARCH}.tar.gz" | sha256sum --check
tar -xzf muxc.tar.gz muxc
install -m 0755 muxc ~/.local/bin/muxc
```

**Distribution packages** (Debian/Ubuntu and Fedora/RHEL)

```bash
sudo dpkg -i muxc_1.0.0_linux_amd64.deb       # or
sudo rpm  -i muxc_1.0.0_linux_amd64.rpm
```

**Homebrew (macOS / Linuxbrew)**

```bash
brew install monzim/tap/muxc
```

**Via `go install`**

```
go install github.com/monzim/muxc/cmd/muxc@latest
```

**Build from source**

```
git clone https://github.com/monzim/muxc.git
cd muxc
make build
make install        # copies bin/muxc to ~/.local/bin/muxc
```

**Copy a static binary onto a remote box** (single binary, no runtime deps beyond tmux)

```
scp muxc remote:~/.local/bin/
```

---

## Documentation

- [`docs/MANUAL.md`](docs/MANUAL.md) — full user manual: every command, flag,
  config key, exit code, JSON schema, and troubleshooting guide.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — how to set up a dev environment,
  the test loop, and how to submit a PR.
- [`spec.md`](spec.md) — the implementation-ready design spec; cite sections
  in code comments and PR descriptions.
- [`CHANGELOG.md`](CHANGELOG.md) — release history.
- [`SECURITY.md`](SECURITY.md) — security policy and private disclosure.

---

## Quickstart

```
muxc new ~/code/my-project          # spin up a Claude session
muxc ls                             # see all sessions with memory
muxc attach my-project              # jump into the session
# ... do work, detach with Ctrl-b d ...
muxc kill my-project --yes          # tear it down
```

---

## Interactive mode

Run `muxc` with no arguments to drop into the Bubble Tea TUI: a live
sessions table with cursor navigation, an auto-refreshing display, and
dedicated screens for `new`, `kill`, `info`, and `doctor`. The sessions
screen is the default landing view.

Keyboard:

| Key | Action |
|---|---|
| `↑`/`↓` or `j`/`k` | Move cursor in the sessions table |
| `enter` or `i` | Open the **info** screen for the selected session |
| `a` | **Attach** to the selected session (one-way: detaching from tmux returns to your shell) |
| `n` | Open the **new session** form |
| `K` | Open the **kill** picker (capital K so it doesn't clash with `k` navigation) |
| `s` | Cycle sort key (name → mem → idle → created → name) |
| `r` | Force an immediate refresh |
| `d` | Open the **doctor** screen |
| `?` | Toggle help overlay |
| `esc` | Back to the sessions screen from any sub-screen |
| `q` or `Ctrl-C` | Quit |

The sessions table auto-refreshes every 2 seconds. External tmux sessions
running Claude appear with a `★` prefix on their name.

**Non-TTY fallback.** When stdin or stdout isn't a real terminal (CI, piped
input, dumb terminal), `muxc` falls back to a plain-text numbered REPL so
scripts and tests keep working. Force the fallback with `MUXC_NO_TUI=1`.

---

## External Claude sessions

`muxc ls` surfaces tmux sessions you didn't create through `muxc new`, as long
as they contain a detected Claude process. Those rows are marked with a `*`
suffix in the table and `"is_external": true` in JSON output. Pure tmux
sessions without Claude stay hidden unless `--all` is passed.

`muxc kill --all` and `muxc kill --idle` never touch external sessions — they
operate only on sessions whose names start with the configured prefix
(default `muxc-`). Kill external sessions with `tmux kill-session -t <name>`.

---

## Commands

| Command | Description |
|---|---|
| `muxc ls` | List all muxc sessions with uptime, idle time, and memory |
| `muxc new <path>` | Create a tmux session and launch Claude in the given directory |
| `muxc attach [name]` | Attach to a session; uses fzf picker or numbered prompt if name omitted |
| `muxc kill <name>` | Kill a session; supports `--idle <dur>`, `--all`, `--stale`, `--dry-run` |
| `muxc mem` | Like `ls`, sorted by memory, with a total RSS footer |
| `muxc info <name>` | Detailed view: process tree, memory per process, transcript path and tail |
| `muxc doctor` | Run 10 environment checks and report OK / WARN / FAIL |
| `muxc completion <bash\|zsh\|fish>` | Emit shell completion script to stdout |
| `muxc version` | Print version, commit, and build date |

Run `muxc <command> --help` for full flag documentation.

---

## Configuration

Global config file: `~/.config/muxc/config.toml`. All keys are optional;
defaults apply for anything absent.

```toml
[defaults]
prefix      = "muxc-"
claude_bin  = "claude"
launch_args = ["--dangerously-skip-permissions"]
```

Full schema with all keys is documented in `spec.md` §9.

**Project-local overrides** — place a `.muxc.toml` at the root of a project to
override `launch_args`, `name_sessions`, or `claude_bin` for that project only.
Other keys are ignored. The file is read once at `muxc new` time.

**Environment variables**

| Variable | Effect |
|---|---|
| `MUXC_CONFIG_DIR` | Override the `~/.config/muxc/` directory |
| `MUXC_NO_COLOR` | Force color off (same as `display.color = "never"`) |
| `CLAUDE_CONFIG_DIR` | Respected automatically for `paths.claude_projects` |
| `MUXC_TMUX_SOCKET` | Target a private tmux socket via `tmux -L <name>` (primarily for tests and sandboxed setups) |
| `MUXC_NO_TUI` | Force the plain-text REPL even on a real terminal (skip Bubble Tea) |

---

## How it works

tmux is the source of truth for whether a session is alive. On every
invocation, `muxc` queries `tmux list-sessions`, reads `/proc` to walk the
process tree from each pane PID, and looks up the corresponding project
directory in `~/.claude/projects/` to find the active conversation transcript.

There is no daemon, no socket, and no background process. If `~/.config/muxc/state.json`
is deleted or corrupted, `muxc` recovers automatically — tmux still knows what
is running. The state file only holds ancillary metadata (launch args, timestamps)
that cannot be reconstructed from tmux alone.

---

## Limitations

- **Linux only in v1.** Memory measurement requires `/proc` (procfs). macOS
  support is planned for v2.
- **tmux >= 3.0 required.** The format strings used by `muxc` rely on fields
  introduced in tmux 3.0.
- **RSS, not PSS.** Memory is read from `/proc/<pid>/status` (`VmRSS`). This
  overcounts shared pages. PSS from `/proc/<pid>/smaps_rollup` may be added
  behind a `--pss` flag in v2.
- **No headless mode.** `muxc` does not orchestrate `claude -p`. It manages
  interactive sessions only.
- **No remote-host management.** SSH to the box and run `muxc` there.
  Remote management adds transport and state-sync complexity that is out of
  scope.

---

## JSON output

Every read command (`ls`, `mem`, `info`, `doctor`) accepts `--json` and emits
valid JSON to stdout. Timestamps are RFC 3339, durations are integers in
seconds, sizes are integers in bytes.

```
muxc ls --json | jq '.[] | select(.idle_seconds > 3600) | .name'
```

Pipe to `watch` for a live view:

```
watch -n 5 'muxc ls'
```

---

## FAQ

**Will killing a session lose my conversation?**

No. Claude Code flushes its transcript continuously to
`~/.claude/projects/<encoded-path>/<session-id>.jsonl`. After `muxc kill`,
resume from the same directory with `claude --resume <name>` and the full
conversation is available.

**Can I use this with screen, zellij, or wezterm?**

No. `muxc` is tmux-only. The implementation depends on tmux's session model,
format strings, and `send-keys` interface. Forks for other multiplexers are
welcome.

**Does muxc make any API calls or send telemetry?**

No. `muxc` has zero network I/O. It shells out only to `tmux`, reads procfs,
and reads `~/.claude/projects/`. No tokens, no telemetry, no auto-update.

**Why RSS instead of PSS?**

RSS requires a single line read from `/proc/<pid>/status`. PSS requires
parsing `/proc/<pid>/smaps_rollup`, which is more complex and slower across
many processes. RSS is a reasonable approximation for identifying memory-heavy
sessions. PSS may appear behind a flag in v2.

**What if `~/.config/muxc/state.json` is corrupted?**

`muxc` logs a warning to stderr and continues with empty state. tmux is the
source of truth for which sessions are alive. The only data lost is ancillary
metadata (original launch args, `last_attached_at` timestamps). It is
reconstructed on the next `muxc new`.

**How do I install shell completions?**

```
# bash
muxc completion bash > /etc/bash_completion.d/muxc

# zsh
muxc completion zsh > "${fpath[1]}/_muxc"

# fish
muxc completion fish > ~/.config/fish/completions/muxc.fish
```

---

## Development

```
make build       # compile bin/muxc
make test        # unit tests
make test-int    # integration tests (requires tmux on PATH)
make lint        # golangci-lint run
make install     # install to ~/.local/bin/muxc
make clean       # remove bin/
```

Integration tests start a temporary tmux server on a private socket and
exercise the full command surface against it. They are tagged
`//go:build integration` and skipped by `make test`.

The full specification is in `spec.md`. Non-goals are listed in §3 and §23 —
please read them before proposing new features or opening a pull request.

---

## License

MIT. See [LICENSE](LICENSE).
