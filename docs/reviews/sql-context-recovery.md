# SQL context recovery — phased implementation ledger

Baseline: `673d74b281c4f800bfa86ef7415443d99209ba24`. Work is tracked in PR #62.
This is a recovery extension of phases 16–18 and 24, not a new release gate.
Implementation, passing synthetic fixtures, native execution, and live owner
qualification are separate evidence states. Nothing here declares full parity.

## Current completion status

The [completion tracker](sql-recovery-completion.md) controls the current
AP-00–AP-08 status and the finite S1–S12/Q1–Q3 backlog. All phases have delivered
increments, but the overall recovery remains **in progress**. The last qualified
baseline is `ea38c07`; the P0/P1 review is not full implementation/parity closure.

## Phase status

| Phase | Delivered implementation | Remaining implementation | Qualification still required |
|---|---|---|---|
| AP-00 | Exact source snapshots; engine and effective provider-wire capture; EN/ES synthetic fixtures | Paired cohort execution/reporting as needed by the existing evaluation runtime | Same-snapshot original/replacement owner cohort with frozen expected results; opt-in live diagnostics |
| AP-01 | Catalog-backed atomic dependencies; nested KPI closure; selected roots shared with rules and clarification; provenance, omissions and replay | S1 implemented in this checkpoint, integration qualification pending; S2: reviewed, bounded paraphrase selection | Held-out selection/ambiguity calibration, not similarity-as-confidence |
| AP-02 | Metric/dimension/multi-topic prompt projection; ranked examples and actual-use evidence; strategy and effective-envelope fitting | S3: minimal confirmed-join-aware projection | Operator tokenizer/model-window/framing qualification |
| AP-03 | Native-first selected metrics, exact arithmetic, independent populations, direct/calendar grain, typed WHERE/HAVING and versioned replay | S4: reviewed intent for order/limit/filter and broader expressions; S5: physically backed join/fan-out proof and required nested/multi-relation forms; S6: required dialect proof | Business-result cohorts and actual required engines; safe rejection is not support |
| AP-04 | Targeted unbound SQL repair, private binding restoration, bounded attempts, terminal exclusions and actual durable source-error path | S7: additional safe diagnostic/correction classes where acceptance requires them | Broader privacy, repair/result and engine matrix within existing attempt limits |
| AP-05 | Protected parent lineage; typed reference/metric edits; model-slot custody; explicit same-kind parameter replacement; public SDK | S8: inferred scalar/time inheritance and explicit replacement/removal; S9: grouping changes and broader native role proof | Multi-turn EN/ES business journeys, restart/replay and language quality |
| AP-06 | Durable accepted redacted explanations; value-free typed learned examples through review, persistence, import/export and generation | S10: reviewed service-owned predicate learning and additional required parameter domains | Parameter-domain/probe and dialect matrix; owner result quality |
| AP-07 | One generator/validator name vocabulary across six dialect profiles | S11: proven syntax/function signature/type matrix shared at generation and validation boundaries | Actual required engines plus selected analytical cases |
| AP-08 | Strict ready/clarify/insufficient-context outcomes; blocks cannot yield an executable plan; bounded redacted questions and HTTP/MCP/SDK parity | S12: durable pending-question/resumption and richer reviewed choices | Calibrated ambiguity detection, paired result quality and release evidence |

## Historical implementation checkpoints

The sections below describe their own checkpoint dates and scopes. Statements
that an increment was pending are historical; the current table and completion
tracker above supersede them. Their tests/results remain evidence only for the
stated source, not automatic evidence for later code.

## First repair checkpoint (SQL-09, SQL-13, SQL-15)

The failed model-authored statement is retained separately from service-bound
SQL. A typed internal repair packet includes that unbound statement, positional
parameter kinds, original selected instructions, and a closed diagnostic code.
It does not contain parameter values, upstream error text, generated explanations
or service-owned predicate values. The original sealed context and its token
counter fit the packet before the one permitted correction call.

The model must preserve existing parameter positions and kinds. Returned scalar
placeholders are discarded; the service restores the original private bindings,
reapplies owned business constraints and performs ordinary native validation.
Unknown/transport, authority, stale binding, cancellation, timeout and uncertain
failures do not spend correction calls. Execution repair's AST-equivalence and
exact-binding policy is unchanged. A repair packet too large for current bounds
returns explicit insufficiency before a second model attempt.

