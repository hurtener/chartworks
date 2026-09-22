# Phase 24 evaluation implementation evidence

Date: 2026-09-22. Status: implementation submitted for adversarial review; not release qualification.

The implementation adds the `internal/evaluation` suite, deterministic gate, governed live runner, durable suite, runtime-pack and optimization review lifecycles, protected feedback export and fixture-only local CLI. Migration 048 stores immutable tenant-scoped runtime packs and suite revisions, distinct authenticated review receipts, admitted and terminal runs, cancel intent, protected inputs, pre-training candidate and independently assigned split ledgers, proposals and the reviewed pack-selection pointer. HTTP, MCP and SDK consumers use the same service for runtime-pack author/review, suite author/review and run/read/cancel.

`TestPhase24/AC01`–`AC06` exercise a deliberately failing regression, semantic alternatives and pinned bilingual stage evidence, six adversarial categories, replay/shadow proposal provenance, fixture/live separation with unknown cost, and durable reviewed-suite/export history. Package tests cover strict JSON, duplicate fields, CLI live refusal, distinct author/reviewer enforcement, runtime-pack tamper, stale revision, model and understated-cost rejection, pre-provider over-cap refusal, typed terminal evidence and deterministic hashes.

Verified on the implementation worktree:

- `go test ./internal/evaluation` passed; direct package coverage exceeded its 70% tooling band.
- `CGO_ENABLED=0 go test ./test/acceptance -run '^TestPhase24/(AC01|AC02|AC03|AC04|AC05)$' -count=1` passed the five non-database criteria; the complete test package and PostgreSQL implementation compile gate passed.
- `CGO_ENABLED=0 go test ./internal/nlqexec ./sdk/chartworks` passed after resolving pre-existing lint blockers without changing behavior.
- `CGO_ENABLED=0 go test ./internal/store/postgres -run '^TestSafeErrors$' -count=1` passed the migration manifest assertion.
- `make lint`, planning check, mirror comparison, JSON validation and `git diff --check` passed.

The local race smoke could not link because the existing macOS checkout lacks the pinned Bruin Rust parser archive. AC06 now exercises migration 048 and immutable suite/report persistence against real PostgreSQL; its disposable database could not start because Docker Desktop returned a content-store input/output error, and the already-running shared database was deliberately left untouched. Hosted native/race/PostgreSQL evidence therefore remains required. The live owner-calibrated private suites, six-engine measurements and cross-system comparison remain Phase 34/25 evidence; no fixture result closes those claims.

This branch starts from migration 045. Phase 32 and Phase 33 are concurrently adding schema/decision records, so it must be rebased after those merge and its migration/decision identifiers renumbered in sequence before merge if they land first.
