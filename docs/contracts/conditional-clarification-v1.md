# Conditional clarification and typed answers v1

Status: CW-01 implementation contract submitted in PR #23. Exact-head execution
and the bounded review record determine readiness; this document is not test
execution evidence. It extends phases 16–18 without changing identity ownership,
immutable publication, source partitions, or validator-issued read proof.

## Reviewed activation and outcomes

A clarification policy is optional versioned data on a retained pattern. Version 1
uses only reviewed literal token phrases and exact semantic-reference conditions.
The conditions are a closed OR, not executable expressions, regular expressions,
or model-generated permission decisions. The topic and ruleset must already be
admitted under current signed reach. Explicit reference selections can satisfy a
reference-choice slot without asking again. Free-form question rewriting is not
an answer's semantic effect.

The evaluator distinguishes `not_applicable`, `satisfied`, `missing`, `invalid`,
and `conflicting`. An unrelated question has not-applicable slots and continues
without answering them. A submitted value for an inapplicable or foreign field is
invalid, not a hidden filter. Invalid or conflicting input returns no partially
accepted resolution group. Unresolved governed text returns a missing outcome and
an explicit `unresolved_value` field error.

Runtime ordering is independent of persistence order: blockers, specificity,
author priority, stable pattern ID, and stable slot order under declared
prerequisites. A dependency graph orders questions; only ready blockers are
presented. Independent blockers can be grouped with the explicit explanation that
each resolves a separate reviewed constraint. Conflicting effects on the same
semantic or physical target are evaluated as a group, including incompatible
units/time policies and jointly empty scalar intersections. Priority never makes
one incompatible mandatory policy win.

## Answer-dependent applicability (AP-01 / S1)

The evaluator computes a least positive fixed point from the admitted initial
facts and question. A successfully parsed reference choice, reviewed default or
typed effect target in an already active pattern may activate another reviewed
pattern. It cannot activate a disabled pattern or bootstrap an otherwise inactive
cycle using supplied answers. An answer that remains inactive after convergence
is invalid. Active invalid values and incompatible effects remain atomic failures;
no partial resolutions or executable predicates escape those outcomes.

Applicability facts remain separate from the original selected roots. Resolving
one choice does not automatically answer another independent choice. Local slot
prerequisites, reviewed defaults and stable ordering retain their original rules.
The closure is limited to 128 facts; every changing pass adds a fact. It accepts
simultaneous parent/dependent answers as well as separate preflight submissions.
No new model call, authored matcher language or publication rewrite is introduced.

Preflight computes a non-retained over-approximation of reachable reference-choice
branches solely to decide whether to read the current source binding. A reachable
future typed predicate is source-pinned before issuing its answer context. This
hint never adds a selected reference, resolution, constraint or execution grant;
JSON cannot supply it. Unrelated policies and unseeded cycles do not trigger it.
Source rotation/revision and current authority checks remain enforced on answers.

Canonical pending and accepted state replays through this same evaluator. A
historical receipt that omitted a newly reachable requirement must be replanned;
replay does not silently add that requirement or certify an old incomplete plan.
Changing a controlling answer requires explicit removal of now-inactive retained
answers; the unbound base is regenerated and old owned predicates cannot survive.
Synthetic and real-source coverage is in `clarification_fixedpoint_test.go` and
`sql_clarification_fixedpoint_test.go`; actual executed evidence is tracked in the
[completion tracker](../reviews/sql-recovery-completion.md).

## Declared effects and exact values

Reference choices use only `option_id`, mapped to an exact reviewed semantic
reference. A display label is never a reference or warehouse identifier. All other
inputs use the typed `value` union and a declared effect:

| Family | Canonical effect |
|---|---|
| Time window | Exact local and UTC bounds, Gregorian calendar, named IANA timezone, temporal target/type, reviewed grain and half-open `[)` boundaries. |
| Number | Exact decimal or range strings, reviewed unit, precision, scale, comparison operator, null semantics and interval inclusivity. No floating-point intermediate or silent rounding. |
| Boolean | Locale-neutral true/false bound to the reviewed predicate target. Unknown spellings require repair rather than truthiness conversion. |
| Entity or bounded text | A reviewed spelling maps to one governed canonical value and an exact target/operator/null policy. Unmapped text does not become SQL or implied filtering. |

