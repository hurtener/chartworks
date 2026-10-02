# Reviewed grouped aggregate populations v1

Status: SQL recovery S5 implementation increment. The completion tracker owns
exact-source qualification. This is PostgreSQL grouped-CTE support, not blanket
nested-query, dialect or live-model qualification.

## Reviewed alignment meaning

A topic may explicitly publish `grouped_population` with policy
`union-null-equal-preserve-missing-v1` and a sorted unique list of two through four
fact dataset IDs. Its review, publication digest, version diff and portable
export/import preserve the policy and exact dataset mappings. Unknown policies,
duplicate or missing datasets are invalid. Absence of this policy does not imply
an alignment or zero-fill choice. This metadata grants no source, relation,
column or execution permission.

The compiler uses the policy only when its exact fact set equals the aggregate
inputs of the selected outputs and an exact common reviewed direct-column grain
is selected. Every fact receives its own aggregate lane. Each lane independently
resolves the reviewed relationship tree to the selected dimension fields and
checks physical uniqueness on the opposite side of every metric input. A fact
can be used as a unique dimension bridge in another lane, but that lane never
aggregates its measures. Reviewed INNER and LEFT population semantics remain
exact. Physical-key removal or source revision drift invalidates the proof.

## Native proof

Each lane is one named CTE with exactly its expected aggregate leaves, complete
selected grouping columns and reviewed metric predicates. No extra raw fact
join, unreviewed predicate, nested program, HAVING, grouping, ordering or limit is
admitted inside a lane. Source-qualified columns resolve only inside their own
lane. Aggregate input source identity remains part of count equivalence.

A separate key-spine CTE must UNION the complete keys from every lane exactly
once. UNION is distinct, including NULL groups. UNION ALL, missing or reused
lanes, expressions, filters and positional misalignment are rejected. The outer
query must LEFT JOIN each lane exactly once directly to the spine on every key
with IS NOT DISTINCT FROM. This has one output row for each group appearing in
any lane and matches NULL keys once; ordinary equality, incomplete composite
keys, INNER alignment and chaining through a nullable previous lane are rejected.
Grouping outputs come from the spine. Metric arithmetic uses only the proved
named aggregate outputs and reviewed expressions. Outer reviewed order/limit
checks still apply. Missing lane values remain NULL, including counts; this
policy supplies no business zero-fill rule. A separately reviewed v7 metric
expression may explicitly apply its own exact COALESCE policy after alignment.

## Retention and evidence

The distinct `analytical-metrics-v7` receipt names
`independent_grouped_populations`. The full typed lane policy, source binding,
selected semantic revision, exact SQL/parameters and grouping identity are
hash-bound and reconstructed on durable replay. Migration 063 admits v7 without
rewriting retained v0–v6 rows or widening their policies. The v7 intent and
query-population markers are mandatory when a receipt exists.

Deterministic tests cover two, three and four lanes; composite grouping; NULL
alignment; duplicate-spine/fan-out, filter, grouping, metric, empty-population and
physical-key adversaries; policy review and detached portable mappings. The
synthetic Commerce acceptance executes recorded provider SQL through the real
PostgreSQL validator, native reader, Plan/Run and restarted replay. Independently
frozen regional amounts include matching, gross-only, refunds-only and NULL
keys, duplicate refund events and empty refund input. These fixtures are not
live model measurements or owner migration parity evidence.

Calendar buckets, arbitrary derived/window/set programs, grouped owned predicate
projection and additional dialects remain explicit further recovery work. A
fact-local user predicate must not be copied to another lane without reviewed
population meaning; the current compiler rejects nonempty grouped query-owned
constraints rather than guessing a projection rule.
