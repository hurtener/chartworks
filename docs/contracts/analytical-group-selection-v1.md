# Authenticated final-group selection

The bounded PostgreSQL v11 contract (`analytical-metrics-v11`) selects complete
aligned groups using authenticated predicates on direct final key-spine columns.
Binding receipt schema 4 uses `owned-group-spine-predicates-v1`. Existing v8 group
populations and optional [v10 fact periods](analytical-grouped-owned-periods-v1.md)
keep their meanings. This does not introduce generic predicate pushdown.

## Compilation and placement

New plans obtain constraints from the in-process router seal. Retained execution
replays the exact reviewed route under current signed authority and source binding.
The compiler first reconstructs the independently reviewed lanes, group domains,
physical unique joins and selected shared grain. It accepts a group predicate only
when its exact dataset/column is a direct selected grouping key. Ordinary scalar
and join queries keep their previous query-population compiler and receipt.

The SQL subset is flat named aggregate CTEs plus one named distinct UNION key
spine, followed by NULL-safe complete-key LEFT alignment. The service inserts group
predicates only in the final SELECT's WHERE, using the spine's qualified keys.
It never puts them in a guessed fact lane, a UNION arm, or a nullable aligned lane
output. Optional authenticated periods remain bound inside their fact-owned CTEs.
All supplied model parameters/placeholders, preexisting outer restrictions,
derived wrappers, unreviewed ordering/limits and extra fact restrictions fail closed.

Raw and qualifying domains are checked before selection, with each aggregate's
own reviewed population intact. All-NULL qualifying inputs still establish groups;
zero COUNT values remain zero; missing lane outputs remain NULL. A NULL-only group
selection has a binding effect but does not invent a scalar parameter. Calendar
keys may coexist with direct keys, but predicates on derived calendar buckets are
outside this increment. Non-group fact filters, HAVING ownership and other SQL
dialects remain unsupported.

## Private custody and replay

Typed private constraints participate in protected contract/binding digests.
Provider guidance contains placement and reviewed coordinates, with all predicate
values removed. Public receipts contain policies, digest/ordinal evidence and
parameter indexes only. Authorized Route/answer JSON retains canonical data for
binding and editable continuation, as specified by the existing
[clarification contract](conditional-clarification-v1.md). The unbound model SQL and private parameters stay in
protected storage; refinements start from the unbound base and reroute/rebind the
current answers. Existing structured logging redacts the contract and evidence.

Schema 4 assigns empty `population` only to final-spine effects. Optional period
effects retain their exact fact identity. Replay reconstructs SQL, parameters and
the complete binding receipt, compares the current source coordinates, then proves
exact output ordinals through native/analytical validation before exposing retained
results. Original Plan replay, saved copies and current/retained source admission
use the same authority and immutable-lineage fences. Reusable learning remains
ineligible for scoped binding schemas 2, 3 and 4.

Migration 078 appends the closed v11 analytical shape without rewriting old rows.
V0-v10 and schemas 1-3 retain their prior evidence meanings and cannot claim v11
selection. No new operation, source permission or publication authority is added.

## Qualification boundary

`TestSQLRecoveryGroupedSelectionCompiler`, its custody/receipt tests, and the
native `TestSQLRecoveryGroupedSelection*` cases cover sealed input, source/scope
mismatch, direct-column ownership, private guidance, old-version rejection and
wrong-location/model-parameter negatives. `TestSQLRecoveryGroupedSelectionAcceptance`
uses a real PostgreSQL driver and an independent row oracle across six raw,
qualifying, optional-period and NULL-only configurations. It covers HTTP/SDK
planning and replay, missing/all-NULL/zero lanes, corrupted receipt/SQL/parameter
reads, foreign authority, refinement, saved copies and feedback ineligibility.

These are recorded-provider software tests. Executed results are reported separately;
this contract does not assert live provider quality or completion of wider S4/S5.
