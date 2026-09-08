# Phase 18 — nlq-generation-execution

Status: in_progress. Owner: internal/nlq. Hard dependencies: 09, 10, 17.

The current bounded foundation supplies the first internal consumer for
`edit_base > hints > examples > default`: it carries evaluated mandatory
constraints and pinned metrics, counts the exact final serialized payload with
the phase-17 tokenizer, and stops clarify/no-route outcomes. Preflight, plan,
source execution, correction, templates, refinement, feedback, and learning
remain unimplemented; no phase acceptance criterion is claimed by this slice.
The bounded verification record is [phase 17/18 foundation evidence](../reviews/phase-17-18-foundation.md).

## Authority and design

RFC-001 §9, D-045/D-049 and [COMMON.md](COMMON.md) apply. This exploration orchestrator may generate/correct within limits; the reader and frozen reporting path never do so on their own.

## Brief findings incorporated

Briefs 02, 03, 08, 14: explicit template precedence, previous context/SQL, metric pins, session refinement, feedback, example lifecycle and durable learning.

## Findings I'm departing from

No disappearance of working replay/refinement/multi-topic/language behavior. Empty results cannot authorize broader filters. Query execution permission does not automatically expose protected raw SQL.

## Scope and implementation tasks

1. Implement preflight/plan/run/refine, native-dialect schema-constrained generation and one precedence function.
2. Carry explicit metric choices, previous authorized context/SQL and delta instructions; support confirmed same-source multi-topic queries and bounded self-curation without widening constraints.
3. Preserve templates/examples lifecycle, corrected feedback, DB-first weighting/deduplication and reviewed rule proposals.

## Non-goals

No new agent runtime, hidden SQL correction in the read core, automatic rule publication or source access through previous-session hints.

## Config and persistence

NLQ validation/execution correction ceilings, self-curation policy, template weight/similarity thresholds and learning/recency settings. Persist query/session provenance, protected SQL/evidence, examples/weights/feedback and bounded operation usage. One precedence function governs internal generation, template edits and guided follow-ups.

## Acceptance criteria

1. **AC01** — Question/preflight/plan/run/refine round trips preserve semantics, scope and session isolation across English/Spanish and multi-topic fixtures.
2. **AC02** — edit_base > hints > examples > default is deterministic; pinned choices and mandatory filters outrank retrieval fallback.
3. **AC03** — At most one validation correction and one execution correction occur within global budgets; every candidate is revalidated.
4. **AC04** — Zero-row self-curation never silently changes time ranges/permissions/required filters; unsafe semantic changes are proposed, not executed.
5. **AC05** — Feedback/corrected SQL and candidate/active/retired examples survive restart with DB-first weights, deduplication and provenance.
6. **AC06** — Public routes return structured confidence/assumptions/ambiguities/errors and hide SQL where inspection is unauthorized; current actor cannot refine a foreign session.

## Tests, coverage and smoke

Implement `TestPhase18/AC01` through `TestPhase18/AC06` with real semantic/source boundaries, recorded generator responses and failure/correction-budget cases. Preserve source behavioral fixtures using newly authored neutral tests. COMMON.md sets coverage; `scripts/smoke/phase-18.sh` requires all six results.

## Glossary, decisions and deviations

Refinement and self-curation remain explicit governed exploration. D-049 applies. No runtime completion is claimed.
