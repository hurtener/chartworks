# Phase 18 — nlq-generation-execution (Wave 5)

> **Status:** draft
> **Owner:** orchestrator
> **Depends on:** phase-09-sql-validate-core, phase-10-exec-read, phase-17-nlq-routing-context

This phase **closes the P1b hard gate** (README convention 9, RFC §1.2): generated
or submitted SQL executes against a real data source only from here onward. Everything
prior stops at a `ValidatedSQL` value; nothing has executed it on the NLQ path.

---

## RFC / request sections

- **RFC §9.3** — internal generation (mode a): `sqlgen`, schema-constrained, targeting
  the source's **native dialect** (D-038 — no ANSI-subset constraint; the dialect + its
  SQL requirements ride the context); the one precedence resolution function
  `edit_base > hints > examples > default`; bounded validation repair (≤1 `sqlfix`).
- **RFC §9.5 (amended — layered, D-038)** — the semantics-aware allowlist inside the
  layered validation: where the **parser seam** has a proven driver for the dialect
  (layer 2), the client-side AST walk does **column-grain** (topic pack schema ∩ caller
  dataset grants) + join reachability; the **engine dry-run/EXPLAIN** (layer 3)
  guarantees **table-grain** allowlisting on every engine (dry-run referenced-table set
  ∩ topic ∩ grants), regardless of layer-2 coverage. A dialect without a proven parser
  driver skips layer 2 — recorded, never faked. This phase supplies the
  semantics-dependent halves of layers 2 and 3 that phases 09/10 deferred.
- **RFC §9.6 (amended)** — read-only execution wiring for the NLQ path: read-only
  credentials as the **primary** read-only guarantee (D-038), the dry-run step inserted
  before execution, execution-time bounded repair (≤1 `sqlfix`), server-side timeouts +
  cursor caps (from phase 10), idempotency keys.
- **RFC §9.7** — the normalized result envelope `run_query` returns.
- **RFC §9.8** — feedback + the learn-positive job (DB-first weights: Wilson + recency +
  evidence-growth blend, paraphrase dedup) + the examples lifecycle.
- **D-021** — the intersection proof (topic pack ∩ grants), defense-in-depth read-only,
  `ValidatedSQL` as the only executable type.
- **D-038** — layered read-side validation (credentials primary; engine dry-run/EXPLAIN
  dialect-truth; parser seam adds depth per-dialect); native-dialect generation
  (supersedes D-033's generation-subset obligation; D-033's `crdb` parser pin retained).
- **D-014** — dual generation modes converge on one validation/execution core (BYO is
  phase 19; this phase builds the core mode (b) reuses).
- Capability scopes: **RFC §5.3** `query.preflight` / `query.plan` / `query.execute` /
  `feedback.write`.

## Depends on

- **phase-09-sql-validate-core** — provides the layered-validation skeleton: tokenizer
  screens, the **parser seam** (drivers `crdb`; `sqlglotgo` per-dialect, evidence-gated —
  D-038), the `ValidatedSQL` opaque type, and the typed error vocabulary. This phase adds
  the semantics-aware allowlist as the final AST walk **inside phase 09's machinery**
  where a parser driver covers the dialect — it does not fork a second validator (P7).
- **phase-10-exec-read** — provides `exec.Query(ctx, ValidatedSQL, opts)`, the read-only
  credential/session posture (the primary guarantee, D-038), the **engine dry-run/EXPLAIN
  step** (dialect-true syntax check + the referenced-table set), server-side timeouts,
  cursor caps, `ResultPreview` shaping, and idempotency-key support. This phase is the
  first caller that executes generated SQL through it, and wires the dry-run
  referenced-table set into the table-grain grant check.
- **phase-17-nlq-routing-context** — provides routing decisions, the pruned capability
  contract (query context), clarification slots, and the calibrated confidence primitive
  the complexity directive reads.
- **Transitively (already shipped, consumed not re-built):** phase-05 gateway (`sqlgen`
  and `sqlfix` roles, schema-constrained), phase-06 jobs (the learn-positive handler runs
  on the one leased queue), phase-07 vindex (example re-embed on learn), phase-04 access
  (the resolver + `EffectiveAccess`), phase-15 semantics (topic pack + join graph),
  phase-02 store (`queries`, `examples`, `feedback_events`, `idempotency_cache`).

