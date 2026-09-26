# Phase 18 — nlq-generation-execution

## SQL recovery status — 2026-09-25

The [AP-00–AP-08 completion tracker](../reviews/sql-recovery-completion.md) records the current
PR #62 implementation and qualification gaps. The recovery is **in progress**.
Existing shipped phase labels and historical defect-review results do not
close this subsequent extension. No required behavior is discarded by this
tracker correction; prior named acceptance criteria and historical evidence stay.

Current qualified runtime: `93539cc` (tree `193cf00`). S1 and S8 are complete
within their documented software scope. S8's Go 1.26.4/1.27.1 native suites passed;
S2–S7, S9–S12 and Q1–Q3 remain open. Exact counts and acceptance boundaries live
in the linked completion tracker. This mirror does not recertify historical code.


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
4. Derive an immutable dataset/column validation scope from each admitted reviewed topic and current source binding. Persist it with the query, compare it on later admission, and use `ValidateWithin` for generation, replay, feedback and repair; a prompt relation name does not grant SQL dependencies.

## Non-goals

No new agent runtime, hidden SQL correction in the read core, automatic rule publication or source access through previous-session hints.

## Config and persistence

NLQ validation/execution correction ceilings, self-curation policy, template weight/similarity thresholds and learning/recency settings. Persist query/session provenance, immutable reviewed relation scope, protected SQL/evidence, examples/weights/feedback and bounded operation usage. Existing queries without a pinned relation scope fail closed on revalidation. One precedence function governs internal generation, template edits and guided follow-ups.

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

Refinement and self-curation remain explicit governed exploration. D-049 applies. This original design statement is historical; the shipped baseline and delivered
consumer extensions below have their own evidence. Full SQL recovery completion
is governed by the current tracker linked above.


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
MCP, SDK and the generated CLI operation surface. Migration 042 preserves legacy
rows as inapplicable candidates, and portable import replay cannot amplify evidence.
See [learning lifecycle v1](../contracts/learning-lifecycle-v1.md).

## EXP-01 bounded follow-up lineage

Refinement ancestry is bounded to sixteen children and traversed through the
protected repository under the current tenant, actor and session. Cross-session
or cross-context ancestry fails closed; a cycle or exhausted bound returns the
typed `new_question_required` outcome. A changed semantic publication returns
`context_changed`, requiring a fresh routed question instead of carrying the old
route into a new publication. Measure/KPI references and metric IDs remain paired,
and full typed reference coordinates plus metric IDs are sorted before routing.
Canonical child requests persist the resulting semantic selections and exact
observed parent revision/digest; the child insert locks and rechecks the parent so
concurrent mutation fails closed. Prior SQL remains protected edit context only. See
[conversational continuity v1](../contracts/conversational-continuity-v1.md).


## SQL recovery: strategy-aware generation packet

The [generation packet v2](../contracts/generation-packet-v2.md) initial AP-02
slice keeps complete reviewed relation scope for validation and persistence while
rendering selected single-topic metric dependency columns for the model. Required
semantics remain atomic. Edits/hints suppress typed examples and skip irrelevant
learning work; included demonstrations preserve declared rank and have actual-use
evidence. Optional context yields before required instructions. Runtime system
additions are byte-checked again before dispatch. Effective model-specific provider-envelope fitting is implemented by AP-02B
below. Operator configuration and live quality qualification remain open; frozen
refresh and native authority/SQL validation are unchanged.

## SQL recovery: effective-envelope refitting

AP-02B extends [generation packet v2](../contracts/generation-packet-v2.md).
The context owner prunes optional groups against the adapter's effective model,
system/schema/JSON framing and output reserve. Exact operator model-window entries
are enforced for runtime overrides; no configured window means explicitly unknown
capacity with byte and operation bounds still enforced. Required semantics and
full reviewed relation scope remain unchanged, and repair resolves its own role.
Actual packet-use and estimated per-attempt envelope evidence are distinct from
reported tokens and live qualification. Retained objects are not reinterpreted.

