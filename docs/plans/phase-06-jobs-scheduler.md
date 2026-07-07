# Phase 06 — jobs-scheduler

> **Status:** draft
> **Owner:** orchestrator (Wave 2)
> **Depends on:** phase-02-store-migrations

Authored per CLAUDE.md §16. This phase delivers `internal/jobs`: the single generic
leased job queue, typed handler registration, the worker-pool runner, and the
cron/interval schedule dispatcher — all on the `jobs` / `schedules` / `schedule_runs`
tables already created by phase 02 (RFC §12). It adds **no migration** (see Design).

---

## RFC / request sections

- **RFC §3.3 (Runtime shape)** — one process hosting the worker pool + the schedule
  dispatcher; **one generic leased queue** (Postgres `FOR UPDATE SKIP LOCKED`, lease +
  heartbeat + reclaim) with typed handlers, replacing the predecessors' 13 bespoke
  worker classes; expensive work is async, never on a query's hot path.
- **RFC §7.7 (Refresh & scheduling)** — schedules (cron or interval only in V1) attach to
  a pipeline or a saved query and dispatch runs into the jobs queue; missed-window /
  overlap policy is **skip-if-running, log + metric**.
- **RFC §17 (Operational shape)** — graceful shutdown: job leases release, in-flight runs
  checkpoint status.
- **RFC §12** — the `jobs`, `schedules`, `schedule_runs` table shapes (owned/migrated by
  phase 02); this phase consumes them.
- **RFC §14** — the `jobs` config domain (worker concurrency, lease timeout, heartbeat
  interval) plus the dispatcher poll/batch defaults.
- **RFC §15** — `jobs queue depth / lease reclaims` and schedule-skip metrics.
- **D-025** — one generic leased job queue + typed handlers + schedule dispatcher; no
  per-concern worker classes (the P7 counterexample, brief 01).

## Depends on

- **phase-02-store-migrations** — the `jobs`, `schedules`, `schedule_runs` tables, the
  `Store` seam, the pgx/v5 driver, the fresh-DB conformance harness, and the mandatory
  non-optional `tenant_id` scope discipline. This phase closes the queue seam over those
  tables; it needs their DDL and the `Store` transaction primitives to exist first.

## Informing briefs

Per `docs/research/INDEX.md` (row "background jobs / scheduling → primary 01, 02"):

- `docs/research/01-predecessor-architecture.md`
- `docs/research/02-predecessor-data-and-execution.md`

## Brief findings incorporated

From **brief 01**:

- **The 13-worker scar is the design driver (D-025).** Both predecessors grew one
  `Worker` class per background concern — 13 co-deployed poll-loops, each with its own
  concurrency/lease config, all sharing the API process's envelope. Brief 01's open
  question 4 ("N workers vs. one generic runner with typed handlers") is answered here in
  favour of the latter: **one** runner, **one** claim protocol, handlers registered *by
  kind*. Adding a background concern is a handler registration, never a new poll-loop.
- **The lease-claim shape is the proven substrate.** Every predecessor worker followed
  one shape: construct with a shared registry, spin `N` tasks each polling
  `store.claim_job(...)` on an interval, process one, sleep, repeat until a shutdown event
  — a store-backed leased queue with no external broker. That shape is inherited (Go
  goroutine pool over the pgx store), minus the per-type duplication.
- **Startup fail-fast must be *consistent* (brief 01 scar).** The predecessors made
  router-cache refresh fatal but embedding warmup non-fatal — an inconsistency P4 forbids.
  This phase's config validation (heartbeat < lease) and any construction error are
  **fail-loud at boot**, uniformly.
- **Schedule "cancel" that only set a DB status without interrupting in-flight work is a
  named gap, not a precedent (brief 01 scar).** Graceful shutdown here actually drains
  in-flight handlers (honouring context cancellation) before releasing leases.

From **brief 02**:

