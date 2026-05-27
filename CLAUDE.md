# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository status

This repo is **spec-first and pre-implementation**. The only source file is `spec.md`, which is the authoritative, implementation-ready design document for `muxc` — a Go CLI that manages Claude Code sessions running inside tmux.

When asked to "build", "implement", or "continue" work here, treat `spec.md` as the contract. Section numbers (§N) are referenced throughout this file and should be cited in code comments when an implementation choice is non-obvious. Build in the order laid out in §22 (Skeleton → tmux+ls → new+kill → memory → attach+info → polish).

## What muxc is, in one paragraph

A single static Go binary that wraps `tmux` to manage long-running Claude Code sessions on a headless box: list them with memory/idle stats, create new ones rooted at a project path, attach, and kill idle ones. It is a thin layer — tmux owns liveness, Claude owns its transcripts (`~/.claude/projects/*.jsonl`), and muxc only writes to `~/.config/muxc/`.

## Non-negotiable design constraints (from spec §2)

These override convenience. Reject diffs that violate them, even partially:

- **tmux is the source of truth for liveness.** Never infer "is this running" from `state.json`; always ask tmux.
- **`~/.claude/` is read-only.** muxc reads transcripts and project dirs there; it must never write, lock, or modify anything under it.
- **No daemon, no socket, no background process.** Every command is a fresh one-shot that reconstructs state from tmux + procfs + `~/.claude/projects/`. `state.json` is a side-table for metadata tmux doesn't track (launch args, timestamps) — losing it must not break functionality.
- **No Claude API calls and no `claude -p` orchestration.** Zero network I/O. Interactive sessions only.
- **Pure Go, no CGo.** Static binary, target < 10 MB stripped.
- **`--json` on every read command.** Design the output struct first, table rendering second.
- **All Claude CLI flags live in config.** muxc must not hardcode knowledge of Claude's flag semantics — `defaults.launch_args` is opaque to muxc.

§3 lists features that have been considered and rejected (daemon, auto-restart, remote management, headless mode, multi-user, screen/zellij support, etc.). Don't reintroduce them. §23 lists tempting additions to reject in review.

## Planned source layout (spec §8)

```
cmd/muxc/main.go              # cobra entry point
internal/cli/                 # one file per subcommand (ls, new, attach, kill, mem, info, doctor, completion)
internal/config/              # TOML load + defaults + project-local .muxc.toml merge
internal/state/               # ~/.config/muxc/state.json read/write/prune (atomic writes)
internal/tmux/                # exec.Command wrappers + -F format parsing
internal/proc/                # /proc tree walk, VmRSS reader, Claude-process identification
internal/claude/              # read-only access to ~/.claude/projects/ JSONL transcripts
internal/render/              # table + json output, humanize helpers
internal/sysinfo/             # tmux version check, OS detection
```

Recommended deps (§6): `spf13/cobra`, `spf13/viper` or `BurntSushi/toml`, `olekukonko/tablewriter`, `dustin/go-humanize`. Substitutions are fine if justified.

## Build & test commands (planned Makefile, spec §21)

```
make build       # go build -o bin/muxc ./cmd/muxc with -ldflags injecting Version/Commit/Date
make install     # cp bin/muxc ~/.local/bin/muxc
make test        # go test ./...
make test-int    # go test -tags integration ./...   (requires tmux on the host)
make lint        # golangci-lint run
```

Integration tests must run against a dedicated tmux socket (`tmux -L muxc-test`) so they don't touch the user's real sessions. Mark integration files with `//go:build integration`.

Run a single test: `go test ./internal/tmux -run TestParseListSessions`. Run a single integration test: `go test -tags integration ./internal/cli -run TestKillIdle`.

Mock Claude in tests with a fake `claude` shell script in test PATH — never invoke real Claude Code from automated tests (§18).

## Architecture invariants worth knowing before editing

- **One tmux wrapper.** Every `tmux` invocation must go through `internal/tmux/tmux.go`. No `exec.Command("tmux", …)` scattered across CLI files. The wrapper handles error classification: "no server running" → empty result + exit 0; "session not found" on a targeted op → exit 2; other failures → exit 1 with propagated stderr (§12).
- **One `/proc` walk per invocation.** Build the PPID map once at the top of commands that need process trees; don't re-scan `/proc` per session (§13.1).
- **Claude process identification is heuristic** (§13.3): match on `argv[0]` basename `claude`, or `node` with a `claude`-containing path argument, or `/proc/<pid>/exe` matching `claude_bin`. First match wins; lowest PID breaks ties. If no match, report `claude_pid: 0` and surface tree memory under a separate "shell+tree" field rather than failing.
- **Transcript schema is not stable across Claude versions** (§14.2). Probe the first ~50 lines of each JSONL for `name` / `sessionName` / `session_name` / `title`; fall back to the truncated UUID. Never hardcode the schema.
- **`syscall.Exec` for attach** (§11.3): `muxc attach` must replace its own process with `tmux attach` so the user lands directly in tmux. Update `last_attached_at` in `state.json` *before* the exec, since exec doesn't return.
- **Atomic state writes** (§10): write to `state.json.tmp`, fsync, rename. Stale entries (in state but not in tmux) are pruned silently on every `muxc ls`.
- **Exit codes are part of the contract** (§16): 0 success, 1 general failure, 2 bad usage / target not found, 3 config error, 4 precondition failure, 130 SIGINT. Tests should assert these.

## v1.0.0 acceptance gates (spec §24)

Before shipping, all 14 criteria in §24 must pass. The load-bearing ones to keep in mind while implementing:

- Binary < 10 MB stripped, builds on linux/amd64 and linux/arm64.
- Full lifecycle works end-to-end against a real Claude binary: `new` → `ls` → `attach` → `kill` → `ls` shows it gone.
- `muxc kill` preserves the Claude transcript (verifiable by `claude --resume <name>` after kill).
- Binary still works when `~/.config/muxc/` is missing, `state.json` is corrupted, or tmux isn't running.
- Memory readings match `top`/`htop` within ±5% for the Claude process.
- ≥ 75% coverage on `internal/*`, `golangci-lint` clean.

## When the spec is ambiguous

§25 lists known open questions (color library choice, fzf column format, prompt UI library, path truncation strategy, MCP-name surfacing in `info`). Pick the simpler interpretation, document the choice in a code comment that cites the spec section, and move on — do not block on clarification.
