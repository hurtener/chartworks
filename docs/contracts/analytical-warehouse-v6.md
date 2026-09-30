# Warehouse analytical proof, v6 structural increment

Version 6 can consume the full structural AST from the pinned warehouse parser
for MySQL, SQL Server, BigQuery, Snowflake and Databricks. Native safety and native
planning remain prerequisites; the analytical checker never creates a plan.
Earlier analytical versions retain PostgreSQL-only policy.

The adapter translates positive AST nodes into the shared closed proof vocabulary.
It does not rewrite SQL, execute a reference query or assume that token similarity
proves meaning. Every translated node has an explicit shape and unexpected
semantic modifiers fail. Existing parser input/node/depth bounds and cancellation
checks remain in force. Source binding, parameter identity and immutable receipt
hashes always refer to the original query and source metadata.

This increment covers ordinary selected aggregates and exact arithmetic, independent
CASE metric populations, CASE/NULLIF zero guards, direct grouping, owned scalar
WHERE/HAVING predicates, output/ordinal/expression ordering and native row-limit
forms (LIMIT, SQL Server TOP and FETCH without percentage/tie expansion). Shared
predicate matching validates exact typed values without exposing them in receipts.
The detached proof view canonicalizes verified native integral/decimal types;
approximate floating point and session-sensitive timestamp types are not upgraded.
MySQL `/` has decimal division semantics; SQL Server integer AVG cannot impersonate
an exact mean. Explicit bounded decimal casts only receive normalization where
integral count widening is proved, rather than discarding arbitrary scale changes.

Ordering respects each engine's default null placement. Snowflake ordering with an
unmeasured session null-order policy needs explicit NULLS FIRST/LAST. Positional
parameter order is preserved in the supported grammar; the ambiguous combined
LIMIT/OFFSET parameter representation is rejected. Native source validation can
still reject a generated output alias before analytical comparison; the current
actual MySQL tests use the equivalent aggregate expression in ORDER BY.

This is not completion of S6. Full dialect-specific calendar expressions, native
output-alias binding, warehouse CTE/derived-lane translation and the remaining
required nested/analytical query matrix need further implementation and tests.
A name allowlist or AST fixture is not actual-engine qualification. The structural
suite covers five warehouse grammars with positive and adversarial cases; actual
MySQL tests separately validate source results, and do not qualify the four remote
engines. Current-source CI/race and engine evidence belong in the completion ledger.

## Subsequent structural calendar and nested-lane implementation

The next increment adds registered native calendar forms: MySQL civil/date
DATE_FORMAT cast-to-date buckets and an exact EXTRACT/MAKEDATE quarter anchor;
SQL Server DATETRUNC for date/civil values; BigQuery DATE_TRUNC, DATETIME_TRUNC and
TIMESTAMP_TRUNC with exact reviewed/default-UTC zone; and Snowflake/Databricks
DATE_TRUNC over date/TIMESTAMP_NTZ. Year boundaries, NULL groups, argument positions,
full date-part identity and timezone are preserved. Keyword exemptions are provided
by the shared structural signature policy, not general name exemptions. MySQL
session-sensitive TIMESTAMP and other unproved session-dependent types are not
certified as civil time. Actual MySQL calendar tests run under multiple session
zones and verify this distinction.

Native CTE/derived ASTs can now reach the independent singleton-population proof,
with scoped CTE names, bounded nesting, exact derived aliases and source parameter
order. No extra row filter, grouping, local limit or arithmetic substitution is
introduced by normalization. Source-authority validation of derived-column lineage
remains an independent prerequisite. BigQuery untyped fractional literals are
FLOAT64 and cannot acquire exact decimal proof merely from their spelling. Explicit
temporal cast precision is retained in predicate comparison.

These additions do not replace real-engine qualification for each advertised engine
or close every remaining S6 form. Original native parser syntax/work limits remain
in force. The generation packet now chooses calendar guidance by dialect from the
same registered vocabulary and exact bucket contract.
