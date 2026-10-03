# Complete-group selection qualification

Date: 2026-10-02 UTC. This finite PostgreSQL increment adds authenticated
selection of complete aligned groups after independent fact aggregation. It
supports predicates on direct shared grouping keys, with optional existing
fact-owned periods, under analytical v11 and binding schema4.
[Contract](../contracts/analytical-group-selection-v1.md).

## Implementation and review

The compiler rebuilds the unchanged v8/v10 lane semantics before attaching
current sealed group constraints. The binder first proves the complete named-CTE
program, then inserts typed parameters only against direct final key-spine
columns. Native validation and the v11 analytical checker remain independent
mandatory consumers. Replay reconstructs the exact SQL, parameters and complete
receipt under fresh source/semantic authority; arbitrary stored JSON is not a
binding seal. Migration078 adds a closed receipt shape while retaining prior
version predicates and existing rows.

An integration review found a missing v11 dispatch in retained grouping
refinement; it was corrected and tested with actual private-answer replacement.
A test initially asserted that all authorized Route JSON must omit canonical
answers. The existing clarification contract explicitly retains those editable
answers under signed authority, so the test was corrected to inspect value-free
Analytical/Bindings receipts, ordinary logs and every provider request separately.
No source sensitivity, canary or provider-traffic assertion was weakened.

## Executed local evidence

Go1.27.1, Linux amd64, pinned native parser, real PostgreSQL17.6 with pgvector0.8.2,
and recorded providers only. No paid/live model call was made.

- Complete affected race suites: **1,951 passing unit/subtest events** across
  eleven packages, zero failures. The existing opt-in local MySQL completeness
  test skipped because its separate server was not configured
- Broader real-source consumer suite: **70 passing acceptance events**, zero
  failures/skips, 216.419 seconds. This includes current/prior grouped programs,
  scoped fact periods, ordinary query predicates, grouping/refinement/saved paths,
  owned-example learning/requalification and the automatic grouping producer
- Focused new service suite: all six raw/qualifying, optional-period and NULL-only
  configurations passed in 109.883 seconds. Independent source-row arithmetic
  checked results, missing/all-NULL/zero lanes, original replay, saved copies and
  private-answer refinements
- Rejected model parameters and misplaced lane/spine predicates, corrupted
  SQL/parameter/full receipts, stale source/scope and foreign authority all remain
  failures. Provider traffic, public proof receipts and ordinary logs remain
  value-free; authorized Route echo follows its existing contract
- Full build, vet, goimports, planning coherence, source hygiene and mirrored
  contributor instructions passed

Earlier focused failures are preserved as failed runs and are not counted as
passing suites. The final source includes the scalar grouping producer correction
from PR72; it does not modify the prior frozen live candidates.

## Limits

This does not implement arbitrary fact/shared-dimension predicate pushdown,
HAVING ownership, derived wrappers, predicates on derived calendar buckets,
additional dialects, general windows/sets or scoped-example learning. Schema4
remains ineligible for automatic reusable examples. Hosted checks and live quality
are separate from these local recorded-provider results. Renderer kernel and
final release qualification remain open; no full S4/S5 or recovery completion is
claimed.
