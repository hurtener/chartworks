# Phase 26 — engineering-autopilot

> **Status:** draft
> **Owner:** orchestrator (Wave 7; executes 24 ∥ 26 → 25)
> **Depends on:** phase-12-engineering-profiling, phase-13-engineering-pipelines,
> phase-15-topics-lifecycle, phase-16-rules-clarification, phase-21-http-api
> (and, transitively, the shipped `access`/`identity`/`jobs`/`gateway`/`store`
> seams they stand on).

The L2/L3 agentic layer of the data-engineering stage: a business goal enters, a
**blind planner** enumerates the required capabilities without catalog access,
**retrieve → verify → confirm** matching decides reuse-vs-build against the
canonical registry and existing datasets/topics, and the result is assembled into
one atomic, revertible **proposal** (pipelines + datasets + topic deltas +
schedules) with a **decision record** per choice. A human reviews the *proposal*
(L2); a per-tenant **autonomy policy** may auto-apply the subset that fits (L3),
degrading loudly to review otherwise. Drift/freshness/quality failures feed the
same machinery as *proposed amendments* (the evolution loop). This is the least
battle-tested subsystem in the product, landing **last on proven gates** — the
plan is deliberately rigor-first: every write rides phases 12/13/15's existing
gates, and no autonomy level can weaken P1c or the D-040 write boundary.

---

## RFC / request sections

- **RFC-001 §7.7** — the autonomy ladder (L0…L3 + the evolution loop). This phase
  owns L2 (the V1 target) and ships L3 designed + per-tenant opt-in. Verbatim
  anchors: blind plan "no catalog access at plan time, so gaps are detectable";
  matching "retrieve → verify → confirm; resolve-then-compare, never name
  comparison"; "one atomic, reviewable proposal … each choice carrying a decision
  record"; "Outside policy ⇒ degrade to L2 review, loudly (P4)"; "Proposal/
  decision-record/policy surfaces are management-plane: HTTP + SDK only".
- **RFC-001 §7.6** — the governed write path (the gates approval applies *through*:
  declared managed-schema destinations, definition validation, render gate,
  `PipelineRunner`, blocking quality checks, content-free audit).
- **RFC-001 §1.2 P1c** — the write split + write boundary. "No autonomy level (§7.7)
  can relax this." This phase's D-040 adversarial probe is the mechanical proof.
- **RFC-001 §5** — the grant model (three grains, `read|query|manage`) and the one
  resolver; approval's resource gating rides existing `manage` grants (D-020).
- **RFC-001 §12** — the three budgeted tables this phase creates:
  `proposals`, `decision_records`, `autonomy_policies` (already inventoried; no
  schema-budget amendment needed).
- **RFC-001 §11.2 / §11.3** — the HTTP surface conventions and the SDK/CLI parity
  rule this phase's management-plane endpoints follow.
- **RFC-001 §13** — the gateway roles this phase reuses (`pipeline_draft`,
  `enhance`); it introduces **no** new role (the closed-enum discipline, risk
  register).