This does not yet prove aggregate/population/join equivalence; AP-03 owns that
analytical check. It does not change frozen report refresh, public schemas,
stored query formats, action scopes or provider credentials. AP-02 still owns
full provider-envelope budgeting and optional-lane pruning.

## Synthetic verification

`go test -race -count=1 ./internal/nlqexec -run '^TestSQLRecovery'`

The spy records the exact core `Engine.Generate` role, system, prompt and schema
in memory. Existing Bifrost recorded-wire tests own SDK serialization. These are
synthetic fixtures, not live provider measurements. Regressions assert failed
candidate inclusion, private-value exclusion, restored/nonaliased parameter
bindings, mandatory-context retention, pre-call overflow, terminal-error
classification and a maximum of one validation correction.

## Protected owner-comparison protocol (AP-00 remainder)

Use the existing evaluation runtime and protected input resolver. Pin identical
reviewed semantic definitions, physical schema/source revision, context, locale,
question, temporal anchor and typed selections. Record implementation, model,
role configuration, runtime prompt, tokenizer and schema identities separately.
Store private input and any full request capture only through an explicitly
approved protected diagnostic path; ordinary telemetry retains content-free
hashes, counts and typed outcomes. No private source archive, credentials, prompt
or owner rows may enter repository fixtures or CI artifacts.

Compare root/dependency completeness, first-pass validity, expected result and
analytical semantics, clarification necessity, repair correctness, follow-up
state, parameter ownership and actual example usage. Separate service/source/
model duration, calls and available cost; unknown values remain unknown. Freeze
owner-approved expected outcomes before scoring. Recorded fixtures cannot claim
live success, and a changed source/model/configuration invalidates the comparison.

## Catalog evidence checkpoint (AP-01A; SQL-01/02)

The normal free-text route now hydrates retrieved measure/KPI/dimension candidates
from exact admitted catalog coordinates before reranking. It validates kind/ID,
source, publication version, source-generation digest and the canonical facet body;
misleading text cannot redefine a root. Original vector text remains unchanged in
retrieval receipts. A separate versioned context representation includes nested KPI
constituents, measures, typed physical column mappings, related dimensions/filters,
and the unique confirmed relationship paths required by those dependencies.

The shared closure supports dimension roots, rejects missing dependencies, duplicate
coordinates, KPI cycles, ambiguous/disconnected joins and bounded size/depth failure.
The context assembler retains or omits each hydrated candidate as one unit. Existing
explicitly pinned metrics remain mandatory. This does NOT automatically pin all
retrieved candidates, infer grouping selections, change rule applicability, narrow
physical authorization or alter saved wire contracts. Those AP-01/AP-02 obligations
remain open; an omitted optional group still requires selection-aware handling in
that later slice. Context hydration is not an analytical correctness proof.

`TestSQLRecoveryFreeTextKPIHasAtomicCatalogDependencies` calls the normal route with
no metric IDs or constituent hits in English and Spanish and compares its sealed
context with the explicit KPI closure. Other focused regressions cover dimension
connecting paths, corrupted facet text/revisions/origins, detached copies, missing
constituents/cycles, cancellation and whole-group budget omission. Exact committed
runtime results are tracked in the PR; no live-quality claim is made.

## First runtime verification and surrounding defects

At source `44a4d6777a2c8a783171c4ea99a6531a0889e45a`, all thirteen new recovery
regressions passed under the race detector, as did the semantics/drafts/rulesets/
topics, context, routing and NLQ execution packages. The full unit command failed
in two separate existing paths. Investigation found that warehouse validation
let the fallback erase a native-detected marker when the caller supplied zero
values. Fallback counting now only compensates for omissions; it may never reduce
the native AST inspection count. This preserves the existing accepted read subset.
The existing six-dialect parameter-contract test is the regression oracle. This is
native-parser boundary evidence, not live cloud-engine evidence.

The HTTP example-activation dispatch fixture also omitted the `sources.query`
action already required by production. The fixture now supplies that action for
the authorized dispatch case and adds an explicit denied-action regression. No
production authority rule was relaxed. The synthetic request spy now validates
its recorded response against the same generation schema and emits an actual
empty parameter array rather than a null array.

The initial integrated acceptance output also exposed reference-choice binding,
saved-query repository, stale CTE-shape and fixed migration-count failures. They
led to the integration follow-up below, rather than skipped or removed assertions.
Verification results after each checkpoint belong in the PR with their exact SHA.

## Integration follow-up

