# Changelog

All notable changes to muxc will be documented in this file.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Open-source readiness scaffolding: GitHub Actions CI (lint + unit tests + cross-build snapshot), release workflow on `v*` tags via GoReleaser, Dependabot for Go modules and Actions, issue templates (bug/feature/question), PR template, CODEOWNERS.
- `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1) and `SECURITY.md` (private disclosure policy + threat model).
- `docs/MANUAL.md`: full user reference covering installation, every subcommand and flag, TUI keybindings, configuration schema, exit codes, and troubleshooting.
- `.editorconfig` for cross-editor formatting consistency.
- Linux `.deb` and `.rpm` packages and a Homebrew tap formula produced by GoReleaser alongside the existing tar.gz archives.

### Changed

- README: added status / release / Go version / license badges, a "Download a release binary" install path, and a Documentation pointer block.

## [1.0.0] - 2026-05-28

First public release. Implements the full `spec.md` v1.0 contract.

### Added

- Single static Go binary (`muxc`) that wraps `tmux` to manage Claude Code sessions on a single host.
- Subcommands:
  - `muxc ls` — list sessions with uptime, idle time, memory; `--json`, `--all`, sort flags.
  - `muxc new <path>` — launch Claude in a fresh tmux session rooted at the given project path.
  - `muxc attach [name]` — `syscall.Exec` handoff to `tmux attach`; updates `last_attached_at` before exec.
  - `muxc kill [name]` — kill one or more sessions with `--idle`, `--all`, `--stale`, `--dry-run`, `--yes`.
  - `muxc mem` — memory-sorted view with a total-RSS footer.
  - `muxc info <name>` — process tree, per-process memory, transcript path and tail.
  - `muxc doctor` — 10 environment checks reporting OK / WARN / FAIL.
  - `muxc completion <shell>` — bash / zsh / fish completion scripts.
  - `muxc version` — version, commit, build date injected via ldflags.
- TOML configuration at `~/.config/muxc/config.toml` with project-local `.muxc.toml` override.
- Bubble Tea TUI launched when invoked with no subcommand: live sessions table, auto-refresh, `j/k` navigation, dedicated new/kill/info/doctor screens, modal help.
- Plain-text REPL fallback when stdin/stdout is not a TTY, or when `MUXC_NO_TUI=1` is set.
- External Claude session detection: tmux sessions not prefixed `muxc-` that contain a detected Claude process appear in `ls` (suffix `*` in CLI, `★` prefix in TUI). `kill --all` and `kill --idle` never touch them.
- Atomic state writes to `~/.config/muxc/state.json` with stale-entry pruning on every `ls`.
- `--json` output on every read command (`ls`, `mem`, `info`, `doctor`) with stable shapes.
- Spec-compliant exit codes (0/1/2/3/4/130) and `slog`-based warnings to stderr.

### Notes

- Linux only in v1 (requires `/proc`). macOS support is planned for v2.
- Requires tmux ≥ 3.0.
- Zero network I/O; `~/.claude/` is treated as read-only.

[Unreleased]: https://github.com/monzim/muxc/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/monzim/muxc/releases/tag/v1.0.0
