# SQL context recovery — phased implementation ledger

Baseline: `673d74b281c4f800bfa86ef7415443d99209ba24`. Work is tracked in PR #62.
This is a recovery extension of phases 16–18 and 24, not a new release gate.
Implementation, passing synthetic fixtures, native execution, and live owner
qualification are separate evidence states. Nothing here declares full parity.

## Packets

| Packet | Owned behavior | State |
|---|---|---|
| AP-00 | Synthetic sentinel cohort, effective request inspection, protected owner comparison | Core engine-request spy implemented; provider-wire/owner baseline pending |
| AP-01 | Selected semantic roots, catalog hydration, complete dependencies and rule applicability | Catalog-term selection plus explicit/clarified roots feed required closure and rules; vector-only paraphrase intent and owner qualification pending |
| AP-02 | Relevant typed prompt projection, examples and complete-request budget | Selected single-topic physical rendering, strategy-aware optional fitting, example usage and runtime byte recheck implemented; model-specific full-envelope fitting pending |
| AP-03 | Analytical contract and semantic conformance | Pending |
| AP-04 | Targeted privacy-safe validation repair | Initial implementation with focused regressions; broader qualification remains open |
| AP-05 | Analytical state and parameter ownership through follow-ups | Pending |
| AP-06 | Parameterized learning and retained generation evidence | Pending |
| AP-07 | Generator/validator dialect capability agreement | Native marker-count fallback defect fixed; broader operation matrix and live-engine qualification pending |
| AP-08 | Ambiguity gating and owner-cohort qualification | Pending |

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
