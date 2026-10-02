# SQL context recovery AP-02C — scoped physical projection

## SQL recovery status — 2026-09-30

The [AP-00–AP-08 completion tracker](sql-recovery-completion.md) records the current
PR #62 implementation and qualification gaps. The recovery is **in progress**.
Existing shipped phase labels and historical defect-review results do not
close this subsequent extension. No required behavior is discarded by this
tracker correction; prior named acceptance criteria and historical evidence stay.

Current hosted-qualified checkpoint: `c755ad3`. The subsequent runtime tree
`0ce69bd875deb82df3d3276cbd1c060f815ebfbd` passed integrated local qualification,
including finite S10/S11 domain/signature completion. Its final published head
still needs hosted checks. S4/S5/S6 and the explicitly documented legacy predicate
approval/resume path remain implementation work; Q1/Q2/Q3 are separate gates.
The canonical tracker controls current scope; historical evidence below does not
qualify later source.


Continuation baseline: `cf689701cae5898e5d0c2edad4b06287793d6b37` in PR #62.
Owning runtime remains phases 17/18 and the
[generation packet contract](../contracts/generation-packet-v2.md).
Exact source, test outcomes and remaining obligations are recorded in the PR;
this review note alone is not a runtime pass or complete migration parity.

The context renderer now handles independently selected dimension/column closures
and per-topic mappings without changing `AssembledContext.Relations`. Selection
and dependency identities are consumed exactly as the existing catalog-selection
producer emits them, with no change to stored semantic digests or query proofs.
Public JSON still cannot create a sealed generation context. Shared datasets keep
each topic's own full reviewed column list because the assembler does not receive
confirmed join choices. This retains existing join grounding without asserting
minimal projection, approving a new join or claiming fan-out conformance.

## Adversarial checks

A private predicate is not an output schema. A question containing only an
interpreted value/time filter, clarification effect or required-rule dependency
must retain full detail context. The extended projector requires an independent
catalog/explicit dimension, column or metric root before narrowing a topic.
Unknown and clarification-only reasons remain conservative. Private filter
columns still join the mandatory closure when an independent dimension exists.
The original single-topic metric renderer remains unchanged.

Foreign or unqualified metric ownership, forged dependency ownership, missing
physical fields and one semantic column mapped to two source names are rejected.
Shared datasets cannot lend a column from another topic's projection. Legacy
unscoped selections keep their original rendering; mixed scoped/unscoped metadata
is rejected. Opaque metrics, dataset-only and unselected topics keep full rendering.
Work is bounded by the existing topic/relation limits, 128 selection roots and
4,096 combined dependencies. Retained and projected slices are detached.

## Runtime evidence to collect

The recovery workflow runs all NLQ/semantic/native/store/gateway unit race tests
and the existing integrated acceptance selection on both Go 1.26.4 and 1.27.1.
New tests cover dimension-only EN/ES provider packets, private filter dependencies,
filter-only detail context, durable complete validation scope, saved-result privacy
and terminal replay with no additional provider or source work. Multi-topic tests
exercise the actual selection/constraint producer and independently confirmed
relationship checks; they are not a new live multi-warehouse result qualification.

The workflow records the exact committed source, independently applies the net
continuation patch from the baseline with a separate Git index, and compares the
complete resulting tree. Formatting and validation are read-only; errors are fixed
in commits, never by rewriting tested source in CI. No new migration, action scope,
public operation or analytical proof version is introduced. Owner/live quality,
minimal join-aware projection and the other AP-00–AP-08 obligations remain open.


## S9 qualified grouping slice — current disposition

The [completion tracker](sql-recovery-completion.md) records runtime `996db06` and exact
Go 1.26.4/1.27.1 qualification for grouping inheritance/replacement, explicit
totals/calendar selection and the strict pending-refinement admission fix.
Executable scope, form origin, private binding and immutable-parent checks remain.
S9 broader native parameter roles and the remaining S/Q requirements are still
open; historical checkpoint prose above is not the current completion claim.


### Current SQL recovery checkpoint — S3 qualified, S7 under test

The canonical completion tracker records S3/S9 qualification at c18a5b2 and
the new closed source-diagnostics implementation. Specific native causes do not
authorize different SQL semantics, extra attempts, or disclosure of source error
text. New S7 integration qualification is still pending; all other unchecked S/Q
items remain open. Historical acceptance and release claims keep their old scope.


## Current SQL recovery: S7 qualification and S2 grounded policy

The [canonical completion tracker](sql-recovery-completion.md) now records qualified S7 diagnostics
at 2dd7e9b and the new [bounded grounded concept policy](../contracts/grounded-concept-selection-v1.md) awaiting its
own runtime qualification. The optional clarify-role choice feeds reviewed
closure/rules, not SQL authority. Explicit choices and historical replay retain
their behavior; unknown/ambiguous model choices cannot produce executable plans.
No interpretation-confidence, live quality or full-recovery completion is inferred
from these software changes. Historical results above retain their stated scope.
