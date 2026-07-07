# Phase 15 — topics-lifecycle

> **Status:** draft
> **Owner:** semantics (Wave 5, critical path — staffed Opus-first)
> **Depends on:** phase-04-access-grants, phase-05-gateway, phase-07-vindex, phase-12-engineering-profiling

Copy this file per CLAUDE.md §16 step 6:

```bash
cp docs/plans/_template.md docs/plans/phase-15-topics-lifecycle.md
cp scripts/smoke/_template.sh scripts/smoke/phase-15.sh
```

---

## RFC / request sections

- **RFC-001 §8.1** — topics and packs; the topic-generation job (`enhance` role, DRAFT-first).
- **RFC-001 §8.2** — the lifecycle state machine and its **seven binding rules** (brief 05's
  unified machine, carried verbatim). This is the phase's spine.
- **RFC-001 §8.3** — context engineering: facet decomposition at publish, the capability
  contract's hard structural caps, one-owner pruning boundary (this phase owns the
  *structural caps* half; the complexity-tier token budget is phase 17's `ContextAssembler`).
- Supporting: **§5.2–5.3** (the one resolver + capability scopes — transition authority),
  **§12** (the budgeted tables `topics` / `topic_versions` / `topic_audit` / `facet_vectors`),
  **§13** (`enhance` gateway role), **§15** (content-free typed audit), **§2** (the P6
  forbidden-word list and the `revalidate` / `re-check source` renames).

## Depends on

- **phase-04-access-grants** — the single `access.Resolve(envelope) → EffectiveAccess`. Every
  lifecycle transition's authority (scope ∩ grant) is computed by that resolver, not by a
  bespoke check here (RFC §8.2 rule 5). This phase *consumes* the resolver; it never
  re-implements one.
- **phase-05-gateway** — the `enhance` role (schema-constrained) drives topic generation
  (§8.1). No provider SDK touches this package (P5).
- **phase-07-vindex** — facet upsert/delete scoped `(tenant, topic, version)` is the sink for
  facet decomposition on publish and the target of the invalidation matrix (§8.3).
- **phase-12-engineering-profiling** — dataset schema + profiles are the *input* to topic
  generation and the source of table-level availability/health that gates contracts.

## Informing briefs

- **Brief 05 — predecessor diff (the backbone).** The unified state machine, the seven
  binding rules, the carry table (`update_table` reference-rewriting, source-table health,
  archive cache-clear, export/import sanitization), and the **binding invalidation matrix**
  (stage-10 table).
- **Brief 03 §2 — context engineering.** Facet decomposition into typed units; the capability
  contract's **hard structural caps** (top-5 measures / top-5 dimensions / top-3 joins /
  top-2 query-patterns; ~200-char definition truncation); "never mutate source evidence,
  keep the unpruned contract in metadata"; the packer/generator duplication trap this phase
  avoids by owning caps in *one* place.
- **Brief 07 — WrenAI/MDL.** Selective column exposure (undeclared columns are invisible),
  equality-only join conditions as a narrowing constraint, and the markdown-source-of-truth /
  disposable-index discipline (facets are a rebuildable cache over the durable pack).

## Brief findings incorporated

From **brief 05** (verbatim where it is a binding rule):

1. **The unified four-stage version machine** `DRAFT → REVIEW → PUBLISHED → DEPRECATED` with
   the orthogonal topic-level `ACTIVE ⇄ ARCHIVED`, and the exact transition set
   (create/update-in-place/discard/submit-review/publish/rollback/supersede/archive).
2. **Exactly one PUBLISHED-active version per topic**; publish is an atomic
   `active_version_id` swap that captures the prior id for rollback lineage (rule 1).
3. **The active version is never mutated in place and never discarded** — the `discard` guard
   carried verbatim; edits branch a DRAFT (rules 2).
4. **Every transition is a typed audit row** (actor + change counts); no silent stage change
   (rule 3, P4).
5. **Source-table health is a first-class lifecycle input** — table-level `is_available` +
   `health_status` + `health_message`; unavailable tables are excluded from routing/generation
   and from the capability contract, and a **re-check source** flow refreshes schema and clears
   resolved issues. The client's `update_table` **reference-rewriting** (rewrite every
   measure/dimension/KPI/join reference on a source-table rename/replace) is carried (rule 4).
6. **Transition authority = the one access resolver** over the grant model, tenant predicate
   `NOT NULL` (rule 5) — pinned as an explicit **transition-authority matrix** below.
