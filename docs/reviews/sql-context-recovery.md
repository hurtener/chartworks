# SQL context recovery — phased implementation ledger

Baseline: `673d74b281c4f800bfa86ef7415443d99209ba24`. Work is tracked in PR #62.
This is a recovery extension of phases 16–18 and 24, not a new release gate.
Implementation, passing synthetic fixtures, native execution, and live owner
qualification are separate evidence states. Nothing here declares full parity.

## Packets

| Packet | Owned behavior | State |
|---|---|---|
| AP-00 | Synthetic sentinel cohort, effective request inspection, protected owner comparison | Core engine-request spy implemented; provider-wire/owner baseline pending |
| AP-01 | Selected semantic roots, catalog hydration, complete dependencies and rule applicability | Candidate hydration/closure implemented; unified root selection and applicability pending |
| AP-02 | Relevant typed prompt projection, examples and complete-request budget | Pending |
| AP-03 | Analytical contract and semantic conformance | Pending |
| AP-04 | Targeted privacy-safe validation repair | Initial implementation; runtime verification pending |
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
constituents/cycles, cancellation and whole-group budget omission. Runtime evidence
is pending the dedicated committed-source workflow; no live-quality claim is made.

## First runtime verification and surrounding defects

At source `44a4d6777a2c8a783171c4ea99a6531a0889e45a`, all thirteen new recovery
regressions passed under the race detector, as did the semantics/drafts/rulesets/
topics, context, routing and NLQ execution packages. The full unit command failed
in two separate existing paths. Investigation found that warehouse validation
let the fallback erase a native-detected marker when the caller supplied zero
values. Fallback counting now only compensates for omissions; it may never reduce
the native AST inspection count. This preserves the existing accepted read subset. The existing six-dialect parameter-contract test is the regression
oracle. This is native-parser boundary evidence, not live cloud-engine evidence.

The HTTP example-activation dispatch fixture also omitted the `sources.query`
action already required by production. The fixture now supplies that action for
the authorized dispatch case and adds an explicit denied-action regression. No
production authority rule was relaxed. The synthetic request spy now validates
its recorded response against the same generation schema and emits an actual
empty parameter array rather than a null array.

The initial integrated acceptance output also exposed reference-choice binding,
saved-query repository, stale CTE-shape and fixed migration-count failures. Those
need independent baseline comparison and disposition before this PR is mergeable;
a passing new regression cohort does not erase broader gate failures. Verification
results after this checkpoint belong in the PR with their exact source SHA.
