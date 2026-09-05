# Phase 25 — e2e-release

Status: planned. Owner: test/integration, cmd/chartworks. Hard dependencies: 24, 26, 34.

## Authority and design

RFC-001 §16/17, RFC-002 §9, D-044–D-052 and [COMMON.md](COMMON.md) apply. This is the final gate despite its number: its transitive dependencies cover all 34 phases. The initial demo cannot satisfy this release gate.

## Brief findings incorporated

Briefs 01–06 and 14: cumulative end-to-end verification, source continuity, tenant isolation, honest dependencies and operational recovery.

## Findings I'm departing from

No dual-issuer release scenario, compatibility qualification for established hosts or green-by-skip live evidence. Missing required features cannot be hidden by a feature flag and still called full migration.

## Scope and implementation tasks

1. Run the complete Pengui-issued-authority reference deployment from a fresh store with enabled real sources, workflows, reporting viewer and renderer.
2. Close every required feature/gate/adapter evidence row; verify backup/restore/rotation/retention/erasure, recovery and secret-safe operational documentation.
3. Prepare version/release artifacts and measured behavior only after cumulative review and owner acceptance; no automatic merge/tag/deployment in planning work.

## Non-goals

No new product feature to postpone closure, local authentication fallback, untested engine support claim or implied L3/internal-analyst delivery.

## Config and persistence

Pin the actual image/toolchain/runner/renderer assets and example config; no new runtime feature flags as a way to claim incomplete parity. Verify fresh migrations and restore compatibility for the release candidate, not a pre-migrated development database.

## Acceptance criteria

1. **AC01** — Only Pengui-issued auth is exercised; no dual-mode/bootstrap/grant-management compatibility path survives in the build or documentation.
2. **AC02** — Container starts cleanly, reports dependency readiness correctly and passes shutdown/recovery/backup/restore/JWKS rotation scenarios.
3. **AC03** — Startup/idle, query overhead, normalization, artifact reads/rendering and scheduling recovery benchmarks record hardware/data and raw measurements.
4. **AC04** — All required engine/cohort gates are green with real evidence; missing live tests cannot be converted to skips that pass release.
5. **AC05** — Documentation, schemas, migrations and supported-feature declaration match the released binary; cumulative review findings are resolved.
6. **AC06** — Every required phase acceptance and B/R/Q/N/G coverage row is closed; Q11 is explicitly discarded source debt, not implemented functionality.

## Tests, coverage and smoke

Implement `TestPhase25/AC01` through `TestPhase25/AC06`. Release mode runs every actual acceptance test and rejects planned/missing/skipped results. The phase registry/evidence ledger is not proof of runtime behavior by itself. COMMON.md sets the exact commands and evidence contract; `scripts/smoke/phase-25.sh` requires all six results plus cumulative release coverage.

## Glossary, decisions and deviations

Record measured limits and accepted support boundaries. D-050 makes this the final cumulative gate. No release, tag, deployment or runtime test completion is claimed by this planning change.