7. **Invalidation is part of the transition, not a follow-up** — the brief-05 stage-10 matrix
   is binding: publish/rollback enqueue facet reindex; archive clears routing caches + removes
   facets; discard de-indexes the draft; tenant-delete cascades facet cleanup (rule 6).
8. **Export/import with id sanitization** — the sanitizer is the **P6 reference implementation**
   for externally-visible artifacts; post-import sample-values refresh is enqueued (rule 7).
9. The template **`repair`** action/vocabulary is **not** carried onto any surface; the
   operations are `revalidate` and `re-check source` (RFC §2).

From **brief 03 §2**: the two-layer compression with **all structural caps in one owner**
(`semantics` — the contract projection), the unpruned contract preserved in result metadata,
facet decomposition into the same typed units retrieval matches, and the pack's own validators
(canonicalize ids, resolve KPI→measure by id-then-name, reject joins over undefined tables,
prune dangling context keys recording migration warnings).

From **brief 07**: **selective column exposure** (a column absent from the pack is invisible
to the contract and to routing — omission, not a runtime filter, composing with P1a);
**equality-only join conditions** as a pack-validator constraint (narrows the correctness/
injection surface at the join graph); **facets are a disposable index** rebuilt from the
durable pack on publish/rollback — never the record of truth (D-004 discipline, RFC §8.2 rule 6).

## Findings I'm departing from

- **Brief 07's first-class `cube` (structured aggregation) object — not adopted in V1.** The
  RFC's §8.1 pack inventory is measures / dimensions / derived-KPIs / join graph; adding a
  distinct cube object would exceed that inventory (RFC > brief). The measure + derived-KPI
  model already covers the aggregation class; a cube is a post-V1 consideration, noted for the
  RFC, not built here.
- **Brief 05's richer schedule triggers (`condition`/`event`) — out of this phase and V1.**
  Scheduling is phase 13/§7.7 (cron/interval only, D-013). Nothing here.
- **Brief 05's MLflow per-node tracing — concept only, via the telemetry seam.** No
  provider-specific tracer (P5); lifecycle transitions emit structured telemetry through
  `internal/telemetry`.
- **Brief 03's `char/4` rules-budget currency and the two-independently-tuned-confidence
  scores — not this phase.** Token budgeting and the one calibrated confidence primitive are
  phase 17's `ContextAssembler`; this phase applies only the *structural* caps (a count/length
  discipline, not a tokenizer budget), keeping the pruning owner boundary clean.
- **Brief 03's SQL-text-in-logs (fork's `_sql_snippet`) — rejected.** Audit and logs are
  content-free (§7, §15); only the dialect-aware *structured* error/audit is carried.

## Scope

Delivers `internal/semantics` (the first code in the package):

- **The pack model + validators.** `TopicPack` (enhanced tables with columns/keys, keyed
  measures / dimensions / derived-KPIs, a join graph, semantic context, query patterns,
  governed-rule *anchors* — the rule bodies themselves land in phase 16) and its validators:
  id canonicalization, KPI→measure reference resolution (id-then-name), join-graph
  reachability + **equality-only** condition check, dangling-context-key pruning with recorded
  `migration_warnings`, and **selective column exposure** (undeclared columns never surface).
- **The lifecycle engine.** The `DRAFT → REVIEW → PUBLISHED → DEPRECATED` version machine and
  the orthogonal `ACTIVE ⇄ ARCHIVED` topic machine, over `topics` / `topic_versions` /
  `topic_audit`: update-in-place-for-DRAFT-only, the discard guard, the atomic publish swap
  with prior-id capture, rollback to the prior published version, supersede-on-next-publish,
  and the typed `topic_audit` row on every transition.
- **The transition-authority matrix**, resolver-computed (scope ∩ grant), tenant predicate on
  every read/write.
- **Source health as lifecycle input** — table-level availability/health fields, health-gated
  accessors, the **re-check source** flow (schema-refresh job, resolved-issue clear), and
  **reference-rewriting** on source-table rename/replace across measures/dims/KPIs/joins.
- **Facet decomposition → vindex on publish** — typed facets (measure / dimension /
  derived-KPI / query-pattern / example) upserted under `(tenant, topic, version)`, batched;
  the **invalidation matrix** wired into each transition (via the `internal/jobs` queue for
  reindex).
- **Capability-contract projection** — the compact, prompt-safe view with the hard structural
  caps (top-N, definition truncation), health-filtered, undeclared-column-free; the unpruned
  contract retained for result metadata.
