# Shared read-SQL vocabulary v1

Status: AP-07A in PR #62. The registry is implemented; qualification is recorded
against exact source/CI results, not inferred from this document's test inventory.
This is a necessary name gate, not complete parser support or business approval.

## One owner and two concrete consumers

`internal/exec/sqlpolicy` owns the current PostgreSQL function/type/operator/value
name gates and the existing conservative warehouse function gate. The PostgreSQL
whole-tree resolver and warehouse validator read those constants directly. SQL
creation and both validation/execution correction use a detached JSON snapshot of
the same registry in server-owned system instructions. No positive list is widened.

Profiles map only the actual admitted source dialects: postgres, mysql, sqlserver,
bigquery, snowflake and databricks. The SQL Server native parser alias remains tsql.
Human aliases and unknown dialects are not silently accepted. PostgreSQL parsed
names retain their case rules and only the existing unqualified/pg_catalog namespace;
warehouse inspector names remain case-insensitive with no foreign qualification.
Parameter-style metadata is checked against the existing binder's exact marker
emission and parsing. This slice does not replace the binder or source driver.

The profile's version and deterministic digest identify the complete, sorted name
snapshot including dialect/parser identity and parameter style. All returned slices
are independent; editing a profile cannot change runtime validation. The registry
has no question, source identifiers, private values, credentials or authority.
The error for an unknown dialect is closed, not an echo of untrusted input.

## What the profile does and does not claim

`function_names` are native name-gate entries, not guarantees about signatures,
implicit coercions, output types, use with windows, or the entire database grammar.
PostgreSQL cast names are canonical AST type spellings. Special expressions such
as COALESCE and NULLIF do not share the ordinary FuncCall node; the snapshot names
them separately and native parser tests exercise them. Value keywords are derived
from the existing allowed SQLValueFunction opcodes, not connection/user functions.

The warehouse snapshot deliberately advertises only its established function-name
subset. Missing cast/operator/special-form lists mean unadvertised, not unrestricted
support. A dialect-specific function outside the old list remains rejected, even if
the warehouse itself implements it. Per-dialect live support is not established by
this registry or by a mocked source EXPLAIN boundary.

Source types, dependency resolution, whole-tree native safety, current signed
access, governed predicates and scoped analytical conformance can all narrow these
names further. An advertised AVG cannot replace a selected SUM; an advertised window
function cannot bypass a single-base analytical contract. The profile never produces
an executable plan, invokes a source or declares broad dialect parity.

## Effective requests, compatibility and privacy

The NLQ service derives the profile from its admitted physical binding, not request
text, runtime model preferences or learned demonstrations. The same policy goes to
sqlgen and sqlfix before full effective-envelope fitting. It is mandatory system
material, not an optional retrieval/example item; when it cannot fit with required
context, generation fails without dispatch rather than appending it after admission.
The gateway's existing estimated-envelope digest covers the effective system string.
No new model role, parallel inference call, response schema or retry budget is added.

No database migration, analytical policy version or public operation changes.
Stored SQL, parameters, explanation text, retained proof versions and frozen refresh
behavior are not rewritten. Terminal replay does not call a model to rediscover a
vocabulary. Old serialized evidence is not retroactively labeled as having received
this guidance. Existing validation still rechecks its current safety policy before
new execution as it did before this refactor.

## Regression boundary

Frozen pre-refactor name lists in tests are independent oracles. Registry tests
cover all six dialects, sorted exact snapshots, namespace/case behavior, unknown and
oversized inputs, digest integrity, detached state, concurrency and fuzzed name
membership. Native tests exercise every advertised PostgreSQL function with one
valid form, special expressions, foreign functions/columns/types and real warehouse
parser admission with a controlled EXPLAIN adapter. They do not prove all argument
combinations or warehouse server execution.

Recorded-provider/PostgreSQL acceptance checks EN/ES requests, invalid-function
correction, exact source IDs and decimals, private predicate grounding, full retained
scope and zero-work terminal replay. A separate budget test proves removing the
vocabulary would fit while the complete mandatory request fails before dispatch.
The existing fit fixture reserves the vocabulary's exact JSON-encoded overhead;
its original optional-data budget and pruning assertions remain unchanged.

Remaining AP-07 work includes a broader positively proven syntax/capability matrix,
per-engine signatures/types and analytical qualification, live owner cohorts and
safe admission of additional native functions. This shared vocabulary is not a
substitute for any of those gates.