Committed follow-up fixes distinguish a reviewed reference-only clarification from
an executable scalar predicate. Both paths retain current rule/source replay; a
reference-only path rejects fabricated SQL binding receipts, altered current choice
IDs and missing predicate evidence for an actual effect. Saved-query projections
now derive their field order from the shared scanner list while replacing result
rows with SQL NULL, retaining the newly added reviewed relation scope.

The CTE acceptance fixture now tests supported flat CTEs positively with actual
filtered results and keeps an unsupported recursive-shape negative. Migration
acceptance verifies the shipped 44-entry prefix and all current suffix migrations,
rather than incorrectly rejecting legitimate later forward migrations by count.
No migration content, production predicate scope or permission was loosened.

`TestSQLRecoveryReferenceOnly` covers current rule replay and rejects forged binding,
source, effect and change evidence. Existing saved-query/CW-01 integration tests are
kept in the dedicated workflow, which also runs the directly changed exec and
PostgreSQL packages. Full live/owner qualification and the remaining AP packets are
still pending. The initial temporary branch-only patch publisher was removed before that checkpoint.


## Selected semantic intent (AP-01B; SQL-01/02/06)

`catalog-selection-v1` combines explicit references/metric IDs, longest matching
reviewed catalog names/aliases, interpreted dimension targets and resolved
clarification references. It computes bounded dependency and rule-requirement
expansion before embedding. The same transitive facts activate rules and
clarification policies; only selected roots, not incidental KPI ingredients or
column description associations, may automatically answer a reference-choice slot.
A hard exclusion, unresolved name collision, missing dependency or ambiguous join
stops before SQL generation. Expansion uses at most 128 references per topic and
16 passes, in addition to the existing semantic/token limits and cancellation.

The complete selected graph, including connecting paths to grouping/filter
relations, is mandatory context. Existing metric pins retain their contract; common
extra dependencies are included once through the existing mandatory lane. Atomic
retrieved candidates remain optional. Retrieval similarity alone does not select
all returned concepts. Exact duplicate concepts across explicitly combined topics
are accepted only after their typed coordinates, full dependency definitions and
physical source bindings agree; independently confirmed multi-topic joins are still
required. Unqualified explicit metric IDs retain their ambiguity rejection.

The detached route now includes `semantic_selection` with version, topic/digest
pins, roots/reasons, transitive facts and dependency digest. It is interpretation
metadata, not authority, native validation or an analytical correctness certificate.
Selection participates in in-process business evidence sealing and deterministic
clarification replay. Legacy records with no selection keep the prior replay path;
new selected-context markers cannot be silently downgraded to it.

`omitted_roots` is an optional request/saved-selection field. Paired typed removal
or replacement records an omission so an unchanged utterance cannot reselect the
removed concept. Explicit add clears it. Omission cannot disable a reviewed hard
requirement or erase a constituent needed by a different selected KPI. Inferred
catalog roots are made addressable by existing refinement edits; required/default
choices are reevaluated rather than promoted to user selections. A changed
omission set cannot borrow another preflight's clarification answer context.

These additive fields leave absent-field legacy JSON behavior intact. This change
does not add operations, permissions, model roles, a second tokenizer or a new
executor; it does not affect frozen report refresh. `ClarificationInput.selection`
is an optional pure preview input distinguishing roots from applicability facts;
nil keeps previous choice inference. Selection must be a subset of supplied facts.

Qualification is split deliberately: deterministic catalog terms and reviewed
aliases are implemented, not calibrated free-form paraphrase intent. Broader
semantic/value/time continuation, cross-pattern answer-dependent applicability,
full provider-envelope fitting and SQL analytical conformance remain separate
obligations. The regression corpus checks API/route adaptation, mandatory closure,
rule fixed point, ambiguous aliases/graphs, shared-topic identity, deterministic
replay, explicit omissions and a real PostgreSQL/Bifrost-recorded-wire authoring
flow. Runtime results belong to the exact committed CI run, not this source note.


## AP-02 initial packet checkpoint (SQL-03/04/05)

[Generation packet v2](../contracts/generation-packet-v2.md) specifies the scoped
render-only selected projection, one example lane, precedence-aware fitting and
actual usage metadata. Native authority still checks the full reviewed relations.
Normal free-text route tests add twenty unrelated hundred-column relations while
requiring the same prompt and exact selected dependency content. Missing/foreign
column coordinates fail, and legacy/unselected shapes retain their rendering.

