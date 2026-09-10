# Phase 27: adversarial review and verification

Scope: completion of `feat/phase-27-reporting-blocks`, recovered from
`45436feb15353dcd3c5b52ff05d779dace5ae880` on merged baseline
`55463d424c2deed555fa680f47d5444fc89ae950`. This is a same-author adversarial,
test-driven review, not independent human approval, deployment or a security audit.
The substantial interrupted implementation was preserved rather than rewritten.

## Findings reproduced and repaired

| Finding | Correction and regression |
| --- | --- |
| A fresh validation receipt could override known unavailable/stale health during publication or certification. | Both the service freshness check and the PostgreSQL mutation check require healthy current status and its exact evidence dependency digest. `TestFreshValidationRejectsKnownUnhealthyDependencies` was red before the fix; AC07 proves actual source unavailability cannot recertify while historical attestation remains readable. |
| Same-name database/role, table or column replacement could be classified as unchanged from registered names alone. | Impact comparison checks previously established native catalog authority and object continuity even when names/binding digests are unchanged. Lost native proof and replacement require review, never rename. `TestImpactDoesNotMistakeReplacementForContinuity` reproduced each original false-continuity result. |
| Logical parameter count could admit more physical bind slots than the executor supports. | Declarations now enforce the existing 64-scalar-slot ceiling; relative periods consume two. `TestParameterPhysicalSlotBudget` accepts 32 periods and rejects 33, without widening execution limits. |
| Closed optional union pointers prevented reporting API schema registration. | `NullableCollections` now supports closed optional pointer members while required scalars, request roots and arbitrary maps remain strict. Regression rejects null scalar, extra identity fields, duplicate keys and null collection elements. All eight feature combinations register. |
| Top-level route admission called the resource enforcer with no resources, denying all reporting requests. | The handler checks the signed action first, then addressed block reach before body decoding. The common service and PostgreSQL remain responsible for creation/list/parent/reference/private eligibility. Actual SDK creation through the protected handler now succeeds. |
| Nullable fields inside embedded request DTOs were not adjusted, so a valid SDK preview was rejected. | Wire schema adjustment follows encoding/json's flattened embedded structs. `TestNullableCollectionsInEmbeddedDTO` preserves required-scalar rejection; AC08 executes and schema-checks a real private preview. |
| Phase 27 had six of eight declared acceptance criteria; several API/SDK routes had no successful end-to-end coverage. | AC05 and AC07 now use real PostgreSQL/native execution and catalog changes. AC08 exercises the completed query handoff and complete lifecycle. Positive wire responses are validated against the registered schemas. |
| The acceptance actor exceeded the existing JWT scope ceiling; the migration invariant still named schema 21. | Removed an unused test-only feedback permission (not a production scope-limit change). The migration assertion verifies migration 022's identity, checksum and block tables, and phase 02's exact cumulative schema inventory includes all eleven new tables. |
| SQL-assistance fuzz workers could exceed the per-input watchdog while compiling the embedded PostgreSQL WASM grammar for the first time. | Initialize the unchanged grammar before entering the input loop; the 128-input campaign then passes without weakening input checks, corpus bounds or production parsing. |
| Phase 21's cumulative test registry did not include the new routes. | Its registry now includes reporting's full concrete surface and a guard requires `createBlock`. Installed capabilities expose `governed_blocks`, not future reporting execution/rendering. |

The query-to-block adapter is owned once by reporting and injected by foundation;
HTTP/SDK do not reconstruct query provenance or fabricate validation. The repeated
dependency-topic collection pass was removed without changing the dependency set.

## Real acceptance boundaries

AC01 tests stable identity, localized metadata, immutable revisions and invalid
closed shapes. AC02 exercises actual PostgreSQL CAS races and lifecycle history.
AC03 validates exact SQL through the pinned native parser/executor, compares real
schema/evidence and invalidates edits. AC04 separates publication, scoped
certification, current health, withdrawal and historical attestation.

AC05 executes all nine parameter kinds with exact typed binds, proves pure
resolution does not touch the source, rejects invalid dimension/range/default
inputs, and keeps SQL-injection-shaped category text as data. Native AST period
assistance preserves unrelated filters/comments and approved SQL; its amendment
needs new validation. Unit goldens cover DST gaps/folds, leap/calendar boundaries,
first schedule-window policy and month-end clamp/reject behavior.

