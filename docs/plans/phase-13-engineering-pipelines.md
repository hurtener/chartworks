# Phase 13 — engineering-pipelines (Wave 4)

> **Status:** draft
> **Owner:** orchestrator (Opus-staffed — difficulty: high)
> **Depends on:** phase-06-jobs-scheduler, phase-09-sql-validate-core,
> phase-10-exec-read, phase-12-engineering-profiling

---

## RFC / request sections

- **RFC-001 §1.2 (P1c — the write split)** — the governing invariant, in its
  **strengthened D-036 shape**: the NLQ adapters have **no write capability at all**;
  writes live behind the `PipelineRunner` seam in a separate executor process.
- **RFC-001 §7.3** — transformations & pipelines (definition model, write-shaped
  allowlist, LLM-assisted drafting, the canonical identity registry).
- **RFC-001 §7.5** — versioning, lineage, freshness.
- **RFC-001 §7.6 (amended, D-036)** — materializations executed by **Bruin (pinned
  v0.11.666)** as a CLI subprocess behind the **`PipelineRunner` seam**: render-at-run-time,
  custody-injected connections, SQL-only assets, telemetry disabled, `bruin validate -o
  json` gating and `bruin lineage -o json` feeding lineage, exit codes → typed outcomes.
- **RFC-001 §7.7** — refresh & scheduling (phase-06 dispatcher/queue, skip-if-running).
- **RFC-001 §9.5** — the validation machinery whose **write-shape variant** (declared
  inputs / one declared output, single statement) phase 09 ships for this phase.
- **RFC-001 §12** — the budgeted schema: `pipelines`, `pipeline_runs`, `datasets`,
  `data_sources.writable_destinations`, `schedules`, `schedule_runs`, `jobs`,
  `audit_events`. This phase adds **no new table or column**.
- **RFC-001 §14** — config surface (the `pipelines` config domain below).
- **RFC-001 §17 (amended, D-037)** — the container is the deployment unit; the reference
  image carries the pinned `bruin` binary (glibc-dynamic ⇒ glibc base image — a phase-25
  constraint this plan records as a handoff note); bare-metal degrades loud.
- Decisions: **D-036** (Bruin as the DE write executor — narrows-and-supersedes D-023),
  **D-037** (container as deployment unit; D-005 reversed per its clause), **D-013** (full
  DE stage), **D-017** (write-posture split), **D-021** (write-shape validation +
  unforgeable validated types), **D-025** (one leased queue), **D-020** (grants gate the
  write), **D-004** (store vs. customer-warehouse boundary), **D-035** (pin discipline —
  Bruin's daily release cadence ⇒ strict pin + conformance re-run per bump).

## Depends on

- **phase-06 (`jobs-scheduler`)** — pipeline runs execute as a typed `pipeline_run` handler
  on the one generic leased queue (D-025); the handler wraps the `PipelineRunner`
  invocation; schedule attachment dispatches into the same queue. Graceful shutdown awaits
  or lease-reclaims an in-flight Bruin run with status checkpointed (RFC §17).
- **phase-09 (`sql-validate-core`)** — consumes the **write-shape validator variant**
  (single statement, writes only the declared output, reads only declared inputs) shipped
  "for phase 13's use". Produces the opaque `exec.WriteValidatedSQL` — the only SQL the
  renderer will emit into a Bruin asset.
- **phase-10 (`exec-read`)** — the read seam backs the profile/preview reads drafting
  needs; the strengthened P1c architecture test spans `internal/exec` (no write capability
  anywhere on the read path).
- **phase-12 (`engineering-profiling`)** — drafting is grounded in dataset profiles +
  freshness; the canonical registry annotates profiled datasets; a materialized output is
  re-profiled by phase-12 machinery.

## Informing briefs

Per `docs/research/INDEX.md` (`internal/engineering` → primary **11**, secondary **09, 10,
12, 02**): this phase cites **`docs/research/11-ssr-de-pipeline-draft.md`**,
**`docs/research/10-bruin-engine-evaluation.md`**, **`docs/research/02-predecessor-data-and-execution.md`**.

## Brief findings incorporated

- **Brief 10 — Bruin's pipeline model, now adopted as the executor (D-036).** The brief's
  §4 findings are load-bearing: the materialization strategies (create+replace,
  delete+insert, append, merge/incremental, time_interval, scd2), the quality-check
  families with per-check `blocking` flags, machine-readable `validate`/`lineage` output,
  and the per-invocation CLI shape (brief 10 §7 flagged long-lived in-process embedding as
  unproven — the subprocess-per-run boundary D-036 chose is exactly the usage pattern the
  brief found evidence for). The brief's disqualifiers (CGo/Rust library build, Python
  `ingestr` ingestion, plaintext YAML credentials) are each neutralized by D-036's hard
  rules rather than ignored: subprocess not library, SQL-only assets, custody-rendered
  connections.
