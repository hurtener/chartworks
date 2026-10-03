# Grouped-fact owned-base learning: local qualification

Date: 2026-10-03. Tested implementation:
`e2c8dd4a3cb3ad5c737d676dcf00977651660b5f`, tree
`2fc733c43944c8b20ef065693b3ca10b43e330a4`, based on `18342a3`.
This note is evidence-only; composed, hosted and live qualification remain separate.

## Missing-case and producer-custody REDs

- The new policy unit failed on the original closed registry: schema 5 returned
  an empty policy instead of its distinct value-free policy.
- The native lifecycle was run with the original production policy/applicability/
  retrieval files overlaid unchanged, apart from a compile-only constant. Both
  schema-5 cases passed original Plan/Run and their independent row oracles, then
  failed because authenticated positive feedback produced zero examples.
- The first enabled producer admitted an ordinal-only analytical-receipt mutation
  in all five grouped-fact acceptance variants. Existing Run rejected it. The
  final producer now replays the authenticated contract and compares the complete
  freshly native-proved receipt before learning; those assertions were retained.

## Exact-source executed checks

Go 1.27.1, restored native parser and real PostgreSQL 17 were used with serial
package builds, GOMAXPROCS=2 and GOMEMLIMIT=768MiB. Each database run started and
stopped its own server. Source hashes, index tree and status matched before/after.

- Native acceptance: 86 passing test/subtest events, zero failures/skips,
  117.287 seconds. Includes all scoped requalification families, both new schema-5
  cases, source-rotation fences, all five grouped-fact cases and the twelve-case
  three/four-fact direct/composite/calendar matrix. This run did not use `-race`.
- Full `internal/nlqexec` race suite: 536 passing events, zero failures, 19.529
  seconds. The existing optional `TestSQLRecoveryMySQLCompletenessLocal` skipped
  because its DSN was unset; it is not PostgreSQL/schema-5 qualification.
- Planning coherence and diff whitespace checks passed. Planning is not runtime
  evidence. Evidence bundles: producer RED `W0GDPD`, ordinal RED `ckNPFp`, native
  GREEN `Ks02Vj`, unit/race GREEN `UiWCsC`.

## Demonstrated boundary

The public lifecycle covers feedback, separate activation, current-origin retrieval,
actual rendered model use, independently expected SUM/COUNT/NULL/empty rows, fresh
fact values and periods/final groups, portable v3 candidate import and v5
requalification/import. It rejects stale publication, forged/downgraded policies,
changed reviewed COUNT and join semantics, foreign actor/session/context/tenant,
source rotation and altered binder/native receipts. Original origins remain
immutable, and denied/retained paths assert no model or result-row execution.

The policy introduces no migration, historical values/defaults, budget increase,
LEFT/shared-fact support or schema-6 learning. Recorded provider responses prove
prompt inclusion and deterministic lifecycle behavior, not live-model improvement.
See [owned learning](../contracts/owned-example-learning-v1.md) and
[grouped fact predicates](../contracts/analytical-grouped-fact-predicates-v1.md).