- **One generic `jobs` queue, kept separate from the user-facing cron/interval
  scheduler.** Brief 02's keeper: `jobs` (`FOR UPDATE SKIP LOCKED`, `lease_owner` /
  `lease_expires_at` refreshed by heartbeats, `lease_timeout_seconds`) backs all
  maintenance; the `ScheduleDispatcher` polls due `schedules` and enqueues **one job per
  due schedule into that same queue**. This phase reproduces exactly that seam boundary:
  the dispatcher is a *producer* into the one queue, not a second execution substrate.
- **Reclaim-on-lease-lapse is evidenced, not inferred (brief 02).** "A crashed worker's
  job becomes reclaimable once its lease lapses" — carried as the reclaim protocol, with
  an `attempts`-bounded dead-letter so a poison job cannot reclaim forever.
- **The dispatcher's own numbers.** Brief 02 records a default 15s dispatcher poll and a
  100-schedule due-batch cap — carried as `jobs.dispatcher_poll_interval` (15s) and
  `jobs.dispatcher_batch_size` (100).
- **Overlap / missed-window policy = skip-if-running (brief 02 + RFC §7.7).** The
  dispatcher skips a schedule whose prior run has not reached a terminal state, logging +
  metering the skip rather than stacking runs.

## Findings I'm departing from

- **The 13-worker cardinality (brief 01/02).** Deliberately not carried — it is the
  D-025 counterexample. One runner + typed handlers replaces it.
- **`condition` / `event` trigger kinds (brief 01/02 stubs).** The predecessors carried
  four trigger kinds; V1 ships **cron + interval only** (RFC §7.7, D-013 scope note).
  Unsupported kinds are a typed rejection, never a silent accept.
- **`job_batch_checkpoints` and progress-percent/progress-stage columns (brief 02).** Not
  in the RFC §12 budgeted `jobs` shape; not reintroduced. Progress is out of V1 scope for
  the queue (checkpointing on shutdown is a status write, §Design, not a new table).
- **A regex/heuristic layer of any kind.** N/a here, but noted: the queue does no content
  inspection; correctness comes from the atomic claim and the `attempts` bound, not
  guesswork.

## Scope

`internal/jobs`, delivering:

1. **The queue** — `Queue` over the `Store`: `Enqueue(ctx, scope, Job)` (tenant-scoped,
   no unscoped variant), atomic `claim` (`SELECT … FOR UPDATE SKIP LOCKED`, priority
   desc / age asc, picking pending **or** lease-expired rows), heartbeat, complete
   (succeed/fail), and lease reclaim.
2. **The handler registry** — a `Handler` interface keyed by a `Kind` newtype; typed
   payload decode; unknown-kind-fails-loud.
3. **The runner** — a worker pool of `jobs.concurrency` goroutines; per-job heartbeat
   goroutine; graceful drain + lease release on shutdown. A reusable, concurrency-safe
   artifact (immutable after construction; per-job state in locals/context).
4. **The dispatcher** — a `Dispatcher` polling `schedules` for due rows, evaluating
   `cron` / `interval` triggers, enqueuing **idempotently per due-window** into the same
   queue, applying skip-if-running overlap policy, and recording `schedule_runs`.
5. **Telemetry** — queue-depth gauge, `jobs_lease_reclaims_total`,
   `jobs_completed_total{kind,outcome}`, `schedule_dispatch_total`,
   `schedule_skips_total{reason}` (all registered so the phase-01 telemetry conformance
   test sees them exported).
6. **Config** — the `jobs` config domain with fail-loud validation (below).

Constructed once in `cmd/chartworks` (the runner + dispatcher are started by `serve`);
this phase wires the construction and its graceful-shutdown hook.

## Non-goals

- Concrete handlers (profiling, topic generation, publishing, pipeline runs, refresh) —
  each lands with its owning phase (12/13/15/…), registering its `Kind`. This phase ships
  only the infra + test/mock handlers.
- `condition` / `event` triggers (post-V1, RFC §7.7).
- Incremental progress reporting / `job_batch_checkpoints` (not budgeted, §Departing).
- Any HTTP/MCP surface for schedules or jobs — the `Schedules` HTTP group is phase 21;
  agents never administer jobs (RFC §11.1).