New regressions cover ranked example order, edit/hint suppression, deduplication,
whole optional-group pruning, mandatory preservation, oversized demonstration
fallback, detached usage receipts, zero irrelevant learned-example selection and
repair without nested suppressed demonstrations. A recorded Bifrost wire test
checks runtime instructions/model/schema/output ceiling and rejection before any
provider call when the effective system addition exceeds the byte ceiling.

The added `fit`/`usage` evidence is optional on retained legacy JSON: absence is
unknown, not zero. Final provider-window allocation, broader root coverage and
live owner-cohort comparison are not completed by this slice. No frozen query,
report refresh, semantic publication or signed authority policy changes.

## AP-02B checkpoint: effective-envelope admission

Implements local preparation and refitting of generation/repair packets against the
actual runtime role/model/system, normalized JSON/schema framing, conservative input
bound and full output reserve. Required context never yields. Optional examples and
evidence carry bounded `provider_budget` omissions and persisted initial-use evidence.
An exact operator model-window registry rejects uncovered runtime overrides; absent
registry leaves context capacity unknown, not guessed. Actual attempt envelopes are
content-free digests/counts distinct from provider-reported usage. Runtime model slices
are detached to prevent fit/dispatch drift. No authority, native validation, owned
predicate, replay or frozen execution boundaries are changed.

Tests added for total-envelope boundaries/escaping, exact models/configuration,
immutability/redaction, no-dispatch failure, mandatory preservation, ranked omissions,
default fallback, unsealed/retained rejection and repair-specific fitting. Runtime
results are maintained in PR #62 after the exact committed-source workflow completes;
this checkpoint is not by itself a passing gate or live-model qualification.

## AP-03A — scoped analytical metric proof (implementation checkpoint)

Baseline head: `9c2b71c084cee9f801c662f87af54c12e3d336e7`. The
[analytical metrics contract](../contracts/analytical-metrics-v1.md) defines the
new positive native proof and its explicit limits. Catalog-backed user output
roots compile into a bounded aggregate/arithmetic tree; distinct per-metric
populations do not become one global intersection. Native-safe wrong aggregates,
missing filters, integer division and unsupported scopes stop before execution.
The existing bounded validation correction can repair a candidate without exposing
private bound scalars. Migration 053 preserves versioned scoped receipts and legacy
unmeasured disposition through query/saved reads, execution and idempotent replay.

Fixtures now asking for a reviewed Revenue metric return its real SUM (grouped by
id where the existing execution fixture checks per-id rows), not raw rows disguised
as a metric. The reference-choice fixture restores its raw-row response after its
metric-only case so subsequent scalar constraints retain their original assertions.
New synthetic tests exercise native results, persisted proof fences, private repair
and terminal replay. Local formatting/diff/planning checks are separate from actual
runtime tests; the PR records exact CI outcomes. No full AP-03 or live-parity claim.

### AP-03A integration and arithmetic review corrections

The first full runtime pass exposed an incorrect assumption in the new checker:
PostgreSQL source discovery uses the broad `numeric` category for integers,
decimals, floats and money. The checker now resolves exact arithmetic from verified
native type metadata (including numeric precision/scale), not from the category or
sample values. Its pure fixtures use the real source category too. Unknown and
approximate native types cannot acquire an exact-arithmetic proof through casting.

The adversarial pass also rejects NULLIF hidden inside aggregate inputs or grouping
terms: that can change the population/grain rather than merely guard a denominator.
Exact integer/decimal widening remains supported, but casting a text or approximate
aggregate to numeric is not normalized into the original reviewed metric.

Existing saved-query fixture responses now aggregate revenue per ID instead of
returning raw detail rows under a metric request; frozen block SQL and source/replay
assertions are unchanged. A shared model fixture is restored with deferred cleanup
so a failed subtest cannot change later scalar-filter results. None of these fixes
relaxes native validation, changes signed permissions or skips failing tests. Exact
final run evidence belongs in the PR; inventory and intermediate green units are
not a claim that the full integration passed.


## AP-03B1 — direct grouping and versioned proof

Continuation baseline: `dba8a7fc16ccd8fc48e49d6d4ce1a436bf4eecee`. Its final
expanded logs contain 1,219 passing unit/subtest events, one pre-existing opt-in live
provider skip, and 113 passing acceptance events without failures or skips. That
qualifies the scoped AP-03A software slice, not live model quality or broad parity.

