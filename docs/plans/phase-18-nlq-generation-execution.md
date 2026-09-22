# Phase 18 — nlq-generation-execution

Status: shipped. Owner: internal/nlq. Hard dependencies: 09, 10, 17. Current cumulative evidence: [phases 15–18 and 21](../reviews/phase-15-18-current-evidence.md).

Proposed PR #11 delivery; this status becomes effective after every required hosted
check passes and the PR merges.

The integrated core now supplies a durable first consumer for
`edit_base > hints > examples > default`: it admits the sealed phase-17
context, persists sessions and protected query evidence, generates and
revalidates one opaque read plan through the existing validator, executes it
through the existing executor, and records bounded correction, refinement,
feedback, and learning state. PostgreSQL query reads consume the phase-16 rule
publish/retire invalidation ledger. An invalidated query keeps its immutable
topic/version and rule pins and replays through the retained topic reader rather
than silently using the current publication. The current core evidence is in
[phase 18 NLQ runtime evidence](../reviews/phase-18-nlq-runtime.md).

The public HTTP/SDK operation surface and real invalidation consumer are integrated,
the strict six-child check passed, and the two complete review rounds are clear. PR
#11 records this phase as shipped conditionally; that status becomes effective after
every required hosted check passes and the PR merges. The bounded corrections preserve the
existing Bruin validator/executor path: every dialect retains exact SQL, bound
parameters, plan coordinates and source/context revision; PostgreSQL may also
accept an AST-equivalent statement when only locations or formatting differ.
Any other SQL change returns `ErrUnsafeCorrection` without a second execution,
and every attempt records a receipt. Hosted CI and release gates still belong to
the integration head. This plan does not claim live provider quality.

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
6. **AC06** — Public routes return structured confidence/assumptions/ambiguities/errors and hide SQL where inspection is unauthorized; current actor cannot refine a foreign session. The core lifecycle also proves activation after an unruled query, publish/retire invalidation, ordered multi-topic pins, and retained stale replay through the real PostgreSQL/native read seams.

## Tests, coverage and smoke

Implement `TestPhase18/AC01` through `TestPhase18/AC06` with real semantic/source boundaries, recorded generator responses, failure/correction-budget cases, and the rule-evidence invalidation/replay regression nested under AC06. Preserve source behavioral fixtures using newly authored neutral tests. COMMON.md sets coverage; `scripts/smoke/phase-18.sh` requires all six results.

## Glossary, decisions and deviations

Refinement and self-curation remain explicit governed exploration. D-049 applies. No runtime completion is claimed.


## CW-01 delivered clarification extension

The [conditional clarification contract](../contracts/conditional-clarification-v1.md)
extends this phase's existing core, without changing authority ownership, immutable
publication, source partitions or validated-read prerequisites. It supplies reviewed
conditional applicability and typed answer resolution, mandatory-group token/privacy
handling, source-bound parameter effects, session correction/removal and retained
consumer replay. Preview, replay/shadow and safe import dispositions use the existing
API/SDK surfaces rather than a new authoring application.

`TestCW01/AC01` through `TestCW01/AC10` add the clarification acceptance corpus;
the six existing `TestPhase16`, `TestPhase17` and `TestPhase18` criteria remain
unchanged and continue to run. [CW-01 delivery evidence](../reviews/cw-01-delivery-review.md)
records exact executed checks and review corrections. No CLAR-AC11 comprehension
study, live cloud/model quality measurement or general SQL-shape expansion is
claimed by these software tests.

## CW-07 interpreted execution constraints

Governed values and temporal spans now reach generation as mandatory typed context
and reach execution only as sealed `BusinessConstraint` values over exact reviewed
dataset/column/source revisions. Immediate planning binds the in-process seal;
retained run/refinement reconstructs it through current authorized publications and
source bindings, requires the protected base candidate and exact binding receipt,
and fails on publication/parser/vocabulary/source drift before ordinary or terminal
execution. Refinement inherits the anchor and replaces
same-target correction/removal edits. Interpretation still cannot issue a plan or
bypass the existing validator, native source planning or read-only execution.

## CW-08 reviewed learning lifecycle

Feedback storage and its example effect are one transaction. Exact retries and
same-outcome retries are no-ops; distinct corrected outcomes remain explicit.
Positive and negative counts produce a bounded beta posterior and uncertainty,
replacing fixed increments. Promotion requires a current exact origin, sufficient
positive evidence, a reviewer, a review note and CAS version. Generation admits only
active current rows and persists exact selection/exclusion provenance before SQL
work. Retirement is the rollback path and cannot rewrite frozen queries.

Protected export and revalidated candidate-only import are registered through HTTP,
MCP, SDK and the generated CLI operation surface. Migration 041 preserves legacy
rows as inapplicable candidates, and portable import replay cannot amplify evidence.
See [learning lifecycle v1](../contracts/learning-lifecycle-v1.md).
