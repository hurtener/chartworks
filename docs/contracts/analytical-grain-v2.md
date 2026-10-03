# Analytical grouping v2

Status: AP-03B1 implementation in PR #62, not full AP-03 or release qualification.
Extends [analytical metrics v1](analytical-metrics-v1.md) without reinterpreting
retained version-one receipts or changing the native SQL authority boundary.

## Selected intent, not every mentioned dimension

The analytical compiler derives an optional direct-column grouping contract from
the same admitted publication and selected semantic roots used by generation. The
versioned `reviewed-dimension-suffix-v1` recognizer accepts a complete terminal
`by`, `per` or `por` list of reviewed dimension names/aliases, separated by commas
and/or `and`/`y`. This is a deliberately closed grammar, not free-form language
understanding. English and Spanish aliases must be explicitly reviewed.

A dimension mentioned as a filter is not automatically a grouping. A rule-required
reference cannot impersonate a user root; a complete affirmative grouping clause
must independently resolve to selected dimensions. Exact selected metric labels
containing `by`/`per` are not split into grouping instructions. Known sensitive
answers are redacted, quoted literals are masked and the redaction marker is never
a selectable term. Negated grouping and ordering clauses are not affirmative grain.

No recognized grouping remains **unmeasured**, not an instruction to return a
scalar total. This preserves existing metric-only callers, including raw-column
phrases without a reviewed selected dimension. Unknown grammar does not acquire a
grain receipt. Once a dimension in a grouping clause is recognized, an incomplete,
ambiguous or extra suffix returns explicit unsupported rather than a partial proof.
Temporal buckets, filtered dimension definitions and multi-topic grouping proofs
are not implemented by this version. Recognition and compilation have explicit
text/list bounds and cancellation checks.

## Positive SQL conformance

Only after the ordinary native validator issues a plan, the analytical checker
compares the complete direct-column GROUP BY set and projected grouping keys with
the compiled physical columns. Missing, substituted and invisible extra grouping
keys fail. Native aliases and output ordinals retain PostgreSQL's input-column
precedence; ambiguous duplicate output aliases are not used to infer grain.

Grouping does not weaken the existing aggregate, metric-filter, numeric division
or zero-denominator checks. A candidate with the correct GROUP BY but the wrong
aggregate or metric population still fails. No new join, CTE, nested, window,
set-operation or non-PostgreSQL analytical grammar is enabled here. In particular,
reviewed cardinality labels alone are not physical uniqueness/fan-out proofs.

Generation receives the validated grouping policy/column identities in its system
instructions before complete-envelope fitting. A mismatch uses the existing one
validation correction and the closed `analytical_grain_mismatch` diagnostic. The
failed **unbound** SQL and parameter-slot metadata remain separate from service-owned
scalar values. Native safety, owned-predicate binding and analytical checking run
again; conformance failure does not execute SQL. No extra model role/call is added.

## Versioning, scope and persistence

Migration 054 adds analytical record version two. Version zero remains unmeasured.
Version one recompiles the exact original metric-only policy and hashes, even when
its original question contains grouping words. Version two uses
`analytical-metrics-v2`; an optional grain is part of the contract hash. A grain
receipt names the sorted reviewed dimension identities, not raw user text, physical
SQL, scalar values or result rows. Its scope is:

`selected_metric_expression_population_and_grouping;single_base_relation`

When grain is unknown, v2 keeps the explicitly narrower
`selected_metric_expression_and_population;single_base_relation` scope and omits
`grouping`. Absence must not be displayed as a verified scalar-total choice.

The database's immutable analytical trigger remains intact: the record version,
contract, scope and grouping identities cannot change or disappear. Execution and
terminal replay reconstruct the contract using the row's original policy version;
query and saved-query projections retain its proof. Replay checks metadata without
introducing source/model work. Receipt slices are detached from callers.

The proof remains scoped: it does not establish natural-language correctness,
query-wide WHERE/HAVING/order/limit intent, general functional dependencies,
cardinality/fan-out, joins or business approval. Unchanged frozen report execution
continues to use its approved definition without model work.

## Acceptance and remaining obligations

Pure tests cover EN/ES aliases, conjunction lists, metric names with grouping-looking
words, filtered/required/literal/negated cases, missing/foreign mappings, partial or
ambiguous clauses, scope downgrade, v1 replay, missing/extra/invisible grouping keys,
unsupported join/nested forms and unchanged aggregate/population checks. Integrated
PostgreSQL/recorded-Bifrost tests cover actual grouped results, bounded correction,
private scalar binding, immutable persistence, saved projections and zero-work replay.

The PR records actual executed results and exact source evidence; this document's
inventory is not a passing/live gate. Broader role-aware language interpretation,
explicit grouping edits/inheritance, temporal grouping, query-wide population,
physical uniqueness-backed joins and per-engine analytical proof remain open.