- **The topic-generation job** — `enhance` gateway role, schema + profiles → schema-constrained
  DRAFT synthesis; publication is always an explicit human gate (never auto-publish).
- **Export/import with the P6 sanitizer** — id/metadata stripping on export, re-canonicalization
  on import, post-import sample-values refresh enqueued.

## Non-goals

- **Governed-rule bodies + clarification patterns** — phase 16 (this phase carries only the
  pack anchor/slot where a rule attaches).
- **Routing / retrieval / context *token* budgeting** — phase 17 (`ContextAssembler` consumes
  the contract this phase projects).
- **SQL generation / validation / execution** — phases 18/09/10 (no generated SQL executes
  here; P1b hard gate).
- **The HTTP topic endpoints and the MCP describe/list tools** — phases 21/22 (thin surfaces
  over this core; a lifecycle transition's side effects all live *here* so no surface can omit
  them, P7).
- **Warehouse-write / materialization** — engineering stage (P1c); semantics never writes to a
  customer source.
- **A first-class cube object, condition/event schedules, cross-topic relationship discovery**
  — out of V1 (see departures).

## Design

### Data flow

```
profiles + schema (ph12) ──▶ topic-generation job (enhance role, ph05) ──▶ DRAFT TopicPack
                                                                              │
   author edits (thin surfaces) ─▶ persistMutation ─▶ DRAFT (in place, diff+audit)
                                                                              │  submit
                                                                              ▼
                                       REVIEW ──(validators + resolver authority)──┐
                                                                              │     │ reject
                       publish (scope topic.publish ∩ grant manage)          │     ▼
                                                                              ▼   (back to DRAFT)
   active swap ─▶ facet-decompose ─▶ vindex upsert (tenant,topic,ver) ─▶ PUBLISHED ◀─ rollback
                 (reindex job, ph07)                                          │
                                              supersede-on-next-publish       │  archive (topic)
                                                                              ▼
                                                                          DEPRECATED       ACTIVE ⇄ ARCHIVED
```

One shared core, constructed once; the HTTP (§11.2) and MCP (§11.1) surfaces are thin callers
(P7). Every transition's validation, audit, and invalidation are core-side, so a surface
cannot skip them.

### Key types (indicative)

- `TopicPack`, `EnhancedTable` (carries `IsAvailable bool`, `HealthStatus ∈ {healthy,
  unavailable,disabled}`, `HealthMessage string`), `Measure`, `Dimension`, `DerivedKPI`,
  `JoinEdge` (equality-only condition), `SemanticContext`, `QueryPattern`.
- `CapabilityContract` — the projected, capped, health-filtered, prompt-safe view; a pure
  function `Project(pack, caps) → CapabilityContract` (no I/O, deterministic — golden-tested).
- `Lifecycle` — the transition engine: `CreateDraft`, `UpdateDraft` (DRAFT-only guard),
  `DiscardDraft` (active-guard), `SubmitReview`, `Publish`, `Rollback`, `Archive`,
  `RecheckSource`, `RebindSourceTable`. Each takes the frozen `identity.Envelope`, resolves
  authority, mutates through the `store` seam under a tenant predicate, writes a typed
  `topic_audit` row, and enqueues its invalidation.
- `FacetSet` + `Decompose(pack) → []Facet` (typed kinds) and the publish-time upsert into
  `vindex`.
- `Sanitizer` — `SanitizeExport(pack)` / `CanonicalizeImport(bundle)`; the reference P6
  id-stripper.

### The transition-authority matrix (RFC §8.2 rule 5; scopes §5.3, grants §5.1)

Both a **capability scope** (token-carried, gates the operation family) and a **grant**
(resource-level, gates *this* topic) must pass; the resolver computes both, deny-by-default,
admin as an explicit sentinel. Tenant predicate always intersected.

| Transition | Required scope | Required grant on topic | Notes |
|---|---|---|---|
| generate (create DRAFT job) | `topic.write` | `manage` | async; enhance role |
| update DRAFT in place | `topic.write` | `manage` | DRAFT stage only (guard) |
| discard DRAFT | `topic.write` | `manage` | active version can never be discarded (guard) |
| submit DRAFT→REVIEW | `topic.write` | `manage` | validators must pass |
| publish REVIEW→PUBLISHED | `topic.publish` | `manage` | atomic active swap + reindex |
| rollback | `topic.publish` | `manage` | re-point to prior published |
| archive (topic-level) | `topic.write` | `manage` | clears caches + removes facets |
| re-check source / rebind | `topic.write` | `manage` (+ `read` on the source) | schema refresh reads the source |
| export | `topic.read` | `read` | read-only projection |
| import (new DRAFT) | `topic.write` | `manage` | re-canonicalized, sanitized |

