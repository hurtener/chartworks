# Authenticated grouped fact periods

This bounded S4/S5 increment uses `analytical-metrics-v10` and binding receipt
schema 3. It extends the existing authenticated `KPI.periods` consumer to
independently aggregated grouped fact lanes. It does not close the grouped
owned-filter parent scope or the live-model and migration qualification gates.

## Reviewed ownership, never inferred reach

The existing `metric-period-bindings-v1` metadata maps every selected transitive
measure leaf to an exact reviewed temporal dimension. The router's in-process
seal, or authenticated replay against the retained publication and current source
binding, supplies each typed interval. The compiler rechecks publication/version,
mapping/source digests, complete leaf coverage, fact identity, temporal type,
calendar, timezone, and agreement of the selected periods within each fact.

Two to four fact lanes must also have an explicit reviewed grouped alignment
policy. Filtered lanes require their reviewed raw/qualifying group-domain policy.
A shared order-date dimension may restrict both an order fact and a refund fact
only when each fact's reviewed period mapping explicitly names it. A refund-event
period instead restricts that fact's event date. The presence of an accessible
join, a repeated dimension, a suggestive field name, or source values supplies no
business ownership. Physical source-backed uniqueness is still mandatory.

## Exact grouped populations

Each v10 lane carries its protected typed `QueryPopulation` together with its
fact, reviewed INNER joins and group domain. Each lane has exactly one row-level
time-window constraint in this increment. The complete lane evidence, including
private interval values and raw/qualifying policy, participates in the contract
and binding digests. The outer contract has an empty owned population.

The native checker composes the independently reconstructed owned predicate with
exact raw/qualifying domain semantics using the existing bounded positive AND/OR
proof. Raw groups are the owned-period rows; qualifying groups additionally
satisfy the union of the selected lane's complete metric-filter populations.
Each aggregate retains its own reviewed filters. Qualifying all-NULL amounts
still establish a group, `COUNT(amount)` can genuinely be zero, and an absent lane
remains NULL after alignment. NULL grouping keys use the existing NULL-safe
complete-key alignment. No COALESCE or zero fill is invented.

The new binder only accepts flat named aggregate CTEs and one named distinct-UNION
key spine. It binds inside the unique fact-rooted aggregate body, offsets all
private parameter indexes, and records each effect's fact population. All model
parameters and preexisting placeholders are rejected before insertion. Missing,
duplicate or unknown fact lanes, outer/spine substitutes, or extra restrictions
cannot obtain the final native-plus-analytical proof.

## Private custody and retained execution

Provider guidance projects away every lane's private population. The public
binding receipt is value-free and names `grouped-owned-query-predicates-v1`.
The protected period-free SQL is retained separately for refinements. Refinement
replays the parent's authenticated applications to recover its grouping, then
routes the new request and binds new current applications to the unbound base.
It never carries the parent's private period parameters into the edit prompt.
The separately reconstructed [owned-base learning consumer](owned-example-learning-v1.md#scoped-population-reconstruction)
admits schema 3 only after exact authenticated binder reconstruction and current
scoped applicability. Eligibility alone does not establish actual prompt use.

V10 receipts use `independent_owned_grouped_populations`, exact output ordinals,
and the full SQL/parameter and contract digests. Terminal execution replay
reconstructs ownership and fresh native/analytical output evidence before exposing
retained values, without executing result rows or calling a model. Original Plan
operation replay and immutable store evidence use the same existing fences.
Migration 077 appends the closed v10 receipt scope without rewriting v0-v9 rows.
V7/v8/v9 cannot accept the new lane population field; binding schemas 1 and 2
retain their prior meanings and cannot impersonate schema 3.

## Remaining scope

Generic non-temporal fact-only or shared-dimension query filters remain rejected
by v10. The separate [v11 final-group-selection contract](analytical-group-selection-v1.md)
admits only authenticated direct final-spine predicates; it does not change v10. Different
periods within one fact, HAVING-owned restrictions, more than four fact lanes,
LEFT joins inside period-bound aggregates, derived-wrapper/derived-spine binding,
mixed ordinary-completeness/period capabilities, and non-PostgreSQL execution
are not added. Existing v8 unowned derived/calendar
programs and v9 scalar/completeness capabilities retain their existing contracts.
No new source, identity, or publication authority follows from this increment.

## Qualification

Focused compiler and native-checker tests cover shared versus fact-specific time
ownership, both domains, exact bindings, private guidance and refinement bases,
wrong-lane/capture/spine negatives, and prior-version/schema rejection. Real
PostgreSQL acceptance uses independently computed row oracles for NULL keys,
all-NULL amounts, zero counts, missing lanes, canceled-only groups, cohort versus
activity intervals, durable Plan/Run replay, corrupted output ordinals and
period refinement. Shared order-month cases distinguish month/year boundaries
and refund-activity rows whose order-month has no matching gross lane. Schema-3
initial HTTP/SDK planning and terminal Run replay retain the same evidence and
results; repeated Plan operations retain their original query identity.
Executed results are reported separately; this inventory is
not a claim that a test has run or a live-provider measurement.