- Any migration — see Design (the tables exist from phase 02; idempotency needs no new
  column).

## Design

### Vocabulary posture (P6)

`job` and `worker` are on the RFC §2 **forbidden wire/UI list**. They are used freely in
this plan and in `internal/jobs` code (internal words, permitted there), but **never** on
an API field, MCP tool, error message, or human-read log line. User-facing operations are
named by their domain verb — a schedule run surfaces as "refreshing", "publishing",
"running a pipeline", never "job N started". `schedule` *is* an allowed domain noun (it
appears on the RFC §11.2 HTTP surface); the queue and runner behind it are not named
outward.

### The `jobs` table semantics (RFC §12; created by phase 02)

`jobs`: `job_id` (PK), `tenant_id` (NOT NULL, non-empty — P3), `kind`, `status`,
`priority`, `attempts`, `lease_owner`, `lease_expires_at`, `payload` / `result` / `error`
JSONB, timestamps. Status lifecycle:

```
pending ─claim─▶ running ─complete─▶ succeeded
   ▲               │ lease lapses / shutdown        └▶ failed (terminal)
   └──────reclaim──┘ (attempts++; > max_attempts ⇒ failed dead-letter)
```

`payload` is our own schema-typed JSON (decoded by the handler for its kind — no free-text
parse). `result` / `error` are content-free enough to hold no customer rows or secrets
(P4 / §7).

### Claim / lease / heartbeat / reclaim protocol

- **Claim** (one transaction, `FOR UPDATE SKIP LOCKED`): select the highest-priority,
  oldest row whose `status = pending` **or** (`status = running` **and**
  `lease_expires_at < now()`) — the second disjunct is reclaim. Set `status = running`,
  `lease_owner = <runner-instance-id>`, `lease_expires_at = now() + lease_timeout`, and on
  a reclaim increment `attempts`. `SKIP LOCKED` gives single-delivery under concurrent
  claimers with no double-claim.
- **Heartbeat**: while a handler runs, a heartbeat goroutine every `heartbeat_interval`
  issues `UPDATE … SET lease_expires_at = now()+lease_timeout WHERE job_id=$1 AND
  lease_owner=$me`. If it updates **0 rows** (the lease was reclaimed elsewhere), the
  handler's context is cancelled — cooperative single-execution, closing the "double-run
  after reclaim" hazard. `heartbeat_interval < lease_timeout` is a boot invariant.
- **Reclaim bound**: on claim of a reclaimed row, if `attempts > max_attempts`, the row is
  written terminal `failed` with a typed `jobs.max_attempts_exceeded` error + a metric,
  never re-run — a poison job cannot reclaim forever (brief 02 gap closed).
- **Complete**: `UPDATE … SET status=succeeded|failed, result/error=…, lease_owner=NULL`
  in one statement; a handler-returned error → `failed` (loud: status + `error` JSONB +
  metric + structured content-free log, P4).

Every queue method takes a non-optional tenant scope on the paths that address a specific
tenant's rows; the runner's *claim* is intentionally tenant-agnostic (it is infrastructure
draining a shared queue), but the claimed row's `tenant_id` is read from the row and
carried into the handler's `JobContext` — a handler never runs without its job's tenant,
and cannot widen it (P3). There is **no** unscoped `Enqueue`.

### Typed handler registration by kind

```
type Kind string
type Handler interface {
    Kind() Kind
    Handle(ctx context.Context, jc JobContext) error   // jc.Tenant(), jc.Decode(&payload)
}
type Registry // Register(Handler); Lookup(Kind) (Handler, bool)
```

Registration happens at construction (owning phases register their handler; a blank-import
`init()` seam is available per §4.4). A claimed job whose `kind` has no registered handler
is **failed loud** — typed `jobs.unknown_kind` error, status `failed`, metric — never
silently dropped or infinitely retried (P4). `Kind` values are effectively a closed set:
an unregistered kind can never be claimed to success.

### Worker-pool runner lifecycle + graceful shutdown

