# Native-proved learned parameter domains v2

Status: S10 implementation increment. Extends the existing parameterized-example
and service-owned-base contracts; it does not authorize SQL or infer a user's
future values. Qualification is recorded in the recovery completion tracker.

## Value-free domain custody

`example-parameters-v1` remains unchanged, including its public `example` text
probe, digest identity and immutable historical rows. A newly produced
`example-parameters-v2` schema adds optional `domain` to a text slot, with one of
`date`, `timestamp`, `timestamptz`, `uuid`, `time`, `timetz`, `interval`, `json`
or `jsonb`. At least one slot must have a domain;
positions remain dense and bounded to 64, and ordinary kinds remain unchanged.
There are no value, default, sample, regular-expression or caller-defined fields.
Null parameters keep their original null kind rather than becoming text domains.

The producer consumes an opaque, freshly native-validated `exec.Plan`. Its SQL,
parameter kinds and exact source binding are authenticated before parsing; retained
plan JSON cannot supply this proof. Historical parameter values never select a
domain. PostgreSQL evidence is an exact direct native cast, or a direct comparison
against a safe column of a registered schema-qualified relation or a proven cast.
Timestamp precision 0 through 6 preserves the same second-aligned input domain.
The other five dialects use the native structural companion directly, preserving
positional marker order or exact unquoted `@pN` indices, original casts and actual
safe catalog types. It does not borrow the analytical normalizer's erased cast
details. Supported mappings use these closed engine-native distinctions:

- PostgreSQL includes all nine named domains, with JSON and JSONB kept distinct;
  TIME/TIMETZ precision and native INTERVAL field/precision modifiers remain
  bound to the exact SQL, never treated as semantic-equivalence evidence
- MySQL DATE selects date; DATETIME/TIMESTAMP selects the offset-free probe;
  TIME and JSON select their matching public probes
- SQL Server DATE, DATETIME/DATETIME2/SMALLDATETIME, DATETIMEOFFSET and
  UNIQUEIDENTIFIER select date, timestamp, timestamptz and uuid respectively;
  TIME selects the offset-free time probe;
  binary TIMESTAMP/ROWVERSION never selects a temporal probe
- BigQuery DATE/TIME/DATETIME/TIMESTAMP selects date/time/timestamp/timestamptz
- Snowflake DATE, DATETIME/TIMESTAMP_NTZ, TIMESTAMP_TZ/TIMESTAMP_LTZ selects
  date/timestamp/timestamptz; TIME selects time; unqualified TIMESTAMP is a session-selected alias
  and cannot establish a specialized domain
- Databricks DATE/TIMESTAMP_NTZ/TIMESTAMP selects date/timestamp/timestamptz

These are input-probe families, not timezone/analytical equivalence certificates.
Their distinctions follow the official [SQL Server temporal types](https://learn.microsoft.com/en-us/sql/t-sql/data-types/date-and-time-types),
[BigQuery data types](https://docs.cloud.google.com/bigquery/docs/reference/standard-sql/data-types),
[Snowflake temporal types](https://docs.snowflake.com/en/sql-reference/data-types-datetime)
and [Databricks data types](https://docs.databricks.com/aws/en/sql/language-manual/sql-ref-datatypes).
Unknown/custom types, out-of-range temporal precision and ambiguous aliases do not
establish a domain. Native source planning remains independently mandatory.
BigQuery string-to-JSON requires PARSE_JSON, which is outside the current approved
function registry; JSON casts are not substituted for it. SQL Server JSON and
Databricks TIME/JSON are absent from their current source type contracts, and
Snowflake uses VARIANT rather than an equivalent JSON text cast. These existing
native/source limits are not silently widened by the learning schema. No enum or
source-specific constrained values are inferred.

Every occurrence of a numbered text slot must agree. Repeated date/timestamp
uses, a domain use mixed with an untyped expression, unknown/ambiguous source
lineage, or unsupported positional traversal cannot authenticate a new domain.
CTE/derived output names never acquire source-column domains by name resemblance;
explicit casts inside supported native CTE/derived scopes can still prove their
own slots, with original traversal order preserved.
Domain proof uses a shared two-slot PostgreSQL admission bound and the existing
shared warehouse structural-parser admission; cancellation while queued does not
consume another caller's slot.
A domain proof never grants safety to a SQL node: the existing whole-tree native
validator and source EXPLAIN must already have accepted the exact statement.
Feedback remains recordable when this conservative learning proof is unsupported.

## Public probes and independent review

Fixed validation-only text probes are:

- date: `2000-01-02`
- timestamp: `2000-01-02 03:04:05`
- timestamptz: `2000-01-02T03:04:05Z`
- uuid: `00000000-0000-4000-8000-000000000001`
- time: `03:04:05`
- timetz: `03:04:05+00:00`
- interval: `1 day`
- json: `{}`
- jsonb: `{}`

The values are public constants, not source-derived examples or defaults. Review
and protected import dry-validate with these probes, then independently recompute
all slot domains from the resulting current-source sealed plan. Exact schema
agreement is mandatory even if another domain's probe happens to parse. A caller
cannot acquire a domain by recomputing a template digest. Failed probes are never
retried with historical private bindings. This establishes admissible public
probes, not the validity of every future value, nor business-result semantics of
an implicit temporal conversion. Generation receives only domain metadata and
must still bind and validate the current request normally.

## Persistence and portability

Migration 064 extends the strict JSON constraint to the second schema version;
it does not update existing rows, origins or digests. Existing immutable schema,
SQL/question/digest and origin triggers continue to apply. New ordinary domain
templates use the `parameterized-example-v2` digest domain. The existing owned-base
digest already includes the entire schema and unchanged binding policy.

Portable rows carrying a v2 domain schema use version 4, including owned bases;
legacy portable versions 1, 2 and 3 retain their contracts. A mixed export uses the
maximum row version. Import still checks exact current source/context/topic/rule
origin, and imported examples remain candidates pending separate review. Public
SDK aliases forward the same value-free schema. Service-owned predicates remain
separate; only independent model slots gain domain metadata. The next request
resolves service-owned predicates afresh.

## Evidence and boundaries

Dedicated PostgreSQL acceptance covers all nine domains through feedback, durable
storage/restart, fixed-probe review, export/import, current-question generation, actual result IDs, immutable
origins and zero-work replay. Wrong/cross-domain assertions, schema downgrade,
conflicting repeated use and copied private literals are negative cases. Actual
MySQL tests exercise DATE, DATETIME, TIME and JSON casts plus DATETIME/TIMESTAMP
catalog input domains, fixed-probe native planning and read results under a pinned UTC session.
These are recorded-model/source tests, not live model measurements or all-engine
qualification. The SQL Server, BigQuery, Snowflake and Databricks mappings have native-AST,
recording-adapter validator and fixed-probe-schema regressions, including named
slot reordering, repeated conflicts, nested casts and exact catalog domains.
Live execution qualification for those four engines remains separate Q2 evidence;
it is not substituted by native AST acceptance. Existing SQL authority is unchanged.
