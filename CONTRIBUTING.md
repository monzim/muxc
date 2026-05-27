# Contributing to muxc

Thanks for your interest in helping muxc become a better tmux/Claude session
manager. This document covers what you need to know to land a PR.

If you're skimming for the essentials:

```bash
git clone https://github.com/monzim/muxc.git && cd muxc
make build && bin/muxc doctor   # confirm your env is set up
make test                        # unit suite
make test-int                    # integration suite (needs tmux ≥ 3.0)
gofmt -w . && go vet ./...
```

---

## Quick orientation

The project's contract is `spec.md` — a 813-line implementation-ready design
document. The big-picture invariants (no daemon, no network, tmux as source
of truth, `~/.claude/` is read-only) live there.

The `CLAUDE.md` file at the repo root carries developer-facing notes and
documents **post-v1.0 additions** that deliberately diverge from the spec
(external Claude detection, the TUI, etc.). Read it before changing
architecture — it explains *why* each deviation exists so they don't get
removed as scope creep.

---

## Development environment

**Requirements:**

- Go 1.22+ (development host uses 1.24.x)
- tmux ≥ 3.0
- Linux for full procfs support; macOS works but memory readings are empty
- `golangci-lint` (optional but recommended; CI runs it)
- `jq` (optional; integration tests skip cleanly without it)

**Get the source:**

```bash
git clone https://github.com/monzim/muxc.git
cd muxc
go mod download
make build
```

**Run the binary you just built:**

```bash
bin/muxc doctor          # should report 10 OK / WARN checks
bin/muxc                  # opens the TUI (or REPL fallback if not a TTY)
MUXC_NO_TUI=1 bin/muxc    # force the plain-text REPL
```

---

## The dev loop

| What | Command |
|---|---|
| Unit tests | `make test` |
| Integration tests (real tmux on a private socket) | `make test-int` |
| Race detector | `go test -race ./...` |
| Format | `gofmt -w .` |
| Static analysis | `go vet ./...` |
| Lint | `golangci-lint run` |
| Build (with version ldflags) | `make build` |
| Install to `~/.local/bin/` | `make install` |
| Coverage (unit) | `go test -cover ./internal/...` |

CI runs `make test`, `gofmt -l .`, `go vet ./...`, `golangci-lint run`, and a
`goreleaser release --snapshot` cross-build check on every PR — keep them
green locally and CI stays happy. Integration tests (`make test-int`) are
run by contributors locally but not in CI, so they don't gate merges; please
still run them before pushing changes that touch `internal/tmux`,
`internal/proc`, or `internal/cli`.

---

## Code style

We follow **idiomatic Go**, plus a few project conventions:

- **Comments explain WHY, not WHAT.** The code already says what; comments
  exist to capture non-obvious constraints, design decisions, or spec
  references. See `CLAUDE.md` for the full guidance.
- **No narrative comments** (`// Now we check the foo to bar the baz`).
  Cite spec sections (`spec §13.3`) when behaviour is non-obvious.
- **Errors propagate.** `%w` wrap. No `panic` in non-test code.
- **Tests use real fixtures.** `internal/proc/testdata/` has a fake `/proc`,
  `internal/claude/testdata/` has fake JSONL transcripts. New tests should
  add to these rather than mock interfaces unless the interface already
  exists for the production code's own reasons.
- **Integration tests run on a private tmux socket** (`muxc-int`) so they
  never touch the user's real sessions. Use the `setupTestEnv` helper.

---

## Architecture overview

```
cmd/muxc/                  entry point, ldflags injection, signal handling
internal/cli/              cobra commands (ls, new, attach, kill, …), REPL fallback
internal/tui/              Bubble Tea TUI (sessions, info, kill, new, doctor)
internal/session/          Gather() + SessionRow + ExternalMode
internal/doctor/           the 10 doctor checks
internal/tmux/             single point of contact for tmux subprocess calls
internal/proc/             /proc walker, Claude-process identification, RSS
internal/claude/           read-only access to ~/.claude/projects/ JSONLs
internal/config/           TOML loader + defaults
internal/state/            ~/.config/muxc/state.json read/write
internal/render/           table + JSON + humanize helpers
internal/sysinfo/          OS / tmux version detection
```

The `cli` and `tui` packages both depend on `session` and `doctor` so neither
package needs to import the other. Don't reintroduce the cycle.

---

## Commit messages

Match the existing history style: a short prefix identifies the affected
area, then a lowercase imperative summary.

```
tui: full-width selection bar on kill picker and doctor
cli: add --filter glob support to ls
session: extract Row from cli package
ci: pin Go toolchain to 1.24.2
docs: clarify external Claude semantics
```

Prefixes you'll see: `tui`, `cli`, `session`, `doctor`, `proc`, `claude`,
`tmux`, `state`, `config`, `render`, `ci`, `docs`, `feat`, `fix`, `chore`.

Body lines should explain *why* and link to spec sections or issue numbers
when relevant. Keep subject lines ≤ 72 characters; body lines ≤ 80.

---

## Submitting a PR

Before opening the PR:

- [ ] `make test` passes
- [ ] `make test-int` passes (or you've explained why integration coverage
      isn't applicable)
- [ ] `gofmt -l .` is empty
- [ ] `go vet ./...` is clean
- [ ] `golangci-lint run` is clean (optional but encouraged)
- [ ] You've updated `CHANGELOG.md` under `[Unreleased]` if the change is
      user-visible
- [ ] You've updated the README if you've added a flag, env var, or command
- [ ] For changes that touch process-tree code, the spec §13 invariants are
      preserved (one `/proc` walk per invocation, etc.)

When opening the PR, fill out the template in `.github/PULL_REQUEST_TEMPLATE.md`.

User-visible changes should add a bullet to the `[Unreleased]` section of
[`CHANGELOG.md`](CHANGELOG.md) — Keep a Changelog format. The bullet belongs
under one of: `Added`, `Changed`, `Fixed`, `Removed`, `Deprecated`, or
`Security`.

---

## Out-of-scope features

Some features are **deliberately rejected** — see `spec.md` §3 ("Out of Scope")
and §23 ("Non-Goals That Implementers Will Be Tempted to Add"). Before
proposing one of these, please open a discussion issue first so we can talk
through the design trade-offs:

- Daemon / background process
- `claude -p` headless mode orchestration
- Remote-host management (SSH)
- Multi-user mode
- Conversation editing
- Support for screen / zellij / wezterm

Post-v1.0 additions (the TUI, external Claude detection) **did** require
explicit maintainer sign-off; that bar still applies to anything in the
rejected list.

---

## Reporting bugs

Open an issue with the **Bug report** template. The form asks for:

- `muxc doctor` output (paste verbatim — it tells us 90% of what we need)
- tmux version (`tmux -V`)
- OS / arch (`uname -a`)
- muxc version (`muxc version`)
- Steps to reproduce, expected vs actual

For security issues, **do not open a public issue**. See `SECURITY.md`.

---

## Questions

Open a GitHub Discussion (when enabled) or an issue with the question
template. The maintainer reads every one.

---

## Code of conduct

This project follows the [Contributor Covenant 2.1](./CODE_OF_CONDUCT.md).
Be kind. Reports to `azrafalmonzim@gmail.com`.

---

## License

By submitting a PR, you agree to release your contribution under the
[MIT License](./LICENSE), the same license as the rest of muxc.
