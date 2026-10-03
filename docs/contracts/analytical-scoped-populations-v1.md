# Fact-owned scalar populations

Status: bounded v9 recovery increment; full generated-corpus qualification is
tracked separately. Recorded provider fixtures are not live model evidence.

## Reviewed meaning and current custody

`KPI.periods`, policy `metric-period-bindings-v1`, maps every transitive measure
leaf to one exact reviewed temporal dimension. Each selected KPI has at most
four bindings. A measure's input relation identifies its fact; filter and period
dependencies do not create additional facts. Every leaf on the same fact must
agree on its time basis and independently resolved interval. Conflicting or
missing mappings require review; no business date is inferred from column names.

Authenticated routing binds the selected KPI, immutable publication/version,
canonical mapping digest and source binding to a current interval for each mapped
dimension. Date/civil/instant type, calendar and timezone are resolved separately
for each dimension. The compiler checks those pins and complete leaf coverage
again. Wire JSON cannot recreate the in-process application seal. Replay uses
the existing authenticated semantic/interpretation reconstruction, not saved
SQL clauses or caller-supplied application objects.

The first runtime permits two to four distinct fact lanes, up to four selected
period applications, and one period per fact. A reviewed cohort can apply an
order-date interval to both the order fact and its refund fact; an activity
definition can instead map refunds to refund-event time. Repeated parent-table
references alone never authorize copying a predicate. Unrelated generic owned
constraints and different periods for one fact are explicitly unsupported.

## Existing proof engine, separate lane scopes

`analytical-metrics-v9` carries `ScalarPopulations` with policy
`scoped-singleton-populations-v1`. Each lane records its fact, exact reviewed
joins and protected typed query population. Relationship-bound metric filters
must retain their selected reviewed relationship ID. The first joined path is a
confirmed INNER fact-to-parent relationship with complete equality keys and
actual source-backed uniqueness on the opposite side of every aggregate input.
Cardinality labels, sampled distinct counts and period metadata grant no reach.

The existing singleton outer checker combines named, flat aggregate CTEs by
CROSS JOIN. Each lane reuses the ordinary metric, join and query-population
checker. GROUP BY, HAVING, windows, lane ordering/limits, derived/reused/missing
lanes, raw inter-fact joins and extra restrictions remain rejected. Only the
reviewed outer arithmetic and final intent are admitted. Scalar aggregates retain
one row on empty input: SUM remains NULL and COUNT remains zero. No zero fill is
invented, and all-NULL input is distinguished from an empty population.

## Binding and retained evidence

The scoped binder reuses the existing CTE scanner/layout and predicate binder.
It selects each CTE by its unique fact-root identity, applies only that fact's
proved period, and offsets parameter positions when combining the bodies. The
generic CTE binder retains its existing ambiguous-target rejection. Initial
scoped programs reject every model parameter and unbound SQL placeholder, so an
outer expression cannot capture a newly inserted private period value.

Business binding receipt schema 2 records each effect's fact population, exact
scoped-constraint digest and final parameter indexes. Its public projection is
value-free. The protected base statement is retained separately, and replay
reconstructs the same full statement, parameter vector and placement receipt.
Receipt 2 is accepted only with v9. The separately reconstructed [owned-base learning consumer](owned-example-learning-v1.md#scoped-population-reconstruction)
admits schema 2 only after exact authenticated binder reconstruction and current
scoped applicability; its policy does not itself grant predicate authority.

Ordinary fresh plans still issue v8 unless they select the explicitly reviewed
known-amount completeness capability described below. V9 scoped singleton
placement is selected only for an authenticated scoped application. Existing standard metric/grain/intent proof components use
an internal detached v8 policy view, while receipts retain the original v9
version and complete contract digest. V8 cannot accept v9-only lane fields.
Migration 072 preserves the prior v0-v8 database receipt predicate and adds the
closed v9 singleton and ordinary-completeness scopes; retained records are never
relabeled.

## Reviewed ordinary known-amount completeness

The separately reviewed `Measure.completeness` link identifies a scope-inheriting
unknown-count KPI. The semantic validator requires numeric SUM and an exact
`COUNT(nonnullable same-fact identity) - COUNT(amount)` companion under the same
metric filters, including relationship meaning. The companion has no independent
period mapping. The compiler additionally verifies the identity is nonnullable
in the current physical source binding and remains nonnullable in the effective
joined population. A null-extended relation in a LEFT join cannot certify complete
amounts without an explicit missing-row policy; none exists in this slice. The
compiler requires both owner and companion as
selected outputs before inference.

This first consumer applies only to directly selected SUM measures. It does not
invent obligations for transitive leaves of a selected net KPI. It also does not
collapse a period-mapped KPI into an ordinary fact scope: mixing ordinary
completeness and scoped singleton applications remains rejected. The ordinary
query's existing authenticated predicates, grouping, calendar, raw/qualifying
group-domain and final ordering/limit proofs all remain mandatory. No one-lane
singleton variant is introduced. Ordinary completeness admission is qualified for PostgreSQL and MySQL 8.4;
other engines remain unqualified for this new capability. Fact-owned scoped
singleton placement remains PostgreSQL-only.

The v9 contract and receipt retain policy `reviewed-known-amount-outputs-v1` with
exact selected owner/unknown-count MetricID pairs. Ordinary receipt scope gains
`reviewed_known_amount_completeness`. Every v9 receipt also records an exact
MetricID-to-zero-based-final-output-ordinal map, derived from proved target terms.
Equivalent repeated outputs choose the lowest matching ordinal deterministically.
Column aliases and driver labels are not semantic evidence. Grouping columns
remain in their actual positions and do not shift the meaning of metric IDs.

Receipt pairs and ordinals remain bound to the complete contract and SQL/parameter
digests. Stored metadata shape is checked before planning, then fresh native and
analytical proof must reproduce the entire receipt. Cached terminal replay also
revalidates that proof before using ordinal evidence, without executing result
rows or calling a model. V1-v8 receipts reject the new fields. Downstream amount
completeness concerns only returned query rows; it does not certify whole-source
coverage, truncated results or an unqualified business population.

## Mandatory context

Selected metric closures retain every root and dependency edge. Rendering may
share an exact definition body under its topic/version/kind/entity identity;
compact references do not omit semantic evidence. Conflicting same-identity
definitions fail closed, while definitions from different topics remain
separate. Unscoped legacy roots cannot acquire a shared namespace. The ordinary
1500/3000/6500 token ceilings remain unchanged. When no body is shared, historical
metric rendering remains unchanged.

## Qualification boundaries

Native parser/compiler tests cover period placement, complete composite keys,
source and version custody, wrong interval targets, model-parameter capture,
required-parent filters and missing uniqueness. Actual PostgreSQL controls use
generated reviewed Commerce definitions and independent cohort/activity results,
unknown-count companions, duplicate values/IDs, all-NULL and empty populations.
The actual MySQL control additionally checks scalar and monthly ordinary
completeness, misleading output aliases, duplicate amounts, canceled-only groups,
qualifying all-NULL groups, genuine zeros, current physical identity nullability,
and production annotation of typed results. Its private source registry is an
in-memory fixture; discovery, EXPLAIN and result execution are real MySQL.
Those controls do not by themselves certify natural-language output selection,
all nineteen held-out corpus cases, other dialects or live-provider quality.
