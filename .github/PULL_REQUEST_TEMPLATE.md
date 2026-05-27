<!--
Thanks for sending a PR! Please run through this checklist before requesting
review. If something doesn't apply, leave the box unchecked and explain why
in the PR description.
-->

## Summary

<!-- 1–3 sentences. What does this PR change, and why? -->

## Related issue

<!-- Closes #123 / Fixes #456 / Refs #789. Skip if there's no tracking issue. -->

## Type of change

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing behaviour to change)
- [ ] Documentation only
- [ ] CI / build / tooling
- [ ] Refactor (no functional change)

## Checklist

- [ ] `make test` passes
- [ ] `make test-int` passes locally, or doesn't apply to this change
- [ ] `gofmt -l .` is empty
- [ ] `go vet ./...` is clean
- [ ] `golangci-lint run` is clean (optional but encouraged)
- [ ] `CHANGELOG.md` has an entry under `[Unreleased]` for user-visible changes
- [ ] `README.md` updated if a flag, env var, or command surface changed
- [ ] `docs/MANUAL.md` updated if behaviour, flags, or config keys changed
- [ ] Spec invariants in `spec.md` (especially §2, §13) preserved for code that touches tmux, /proc, or `~/.claude/`

## Screenshots / output

<!-- Paste a terminal recording, `muxc ls` table, or screenshot if this PR changes
     user-visible output. Delete this section if not applicable. -->