- **Brief 10 — the credential-model warning carried as a criterion.** Brief 10 §3/risk
  table: Bruin's default is plaintext YAML fields. D-036's answer — `${ENV}` interpolation
  or a custody-rendered tmpfs config, plaintext never on persistent disk — is acceptance
  criterion 12 here, not a convention.
- **Brief 11 — whitelist-only composition.** Unchanged: the write-shape allowlist (one
  statement, declared output only, declared inputs only) bounds what any step — assisted or
  hand-authored — may contain, *before* rendering to Bruin. Bruin executes; it never widens
  what a step may do (Chartworks validation runs first, `bruin validate` second).
- **Brief 11 — contract-before-materialize, human-gated.** Unchanged: assisted drafting
  yields a **draft**; the `pipeline.manage`-scoped **publication gate** is the sole path to
  a runnable pipeline. Brief 11's orchestration keepers remain Chartworks-owned — Bruin is
  the executor, not the designer (D-036).
- **Brief 11 — the canonical identity registry (mint-once, fail-closed,
  resolve-then-compare).** Unchanged: one tenant-scoped vocabulary shared by pipeline
  assistance and topic generation (P7).
- **Brief 11 — grow-by-addition / immutable versioning.** Unchanged: published definitions
  are never mutated in place; edits branch a new version.
- **Brief 02 — one generic leased job queue; declared destinations; the fork's fail-loud
  connection discipline.** Unchanged: runs ride the phase-06 queue; no declared destination
  ⇒ no write ever; a missing `bruin` binary fails loud with a typed "pipeline execution
  unavailable" (D-037), never a silent skip — the `NullWarehouseAdapter` posture applied to
  the runner seam.

## Findings I'm departing from

- **Brief 10's headline verdict ("mine-ideas-only, do not adopt as a CLI subprocess") is
  deliberately superseded — by decision, not silently.** D-036 narrows-and-supersedes
  D-023 on new evidence (spike-verified SQL-only runs need no Python; machine-readable
  validate/lineage; D-037 removing the single-static-binary invariant). This plan follows
  D-036; the brief's objections survive as the hard rules and criteria above.
- **Brief 11 — the five-rung demand-driven medallion, the blind planner, and the
  self-unfolding content-addressed execution graph: DITCHED for V1.** V1 pipelines are
  flat, ordered, human-published SQL steps. Only the registry, whitelist-composition, and
  contract-gate keepers survive.
- **Brief 11 — the refresh/drift fingerprinting pipeline: DITCHED for V1.** The brief
  admits it "isn't actually specified." V1 refresh is a scheduled re-materialization on
  the phase-06 dispatcher. Schema-drift detection on sources is phase-12's re-discovery
  diff.
- **Brief 11 — the assumed dbt/SQLMesh/external-orchestrator toolchain: still ditched.**
  D-036 adopts Bruin as the *executor inside a run node* — precisely the slot brief 11 §4
  assigned to "delegate to dbt" — while the orchestration (queue, gates, destinations,
  audit) stays Chartworks-owned.
- **Brief 10 — Bruin's Python/R assets and `ingestr` ingestion: FORBIDDEN in V1** (FSL-1.1
  license + Python runtime — D-036 hard rule; criterion 11).
- **Brief 11's topic-pack↔registry integration** designed only far enough for one
  vocabulary (P7); the pack-side read is phase-15's to wire.

## Scope

Delivers, in `internal/engineering` (extending the package phase 12 created):

- **Pipeline definition model** — versioned; ordered SQL steps, each with declared inputs,
  exactly one declared output, optional per-step quality checks; a pipeline-level declared
  destination `(source_id, schema)`; a per-step **materialization strategy** from the
  inherited set. Persisted in the budgeted `pipelines` columns.
- **Definition validation** — phase-09 write-shape validation of every step
  (`exec.WriteValidatedSQL`); **SQL-only enforcement** (a definition declaring a Python/R
  asset or any ingestion asset is rejected, typed); **strategy/engine compatibility**
  (per-engine variance recorded per driver — e.g. Databricks lacks merge; an unsupported
  pair is a typed validation error, never a runtime surprise).
- **The `PipelineRunner` seam** — interface + factory + driver (§4.4); V1 driver **`bruin`**
  (pinned v0.11.666, CLI subprocess): renders the declarative definition to Bruin's
  pipeline format at run time in a tmpfs-backed workdir; injects connections via `.bruin.yml`
  `${ENV}` interpolation or a custody-rendered tmpfs config; disables Bruin telemetry on
  every invocation; gates on `bruin validate -o json`; executes; parses per-asset results;
  consumes `bruin lineage -o json`. A `mock` runner driver backs tests.