`Runner` starts `jobs.concurrency` goroutines. Each: claim → if none, sleep
`claim_poll_interval` → else run its handler under a per-job context with the heartbeat
goroutine attached → complete → loop. The `Runner` is immutable after construction and
safe under concurrent reuse (per-job state is in locals + context, never receiver fields);
proven with a `-race` concurrent test.

**Graceful shutdown** (RFC §17): on context cancel / SIGTERM the runner (a) stops claiming
new jobs, (b) waits up to `jobs.shutdown_grace` for in-flight handlers to return, then (c)
**releases** their leases — `UPDATE … SET status=pending, lease_owner=NULL,
lease_expires_at=NULL` — so a surviving instance reclaims immediately instead of waiting
out the lease. In-flight status is checkpointed (the release write *is* the checkpoint;
no partial-success is reported as success). Nothing is lost or double-completed.

### Cron / interval dispatcher enqueuing into the same queue

`Dispatcher` polls `schedules` every `jobs.dispatcher_poll_interval` for up to
`jobs.dispatcher_batch_size` due rows (`status=active`, next-due `<= now()`), tenant-scoped
per row. For each due schedule:

1. **Trigger evaluation** — `trigger_json` is `cron` or `interval`; a pure function
   computes the current **due-window** (window start) and the next due time. `condition` /
   `event` kinds are rejected with a typed `schedule.unsupported_trigger` error (fail-loud,
   never a silent skip-to-accept).
2. **Overlap policy (skip-if-running)** — if the schedule's prior run is not terminal
   (a job for `(target, target_id)` still `pending`/`running`), skip: increment
   `schedule_skips_total{reason="overlap"}` and emit a structured content-free log; do not
   enqueue.
3. **Idempotent enqueue per due-window** — enqueue **one** job with a **deterministic**
   `job_id = uuidv5(schedule_id ‖ due_window_start)` via `INSERT … ON CONFLICT (job_id)
   DO NOTHING`. A second dispatcher tick — or a second process instance — evaluating the
   same schedule inside the same window computes the identical `job_id`, so the insert is a
   no-op: exactly-once per due-window, using only the existing `jobs.job_id` primary key
   (**no new column, no new index, no migration**). Record the `schedule_runs` row
   `(schedule_id, run_id=job_id, outcome, ts)`.

The dispatcher is a producer into the one queue (D-025 / brief 02); the target's handler
(pipeline run, saved-query run) is owned by a later phase.

### Seams and properties

- **Store seam (§4.4 / D-004)** — all durable state via the phase-02 `Store`; the queue is
  Chartworks' *own* state, never the customer warehouse.
- **P3** — tenant travels on every row and into every handler; no unscoped enqueue.
- **P4** — every failure (handler error, unknown kind, max-attempts, unsupported trigger,
  bad config) is a typed error + metric + content-free log; no silent drop, no empty-catch,
  no inconsistent fail-fast.
- **P7** — one queue, one claim protocol, one handler contract; the dispatcher shares the
  queue substrate rather than forking a second one.

## Config keys added

All under the `jobs` domain (RFC §14). No secrets, so no `env:` indirection needed; all
have contractual defaults. Documented here, added to the example config, and covered by a
config-validation smoke check in the implementation PR (§4.2). Fail-loud validators
(refused boot on violation, P4): `concurrency ≥ 1`; `heartbeat_interval > 0` **and**
`heartbeat_interval < lease_timeout`; `max_attempts ≥ 1`; all intervals `> 0`;
`dispatcher_batch_size ≥ 1`.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `jobs.concurrency` | int | 4 | no | Worker-pool goroutine count. |
| `jobs.lease_timeout` | duration | 60s | no | Lease TTL; a lapsed lease is reclaimable. |
| `jobs.heartbeat_interval` | duration | 15s | no | Must be `> 0` and `< lease_timeout`. |
| `jobs.claim_poll_interval` | duration | 2s | no | Idle poll interval when no job is claimable. |
| `jobs.max_attempts` | int | 5 | no | Reclaim/retry ceiling before dead-letter. |
| `jobs.shutdown_grace` | duration | 30s | no | Drain window for in-flight handlers on shutdown. |
| `jobs.dispatcher_poll_interval` | duration | 15s | no | Schedule due-poll interval (brief 02 default). |
| `jobs.dispatcher_batch_size` | int | 100 | no | Max due schedules processed per tick (brief 02 default). |

