# Phase 28 — reporting-execution-artifacts

Status: in_progress. Owner: internal/reporting. Hard dependencies: 05, 06, 10, 20, 27.

## Authority and design

RFC-002 §§4–6, D-045/D-047/D-051 and [COMMON.md](COMMON.md) apply. Frozen execution and retained artifact reading are separate operations. Use the existing queue, reader, gateway and signed-scope checks, not parallel reporting infrastructure.

## Brief findings incorporated

Briefs 02, 14; frozen stage prohibitions, one logical query/multiple outputs, expected schema, retained artifacts/privacy, idempotency and bounded narratives. Coverage B07–B20 and R09–R12 maps to these acceptance IDs.

## Findings I'm departing from

No inference/SQL correction/chart picker in frozen refresh; no bearer-based or tenant-only result cache; no expired-artifact silent rerun or universal exactly-once claim. Narratives are explicit optional consumers, not a second analyst.

## Scope and implementation tasks

1. Implement the frozen execution lane, one-time revision/parameter/dependency resolution and selected-output fan-out over one logical normalized query result.
2. Persist immutable run manifests, durable attempts/intermediate results where necessary, artifacts, output payloads, current access projection and retention/tombstones.
3. Add bounded evidence-based narratives and policy-aware idempotency/result reuse; route long work through the shared queue/current Pengui authority.

Acceptance sequence: reserve key/request hash -> resolve exact revisions/parameters/window/context -> seal manifest -> claim fenced attempt -> verify current authority and eligibility -> validated query -> ordered normalization -> selected deterministic outputs -> optional bounded narratives -> seal artifact/usage/partial outcome. Recover from retained intermediate results when possible. Indeterminate remote work is reconciled or explicitly counted as another attempt.

## Non-goals

No arbitrary SQL materializer hidden behind artifact GET, unlimited raw-row retention, autonomous narrative tools or new authentication/worker platform.

## Config and persistence

Reporting default retention=7d, private preview retention=24h (operator adjustable), result bytes/rows, narrative calls/tokens/time, artifact paging and per-tenant retained-byte ceilings. Add run/manifest/artifact/output state; reuse queue/attempt/idempotency and secret-safe delegation references. Store exact renderer/model/prompt versions, observed source freshness and actual data partition. Renditions inherit the parent artifact's privacy and expiry.

## Acceptance criteria

1. **AC01** — Instrumented tests forbid interpretation/retrieval/routing/SQL generation/correction/chart selection during frozen refresh; zero model calls without a narrative.
2. **AC02** — Several selected outputs share the same logical result; unknown/disabled outputs fail explicitly and physical retries remain visible.
3. **AC03** — Parameter precedence/window and ordered result schema/type/nullability/precision checks preserve meaning; incompatible drift cannot auto-rebind.
4. **AC04** — Narratives see only allowed/redacted evidence, have no query tools, obey pre-call budgets, ground claims and retain exact model/prompt/output provenance.
5. **AC05** — Idempotency accepts one manifest, conflicts on changed requests and survives crash/fencing; expired payload replay never silently issues another query.
6. **AC06** — Reuse/read checks actual context partition, target reach, privacy, revisions/parameters/locale/freshness; no tenant-only or raw-token-keyed shared result cache.
7. **AC07** — Retention removes values/renditions consistently while preserving permitted tombstones; reading or re-rendering an existing result performs no data/model execution.
8. **AC08** — API run/summary/result paging and cancellation are functional; budgets apply before/during work, including retries and partial failures.

## Tests, coverage and smoke

Implement `TestPhase28/AC01` through `TestPhase28/AC08`. Negative stage spies must fail if forbidden inference is invoked. Use real query/store paths; inject crashes after remote acceptance, normalized-row persistence, output generation and before commit. Test two callers with the same tenant/report but different execution contexts, private preview history and exact-value schemas. COMMON.md supplies coverage/evidence rules; `scripts/smoke/phase-28.sh` requires all eight results.

## Glossary, decisions and deviations

Logical operation, attempt, retained artifact and rendition are separate. D-047/D-051 apply. No runtime completion is claimed.

## Runtime completion evidence

See the [runtime contract](../contracts/reviewed-engineering-and-frozen-runs.md) and
[review and verification ledger](../reviews/phase-26-28-runtime.md). Exact-source
acceptance and coverage are required before closure.

## CW-03 output-intent and evidence-policy continuation

AC01/AC03/AC04/AC05/AC06/AC07/AC08: exact accepted selection/revision and query ceilings; current-cap clamps and reuse segregation; inherited sensitivity before model input; deterministic bounded narrative claims and retained zero-work behavior.

Use [D-073](../decisions/2026-09-16-reporting-output-policies.md) and the
[v2 field-level contract](../contracts/reporting-output-intent-v2.md). The
[scoped adversarial record](../reviews/cw-03-adversarial.md) links real PostgreSQL,
source execution, provider-fixture and browser checks. Keep the existing named
phase criteria and phase status; this assignment closes only its three owned gap
entries, not the whole phase, other reporting work, or full migration/release.

## CW-06 frozen rule dependencies

Frozen manifests, reuse identity, composition grouping and scheduled admission
retain exact rule pins and per-topic template selections. Sealing locks every
manifest rule head through the exact block revision and re-reads the active
publication under current signed authority, refusing any rule-version/digest
drift. The frozen
lane performs no question interpretation, rule selection, SQL generation or
model work. These additions extend AC01/AC05/AC06; broad race, full PostgreSQL
matrix and release checks remain assigned to the D-074 manual final workflow.

## CW-05 retained display continuation

AC01/AC02/AC03/AC06/AC07: frozen fan-out builds v3 KPI/table output from the same
normalized retained rows without selector, source or model work. Exact derived KPI
values, table presentation policy and column display metadata survive output storage
and reuse identity. Existing artifact authority/expiry remains authoritative.
Static export reads that artifact through the delivery facade under fresh read and
exact run-export reach; it does not re-execute expired/missing values.

## Frozen reuse identity hardening

New frozen runs seal a v2 reuse key over resolved semantic definitions, rule and
source dependencies, required resource reach, output policy, parameters, source
binding, locale, privacy and runtime model selection. Seal and PostgreSQL reuse
recompute the identity instead of trusting a stored key; both the target and
candidate must match. Existing v1 manifests remain readable and executable,
but cannot be used as cross-run reuse evidence under v2. Phase 28 AC06 and the
real-PostgreSQL reuse identity regression cover distinct operation IDs,
concurrent reuse, a substituted stale candidate key, signed
tenant/context/action denials and source-revision drift.
The runtime model string remains the authored narrative-policy version. Production
narrative admission now resolves the current tenant-selected, independently
accepted Phase 24 runtime pack under signed block/source/context execution reach.
The sealed manifest and v2 reuse identity pin its exact pack/runtime/configuration
digests and narrative role model. Execution and PostgreSQL reuse re-read the
current accepted selection; the Bifrost narrative call applies its reviewed
configuration and its actual role/model/configuration receipt is checked before
the narrative can be retained. Legacy unpinned manifests stay readable and
their deterministic outputs can execute under the reviewed-pack policy, while
their narrative is marked unavailable without a model call or cross-run reuse.
The selected pack must also carry an exact approved proposal and reviewer
receipt. Deterministic runs select no pack and
make no model call. The real PostgreSQL and recorded-gateway regression covers
two accepted packs, same-pack reuse, changed/rejected selections and signed
context denial. With no selected pack, explicit partial mode retains deterministic
outputs and a failed model-free narrative without cross-run reuse. Phase 25 still
owns final measured stress and release evidence.
