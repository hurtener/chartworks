# Phase 13 — engineering-pipelines (Wave 4)

> **Status:** draft
> **Owner:** orchestrator (Opus-staffed — difficulty: high)
> **Depends on:** phase-06-jobs-scheduler, phase-09-sql-validate-core,
> phase-10-exec-read, phase-12-engineering-profiling

---

## RFC / request sections

- **RFC-001 §1.2 (P1c — the write split)** — the governing invariant of this phase.
- **RFC-001 §7.3** — transformations & pipelines (definition model, write-shaped
  allowlist, LLM-assisted drafting, the canonical identity registry).
- **RFC-001 §7.5** — versioning, lineage, freshness (declared, not inferred).
- **RFC-001 §7.6** — materializations, the governed write path (`sources.Materializer`,
  `create-or-replace` + `full refresh`, declared destinations, fail-loud quality checks).
- **RFC-001 §7.7** — refresh & scheduling (attach to the phase-06 dispatcher/queue,
  skip-if-running).
- **RFC-001 §6.1** — the adapter seam's separation of the write interface from `Query`.
- **RFC-001 §9.5** — the AST validation machinery whose **write-shape variant** (declared
  inputs / one declared output, single statement) phase 09 ships for this phase to consume.
- **RFC-001 §12** — the budgeted schema: `pipelines`, `pipeline_runs`, `datasets`
  (lineage/freshness/version), `data_sources.writable_destinations`, `schedules`,
  `schedule_runs`, `jobs`, `audit_events`. This phase adds **no new table or column**.
- **RFC-001 §14** — config surface (the `pipelines` config domain added below).
- Decisions: **D-013** (full DE stage in V1), **D-017** (write-posture split — NLQ
  read-only, DE writes through a distinct governed path), **D-021** (split read/write
  interfaces; `ValidatedSQL` the only executable type), **D-023** (Bruin mine-ideas-only —
  no engine dependency), **D-025** (one generic leased job queue), **D-020** (grants:
  `manage` on the destination source gates a write), **D-004** (store vs. customer-warehouse
  boundary).

## Depends on

- **phase-06 (`jobs-scheduler`)** — pipeline runs execute as a typed `pipeline_run` handler
  on the one generic leased queue; schedule attachment enqueues into the same queue via the
  dispatcher (D-025). This phase registers a handler and a schedule target; it does not
  build a second runner (P7).
- **phase-09 (`sql-validate-core`)** — consumes the **write-shape validator variant**
  (single statement, writes only the declared output, reads only declared inputs) that
  phase 09 ships explicitly "for phase 13's use". Produces the opaque `exec.WriteValidatedSQL`
  the materializer requires.
- **phase-10 (`exec-read`)** — reuses the read seam only for the profile/preview reads a
  draft needs; the write path is *distinct* from `exec.Query` (P1c).
- **phase-12 (`engineering-profiling`)** — pipeline drafting is grounded in dataset
  profiles + freshness; the canonical registry annotates profiled datasets; a
  materialization produces a new dataset that phase-12 profiling can then re-profile.

## Informing briefs

Per `docs/research/INDEX.md` (`internal/engineering` → primary **11**, secondary **09, 10,
12, 02**): this phase cites **`docs/research/11-ssr-de-pipeline-draft.md`**,
**`docs/research/10-bruin-engine-evaluation.md`**, **`docs/research/02-predecessor-data-and-execution.md`**.

## Brief findings incorporated

- **Brief 11 — whitelist-only composition / "the LLM composes, never authors free SQL".**
  The write-shape allowlist (RFC §7.3) is the concrete form: assisted drafting proposes
  steps, but a step is admissible only if it validates against the write-shape variant
  (one statement, declared output only, declared inputs only). The model never gets a raw
  write channel.
- **Brief 11 — contract-before-materialize, human-gated.** Assisted drafting yields a
  **draft** pipeline; nothing materializes without an explicit `pipeline.manage`-scoped
  **publication gate**. The gate is scoped narrowly to the publish transition, not a person
  on every step.