A caller missing either half is denied with a typed `access.*` error, **no state change, no
partial write** (P4), the decision metered (`access_decisions_total`).

### Source health, re-check, and reference-rewriting

Table-level health lives on `EnhancedTable`; accessors (`ActiveTables`, `ActiveMeasures`, …)
filter on `IsAvailable`, so an unavailable table is excluded from the capability contract and
from facet decomposition — never producing broken SQL downstream (brief 05 rule 4, P4).
`RecheckSource` enqueues a schema-refresh job (`recalculate_joins`, `preserve_manual_edits`),
updates health, and clears resolved issues; on a rename/replace, `RebindSourceTable` rewrites
**every** reference (measure/dimension/KPI/join) so no dangling reference survives — carried
from the client's `update_table` (the fork lacked it). The operations are named `revalidate`
and `re-check source`; the `repair` vocabulary appears on no surface (P6, enforced by
drift-audit).

### Facet decomposition + the binding invalidation matrix (RFC §8.2 rule 6)

Facets are a **disposable index over the durable pack** (brief 07): on publish/rollback the
active pack is decomposed into typed facets and upserted to `vindex` under
`(tenant, topic, version)`, in configurable batches. The invalidation is *part of* the
transition, enqueued on the one `internal/jobs` queue:

| Transition | Metadata effect | vindex effect | Cache effect |
|---|---|---|---|
| DRAFT update | new payload, same version_number | none (draft not indexed) | — |
| DRAFT discard | row deleted, `discard` audit | delete facets(version) | — |
| publish | `active_version_id` swap, `publish` audit | reindex(version) | routing cache refreshed |
| rollback | `active_version_id` re-point, `rollback` audit | reindex(prior version) | routing cache refreshed |
| archive (topic) | status `archived` | facets removed | routing cache **+ template-prior cache cleared** |
| tenant delete | cascade | facet cleanup (tenant scope) | — |

A transition that fails to enqueue its invalidation is a bug the matrix test catches; context
projections are **regenerated** from the active pack, never held as a separate mutable store.

### Capability-contract caps (one owner)

`Project` applies the hard structural caps (top-N measures/dimensions/joins/patterns,
definition truncation) and health/column filtering, then stops — the complexity-tier token
budget is phase 17's, in a *different* component (avoiding the predecessors' packer/generator
duplication trap, brief 03 §2.2). The unpruned contract is preserved in result metadata; the
projection is a pure, deterministic, I/O-free function (golden-tested).

### How it upholds P1–P7

- **P1a** — every store read/write takes non-optional scope params from the resolver; empty
  effective set short-circuits (no query). Selective column exposure composes with grants.
- **P3** — tenant predicate on every `topics`/`topic_versions`/`topic_audit`/`facet_vectors`
  access; a cross-tenant transition matches nothing.
- **P4** — health gating excludes unavailable tables (no broken SQL); every transition is a
  typed audit + metric; denied authority is a typed error, never a silent no-op.
- **P5** — topic generation goes through the `gateway` `enhance` role, schema-constrained; no
  provider SDK here.
- **P6** — the export sanitizer is the reference id-stripper; `repair`/plumbing words banned on
  every surface (drift-audit).
- **P7** — one lifecycle core; side effects (validation, audit, invalidation) are core-side so
  the HTTP and MCP surfaces stay thin.

## Config keys added

Documented here, in the example config, and smoke-checked (§4.2). The structural caps
implement brief 03 §2.2 (the contract-projection half of §8.3); they are a `semantics`-domain
addition to the §14 surface (a plan-level config addition, not an architecture decision).

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `semantics.contract.max_measures` | int | 5 | no | top-N cap (brief 03 §2.2) |
| `semantics.contract.max_dimensions` | int | 5 | no | top-N cap |
| `semantics.contract.max_joins` | int | 3 | no | top-N cap |
| `semantics.contract.max_query_patterns` | int | 2 | no | top-N cap |
| `semantics.contract.definition_max_chars` | int | 200 | no | definition truncation |
| `semantics.facet_batch_size` | int | 64 | no | vindex upsert batch on publish |
| `semantics.generation.enabled` | bool | true | no | topic-generation job on/off |

Fail-loud validation: non-positive caps or batch size ⇒ refused boot (typed config error).

