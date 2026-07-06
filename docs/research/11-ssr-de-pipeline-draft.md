# Brief 11 — SSR Analyst Draft: Demand-Driven Medallion Build

> Status: draft · 2026-07-06 · source: external_refs/ssr_analyst_analysis_DE_pipeline (internal draft)

## Summary

The staged draft is an internal, non-validated first pass at Chartworks' data-engineering
(DE) stage: a **demand-driven medallion** that builds bronze/silver/gold tables and a topic
pack lazily, only for what a business goal actually needs, matching top-down against what
already exists and building bottom-up what's missing. It pairs a "blind" planner (no
catalog access, so gaps are detectable, not silently trimmed) with a retrieve → verify →
confirm matching discipline, a global canonical-identity registry, whitelist-only LLM-composed
transformations (never freeform SQL), contract-before-materialize human gates, and a
dynamically-unfolding, content-addressed execution graph. The core reuse-economics idea and
several of its safety disciplines are strong and worth carrying into the RFC's DE-stage
design. Its weakest parts — the refresh/drift pipeline, the transform whitelist itself, the
external-toolchain assumption, and the topic-pack integration — are explicitly unfinished in
the source document itself, and the draft is silent on the DE-write-path governance D-017
requires. **Verdict: take the demand-driven matching/build discipline, the blind planner,
the canonical-registry pattern, and the retrieve→verify→confirm/whitelist-composition safety
patterns; ditch (or substantially redesign before adopting) the refresh pipeline, the assumed
dbt/SQLMesh/Cocoon toolchain, and the topic-pack integration, and treat the full five-rung
machinery as a scope decision to validate narrower before committing to in full.**

---

## 1. What the draft actually proposes

**Two pipelines.** A **build** pipeline, triggered by a new natural-language business goal
("stand up subscription-revenue analytics for finance"), and a **refresh** pipeline that runs
on a schedule to keep the already-built medallion current and watch for drift. Only the build
pipeline is actually specified; the refresh pipeline's dedicated document is a placeholder
image and the word "TBD" — the drift-detection story is *asserted* in the overview (use
fingerprints and value baselines to catch a "silent meaning shift," escalate anything
ambiguous to a human) but not designed anywhere in the draft.

**Five rungs, matched top-down, built bottom-up.** A goal decomposes into **topic** (the
finished deliverable — one pack per goal), **gold** (one star schema per metric/"component"),
**silver** (conformed, contracted base tables and dimensions), **bronze** (raw sources landed
into the warehouse, renamed and safely cast, mess kept), and **raw** (the customer's own
uncurated warehouse). Each rung asks the same question — reuse what exists, or build it? —
and a miss hands a smaller, annotated need down to the rung below. Because the lower layers
are shared and cumulative, later goals increasingly land on tables an earlier goal already
built, which is the entire cost argument for demand-driven scoping.

**The planner (analyst/agent role #1) is deliberately blind.** It takes one NL capability
sentence and, **without reading the catalog**, emits a typed, enumerable "requirement
family": a list of `components` (one per metric, each carrying a closed-grammar derivation
formula over *concepts* — not columns — plus additivity, facets, population, window, and the
dimensions it slices by) and a shared `dimension_pool`. Every field exists only because a
specific downstream check consumes it; free-text fields are retrieval-only, never a decision
input. The stated reason for blindness: if the planner could see what's already built, it
might silently trim the goal to fit inventory, hiding a real gap instead of surfacing it.