AC06 previews all four saved output kinds over an actual single-row result and validates output subsets, private previews and SQL-private metadata;
narrative definitions do not invoke a model. AC07 uses real source connectivity
loss/recovery, same-name PostgreSQL column replacement and restoration, reviewed topic revisions, a physical column rename,
registered source rotation, reprofile and a missing-column change. Only an exact
server-recomputed rename proposal creates a private amendment. Semantic changes
and missing dependencies require review; publication bytes remain unchanged.

AC08 covers manual authoring, completed-query capture, validation, publication,
listing/question assessment, private preview, certification/withdrawal, amendment,
rejection/restore, history, archive and exact SQL inspection through the SDK and
protected HTTP handlers. Planned-query, cross-session, missing SQL authority,
cross-tenant, same-tenant different-context, private other-actor, lost parent reach and malformed body cases reject. Unauthorized metadata, SQL, history and list access conceals rows/cursors without source-secret lookup. Parameter
resolution, assistance and impact are exercised through SDK calls in AC05/AC07.
Successful JSON responses are checked against their actual registry schemas.

## Verification and limits

The eight named criteria passed together under `go test -race -count=1` against
real PostgreSQL 17.10/pgvector 0.8.2, Go 1.26.4 and the pinned native Bruin source
`5f562c2959496a04d57f5f199f5e3ad22159fa9f`. The phase-21 cumulative guard, migration
assertion and nullable-schema regressions passed in that same targeted invocation.
The strict phase-27 smoke reports eight passing criteria and zero unimplemented
skips. A clean focused race/coverage invocation across reporting, reportingapi,
chartdata, the API registry, SDK unit tests and phase 27 passed. Its actual
instrumented counts for the new production packages are:

| Package | Covered / total statements | Coverage | Required |
| --- | ---: | ---: | ---: |
| `internal/reporting` | 1427 / 1769 | 80.67% | 80% |
| `internal/reportingapi` | 209 / 240 | 87.08% | 80% |
| `internal/chartdata` | 55 / 56 | 98.21% | 80% |

These are focused package measurements, not a claim that the complete cumulative
coverage gate passed. Existing SDK functionality also needs its other owning
phases; its deliberately smaller focused invocation is not used as a global band.
Both bounded race-enabled fuzz campaigns passed 128 inputs. `make planning-check`,
`make build`, `make vet` and `make drift-audit` passed.

The full local race/coverage suite was run and was **not green**. It exposed the
old cumulative schema allowlist, a disposable PostgreSQL fixture using trust
instead of password authentication, and resource-sensitive read-test failures.
After the exact allowlist and local fixture corrections, phases 02/08 and all
affected read-regression groups passed on rerun; phase 11 also passed in
the subsequent expanded invocation. The isolated pipeline run reached its real
managed-runner boundary but still failed execution checks in this local
environment. The required MySQL/SQL Server fixtures were unavailable. These
failures are not waived, relabeled as passes or used to reduce package thresholds.
The normal PR CI retains the full pinned native, pipeline and engine fixtures;
its result is an outstanding qualification gate, along with full lint and native
image/platform builds.

The initial remote checkpoint on `aabc6979e9e80be271ee7b381d4e1fa7ccdefbaf`
failed the original oversized-token fixture. Its separate all-package native
coverage build exhausted runner disk before completion. Neither failure counts
as green evidence. Permanent CI bounds Go build parallelism and removes unrelated
preinstalled SDKs to reserve disk; it retains all production packages, race
instrumentation and existing coverage thresholds. Temporary source-generation,
workspace-export and checkpoint workflows are removed from the delivered tree.

This local environment does not qualify every required warehouse/managed-runner
boundary, live model/provider or native Docker image. The
normal committed-source CI retains those existing checks. No coverage threshold,
security scope ceiling, production budget or acceptance assertion was weakened.
Phase 28 execution/artifact work, scheduled refresh, report/dashboard composition,
static rendering and narrative generation remain outside this PR.

## Reproduction

```sh
make planning-check
make drift-audit
make build
make vet
make coverage
python3 scripts/run_phase_acceptance.py --phase 21
bash scripts/smoke/phase-27.sh
go test -race ./internal/reporting -run '^$' -fuzz '^FuzzParameterScalarDoesNotBecomeSQL$' -fuzztime=128x -timeout=3m -parallel=2
go test -race ./internal/reporting -run '^$' -fuzz '^FuzzSQLAssistSpanSafety$' -fuzztime=128x -timeout=3m -parallel=2
```
