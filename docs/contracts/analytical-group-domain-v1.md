# Reviewed ordinary aggregate group domains

`TopicPack.group_domain` and the matching public/portable definition carry the
optional reviewed policy `{policy:"metric-group-domain-v1", domain:...}`. This
metadata grants no dataset, join, filter value, or execution authority. It is
consumed only by analytical v8 ordinary grouped queries, including physically
proved nonmultiplying raw joins. Independent fact lanes retain their separate
per-lane domains and alignment contract.

- `raw_source_groups`: grouping rows are the admitted source/join population,
  restricted by independently owned row predicates. Metric-specific filters stay
  on their aggregates and cannot remove groups from the input population
- `qualifying_population`: grouping rows additionally satisfy the union (OR) of
  the complete reviewed filter populations of the selected aggregate leaves.
  Each aggregate still has its own exact population. Group existence precedes
  aggregate NULL treatment: all-NULL qualifying inputs create a group, whereas
  a group containing only nonqualifying rows does not

No zero fill is inferred. `COUNT(column)` may be zero for an existing group with
all-NULL inputs; this does not justify creating an absent group. A group key of
NULL has ordinary SQL grouping semantics. Empty grouped input produces no groups.

For filtered-only grouping, every topic contributing a selected metric must
explicitly supply the same policy. Absence or disagreement fails before SQL
generation with internal `analytical_group_domain_review_required`. Fresh HTTP/MCP requests expose the existing `generation_context_insufficient` problem with fixed English/Spanish instructions to have the topic owner review and publish the domain policy, then submit a new question. This is distinct from retained-v7 intent review. A source sample,
example, model answer, successful query, or prose heuristic cannot resolve it.
The author must supply a reviewed draft policy and publish it through the normal
lifecycle. If a selected aggregate is unfiltered, the qualifying union is TRUE;
v8 canonically uses `raw_source_groups` because the two domains are equivalent.
Scalar totals and retained v1–v7 compilation/checking remain unchanged. This fix
does not relabel historical receipts or claim stronger retained-version proof.

The v8 compiled contract includes the policy in its immutable hash. Positive
AND/OR normalization is bounded to 64 clauses/products and 64 atoms per clause,
with depth 32. Typed reviewed metric predicates and independently reconstructed
query-owned predicates form the exact expected row domain. Bound values remain
private. The ordinary query-population checker still verifies HAVING, and the
metric, grain, join, ordering, limit, native-safety, and authority checks remain
independent. Unknown topology or expressions fail closed.

`TestSQLRecoveryOrdinary*` covers compiler defaults, policy hashing, retained
versions, typed scalar/range/time constraints and altered-condition negatives.
`TestSQLRecoveryOrdinaryGroupDomainAcceptance` uses recorded provider responses
and actual PostgreSQL single-base and unique-dimension joined results, including
canceled-only, duplicate-fact, NULL-key, all-NULL-value and empty populations.
The WHERE/FILTER relocation is explicitly rejected before execution.
`TestSQLRecoveryMySQLOrdinaryGroupDomainLocal` qualifies the matching MySQL 8.4
WHERE/CASE path with duplicate inputs, NULL/all-NULL groups and canceled-only
phantom groups. This increment records actual PostgreSQL/MySQL evidence only;
shared normalized AST code is not additional engine qualification. These are
runtime correctness fixtures, not live-model quality evidence.
