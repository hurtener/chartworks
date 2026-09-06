# Validated read execution — D-065

This extends the merged phase-09 plan contract in
[vector-sources-validation.md](vector-sources-validation.md). Phase 09 remains the
only constructor of executable plans. Phase 10 supplies one bounded, model-free
read core, not another scheduler, authentication service or general query bypass.

## Authority and surfaces

Pengui remains the sole issuer and access-policy owner. The supplied JWT must
have `sources.query` plus signed `cw.source.query`, `cw.execution_context.use` and
all resolved `cw.dataset.query` resource scopes. Operation IDs, attempt IDs,
validation receipts, SQL, actor names and context labels do not confer authority.
Every read/control request rechecks the current supplied bearer and actual source
context. A rotated/restricted context requires revalidation; an old plan or receipt
cannot select the old credential state.

The [route inventory](chartworks-read-operations.json) is checked against actual
registration. HTTP and the Go SDK delegate to the same domain services:

| Operation | Surface | Effect |
| --- | --- | --- |
| Validate and execute once | `POST /v1/sources/{id}/execute` | Native proof, bounded read and attempt journal |
| Recover a lost response's attempt ID | `GET /v1/read-operations/{id}` | Latest actor/session-scoped attempt metadata |
| Inspect an attempt | `GET /v1/read-executions/{id}` | Content-free attempt metadata |
| Request cancellation | `POST /v1/read-executions/{id}/cancel` | Durable intent; live owner signals its original connection |
| Reconcile an uncertain attempt | `POST /v1/read-executions/{id}/reconcile` | Exact backend observation, never a query replay |

The execution body is closed and requires `context`, `sql`, `parameters` and
`execution`; the latter requires `operation`, `attempt`, `preview`, `rows` and
`bytes`. Send an empty parameter array, not null. Cancellation/reconciliation take
`{}`. Duplicate/unknown/case-variant/null members, query-string overrides, encoded
paths, content encodings and an alternative Idempotency-Key header are rejected.
The Go SDK supplies empty parameter arrays and never automatically retries.

HTTP 200 means an accepted attempt has a durable receipt, **not necessarily that
the query succeeded**. Inspect `attempt.status` and `attempt.code`. Only
`succeeded`, `empty` and `truncated` have a result. Admission and authorization
failures use fixed typed HTTP errors without raw source error text. A failure to
commit the final receipt drops all values and returns an indeterminate error;
recover through the logical-operation lookup rather than blindly reissuing SQL.

## Execution and limits

`Executor.Execute` takes an opaque nonzero validator-issued Plan and explicit
Options. HTTP first validates the complete submitted SQL. Frozen/generated/BYO
consumers must use this same core; none can skip validation or raise the ceilings.
The original phase-08 `sources.Read` remains a strict-cap compatibility projection
onto this cursor implementation, not an independent query executor.

The qualified PostgreSQL 17 driver rechecks actual credentials/catalog/relation
restrictions inside a read-only transaction. It uses DECLARE/FETCH around the
exact validated statement and bound values. There is no LIMIT string rewrite,
reordered SELECT, application-side authority filter or execution-time SQL repair.
An optimizer estimate is checked before result dispatch; parsing or EXPLAIN alone
never grants execution. Every FETCH obtains its actual description rather than
reusing a prepared statement's schema from a previous cursor.

| Typed configuration | Default | Bound and meaning |
| --- | --- | --- |
| `exec.rows_default` | 10,000 | 1 through rows_ceiling |
| `exec.rows_ceiling` | 100,000 | Hard maximum for all consumers |
| `exec.preview_rows` | 200 | 1 through rows_default; cannot widen authority |
| `exec.bytes_default` | 4 MiB | At least 1 KiB, no more than bytes_ceiling |
| `exec.bytes_ceiling` | 16 MiB | Actual JSON schema+row byte ceiling |
| `exec.timeout` | 60s | 1ms through 60s; also bounded by JWT and caller deadline |
| `exec.cancel_grace` | 2s | 1ms through 3s, bounded cleanup/receipt finalization |
| `exec.execution_concurrency` | 2 | 1 through 16; bounded per process |
| `exec.max_read_attempts` | 3 | 1 through 3, explicit caller-controlled attempts |
| `exec.planner_cost_ceiling` | 10,000,000 | Positive finite optimizer units, at most 1e12 |