## Informing briefs

Per `docs/research/INDEX.md`: **03** (primary — NLQ pipeline), **02** (execution path,
idempotency, self-curation bounds), **08** (reflect/fix split, layered gates).

## Brief findings incorporated

- **Brief 03 §2.5 — one explicit, testable precedence function.** `edit_base > hints >
  examples > default`, resolved once (not scattered conditionals); edit-base/hints gate
  on the two documented thresholds (weight ≥ 0.9, similarity ≥ 0.8) plus a topic-match
  check, and **disable few-shot demos** when they fire; examples become ranked few-shot
  only if neither higher path fired. Carried as `nlq.ResolvePrecedence`, unit-tested
  table-driven (brief 03 keeper: "documented twice because demos 'disappear' surprised
  engineers").
- **Brief 03 §3 — generator concurrency safety.** The generator is a shared singleton;
  per-request state (the resolved few-shot set) rides `ctx`, never a receiver field —
  proven by a `-race` concurrent-reuse test (CLAUDE.md §5).
- **Brief 03 §4 / §9 — the CTE scar.** The semantics-aware allowlist is an AST walk (and,
  at layer 3, the engine's own parse) — never a pre-parse string check; a tokenizer screen
  must not duplicate parser-level judgment; the standing golden CTE fixture (phase 09)
  still passes after this phase adds its walk.
- **Brief 03 §5 / ADR-005 — DB-first learn-positive.** Learned weight persists to the
  `examples` row (Wilson-score + recency + log-growth-of-evidence blend), cache is
  read-through only; a restart never regresses quality. Paraphrase dedup keyed
  `(tenant, topic, example, sample_hash)`. Positive **corrections** may *propose* a
  governed rule (into phase 16's lifecycle) — never auto-activate.
- **Brief 03 §7 — first-class ambiguity output.** Assumptions + ambiguity assessment ride
  the §9.7 envelope; ids resolve to display labels before returning (never an id as a
  user-facing label).
- **Brief 02 — plan/run split + idempotency.** Distinct capability scopes make the
  execute-vs-plan boundary a route-level fact, not a convention; `run` idempotency scoped
  `(operation, tenant, principal, client_key)` short-circuits to the cached envelope with
  no second warehouse execution.
- **Brief 02 — bounded repair caps, fail loud past the cap.** The `fix_attempts` ceiling
  and "zero rows → at most one re-plan" are carried as the *principle* (cap total repair
  rounds, typed terminal past the cap — P4), not the counter mechanics.
- **Brief 08 §2 — reflect/fix split, explicit observable round counter.** Validation-fix
  and execution-fix are distinct repair actions, each ≤1, each incrementing a
  telemetry-visible counter; an exhausted budget is a typed error, never a silent
  best-effort answer.
- **Brief 08 §4 — layered, independently-testable gates.** Statement-type, single-
  statement, allowlist, and join-reachability are separate checks (from phase 09) the
  allowlist walk composes, not one monolithic "validate" call.

## Findings I'm departing from

- **Brief 03 §3 — GEPA prompt-pack optimizer / autopilot.** Out of V1 scope (RFC §19).
  Prompt packs are static config here; no genetic optimizer.
- **Brief 03 §5 / §7 — rule *shadow* evaluation + historical replay on positive
  corrections.** Deferred (D-027): a positive correction may only *propose* a rule; the
  shadow/replay machinery is a later wave. This phase files the proposal, nothing more.
- **Brief 02 — the "zero rows → one re-plan" self-curation replan.** Not carried in V1:
  a zero-row result is a valid answer, not an error to repair; re-planning on empty
  results risks masking a correct empty result (P4 — an empty result that reads as
  "nothing found" is exactly the confusion we avoid). Only *execution errors* trigger the
  bounded execution repair.
- **Brief 08 §2 — `parallel`/`selection` best-of-N generation.** Not V1; one generation
  path, bounded repair, typed terminal.

## Scope

Delivered in `internal/nlq` (generation, precedence, plan/run/preflight/refine services,
feedback + learning) and `internal/exec` (the semantics-aware allowlist stage + bounded
repair orchestration over phase 10's executor):

1. **Internal generation** (`nlq.Generate`): a schema-constrained `sqlgen` gateway call
   (P5 — never free-text JSON) taking the §9.2 query context + resolved precedence inputs,
   targeting the routed source's **native dialect** (D-038 — no ANSI-subset directive;
   the dialect and its SQL requirements are stated in the context), emitting a candidate
   SQL string + generation provenance (`generator: internal`).
2. **The one precedence function** (`nlq.ResolvePrecedence`): `edit_base > hints >
   examples > default`, threshold-gated, few-shot-disabling on the top two paths;
   unit-tested table-driven.
3. **The semantics-aware allowlist inside the layered validation** (`exec` — extends
   phases 09/10, D-038):
   - **Layer 2 (client-side AST, where the parser seam covers the dialect):** the
     allowlist walk intersecting every table/**column** reference against
     (topic-pack schema ∩ caller dataset grants) and checking join reachability against
     the declared join graph; typed codes `statement.blocked`, `table.not_granted`,
     `table.not_in_topic`, `column.unknown`, `join.unreachable`, `parse.unsupported`. A
     dialect without a proven driver skips this layer — recorded in the validation
     report, never faked.
   - **Layer 3 (engine dry-run/EXPLAIN, every engine, pre-execution):** the dry-run's
     referenced-**table** set is checked against (topic ∩ caller grants) before the real
     run — table-grain allowlisting guaranteed on every engine regardless of layer-2
     coverage; a dry-run syntax error is a typed validation failure.
   `ValidatedSQL` is produced after tokenizer screens + layer 2 (where covered); the
   layer-3 gate lives inside `exec.Query` so no caller can execute without it.
4. **Bounded repair** (`nlq`/`exec`): ≤1 `sqlfix` on validation failure — including a
   **dry-run/EXPLAIN failure**, whose engine error message is dialect-true fix context
   (cheaper than an execution round, D-038) — revalidate, then typed terminal
   `sql.generation_failed`; ≤1 `sqlfix` on a real warehouse execution error (revalidate →
   re-execute, then typed terminal `sql.execution_failed`); each attempt increments an
   observable round counter. Grant denials (`table.not_granted`) short-circuit repair on
   the run path once generation context already reflected the caller's grants — repair
   never becomes a grant-probing loop.
5. **The service surface** (`nlq`): `Preflight` (routability + clarification slots, no
   SQL), `Plan` (route → generate → client-side validate, **no execution and no engine
   contact** — layers 1–2 only; the validation report marks the dry-run gate as pending),
   `Run` (plan, then **dry-run + table-grain allowlist, then execute** — the D-038
   sequencing, all engine contact under `query.execute`), `Refine` (session-scoped
   re-run). Scope mapping: `Preflight` → `query.preflight`, `Plan` → `query.plan`,
   `Run`/`Refine` → `query.execute`. `Run` idempotency scoped
   `(operation, tenant, principal, client_key)` via `idempotency_cache`.
6. **The §9.7 result envelope**: routing evidence (topics, confidence, decision),
   assumptions + ambiguity assessment, the SQL + provenance + validation report, the
   `ResultPreview`, a chart-spec slot (populated by phase 20; reserved shape here), and
   typed warnings — ids resolved to display labels.
7. **Feedback + learn-positive** (`nlq`): `SubmitFeedback` records a verdict (optional
   correction SQL) into `feedback_events`; the learn-positive job (jobs queue) re-embeds
   the example (vindex), dedupes paraphrases, recomputes the DB-first routing weight
   (Wilson + recency + evidence-growth), and advances the examples lifecycle
   `candidate → active → retired` (audited).

## Non-goals

- **BYO-agent mode** (`get_query_context` / `submit_sql`) — phase 19. This phase builds
  the single validation/execution core mode (b) reuses; it ships no bundle contract.
- **Chart selection** — phase 20 populates the reserved chart-spec slot; this phase only
  reserves it.
- **The MCP/HTTP surfaces** — phases 21–22 wrap these services as thin callers; this phase
  ships no route or tool (P7 — core first, surfaces later).
- **Rule shadow evaluation / replay** — deferred (D-027).
- **Query-result caching / pagination** — explicit non-goals (RFC §19); idempotency keys
  cover retries only.
- **GEPA / best-of-N generation** — RFC §19.

## Design

### Data flow (Run)

The D-038 sequencing: **validate (layers 1–2, client-side) → dry-run + table-grain
allowlist (layer 3, engine) → execute**. `Plan` stops after the client-side layers;
all engine contact (dry-run included) sits behind `query.execute`.

```
question ──▶ [17] route + assemble context (native dialect + ──▶ ResolvePrecedence
   │              SQL requirements ride the context, D-038)          │
   │                                          Generate (sqlgen, schema-constrained,
   │                                                    │            native dialect)
   ▼                                                    ▼  candidate SQL
scope+grant gate (04)                  [09] tokenizer screens ─▶ AST allowlist walk
   │  query.execute?                        (parser seam,          (column-grain
   ▼                                         where covered)         pack ∩ grants
short-circuit if empty access                           │           + join graph)
   │                                     fail? ─▶ sqlfix ×≤1 ─▶ revalidate ─▶ terminal
   ▼                                                    │ pass  ─── Plan stops here ───
idempotency_cache lookup ──hit──▶ cached env.      ValidatedSQL ← only executable type
                                                        ▼
                                       [10] exec.Query: engine dry-run/EXPLAIN
                                            referenced tables ∩ topic ∩ grants
                                                        │  dry-run error ─▶ feeds the
                                                        │  ≤1 validation sqlfix (D-038)
                                                        ▼ pass
                                            real execution (read-only creds primary,
                                            timeout, cursor caps)
                                                        │  exec error? ─▶ sqlfix ×≤1 ─▶ terminal
                                                        ▼
                                        §9.7 envelope (labels resolved) ─▶ persist queries row
```

### Key types / interfaces

- `nlq.GenerationInput` — query context (§9.2) + `PrecedenceResult`; `nlq.ResolvePrecedence(ctx, RoutingDecision, retrieved, cfg) → PrecedenceResult` where `PrecedenceResult.Strategy ∈ {edit_base, hints, examples, default}` and `FewShotDisabled bool`. One function, one discriminator (brief 03 §2.4 stable-envelope shape).
- `exec.Validate(ctx, raw string, scope AllowlistScope) → (ValidatedSQL, error)` — extends phase 09 (layers 1–2); `AllowlistScope` carries the routed topic pack's allowed tables/columns **and** the caller's `EffectiveAccess` dataset set. The intersection is computed here (D-021 requirement 2), never conflated: a reference outside the pack → `table.not_in_topic`; a reference in the pack but outside grants → `table.not_granted`. When the parser seam has no proven driver for the dialect, the AST walk is skipped and the `ValidationReport` records `ast_layer: skipped` — never faked (D-038).
- The **same `AllowlistScope`** rides into `exec.Query` (layer 3): the engine dry-run/EXPLAIN's referenced-table set is checked against its table-grain projection before the real run — one scope value, two layers, no second access representation (P7). The gate is internal to `exec.Query`, so a caller structurally cannot execute without it.
- `exec.RepairBudget` — `{ValidationAttempts: 1, ExecutionAttempts: 1}` are **invariant constants**, not config (weakening the cap would weaken P1b); the on/off toggle (`exec.self_repair`) only disables the loop, never raises the cap. A dry-run/EXPLAIN failure consumes the *validation* attempt (its engine error is dialect-true fix context — cheaper than an execution round, D-038); a real execution error consumes the *execution* attempt. Each attempt increments `nlq_repair_rounds_total{stage}`.
- The service methods return the §9.7 envelope; `Plan` returns it with `Result == nil` and never constructs an executor call path (compile-time: `Plan` has no `exec.Query` reference — and therefore no dry-run: plan-scope callers never contact the engine).

### P1–P7 upholding

- **P1a** — the allowlist walk intersects grants *inside* validation; an empty
  `EffectiveAccess` dataset set short-circuits at the `Run`/`Plan` entry (typed
  `access.none`, no generation, no query). Store/adapter call-count assertions prove no
  query issues on denial.
- **P1b (this phase closes it)** — layered per D-038: read-only credentials/sessions are
  the primary guarantee (phase 10); the engine dry-run's table-grain check runs inside
  `exec.Query` on every engine; the client-side AST walk adds column-grain depth where the
  parser seam covers the dialect. `ValidatedSQL` is the only type `exec.Query` accepts
  (phase 09 compile-time proof, re-asserted here). No layer's absence silently widens
  access: a skipped AST layer is recorded, and table-grain + read-only hold regardless.
- **P1c** — the NLQ path never references `sources.Materializer`; an architecture test
  asserts `internal/nlq` and the read path of `internal/exec` import no write interface.
- **P4** — every repair round is a metric + structured log; an exhausted budget is a
  typed terminal error, never a silent best-effort answer; a zero-row result is a valid
  answer, never repaired into a fabricated one.
- **P5** — `sqlgen` and `sqlfix` are gateway roles, schema-constrained; no provider SDK
  in `internal/nlq`; free-text JSON parse of model output is forbidden and lint-checked.
- **P6** — the terminal error codes are `sql.generation_failed` / `sql.execution_failed`
  (no `repair`/`broken`/`enhance` on the wire); "bounded repair" is an internal
  implementation term only, matching the RFC's own internal usage (§9.3/§9.6). The
  drift-audit forbidden-word scan stays green.
- **P7** — one generation path, one validation core, one executor; the plan/run/preflight/
  refine methods are the core, surfaces (21–23) are thin callers.

## Config keys added

Documented here, in the example config (RFC §14 `nlq`/`exec` domains), and smoke-checked.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `nlq.example_weight_threshold` | float | `0.9` | no | Minimum learned weight for an example to enter as `edit_base`/`hints` (brief 03 §2.5); below it, examples are ranked few-shot only. |
| `nlq.example_similarity_threshold` | float | `0.8` | no | Minimum retrieval similarity for `edit_base`/`hints` eligibility (brief 03 §2.5). |
| `exec.self_repair` | bool | `true` | no | Enables the bounded execution-time repair loop (RFC §14). Off = a warehouse execution error is terminal immediately. Cannot raise the ≤1 cap — the cap is an invariant constant. |

The ≤1 validation-repair and ≤1 execution-repair caps are **not** config (invariant per
P1b/P4). `exec` row cap, statement timeout, and preview rows are phase-10 keys, unchanged.
`nlq` complexity-tier budgets, confidence bands, and example caps (max 7) are phase-17
keys, unchanged.

## Acceptance criteria

1. **Plan→run golden round-trip on the mock stack, D-038 sequencing.** `Preflight → Plan
   → Run` over a fixed question + pinned pack drives the mock gateway (`sqlgen`) + mock
   adapter and produces the §9.7 envelope matching a golden fixture (normalized compare;
   ids resolved to labels); the mock adapter's call log proves the ordering
   **validate → dry-run → execute**, and that `Plan` made no adapter call at all.
   *(master plan)*
2. **Generation is schema-constrained and native-dialect.** `Generate` calls the gateway
   `sqlgen` role with a JSON schema; a malformed/partial model output yields a typed
   error, never a free-text parse; the generation context states the routed source's
   native dialect + SQL requirements and carries **no ANSI-subset directive** (golden
   context fixture — D-038); no provider SDK is importable from `internal/nlq`
   (architecture assertion).
3. **One precedence resolution function.** `ResolvePrecedence` is table-driven unit-tested:
   `edit_base > hints > examples > default`, threshold-gated (weight ≥ 0.9, similarity ≥
   0.8, topic-match), and the top two strategies set `FewShotDisabled = true`.
4. **D-021 intersection proof (AST path, postgres).** Via the `crdb` parser driver on a
   postgres source: an ungranted-but-in-topic table → `table.not_granted`; an
   in-grant-but-not-in-topic table → `table.not_in_topic`; the two are distinct codes at
   **column-grain**, proving pack ∩ grants is an intersection, not a conflation.
   *(master plan)*
5. **Table-grain everywhere (dry-run path, no AST driver).** For a dialect the parser
   seam does not cover, the AST layer is recorded as skipped (never faked), and the
   engine dry-run's referenced-table set is checked against topic ∩ grants **before**
   execution: an ungranted referenced table → typed `table.not_granted` with zero real
   executions (mock adapter: dry-run called, execute never called). *(D-038)*
6. **Join reachability (AST path).** A query joining two topic tables with no declared
   join-graph edge → typed `join.unreachable`.
7. **`ValidatedSQL` + the dry-run gate structurally precede execution.** The
   semantics-aware stage yields `ValidatedSQL` only after the client-side layers pass;
   `exec.Query` accepts no other type (compile-time proof re-asserted) and performs the
   dry-run + table-grain check internally — no call path reaches real execution without
   both; a raw string cannot reach an adapter on the NLQ path.
8. **Bounded validation repair, dry-run errors as fix context.** On a client-side
   validation failure — or a dry-run/EXPLAIN failure, whose engine error message is
   passed to `sqlfix` as dialect-true fix context — ≤1 `sqlfix` attempt runs, is
   revalidated, and a still-invalid result returns typed terminal
   `sql.generation_failed`; `nlq_repair_rounds_total{stage="validation"}` increments
   exactly once. *(master plan)*
9. **Bounded execution repair.** On a real warehouse execution error, ≤1 `sqlfix` attempt
   runs (revalidate → re-execute); a still-failing result returns typed terminal
   `sql.execution_failed`; the round counter increments exactly once;
   `exec.self_repair=false` makes the first execution error terminal.
10. **Plan-scope cannot execute (P1b hard gate).** A caller holding `query.plan` but not
    `query.execute` calling `Run` gets typed `access.scope_missing`, and the adapter call
    count is zero — **including dry-run** (all engine contact sits behind
    `query.execute`). *(master plan)*
11. **Run idempotency.** A repeated `Run` with the same `(operation, tenant, principal,
    client_key)` short-circuits to the cached §9.7 envelope with no second warehouse
    execution (adapter execute call-count = 1 across two calls).
12. **Learn-positive survives restart (DB-first).** A positive feedback event runs the
    learn-positive job, which recomputes and persists the example's weight (Wilson +
    recency + evidence-growth) to the `examples` row; a fresh service instance (cold cache)
    reads the improved weight from the store, not memory. *(master plan)*
13. **Examples lifecycle + dedup.** An example transitions `candidate → active → retired`
    with a typed audit row per transition; a paraphrase duplicate keyed
    `(tenant, topic, example, sample_hash)` is deduped, not re-inserted.
14. **Injection corpus rejected.** The red-team injection corpus (statement smuggling,
    tautology, UNION exfiltration, dialect-escape) is each rejected with a typed §9.5 code
    by the layered validation; `FuzzValidateAllowlist` seed corpus asserts "never panics,
    never yields `ValidatedSQL` for a write". *(master plan)*
15. **Cross-tenant NLQ probe.** A `Run` whose routed topic/dataset belongs to another
    tenant returns typed denial with no query issued; the generator singleton is safe
    under concurrent reuse (`-race`). *(master plan)*

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven `ResolvePrecedence` (all four strategies + threshold edges); the
  allowlist walk over pack/grant intersection fixtures; the §9.7 envelope shaping incl.
  id→label resolution; the repair round-counter caps.
- **Integration:** required — this phase closes the seam phases 09/10/17 opened. Real
  Docker Postgres (`make pg-up`) for `examples`/`feedback_events`/`idempotency_cache`/
  `queries` + the learn-positive job on the real jobs queue; the gateway **`mock`** driver
  is the one sanctioned boundary mock (paired with a recorded-fixture `sqlgen`/`sqlfix`
  test from phase 05). Proves plan/run scope propagation and ≥1 failure mode (repair
  exhaustion). Runs under `-race`. Lives in `internal/nlq` (the wiring boundary) with the
  execution half in `internal/exec`.
- **Adversarial:** required (SQL-safety + access path, §11): cross-tenant NLQ probe
  (criterion 15), empty-access-set short-circuit, injection corpus (criterion 14), a
  schema-escape probe (a table one join-hop outside the pack), a write/DDL-injection
  probe smuggled through generation, a table-grain escape probe on a no-AST-driver
  dialect (criterion 5 — dry-run gate), and a fetch-then-filter regression guard (grants
  intersected inside validation/dry-run, never after execution).
- **Fuzz:** required — `FuzzValidateAllowlist` (generated/submitted SQL → validation) with
  a seed corpus and the asserted invariant "never panics, never returns `ValidatedSQL`
  for a non-SELECT-family or out-of-allowlist statement." Complements phase 09's
  `FuzzValidate`; runs as an ordinary CI test.
- **Bench:** `BenchmarkResolvePrecedence` and `BenchmarkAllowlistWalk` (hot per-request
  artifacts; baseline, not a CI gate).

## Coverage targets

Phase 18 adds no new package: it extends `internal/nlq` (banded 80% by phase 17) and
`internal/exec` (banded 85% by phases 09/10). No new `scripts/coverage-bands.conf` entry
is added; the existing bands hold and the new code must not regress them.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/nlq` | 80% | Default new-package band (already configured, phase 17). |
| `internal/exec` | 85% | SQL-safety / access-path package — the §11 band for `exec`, `access`, and conformance-tested subsystems. The semantics-aware allowlist added here is a P1b safety path carrying standing adversarial + fuzz obligations, so it holds the higher band. |

## Smoke checks

Each criterion maps to one Go test, exercised via `run_group` (SKIPs cleanly until the
package + test exist). Fuzz criterion 14 runs its seed corpus as an ordinary test.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `internal/nlq` `TestPlanRunGolden` PASS (asserts validate→dry-run→execute order) |
| 2 | `internal/nlq` `TestGenerationSchemaConstrainedNativeDialect` PASS |
| 3 | `internal/nlq` `TestPrecedenceResolution` PASS |
| 4 | `internal/exec` `TestAllowlistIntersectionASTPostgres` PASS |
| 5 | `internal/exec` `TestTableGrainDryRunAllowlist` PASS |
| 6 | `internal/exec` `TestJoinReachability` PASS |
| 7 | `internal/exec` `TestSemanticsValidatedSQLUnbypassable` PASS |
| 8 | `internal/nlq` `TestRepairValidationBounded` PASS (incl. dry-run-error fix context) |
| 9 | `internal/nlq` `TestRepairExecutionBounded` PASS |
| 10 | `internal/nlq` `TestPlanScopeCannotExecute` PASS (zero adapter calls incl. dry-run) |
| 11 | `internal/nlq` `TestRunIdempotency` PASS |
| 12 | `internal/nlq` `TestLearnPositiveSurvivesRestart` PASS |
| 13 | `internal/nlq` `TestExamplesLifecycle` PASS |
| 14 | `internal/exec` `TestInjectionCorpus` + `FuzzValidateAllowlist` (seed corpus) PASS |
| 15 | `internal/nlq` `TestNLQAdversarialCrossTenant` PASS |
| config | example config parses with `nlq.example_weight_threshold` / `nlq.example_similarity_threshold` / `exec.self_repair` present |

## Glossary additions

Only terms this phase introduces that are not already in `docs/glossary.md`:

- **Generation strategy precedence** — the single resolution `edit_base > hints >
  examples > default` (RFC §9.3): which prior-SQL source grounds a generation, threshold-
  gated; the top two disable few-shot demos. One unit-tested function, not scattered
  conditionals.
- **Bounded repair** — the internal mechanism that, on a validation failure (including an
  engine dry-run/EXPLAIN failure, whose dialect-true error is the fix context — D-038) or
  a real execution failure, makes at most one `sqlfix` attempt per stage before returning
  a typed terminal error (RFC §9.3/§9.6). Internal term only; the wire error is
  `sql.generation_failed` / `sql.execution_failed` (never "repair" — P6).
- **Learn-positive** — the DB-first learning job (RFC §9.8): on positive feedback it
  re-embeds an example, dedupes paraphrases, and recomputes its routing weight
  (Wilson-score + recency + evidence-growth blend) persisted to the store, so a restart
  never regresses quality.
- **Refine** — a session-scoped re-run of a prior query within a conversation
  (`refine_query`), executing under `query.execute` (RFC §11.1).

## Decisions filed

No new `D-NNN` entry. This phase implements existing decisions: **D-021** (SQL-safety
mechanism — the intersection + defense-in-depth this phase realizes on the NLQ path),
**D-038** (layered read-side validation: credentials primary, engine dry-run table-grain
everywhere, parser-seam column-grain where covered; native-dialect generation), **D-033**
(the `crdb` parser pin the AST path rides; its generation-subset obligation is superseded
by D-038), **D-014** (dual modes, one core — this phase builds the core), **D-020**
(grants/scopes), **D-025** (learn-positive runs on the one leased queue), **D-031** (the
red-team injection corpus this phase's adversarial suite anchors). Any reasonable
deviation discovered in implementation is logged in the Deviation log below and the plan
updated in the same PR (CLAUDE.md §4.3); a genuinely new architectural decision would be
filed as the next decision number in `docs/decisions.md`.

## Deviation log

<!-- Filled DURING implementation, not at authoring time (CLAUDE.md §4.3). -->
- none yet.
