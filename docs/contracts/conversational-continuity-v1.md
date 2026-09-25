# Conversational continuity v1

Status: implemented for review, 2026-09-22. Owns the deterministic runtime and Phase 24 fixture portions of EXP-01. Live calibrated comparison remains Phase 34/25 evidence.

A refinement is a child of one exact protected query in the same signed tenant, actor and session. The server reloads the parent under that scope, reauthorizes its exact topic, source, dataset and execution-context dependencies, and routes the child against current publications before generation. A query identifier, parent route or prior SQL never grants authority. The retained route request is the semantic base; raw prompts, SQL and rows are not conversation memory or log material.

The existing typed clarification answer delta owns correction and removal of reviewed clarification values. The existing interpretation delta owns replacement and removal of reviewed governed-value and temporal filters. This contract adds explicit reference and metric deltas for follow-up changes that additive request merging could not express safely. Reference edits add, replace or remove one exact typed semantic reference. Metric edits apply the same closed operations to one exact pinned metric ID. Remove and replace fail when their target is absent; add fails when already present; duplicate targets, unknown actions and invalid replacements fail before any provider work. An edit touching a measure/KPI or metric ID must leave the final measure/KPI reference IDs exactly equal to the pinned metric IDs; an unpaired replacement such as retaining one measure reference with another metric ID fails before routing or provider work. The service sorts references by full typed coordinate and metric IDs lexically before routing, digesting, context assembly and persistence, so request permutations produce one replay form.

Conversation ancestry is bounded to sixteen refinements. Every new child persists the exact observed parent revision and a digest of the protected parent record. PostgreSQL locks and rechecks that parent inside the child insert transaction; a concurrent parent mutation returns a conflict, and gateway work cannot commit stale lineage. Pre-migration children remain readable with null lineage evidence, while every newly created child requires the fence. The service traverses exact protected parent rows, rejects cycles and cross-context or cross-session ancestry, and returns `new_question_required` when the next child would exceed the limit. Starting a new question creates a new lineage that is routed against current semantics. A semantic republication also makes the old current-publication admission fail with `context_changed`; callers must start a newly routed question rather than silently applying the old route to the new publication. Immutable stale evidence remains available only through the separately governed retained replay behavior already owned by Phase 18.

HTTP, MCP, SDK and generated CLI retain the existing `query_refine_or_clarify` interaction role and one `refineNLQ` operation. The request schema adds only typed refinement fields, so installed-operation discovery and consumer dispatch remain compatible with the consumer-conformance contract. English and Spanish journey fixtures inspect the retained canonical selections and terminal classifications, not SQL string similarity.

The focused journey covers ask, add and remove a dimension, change a metric, filter replacement/removal through interpretation edits, clarification correction/removal through typed answers, semantic republication, and cross-session/cross-context denial. The Phase 24 fixture suite records bilingual EXP-01 cases with protected digests. These deterministic cases do not claim live model quality, an end-user comprehension result, or cross-system parity; those measurements remain in the Phase 34 cutover ledger and final Phase 25 qualification.


### Catalog-selected roots

New routes may retain `semantic_selection` version `catalog-selection-v1`.
Catalog-name/alias roots are carried as exact references into typed refinement.
A remove/replace also records optional `omitted_roots` so the old utterance does
not reintroduce the root; explicit add restores it. Shared-topic concepts retain
exact reference semantics rather than manufacturing ambiguous metric IDs.
Omissions remain in saved routing and clarification-origin comparison, grant no
access, and cannot switch off hard rules or constituent dependencies. Nil selection
uses legacy replay behavior; versioned selection is reconstructed from current
reviewed pins before clarification replay. This is not a migration of legacy
scalar/time conversation state, nor a SQL conformance proof.

### Retained model parameter custody

[Refinement parameters v1](refinement-parameters-v1.md) keeps an authorized parent's
model-authored bindings private during follow-up SQL generation. The edit packet
contains only previous unbound SQL and slot positions/kinds. PostgreSQL edits must
preserve complete parameter-bearing clauses and their source namespace; returned
placeholder values are replaced server-side before owned binding and validation.
Typed business-answer replacement remains separate from model slot mutation. The
exact observed parent and existing atomic lineage fence bind the ephemeral state;
public request schemas cannot supply it. This does not complete inferred filter/
time inheritance or introduce general free-text parameter-value editing.

### Explicit model-parameter replacements

[Parameter edits v1](refinement-parameter-edits-v1.md) accepts a bounded optional
`parameter_edits` list on the existing Refine operation. Each one-based model slot
can receive one same-kind private replacement; its native clause role is unchanged.
The parent is reauthorized and fenced before values are selected server-side.
Owned filters continue through their reviewed answer path, while child persistence
and replay use the existing protected parameter fields. This is typed value intent,
not arbitrary changes to the original question or inferred scalar/time inheritance.


## Retained inferred interpretation (S8)

The [interpretation continuity contract](interpretation-continuity-v1.md)
extends the existing router and Refine consumer with detached reviewed value IDs
and exact half-open calendar selections. Source/semantic admission and parent
lineage still precede inheritance. Fresh recognized dimension intent replaces its
old value; explicit value/time edits support replacement/removal. Replay and saved
routing retain anchors and selections. General grouping edits, wider language and
owner qualification are separate requirements. The canonical completion tracker
records actual tests; implementation text alone is not a passed integration gate.