## Acceptance criteria

1. **Pack validators.** `Validate(pack)` canonicalizes entity ids, resolves KPI→measure
   references id-then-name, rejects a join path referencing an undefined table, rejects a
   non-equality join condition, and prunes dangling semantic-context keys while recording a
   `migration_warnings` entry — asserted against a golden fixture corpus.
2. **Exactly-one-active-version under concurrency.** Two concurrent `Publish` calls on the same
   topic yield exactly one `active_version_id`; the loser returns a typed conflict, and no
   second active version exists (test under `-race`, Docker Postgres).
3. **DRAFT-only mutation + discard guard.** `UpdateDraft` on a non-DRAFT stage is a typed
   error; `DiscardDraft` on the active version is a typed error; neither mutates state.
4. **Atomic publish swap + rollback.** `Publish` swaps `active_version_id` capturing the prior
   id; `Rollback` re-points to the prior published version; a mid-swap failure leaves the old
   active version intact (no half-published state).
5. **Typed audit on every transition.** Each of create/update/discard/submit/publish/rollback/
   archive writes exactly one `topic_audit` row (typed action + actor + change counts); a
   transition that skips its audit fails the test.
6. **Transition-authority matrix.** A table-driven suite proves each transition allows only the
   matrix's `(scope ∩ grant)` and denies every deficient caller with a typed `access.*` error
   and no state change (resolver-computed, tenant-scoped).
7. **Health gating.** An `IsAvailable=false` table is excluded from the capability contract and
   from the decomposed facet set (health-gating proof over a fixture with mixed availability).
8. **Re-check + reference-rewriting.** `RecheckSource` clears a resolved health issue; a
   source-table rename rewrites every measure/dimension/KPI/join reference with zero dangling
   references remaining (fixture assertion).
9. **Facet decomposition on publish.** Publishing decomposes the active pack into the typed
   facet kinds and upserts them to `vindex` under `(tenant, topic, version)` in batches of
   `semantics.facet_batch_size` (asserted via the vindex/mock call log).
10. **Invalidation matrix binding.** Each transition enqueues exactly the vindex + cache
    effect its matrix row prescribes (publish/rollback → reindex; discard → delete-facets;
    archive → remove-facets + clear caches; tenant-delete → tenant facet cleanup) — a
    matrix-driven test, no row omitted.
11. **Capability-contract caps + one owner.** `Project` enforces the top-N caps and definition
    truncation, excludes undeclared columns, is pure/deterministic (same input ⇒ same output,
    no I/O), and the unpruned contract is preserved in metadata.
12. **Topic generation via `enhance`, human-gated.** The generation job produces a DRAFT via
    the `enhance` role (schema-constrained; mock driver in CI) and never auto-publishes
    (post-job stage is DRAFT).
13. **Export carries no internal ids.** `SanitizeExport` output contains no store id / internal
    metadata (golden); `CanonicalizeImport(SanitizeExport(pack))` round-trips to an equivalent
    pack and enqueues a post-import sample-values refresh.
14. **No `repair` (or plumbing) word on any surface.** `make drift-audit` finds no
    `repair`/`broken`/`reindex`/`embedding`/`index`/`sync` term in any wire-facing string,
    field name, or description in this package.

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven validators (criterion 1), the pure `Project` projection with golden
  fixtures (11), the authority matrix (6), the invalidation matrix (10), reference-rewriting
  (8) — all mock/in-memory where they need no store.
- **Integration:** required — this phase consumes phase-04 (resolver), phase-07 (vindex),
  phase-05 (gateway), and phase-12 (profiles), and opens the `semantics` interface phases
  16–20 build on. Real Docker Postgres for `store` + `vindex`; a real frozen envelope for
  authority; the gateway **`mock`** driver (the one sanctioned boundary mock) for `enhance`,
  paired with a recorded-fixture test. Proves: the concurrent-publish invariant (2), audit
  persistence (5), facet upsert scoping (9), and identity/scope propagation through a
  transition. Lives in-package (the package *is* the wiring boundary) plus
  `test/integration/` for the cross-subsystem publish→vindex path.
- **Adversarial:** the authority matrix suite is the ACL path — a cross-tenant transition
  probe (a `manage` grant in tenant A cannot transition a tenant-B topic), an empty-access-set
  probe (no grant ⇒ typed `access.none`, no store query — asserted by call-count), and a
  forged-envelope attempt (authority read only from the frozen envelope, never a header). A
  fetch-then-filter regression guard asserts the tenant predicate is inside the query.