## SQL recovery AP-03A — scoped analytical metrics

[Analytical metrics v1](../contracts/analytical-metrics-v1.md) adds catalog-derived
selected aggregate/population contracts after native safety and before execution,
using the same one-correction budget and private scalar ownership. Versioned query
proofs reconstruct against exact current/retained publications and remain distinct
from business approval. Legacy plans stay explicitly unmeasured. New unsupported
shapes fail explicitly; join/grain/full-question conformance and wider native
qualification remain open in the SQL recovery program. Real PostgreSQL acceptance
is `TestSQLRecoveryAnalyticalAcceptance` and
`TestSQLRecoveryAnalyticalOwnedScalarRepair`; the PR records actually executed results.


## AP-03B1 direct grouping conformance

[Analytical grouping v2](../contracts/analytical-grain-v2.md) compiles a complete
reviewed-dimension suffix into a source-bound grouping contract. The existing
validator-first checker rejects missing, substituted and invisible extra grouping
keys without promoting filter mentions or required dependencies into user choices.
Migration 054 preserves the distinct v1 policy for retained replay, while v2 receipts
state whether grain is measured. Native/metric checks, private owned predicates,
one-correction budgets and frozen-refresh behavior are unchanged. Broader natural
language, temporal grain, query-wide population and physical join proof remain open.
Exact runtime evidence is recorded in PR #62, not inferred from this task inventory.


## AP-03B2 reviewed calendar partitions

The [calendar grain contract](../contracts/analytical-calendar-v3.md) extends
new authoring with explicit reviewed Gregorian day/month/quarter/year buckets
while retaining original v1/v2 replay. Exact native temporal typing and explicit
instant timezone are required; the complete grouping/output partition set is
checked after native validation. This is not query-wide population, join or
ordering conformance. Final runtime qualification is recorded in PR #62; code
presence alone is not a passing acceptance result.


## AP-03C1 owned query-predicate conformance

The [v4 population contract](../contracts/analytical-query-population-v4.md)
checks complete WHERE/HAVING predicate provenance against authenticated typed
constraints, retaining common metric filters and all existing native/metric/grain
checks. Private values do not enter provider guidance or public receipts. Record
v4 is forward-only; older policy replay remains distinct. Query-wide natural
language completeness, ordering, joins and other dialects remain unqualified.
Tests and actual qualification are recorded in PR #62, not asserted by this plan.

## AP-02C topic-scoped physical context