- **Run orchestration** — a `pipeline_run` typed handler on the phase-06 queue wrapping the
  runner; exit codes and per-asset results mapped to **typed run outcomes** into
  `pipeline_runs`; blocking quality-check failures fail the run loudly.
- **Lineage + freshness stamping** — declared inputs remain the authoritative
  `datasets.lineage_json` source (§7.5); `bruin lineage -o json` is recorded as per-run
  lineage evidence and cross-checked against the declaration (divergence ⇒ loud typed
  warning, P4); version bump + `freshness = fresh` on success.
- **Schedule attachment** — a `schedules` row targeting a pipeline, dispatched by phase 06,
  skip-if-running.
- **LLM-assisted drafting** — `pipeline_draft` gateway role, schema-constrained, grounded
  in profiles + the canonical registry; draft-only; explicit publication gate.
- **The canonical identity registry** — tenant-scoped, mint-once, fail-closed,
  resolve-then-compare; shared with topic generation by interface (P7).

**Implementation blocker (D-036, resolved before any Bruin invocation ships):** confirm
Bruin's **real telemetry-disable mechanism** against the pinned v0.11.666 binary (env var
vs. flag vs. config key — verified empirically, convention 8), and encode it in the driver
+ criterion 13. The finding lands in this plan's deviation log.

## Non-goals

- Bruin as a **library import** (the D-023 objections to that shape stand: CGo/Rust build,
  mega-factory dependency graph — D-036 adopts the subprocess boundary only).
- Python/R assets, `ingestr`/SaaS ingestion (D-036 hard rule; the *enforcement* is in
  scope, the capability never is).
- Drift/refresh fingerprinting (§19); condition/event triggers (post-V1, §7.7).
- The autonomous demand-driven medallion / blind planner (brief-11 departures).
- Topic-pack reads of canonical entities (phase 15 wires the pack side).
- New store tables/columns — the §12 budget is closed.
- HTTP/MCP surfaces for pipelines (phase 21; this phase ships the core — P7).
- The reference Dockerfile / glibc base image (**phase 25** — recorded here as a handoff
  constraint: the pinned `bruin` binary is glibc-dynamic; debian-slim class, never bare
  musl/Alpine; dev/CI images used by this phase's integration tests must already respect
  it).

## Design

### Data flow

```
draft (assisted, pipeline_draft role) ─┐
hand-authored (Console/HTTP core) ─────┴─▶ DRAFT pipeline (definition_json, version N)
        │ definition validation: write-shape (exec.WriteValidatedSQL) + SQL-only
        │ + strategy/engine compatibility + destination declared
        ▼                                    publication gate (pipeline.manage scope +
                                             manage grant on destination source)
                                    PUBLISHED pipeline (immutable version N)
                                             │ run (manual :run OR schedule tick → phase-06 queue)
                                             ▼
                          pipeline_run handler (one leased job, D-025)
                                             ▼
                          PipelineRunner seam ── driver: bruin (pinned v0.11.666)
   render definition → Bruin format (tmpfs workdir; ${ENV}/custody-rendered connections;
                                     telemetry disabled)
     → bruin validate -o json   (gate: invalid ⇒ typed error, no execution)
     → bruin run                (exit code + per-asset results → typed outcomes)
     → bruin lineage -o json    (per-run lineage evidence)
   blocking quality-check failure ⇒ run fails LOUDLY (status+metric+audit), no partial publish
   success ⇒ stamp datasets.lineage_json (declared) + version++ + freshness=fresh
                                             ▼
                          pipeline_runs row (status, stats_json, error_json)
```

### Pipeline definition model

Unchanged in shape from RFC §7.3, extended per §7.6 (amended):

- `Step`: `{ id, ordinal, name, sql_text, declared_inputs []DatasetRef,
  declared_output DatasetRef, strategy Strategy, quality_checks []QualityCheck }` —
  exactly **one** declared output per step.
- `PipelineDefinition`: `{ steps (ordered), destination Destination{source_id, schema},
  dialect }` in `pipelines.definition_json`; `pipelines.version` monotonic;
  `pipelines.status ∈ {draft, published}`. Published versions are immutable; edits branch
  a new version row (grow-by-addition, brief 11).
- `Strategy` is the **inherited Bruin set**: `create+replace`, `delete+insert`,
  `truncate+insert`, `append`, `merge` (incremental), `time_interval`, `scd2`. Per-engine
  support is recorded in a static compatibility table per destination driver (Databricks
  lacks merge — the named example); an unsupported `(strategy, engine)` pair is rejected at
  definition validation with a typed `pipeline.strategy_unsupported`, never discovered at
  run time.

