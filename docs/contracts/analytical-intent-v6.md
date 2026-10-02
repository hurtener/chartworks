# Reviewed analytical ordering and limits v6

Record version 6 adds optional `reviewed-order-limit-v1` intent to native-first
analytical validation. Retained records 0–5 reconstruct their original policy;
no historical receipt acquires this proof. The contract and query digests bind
priority, reviewed metric/field identity, direction, null placement and row count.
The public receipt contains only the policy marker, never filter values.

The current deterministic English/Spanish compiler resolves a terminal
`ordered by` / `sorted by` / `ordenados por` clause against selected reviewed
metric and categorical/identifier dimension names and aliases. It supports
ordered lists, ASC/DESC, explicit NULLS FIRST/LAST and an optional terminal
`limit N` / `límite N`, bounded to 1–100000. PostgreSQL default null placement is
explicitly retained when omitted. A `top N` / `bottom N` / `mayores N` /
`menores N` prefix uses the single selected metric with nulls last. Ambiguous
labels, multiple candidate metrics, invalid bounds, unsupported trailing syntax,
and combined prefix/suffix ranking instructions fail rather than invent intent.
The parsed suffix is removed only from a detached grouping-compiler input;
original request, protected route and durable replay evidence remain unchanged.

A terminal limit-only clause is also supported; it leaves ordering explicitly
unmeasured rather than inventing a ranking key.

The native AST matcher accepts exact aggregate expressions, output aliases and
whole-target ordinals. It follows PostgreSQL output-alias precedence for ORDER BY,
checks priority/direction/null semantics and rejects extra or missing keys.
FETCH FIRST N ROWS ONLY and LIMIT N are equivalent; changed counts, nonzero offsets,
LIMIT ALL and WITH TIES are not. Integer-bound parameters must retain the exact
count. An explicit ordering request without a count prohibits added limits.
Ambiguous duplicate output aliases cannot produce proof.

Recognized intent also activates exhaustive query-predicate accounting against
the existing typed owned-predicate contract, including the empty contract.
Generated WHERE/HAVING restrictions cannot silently narrow the requested result.
Aggregate-local reviewed populations remain independently checked. The existing
single correction allowance receives closed order/limit mismatch diagnostics;
failed proof produces no executable query record.

This is a bounded reviewed grammar, not exhaustive natural-language understanding.
Questions outside it retain the separately scoped metric/grouping/population proof.
Reviewed calendar bucket ordering retains the exact unit, calendar and timezone
from the grouping contract; it cannot substitute a different bucket. Generalized
ranking/window functions and arbitrary algebraic equivalence are not claimed. Joins, physical fan-out
and non-PostgreSQL dialect proofs have independent requirements. Source-backed
acceptance tests exercise duplicate metric ties, NULLs, exact record ordering,
limit changes, bounded correction, persisted restart replay and immutable evidence.
Test execution evidence must be recorded separately from this specification.

Version 6 additionally admits reviewed unary plus/minus KPI arithmetic. Unary
minus is proved as exact zero-minus-expression, while signed numeric constants
retain their canonical rational value. This extension does not change integer
division, denominator-zero guards, nullable aggregate populations or floating
point exclusions. Older policy versions retain their prior expression grammar.

The ratio proof also recognizes searched CASE zero guards whose tested expression
is exactly the reviewed denominator: `WHEN denominator=0 THEN NULL ELSE ratio`,
or `WHEN denominator<>0 THEN ratio ELSE NULL`. Changed denominator populations,
nonzero thresholds, non-NULL alternatives and generalized conditions fail. Native
source tests compare positive, zero and NULL denominator results.
