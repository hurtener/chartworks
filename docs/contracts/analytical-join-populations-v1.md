# Physical joins and independent aggregate populations

Status: SQL recovery S5 implementation increment. Exact-source qualification is
recorded separately in the completion tracker. This contract does not claim all
nested or grouped multi-fact queries are implemented.

## Source-backed fan-out proof

The PostgreSQL adapter discovers complete unconditional immediate unique keys
while holding the existing read-context table locks. Only valid ready plain-column
default catalog btree indexes with matching column collations qualify. Partial,
expression, deferred, custom-operator, hidden-column and incomplete composite keys
do not qualify. INCLUDE columns do not become keys. Keys are detached, bounded and
included in the source binding and physical fingerprint; native revalidation and
execution reject stale source evidence. A published cardinality label is not proof. Existing stored sources with no key
evidence retain their original metadata policy and cannot borrow newly observed
keys; explicit source rediscovery/revision is required to gain join proof. Stored
key-bearing sources always revalidate all key evidence.

The v6 compiler finds one reviewed relationship tree connecting selected metric
and grouping fields. Exact equality keys and INNER versus LEFT population semantics
are retained. Every edge needs a physical unique key on the opposite side of each
selected metric input. This admits many-to-one fact aggregation without claiming
that dimension totals are also safe. Ordinary equality permits nullable keys:
NULL does not match NULL. Null-safe equality is not interchangeable.

The native checker verifies the complete left-deep join tree, rejects extra or
missing conditions and relations, and resolves fields in a source-qualified local
namespace. Outer-extended columns are nullable for count equivalence. Grouping,
metric-specific filters, reviewed scalar arithmetic, order/limit and native safety
remain separate mandatory proofs. The receipt names the joined scope explicitly.

## Independent singleton aggregation

For ungrouped metrics from independent source populations, v6 can require one
single-row aggregate lane per selected source. Each CTE or derived table must be a
plain aggregate SELECT over its exact admitted base relation, with only reviewed
metric predicates. GROUP BY, HAVING, limits, nested joins, correlated references,
extra filters and unused or reused lanes are rejected. Every lane returns exactly
one row, including empty input. Combining them by CROSS JOIN cannot multiply facts.

The outer SELECT may combine the proved named aggregate outputs using the existing
closed arithmetic/zero-division rules. Counts retain source identity, so reversing
counts from different populations cannot compare equal. Empty SUM/AVG remains NULL;
the service does not invent a COALESCE-to-zero business rule. The receipt explicitly
identifies independent singleton populations. This proof is neither arbitrary SQL
rewriting nor a license to join unaggregated facts.

## Retention and boundaries

Forward migration 062 admits only the new v6 receipt scopes. Older policy records
remain unchanged and are not upgraded during replay. Source binding, semantic
revision, selected outputs, exact SQL/parameters and the complete analytical
contract remain digest-bound. Reconstructed proofs must match on replay.

The [v7 grouped-population contract](analytical-grouped-populations-v1.md) adds
reviewed grouped multi-fact CTE lanes. General CTE/window/set programs, grouped
calendar/derived and owned-predicate projection, broader join forms, composite
reviewed relationship authoring and all required dialect/engine qualification
remain explicit recovery work. Unit parser cases are not actual
engine result evidence. Synthetic Commerce acceptance checks independently fixed
regional totals and net revenue through real PostgreSQL/native boundaries with
recorded provider responses, not live model quality or production-owner parity.


### MySQL physical-key increment

MySQL 8.4 discovers complete visible-column BTREE uniqueness, excluding prefix and
expression indexes. A zero-row access opens each registered base table inside the
same read-only transaction before catalog inspection; its metadata lock stays held
through the actual read. Real-engine tests block concurrent index DDL during that
transaction, then reject stale key evidence after it closes. Legacy no-key bindings
retain their original policy and gain no join authority implicitly. The ordinary
native statement/dependency/permission checks remain mandatory.

MySQL text-index uniqueness is not analytical evidence until collation identity and
comparison coercibility are sealed. A full text index is excluded alongside prefix
and expression indexes; numerical physical-key paths remain supported.
