# Revision-bound report filter options v1

REP-01 is implemented as an explicit governed source read. A report filter may
carry a version-one option source that pins an exact block revision, topic
publication, dataset and reviewed semantic column. The source must belong to an
exact block revision used by the same immutable report revision. Dimension
parameters additionally require an exact topic/version/dimension reference whose
reviewed field is that same dataset and column. Display labels,
parameter names and physical names supplied by clients never select relations or
columns.

`POST /v1/reports/{id}/filter-options` and the shared
`reporting_filter_options` MCP/viewer operation require `reporting.execute`, exact
report read reach, `sources.query`, source query reach, context use reach, topic
read reach, block read reach and dataset query reach. The domain service resolves
the current source binding, active exact topic publication and safe physical
column before constructing a single-column `SELECT DISTINCT`. Closed builders
cover PostgreSQL, MySQL, SQL Server, BigQuery, Snowflake and Databricks with
connector-specific qualification, quoting, placeholders, literal LIKE escape,
NULL-last keysets and row-limit syntax. The ordinary
validator issues the only executable plan and the ordinary read executor applies
read-only credentials, planner admission, deadlines, row/byte caps, cancellation
and attempt receipts. No model is called and no SQL, values or credentials enter
ordinary audit output.

Each request returns at most 200 exact normalized JSON values and bounded labels.
Integers and decimals retain string encoding, booleans retain JSON booleans and
NULL remains JSON null. Binary and structured values are explicitly unsupported.
Search is bounded to reviewed string columns, uses an escaped bound parameter and
starts a new keyset sequence. Database collation controls ordering; the requested
locale controls presentation of NULL only and is part of the cursor identity.
Values exceed the cursor-safe 1,024-byte scalar ceiling fail with a typed budget
error before a page is exposed. Any row/byte-truncated source result also fails
with a budget outcome and can never be labeled complete.

Continuation cursors are opaque HMAC-authenticated process-local envelopes with a
five-minute lifetime. They bind tenant, user, session, complete signed scope set,
report, immutable revision, filter, search, limit, locale, source revision, value
type and last key. Tamper, expiry, restart, authority change, source revision drift
or changed request coordinates fail closed before execution. Chartworks does not
cache option values: every page repeats current authority, semantic and source
revision checks. This avoids tenant-only or stale value reuse.

The document route is registered only when validated source execution is mounted,
matching the delivery/MCP capability boundary. Import/export needs no parallel
mapping. The option binding is part of the
existing immutable document definition JSON and therefore survives the ordinary
versioned document import/export path. Historical definitions without the
optional field retain their original behavior and do not acquire a source read.