- **Brief 11 — the canonical identity registry ("resolve then compare, never compare
  names"), mint-once, fail-closed on ambiguity.** Adopted as the single tenant-scoped
  vocabulary shared by pipeline assistance and topic generation (P7). An unconfident
  resolution stops and escalates — it never mints a duplicate identity (the brief's named
  "one truly silent, unrecoverable failure mode").
- **Brief 11 — grow-by-addition / immutable versioning.** A published pipeline definition
  is never mutated in place; an edit branches a new version. Mirrors the store's
  forward-only discipline.
- **Brief 10 — Bruin's pipeline model as design input only (D-023).** The quality-check
  families (`not_null`, `unique`, `accepted_values`, row-count bounds, referential
  integrity, freshness) and the per-check **blocking** flag are mined as the shape of this
  phase's quality checks; the declared-materialization + declared-lineage + cron/interval
  scheduling contract is mined as the shape of §7.6/§7.7. **No Bruin code, CLI, or engine is
  a dependency** — the cgo+Rust/Python collisions with D-005 are exactly why (brief 10 §6).
- **Brief 02 — one generic leased job queue for background work; declared lineage; the
  fork's destination discipline.** Pipeline runs ride the phase-06 queue (D-025), lineage
  is recorded from the declared definition (never inferred by SQL parsing — §7.5), and the
  write path carries the fork's "fail loud, never silently default to a connection" posture
  through the declared-destination check.

## Findings I'm departing from

- **Brief 11 — the five-rung demand-driven medallion, the blind planner, and the
  self-unfolding content-addressed execution graph: DITCHED for V1.** V1 pipelines are
  *flat, ordered, human-authored (or human-published) SQL steps*, not an autonomous
  bronze/silver/gold auto-matching ladder. The brief itself flags the full machinery as
  "a large matching engine to build before there's evidence demand-driven scoping saves
  effort … in the same wave as the NLQ-core migration"; the RFC scopes it out. Only the
  registry, whitelist-composition, and contract-gate keepers survive.
- **Brief 11 — the refresh/drift pipeline (fingerprints, value baselines, silent-meaning-
  shift detection): DITCHED for V1.** The brief admits it "isn't actually specified." V1
  refresh is a scheduled **full re-materialization** (`create-or-replace` / `full refresh`)
  on the phase-06 dispatcher — no drift fingerprinting. Schema-drift *detection* on sources
  is phase-12's re-discovery diff, not a pipeline concern here.
- **Brief 11 — the assumed dbt/SQLMesh/Cocoon/external-orchestrator toolchain: DITCHED
  (D-023).** No external engine; the phase-06 queue orders runs, the declared step order
  orders steps, the adapter's `Materializer` executes them.
- **Brief 10 — incremental materialization strategies: DEFERRED (RFC §7.6, §19).** V1 ships
  `create-or-replace table` and `full refresh insert` only; incremental is post-V1.
- **Brief 11's topic-pack↔registry integration** is designed here only far enough to keep
  *one* vocabulary (P7); the pack-side read is phase-15's to wire (see the wiring note in
  Design → Canonical identity registry).

## Scope

Delivers, in `internal/engineering` (extending the package phase-12 created):

- **Pipeline definition model** — versioned; ordered SQL steps, each with declared inputs
  (upstream dataset refs), exactly one declared output, and optional per-step quality
  checks; a pipeline-level declared destination `(source_id, schema)`. Persisted in the
  budgeted `pipelines.definition_json` / `pipelines.version` / `pipelines.destination`.
- **Write-shape validation** of every step, delegated to phase-09's write-shape variant
  (`exec.ValidateWriteShape → exec.WriteValidatedSQL`).
- **`sources.Materializer`** — the write interface, distinct from `sources.Query`,
  implemented only by destination-capable drivers (postgres in V1); strategies
  `create-or-replace` and `full refresh`; writes only to a **declared destination**
  (`data_sources.writable_destinations` ∩ a `manage` grant on that source).
- **Run orchestration** — a `pipeline_run` typed handler on the phase-06 queue; step
  execution in declared order; per-step quality checks; run status/stats/error into
  `pipeline_runs`; loud failure on a failed check.
- **Lineage + freshness stamping** — at materialization time, from the declared definition,
  onto the output `datasets` row (upstream ids + pipeline id + step; version bump; freshness
  → `fresh`).
- **Schedule attachment** — a `schedules` row targeting a pipeline; dispatched by phase-06;
  skip-if-running overlap policy.
