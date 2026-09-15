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