### Definition validation (phase-09 variant + D-036 hard rules)

Every step passes, at authoring/publish time (all typed errors, P4):

- **Write-shape** (phase 09): exactly one statement; writes only the declared output
  (`pipeline.step.write_out_of_scope`); reads only declared inputs
  (`pipeline.step.input_not_declared`) — the whitelist-composition discipline (brief 11);
  shape admissible for the declared strategy. Success yields `exec.WriteValidatedSQL` —
  the **only** SQL the renderer will emit into a Bruin asset (unconstructible outside
  `internal/exec`; D-021's unforgeability carried to the write side: a raw string cannot
  reach the runner).
- **SQL-only** (D-036): a definition declaring any non-SQL asset kind (Python/R) or any
  ingestion asset ⇒ typed `pipeline.asset_kind_forbidden` at validation — rejected before
  persistence, long before rendering.
- **Strategy compatibility** and **destination declared** (below).

### The `PipelineRunner` seam (replaces the native `Materializer` — D-036)

```
// in internal/engineering — interface + factory + driver (§4.4).
type PipelineRunner interface {
    Validate(ctx, rendered RenderedPipeline) (ValidateReport, error)  // bruin validate -o json
    Run(ctx, rendered RenderedPipeline) (RunReport, error)            // exit code + per-asset results
    Lineage(ctx, rendered RenderedPipeline) (LineageReport, error)    // bruin lineage -o json
}
```

- **Drivers:** `bruin` (V1 — CLI subprocess, binary pinned v0.11.666, path from config,
  version asserted when pipelines are enabled: a missing or pin-mismatched binary ⇒ typed
  `pipeline.execution_unavailable`, loud, per D-037's bare-metal degradation — the rest of
  the product is unaffected) and `mock` (tests). Standard factory registration; a second
  executor is a driver, never core surgery (P7).
- **Render-at-run-time.** The published definition renders to Bruin's pipeline format
  (`pipeline.yml` + SQL assets) in a per-run, tmpfs-backed workdir
  (`pipelines.render_dir`), from `WriteValidatedSQL` step bodies only. The rendered
  project is deleted on run completion (success or failure).
- **Connection injection — plaintext never persists (D-036 hard rule).** The rendered
  `.bruin.yml` references credentials via `${ENV}` interpolation (secrets decrypted by
  phase-08 custody at invocation and passed only in the subprocess environment) or, where a
  connection shape cannot ride env interpolation, a custody-rendered config written **only**
  to the tmpfs workdir. No secret byte is ever written to a persistent path, logged, or
  echoed into an error (§6.3 custody rules apply end-to-end).
- **Telemetry disabled on every invocation** (D-036): the confirmed disable mechanism (see
  the implementation blocker) is applied unconditionally by the driver to validate, run,
  and lineage invocations alike; a test asserts its presence on every constructed
  invocation.
