## Summary

<!-- What does this PR do, and why? -->

## Context

- **RFC / request section(s) touched:** <!-- e.g. RFC-001-Chartworks §4.2, or "n/a — pre-RFC" -->
- **Phase:** <!-- e.g. phase-03-slug, or "n/a — one-off fix" -->
- **Plan deviations:** <!-- any reasonable deviation from the phase plan, documented here AND in the plan file itself (CLAUDE.md §4.3) -->
- **Decisions filed:** <!-- new docs/decisions.md D-NNN entries this PR adds, or "none" -->

## Pre-merge checklist (CLAUDE.md §14)

- [ ] `make drift-audit` passes.
- [ ] `make check-mirror` passes (`AGENTS.md` == `CLAUDE.md`).
- [ ] `make preflight` passes.
- [ ] `go test -race ./...` and `golangci-lint run` are clean.
- [ ] All cross-references (`RFC §X.Y`, `D-NNN`, `brief NN`) resolve.
- [ ] Coverage on touched packages ≥ the phase's stated target — `make coverage` passes (a new package is added to the coverage config in the same PR).
- [ ] A new CLI command / endpoint / MCP tool / config key has a smoke check in this PR.
- [ ] If a reusable artifact changed: a concurrent-reuse test passes under `-race`.
- [ ] If an ACL/auth path changed: the adversarial test obligations (§11) still pass.
- [ ] If a cross-subsystem seam was opened or consumed: an integration test against the Docker Postgres exists (§17).
- [ ] New vocabulary added to `docs/glossary.md` in this PR.
- [ ] A new architectural decision (or a departure from a brief / the RFC) is filed in `docs/decisions.md`.

## Test plan

<!-- How was this verified? Commands run, smoke output, screenshots if relevant. -->