**Matching (analyst/agent role #2) runs a fixed three-move shape at every rung: retrieve →
verify → confirm.** Retrieve is a fuzzy, recall-biased shortlist (embeddings, fuzzy names,
usage) that is allowed to be wrong because it never decides anything. Verify is deterministic
code comparing *declared meaning* — never names — and rejects on any hard conflict; this is
where the actual decision is made, and an LLM is explicitly never given veto power over a
reuse call. Confirm is a cached-verdict lookup, or failing that an empirical "compute the
answer both ways and diff it" probe, escalating to a human when the diff is ambiguous, and
caching whatever verdict lands. The threshold is deliberately asymmetric and fail-closed:
a wrong reuse is silent and inherited forever; an unnecessary rebuild is merely wasteful.
"Unsure" always means "don't reuse."

The **interface degrades as you descend**: gold and silver can verify on declared semantics
(formulas, canonical columns, contracts); bronze can only do a lineage/name lookup (no
contract exists yet); raw has nothing declared at all, so it runs a broad, recall-biased
multi-signal search plus a hard **realizability gate** — every required piece must map to a
real, connectable source, or the draft escalates to a human rather than fabricating one.

**A global canonical registry underwrites all of it.** Every rung resolves its own local name
(a physical column, a metric's informal name) to one durable, tenant-scoped canonical
identity before comparing anything; matching is always id-against-id, never string-against-
string. An id is minted once, the first time a concept is conformed, and every later mention
resolves against that same entry. If a name can't be confidently resolved, the system does
not guess and does not mint a duplicate — it stops and asks a human, because minting a second
identity for a concept that already has one is, per the draft, the one truly silent and
unrecoverable failure mode. The draft itself flags this registry's durability as the single
most load-bearing assumption in the whole design.

**Build (analyst/agent role #3) composes, it never writes free SQL.** At every layer, an LLM
selects and orders transformations from a small, layer-specific curated whitelist (dbt-utils-
style patterns); anything the whitelist doesn't cover is a gap escalated to a human or
explicitly added to the whitelist — never auto-generated. The composed pipeline renders into
a dbt model, is statically validated before touching the warehouse, executed (`dbt run`),
and enforced with generated tests (`dbt test`). At silver specifically — where meaning is
actually established — profiling first produces a **data contract** (schema, types, ranges,
plus every irreducible cleaning assumption the data alone can't resolve: dedup rules,
imputation, ambiguous-format handling) that a **human approves before the table is
materialized**, and every irreducible assumption becomes a permanent test tripwire.

**Orchestration treats the dependency graph as something matching computes, not something
that's designed up front.** Because a leaf's real source often isn't known until raw
table-search runs, and because human verdicts can prune, graft, or demote whole subtrees, the
draft models execution as one frontier-expanding loop over a persisted, content-addressed
node set: reuse is a node born already "done"; a match/discovery node's job is to *emit* the
nodes below it when it runs; a human gate blocks only its own branch while siblings keep
moving. The draft is explicit that the orchestrator never orders the actual SQL — a build
node just hands a model to dbt (or, notably, to whatever engine ends up executing it) and
dbt's own DAG does that ordering. Delivery stays atomic even though execution is partial and
asynchronous: the topic pack is a single terminal node, built once, over whichever components
actually resolved (unrealizable ones cleanly excluded, not faked).

**Outputs / metadata.** Each layer writes back exactly the fields its own matcher (and the
one above it) will read next time — canonical ids and facets at silver, measure derivations
and grain at gold, lineage and fingerprints at bronze, a signature side-index at topic. This
write-back is what makes reuse compound across goals over time.

---

## 2. What is genuinely good

- **Demand-driven, top-down-match / bottom-up-build scoping.** Nothing is engineered
  speculatively; a goal only pulls in the layers it actually needs, and the medallion gets
  cheaper to extend the more it's used. This is a concrete mechanism for exactly the "lean,
  budget-friendly" DE-stage crown jewel D-013 asks the RFC to keep and enhance, rather than a
  restatement of "do ETL smartly."
- **The blind planner.** Planning without catalog access converts a *silent* requirement
  trim into a *detectable* gap (something a human or a downstream realizability check can
  catch) — a direct, planning-stage instance of the P4 "fail loud, never silently degrade"
  property, applied one layer earlier than P4 is usually discussed.
- **The canonical registry / resolve-then-compare discipline.** Comparing identities instead
  of names, minting once and resolving forever, and treating an unconfident resolution as an
  escalation rather than a guess is a clean, reusable mental model for how Chartworks' topic-
  pack semantic layer could keep measure/dimension identity stable across the topic
  lifecycle the predecessor-diff brief will need to reconcile.
- **Retrieve → verify → confirm with a fail-closed asymmetric threshold.** The LLM never gets
  to say "yes, reuse" — only deterministic code does, and "unsure" always defaults to the
  safe (expensive) branch. This is a directly portable pattern anywhere Chartworks lets a
  model propose something consequential, not just in the DE stage.
- **Whitelist-only transformation composition (no freeform LLM SQL).** Bounding what an LLM
  is even allowed to propose to a small, curated, per-layer set of vetted operations is the
  same shape of safety property P1's SQL-safety mandate wants for NLQ-generated SQL —
  schema/operation allowlisting instead of trusting free-text output. Worth evaluating
  whether the same pattern (a curated grammar the model composes over, rather than authors)
  should also shape the NLQ SQL-generation design, not just the DE stage.
- **Contract-before-materialize, human-gated on irreducible/PII decisions.** A concrete,
  auditable mechanism for exactly the kind of human-in-the-loop point a real DE stage needs,
  scoped narrowly (irreducible assumptions and PII, not every table) so it doesn't require a
  person on every reuse.
- **The dynamically-unfolding, content-addressed execution graph.** Correctly rejects the
  false assumption that a build DAG can be fully planned before discovery runs — a leaf's
  real source is often unknowable until raw search executes. The four primitives (a step
  result that can emit more graph, a frontier-scheduling tick, placeholder-dependency
  forwarding, and out-of-band verdicts that only ever add or cut) are a reasonable, engine-
  agnostic mental model regardless of what actually executes the work.
- **Grow-by-addition / immutable versioning.** New version or variant, never in-place
  mutation of a built asset — the same discipline the Store seam already commits to
  (forward-only migrations, §9), applied to engineered datasets.

---

## 3. What is weak, unvalidated, or overcomplicated

- **The refresh pipeline is essentially unwritten.** Its dedicated document is "TBD" despite
  being co-equal to the build pipeline in the overview; drift detection via fingerprints and
  value baselines is asserted, not designed. At most this names a real, still-open phase-0
  problem — it is not a design to adopt.
- **The silver transform whitelist and the defect→transform mapping are explicitly
  unspecified in the source document itself.** The draft says outright that "the curated
  silver set itself and the defect→transform mapping are open" — arguably the single most
  load-bearing implementation detail (what an LLM is even allowed to compose) doesn't exist
  yet, only a hoped-for external guidance basis.
- **Heavy, unevaluated reliance on an external toolchain.** The build layer assumes dbt (or
  SQLMesh, left open), an external lineage tool, and a third-party transform-composition
  aid, with the orchestration engine itself left open between a hand-rolled prototype,
  Dagster, Prefect, or Temporal. None of this is weighed against Chartworks' CGo-free,
  single-static-binary posture (D-005) or license/operational fit — it reads as an
  architecture sketched around familiar tools, not a fit assessment for this repo.
- **The canonical registry's durability is a foundational unknown, not a detail.** The draft
  itself calls it "the top open risk": every cross-rung, cross-time matching claim holds only
  if this registry is one stable namespace forever, and no bootstrap, migration, or drift-
  recovery story exists for it.
- **A lot of conceptual surface for a V1 that must also migrate/enhance the NLQ core in the
  same wave (D-013).** Five rungs plus two-phase build-unit consolidation plus a
  self-rewriting placeholder graph is a large matching engine to build before there's
  evidence demand-driven scoping saves meaningful effort on this product's actual sources.
- **Topic-level matching — nearest the inherited topic-pack semantic layer — is explicitly
  off in v0** ("every goal mints a new pack"). The seam that matters most for reconciling
  with the predecessor topic lifecycle is the least-designed part of the draft.
- **Numerous human-in-the-loop points, several themselves marked deferred or open**
  (scope ratification, gate wave-batching, verdict authority/audit). The draft candidly
  flags that whether a non-technical reviewer can adjudicate a semantic cleaning assumption
  is unresolved — a real adoption risk, not just an implementation gap.
- **Requirement-enumeration completeness is acknowledged as structurally unanswerable** — no
  ground truth exists for "did the planner list every needed metric," and a more capable
  planner only makes an incomplete list *look* complete. This is a documented boundary of
  blind planning, not a fixable bug; the RFC should record it as an accepted limitation.
- **The draft is silent on write governance.** It has the DE stage materializing tables into
  the customer's own warehouse as a matter of course, with no discussion of declared
  destinations, scoped credentials, or audit. This must be explicitly reconciled against
  D-017's split write posture before any of this ships — the draft simply assumes write
  access exists.

---

## 4. How it composes with a Bruin-style engine and the topic-pack semantic layer

**Bruin (evaluated separately in brief 10) — interface points only, not re-litigated here.**
The draft's "build" role (profile → compose from a whitelist → render a dbt model → validate
statically → execute → enforce contract-derived tests) is a pipeline-execution
responsibility a proven multi-warehouse engine could plausibly absorb, if brief 10 concludes
it fits. If adopted, Bruin would sit *inside* a build node — the same role the draft assigns
to "delegate to dbt" — not replace the orchestrator. The draft's own separation between the
orchestrator's dynamic, human-gated node graph and the SQL engine's static per-run DAG is a
useful frame either way, and should carry over even if the SQL-engine slot changes from dbt
to Bruin. Open question for brief 10: does Bruin natively support "validate a composed
transformation before touching the warehouse, then materialize, then run contract-derived
tests," or would Chartworks still need to own that composition/validation layer in front of
whatever engine it picks.

**Topic-pack semantic layer.** Directionally compatible but thinly connected. The draft's
gold rung is exactly where measures and dimensions get invented — the same content the topic
pack's context-card engineering (the crown jewel D-013 wants enhanced) ultimately serves —
and silver's generous, leaf-preserving conformation is the right shape to feed it. But the
draft treats `EnhancedTopicPack` as an external, fixed artifact it only writes into, and never
addresses how the pack's routing/context-card internals should read from, or feed signal
back to, the canonical registry or the gold verdict cache. That seam needs its own RFC design
pass; it is not something to inherit from this draft as-is.

---

## 5. Verdict

**Take:** the demand-driven, top-down-match / bottom-up-build scoping principle as the DE
stage's core cost model; the blind planner discipline (plan without catalog access so gaps
are detectable, not silently trimmed); the canonical-registry / resolve-then-compare pattern
as a candidate identity model for cross-rung, cross-time semantic matching; retrieve →
verify → confirm with a fail-closed, asymmetric reuse threshold as a general pattern for any
place an LLM proposes something a deterministic gate must approve (potentially informing
NLQ SQL-safety too, not just the DE stage); curated whitelist-only composition (an LLM
composes from vetted operations, never authors free SQL); contract-before-materialize with a
human gate scoped to irreducible/PII decisions; the dynamically-unfolding, content-addressed
execution-graph mental model (not necessarily its specific hand-rolled prototype); and
grow-by-addition / immutable versioning for engineered datasets.

**Ditch, or substantially rework before adopting:** the refresh/drift pipeline as specified —
it isn't actually specified, so treat drift detection as its own open phase-0 question rather
than something inherited here; the assumed dbt/SQLMesh/Cocoon/community-tooling stack as a
given — evaluate independently against the CGo-free posture (D-005) and whatever brief 10
concludes about Bruin; proceeding with DE-stage writes into customer warehouses without first
reconciling credential scoping, destination declaration, and audit against D-017's governed
write path; the topic-matching / registry-to-topic-pack integration as designed — admittedly
skipped and thin in v0, needing its own design against the actual `EnhancedTopicPack` shape;
and treating the full five-rung, two-phase-consolidation machinery as mandatory V1 scope —
worth validating the reuse-economics claim on a narrower slice before committing engineering
effort to the complete ladder in the same wave as the NLQ-core migration.