- **LLM-assisted drafting** — the `pipeline_draft` gateway role (phase-05), schema-
  constrained, grounded in profiles + the canonical registry; produces a **draft** pipeline
  only; a distinct, `pipeline.manage`-scoped **publication gate** is the sole path to a
  runnable/materializable pipeline.
- **The canonical identity registry** — a tenant-scoped, resolve-then-compare vocabulary
  (mint-once, fail-closed) shared by pipeline assistance and (by interface) topic
  generation.

## Non-goals

- Incremental materialization strategies (post-V1, §7.6/§19).
- Drift/refresh fingerprinting (§19; V1 refresh = scheduled full re-materialization).
- Condition/event schedule triggers (post-V1, §7.7); V1 is cron/interval only (phase 06).
- The autonomous demand-driven medallion / blind planner / auto-DAG (brief-11 departures).
- Any external pipeline engine (D-023).
- Topic-pack authoring/reads of canonical entities (phase 15 wires the pack side).
- New store tables/columns — the §12 budget is closed; a durable canonical-registry table,
  if authoring later needs one, is an RFC §12 amendment + a decision entry, **not** silently
  added here.
- HTTP/MCP surfaces for pipelines — the endpoints in RFC §11.2 land in phase 21; this phase
  ships the core the surfaces call (P7).

## Design

### Data flow

```
draft (assisted, pipeline_draft role) ─┐
hand-authored (Console/HTTP core) ─────┴─▶ DRAFT pipeline (definition_json, version N)
                                                     │  publication gate (pipeline.manage
                                                     │  scope + manage grant on destination)
                                                     ▼
                                            PUBLISHED pipeline (immutable version N)
                                                     │  run (manual :run  OR  schedule tick → phase-06 queue)
                                                     ▼
                              pipeline_run handler (one leased job, D-025)
   for each step in declared order:
     write-shape validate (exec.WriteValidatedSQL) ─▶ Materializer.Materialize(dest, strategy, wsql)
        └ quality checks ─ fail ⇒ run fails LOUDLY (status+metric+audit), stop, no downstream write
        └ pass ⇒ stamp datasets.lineage_json + version++ + freshness=fresh
                                                     ▼
                              pipeline_runs row (status, stats_json, error_json)
```

### Pipeline definition model

A `PipelineDefinition` is versioned and immutable-by-version (brief 11 grow-by-addition):

- `Step`: `{ id, ordinal, name, sql_text, declared_inputs []DatasetRef,
  declared_output DatasetRef, quality_checks []QualityCheck }` — exactly **one** declared
  output; at least the output must be materializable to the pipeline destination.
- `PipelineDefinition`: `{ steps []Step (ordered), destination Destination{source_id,
  schema}, dialect (= destination source's dialect) }`, persisted in
  `pipelines.definition_json`; `pipelines.version` is the monotonic version;
  `pipelines.status ∈ {draft, published}`; `pipelines.destination` mirrors the pair for
  indexed lookup.
- **Immutability:** editing a published pipeline writes a *new* `pipelines` version row
  (grow-by-addition); the published version's `definition_json` is never mutated in place
  (the topic-lifecycle discipline, applied to pipelines).

### Write-shape validation (phase-09 variant)

