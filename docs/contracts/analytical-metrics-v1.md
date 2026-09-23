# Analytical metrics v1

Status: AP-03A implementation under review in the SQL recovery PR. This is scoped
analytical evidence, **not** business approval, an execution credential or complete
natural-language/result-correctness certification.

## Ownership and supported proof

The existing NLQ service compiles selected user metric roots from the exact admitted
published topic definitions. Catalog text, model declarations and optional retrieval
fragments cannot issue the contract. Required-rule ingredients remain governed
context but do not become extra requested output metrics. Exact topic/version/digest,
selection digest, physical source binding and the original parameterized statement
are retained separately from user-facing content-free proof metadata.

`exec.CheckAnalyticalPlan` only accepts an existing native-validated opaque plan. It
reuses the pinned native PostgreSQL parser for a narrower positive proof. It cannot
create a plan or expand signed reach. Normal authority, native validation, EXPLAIN,
read-only source credentials, execution caps and finalization remain mandatory.

The first version checks selected aggregate expressions and per-metric populations
relative to the query-wide population over one physical base relation:

* SUM, AVG, MIN, MAX, non-null COUNT and COUNT DISTINCT use exact reviewed columns.
  COUNT(column) and COUNT(*) are equivalent only for a non-null source column.
* Reviewed `eq`, `in` and `not_null` metric filters stay with each aggregate. FILTER
  and a searched CASE with NULL ELSE are accepted equivalent shapes. Different
  populations are not globally intersected. A reviewed filter common to every
  constituent may be applied as a top-level WHERE conjunct.
* A KPI can use bounded arithmetic `+`, `-`, `*`, `/`, decimal constants and exact
  declared input IDs, including nested KPIs. Prose expressions, guessed synonyms,
  implicit dependencies, cycles and ambiguous/missing mappings are not compiled.
* Division must be numeric and use NULLIF(denominator,0), or a known nonzero literal
  denominator. This explicit v1 policy yields NULL on zero. Casting an already
  truncated integer division does not satisfy the policy. NULLIF cannot silently
  change a numerator or an ordinary selected SUM.
* Harmless aliases, equivalent commutative operands, unconstrained numeric widening
  casts and direct-column grouping (including aliases/ordinals) are supported.
  Equivalent selected meanings from more than one admitted topic can be checked
  when they still use the same base relation; signed reach for all topics remains.

No join/cardinality/fan-out proof, CTE/nested-query proof, window proof, set-operation
proof, derived grouping-expression proof or non-PostgreSQL analytical proof is
claimed. Unsupported new selected-metric plans fail explicitly rather than acquire
an executable plan labeled analytically passed. This draft is not ready to replace
all previously supported analytical shapes in a parity release.

The receipt scope is deliberately
`selected_metric_expression_and_population;single_base_relation`. It does **not**
prove that the selected metrics answer the user's question, that grouping columns
match the intended grain, or that all additional query-wide WHERE/HAVING/limit
choices express the user's intent. Service-owned scalar/time constraints still have
their independent binding/replay proof. AP-03's broader analytical contract and
AP-08 result-quality qualification remain open.

## Generation, correction and privacy

The enforced shape and division policy are part of the system instructions before
complete effective-envelope fitting. The compiler runs before SQL generation;
unsupported catalog arithmetic does not spend an SQL-generation/correction call.
Routing/embedding activity may already have happened and is not reported as zero.

A candidate first passes native safety, then analytical conformance. A classified
candidate mismatch can use the existing one validation correction, with the rejected
**unbound** statement and a closed diagnostic. Original parameter bindings stay
server-side. Private clarification predicates are rebound after correction. No raw
source error, result row or bound private scalar is inserted into repair guidance.
All attempts use the existing gateway budget; failed conformance never executes SQL.

`analytical_mismatch` and `analytical_unsupported` are distinct HTTP 422 outcomes,
not permission grants or successful/native-only plans. The native positive grammar
adds the explicit PostgreSQL NULLIF node with recursively checked operands; unknown
functions/columns inside NULLIF remain unsafe.

## Persistence and compatibility

Migration 053 adds an immutable analytical-version marker and nullable bounded
receipt to the existing query row. Version zero means legacy/unmeasured and has no
receipt. It is not retroactively certified. Newly generated plans use version one;
selected user metric roots require a receipt, while plans without selected metric
outputs explicitly have none. Scope-aware query and saved-query projections retain
these fields; saved reads continue to exclude result rows.

A receipt has the proof version, explicit limited scope, contract and parameterized
query hashes, and ordered metric identities. No formulas, filter values, SQL or
results appear in that receipt. An empty parameter list has one canonical hash across
JSON/database round trips. The normal query-lineage hash includes new nonempty
fields without changing absent version-zero serialization.

Execution reconstructs the contract from current or retained exact semantic pins,
compares the stored proof/query hashes, revalidates native safety and recomputes the
analytical proof before a physical call. Terminal idempotent replay checks retained
metadata without introducing model/source execution. The database prevents version
downgrade, proof removal and changing contract/scope/metric identity. Only the query
hash may change after an already permitted semantics-equivalent execution correction,
which also receives a fresh analytical check. Existing execution equivalence is not
relaxed. No model work is added to frozen reporting refresh or artifact reads.

## Qualification and remaining work

Synthetic fixtures cover aggregate and population substitution, exact decimal
comparison above 2^53, nullable/non-null counts, numeric division/zero policy,
unsupported scopes, missing/stale pins, private scalar repair, two-population actual
results, persistence/proof-downgrade denial and zero-work terminal replay. The owning
recovery ledger and PR record exact executed commands/results. Test inventory alone
is not a passing result or live model/cloud-engine evidence.

Remaining AP-03 obligations include canonical or positively proven multi-relation
plans, fan-out avoidance, shared versus metric-specific population interpretation,
question-level grouping/ordering/filter conformance, more safe arithmetic equivalences,
per-engine qualification and result-based owner acceptance. An unsupported expression
is disclosed rather than counted as a correct generated answer.