[Grouping v2](../contracts/analytical-grain-v2.md) introduces a narrow, complete
reviewed-dimension suffix recognizer and exact SQL grouping/projection conformance.
Unknown grain remains explicitly unmeasured. No ordinary dimension mention, metric
label, quoted value or required-only reference becomes a grouping by itself. The
existing correction includes a closed grain diagnostic; scalar ownership stays
server-side. Old metric receipts are reconstructed with their original v1 policy.

Migration 054 retains the immutable version/contract/scope fence and adds bounded
grouping identities to v2 proof evidence. The native checker still refuses joins;
cardinality metadata alone does not demonstrate that physical rows cannot multiply.
Selected time buckets, query-wide predicates and general role inference are pending.
This slice is not the full AP-03B join/population program. Synthetic regressions and
real PostgreSQL/recorded-wire tests run in the existing read-only recovery workflow.
Actual current-head results and patch identities belong in the PR completion ledger.


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

## AP-06A accepted candidate explanation custody

Continuation baseline `10c06b9c89a5fac960de8db7c342fe8193ccb4ce`.
[Generation explanations](../contracts/generation-explanations-v1.md) closes the
lifecycle split in which fresh Plan returned model notes but persisted queries
kept only a generic routing statement. The accepted candidate's bounded, redacted
notes now populate existing durable fields and all ordinary result projections.
Validation-rejected proposals do not survive or enter repair prompts. Execution
correction updates notes only after all applicable acceptance checks; rejected
corrections retain the prior interpretation. Parent metadata remains immutable.
Known private parameters and current/parent sensitive answer spellings/aliases are
removed before storage/response, with whole-note withholding on marker overflow.

No old row, source scope, SQL/parameter proof, public schema or migration changes.
The opaque idempotent Plan-ID path is deliberately unchanged. Notes remain model
claims; ambiguity gating and parameterized learning remain separate obligations.
Unit and actual PostgreSQL/recorded-wire regressions cover both lifecycle and
privacy boundaries. Exact executed local/CI results are recorded in the PR.

## AP-05A parameter custody checkpoint

Baseline `a2559bdf4e468d6436f0bd37f2066250f628add6`. The
[refinement parameter contract](../contracts/refinement-parameters-v1.md) closes the
unbound-SQL/parameter-context gap: the service reauthorizes and verifies the exact
parent, retains model values privately, and supplies slot metadata to generation.
A native PostgreSQL clause comparison rejects silent slot reassignment, changed
Boolean/predicate structure, dropped/literalized placeholders and source changes.
First candidates and the existing bounded correction both restore original values
before owned business binding/native validation. Reference-only receipts no longer
wipe the valid SQL base. Current typed answer replacement does not inherit the old
owned SQL/value, while independent model slots remain intact.

No migrations, public DTO fields, authority grants or frozen execution changes.
Unchanged non-PostgreSQL SQL can preserve bindings, but modified parameterized SQL
requires future native role qualification. Model-slot value edits and full inferred
scalar/time continuity are explicitly not delivered. Runtime/CI results, source
hashes and remaining phase status are updated at the PR checkpoint after testing.

## AP-05B — explicit typed model-slot replacements

Continuation baseline: `41acc1cb19353100408f9496c9c6cc221f3e0e14`. The
[parameter-edit contract](../contracts/refinement-parameter-edits-v1.md) closes the
absence of explicit value replacement within AP-05A's bounded SQL-role proof.
Refine may replace an existing model slot with a same-kind private typed value;
it cannot remove slots, change their role or address a service-owned parameter.
Selections are detached, parent-fenced and restored before binding/validation and
again on the existing correction path. Only parameter kinds/positions reach the
model. New child bindings persist in existing fields, without rewriting ancestors,
changing analytical versions or exposing values on public responses.

The optional input is defined on the shared Refine DTO, with closed-schema tests.
Native/analytical checks and zero-work denied/replay expectations remain. Units,
EN/ES lifecycle, source-result, mixed-owned and correction fixtures are registered
in the recovery workflow. Local runtime verification cannot be claimed from a
dependency-missing environment; executed current-head evidence belongs in the PR.
This slice does not close free-text interpretation, time/value inheritance,
broader native shapes or live qualification.


AP-05B review also found that the new field needed named SDK value/edit aliases.
`NLQRefineRequest` already forwards the service DTO, but external callers should
not need internal package names to construct its slice elements. Public aliases
and an external-package authenticated wire test now cover that consumer. The
recovery suite includes the actual SDK package and requires this test's pass
marker; forwarding does not duplicate service-side edit validation.