- **Fuzz:** `FuzzCanonicalizeImport` over the import bundle (a parse/decode surface) with a
  seed corpus, asserting the invariant "never panics, never emits a pack with an unresolved id
  or an undeclared-column reference."
- **Bench:** `BenchmarkProjectContract` (the hot, reused projection) and
  `BenchmarkDecomposeFacets` — baselines, not a CI gate.

## Coverage targets

Package default 80% (§11). The security-critical **lifecycle transition, audit, and
invalidation** code is separately held to **≥85%** by the enumerated conformance-style tests
(criteria 2, 5, 6, 10) — a documented sub-target rather than a separate mechanical band, since
the package also contains projection/export/generation code that does not warrant the 85% band
package-wide. If, at implementation, the lifecycle core is factored into its own file/subpackage
whose coverage is separately measurable, promote it to an explicit `85` band in the same PR.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/semantics` | 80% | new-package default; lifecycle/audit/invalidation sub-area ≥85% via conformance tests (criteria 2/5/6/10) |

`scripts/coverage-bands.conf` entry added in this PR: `internal/semantics 80`.

## Smoke checks

Each criterion maps to a Go test the smoke script runs via `run_group` (SKIPs cleanly until the
test exists / the store URL is set).

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestPackValidators` PASS |
| 2 | `TestExactlyOneActiveVersionConcurrent` PASS (`-race`) |
| 3 | `TestDraftOnlyMutationAndDiscardGuard` PASS |
| 4 | `TestAtomicPublishSwapAndRollback` PASS |
| 5 | `TestTypedAuditOnEveryTransition` PASS |
| 6 | `TestTransitionAuthorityMatrix` PASS |
| 7 | `TestHealthGatingExcludesUnavailableTables` PASS |
| 8 | `TestRecheckSourceAndReferenceRewriting` PASS |
| 9 | `TestFacetDecompositionOnPublish` PASS |
| 10 | `TestInvalidationMatrix` PASS |
| 11 | `TestCapabilityContractCaps` PASS |
| 12 | `TestTopicGenerationHumanGated` PASS |
| 13 | `TestExportSanitizerNoInternalIds` PASS |
| 14 | `make drift-audit` clean (no `repair`/plumbing word in `internal/semantics` wire strings) |

## Glossary additions

Pre-written for `docs/glossary.md` (same PR, §14) — only terms not already present:

- **Facet** — a typed, embeddable semantic unit decomposed from a published topic pack: one of
  `measure` / `dimension` / `derived-KPI` / `query-pattern` / `example`. Facets are what
  retrieval matches against (never raw schema); they are a **disposable index** over the durable
  pack, rebuilt on publish/rollback, scoped `(tenant, topic, version)` in `vindex`.
- **Facet decomposition** — the publish-time operation that turns the active pack into its
  facet set and upserts it to `vindex`.
- **Transition-authority matrix** — the binding mapping of each lifecycle transition to the
  `(capability scope ∩ resource grant)` it requires, computed by the one access resolver
  (RFC §8.2 rule 5).
- **Invalidation matrix** — the binding mapping of each transition to its metadata / `vindex` /
  cache effect; the effect is part of the transition, never a follow-up (RFC §8.2 rule 6).
- **Table-level source health** — a pack table's `is_available` / `health_status` /
  `health_message`; an unavailable table is excluded from the capability contract and facet set
  (fail-loud on source drift, P4).

Existing terms reused unchanged: *topic*, *topic pack*, *capability contract*, *re-check
source / revalidate*, *grant*, *principal*, *session*.

## Decisions filed

No new `D-NNN`. This phase implements existing decisions:

- **D-020** — the one grants relation + one resolver; the transition-authority matrix is that
  resolver applied to lifecycle transitions.
- **D-021** — validation ∩ grants; the pack's schema/allowlist is the topic-pack half the exec
  validator (phase 09/18) intersects with caller grants.
- **D-025** — the one leased job queue: topic generation, publish reindex, and re-check-source
  run as typed handlers on it (no bespoke worker class).
- **D-029** — `vindex` facets-only; this phase is the primary producer of facet vectors.
- **D-027** — governed rules/clarification are V1-scoped but land in phase 16; this phase
  carries only the pack anchor a rule attaches to.

## Deviation log

<!-- Filled DURING implementation (§4.3): every reasonable deviation, why, and confirmation
     this file + the example config + the smoke script were updated in the same PR. Empty at
     authoring time. -->

_none yet._