English/Spanish number and boolean inputs and named month periods normalize to
the same locale-neutral values. Explicit dates use `YYYY-MM-DD`. Calendar,
timezone and grain are required, not inferred from a display label or server
locale. Reviewed grains are day, week, month, quarter and year; both boundaries
must align. Time windows are half-open. Missing or ambiguous local-midnight
boundaries fail explicitly rather than guessing a daylight-saving offset. General
natural-language period interpretation and arbitrary calendars are not advertised.

Null behavior is explicit: exclude nulls, include nulls alongside the comparison,
or a null-only predicate. Numeric comparisons use exact declared units; unit
conversion is not inferred. Approximate source numeric types are not accepted for
an exact threshold. Adapter precision/type limits fail before model work.

Every required field must be answered. Required defaults are rejected at
compilation. Optional defaults are reviewed, visible as defaulted, and retained
with `reviewed_default` provenance. They are reevaluated on replay rather than
being converted into user answers. Explicit removal cannot skip a required field.

## Propagation, budgets and privacy

`QuestionRequest` and the routing request carry typed answers with topic,
topic-version, ruleset-version, pattern/version and slot pins. Resolution records
add the parser version, locale, canonical value/effect, provenance, exact digests
and question digest. The route seals the resolution set and live source-binding
identity. These records are business evidence, never bearer authority or an
executable plan.

Before embeddings, reranking or generation, the existing tokenizer counts the
whole mandatory group together with pinned metrics and existing hard rules. The
1500/3000/6500 tiers remain one token currency. A group that does not fit produces
typed insufficiency; no selected answer is dropped to make space. Provider context
contains reviewed effect metadata, not scalar values or governed dictionaries.
Classification uses the reviewed slot's sensitivity before persistence. Known
sensitive submitted/canonical spellings are redacted from questions and
instructions before storage or provider submission. This is not a general PII
classifier for arbitrary unrelated text.

Ordinary logs contain identifiers, reason codes and bounded outcome evidence, not
raw answers, SQL, result rows or prompts. Direct typed-answer, resolution and
business-constraint structured log attributes are redacted. Arbitrary serialized
wire DTOs or containers are not an ordinary logging interface. Authorized API JSON
and protected domain storage intentionally retain the canonical data needed for
binding and replay.

After generation, the service resolves exact semantic targets through the reviewed
column graph and current source metadata. It binds values as parameters, preserving
existing predicates by conjunction. Row targets become row predicates; aggregate
measure thresholds become reviewed aggregate/HAVING predicates, not filters on
individual measure input rows. The binding receipt identifies exact parameter
positions, statement/parameter digest, source identity and resolution IDs without
scalar values. The ordinary validator must then issue a nonzero read plan. Neither
a parsed statement nor a clarification digest substitutes for that proof.

The current binding transformer accepts bounded single SELECT statements over
qualified base relations, including joins, WHERE, grouping, HAVING, ordering and
limits. Ambiguous self-join targets, derived/nested SELECTs, CTEs, set operations
and unsupported dialect shapes return explicit unsupported outcomes. No constraint
is silently omitted to accept a broader SQL shape. Queries with no scalar business
constraints retain their existing validator behavior. Six dialect parameter/quoting
paths have synthetic transformation tests; this assignment's real warehouse
acceptance uses PostgreSQL and does not newly qualify live cloud engines.

## Sessions, correction and current reach

Submitting answers through preflight/planning requires the retained originating
`clarification_query` and `answer_context`. The service verifies owner, session,
question, topic set, selected references, metrics, joins and context before provider
work. A query ID or answer-context digest grants no access. New requests supply a
fresh bearer; no bearer is saved for replay.

Refinement rereads the parent under current authority and reevaluates its current
publication/source pins. Changed relevant state returns a stale/reevaluate error,
not an automatic adoption of a new meaning. Answer deltas replace or remove by
exact topic/pattern/slot. The child retains value-free supersession/removal lineage;
the parent's historical evidence remains immutable. Stored unbound base SQL is
separate from service-bound predicates, and answer edits do not inherit old bound
filters as generation instructions.

