# Grouping continuity v1 (S9)

Status: grouping and pending-refinement slice qualified at `996db06` in PR #62.
Broader S9 native parameter roles remain open; exact evidence is in the tracker. Extends phases 17/18 and the existing
[interpretation continuity](interpretation-continuity-v1.md),
[analytical grouping](analytical-grain-v2.md) and
[calendar grouping](analytical-calendar-v3.md) contracts.

## One complete logical grouping

An optional `grouping` input on normal routing/Plan/Refine/saved selections contains
`policy: reviewed-grouping-v1` and at most 16 unique keys. Each key identifies an
admitted `topic`, reviewed `dimension` and optional allowed calendar `grain`.
There is no SQL, physical relation, scalar value, timezone override or authority
in this input. Nil means unspecified. A nonnil input with an empty key list
explicitly requests a scalar total; this differs from unmeasured grouping.

The complete set replaces prior grouping. Adding/removing an axis uses a replacement
set; clearing the set requests the total. The current router resolves every key,
checks reviewed temporal policy and required rules, and includes all selected
dependencies in mandatory context. An exact grouping input takes precedence over
stale grouping words in the carried question. Inferred old grouping roots are not
reactivated from those words. Explicit references supplied with the current edit,
interpreted filters and hard rule dependencies retain their independent paths.

## Refine and source custody

After the existing actor/session, parent lineage, current source and authenticated
scalar-replay checks, Refine reconstructs the parent's original analytical proof.
Only a verified grain becomes an inherited grouping. Old SQL is never parsed as
intent and a legacy unmeasured record does not gain an assertion about its previous
result. Existing known groupings use their exact source-bound logical dimensions.

A new complete reviewed EN/ES `by`/`por`/`per` suffix replaces inherited grouping,
including reviewed month/day/quarter/year buckets. This reuses the existing bounded
recognizer and its column/source checks. Candidate dimensions considered during
recognition are not promoted into rule facts. No provider call is added. When no
grouping change is recognized, the previous group survives a filter/period change.
An explicit affirmative but unresolvable grouping fails rather than silently
retaining an existing verified group. When the parent had no measured grain, an
unrecognized raw-column phrase stays on the pre-existing unmeasured routing path;
no old group is silently inherited and no new correctness proof is manufactured. Exact standalone `total`, `grand total`, `overall
total`, `total general`, `sin agrupar`, `without grouping` and `no grouping` clear
it. General paraphrase/negative-language understanding remains S2, not an implicit
claim of this recognizer. Private answers are redacted before recognition.

Typed grouping changes compose with existing private model-parameter custody and
explicit same-type parameter edits when the question text is unchanged. Parameter
positions, kinds, source/alias namespace and every containing clause remain fenced.
Grouping changes outside those clauses are allowed; moving a parameter from WHERE
to HAVING, reassigning its column/operator or changing its source is rejected. New
free-text language on model-parameter parents still requires fresh Plan. Broader
nested/join/window parameter-role proof remains open under S9.

## Proofs, persistence and replay

Forward migration 059 adds analytical record version 5 (`analytical-metrics-v5`).
Older v0-v4 records reconstruct their original policy and cannot accept the new
explicit grouping input on replay. The selected grouping is included in the query's
protected routing record and analytical contract hash. Initial/native analytical
validation checks both GROUP BY and projected keys, including the deliberate empty
set. A verified total has its own explicit
`selected_metric_expression_population_and_scalar_total;single_base_relation` scope;
unknown grouping keeps the narrower metric scope. Existing private population proof,
calendar and arithmetic checks remain in force. No join, nested scope, ordering or
other-dialect analytical support is added by this version.

Pending answer origin checks include the canonical grouping, even when only a
calendar grain changes and the reference set does not. In-process evidence seals
include it without changing historical nil-input seals. Saved selection reads use
detached canonical sets. The existing immutable analytical trigger prevents removing
or swapping proof/version/scope; execution and terminal replay reconstruct the exact
retained policy. Frozen artifact/report refresh adds no model work.

## Qualification

New route, compiler, native-plan, SDK and PostgreSQL/recorded-provider tests cover
replacement/inheritance, explicit totals, calendar reselection, source/key bounds,
stale grouping rejection, private model/owned bindings, immutable parents/proofs,
saved-result privacy and terminal replay without new reads or model calls. A fixed
synthetic result cohort distinguishes per-record 10/20/5, status 5/30, total 35 and
monthly 15/20 results. Historical tests retain their exact proof versions; only
fresh-authoring expectations advance to v5. Test inventory is not passing evidence;
current commands/results are in the PR and completion tracker.

Remaining S9: wider native parameter-role proof for required complex SQL shapes,
explicit broader language/grouping intent and owner journeys. S2/S4/S5/S6 keep their
original scope. A safe unsupported result is not counted as implemented support.

### Additional review cases

Multiple temporal dimensions may share one physical timestamp while declaring
separate grain/timezone policies. Reconstructing a logical grouping checks the full
bucket definition against that dimension's reviewed policy, not just its column.
An existing explicit grouping retains its original protected dimension-to-grain
pairing instead of expanding a flat set into every possible combination. When a
flat older/natural proof leaves two same-field dimensions compatible with multiple
buckets, reconstruction returns unsupported rather than guessing those pairings;
an explicit logical set resolves the ambiguity. Tests
cover that shared-field case, independently changed inferred values/periods,
period removal with calendar grouping intact, and managed saved grouping questions
through inspect/prepare/execute/recover with mismatch rejection.

Pending forms also allow an authorized Refine question to replace their prior
explicit grouping. A supplied but not-yet-resolved scalar is masked before this
local recognition, even though it does not yet have a sensitivity resolution.
Those ephemeral redaction descriptors never become resolved answers or rule facts.
The ordinary pending-origin and current reviewed answer checks still run.


### Pending-form admission is not executable-query admission

A pending preflight intentionally has no validated SQL relation-scope receipt.
Refining it first reauthorizes the exact current topic versions and source binding,
then replays its original clarification/selection evidence before constructing a
new child. It must not be rejected solely because it lacks a planned-query scope,
but it also must not use that absence as an executable-scope fallback. The pending
path rejects any SQL, parameters, operation/result, analytical receipt, correction
count or stale evidence. Actual planned-query and terminal replay admission still
require the exact retained nonempty relation scope. Parent metadata is never filled
in or mutated during this read. New child native and analytical validation remain
mandatory. A changed stateless Plan submission cannot borrow the old form; the
existing authorized Refine path owns the interpretation change.

Regression qualification is recorded against the exact published source, including
actual pending-form answer/refinement and rejected unplanned Run. This subsection
records behavior and bounds, not a claim that an unexecuted checkpoint is green.


## S9 qualified grouping slice — current disposition

The [completion tracker](../reviews/sql-recovery-completion.md) records runtime `996db06` and exact
Go 1.26.4/1.27.1 qualification for grouping inheritance/replacement, explicit
totals/calendar selection and the strict pending-refinement admission fix.
Executable scope, form origin, private binding and immutable-parent checks remain.
S9 broader native parameter roles and the remaining S/Q requirements are still
open; historical checkpoint prose above is not the current completion claim.
