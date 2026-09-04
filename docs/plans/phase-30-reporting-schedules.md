# Phase 30 — reporting-schedules

Status: planned. Owner: internal/jobs, internal/reporting. Hard dependencies: 06, 18, 23, 28, 29.

## Authority and design

RFC-002 §7, `docs/contracts/pengui-authority.md`, D-048/D-051 and [COMMON.md](COMMON.md) apply. Scheduling is domain work in Chartworks; identity and permission issuance remain Pengui's. This phase wires the first real asynchronous authority consumer to Pengui's existing broker contract.

## Brief findings incorporated

Briefs 02, 14; source coverage Q01–Q11. Preserve all real schedule target and run/catalog behavior, but discard event/condition/condition-check/custom-code stubs.

## Findings I'm departing from

No local service accounts, permission expansion by selecting a stronger account, persisted end-user bearer, fresh local JWT or guessed broker API. Recipient metadata is not email-sent evidence. Idempotency is logical, not universal exactly-once remote execution.

## Scope and implementation tasks

1. Implement real targets for reviewed saved SQL, dynamic saved questions, direct certified block/output selection and published reports using the existing queue.
2. Wire a thin ExecutionAuthorityProvider to Pengui's existing broker, binding authorization at admission and validating fresh JWTs at each occurrence/retry/checkpoint.
3. Implement test/pause/resume/update/retire, exact recurrence/window/revision policies, bounded budgets and catalog delivery with optional Pengui notification intent.

The owning implementation first reads the actual Pengui broker request/response contract, records it in the consumer handoff and supplies an adapter fixture plus actual integration consumer. No invented endpoint may be treated as existing. Admission needs signed use reach for the opaque execution binding and the target/dependencies. Each attempt obtains a fresh Pengui JWT and passes it through the ordinary verifier/enforcer. Missing/denied renewal produces a blocked outcome; it never falls back to ambient credentials. This port also closes long-lived semantic/profile/report operations that use phase 06.

## Non-goals

No new authentication service, mail delivery platform, generic event processor or arbitrary scheduled code.

## Config and persistence

Scheduler default no overlap/no unbounded catch-up, retry ceiling/backoff, timezone database version, max elapsed/model/warehouse attempts, Pengui broker connection reference and optional notification integration. Persist exact due instant, half-open window, selected revision manifest, target, binding reference and delivery intent/receipt independently. Retry uses the original occurrence clock; a schedule edit changes only future unaccepted occurrences.

## Acceptance criteria

1. **AC01** — All four target types execute their actual domain path; direct block schedules create no hidden report and dynamic targets have explicit opt-in.
2. **AC02** — Admission prevents stronger-binding selection; current Pengui refusal/expiry/missing scope stops work. No account creation or stored user bearer is used.
3. **AC03** — Cron/interval/DST/leap/first/missed occurrence windows are specified and persisted as half-open instants; retries never recompute from wall clock.
4. **AC04** — Pinned/default and explicit latest-published policies resolve once; pause/resume/update/retire/test/history are durable and CAS-safe.
5. **AC05** — Overlap/catch-up/backoff/cancel/timeout/fencing and per-occurrence model/query budgets are tested with concurrent workers and crashes.
6. **AC06** — Artifact retention, catalog publication and notification intent/receipt are distinct; recipient metadata is not email-sent evidence or a data grant.
7. **AC07** — Missing outputs/dependencies/approval/authority mark attention/blocked states instead of silently repinning or using ambient credentials.
8. **AC08** — Event/condition/condition-check/unrestricted custom schedule kinds are absent from registration; maintenance cleanup remains bounded internal work.

## Tests, coverage and smoke

Implement `TestPhase30/AC01` through `TestPhase30/AC08`. Use the actual authority adapter contract, real queue/store/targets and a deterministic timezone clock. Refusal/expiry tests assert zero fresh source work; crash tests cover post-query/pre-delivery uncertainty. Exercise fixed/relative periods and exact schedule mutation races. COMMON.md sets coverage and evidence; `scripts/smoke/phase-30.sh` requires all eight results.

## Glossary, decisions and deviations

Execution binding and delegated JWT are supplied by Pengui; occurrence/attempt/delivery state belong to Chartworks. D-048 applies. The exact broker integration is an implementation deliverable, not a capability claimed tested in this document.