## Acceptance criteria

1. **Single delivery under concurrency (`-race`).** With `jobs.concurrency > 1` and M
   pending jobs across multiple claimers, every job's handler is invoked **exactly once**
   (no double-claim); the race detector is clean. *(master-plan: `-race` concurrent
   handlers)*
2. **Reclaim after a killed worker.** A handler goroutine abandoned mid-run (its lease no
   longer heartbeated) has its job reclaimed by another claimer once `lease_expires_at`
   passes, with `attempts` incremented — proven with a killed/abandoned goroutine.
   *(master-plan: lease reclaim after killed worker)*
3. **Reclaim is bounded.** A job reclaimed past `max_attempts` is written terminal
   `failed` with `jobs.max_attempts_exceeded` and `jobs_lease_reclaims_total` /
   completion metrics; it is never re-run.
4. **Heartbeat extends the lease.** A long-running handler that heartbeats is **not**
   reclaimed while alive: `lease_expires_at` advances and no second claimer picks the row.
5. **Unknown kind fails loud.** A claimed job whose `kind` has no registered handler
   becomes terminal `failed` with a typed `jobs.unknown_kind` error + metric — never
   silently dropped, never infinitely retried.
6. **Graceful shutdown releases leases (`-race`).** On shutdown, in-flight jobs are
   drained within `shutdown_grace`, then their leases are released (`status=pending`,
   `lease_owner=NULL`) so another instance reclaims immediately; no job is lost or
   double-completed.
7. **Tenant scope propagates.** A job's non-empty `tenant_id` is delivered to its handler
   via `JobContext`; there is no unscoped `Enqueue`, and a handler cannot observe another
   tenant's job (P3).
8. **Overlap policy skips + logs + metric.** A schedule whose prior run is still
   non-terminal is skipped on the next due tick; `schedule_skips_total{reason="overlap"}`
   increments and a structured content-free log emits — no run is stacked.
   *(master-plan: overlap skip+log+metric)*
9. **Idempotent dispatch per due-window.** Two dispatcher ticks (or two instances)
   evaluating the same schedule in the same due-window enqueue **exactly one** job; the
   second `INSERT … ON CONFLICT DO NOTHING` is a no-op. *(master-plan: idempotent dispatch
   per due-window)*
10. **Trigger catalog is correct and closed.** `cron` and `interval` triggers compute the
    expected next-due / due-window across a table-driven fixture set; `condition` /
    `event` kinds are rejected with `schedule.unsupported_trigger` — never silently
    accepted.
11. **Config validation fails loud.** A `jobs` config with `heartbeat_interval >=
    lease_timeout` (or `concurrency < 1`, or a non-positive interval) is a **refused boot**
    with a typed config error — never a silent clamp.

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven trigger next-due / due-window computation (cron + interval) and
  unsupported-kind rejection (criterion 10); handler registry lookup + unknown-kind path
  (5); config validators (11); heartbeat 0-row → context-cancel logic.
- **Integration (required — closes the queue seam over phase-02's `jobs`/`schedules`
  tables; introduces `Queue`/`Runner`/`Dispatcher` that later phases build on).** Real
  Docker Postgres (`make pg-up`), `-race`: single-delivery concurrency (1), reclaim (2),
  bounded reclaim (3), heartbeat-extends (4), graceful-shutdown lease release (6),
  tenant-scope propagation (7), overlap skip (8), idempotent per-window dispatch (9). No
  boundary mock — the store is the real driver. Store-URL-unset ⇒ `t.Skip` (surfaces as a
  clean smoke SKIP).
- **Adversarial:** limited to a **tenant-scope regression guard** (a job's tenant travels
  into its handler; no unscoped enqueue exists — a compile-time/architecture assertion).
  The registry-driven cross-tenant route/tool probe is phase 04/21/22's surface, not this
  package's (jobs is not an ACL/auth path). Full cross-tenant probe: **n/a here.**