- **D-039** — the autonomy ladder decision (phase 26 owns L2/L3 on top of
  12/13/15/16; built on validated primitives, not Teramot's opaque loop).
- **D-040** — the write boundary: Chartworks-managed schemas only; baseline
  read-only forever. Structural, **not** policy-expressible.
- **D-020** — the access primitive approval reuses. **D-036** — Bruin as the
  `PipelineRunner` executor approval drives. **D-025** — the one leased job queue
  the intake/amendment handlers register on. **D-027** — governed-rule lifecycle
  (topic deltas may carry rules; they follow phase-16's lifecycle, never
  auto-activated past review).

## Depends on

- **phase-12-engineering-profiling** — the evolution loop's triggers. Consumes
  `DriftDiff`, the `SourceHealthReporter.DatasetHealth(ctx, scope, ids…)` interface,
  `FreshnessClass` (`fresh|stale|very_stale|unknown`), and `QualityAssessment` as
  **already-normalized shapes** — the amendment planner reads dataset health, it
  never re-derives freshness/quality from raw adapter reads.
- **phase-13-engineering-pipelines** — the write spine. Reused *without
  modification*: `PipelineDefinition{steps, destination Destination{source_id,
  schema}, dialect}`; the definition-validation error codes
  (`pipeline.step.write_out_of_scope`, `pipeline.step.input_not_declared`,
  `pipeline.asset_kind_forbidden`, `pipeline.strategy_unsupported`); the
  render-gate / destination check (`materialize.destination_undeclared` /
  `access.none`, checked before any render); the `PipelineRunner` seam
  (`Validate`/`Run`/`Lineage`, `bruin`+`mock` drivers); the `pipeline_run` job
  handler; and the **`CanonicalRegistry`** (`Resolve(ctx, tenant, term) →
  (CanonicalEntity, ok)` yielding `(canonical_id, governing_dataset, key_columns)`;
  fail-closed `canonical.resolution_ambiguous`).
- **phase-15-topics-lifecycle** — the topic-delta path. Topic deltas are created
  through `semantics.Lifecycle.CreateDraft` (and at most `SubmitReview`) — **never**
  `Publish`. Reads the capability contract via `semantics.Project(pack, caps)`.
- **phase-16-rules-clarification** — governed-rule authoring/lifecycle
  (`proposed → active → retired`); any rule a topic delta introduces is left
  `proposed`, never auto-activated.
- **phase-21-http-api** — the surface. New endpoints are appended as `Route{}` rows
  to the single `[]Route` descriptor table; the middleware chain (request-id → auth
  → envelope → access-resolve → scope-gate → audit → handler) gates them
  structurally; SDK methods land on the one `sdk/chartworks.Client` interface.

## Informing briefs

Per `docs/research/INDEX.md`: primary **11** (the engine — the ssr analyst draft),
secondary **12** (GenBI landscape / Teramot context) and **09** (freshness/
profiling shapes the evolution loop consumes).

## Brief findings incorporated

This phase is **brief 11's machinery made real** on Chartworks' own validated
primitives. Adopted, and where it lands:

- **Demand-driven, top-down-match / bottom-up-build scoping** (brief 11 §2) — the
  core cost model. A goal pulls in only the layers it needs; the planner emits a
  requirement manifest, matching reuses what the registry/catalog already governs,
  and only the gaps become new pipelines. This is the "lean, budget-friendly" DE
  crown jewel D-013 asks to keep.
- **The blind planner** (brief 11 §2/§2-good) — planning *without catalog access*
  converts a silent requirement-trim into a detectable gap (a plan-stage instance of
  P4). Implemented as a `gateway` `pipeline_draft` call whose prompt carries **only
  the goal text** (no dataset/topic/schema context), producing a schema-constrained
  `CapabilityManifest`; unrealizable requirements surface as typed gaps in the
  proposal, never trimmed.
- **The canonical registry / resolve-then-compare discipline** (brief 11 §2-good) —
  matching is id-against-id via `CanonicalRegistry.Resolve`, **never** name-vs-name.
  Consumes the *identical* phase-13 interface (P7 — one vocabulary, no second map).
- **Retrieve → verify → confirm with a fail-closed asymmetric threshold** (brief 11
  §1) — retrieve is a recall-biased shortlist that decides nothing; **verify** is
  deterministic Go comparing declared meaning (canonical ids, formulas, contracts) —
  the LLM has no reuse veto; **confirm** is a cached verdict or an empirical
  "compute both ways and diff" probe run **through the validated read path**;
  "unsure" ⇒ don't reuse ⇒ build. A wrong reuse is silent and inherited forever, so
  the threshold is asymmetric and fail-closed.
- **Whitelist-only / grounded composition, never freeform LLM SQL** (brief 11
  §2-good) — the planner composes over *declared capabilities and vetted pipeline
  strategies*; every generated pipeline still renders through phase-13's definition
  validation + `bruin validate` render gate, so no autopilot-authored SQL reaches a
  warehouse un-gated (P1b/P1c).
- **Contract-before-materialize, human-gated** (brief 11 §2-good) — the *proposal*
  is that gate at L2: nothing materializes until a human approves. At L3 the
  autonomy policy is the codified, per-tenant standing approval, and the decision
  record is the permanent audit tripwire.
- **Grow-by-addition / immutable versioning** (brief 11 §2-good) — proposals never
  mutate an applied asset in place; apply creates new pipeline/topic versions;
  revert re-points to the prior version. This is what makes "revert as a unit"
  tractable.
- **Brief 09 shapes reused, not reinvented** — the evolution loop consumes phase-12's
  `FreshnessClass`, `QualityAssessment`, `DatasetProfile`, `DriftDiff`, and
  `DatasetHealth` as the sole trigger vocabulary (brief 09's age-bucket freshness
  scale and structured profile artifact, already normalized and access-scoped in
  phase 12).
- **Brief 12 / Teramot** — its "agent fleet" claim is marketing-opaque (brief 12);
  this phase deliberately does **not** chase an opaque multi-agent loop. The engine
  is brief 11's single disciplined plan→match→assemble loop over our own gates
  (D-039 rationale), which is why every step is auditable and revertible.

## Findings I'm departing from

- **Brief 11's refresh/drift pipeline** — explicitly unwritten in the source ("TBD").
  Ditched wholesale. The evolution loop is built on **phase-12's real drift
  detection** (`DriftDiff` + `SourceHealthReporter`) and the freshness/quality
  shapes, not on the draft's fingerprint/value-baseline sketch.
- **Brief 11's external toolchain** (dbt / SQLMesh / Cocoon; Dagster/Prefect/Temporal
  orchestration) — ditched. Execution is **Bruin behind `PipelineRunner`** (D-036);
  orchestration is the **one leased jobs queue** (D-025), not a bespoke
  content-addressed frontier scheduler. The draft's "orchestrator's dynamic
  human-gated node graph vs. the SQL engine's static per-run DAG" separation is kept
  only as a *mental frame*: a proposal is the human-gated unit; Bruin owns the
  per-run DAG.
- **Brief 11's five-rung engine as literal V1 scope** — ditched as-is. The
  reuse-economics *discipline* (match top-down, build bottom-up, blind plan,
  resolve-then-compare) is adopted over Chartworks' real medallion + topic model; the
  literal bronze/silver/gold/raw rung machinery and two-phase build-unit
  consolidation are not built. The brief itself flags validating the reuse claim on a
  narrower slice first.
- **Brief 11's "topic matching off in v0 / every goal mints a new pack"** — departed.
  This phase **does** produce topic deltas, matched against existing topics via the
  registry, but stops them at **DRAFT/REVIEW** (never publishes) — see the guardrail
  below.
- **Brief 11's assumption that write access "just exists"** (the draft is silent on
  write governance) — reconciled hard against **D-017/D-040**: every materialization a
  proposal contains is a declared managed-schema destination, gated at definition
  validation AND render, credential-scoped where the engine supports it. This is the
  single largest thing the draft omitted and this phase makes structural.
- **Brief 11's registry durability / bootstrap gap** — not this phase's to solve; the
  registry is owned by phase 13. This phase treats `canonical.resolution_ambiguous`
  as a fail-closed escalation (a proposal gap), never a guessed mint.

## Scope

Delivers the autonomy layer as thin orchestration over shipped seams. New packages:

