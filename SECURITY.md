# Security policy

## Supported versions

muxc follows [Semantic Versioning](https://semver.org/). Only the most recent
minor release is actively supported with security fixes.

| Version | Supported |
|---|---|
| Latest minor (e.g. `1.x`) | ✅ |
| Older minors             | ❌ |
| `main` branch             | ✅ best-effort |

## Reporting a vulnerability

**Please do not file public GitHub issues for security vulnerabilities.**

Email the maintainer at **azrafalmonzim@gmail.com** with a subject line that
starts with `muxc security:` (this routes the report past noise filters).

A good report includes:

- The muxc version (`muxc version`) and how it was installed.
- A clear description of the issue and its impact.
- A minimal reproduction (commands, configuration, environment).
- Any suggested mitigation, if you have one.

You can expect:

- Acknowledgement within **48 hours**.
- An assessment and disclosure timeline within **7 days** of acknowledgement.
- A fix released as soon as a viable patch is available, coordinated with you
  on the disclosure date.

If you would like to publish your own write-up after the fix is released, we
will gladly link to it from the release notes.

## Threat model

muxc is intentionally a thin, narrow surface. It is worth being explicit about
what is — and is not — in scope so that reports stay focused.

**In scope.** Bugs that could let a local actor:

- Escape tmux pane isolation, modify state outside `~/.config/muxc/`, or write
  anywhere under `~/.claude/` (which muxc treats as read-only).
- Inject shell metacharacters into the `tmux` exec wrappers (`internal/tmux/`).
- Traverse paths outside the configured `paths.claude_projects` root when
  reading JSONL transcripts (`internal/claude/`).
- Cause arbitrary process kill or signal delivery beyond the explicitly
  targeted session.
- Corrupt `~/.config/muxc/state.json` in a way that survives the next
  invocation and causes follow-on misbehaviour.

**Out of scope.**

- Anything requiring code execution as the user running muxc (the user already
  controls their own shell, config, and `~/.claude/` directory).
- Anything that requires modifying muxc's own binary or config file.
- Vulnerabilities in `tmux`, `claude`, or the Linux kernel — report those
  upstream.
- Denial of service caused by running out of file descriptors, PIDs, or disk
  space.

## Design properties that constrain the attack surface

The project's `spec.md` §2 constraints translate directly into a small,
auditable attack surface:

- **No network I/O.** muxc never opens a socket, makes an HTTP request, or
  resolves a hostname. No telemetry, no auto-update.
- **No daemon, no shared state, no privileged operations.** Every invocation
  is a fresh one-shot run as the invoking user.
- **`~/.claude/` is read-only.** muxc does not write, lock, or modify anything
  under it; only reads JSONL transcripts.
- **All tmux invocations go through a single wrapper** (`internal/tmux/`),
  which is the right place to audit for shell-quoting issues.
- **State is atomic.** `~/.config/muxc/state.json` is written via
  write-temp-then-rename and is fully reconstructible from tmux + `/proc`,
  so corruption is recoverable.

## Acknowledgements

Once we have a credited fix, reporters who consent will be acknowledged in the
release notes and in `CHANGELOG.md`.
