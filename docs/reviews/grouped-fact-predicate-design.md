# Bounded grouped fact predicates: new implementation design

This is a new implementation proposal derived from the current v8/v10/v11 source
contracts. It does not recover an earlier unpublished implementation. Runtime
qualification is pending.

## Existing failure and owner

`compileCurrentAnalytical` dispatches all nonempty typed constraints to v11.
`compileAnalyticalGroupedSelection` reconstructs the reviewed independent lanes,
then `validateGroupedSelection` accepts only direct selected grain coordinates.
Thus a reviewed sales-only row predicate cannot restrict sales while leaving the
independently selected refund population intact. The new regression supplies a
sales coordinate while grouping by a separately reviewed customer-region key.
It expects an owned sales restriction and fails in the existing v11 compiler.

The owning phase is 18, using the existing authenticated router, source binder,
native validator, analytical checker and durable query evidence. No new endpoint,
semantic authoring syntax, permission, inference role or SQL dialect is proposed.

## Proposed closed v12 placement

1. Reconstruct the exact current v8 grouped program or v10 period program from
   the admitted publications, selected transitive metric leaves, current binding,
   and authenticated constraints. Preserve the reviewed joins and group domains.
2. Keep direct selected grouping-key constraints as v11 final-spine selection.
3. Admit a non-key row constraint only when its exact reviewed physical dataset
   is one selected aggregate fact root and is absent from every other lane's
   reviewed join path. A dimension seen through a join is never assigned a fact
   owner, even if it occurs once. A fact that is also another lane's joined parent
   is ambiguous and rejected. Table coincidence never creates global ownership.
4. Reject aggregate/HAVING constraints, shared joined dimensions, derived bucket
   predicates, missing domain policies and outer joins on an affected fact lane. Keep existing physical uniqueness,
   null-safe alignment, missing-lane NULLs and metric filter obligations intact.
5. Represent new fact predicates separately from reviewed period populations;
   compose them only inside the owning aggregate before GROUP BY. Optional final
   selection stays after complete-key alignment. No new join is introduced.

## Proof and durable custody

Use an immutable `analytical-metrics-v12` contract and a separate binding schema 5
with a distinct policy. Older versions reject new fields and shapes. A v12 record
must have at least one new fact restriction; pure group selection keeps v11.

The binder continues to accept only flat named aggregate CTEs and one named
UNION DISTINCT spine, rejecting all model parameters and unbound placeholders.
It binds each exact fact root and then applies any final-key predicate. It offsets
private parameters and records the owner of each effect. Native validation,
current source/context/rule admission, analytical provenance, exact output
ordinals, SQL/parameter digest, and full receipt reconstruction remain mandatory.
Provider guidance removes all private populations; receipts retain no values.

Migration 080 appends only the closed v12 analytical shape; migrations through
079 are immutable. Retained v0-v11 and schemas 1-4 keep their original meanings.
Current schemas 2-4 learning consumers remain unchanged. Schema 5 learning is
explicitly ineligible until it has a separately qualified current-placement
consumer; it must never fall back to ordinary or schema-4 example reuse.

## Required independent qualification

- Actual red compiler regression before production changes, followed by green
  fact-only, multiple fact predicates, optional period and final selection cases.
- Native placement negatives: wrong lane, copied predicate, joined/shared field,
  final/spine relocation, extra WHERE/HAVING, model parameter capture, changed
  values, source/context/revision drift, old version/schema impersonation.
- Real PostgreSQL numeric and NULL oracle independent of generated SQL: missing
  lanes, genuine zero count, qualifying all-NULL input, null grouping keys and
  distinct raw versus qualifying domains.
- Current-value replacement from protected unbound SQL; original Plan replay,
  terminal Run replay with zero model calls, current authority/rule admission,
  exact receipt/schema tamper and learning ineligibility checks.

Exact executed commands, output and tested tree will be reported separately.

## Source-derived narrowed checkpoint

The generic typed binder rejects all outer-join target scopes. The original LEFT
regression remains a supported-boundary failure; it is not relabeled as fixed.
An adjusted INNER fixture was independently run on unchanged production and also
failed with `analytical_group_selection_unsupported`. Only that INNER slice is
implemented now. Future preserved-root LEFT support needs a separate exact AST
preservation-side proof and scoped binder; no generic join guard is weakened.