Before execution, the service reevaluates canonical resolutions and reconstructs
the binding from the protected base. It compares exact SQL, parameters and receipt,
then uses fresh ordinary validation and execution. Validation correction reapplies
the complete constraint group. Execution correction cannot alter bound SQL or
parameters to widen an answer. Cancellation returns no partial bound query.

## Authoring, portability and consumers

The existing topic API/SDK provides:

- `POST /v1/topics/{topic}/clarifications/preview` / `PreviewClarifications`;
- `POST /v1/topics/{topic}/clarifications/export` / `ExportClarifications`;
- `POST /v1/topics/{topic}/clarifications/import-preview` / `PreviewClarificationImport`.

Preview accepts bounded synthetic matching/nonmatching/conflicting cases, reports
human-readable conditions/effects and typed outcomes, and does not publish or call
a provider. Saving, review, publication and retirement reuse the existing ruleset
lifecycle and signed topic/dependency reaches. Retained replay and shadow reuse
the same deterministic evaluator and persist immutable comparison results. Review
is required before activation; no standalone authoring application is introduced.

Execution uses the existing preflight/plan/run/refine HTTP, MCP and SDK consumers.
Localized field errors expose field, code and repair message. Questions expose
concise prompts, why they matter, readable reviewed choices, prerequisites and
visible defaults. Client DTOs preserve canonical typed unions and exact decimal
strings rather than converting non-reference values into choice IDs.

Portable version 1 retains exact rule/topic digests and explicit per-pattern
migration dispositions. Import is a preview/proposal until normal reviewed
publication. Exact-topic roundtrips are supported; foreign topic/digest mapping
requires an explicit new semantic draft and review, never display-name matching.
Legacy imports require `preserve_reference_only`. Retained patterns without a
policy remain digest-stable explicit reference choices only; they never acquire
new automatic blocking. Legacy scalar values require a reviewed typed rewrite.
Unknown executable matcher fields are rejected by the closed API decoder.

Migrations 036 and 037 append protected NLQ clarification evidence and retained
comparison result columns after the shipped prefix. Existing migrations and
published definitions are not rewritten. Acceptance upgrades from the shipped
schema, verifies its checksums and retained tenant data, then checks the new
columns through the normal migration runner.

## Evidence and explicit limitations

`TestCW01/AC01` through `AC10` cover the software corpus together with the phase
16–18 regressions, parser/binding unit and fuzz cases, race tests, authoring and
actual HTTP/MCP/SDK journeys. Recorded provider responses prove deterministic
integration, not live model accuracy. SQL-shape and calendar limitations above are
typed boundaries, not fallback permission to ignore answers. Representative-user
completion, correction time and comprehension testing (`CLAR-AC11`) was not
performed and remains separate usability research. This change does not expand
routing calibration, rich topic generation, chart selection or reporting policies.


## Reviewed scalar boundary corrections

Timezone is an explicit reviewed IANA location or `UTC`; empty and `Local` runtime
defaults are rejected by authoring, value resolution and business binding. A host's
local timezone is never a substitute for a publication pin. Native timestamp
identity remains distinct: instant-bearing `TIMESTAMP` is not a wall-clock target
where the adapter defines it as an instant, and wall-clock bindings use the native
non-timezone cast rather than a session-dependent cast.

Exact numeric admission checks both fractional and integral digit capacity. The
BigQuery binding selects `NUMERIC` only when the declared scale is at most 9 and
integral capacity at most 29; otherwise a supported declaration uses `BIGNUMERIC`,
with at most 38 integral and 38 fractional digits. A declaration outside the fully
representable domain fails before provider work, even when one particular answer
is small. No rounding, floating-point conversion or partially representable extra
digit is used to make a declaration appear supported.

These corrections are covered by the native scalar-boundary regression tests in
`internal/exec/business_temporal_test.go`, `business_timezone_test.go`, and
`business_precision_test.go`, plus
`internal/semantics/clarification_timezone_test.go`. They do not expand the SQL
shape subset or qualify a live cloud deployment.
