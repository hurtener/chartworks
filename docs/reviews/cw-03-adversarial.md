# CW-03 adversarial implementation review

Scope: BLK-01, BLK-05 and BLK-07 only. Baseline main
`35027d93d68c9c2ad3afc4ef26b9deca2b28aa73`. Scoped branch:
`feat/cw-03-reporting-output-policies`. No merge, tag, force-push or deployment.
This is an implementation self-review with executable negatives, not an
independent external audit or a broad parity/release certificate.

## Contract and disposition

[D-073](../decisions/2026-09-16-reporting-output-policies.md) and the
[v2 field-level contract](../contracts/reporting-output-intent-v2.md) specify
immutable v1 compatibility, v2 selection, restrictive sensitivity, native
import/export and exact unsupported mapping outcomes. Existing authority,
validator, lossless result, chart, queue and retention contracts remain in use.
No new issuer, token, identity, scope grant or standalone authoring app is added.

## Concrete findings and fixes

| Finding | Fix and regression evidence |
|---|---|
| Output labels/defaults/enablement could not survive the full reporting lifecycle. | V2 `OutputIntent`, detached migration, selection snapshot, immutable JSON constraints and propagation through manifests/reuse/artifacts/compositions/delivery. `TestCW03ReportingPublicationAndExecution`, `TestCW03PopulatedLegacyDatabaseUpgrade` and actual viewer component tests. |
| Display order and execution order were conflated in viewer expectations; unselected metadata was confused with unselected result data. | Display-ordered choices are distinct from accepted execution sequence. Phase31 AC02 checks both and still rejects an unselected widget payload with a typed error and an empty response. |
| Filter-triggered new runs dropped accepted query caps and reconstructed output selection from display order. | Delivery exposes accepted selection/caps separately from navigation. The viewer copies them into the explicit run request; the actual browser checks reversed execution order, localized/disabled/omitted choices and unchanged caps. |
| Manual redaction was the only narrative egress defense. | Shared reviewed semantic sensitivity derives restrictive effective result/evidence policy; unknown/conflicting/sensitive dependencies cannot be declassified. Actual HTTP provider input tests cover inherited and manual redaction, including unknown/conflicting/authored-safe cases. |
| Model availability and cumulative token checks could mask a deterministic no-evidence result. | Versioned evidence preparation/exclusion runs before model availability/reservation. Unknown evidence with an unavailable provider or newly narrowed token budget returns the same typed result with zero provider calls and reservations. Legacy receipt semantics stay versioned. |
| Authored type/tone and max-claims intent could be merely forwarded to a provider. | Versioned closed claim schemas, local grounding/type validation and deterministic English/Spanish tone templates enforce the policy. Claims and final characters including caveats are bounded. Historical unversioned narrative text/hashes are not rewritten. |
| Unsupported migration could silently repair malformed v2 intent or lose authored policy through a read projection. | V2 migration is a detached identity and rejects missing intent/invalid policy. Exact native definition export uses existing SQL-plus-read authorization. Create/edit imports retain authored policies and require normal validation/publication. Metadata-only readers cannot export SQL/definitions. |
| Accepted per-block and widget limits could be bypassed by later deployment or parent/child admission. | Authored/requested/authorized/deployment limits intersect at admission and again during actual execution; physical attempt checkpointing uses the current cap. Reuse/group identities include accepted caps and reject over-cap evidence rather than regenerate. Real persistence tests cover lower and higher stored caps and group segregation. |
| Later publication or catalog failure could cause a retry to replace selected outputs/caps or redo the query. | Exact parent/child manifests retain revisions, selection, budgets and privacy. A real signed-queue/catalog-failure test republishes narrower/different definitions before retry and verifies unchanged manifest bytes and zero additional query/model work. |
| Serializer-only migration and bound tests would miss storage/provider behavior. | Populated schema34-to-35 production migration, immutable triggers and invalid SQL inserts are exercised. HTTP-provider fixtures exercise claims/type/characters/rows/bytes/tokens/calls/time and failed retained redraw. |
| The original validation record did not cover the final fixes and new acceptance cases. | The exact-source receipt now records the successful final implementation run, its artifact digest and actual pass-event counts. Historical failing runs are not presented as successful checks. |

The adversarial pass also inspected cancellation, uncertain physical attempts,
commit fences, cross-tenant and same-tenant/context negatives, private previews,
retention tombstones and one-query fan-out. Existing phase27-31 tests remain
mandatory; they were not replaced by weaker CW-03 planning/serializer checks.
The final source review rechecked output-selection rejection, restrictive
sensitivity before provider input, accepted/current caps, immutable revisions and
retained no-regeneration paths. No unresolved P0/P1 finding is recorded within
this assignment's reviewed scope; the qualification limitations below still apply.