- **Fuzz:** **n/a** — the queue decodes only Chartworks-authored JSON payloads (schema-typed,
  not untrusted wire). Trigger `trigger_json` is covered by table-driven bad-input
  rejection tests (criterion 10) rather than a `FuzzXxx` target; revisit if a trigger
  grammar ever accepts caller free-text.
- **Bench:** `BenchmarkClaim` on the hot atomic claim path (a reusable artifact under
  concurrent contention). `make bench` baseline, not a CI gate.

## Coverage targets

Per CLAUDE.md §11 default (80% new `internal/` packages — `internal/jobs` is not one of the
85% store/vindex/auth/identity/access/exec set). Added to
`scripts/coverage-bands.conf` in this PR.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/jobs` | 80% | Default for a new `internal/` package. The lease/reclaim/dispatch paths are exercised against Docker Postgres, so the band is reachable hermetically under `make pg-up`. |

## Smoke checks

`scripts/smoke/phase-06.sh` runs the `internal/jobs` tests (via `run_group`, `-race`) and
SKIPs cleanly when the package or the Docker Postgres URL is absent.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestClaimSingleDeliveryConcurrent` PASS (`-race`) |
| 2 | `TestReclaimAfterKilledWorker` PASS |
| 3 | `TestReclaimRespectsMaxAttempts` PASS |
| 4 | `TestHeartbeatExtendsLease` PASS |
| 5 | `TestUnknownKindFailsLoud` PASS |
| 6 | `TestGracefulShutdownReleasesLeases` PASS (`-race`) |
| 7 | `TestJobTenantScopePropagates` PASS |
| 8 | `TestScheduleOverlapSkipsLogsMetric` PASS |
| 9 | `TestScheduleDispatchIdempotentPerWindow` PASS |
| 10 | `TestTriggerCatalogAndUnsupported` PASS |
| 11 | `TestJobsConfigValidationFailsLoud` PASS |

## Glossary additions

New internal-seam terms (P6: internal only — never surfaced outward). Pre-written for
`docs/glossary.md` "Internals & seams", landed in the implementation PR:

- **Leased job queue** — the single generic Postgres-backed queue (`FOR UPDATE SKIP
  LOCKED`) on which all background work runs as typed handlers; replaces the predecessors'
  13 worker classes (D-025). Internal word; never on the wire.
- **Lease** — a claimed job's time-boxed ownership (`lease_owner` + `lease_expires_at`);
  refreshed by heartbeats, reclaimable once lapsed.
- **Heartbeat** — the periodic lease-extension write that keeps a live handler's job from
  being reclaimed; a 0-row heartbeat (lease lost) cancels the handler's context.
- **Reclaim** — re-claiming a job whose lease lapsed (crashed/slow worker); bounded by
  `max_attempts` before a terminal dead-letter.
- **Job kind** — the typed discriminator selecting a registered `Handler`; an unregistered
  kind fails loud.
- **Schedule dispatcher** — the poller that evaluates cron/interval triggers and enqueues
  due runs (idempotently per due-window) into the one queue; a producer, not a second
  substrate.
- **Due-window** — the deterministic time bucket a schedule fires in; the idempotency key
  (`uuidv5(schedule_id ‖ window)`) guaranteeing exactly-once enqueue per window.
- **Overlap policy (skip-if-running)** — the missed-window rule: skip (log + metric) a
  schedule whose prior run is still non-terminal, rather than stacking runs.

## Decisions filed

No new decisions. This phase implements existing ones:

- **D-025** — the one generic leased queue + typed handlers + schedule dispatcher (the
  entire phase).
- **D-004** — the queue is Chartworks' own `Store` state, distinct from customer sources.
- **D-013 / RFC §7.7** — cron + interval triggers only; condition/event deferred.

Any settled deviation discovered in implementation is filed as D-032+ and logged below.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). Empty at authoring time. -->

none yet.
