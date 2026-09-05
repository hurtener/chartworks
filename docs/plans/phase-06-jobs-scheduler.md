# Phase 06 — jobs-scheduler

Status: planned. Owner: internal/jobs. Hard dependencies: 02, 03, 04.

## Authority and design

RFC-001 §12, RFC-002 §7, D-048/D-051/D-055 and [COMMON.md](COMMON.md) apply. One queue and occurrence engine serves early profiling/semantics as well as later reporting. Fresh Pengui authority is consumed here with the first durable worker; the concrete adapter must not wait for phase 30.

## Brief findings incorporated

Briefs 01, 02, 14: co-deployed durable work, logical idempotency, execution attribution, fixed occurrence windows and bounded cleanup. Pengui remains the only issuer/access-policy owner.

## Findings I'm departing from

Remove the previous phase-30-only authority-adapter dependency, which left early jobs without a real consumer. No new issuer or guessed Pengui endpoint: reuse its actual supported broker/binding API or make a Pengui-owned extension. Reject event/condition/condition-check/custom-code stubs, persisted user JWTs and universal exactly-once claims.

## Scope and implementation tasks

1. Implement one PostgreSQL SKIP LOCKED queue with durable attempts, leases, heartbeat/fences, bounded handlers and idempotent acceptance.
2. Implement actual cron/interval occurrence production, manual test runs, unique occurrence keys, persisted windows and no-overlap/bounded catch-up policies. Domain target handlers arrive with their owning phases; unbuilt handlers are not advertised.
3. Implement the thin injected ExecutionAuthorityProvider against the actual Pengui contract and pair it with a bounded maintenance worker over existing operation/audit retention records. Record the platform endpoint/schema/version and signed refusal/success fixtures; extend Pengui as needed rather than inventing a local auth service.
4. Admission stores the authorized opaque execution binding, target and attribution, not token bytes. Each dispatch/retry obtains fresh authority, validates it through phases03/04 and enforces exact target/context before privileged work. Renewal cannot change the accepted window/revision/actor attribution. Missing authority blocks the attempt explicitly.

## Non-goals

No second scheduler/queue, local service-account/token registry, external message bus, general workflow or unrestricted custom jobs. Reporting-specific target/delivery policy remains phase30; the shared authority adapter does not.

## Config and persistence

jobs.concurrency=4; heartbeat shorter than lease; bounded attempts/backoff; overlap=skip and missed_run=skip with explicit bounded catch_up. Use the platform connection secret reference for the real provider adapter; no signing key or retained user bearer. Occurrences/attempts have tenant-composite uniqueness. No event/condition settings. Maintenance is fixed bounded internal behavior, never arbitrary user-supplied executable code.

## Acceptance criteria

1. **AC01** — Concurrent dispatch accepts one logical occurrence; a reused request key with different content cannot replace accepted work.
2. **AC02** — Crash/reclaim and stale-owner fencing prevent competing commits; indeterminate external effects stay recorded as uncertain.
3. **AC03** — Only implemented cron/interval/manual and bounded maintenance behavior is registered; unsupported event/condition/condition-check/custom-code kinds are rejected.
4. **AC04** — Worker/global/tenant admission, retry backoff, cancellation and joined shutdown are enforced with deterministic clock and concurrent fixtures.
5. **AC05** — Occurrence time/window/target survive restart/retry; overlap and missed-run outcomes remain explicit rather than lost or recalculated from today's clock.
6. **AC06** — The actual Pengui authority adapter and first durable consumer execute together: fresh signed success enables only the accepted work; denial/expiry/missing reach/binding escalation causes zero protected work, and no user bearer is stored or locally renewed.

## Tests, coverage and smoke

Implement TestPhase06/AC01 through TestPhase06/AC06 with real PostgreSQL and the actual platform adapter over recorded contract responses plus applicable Pengui integration evidence. Use multiple workers, deterministic time and kill/reclaim/fence cases. A stub TokenProvider alone does not close AC06. Later domain phases reuse the same provider without new auth plumbing. COMMON.md supplies coverage; scripts/smoke/phase-06.sh requires six runtime passes.

## Glossary, decisions and deviations

Occurrence, attempt, fence and execution binding remain shared terms. D-055 moves the real adapter/first consumer earlier; it does not move identity ownership. No runtime completion is claimed.