## AP-06B — typed learned SQL examples (unpublished implementation checkpoint)

Baseline verified at `c655c8fc1c4f81b3e66f8501187b78538b49fbe5`, which includes
explicit typed parameter replacements and their public SDK support beyond the
older PR-body checkpoint. That baseline's standard and expanded CI are green; those
results do not qualify this continuation.

[Parameterized examples v1](../contracts/parameterized-examples-v1.md) adds a
strict, value-free positional schema to learned candidates. Review and import use
public typed probes with the existing native validator rather than private historic
values. Questions are known-value redacted; unsupported/private copied SQL
annotations are not proposed for learning. Service-owned predicate queries retain
feedback without automatic examples. Immutable schema persistence, a versioned
digest/portable row and public SDK aliases preserve the contract through reuse.

Implemented tests cover the pure schema, service/type/digest seams, conservative
SQL disclosure, SDK transport and actual PostgreSQL/recorded-provider lifecycle.
The current environment exposes no GitHub write actions and has no native/module
cache, so patch checkpoints are the publication mechanism for this continuation.
Only actually executed checks may be marked passed in the delivery summary. The
new integration/migration tests remain a required gate; AP-06 is not declared closed.

## AP-07A — shared validator/generator vocabulary

Continuation baseline: `2838b22dc681ce2cdddda911ab11660dc3f91eb6`.
[Read-SQL vocabulary v1](../contracts/sql-vocabulary-v1.md) gives the existing native
name gates and generation/correction instructions one immutable owner. This is a
function/type/operator/value-name refactor and a concrete prompt consumer, not new
SQL grammar, vendor capability discovery or proof of analytical intent. Frozen
baseline lists and parser-backed tests guard against accidental permission widening.

The profile is derived from the admitted dialect and included before full provider
fitting. Runtime model overrides cannot change the source dialect. Registry digest,
case/namespace/marker behavior and private-value boundaries are tested. Unknown
profiles fail without model dispatch. No migration, response field, source scope,
analytical version, retry allowance or frozen-execution behavior changes. Broader
function signatures, read shapes, per-engine native/analytical qualification and
live-owner quality remain open under AP-07. Exact CI results belong in PR #62;
this implementation inventory alone is not a passing/runtime claim.

## AP-08A readiness discriminator (implementation checkpoint)

Continuation baseline `858a4ec90b0600e4e7cb0db5821cbea12f64493d`.
[Generation decisions](../contracts/generation-decisions-v1.md) closes the control
flow gap in which a model could only return SQL plus unclassified ambiguity prose.
New generation/repair responses must distinguish ready, user clarification, and
insufficient reviewed context. Incompatible mixtures or missing declarations fail;
blocked results never reach the next SQL validation or a new physical execution.
A positive model declaration remains subject to all existing proofs. The protocol
cannot itself prove the model detected every material ambiguity.

No previous query/explanation is backfilled or given a new interpretation. Typed
errors preserve bounded redacted questions across HTTP/MCP/SDK without minting a
plan, reviewed answer token or source permission. Earlier routing/model work is
not reported as zero; transient blocked questions are not a new durable workflow.
Recorded ready fixtures change their schema declaration, not their SQL/results or
assertions. Exact current-head runtime verification is recorded in the PR; this
checkpoint does not by itself qualify the full AP-08 release/owner-quality program.

## Adversarial P0/P1 hardening checkpoint

At baseline `c8c0a8db9327b147f313830037fcc56353536909`, the dedicated
[P0/P1 audit](sql-adversarial-p0-p1.md) identifies two analytical correctness defects
(parent-only relation populations and falsely inferred decimal arithmetic) plus an
operation-finalization defect after correction-context failure. Fixes preserve
all existing authority/native/analytical checks and have independent historical
failure assertions plus actual PostgreSQL result/replay cases. The full existing
matrix remains selected. Implementation is not runtime qualification: exact-head
results and the final open/closed finding ledger are maintained in PR #62.

Adversarial integration refined the findings: ONLY is blocked by the existing
native layer and is defense-in-depth, not a reachable P1. The real Executor's
nil-error/durable-query-failure contract exposed a separate P1 in NLQ correction
admission. The consumer now recognizes only stopped terminal query-error receipts
without rows; it preserves the same single-correction budget, rejects uncertain
states and finalizes retained-context preparation failures. No native or executor
interface is weakened. Exact rerun evidence belongs to the final reviewed head.