The [generation packet v2 extension](../contracts/generation-packet-v2.md#ap-02c-topic-scoped-dimension-and-multi-topic-rendering)
adds independently selected dimension/column and per-topic physical rendering from
existing mandatory selection/dependency identities. Full reviewed relation scope
remains unchanged in admission, persistence, native validation, saved reads and
replay. Shared datasets retain each topic's complete reviewed columns to protect
independently confirmed join endpoints. Opaque, unselected and filter-only topics
keep their original rendering. Missing, conflicting or cross-topic physical
mappings fail explicitly. This is not minimal join-aware projection, fan-out proof
or wider analytical SQL support. See the [AP-02C review](../reviews/sql-scoped-projection.md).

`TestSQLRecoveryScopedProjectionAcceptance` and
`TestSQLRecoveryScopedProjectionPrivateAcceptance` extend AC01/AC02/AC06 with real
PostgreSQL, recorded provider wire, EN/ES dimension-only generation, private-filter
grounding, exact source records, durable scope and zero-work terminal replay.
Scoped assembler and actual selection-producer tests cover shared join keys,
foreign mappings, filter-only fallback, refitting and unrelated schema growth.
No new migration, action, public field or analytical policy version is introduced.
Exact passing/failing runtime evidence is recorded in PR #62; this source checkpoint
alone is not release qualification.

## AP-06A accepted generation explanations

[Generation explanations v1](../contracts/generation-explanations-v1.md) retains
known-value-redacted assumptions and ambiguities from the accepted candidate in
existing query fields. Fresh Plan, Run, saved views and replay consume one durable
value. Failed repairs cannot replace notes; accepted execution corrections and
refined children describe their own candidates. Preflight and opaque idempotent
Plan lookup retain their existing boundaries. Historical rows are not backfilled.

The three `TestSQLRecoveryExplanation*Acceptance` tests extend AC01/AC03/AC06
through actual PostgreSQL/native boundaries and recorded provider responses,
including EN/ES, restart, private answer replacement and zero-work terminal replay.
No new permission, model role, migration or analytical proof is introduced. Model
text is advisory: this does not complete the AP-08 ambiguity gate, AP-05 analytical
continuity or AP-06 parameterized learning. Exact CI outcomes remain in PR #62.

## AP-05A parameter-preserving refinements

The [parameter custody contract](../contracts/refinement-parameters-v1.md) extends
AC01/AC03 with authorized parent binding retention. Exact private model values are
not sent to the generator, complete parameter-bearing clauses retain their binding
roles, and returned placeholders are restored before native/analytical checks and
owned predicate rebinding. Restart/replay use existing durable fields and the parent
revision/digest fence. No new model role, migration or public request field is added.
The dedicated `TestSQLRecoveryParameterContinuationAcceptance` and
`TestSQLRecoveryParameterContinuationOwnedAnswerAcceptance` exercise actual
PostgreSQL/recorded-provider boundaries. Full language/time inheritance and explicit
parameter editing remain separate obligations; test inventory is not runtime proof.

## AP-05B explicit parameter replacements

[Refinement parameter edits](../contracts/refinement-parameter-edits-v1.md) add a
closed optional Refine input for one-based same-kind model-slot replacements.
Native parameter-role proof, signed resource reach, parent lineage CAS,
owned-predicate separation and single-correction budgets are unchanged. New values
stay outside generated context; only the accepted child binds them durably. Empty
or malformed edits cannot invent slots, and changed free-text remains separately
routed. Existing public dispatch derives the field from the shared request DTO.

`TestSQLRecoveryParameterEditLifecycleAcceptance`,
`TestSQLRecoveryParameterEditBoundariesAcceptance` and
`TestSQLRecoveryParameterEditOwnedAnswerAcceptance` extend AC01/AC03/AC06 through
real PostgreSQL/native and recorded-provider boundaries, not live language quality.
Existing no-edit, analytical, replay and explanation regressions remain required.
Exact CI outcomes are recorded in PR #62.

## AP-06B typed learned-example checkpoint

[Parameterized examples v1](../contracts/parameterized-examples-v1.md) carries
value-free slot metadata from validated feedback through candidate persistence,
current-source native review, protected portable import/export and the existing
example prompt lane. Fixed public dry-validation probes never become source-query
bindings or demonstrated defaults. New queries supply and validate their own
parameters. The immutable schema is additive in migration 057; legacy rows stay
unchanged. Unpublishable bound annotations/predicates still allow feedback recording.

The new `TestSQLRecoveryParameterizedExample*Acceptance` and
`TestSQLRecoveryLearningDoesNotCopyOwnedOrAnnotatedPredicates` tests extend AC03/AC06
without changing signed reach, native authority, retry limits or frozen refresh.
This checkpoint is not qualified solely from added tests: exact executed local and
native/CI outcomes are recorded in the delivery evidence. Per-dialect and protected
owner qualification remain separate.

## AP-07A shared native-name vocabulary

[Read-SQL vocabulary v1](../contracts/sql-vocabulary-v1.md) uses one immutable
registry for validator function/type/operator/value name gates and server-owned
SQL-generation/correction guidance. The underlying accepted names and native safety
rules are unchanged. Both model roles receive the admitted dialect's exact snapshot
before complete-envelope fitting; no extra role/call or permission is introduced.
Unknown dialects fail rather than borrow another engine's profile. Retained SQL and
analytical versions are not rewritten. Names are necessary but not sufficient;
source scope, typed arguments, full-tree safety and analytical proof still apply.

`TestSQLRecoveryVocabularyProviderAcceptance` and
`TestSQLRecoveryVocabularyKeepsStrongerProofs` extend real PostgreSQL/recorded-wire
coverage with EN/ES, bounded invalid-function correction, private filters, exact
results and zero-work replay. Registry/native/service tests enforce the original
lists and namespace/parameter conventions. The PR records executed verification;
full per-engine and live analytical qualification remains open.

## AP-08A model-reported readiness

[Generation decisions v1](../contracts/generation-decisions-v1.md) requires a closed
ready/clarify/insufficient-context response from each fresh SQL generation/repair.
Only ready candidates reach the existing validators. Blocked decisions carry no
SQL/bindings, are not automatic repair input, and cannot publish or execute a plan.
HTTP/MCP/SDK use distinct bounded, known-value-redacted problem metadata; this is
not a deterministic reviewed AnswerContext or a model-generated authority token.
Legacy stored notes and frozen replay are not reinterpreted. The original route,
private bindings, one-correction limit and native/analytical checks remain.

The two `TestSQLRecoveryDecision*Acceptance` tests exercise actual PostgreSQL
state/attempt boundaries, recorded-provider readiness, EN/ES, composed PlanAndRun,
refinement, HTTP/MCP privacy and current authority. Existing ready fixtures declare
the new output contract explicitly. Runtime results and remaining pending-outcome
conversation/live-quality qualification belong in the PR tracker.

## Adversarial review — analytical and terminal operation hardening

The [P0/P1 audit](../reviews/sql-adversarial-p0-p1.md) adds independent baseline-red
regressions for parent-only scans, unknown/bigint numeric literal typing and
terminal correction-context failures. Current-source acceptance uses exact native
PostgreSQL results and requires failed analytical proposals to remain non-executable.
The existing one-correction, immutable proof and private-value boundaries remain.
No migration, vocabulary expansion or legacy result reinterpretation is introduced.
The PR ledger records exact executed results before closing each finding.

The adversarial correction gate also recognizes the real executor's durable
failed/query_error receipt (nil transport error) only when stopped and terminal
with no result. Unknown/unconfirmed outcomes cannot retry. Native validation
already rejects ONLY, so its analytical hardening is defense-in-depth; this is
not counted as a demonstrated native execution bypass.


## Adversarial query-finalization and result-custody correction

The [P0/P1 review](../reviews/sql-adversarial-p0-p1.md) covers the source-error
sanitizer, incomplete receipts, response cancellation and failed reruns. Logical
query status is terminal even when physical evidence is withheld; uncertain work
is not retried or claimed stopped. Bounded metadata-only cleanup persists completed
outcomes after caller cancellation. Failed operations clear prior rows rather than
returning stale success. Native ledgers, scoped authority, immutable analytical
proofs and the one-correction policy remain independent and unchanged.

Required regression tests include real PostgreSQL failure/cancellation edges and
separate exact-baseline assertion failures. Unknown or unavailable metadata still
cannot be claimed successfully persisted. The PR records actual current-head gates.


## Retained inferred interpretation (S8)

The [interpretation continuity contract](../contracts/interpretation-continuity-v1.md)
extends the existing router and Refine consumer with detached reviewed value IDs
and exact half-open calendar selections. Source/semantic admission and parent
lineage still precede inheritance. Fresh recognized dimension intent replaces its
old value; explicit value/time edits support replacement/removal. Replay and saved
routing retain anchors and selections. General grouping edits, wider language and
owner qualification are separate requirements. The canonical completion tracker
records actual tests; implementation text alone is not a passed integration gate.
