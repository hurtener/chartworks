# Phase 25 — e2e-release

Status: in_progress. Owner: test/integration, cmd/chartworks. Hard dependencies: 24, 26, 34.

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

The [performance evidence contract](../contracts/performance-evidence-v1.md)
defines the required final-stress scenario counts, authority/revision identity,
correctness-first gate and raw measurement schema. The release candidate must
materialize the actual Phase 24 suite/report hashes and post-Phase-34 source,
rule, topic, context and runtime-pack revisions. The checked-in synthetic smoke
does not satisfy AC03.

## Release evidence implementation boundary

The [release evidence v1 contract](../contracts/release-evidence-v1.md) pins an
external content-free bundle to the exact source head, binary, image, active
documentation/schema files, named live test logs, Phase 34 cohort inventory and
cumulative review. `TestPhase25/AC01`, `AC02`, `AC04`, `AC05` and `AC06` now run
local substantive checks and require the bundle in strict release mode. The
runner passes validated earlier-phase Go results directly to AC06; coverage
rows cannot be closed with a registry label alone.

No release bundle exists for this implementation head. The Phase 34 cutover
implementation has merged but its phase status remains in progress; an accepted
live cohort inventory and source evidence are still required. AC03 awaits the
selected final stress run, and five AC tests are not full Phase 25 acceptance.
Phase 25 remains in progress until integration supplies all six criteria and the
strict preflight passes. Live Pengui/engine/cohort and container qualification
cannot be inferred from synthetic fixtures.

## Final-stress integration boundary

The bounded release orchestration now verifies caller authority independently,
loads the exact accepted Phase 24 suite/report/runtime pack, and requires current
revision evidence before running `final_stress`. Its concrete governed adapter
composes the existing query service's Plan→Run path and requires the PostgreSQL
cross-process operation lock. The operation ledger does not exercise the
frozen-run product reuse key, so the factory rejects the required invalidation
steps; a fresh operation per changed binding cannot satisfy AC03. The read
attempt exposes physical-call evidence but no source-only duration, and the
timing gate also fails closed. The signed-action negative uses a separately
verified bearer with one query action removed and sends it through Plan→Run;
it cannot be satisfied by testing an unrelated action. Each invalidation step pins a consumer case and exact
accepted report hash; the resolver must provide the matching current revision
binding and report-selected runtime pack. Bifrost attests live mode; a recorded
gateway engine must explicitly attest recorded mode for integration runs. Phase
34 still has to supply selected source/rule/topic/context revisions from its
current stores,
and the composition root must provide the recorded engine for integration mode.
An AC03 adapter must instead exercise distinct frozen run IDs over the same
approved block via the real `ReuseFrozenRun` path, observe its reuse key and
`ReusedFrom`, then assert physical source calls across one-field changes. It
also needs a persisted native PostgreSQL read duration that excludes journal
and finalization time. The runtime-pack dimension needs an actual reviewed
pack pin in that frozen-run identity before it can be claimed.
Phase 25 remains in progress until the real release profile executes and is reviewed;
this change does not claim a final stress run or release acceptance.
