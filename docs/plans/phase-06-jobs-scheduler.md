# Phase 06 — jobs-scheduler

Status: planned. Owner: internal/jobs. Hard dependencies: 02, 03, 04.

## Authority and design

RFC-001 §12, RFC-002 §7, D-048/D-051 and [COMMON.md](COMMON.md) apply. One queue and occurrence engine serve engineering and reporting. Domain target handlers arrive with their owning phases; the real reporting broker adapter is phase 30.

## Brief findings incorporated

Briefs 01, 02, 14: co-deployed durable work, logical idempotency, current execution attribution, schedule windows and bounded cleanup.

## Findings I'm departing from

Reject event/condition/condition-check/unrestricted custom-code stubs. Do not persist a caller JWT, mint a replacement locally or claim physical exactly-once external work.

## Scope and implementation tasks

1. Implement one PostgreSQL SKIP LOCKED queue with durable attempts, leases, heartbeats/fences, bounded handlers and idempotent acceptance.
2. Implement cron/interval occurrence production and manual tests, unique occurrence keys, persisted windows and no-overlap/bounded catch-up policies.
3. Keep execution binding references, not caller JWTs, for long-lived work. Reject event/condition/custom-code schedule kinds; bounded housekeeping is internal.

## Non-goals

No second scheduler/queue, service-account registry, external message bus, workflow platform or unbounded custom jobs.

## Config and persistence

`jobs.concurrency=4`; heartbeat shorter than lease; bounded attempts/backoff; scheduler overlap=skip and missed_run=skip by default, optional bounded catch_up. No event/condition configuration. Occurrences and attempts use tenant-composite uniqueness; secret tokens remain transient. Authority acquisition is an injected narrow Pengui client port, exercised with signed test tokens here and the real adapter in phase 30.

## Acceptance criteria

1. **AC01** — Concurrent dispatch inserts one logical occurrence; request-key conflicts cannot silently replace accepted work.
2. **AC02** — Crash/reclaim and stale-owner fencing prevent conflicting commits; remote indeterminate effects remain recorded as uncertain.
3. **AC03** — Only implemented cron/interval/manual behavior is advertised; event/condition/condition-check/unrestricted custom targets are rejected.
4. **AC04** — Worker/global/tenant limits, retry backoff, cancellation and graceful shutdown are enforced with a deterministic clock.
5. **AC05** — Occurrence time/window/target resolution survive restart and retry; overlap and missed-run records are explicit, not silently lost.
6. **AC06** — Workers validate transient current authority supplied by the shared Pengui client before privileged work; no persisted/locally renewed user bearer.

## Tests, coverage and smoke

Implement `TestPhase06/AC01` through `TestPhase06/AC06` with real PostgreSQL, multiple worker instances, a deterministic clock and kill/reclaim/fence scenarios. COMMON.md supplies coverage and strict smoke rules. `scripts/smoke/phase-06.sh` requires all six results; stub success and missing tests are failures.

## Glossary, decisions and deviations

Occurrence, attempt, lease fence and execution binding are shared terms. D-048/D-051 separate logical delivery from external effects. No runtime completion is claimed.