## Executed validation and exact-source evidence

At implementation head `f517ab6e2f6048632249d57c77c4783cb9a0e1a2`, Actions run
[35061994750](https://github.com/hurtener/chartworks/actions/runs/35061994750)
completed successfully. Its downloaded `cw-03-contract-evidence` artifact
(ID `10433395915`) has SHA-256
`11180043b210702e5864ea67e63f6dc59f75145c770d85834d2df1ef0ca9e5b1`.
`reporting-source.log` identifies that exact tested commit. The
[machine-readable receipt](cw-03-validation.json) records counts and boundaries.

| Executed check | Actual result |
|---|---|
| CW-03 acceptance with race detection | 23 passing test/subtest events across five top-level tests; zero failures or skips |
| Strict phase27, phase28, phase29, phase30 and phase31 runners | All 40 required AC01-AC08 criteria passed; zero unimplemented skips |
| Actual browser | Phase31 AC08 passed with real Chrome, including localized/disabled/omitted selectors and retained filter-request selection/limit preservation |
| Unit/race regressions | 594 passing test/subtest events across 13 packages; zero failures or skips |
| Planning | 60 checker tests passed; coherence for 224 criteria, 63 features, 34 phases and 41 gates |
| Build, vet, Go formatting, JavaScript syntax, module verification and clean source | Passed |

Test/subtest event counts include parent tests; they are not a count of independent
scenarios. The planning check's synthetic planned-skip fixture is not a skipped
runtime acceptance case. Native build steps used a matching pinned cache; the
actual test, browser, build and vet steps executed and passed.

The five CW-03 top-level tests are `TestCW03PopulatedLegacyDatabaseUpgrade`,
`TestCW03NarrativeHardBounds`, `TestCW03ReportingPublicationAndExecution`,
`TestCW03NarrativeSensitivityAtProviderBoundary` and
`TestCW03ScheduledCompositionRetryPins`. These include the previously unverified
upgrade, retry, native-export and hard-bound cases. The former phase31 AC02
selector-count failure and obsolete test-import failure are fixed at this head.

The read-only [CW-03 workflow](../../.github/workflows/cw-03-validation.yml)
tests committed source without mutation and runs:

```sh
go test -race -count=1 -json -timeout=12m ./test/acceptance -run '^TestCW03'
# Each strict runner requires every AC01-AC08 result, with no allowed skips.
python3 scripts/run_phase_acceptance.py --phase 27
python3 scripts/run_phase_acceptance.py --phase 28
python3 scripts/run_phase_acceptance.py --phase 29
python3 scripts/run_phase_acceptance.py --phase 30
python3 scripts/run_phase_acceptance.py --phase 31
# See workflow for the full API/semantics/config/jobs/gateway/store/SDK package list.
go test -race -count=1 -json -timeout=10m <workflow-package-list>
make planning-check
make build
make vet
git diff --exit-code
```

Reference environment: Ubuntu24, PostgreSQL17/pgvector0.8.2, Go1.26.4,
Node22.14, actual Chromium/Chrome and the repository's pinned native read driver.
The model boundary is an actual HTTP provider fixture behind the production
gateway, not a live commercial-provider measurement.

This evidence applies to the exact implementation head above. A subsequent
receipt-only documentation commit does not imply a runtime rerun at its own SHA;
later exact-source workflow results and PR checks must be identified separately.
The resumed local environment confirmed the artifact digest and parsed its logs.
Local Go execution remains blocked: available Go1.23.2 cannot build the required
Go1.26.4 module, and network/toolchain downloads are unavailable. No local Go or
browser pass is claimed. Source writes use the authorized repository connection;
final validation has read-only contents permissions and no source preparation or
repair workflow.

## Remaining qualification boundaries

No live provider/model quality, cloud warehouse, cost/scan-byte guarantee, stress,
full coverage-band certificate, hosted consumer security qualification, phase34
foreign import/cutover or phase25 full release check is claimed. Native alias-based
expression lineage is not invented; sensitivity conservatively restricts the
whole reviewed query dependency set. Reservations are not measured usage.
Unsupported causal/unrestricted prose and unmapped legacy instructions fail
explicitly. No new notifications, deletion, dynamic option discovery, richer KPI,
table styling, chart-binding or clarification implementation is included.

## Final CI follow-up after owner merge

The last repository-wide run did not finish green. The verified original logs,
concrete final-review findings, forward-only locale migration and regression
coverage are recorded in [the final adversarial follow-up](cw-03-final-adversarial.md).
Its PR carries the final new-head CI receipt; earlier exact-SHA receipts above
remain historical and are not rewritten as passing evidence for newer changes.