Every step's `sql_text` passes `exec.ValidateWriteShape(ctx, sql, dialect, declaredInputs,
declaredOutput)`, which enforces (all typed errors, P4):

- exactly one statement (no multi-statement);
- the statement writes **only** the declared output (a write to any other target ⇒
  `pipeline.step.write_out_of_scope`);
- every read reference ∈ declared inputs (a read of an undeclared dataset ⇒
  `pipeline.step.input_not_declared`) — the whitelist-composition discipline (brief 11);
- no DDL/DML outside the sanctioned write shape of the chosen strategy.

Success yields an opaque **`exec.WriteValidatedSQL`** — the *only* type the `Materializer`
accepts, constructible only inside `internal/exec` (the write-side twin of `ValidatedSQL`,
D-021). A raw string cannot be materialized — structurally, not by convention.

### The `Materializer` interface (distinct from `Query` — P1c)

```
// in internal/sources — a SEPARATE interface from the read-only Query seam.
type Materializer interface {
    Materialize(ctx, dest Destination, strategy Strategy, wsql exec.WriteValidatedSQL) (MaterializeResult, error)
}
```

- **Distinct interface, distinct type.** `Query(ctx, ValidatedSQL, …)` (read) and
  `Materialize(ctx, dest, strategy, WriteValidatedSQL)` (write) share no entry point and no
  executable type. No flag selects read-vs-write on a shared method (D-017). Drivers that
  cannot write do not implement `Materializer`; the `null`/`mock` drivers implement it
  fail-loud / in-memory for tests.
- **Declared destinations only.** `dest` must be a `(source_id, schema)` pair present in
  `data_sources.writable_destinations` **and** the caller must hold a `manage` grant on that
  source (D-020). Either miss ⇒ typed `materialize.destination_undeclared` (or
  `access.none` at the grant check), **no write issued** (P1a short-circuit).
- **Strategies (V1):** `create-or-replace table` (drop-and-recreate the output) and
  `full refresh insert` (truncate-and-reload). Both are the whole-table write shapes the
  write-shape validator admits. Incremental is post-V1.
- **P1c architecture proof.** A build-time architecture test asserts **no package under
  `internal/nlq` and no read-path package imports `sources.Materializer` or
  `exec.WriteValidatedSQL`**; the NLQ/exec read seam exposes no write operation. This is the
  mechanical guard that NLQ-generated or BYO SQL can never reach a write — the standing
  P1c test that lands *with this phase*, not at the end (master-plan risk register).
- **Defense-in-depth on the write, mirroring the read.** As the read path re-enforces
  read-only at the adapter (brief 02's headline scar), the write path re-checks the declared
  destination at the adapter boundary — the validator's write-shape gate is not the sole
  guarantee.

### Run orchestration (phase-06 queue)

A `pipeline_run` job kind registers a typed handler on the one leased queue (D-025). The
handler:

1. Loads the **published** definition (a draft is not runnable — see the gate).
2. Runs steps in declared `ordinal` order. Per step: write-shape validate → `Materialize`
   → run quality checks.
3. **Quality checks** (families mined from brief 10): row-count bounds, `not_null`,
   `unique`, `accepted_values`, referential integrity, freshness; each carries a `blocking`
   flag (default configurable). A failed **blocking** check **fails the run loudly**: run
   status `failed`, a typed `pipeline.quality_check_failed` error into
   `pipeline_runs.error_json`, a `pipeline_quality_check_failures_total` metric increment,
   and a content-free `audit_events` row — **no downstream step materializes** (P4; never a
   silent partial publish, §7.6).
4. On success per step: **stamp lineage + freshness** (below). Run status `succeeded`,
   `pipeline_runs.stats_json` carries per-step row counts + durations.

Overlap/idempotency ride phase-06 (skip-if-running; lease/heartbeat/reclaim). Missed-window
policy is skip-if-running + log + metric (§7.7).

### Lineage + freshness stamping

At each successful materialization, from the **declared** definition (never inferred —
§7.5): the output `datasets` row gets `lineage_json = { upstream: declared_inputs,
pipeline_id, step_id }`, `version` bumped (schema/re-materialization bump), and `freshness =
fresh` stamped at run time. Lineage is recorded **per materialization** — the master-plan
criterion.

### Schedule attachment

A `schedules` row with `target = pipeline`, `target_id = pipeline_id`, and a cron/interval
`trigger_json` attaches a published pipeline to the phase-06 dispatcher, which enqueues one
`pipeline_run` job per due window into the same queue. Pause/resume via `schedules.status`.
`schedule_runs` records each dispatch outcome. No new machinery — phase 06 owns the
dispatcher; this phase provides the `pipeline` target.

### LLM-assisted drafting (draft-only, explicit publication gate)

`POST /pipelines:draft` (core here; HTTP wrapper phase 21) calls the `pipeline_draft`
gateway role (phase-05), **schema-constrained** (P5 — never free-text JSON), grounded in
dataset profiles (phase 12) + the canonical registry. Output is a **draft** pipeline
(`status = draft`) whose steps are each write-shape-validated before persistence (an
un-validatable proposed step is dropped/typed-errored, never stored as runnable). A draft is
**not runnable and not materializable**; running one yields `pipeline.not_published`. The
**publication gate** — `pipeline.manage` scope + `manage` grant on the destination source —
is the single transition `draft → published` (contract-before-materialize, brief 11). This
is the human-in-the-loop point, scoped to publication, not to every step.

### The canonical identity registry (where it lives; P7, one vocabulary)

A tenant-scoped registry mapping a business entity/term → its **canonical id** + **governing
dataset** + **key columns** ("resolve then compare, never compare names"; brief 11). Rules:

- **Mint-once, fail-closed.** A concept's canonical id is minted the first time it is
  conformed; every later mention **resolves** against it. An unconfident resolution stops
  and escalates (a typed `canonical.resolution_ambiguous`) — it never mints a duplicate
  (brief 11's unrecoverable failure mode).
- **One vocabulary (P7).** Both pipeline assistance (this phase) and topic generation
  (phase 15) resolve through the *same* `CanonicalRegistry` interface. Canonical entities
  flow into `topic_versions.pack_json` at generation exactly as §8.1 specifies — a copy of
  the resolved registry, **not a second map**.
- **Where it lives (ambiguity resolved — see below).** RFC §7.3 says the registry is
  "stored as part of the semantic layer (§8), not a second vocabulary." But §12 budgets no
  `canonical_registry` table and phase 13 ships **before** phase 15 (which, per the current
  master-plan dep list, does *not* name phase 13). To honor the budget *and* P7 *and* the
  ship order: phase 13 defines and owns the `CanonicalRegistry` **component and interface**
  in `internal/engineering`, and persists canonical facts as **annotations on existing
  `datasets` rows** (`schema_json` — a governing dataset + its key columns are already
  dataset-native facts; the canonical id + business terms are added to that JSONB). **No new
  table.** Phase 15 consumes the identical interface (P7). A dedicated durable table, if
  authoring outgrows dataset annotations, is an RFC §12 amendment + a decision entry.
  **Wiring obligation recorded:** phase 15 must consume this component; the master-plan
  phase-15 dependency list should gain `13` (a reasonable-deviation flag for the Wave-4→5
  checkpoint, per §4.3 — surfaced, not silently taken).

### How it upholds the properties

- **P1c** — the write split is the spine: distinct `Materializer` interface, distinct
  `WriteValidatedSQL` type, declared destinations only, and a build-time architecture test
  proving the NLQ path cannot reach any of it.
- **P1a** — a materialization requires a `manage` grant on the destination source, checked
  in the write path; no grant ⇒ no write (short-circuit).
- **P3** — every `pipelines`/`pipeline_runs`/`datasets`/`schedules` read+write carries the
  non-optional tenant scope; a materialization targets a tenant-owned, tenant-marked
  destination only.
- **P4** — a failed quality check, an undeclared destination, an unvalidatable step, and an
  ambiguous canonical resolution are each a typed error + metric + audit; never a silent
  partial publish or a silent duplicate mint.
- **P5** — drafting is the only model call and goes through the gateway `pipeline_draft`
  role, schema-constrained.
- **P7** — one job queue (phase 06), one write interface, one canonical vocabulary, one
  validation core (phase 09's write-shape variant); the surfaces call this core (phase 21).

## Config keys added

New `pipelines` config domain (RFC §14). Documented here; the implementing PR adds each to
the example config and asserts presence in a config smoke check (CLAUDE.md §4.2).

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `pipelines.max_steps` | int | 50 | no | Upper bound on steps per pipeline definition; over-limit ⇒ typed validation error. |
| `pipelines.materialize_timeout` | duration | 300s | no | Server-side statement timeout for a materialization step (the write analogue of `exec.statement_timeout`). |
| `pipelines.draft_enabled` | bool | true | no | LLM-assisted drafting on/off; off ⇒ hand-authoring only (drafting endpoint returns a typed `unavailable`). |
| `pipelines.quality_check_default_blocking` | bool | true | no | Default `blocking` flag for a quality check that does not set one (brief 10). |
| `pipelines.max_quality_checks_per_step` | int | 20 | no | Bound on checks per step; over-limit ⇒ typed validation error. |

## Acceptance criteria

1. **Write-shape: out-of-scope write rejected.** A step whose SQL writes to any target other
   than its declared output is rejected at write-shape validation with a typed
   `pipeline.step.write_out_of_scope`; the step never reaches the `Materializer`.
2. **Write-shape: undeclared input rejected.** A step whose SQL reads a dataset not in its
   declared inputs is rejected with a typed `pipeline.step.input_not_declared`.
3. **Undeclared destination unmaterializable.** `Materialize` to a `(source, schema)` not in
   `data_sources.writable_destinations` (or without a `manage` grant on that source) returns
   a typed error and issues no write (proven by adapter/mock call-count = 0).
4. **Failed quality check fails the run loudly.** A blocking quality-check failure sets the
   `pipeline_runs` status to `failed`, records a typed `pipeline.quality_check_failed`,
   increments `pipeline_quality_check_failures_total`, writes a content-free audit row, and
   materializes no downstream step.
5. **P1c architecture proof.** A build-time architecture test proves no `internal/nlq`
   package and no read-path package imports `sources.Materializer` or
   `exec.WriteValidatedSQL`; the read seam exposes no write operation.
6. **Lineage recorded per materialization.** Each materialized output `datasets` row carries
   `lineage_json` with the declared upstream ids + pipeline id + step, and a bumped
   `version`.
7. **Freshness stamped.** A successful materialization stamps the output dataset
   `freshness = fresh` at run time.
8. **Draft-only publication gate.** An assisted-draft pipeline has `status = draft` and is
   not runnable; running it yields a typed `pipeline.not_published`; only a
   `pipeline.manage`-scoped publish transitions it to `published`/runnable.
9. **Versioned + immutable.** Editing a published pipeline creates a new `pipelines` version
   row; the published version's `definition_json` is unchanged (no in-place mutation).
10. **Materializer strategies + unforgeable write type.** `create-or-replace` and
    `full refresh` both materialize a declared destination through `exec.WriteValidatedSQL`;
    `WriteValidatedSQL` is unconstructible outside `internal/exec` (compile-time proof) — a
    raw string cannot be materialized.
11. **Schedule attachment enqueues on the phase-06 queue.** Attaching a cron/interval
    schedule to a published pipeline enqueues one `pipeline_run` job per due window; an
    overlapping due window is skipped-if-running (log + metric).
12. **Canonical registry — resolve-then-compare, fail-closed.** A term resolves to
    `(canonical_id, governing_dataset, key_columns)` via the registry, never by name-vs-name
    comparison; an unconfident resolution returns a typed `canonical.resolution_ambiguous`
    and mints no duplicate id; the same interface backs pipeline assistance and topic
    generation (one vocabulary).

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven write-shape rejections (criteria 1–2), strategy rendering
  (criterion 10), definition versioning/immutability (criterion 9), canonical
  resolve/mint/fail-closed (criterion 12), quality-check evaluation incl. the blocking flag
  (criterion 4). Golden tests on the normalized pipeline-definition shape and the typed
  error codes.
- **Integration (required — closes seams phase 06/09/10/12 opened; real drivers):** a
  full **draft → publish → run → materialize** round-trip against **Docker Postgres**
  (`make pg-up`) using a real destination in a writable schema: proves lineage/freshness
  stamping (6–7), the `pipeline_run` handler on the real phase-06 queue (11), and the
  declared-destination write path end-to-end. The gateway `mock` driver backs the
  `pipeline_draft` call (paired with a recorded-fixture test for the role). A schedule tick
  enqueuing a `pipeline_run` is exercised against the real dispatcher.
- **Adversarial (required — this phase adds a write path + a grant-gated destination):**
  a cross-tenant materialization probe (a `manage` grant in tenant A must not authorize a
  write to tenant B's destination); an empty-access-set write short-circuit; a
  write-smuggling step (DDL/second-statement/out-of-scope-target) rejected by the write-shape
  validator; the **P1c architecture test** (criterion 5) as a standing guard; a
  fetch-then-filter regression guard on destination resolution.
- **Fuzz:** `FuzzValidateWriteShape` (seed corpus of step SQL) asserting the invariant
  "never panics, never admits a statement that writes outside the declared output or reads
  an undeclared input" — the write-side twin of phase-09's `FuzzValidate`.
- **Bench:** `BenchmarkMaterializeStep` on the hot pipeline-run path (baseline, not a gate);
  the `pipeline_run` handler and the `CanonicalRegistry` are shared artifacts ⇒
  concurrent-reuse tests under `-race`.

## Coverage targets

Per CLAUDE.md §11 defaults (80% new packages). `internal/engineering` was banded at 80% by
phase 12; this phase keeps that band (no store-driver/auth/conformance code lives here, so
the 85% band does not apply). The write-shape validator additions live in `internal/exec`
(banded 85% by phase 09) — the write-shape functions this phase relies on stay within that
band.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/engineering` | 80% | new-package default (band already present from phase 12; unchanged). |