All settings are non-secret. Zero request rows/bytes select configured defaults;
explicit values over ceilings fail. Preview only lowers the row cap. The byte
count is the sum of JSON-encoded ordered schema and rows, including escaping,
commas and delimiters. Bounded receipt metadata is separate. PostgreSQL protocol
messages also have a fixed upper bound. A lookahead row distinguishes a complete
result from a truncated prefix; truncated subtotals are never full-source totals.
A valid empty result retains its schema and is not an error or an invitation to
broaden predicates/time windows.

Read execution has client, JWT, statement and PostgreSQL-17 transaction deadlines.
The source-revision metadata fence has a separate bounded duration covering the
read and its cleanup; ordinary metadata operations retain their short timeout.
The executor requires metadata pool capacity of execution_concurrency plus two
connections, preserving journal/control traffic while source fences are held.
The default HTTP write/client timeouts are 75s. Explicit deployment/client timeouts
must leave room for validation, execution and bounded cleanup; shorter caller
budgets deliberately take precedence. Source probe/legacy-reader timeouts remain
separate from the phase-10 execution timeout.

## Exact result contract

The ordered schema declares `name`, `type`, `encoding` and `native_type`; rows are
ordered arrays, so duplicate output labels do not overwrite values. Exact integer
and numeric/money cells are JSON strings. For example, `9007199254740993` and
`9007199254740993.125` remain exact without a float64 intermediary. Money is decoded
from native int64 minor units under the pinned C monetary locale, including the
int64 minimum. Booleans and NULL retain their JSON types. Finite float4/float8 values
use JSON numbers with the native precision. Bytea uses a hex string, temporal
values use canonical native text with UTC/ISO settings, and JSON/JSONB are encoded
as **JSON text strings**, preserving exact numbers inside the document.

Unknown result types, malformed values, invalid UTF-8 and nonfinite numbers are
explicit errors; partial earlier rows are discarded. Supporting a parsed SQL
expression does not imply that every possible returned database type is qualified.
Arrays/composite/custom outputs remain unsupported rather than silently coerced.

## Cancellation and uncertainty

Admission records a content-free manifest before result execution. The driver
journals dispatch intent with a random attempt tag, backend PID and backend start
instant before issuing DECLARE, then journals its acknowledgment. The PID is not a
credential, and backend cancellation secrets/JWTs/SQL/parameter values/result rows
are never persisted in this ledger.

Each live attempt has one bounded, joined cancellation observer. A cancellation
request persists intent first; its owner cancels its original pgx connection using
the native CancelRequest handler. External requests do **not** call
pg_cancel_backend on a possibly reused PID. Cancellation at the final commit wins
atomically and prevents values from being returned. The audit records the actual
committed status, not a pre-race success assumption.

`remote_state=stopped` requires an acknowledged rollback or observation that the
exact tagged backend transaction no longer exists. A socket close, successful
signal delivery, elapsed local timeout or missing result alone is not termination
proof. Reconciliation matches PID, backend start, random tag, source user and
database. A lost/expired owner is projected as uncertain, then can become
`interrupted` only after proof of termination. Reconciliation does not reconstruct
lost values or invent successful execution. Unsupported control remains explicitly
unsupported; unavailable observation remains unknown. Rotation or missing current
reach may prevent reconciliation and does not authorize a credential fallback.

The capability receipt states `owner_cancel_request`,
`tagged_backend_observation`, server deadlines and optimizer-estimate admission.
Actual scanned bytes are unknown (`null`); PostgreSQL does not supply a hard
scan-byte ceiling here. Response caps are not scan caps. Result retention is false.

## Logical operations, physical attempts and persistence

A logical operation is actor/tenant/session-bound and keeps the same exact
validation-manifest and effective limits. Physical attempt numbers are explicit
and monotonic. Repeating a number is rejected without another result query.
Changing SQL/parameters/context/limits under the same logical key conflicts. A
successful operation cannot silently rerun; only a failed/cancelled/timed-out or
reconciled-interrupted attempt can admit its next numbered attempt. An uncertain
attempt must be reconciled first. There is no universal exactly-once claim.

Forward migration 006 adds the read-attempt journal and its actual audit actions;
001–005 are unchanged. This is not a second leased queue. Completed content-free
receipts are retained for a bounded 24-hour replay window and cleaned at subsequent
admission. Capacity is capped at 1,000 records per actor and 10,000 per tenant;
uncertain attempts are not silently erased and may require operator attention.
The replay guarantee ends when those records expire. Persistent result retention,
artifact viewing and reporting schedule targets remain phases 28/30; this phase
registers no pretend scheduled warehouse target or stored result cache.

See [the adversarial review](../reviews/phase-09-10-adversarial.md) for executable
regressions and final verification requirements. Real PostgreSQL fixtures do not
establish cloud-engine compatibility, production deployment or model quality.