1. **`internal/engineering/autopilot`** — the L2/L3 core:
   - **Goal intake** (`Autopilot` service) — accepts a goal, enqueues an
     `autonomy.intake` job (expensive LLM work off the request hot path, D-025),
     returns a `proposal_id` in `draft`.
   - **Blind planner** — a `gateway` `pipeline_draft` schema-constrained call
     (prompt = goal only) → `CapabilityManifest{components[], dimension_pool[],
     populations[], grains[]}` (brief 11's requirement family). No catalog input.
   - **Matcher** — retrieve → verify → confirm against `CanonicalRegistry` +
     existing datasets/topics; emits per-requirement `MatchVerdict{reuse|build|
     escalate}` with evidence. Empirical confirm probes run through the validated
     read path (`exec`, read-only, scoped).
   - **Proposal assembler** — builds one `Changeset{pipelines, dataset_regs,
     topic_deltas, schedules}` + one `DecisionRecord` per object; persists as one
     `proposals` row (`changeset_json`) + N `decision_records` rows atomically.
   - **Review lifecycle** — `draft → proposed → approved → rejected → applied →
     reverted`; edit-then-approve; apply-through-gates; revert-as-a-unit.
   - **Evolution loop** — an `autonomy.amendment` job handler that turns phase-12
     drift/freshness/quality signals into `proposed` amendments via the same
     assembler.
2. **`internal/engineering/autopilot/policy`** — the deterministic L3 policy
   evaluator: `Evaluate(policy AutonomyPolicy, changeset Changeset) → PolicyVerdict`
   (`auto_apply` | `degrade{reason}`). Pure, table-driven, **no I/O, no model call**.
3. **`internal/store`** additions — three forward-only migrations creating
   `proposals`, `decision_records`, `autonomy_policies` (already in the §12 budget);
   narrow per-domain store ports + conformance-suite additions.
4. **`internal/api`** additions — management-plane routes appended to the single
   `[]Route` table (goal intake, proposal review/approve/reject/revert, policy CRUD).
   **No MCP tools.**
5. **`sdk/chartworks`** additions — the mirroring typed methods on the one `Client`
   interface (HTTP + in-process transports), bound as a **2-surface management
   capability** (HTTP + SDK), never 3-surface.
6. **Two job kinds** registered on the phase-06 queue: `autonomy.intake`,
   `autonomy.amendment`. **Two gateway roles reused** (`pipeline_draft`, `enhance`) —
   none added.

## Non-goals

- **Cross-tenant learning.** The registry, verdict cache, reuse economics, and every
  decision record are strictly tenant-scoped. No signal, embedding, verdict, or
  example ever crosses a tenant boundary (P3). Reuse compounds *within* a tenant
  only.
- **Autonomous topic PUBLICATION at any level.** Topic deltas in a proposal are
  applied only up to `SubmitReview` (draft → review). **No autopilot path — not L2
  approval, not L3 auto-apply — transitions a topic to `published`.** Publication
  remains the separate, human, `topic.publish`-scoped action of phase 15
  ("publication is always an explicit human gate (never auto-publish)", phase-15).
  *This guardrail is proposed explicitly (see Decisions filed) so it is a settled
  invariant, not an emergent behavior.*
- **NL-driven policy authoring.** Autonomy policies are structured JSON authored
  through a typed admin API; they are never generated from natural language by a
  model. (An LLM proposes *changesets*; it never writes the rules that govern its own
  auto-apply.)
- **A bespoke orchestration engine.** No content-addressed frontier scheduler; the
  jobs queue is the orchestrator (D-025).
- **New warehouse writes, new execution paths, new SQL surfaces.** This phase adds
  **zero** SQL assembly or execution; it composes existing validated writes
  (phase 13) and reads (phase 10). It opens no new P1b/P1c surface.
- **Charts.** Out of V1 (D-013); a proposal never contains a chart artifact.

## Design

### Data flow

```
goal (HTTP :submit-goal / SDK)                              ── management plane, autonomy.propose scope
  └─▶ persist proposals row (status=draft) ─▶ enqueue autonomy.intake job
        └─▶ [job] BLIND PLAN   gateway(pipeline_draft, prompt=goal only)  → CapabilityManifest
              └─▶ MATCH  per requirement: retrieve → verify → confirm
                    · retrieve  (vindex facets + fuzzy + registry) → recall-biased shortlist  (decides nothing)
                    · verify    (deterministic: Resolve→canonical_id, compare formulas/contracts) → reuse|reject
                    · confirm   (cached verdict | empirical probe via exec read path) → reuse|build|escalate
              └─▶ ASSEMBLE  Changeset{pipelines, dataset_regs, topic_deltas(→DRAFT), schedules}
                            + DecisionRecord per object      ── one store txn
              └─▶ status=proposed        (L2)   OR   policy.Evaluate → auto_apply|degrade  (L3)
  ── review plane ──────────────────────────────────────────  autonomy.apply scope + manage grants (D-020)
  edit-then-approve ─▶ approve ─▶ APPLY (all-or-nothing txn, through phase-13/15 gates) ─▶ status=applied
  reject ─▶ status=rejected                       revert ─▶ restore prior state as a unit ─▶ status=reverted

drift / freshness / quality failure (phase-12)  ─▶ enqueue autonomy.amendment job ─▶ [same ASSEMBLE machinery] ─▶ proposed amendment
```

### The blind planner (gaps must be detectable)

`Plan(ctx, goal) → (CapabilityManifest, error)` issues **one** `gateway` call on the
`pipeline_draft` role with a JSON-schema-constrained output (P5 — no free-text JSON
parse). The prompt template carries **only `goal.Text`** — a structural test
(`TestBlindPlannerPromptCarriesNoCatalog`) asserts the assembled prompt contains no
dataset id, topic id, schema, or profile string. The manifest is the enumerable
"requirement family" (brief 11): each `Component` carries a closed-grammar derivation
over *concepts* (not columns), additivity, population, window, and slicing
dimensions; plus a shared `dimension_pool`. Every field exists because a downstream
matcher check consumes it; free-text fields are retrieval hints only, never a
decision input. Blindness is the point: if the planner could see inventory it might
silently trim the goal to fit; instead an unmet requirement survives into matching
and surfaces as a typed `escalate` gap in the proposal (P4 one layer early). Manifest
completeness is a documented, structurally-unanswerable limitation (brief 11 §3), not
a bug — recorded in the decision record as `manifest_completeness: best_effort`.

### Matching — resolve-then-compare, fail-closed

For each manifest requirement the matcher runs the fixed three-move shape:

- **retrieve** — a recall-biased shortlist from `vindex` facet vectors + fuzzy name
  match + registry candidates. It is allowed to be wrong; it never decides.
- **verify** — deterministic Go. Every local name (a metric's informal name, a
  physical column) is resolved to a canonical id via
  `CanonicalRegistry.Resolve(ctx, tenant, term)`; comparison is **id-vs-id plus
  declared-meaning** (derivation formula, grain, contract), **never string-vs-string**.
  Any hard conflict rejects the candidate. `canonical.resolution_ambiguous` is
  fail-closed: no guessed reuse, no duplicate mint — it becomes an `escalate`.
  The gateway model has **no veto** here.
- **confirm** — a cached verdict lookup keyed by `(tenant, requirement_signature,
  candidate_canonical_id)`; on a miss and when `autonomy.matching.confirm_probe` is
  on, an empirical probe **computes the answer both ways through the validated read
  path** (`exec` — read-only, scoped, `ValidatedSQL`; never ad-hoc string SQL) and
  diffs; an ambiguous diff escalates to a human (a proposal gap), and whatever
  verdict lands is cached. The threshold is asymmetric: **"unsure" ⇒ build** (a
  wasteful rebuild beats a silent wrong reuse).

Verdict → assembly: `reuse` binds the requirement to the existing dataset/topic;
`build` emits a new pipeline (bottom-up); `escalate` emits a proposal gap
(non-blocking on the rest of the changeset — the proposal is still deliverable over
what resolved, per brief 11's atomic-delivery-over-partial-resolution shape).

### Proposal assembly — one atomic changeset, one decision record per choice

`Assemble(ctx, manifest, verdicts) → Proposal`. The changeset is built and persisted
as **one `proposals` row** (`changeset_json`, `status=proposed|draft`) plus **one
`decision_records` row per contained object**, in a single store transaction (P4 —
never a half-written proposal). Object classes:

- **pipelines** — rendered through phase-13 `PipelineDefinition` + its
  definition-validation (the write-shape codes above) at assembly time, so an
  invalid or out-of-boundary pipeline can never enter a proposal.
- **dataset registrations** — new datasets the build path will produce; registered
  through phase-13's registration path on apply.
- **topic deltas** — created via `semantics.Lifecycle.CreateDraft` on apply, landing
  in **DRAFT**; the assembler records the intended `SubmitReview` transition but
  never a publish.
- **schedules** — cron/interval attachments (phase-13/§7.8) into the jobs queue.

Approval **applies the changeset through the ordinary publication gates** — it adds
no bypass. Apply is one all-or-nothing store transaction wrapping: destination /
grant re-check (`materialize.destination_undeclared` / `access.none`), render gate
(`bruin validate`), `PipelineRunner.Run`, blocking quality checks, dataset
registration, `CreateDraft`(+`SubmitReview`) for topics, schedule attachment. Any
gate failure rolls the whole apply back and returns the proposal to `proposed` with a
typed error list — never a partial publish (criterion 1).

### Review lifecycle & revert semantics (pinned)

States (the §12 `proposals.status` enum): `draft → proposed → approved → rejected →
applied → reverted`.

- **draft** — created at intake; the assembly job fills it. A `draft` proposal past
  `autonomy.proposal_ttl` is garbage-collected.
- **proposed** — assembled, awaiting a human (L2) or the policy evaluator (L3).
- **edit-then-approve** — a reviewer with `autonomy.apply` may drop or modify
  individual objects while `proposed`; each edit writes a new `changeset_json`
  revision (audited) and a superseding decision-record annotation. The proposal
  stays **atomic over the edited set**.
- **approved → applied** — approval runs apply-through-gates (above). `approved` is a
  transient pre-apply marker; success ⇒ `applied`, gate failure ⇒ back to `proposed`.
- **rejected** — terminal; nothing materializes.
- **reverted** — an `applied` proposal rolled back **as a unit**.

**Revert restores prior state as a unit** — and because assembly is grow-by-addition,
"prior state" means *the proposal's objects did not exist* (or their prior versions
were active). Revert is one all-or-nothing transaction, per object class:

| Object class | Revert action |
|---|---|
| **pipeline** | Unpublish: `status → reverted`, definition version rows **retained immutably** (audit/lineage); its schedule detached. |
| **materialized table (managed schema)** | **DROP by default** — the table is Chartworks-created, D-040-guaranteed non-baseline, and fully reproducible from the retained pipeline definition, so dropping it never destroys client data. The drop is issued through the **same managed-schema-scoped write path** D-040 gates (definition-time + render-time destination check +, where supported, the managed-schema-scoped credential) — revert can no more touch a baseline table than apply can. Configurable to **archive** (rename into a `chartworks_archive_*` managed schema) via `autonomy.revert_materialization` for tenants who want a safety net. |
| **dataset registration** | Unregister: dataset marked archived/unregistered; metadata retained for lineage + audit (not hard-deleted). |
| **topic delta** | Discard the DRAFT version (it never published, so no consumer ever saw it). If it had reached REVIEW, it is withdrawn to discarded. |
| **schedule** | Detached and removed. |

Revert is **blocked** with typed `autonomy.revert_blocked_dependent` if a later
*applied* proposal depends on this one's objects (cross-proposal lineage) — the
honest failure over a cascade; the operator reverts the dependent first. Intra-
proposal dependencies revert together by construction.

### L3 policy evaluation (`internal/engineering/autopilot/policy`)

`AutonomyPolicy.rules_json` schema (structured, admin-authored, per optional
`source_id`):

```
{
  "risk_classes":         ["non_destructive"],          // permitted Strategy classes
  "required_quality_gates": ["not_null","unique"],      // checks that must be present + passing
  "cost_ceiling":         { "max_tokens": 50000, "max_gateway_cost": 2.00 },
  "row_ceiling":          { "max_rows_materialized": 5000000 },
  "schedule_bounds":      { "min_interval": "1h" }
}
```

- `risk_classes` maps to phase-13 `Strategy` values: `non_destructive` =
  {`create+replace`, `append`, `merge`, `time_interval`}; the destructive/history
  strategies (`delete+insert`, `truncate+insert`, `scd2`) require explicit listing.
- **Managed-schema scope is NOT policy-expressible** — it is structural (D-040). A
  policy cannot authorize a non-managed destination; the field simply does not exist
  in `rules_json`, and the D-040 gates run *regardless of policy* on every apply.
  `TestPolicyCannotExpressSchemaScope` asserts the schema rejects any destination/
  schema key.
- `policy.Evaluate(policy, changeset) → PolicyVerdict` is **pure and deterministic**:
  no store, no gateway, no clock beyond an injected `now`. Every changeset dimension
  (each strategy, each quality gate, token/row totals, each schedule interval) is
  checked; **all must pass** for `auto_apply`. Any miss ⇒ `degrade{reason}`.
- **Outside policy ⇒ typed degrade to L2, loudly.** The intake/amendment job leaves
  the proposal `proposed`, returns typed `autonomy.outside_policy` with the failing
  dimension, increments `autonomy_degrade_total{reason}`, and emits a content-free
  audit event. It never silently proceeds (P4). Auto-applied changes stay
  decision-recorded, post-hoc reviewable, and revertible.

L3 is globally gated by `autonomy.l3_enabled` (default **false**) AND a per-tenant
`autonomy_policies` row with `status=active`. Absent either, everything is L2.

### The evolution loop

The `autonomy.amendment` job handler subscribes to phase-12's signals: a `DriftDiff`
with a non-empty `AffectedDatasetIDs`/`DependentDatasetIDs`, a `very_stale`/`unknown`
`FreshnessClass`, or a flagged `QualityAssessment` dimension (read via
`SourceHealthReporter.DatasetHealth`). It enqueues an amendment goal ("restore
freshness of dataset X" / "reconform drifted source Y") and runs the **identical**
plan→match→assemble machinery, producing a `proposed` amendment (or an L3 auto-apply
within policy). The medallion evolves under audit instead of decaying behind a flag;
goal history + decision records make "why does this table exist" a query, not
archaeology (§7.7). `autonomy.amendment_jobs_enabled` (default true) gates the
subscription.

### Decision-record content contract (golden-tested)

Each `decision_records.decision_json` is a fixed shape, pinned by
`TestDecisionRecordShapeGolden`:

```
{
  "goal":            { "proposal_id": "...", "goal_text": "..." },
  "subject":         "pipeline|dataset|topic|schedule",
  "matched_vs_built":{ "verdict": "reuse|build|escalate",
                       "bound_canonical_id": "...", "governing_dataset": "..." },
  "alternatives":    [ { "candidate_canonical_id": "...", "rejected_reason": "..." } ],
  "evidence_refs":   { "dataset_ids": [...], "registry_entries": [...],
                       "probe_query_ids": [...], "probe_result_hash": "...",
                       "profile_refs": [...] },
  "model_provenance":{ "role": "pipeline_draft|enhance", "model_id": "...",
                       "gateway_call_id": "..." },        // ← gateway_call_events
  "token_cost":      { "tokens_in": N, "tokens_out": N, "cost": F }
}
```

Provenance and cost come from `gateway_call_events` (§13); no free-text field is a
decision input. The record is content-free of customer data rows (P4/§7): it stores
ids, canonical ids, and hashes — never sampled values or SQL text (SQL lives in the
`queries`/pipeline domain, referenced by id).

### Surfaces (management-plane; HTTP + SDK only)

New `Route{}` rows on the single phase-21 `[]Route` table; all `TenantSensitive:
true`, `Audited: true` with an `AuditAction`; handlers are thin (`decode → one
`Autopilot`/`policy` core call → encode`):

| Method | Path | Scope | Notes |
|---|---|---|---|
| POST | `/v1/engineering/goals` | `autonomy.propose` | submit a goal → `draft` proposal; accepts `Idempotency-Key` |
| GET | `/v1/proposals` / `/v1/proposals/{id}` | `autonomy.propose` | list/read proposal + its decision records |
| GET | `/v1/proposals/{id}/decision-records` | `autonomy.propose` | queryable reasoning trail |
| POST | `/v1/proposals/{id}:approve` | `autonomy.apply` | edit-then-approve body optional; `Idempotency-Key` |
| POST | `/v1/proposals/{id}:reject` | `autonomy.apply` | |
| POST | `/v1/proposals/{id}:revert` | `autonomy.apply` | applied → reverted; `Idempotency-Key` |
| CRUD | `/v1/autonomy-policies` (+ `{id}`) | `admin` | L3 policy authoring (structured JSON) |

- **Scope-gating.** The route `Scope` is the operation-family gate (403
  `scope.denied` before the handler). Inside apply, the **resource** gate reuses
  D-020: `approve`/`revert` additionally require a `manage` grant on **every** source
  a materialization targets and on **every** topic a delta touches, via
  `EffectiveAccess.ScopeFor(grain, permission)` / `Guard` — empty non-admin scope ⇒
  typed `access.none`, surfaced as 404 (existence-hiding). Both scope and grant must
  pass (D-020). Policy CRUD is `admin` — editing what auto-publishes is a tenant
  security decision.
- **Two new capability scopes** — `autonomy.propose` and `autonomy.apply` — are
  proposed additions to the §5.3 set (see Decisions filed); they are flagged as
  decision proposals for the orchestrator, not silently added. Rationale for two: a
  proposal is only a proposal until applied, so "may submit a goal / read proposals"
  must be grantable *without* "may apply to the warehouse" (least privilege — the
  whole point of L2 review). The existing set cannot express this without overloading
  `pipeline.manage` (which spans one grain, not a multi-grain changeset) or `admin`
  (too coarse for review-read). Policy CRUD reuses `admin` — **no** third scope.
- **No MCP tools.** The MCP tool set stays 11 (§11.1 — agents ask questions, they do
  not administer tenants). `TestNoAutonomyMcpTool` asserts the MCP registry gains
  nothing.
- **SDK.** Typed methods on the one `sdk/chartworks.Client` (HTTP + in-process),
  bound as a **2-surface management capability** (HTTP + SDK) in the parity registry —
  the MCP column stays intentionally empty (phase-23 management pattern).
- **Cross-tenant + empty-access + audit coverage** come **for free** from the
  route-table-derived harnesses (phase-21/§5.5): every new `TenantSensitive` route is
  auto-enumerated into the cross-tenant probe; every `Audited` mutating route is
  asserted to carry an `AuditAction`.

### How it upholds P1–P7

- **P1a** — every store read/write of proposals/policies takes non-optional
  `(tenant, scope)` params; approval intersects `manage` grants inside the apply path.
- **P1b** — this phase executes no generated SQL directly; the confirm probe and every
  build read go through the shipped validated read path (`exec`); every build write
  goes through phase-13's validated write path. No new SQL surface.
- **P1c / D-040** — the *only* writes are phase-13 materializations into declared
  managed schemas; apply and revert both ride the D-040 double gate; the adversarial
  probe proves no proposal at any level targets a non-managed schema. **No autonomy
  level relaxes this** (§1.2).
- **P3** — tenant predicate on every proposals/decision_records/autonomy_policies
  query; no cross-tenant learning (non-goal).
- **P4** — outside-policy degrade, gate failure, ambiguous match, and
  `canonical.resolution_ambiguous` are all typed errors + metrics; apply/revert are
  all-or-nothing; no empty-catch, no silent trim (blind planner), no silent partial
  publish.
- **P5** — the blind planner and topic-delta drafting are `gateway` calls
  (`pipeline_draft`/`enhance`), schema-constrained, metered; **no** provider SDK in
  this package; no new role.
- **P6** — wire/UI nouns are domain-clean (`proposal`, `decision record`, `autonomy
  policy`, `goal`, `amendment`, `revert`); no `job`/`worker`/`repair` on any surface;
  path verbs (`:approve`/`:reject`/`:revert`) pass the drift-audit scan.
- **P7** — one proposal primitive, one policy evaluator, one assembler; HTTP/SDK are
  thin callers over the one `Autopilot` core; approval reuses the *existing*
  publication gates (no parallel write path); the canonical vocabulary is consumed,
  never re-mapped.

## Config keys added

New `autonomy` config domain (owned by `internal/engineering/autopilot`; the HTTP
surface adds none — it reuses `server.*`). Fail-loud validators: `retrieve_k > 0`;
`proposal_ttl > 0`; `revert_materialization ∈ {drop, archive}`; `cost_ceiling*`
defaults `> 0`.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `autonomy.enabled` | bool | `true` | no | L2 proposal flow available; `false` ⇒ goal-intake routes return typed `dependency.unavailable`. |
| `autonomy.l3_enabled` | bool | `false` | no | Global L3 gate; auto-apply still requires an `active` per-tenant policy. Off by default — L3 is opt-in (risk register). |
| `autonomy.revert_materialization` | enum(`drop`\|`archive`) | `drop` | no | Managed-schema table treatment on revert. |
| `autonomy.matching.retrieve_k` | int | `20` | no | Recall-biased shortlist size (retrieve move). |
| `autonomy.matching.confirm_probe` | bool | `true` | no | Run the empirical compute-both-ways probe on verdict-cache miss. |
| `autonomy.proposal_ttl` | duration | `168h` | no | `draft`/`proposed` proposals GC'd after this. |
| `autonomy.amendment_jobs_enabled` | bool | `true` | no | Evolution loop subscribes to phase-12 signals. |
| `autonomy.default_cost_ceiling_tokens` | int | `50000` | no | L3 cost ceiling when a policy omits `cost_ceiling`. |

## Acceptance criteria

Numbered, mechanically checkable; each maps to a smoke assertion. Covers the master
plan's six (1, 2–3, 6, 7, 8, 9) plus the rigor obligations this last-landing
subsystem earns.

1. **Atomicity + revert (proven).** Approving a proposal applies every object in one
   store transaction; a forced mid-apply gate failure rolls back **all** objects
   (no partial publish, proposal returns to `proposed` with typed errors). Reverting
   an `applied` proposal restores prior state **as a unit** per the object-class table
   (pipelines unpublished, managed-schema materializations dropped/archived, draft
   topic versions discarded, datasets unregistered, schedules removed).
2. **Queryable decision record per object.** For every object in a proposal's
   `changeset_json` there exists exactly one tenant-scoped `decision_records` row,
   queryable by `proposal_id`.
3. **Decision-record shape (golden).** The `decision_json` matches the content
   contract (goal, matched-vs-built, alternatives, evidence refs, model provenance,
   token cost); content-free of data rows/SQL text.
4. **Blind planner.** The plan-time `gateway` prompt carries no dataset/topic/schema/
   profile string (structural assertion); output is a schema-constrained
   `CapabilityManifest`; an unrealizable requirement surfaces as a typed `escalate`
   gap in the proposal, never silently dropped.
5. **Resolve-then-compare matching (fail-closed).** Reuse decisions are made by
   deterministic verify against canonical ids (never name string-matching); a
   name-collision-but-different-meaning candidate is **not** reused; the confirm probe
   runs through the validated read path; "unsure" ⇒ build.
6. **L3 policy matrix + loud degrade.** Table-driven `policy.Evaluate` across
   risk-class / quality-gate / cost / row / schedule dimensions: inside-policy ⇒
   `auto_apply`; any out-of-policy dimension ⇒ `degrade{reason}`, proposal stays
   `proposed`, typed `autonomy.outside_policy` + `autonomy_degrade_total{reason}`
   emitted. Deterministic (pure).
7. **D-040 adversarial probe.** No proposal at **any** autonomy level (L2 approve or
   L3 auto-apply) can produce an applied changeset whose materialization resolves to a
   non-managed schema — rejected at definition validation AND render gate; managed-
   schema scope is not policy-expressible (`rules_json` rejects any schema/destination
   key).
8. **Evolution loop (integration).** A phase-12 drift flag (and a very_stale
   freshness / flagged quality signal) enqueues an `autonomy.amendment` job that
   produces a `proposed` amendment end-to-end through the same machinery, against a
   real Docker Postgres + jobs queue.
9. **Goal → medallion round-trip (golden, mock stack).** A goal → blind plan → match
   (all-build) → proposal → approve → materialize into a managed schema via the
   `PipelineRunner` mock produces datasets + a topic delta left at **DRAFT/REVIEW
   (not published)** — golden-asserted.
10. **No autonomous topic publication.** No autopilot path (L2 approve or L3
    auto-apply) transitions a topic to `published`; the applied topic delta is at
    draft/review; a publish attempt from the autopilot core is a typed
    `autonomy.topic_publish_forbidden`.
11. **Scope + tenant gating.** Goal submission requires `autonomy.propose`;
    approve/reject/revert require `autonomy.apply` **and** a `manage` grant on each
    affected resource; policy CRUD requires `admin`; every new tenant-sensitive route
    is auto-covered by the route-table cross-tenant probe (bare 2xx fails) and the
    empty-access-set probe.
12. **Management-plane, no MCP.** Autonomy capabilities ship on HTTP + SDK (both
    transports) with a parity test; the MCP tool registry gains **nothing** (still 11
    tools).
13. **Idempotent mutations.** A repeated goal-submit / approve / revert POST with the
    same `Idempotency-Key` returns the recorded response without re-executing (no
    duplicate proposal, no double apply); first-write-wins under concurrency.
14. **Store conformance + forward-only migrations.** `proposals`,
    `decision_records`, `autonomy_policies` join the conformance suite; every method
    takes non-optional `(tenant, scope)` params (a scope-less method cannot be
    expressed); the three migrations are forward-only and the runner idempotent.

## Test obligations

Per CLAUDE.md §11:

- **Unit (table-driven):** `policy.Evaluate` matrix (criterion 6); the matcher
  verify/confirm decision table incl. name-collision and fail-closed cases
  (criterion 5); the blind-planner prompt assembly (criterion 4); revert per
  object-class (criterion 1); decision-record golden (criterion 3); goal→medallion
  golden on the mock stack (criterion 9). The `policy` and `autopilot` cores are pure
  where possible so these are hermetic. The `gateway` `mock` driver backs plan/enhance
  calls, paired with a recorded-fixture test of the `pipeline_draft`/`enhance` wire
  shape.
- **Integration (required — this phase consumes 12/13/15/16/21 seams and closes the
  autonomy loop):** the evolution loop end-to-end (criterion 8) and the
  approve→apply→revert lifecycle (criterion 1) against a **real Docker Postgres** +
  the real jobs queue + the `PipelineRunner` `mock` (the one sanctioned boundary
  mock), under `-race`. Proves identity/scope propagation from the HTTP envelope
  through the intake job to the apply transaction, and ≥1 failure mode (mid-apply gate
  failure → full rollback). Lives in `test/integration/` (crosses subsystems).
- **Adversarial (required — this touches an access/write-boundary path):** the D-040
  probe (criterion 7) at both L2 and L3; the cross-tenant probe over the new routes
  (auto-derived, criterion 11); the empty-access-set probe on approve; a forged-header
  attempt on goal submit; a fetch-then-filter regression guard on the proposals/
  decision-records listing (scope intersected in-query). A revert-touches-baseline
  probe (revert must not issue a drop outside a managed schema) is a standing
  obligation alongside the phase-13 write/DDL-injection + schema-escape probes.
- **Fuzz:** `FuzzCapabilityManifestDecode` (the schema-constrained planner output is a
  decode surface — asserted invariant: a decoded manifest never yields a build node
  whose destination is non-managed, and malformed input is a typed error, never a
  panic) and `FuzzAutonomyPolicyDecode` (a decoded policy never authorizes a
  non-managed destination and never crashes the evaluator). Seed corpora committed;
  run as ordinary CI tests.
- **Bench:** `BenchmarkPolicyEvaluate` (the L3 hot path — evaluated on every L3
  changeset) and `BenchmarkMatchVerify` (the deterministic verify move). Baselines,
  not gates. The `Autopilot` service and the `policy` evaluator are reusable shared
  artifacts ⇒ a `-race` concurrent-reuse test each.

## Coverage targets

Bands registered in `scripts/coverage-bands.conf` in this PR.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/engineering/autopilot` | 80% | New `internal/` package (convention-4 default). The write/access logic it *drives* is covered at 85% in `engineering`/`exec`/`access`; this is the orchestration layer over them. |
| `internal/engineering/autopilot/policy` | **85%** | **The L3 auto-apply decision gate.** A wrong "inside-policy" verdict lets an **unreviewed** change reach a customer warehouse — a safety-adjacent decision, exactly the class (`access`/`exec`) the master plan bands at 85%. Pure and I/O-free, so the band is hermetically reachable. |

## Smoke checks

Every criterion maps to one `run_group` assertion in `scripts/smoke/phase-26.sh`
(`-race`; SKIPs until `internal/engineering/autopilot` exists and each test is
present).

| Acceptance criterion | Smoke assertion (test name) |
| --- | --- |
| 1 | `TestProposalApplyAtomicAndRevertRestoresPriorState` |
| 2 | `TestEveryProposedObjectHasQueryableDecisionRecord` |
| 3 | `TestDecisionRecordShapeGolden` |
| 4 | `TestBlindPlannerPromptCarriesNoCatalog` |
| 5 | `TestMatchResolveThenCompareFailClosed` |
| 6 | `TestL3PolicyMatrixAndLoudDegrade` |
| 7 | `TestNoProposalTargetsNonManagedSchema` |
| 8 | `TestDriftEnqueuesProposedAmendment` |
| 9 | `TestGoalToMedallionRoundTripGolden` |
| 10 | `TestAutopilotNeverPublishesTopic` |
| 11 | `TestAutonomyScopeAndGrantGating` |
| 12 | `TestAutonomyManagementPlaneNoMcpTool` |
| 13 | `TestAutonomyMutationsIdempotent` |
| 14 | `TestAutonomyStoreConformanceAndForwardOnlyMigrations` |

## Glossary additions

Pre-written for `docs/glossary.md` (landed in this PR). The core nouns (proposal,
decision record, autonomy policy/ladder, managed schema, baseline table) already
exist from the RFC seed; this phase adds:

- **Blind planner** — the plan-time step that enumerates a goal's required
  capabilities via a `gateway` call carrying **only the goal** (no catalog), so a gap
  the inventory can't meet is *detectable* rather than silently trimmed (a plan-stage
  instance of P4; brief 11).
- **Capability manifest** — the blind planner's typed, enumerable requirement family:
  components (metrics with a closed-grammar derivation over concepts) + a shared
  dimension pool; every field exists because a matcher check consumes it.
- **Retrieve → verify → confirm** — the fixed three-move matching shape: a
  recall-biased shortlist (decides nothing) → deterministic resolve-then-compare (the
  decision; the LLM has no veto) → cached-verdict or empirical probe, fail-closed
  ("unsure ⇒ build").
- **Changeset** — a proposal's contained set of pipelines + dataset registrations +
  topic deltas + schedules, applied and reverted as one atomic unit.
- **Topic delta** — a proposal-borne change to the semantic model, created through
  phase-15's DRAFT machinery and **never** published by the autopilot (stops at
  draft/review).
- **Amendment (proposed)** — a proposal generated by the evolution loop from a
  drift/freshness/quality signal, through the identical proposal machinery.
- **Revert** — restoring an applied proposal's prior state as a unit; per object
  class: pipelines unpublish, managed-schema materializations drop (or archive), draft
  topic versions discard, datasets unregister, schedules detach — never touching a
  baseline table.

## Decisions filed

Proposals A/B/C below were **ratified by the orchestrator as D-041** during the
planning review (scopes; the topic-publication bar; revert semantics) — this plan
implements D-041 as specified there. Original proposal text retained for the record:

- **Proposed decision A — Two new capability scopes, `autonomy.propose` and
  `autonomy.apply`; policy CRUD reuses `admin`.** Justification: least-privilege
  separation of "may submit a goal / read proposals" from "may apply to the
  warehouse" is the operating premise of L2 review, and the §5.3 set cannot express it
  without overloading `pipeline.manage` (single-grain) or `admin` (too coarse).
  Approval additionally requires D-020 `manage` grants on every affected resource
  (scope gates the op-family, grant gates the resource — both must pass). This is a
  scope-enum addition ⇒ a decision entry (the closed-enum discipline).
- **Proposed decision B — Autopilot never publishes a topic at any autonomy level.**
  Topic deltas stop at draft/review; publication stays the human `topic.publish`
  action (phase-15). A guardrail elevated to a settled invariant so it cannot erode.
- **Proposed decision C — Revert semantics per object class; managed-schema
  materializations DROP by default (archive configurable).** Records the object-class
  revert table and that DROP is safe precisely because D-040 guarantees the table is
  Chartworks-created and reproducible from the retained definition.

Relies on existing: **D-039** (the ladder), **D-040** (write boundary — structural,
not policy-expressible), **D-020** (grants), **D-036** (Bruin executor), **D-025**
(one queue), **D-027** (rule lifecycle).

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). Likely churn points flagged now:
     (a) the phase-13 CanonicalRegistry mint/list method names are unspecified in the
     phase-13 plan — this phase consumes `Resolve` (the only pinned signature) and will
     name any additional read it needs against the shipped phase-13 API, updating this
     plan in the same PR; (b) the exact `semantics.Lifecycle` / `Autopilot` construction
     wiring settles when 13/15 land; (c) if L3 auto-apply is deferred past V1 by the
     wave-7 checkpoint, criterion 6 stays covered by unit-testing the pure evaluator with
     the auto-apply *path* gated off — note it here rather than deleting the criterion. -->