## Smoke checks

`scripts/smoke/phase-13.sh` sources `scripts/smoke/lib.bash` and, guarded on the
`internal/engineering` package existing, runs one `go test -run` assertion per criterion via
`run_group` (SKIPping any not-yet-built criterion, SKIPping the whole script until the
package exists).

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestPipelineStepWriteOutOfScopeRejected` PASS |
| 2 | `TestPipelineStepInputNotDeclaredRejected` PASS |
| 3 | `TestMaterializeUndeclaredDestinationRejected` PASS |
| 4 | `TestQualityCheckFailureFailsRunLoudly` PASS |
| 5 | `TestP1cWriteSplitArchitecture` PASS |
| 6 | `TestLineageRecordedPerMaterialization` PASS |
| 7 | `TestFreshnessStampedOnMaterialization` PASS |
| 8 | `TestDraftPublicationGate` PASS |
| 9 | `TestPipelineVersionImmutable` PASS |
| 10 | `TestMaterializerStrategiesAndUnforgeableWriteType` PASS |
| 11 | `TestScheduleAttachmentEnqueues` PASS |
| 12 | `TestCanonicalRegistryResolveThenCompare` PASS |

## Glossary additions

New terms this phase introduces, pre-written for `docs/glossary.md` (same PR, CLAUDE.md §14);
existing entries (**Pipeline**, **Materialization**, **Lineage**, **Freshness**) are reused
unchanged:

- **Pipeline step** — one ordered, declarative unit of a pipeline: a single SQL statement
  with declared inputs (upstream datasets), exactly one declared output, and optional quality
  checks (RFC §7.3).
- **Declared / writable destination** — a `(data source, schema)` pair a tenant admin has
  explicitly marked writable and holds a `manage` grant on; the only place a materialization
  may write (RFC §7.6, P1c).
- **Write-shape validation** — the phase-09 validator variant a pipeline step must pass:
  single statement, writes only the declared output, reads only declared inputs; yields the
  unforgeable `WriteValidatedSQL` (RFC §7.3/§9.5, D-021).
- **Materialization strategy** — the V1 write shapes: `create-or-replace table` and
  `full refresh insert` (RFC §7.6).
- **Pipeline run** — one execution of a published pipeline as a typed job on the leased
  queue; recorded in `pipeline_runs` with status, stats, and any typed error (RFC §7.6/§7.7,
  D-025).
- **Publication gate** — the single, `pipeline.manage`-scoped `draft → published` transition;
  nothing materializes until it passes (contract-before-materialize, brief 11).
- **Canonical identity registry** — the tenant-scoped, mint-once, fail-closed vocabulary
  mapping a business entity/term to its canonical id + governing dataset + key columns;
  shared by pipeline assistance and topic generation, resolve-then-compare, never
  name-vs-name (RFC §7.3, P7).

## Decisions filed

No new `docs/decisions.md` entry. This phase implements existing decisions: **D-013**
(full DE stage), **D-017** (write-posture split — P1c), **D-021** (split read/write
interfaces, unforgeable executable types), **D-023** (Bruin mine-ideas-only, no engine),
**D-025** (one leased queue), **D-020** (grants gate the write), **D-004** (store vs.
customer-warehouse boundary). If the canonical registry outgrows dataset annotations and
needs a durable table, that is an RFC §12 amendment + a superseding decision entry (§15) —
flagged, not pre-committed here.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). Known candidate at authoring time:
     the master-plan phase-15 dependency list should gain `13` so topic generation
     consumes this phase's CanonicalRegistry (P7, one vocabulary) — raise at the Wave-4→5
     checkpoint and update docs/plans/README.md in that PR. -->
