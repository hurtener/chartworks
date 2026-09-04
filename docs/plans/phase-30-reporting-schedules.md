# Phase 30 — reporting-schedules

Status: planned. Owner: internal/jobs, internal/reporting. Hard dependencies: 06, 18, 23, 28, 29.

## Authority and design

RFC-002 §7, the Pengui authority contract, D-048/D-051/D-055 and [COMMON.md](COMMON.md) apply. Scheduling domain behavior belongs to Chartworks; identity and permissions remain Pengui-owned. Reuse the real shared authority adapter and queue already delivered in phase06, not another reporting auth client.

## Brief findings incorporated

Briefs 02 and 14, coverage Q01–Q11: preserve real target/run/catalog behavior and discard event/condition/condition-check/custom-code stubs.

## Findings I'm departing from

No local service accounts, stronger-account selection, retained user JWT, local re-signing or guessed platform endpoint. Catalog recipients are not email-sent evidence. Phase30 is no longer the first real consumer of the authority provider: early profiling/semantic jobs require that in06.

## Scope and implementation tasks

1. Implement four reporting targets: reviewed saved SQL, explicitly dynamic saved questions, certified pinned block/output selection and published reports. Existing pipeline/maintenance targets share the same occurrence engine.
2. Extend the phase06 provider's target-binding fixtures for these reporting resources; enforce admission reach and fresh authority at occurrence/retry/checkpoints through the ordinary verifier.
3. Implement test/pause/resume/update/retire/history, recurrence/timezone/window/revision resolution, bounded budgets, catalog delivery and optional Pengui notification intents/receipts.

A schedule cannot choose a stronger execution binding than the creator's signed use/target/dependency permissions. Each attempt validates renewed Pengui authority and business eligibility. Missing or refused renewal records blocked state; it does not use ambient credentials. The new token changes authority, not the accepted occurrence's revision/period. No arbitrary request field asks the broker for broader scopes.

## Non-goals

No new auth broker, mail platform, general event processor or arbitrary scheduled code. Established Apps/viewer delivery is independent of this phase; scheduled runs later appear in the same result catalog.

## Config and persistence

Defaults prohibit overlap and unbounded catch-up; configure retry ceiling/backoff, timezone-database version and elapsed/model/warehouse attempt budgets. Reuse the shared platform connection reference; optional notifications use Pengui integrations. Persist due instant, half-open window, resolved revisions, target/binding and delivery intent/receipt separately. A schedule edit affects future unaccepted occurrences only.

## Acceptance criteria

1. **AC01** — All four targets execute their real domain paths; direct block delivery creates no hidden report, and dynamic execution needs explicit opt-in.
2. **AC02** — Reporting admission prevents stronger binding selection; renewed Pengui refusal/expiry/missing reach stops work, without local accounts, alternate issuer or stored user bearer.
3. **AC03** — Cron/interval/DST/leap/first/missed windows are defined and retained as half-open instants; retries never recalculate from current wall clock.
4. **AC04** — Exact pins and explicit latest-published resolve once; pause/resume/update/retire/test/history are durable and CAS-safe.
5. **AC05** — Overlap/catch-up/backoff/cancel/timeout/fencing and model/query budgets hold under concurrent workers and crash/recovery.
6. **AC06** — Query completion, retained artifact, catalog publication and notification intent/receipt are distinct; stored recipients grant no data access and prove no email sent.
7. **AC07** — Missing outputs/dependencies/approval/authority produces attention/blocked states, not automatic repinning or ambient access.
8. **AC08** — Unsupported event/condition/condition-check/unrestricted custom kinds remain absent from registration and rejected on input; bounded internal cleanup is preserved.

## Tests, coverage and smoke

Implement TestPhase30/AC01 through TestPhase30/AC08 with real queue/store/targets, phase06's actual authority adapter and deterministic timezone fixtures. Refusal asserts zero protected work; crashes after source acceptance/before delivery retain uncertainty. Test fixed/relative windows, mutation races and existing viewer catalog consumption without a new execution model. COMMON.md supplies evidence/coverage; scripts/smoke/phase-30.sh requires eight passes.

## Glossary, decisions and deviations

Pengui supplies authority; Chartworks retains occurrence/attempt/delivery state. D-055 puts first adapter delivery in06 and reporting consumption here. No runtime completion is claimed.