- **Validate gates execution.** `bruin validate -o json` runs against the rendered project
  before any `run`; a validation failure is a typed `pipeline.render_invalid` — no
  execution. Bruin's own gate sits *behind* Chartworks' write-shape gate, not instead of
  it (layered, mirroring D-038's read-side philosophy).
- **Typed outcome mapping.** Bruin exit codes and per-asset JSON results map to the typed
  run-outcome vocabulary (`pipeline.run_succeeded`, `pipeline.quality_check_failed`,
  `pipeline.step_failed`, `pipeline.render_invalid`, `pipeline.execution_unavailable`, …)
  via one table-driven mapping — never string-matching stderr prose as control flow. An
  unmappable exit ⇒ typed `pipeline.outcome_unknown`, run status `failed`, loud (P4 —
  never a silent success).
- **Destinations remain declared + granted.** A run refuses any destination not in
  `data_sources.writable_destinations` ∩ the caller's `manage` grant on that source
  (`materialize.destination_undeclared` / `access.none`) — checked **before** any render,
  so an undeclared destination never even reaches Bruin. No declared destination ⇒ no
  write, ever (P1a/P1c).

### P1c — strengthened (D-036 / RFC §7.6 amended)

The NLQ adapters now have **no write capability at all**: reads live in `internal/sources`
adapters; writes live behind `PipelineRunner` in a separate executor **process**. No shared
entry point exists to flag-switch. The build-time architecture test asserts **no package
under `internal/nlq` and none of the `internal/exec` read path imports
`engineering.PipelineRunner` or the renderer**, and that `internal/sources` adapters expose
no write method — the strongest P1c shape, landing with this phase per the master-plan risk
register.

### Run orchestration (phase-06 queue)

The `pipeline_run` handler: load **published** definition (draft ⇒ typed
`pipeline.not_published`) → destination/grant check → render → `Validate` → `Run` → map
outcomes. **Quality checks execute inside Bruin** (unique / not_null / accepted_values /
pattern / min / max / custom — blocking by default per RFC §7.6); a blocking failure
surfaces in the per-asset results and **fails the run loudly**: status `failed`, typed
`pipeline.quality_check_failed` in `pipeline_runs.error_json`,
`pipeline_quality_check_failures_total` increment, content-free audit row, no downstream
asset materialized (the outcome mapping guarantees a record-and-continue check never masks
a blocking one). Chartworks wraps every run with its own audit + metering (content-free:
pipeline, step, destination, row count, duration, outcome — RFC §7.6). Overlap, lease,
heartbeat, and reclaim ride phase 06; graceful shutdown awaits or lease-reclaims the
subprocess with status checkpointed (RFC §17).

### Lineage + freshness stamping

Declared inputs remain authoritative for `datasets.lineage_json` (§7.5 — declared, not
inferred). `bruin lineage -o json` is recorded per run as lineage **evidence**
(column-level where Bruin provides it) into `pipeline_runs.stats_json`; a divergence
between declared inputs and derived lineage raises a typed, loud warning on the run (P4 —
a declaration drifting from reality is surfaced, never silently absorbed). On success:
output dataset `version` bump + `freshness = fresh` stamped at run time.

### Schedule attachment / drafting / canonical registry

Unchanged by D-036, restated for completeness:

- **Schedules:** a `schedules` row (`target = pipeline`, cron/interval `trigger_json`)
  dispatched by phase 06 into the queue; skip-if-running + log + metric (§7.7); pause/
  resume via status; `schedule_runs` records outcomes.
- **Drafting:** the `pipeline_draft` gateway role (phase 05), schema-constrained (P5 —
  never free-text JSON), grounded in profiles (phase 12) + the canonical registry; output
  is `status = draft`, each proposed step write-shape- and SQL-only-validated before
  persistence (an un-validatable step is dropped/typed-errored, never stored as runnable);
  a draft is not runnable. The **publication gate** — `pipeline.manage` scope + `manage`
  grant on the destination source — is the single `draft → published` transition
  (contract-before-materialize, brief 11): the human-in-the-loop point, scoped to
  publication, not to every step.
- **Canonical identity registry:** tenant-scoped, mint-once, fail-closed
  (`canonical.resolution_ambiguous` — never a duplicate mint; brief 11's named
  unrecoverable failure mode), resolve-then-compare, never name-vs-name. Owned as a
  component + interface in `internal/engineering`; persisted as annotations on existing
  `datasets` rows (`schema_json` — governing dataset + key columns are dataset-native
  facts) — **no new table** (§12 budget honored; a durable table later is an RFC §12
  amendment + decision entry). Phase 15's topic generation consumes the identical
  interface (P7 — one vocabulary; canonical entities flow into `topic_versions.pack_json`
  per §8.1 as a copy of resolved entries, never a second map). Wiring obligation recorded:
  the master-plan phase-15 dep list should gain `13` — raise at the Wave-4→5 checkpoint
  (§4.3), not taken silently.

### How it upholds the properties

- **P1c (strengthened)** — no write capability in any NLQ/read adapter; writes only via
  `PipelineRunner` in a separate process, on declared + `manage`-granted destinations, from
  unforgeable `WriteValidatedSQL`; the architecture test proves `nlq`/`exec` cannot reach
  the runner.
- **P1a** — destination grant checked before render; no grant ⇒ no render, no subprocess,
  no write (short-circuit).
- **P2/§7** — custody-decrypted secrets ride the subprocess environment or tmpfs only;
  never persisted, logged, or echoed (criterion 12).
- **P3** — tenant scope on every `pipelines`/`pipeline_runs`/`datasets`/`schedules`
  read+write; a run may only target its own tenant's declared destination.
- **P4** — failed quality check, invalid render, unmappable exit, missing binary,
  undeclared destination, forbidden asset kind, ambiguous canonical resolution: each a
  typed error + metric + audit; never a silent partial publish or silent degradation.
- **P5** — drafting is the only model call, through the gateway `pipeline_draft` role,
  schema-constrained.
- **P7** — one queue (phase 06), one runner seam, one canonical vocabulary, one validation
  core (phase 09's write-shape variant); surfaces call this core (phase 21). Bruin is a
  driver behind a seam — a second executor is a driver, never a parallel path.

## Config keys added

New `pipelines` config domain (RFC §14). Each lands in the example config + a config smoke
check in the implementing PR (CLAUDE.md §4.2).

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `pipelines.bruin_path` | string | `bruin` (PATH lookup) | no | Path to the pinned Bruin binary; missing or pin-mismatched ⇒ typed `pipeline.execution_unavailable` (D-037, loud). |
| `pipelines.render_dir` | string | tmpfs default (`/dev/shm` when present, else OS temp + a loud boot warning) | no | Per-run render workdir; must be non-persistent — custody-rendered configs live only here (D-036). |
| `pipelines.run_timeout` | duration | 1800s | no | Ceiling on one `bruin run` subprocess (context deadline + process kill). |
| `pipelines.max_steps` | int | 50 | no | Upper bound on steps per definition; over-limit ⇒ typed validation error. |
| `pipelines.draft_enabled` | bool | true | no | LLM-assisted drafting on/off; off ⇒ typed `unavailable` from the draft operation. |
| `pipelines.quality_check_default_blocking` | bool | true | no | Default `blocking` flag for a check that does not set one (RFC §7.6). |
| `pipelines.max_quality_checks_per_step` | int | 20 | no | Bound on checks per step; over-limit ⇒ typed validation error. |

## Acceptance criteria

1. **Write-shape: out-of-scope write rejected.** A step whose SQL writes to any target
   other than its declared output is rejected with typed `pipeline.step.write_out_of_scope`
   at definition validation; it never reaches rendering.
2. **Write-shape: undeclared input rejected.** A step reading a dataset not in its declared
   inputs is rejected with typed `pipeline.step.input_not_declared`.
3. **Undeclared destination unmaterializable.** A run against a `(source, schema)` not in
   `data_sources.writable_destinations` (or without a `manage` grant on that source)
   returns a typed error **before rendering** — no render, no subprocess, no write (proven
   by mock-runner call-count = 0).
4. **Failed quality check fails the run loudly.** A blocking quality-check failure maps to
   run status `failed` + typed `pipeline.quality_check_failed` + a
   `pipeline_quality_check_failures_total` increment + a content-free audit row, with no
   downstream asset materialized.
5. **P1c architecture proof (strengthened).** A build-time architecture test proves no
   package under `internal/nlq` and none of the `internal/exec` read path imports
   `engineering.PipelineRunner` or the renderer, and that `internal/sources` adapters
   expose no write method — NLQ code structurally cannot reach pipeline execution.
6. **Lineage recorded per materialization.** Each run stamps the output `datasets` row's
   `lineage_json` from the declared definition (upstream ids + pipeline id + step) with a
   version bump, and records `bruin lineage -o json` output as per-run evidence; a
   declared-vs-derived divergence raises a typed, loud warning.
7. **Freshness stamped.** A successful materialization stamps the output dataset
   `freshness = fresh` at run time.
8. **Draft-only publication gate.** An assisted-draft pipeline is `status = draft` and not
   runnable (typed `pipeline.not_published`); only a `pipeline.manage`-scoped publish (with
   a `manage` grant on the destination source) transitions it to runnable.
9. **Versioned + immutable.** Editing a published pipeline creates a new version row; the
   published `definition_json` is never mutated in place.
10. **Render gated by `bruin validate`.** A definition renders to Bruin's format from
    `exec.WriteValidatedSQL` bodies only (`WriteValidatedSQL` unconstructible outside
    `internal/exec` — compile-time proof; a raw string cannot be rendered), and
    `bruin validate -o json` must pass before any execution; a validate failure is a typed
    `pipeline.render_invalid` with no run.
11. **SQL-only enforcement.** A definition declaring a Python/R asset or any ingestion
    asset is rejected at validation with typed `pipeline.asset_kind_forbidden` (D-036 hard
    rule) — never rendered, never executed.
12. **Plaintext never persists.** Rendered artifacts on any persistent path contain no
    secret bytes: connections ride `${ENV}` interpolation or a custody-rendered config
    written only to the tmpfs workdir and deleted on run completion (asserted by scanning
    the rendered project and workdir remnants for planted sentinel secrets).
13. **Bruin telemetry disabled.** Every constructed Bruin invocation (validate, run,
    lineage) carries the confirmed telemetry-disable mechanism, asserted on the driver's
    command/env construction. *Implementation blocker: the real mechanism is empirically
    confirmed against the pinned v0.11.666 binary before this ships (D-036).*
14. **Typed outcome mapping.** Bruin exit codes and per-asset results map to typed run
    outcomes via one table-driven mapping (golden-tested); an unmappable exit yields typed
    `pipeline.outcome_unknown` with run status `failed` — never a silent success, never
    stderr-prose string-matching as control flow.
15. **Strategy/engine compatibility.** The inherited strategy set (create+replace,
    delete+insert, truncate+insert, append, merge/incremental, time_interval, scd2) is
    accepted per the per-engine compatibility table; an unsupported pair (the named case:
    merge on Databricks) is rejected at definition validation with typed
    `pipeline.strategy_unsupported`.
16. **Missing executor degrades loud.** With no `bruin` binary at the configured path (or a
    version-pin mismatch), pipeline execution returns typed
    `pipeline.execution_unavailable` (D-037); the rest of the product is unaffected;
    nothing silently skips.
17. **Schedule attachment enqueues on the phase-06 queue.** A cron/interval schedule on a
    published pipeline enqueues one `pipeline_run` job per due window; an overlapping due
    window is skipped-if-running (log + metric).
18. **Canonical registry — resolve-then-compare, fail-closed.** A term resolves to
    `(canonical_id, governing_dataset, key_columns)` via the registry, never by
    name-vs-name comparison; an unconfident resolution returns typed
    `canonical.resolution_ambiguous` and mints no duplicate id; the same interface backs
    pipeline assistance and topic generation (one vocabulary).

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven write-shape rejections (1–2), SQL-only rejection (11),
  strategy/engine compatibility table (15), renderer output (golden: definition → rendered
  Bruin project shape), outcome-mapping table (14, golden), definition versioning/
  immutability (9), canonical resolve/mint/fail-closed (18), telemetry-mechanism presence
  on every invocation shape (13), quality-check blocking semantics (4).
- **Integration (required — closes seams phases 06/08/09/12 opened; real drivers):** a
  full **draft → publish → run → materialize** round-trip with the **real pinned Bruin
  binary** against **Docker Postgres** (`make pg-up`) writing to a declared
  workspace-class destination: proves render/validate/run/lineage end-to-end (6–7, 10),
  the `pipeline_run` handler on the real phase-06 queue + a real schedule tick (17), and
  env-interpolated custody injection with the sentinel-secret scan (12). Dev/CI images
  carry the pinned binary (glibc base — the phase-25 constraint applies to dev/CI now).
  The gateway `mock` driver backs `pipeline_draft` (paired with a recorded-fixture test);
  the `mock` runner driver backs unit-level orchestration tests; per D-035, a Bruin
  version bump re-runs this conformance suite.
- **Adversarial (required — write path + grant-gated destinations):** cross-tenant
  materialization probe (tenant A's `manage` grant cannot authorize a write to tenant B's
  destination); empty-access-set short-circuit (no render); a write-smuggling step
  (DDL/second statement/out-of-scope target) rejected pre-render; a forbidden-asset
  smuggle (a definition attempting a Python/ingestion asset via crafted JSON) rejected
  (11); the strengthened P1c architecture test (5) as a standing guard; the
  sentinel-secret persistence scan (12); a fetch-then-filter regression guard on
  destination resolution.
- **Fuzz:** `FuzzValidateWriteShape` (phase-09 twin — invariant: never panics, never
  admits an out-of-scope write or undeclared read) and `FuzzParseBruinOutput` over the
  validate/run/lineage JSON decoders (seed corpus recorded from the real pinned binary;
  invariant: never panics, never maps unparseable output to a success outcome).
- **Bench:** `BenchmarkRenderPipeline` (hot per-run path; baseline, not a gate). The
  `pipeline_run` handler, renderer, and `CanonicalRegistry` are shared artifacts ⇒
  concurrent-reuse tests under `-race` (including two concurrent runs rendering in
  isolated workdirs).

## Coverage targets

Per CLAUDE.md §11 defaults. `internal/engineering` was banded at 80% by phase 12; this
phase keeps that band. The write-shape validator additions live in `internal/exec` (banded
85% by phase 09) and stay within it. Subprocess-driver lines exercisable only with the real
binary are covered by the integration half; no band override is requested.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/engineering` | 80% | new-package default (band already present from phase 12; unchanged). |

## Smoke checks

`scripts/smoke/phase-13.sh` sources `scripts/smoke/lib.bash` and, guarded on
`internal/engineering` existing, runs one `go test -run` assertion per criterion via
`run_group` (per-criterion SKIP until built; whole-script SKIP until the package exists;
real-binary/Postgres-gated tests `t.Skip` cleanly when `bruin` or the store URL is absent,
surfacing as SKIP).

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestPipelineStepWriteOutOfScopeRejected` PASS |
| 2 | `TestPipelineStepInputNotDeclaredRejected` PASS |
| 3 | `TestRunUndeclaredDestinationRejectedBeforeRender` PASS |
| 4 | `TestQualityCheckFailureFailsRunLoudly` PASS |
| 5 | `TestP1cWriteSplitArchitecture` PASS |
| 6 | `TestLineageRecordedPerMaterialization` PASS |
| 7 | `TestFreshnessStampedOnMaterialization` PASS |
| 8 | `TestDraftPublicationGate` PASS |
| 9 | `TestPipelineVersionImmutable` PASS |
| 10 | `TestRenderGatedByBruinValidate` PASS |
| 11 | `TestSQLOnlyAssetEnforcement` PASS |
| 12 | `TestNoPlaintextSecretPersists` PASS |
| 13 | `TestBruinTelemetryDisabledOnEveryInvocation` PASS |
| 14 | `TestBruinOutcomeMappingTyped` PASS |
| 15 | `TestStrategyEngineCompatibility` PASS |
| 16 | `TestMissingBruinDegradesLoud` PASS |
| 17 | `TestScheduleAttachmentEnqueues` PASS |
| 18 | `TestCanonicalRegistryResolveThenCompare` PASS |

## Glossary additions

New terms, pre-written for `docs/glossary.md` (same PR, CLAUDE.md §14); existing entries
(**Pipeline**, **Materialization**, **Lineage**, **Freshness**) reused unchanged:

- **Pipeline step** — one ordered, declarative unit of a pipeline: a single SQL statement
  with declared inputs, exactly one declared output, a materialization strategy, and
  optional quality checks (RFC §7.3/§7.6).
- **Declared / writable destination** — a `(data source, schema)` pair a tenant admin has
  explicitly marked writable and holds a `manage` grant on; the only place a
  materialization may write (RFC §7.6, P1c).
- **Write-shape validation** — the phase-09 validator variant a pipeline step must pass:
  single statement, writes only the declared output, reads only declared inputs; yields the
  unforgeable `WriteValidatedSQL` (RFC §7.3/§9.5, D-021).
- **`PipelineRunner` seam** — the interface + factory + driver seam behind which pipeline
  execution lives; V1 driver: the pinned Bruin CLI subprocess (D-036). Distinct from the
  read-only data-source adapters — the write side of P1c.
- **Rendered pipeline** — the run-time, tmpfs-resident translation of a Chartworks pipeline
  definition into the executor's format (SQL-only assets, custody-injected connections,
  telemetry disabled), deleted on run completion; never a persisted artifact (RFC §7.6,
  D-036).
- **Materialization strategy** — the inherited executor strategy set: create+replace,
  delete+insert, truncate+insert, append, merge (incremental), time_interval, scd2;
  per-engine support recorded per destination driver (RFC §7.6, D-036).
- **Pipeline run** — one execution of a published pipeline as a typed job on the leased
  queue, wrapping one executor invocation; recorded in `pipeline_runs` with a typed
  outcome, stats, and any typed error (RFC §7.6/§7.7, D-025/D-036).
- **Publication gate** — the single, `pipeline.manage`-scoped `draft → published`
  transition; nothing materializes until it passes (contract-before-materialize, brief 11).
- **Canonical identity registry** — the tenant-scoped, mint-once, fail-closed vocabulary
  mapping a business entity/term to its canonical id + governing dataset + key columns;
  shared by pipeline assistance and topic generation, resolve-then-compare, never
  name-vs-name (RFC §7.3, P7).

## Decisions filed

No new `docs/decisions.md` entry. This phase implements existing decisions: **D-036**
(Bruin as the DE write executor — the shape of this whole phase), **D-037** (container as
deployment unit; loud bare-metal degradation), **D-017/D-021** (write split; unforgeable
validated types), **D-013**, **D-025**, **D-020**, **D-004**, **D-035** (pin discipline —
a Bruin version bump is a conformance re-run; any change to a D-036 hard rule is a
superseding decision entry, never a silent edit). The telemetry-disable-mechanism
confirmation (the D-036 implementation blocker) lands its finding in this plan's deviation
log; if no reliable disable mechanism exists, that escalates to a decision entry before
any Bruin invocation ships.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). Known candidates at authoring time:
     (a) the confirmed Bruin telemetry-disable mechanism (env var / flag / config key,
     verified empirically against the pinned v0.11.666 binary) — record the finding here
     and encode it in the driver (criterion 13);
     (b) the master-plan phase-15 dependency list should gain `13` so topic generation
     consumes this phase's CanonicalRegistry (P7) — raise at the Wave-4→5 checkpoint and
     update docs/plans/README.md in that PR;
     (c) the master-plan phase-13 detail block still names the pre-D-036 `Materializer`
     interface — update the block to the PipelineRunner shape in the same checkpoint PR. -->
