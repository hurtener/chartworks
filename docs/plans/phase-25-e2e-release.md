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
per-step correctness gate and raw measurement schema. The release candidate must
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
revision evidence before running `final_stress`. The original Plan→Run query
adapter remains a ledger-only prerequisite. A protected Phase 24 frozen
consumer and bounded performance adapter now enter `reporting.Runs` with
distinct IDs, read canonical `RunManifest.ReuseKey` and `ReusedFrom`, and
consume nullable persisted source-only duration plus physical narrative
receipts. A real-PG17 recorded-model prerequisite test covers cold, warm,
repeat and concurrent reuse, two distinct passing accepted Phase 24 reports
for selected reviewed runtime packs, and rejects incomplete evidence. The
product-sealed reviewed-pack pin is checked against the accepted Phase 24
runtime record before source or model execution; a stale pin fails before
timing. The signed-action negative can remove `reporting.execute` for the
frozen consumer; it sends the altered verified bearer through that same service. Each
invalidation step pins a consumer case and exact
accepted report hash; the resolver must provide the matching current revision
binding and report-selected runtime pack. Bifrost attests live mode; a recorded
gateway engine explicitly attests recorded mode for integration runs. The
internal `releaseprofile` composition selects one operator-reviewed Phase 34
cohort for each accepted Phase 24 consumer case. It reloads the active cutover
and source adapter, current topic/rule/source/context evidence, and all rows of
one bounded native PostgreSQL dataset's validator-safe projection under signed
reach. It refuses
multi-dataset snapshots that cannot be read in one transaction. The recorded
engine matches exact authorized call and reviewed runtime configuration inputs
and returns a recorded receipt with no provider fallback. These tests prove the
integration seam, not a live model or final stress run.
Final AC03 must still run the exact stress profile against accepted Phase 24
case/report evidence for every changed source/rule/context/topic cohort, then
prove current-owner dependency closure and a stale-key negative before timing.
The runtime-pack dimension has a product-selected accepted-pack pin in the
frozen narrative consumer and reuse identity, with bounded recorded invalidation
evidence. The strict adapter now requires the changed pack's own accepted
Phase 24 report and an operator-configured approved proposal whose stored pack
digest matches that report before any selection mutation. It selects the
report-bound pack with verified Pengui authority and a CAS pointer before each
correctness probe and measured step, then restores the baseline selection on
success or failure. A foreign pointer revision blocks rollback rather than
overwriting another operator's choice. The sealed raw samples retain product
run IDs, reuse keys and origins alongside source-only timing and physical
gateway usage; unknown provider costs remain unknown. The bounded PG17
test and transition tests are prerequisites, not a completed `final_stress`.
The recorded composition also refuses the default two-request tenant queue;
the fixed 128-peer final profile requires an explicitly provisioned bounded
request queue and tenant bytes sufficient for 128 simultaneous frozen artifact
reservations. Current topic publication pins source/context and current rule
publication pins topic version/digest. The final profile now requires the
declared primary axis and its exact current-owner dependency closure: rule;
topic plus rule; a different source ID plus its derived context, topic and
rule; or a rotated context on the same source ID plus source, topic and rule.
The resolver checks the selected block's current pins and the gate
requires matching raw changes as well as the product's actual reuse-key
change. A cohort-only hash or missing collateral pin fails before timing.
The recorded production composition now requires an operator-owned revision
transition. The ordered profile measures a step only while its accepted Phase
24 case is current, records that owner snapshot in the sealed report, and
re-resolves it after measurement. A later failed step invalidates the entire
report. No accepted owner transition plan or live cohort is checked in; AC03
therefore remains open.
The release runtime also refuses both Pengui bearers unless their verified
deadlines cover the one-hour profile and a two-minute cleanup/skew reserve. It
rechecks that bound before selecting the changed reviewed pack, so the default
15-minute bearer returns `ErrPerformanceAuthorityWindow` before a profile can
strand the CAS selection. No Chartworks token renewal or issuer was added;
AC03 needs a real Pengui fresh-authority seam for the full run and cleanup.
Changing Chartworks' token-lifetime configuration does not establish that
authority contract.
No measurements are labeled a passing AC03 run until the exact profile and
owner cohort are exercised.
The required full profile still needs accepted current reports for each
source/rule/context/topic cohort, provisioned concurrency, raw recorded and
live measurements, owner cohort evidence, and the final release bundle.
Phase 25 remains in progress until the real release profile executes and is reviewed;
this change does not claim a final stress run or release acceptance.
