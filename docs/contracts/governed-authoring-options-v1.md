# Governed authoring option lookup, version 1

This bounded continuation of [manual chart preparation](manual-chart-preparation-v1.md)
shares one native option service across HTTP, MCP and the typed Go SDK. It adds
explicit option searches for authorized physical/reviewed dataset authoring and exact saved
version-three report pages. It does not change the established
[public filter-option API](report-filter-options-v1.md), expand semantic support,
or make retained report reads execute source queries. Runtime qualification and
browser integration remain separate from this transport contract.

## Exact operations and authority

The four closed POST routes use `/v1/reporting/authoring/v1/<suffix>`; their MCP
names are `reporting_authoring_<suffix>_v1` and match the HTTP operation IDs.

| Suffix | Primary action | Effect | Typed SDK method |
|---|---|---|---|
| `dataset_options` | `reporting.validate` | `bounded_source_read_option_values` | `SearchManualDatasetOptions` |
| `report_options` | `reporting.execute` | `bounded_source_read_option_values` | `SearchReportFilterOptions` |
| `option_status` | `reporting.read` | `retained_metadata_read` | `ReadReportAppOptionStatus` |
| `option_control` | `sources.query` | `existing_source_attempt_control` | `ControlReportAppOptions` |

The primary action is only the transport gate. Every domain method independently
requires current exact target and dependency authority. Dataset targets contain
an already allocated `new_block` and logical `dataset`. They select exactly one
reviewed `dimension` or physical catalog `column` ID, and exactly one immutable
`topic` or registered `source_dataset` pin. Raw sources require `column`; reviewed
topics permit either selector. Pins contain source/context/dataset/revision/schema
coordinates, never connection strings or client-provided schemas. They require
`charts.bind`, tenant read/write, new-block read/write/preview/validate and exact
source/dataset/context reach. Topic origins additionally require topic read/write;
raw origins require source read and no topic authority. Query remains independent
source authority. The client cannot allocate authority by choosing a new ID.

Report targets contain `policy`, `report`, `revision`, `digest`, `page` and
`filter`. `private_preview` requires the current private draft, original actor
eligibility and report/block read/preview/execute; `published` requires exact
current published coordinates and report/block read/execute. Both require every
current actual topic/source/dataset/context dependency; source-only reports have
no topic dependency. The server derives the
option population from the saved page's actual filter-to-parameter bindings.
Report write alone does not authorize either option read. Status and control keep
the original actor/session/tenant/target custody and current dependency checks.

Exactly one target kind is valid. Closed schemas reject client SQL, physical
relation names, physical column names, connection bindings, rows, credentials,
identity, grants and
undeclared predicates. The domain validates target-kind exclusivity, immutable
pins, bounds and native permissions. The only control actions are `cancel` and
`reconcile`; control is not a query-start or retry API.

## Explicit Search and loss recovery

1. Stage search text and selected values locally. Typing, opening a selector,
   changing a selection, applying a filter, redrawing retained output and changing
   pages never initiate an option lookup.
2. On explicit Search, supply an operation key of the form
   `option:<UnixSeconds>:<32 lowercase hex>`. Seconds are canonical decimal UTC
   Unix time. A newly admitted key must be no more than five minutes old or
   thirty seconds ahead of server time. Use fresh randomness for a new operation.
3. Preserve the exact target, key and request while that operation is unresolved.
   The immutable input digest includes search, cursor, limit and locale. An exact
   replay inspects original custody without another source read; changed input
   under the same key conflicts. A continuation page is another explicit search
   using the returned opaque cursor and a fresh operation key.
4. After a missing response or unknown outcome, inspect `option_status` with the
   original target and key. Status performs retained metadata inspection only,
   without native planning, query, cancellation or reconstruction of option values.
5. If needed, explicitly cancel or reconcile the original attempt through
   `option_control`. A disconnect does not establish that source work stopped.
   Control may contact the source control lane and mutate operation custody, but
   never starts a replacement query.
6. A lost-value operation allows another separately explicit search only when
   `new_operation_allowed` is true. Never convert that flag into an automatic
   requery. Unresolved liability for the same actor and target survives session
   changes; a new operation key, session, search, dimension or revision cannot
   bypass it. Retain the original operation for recovery.

The response distinguishes three materially different outcomes:

- `values_available=true`, `options=[]`, `complete=true`: a genuine empty result.
- `values_available=false`, `code=result_not_retained`: no choices are available
  from custody; this is not “No matches.” Completed status/replays do not retain
  or recreate values. Inspect `new_operation_allowed` before a new explicit search.
- An active/uncertain operation or `execution_outcome_unknown`: source liability
  remains unresolved. False `new_operation_allowed` prohibits a replacement read.

`status`, `code`, `execution_status`, `remote_state`, `source_revision` and the
operation/input digest remain typed metadata. An empty options array alone says
nothing about whether values were observed. Unsupported semantic policies remain
explicit dispositions, never fabricated options or a broader query fallback.

## Bounded native execution and transport

The governed subset is PostgreSQL with supported direct reviewed text dimensions
and safe physical text, UUID, integer, numeric and boolean columns (D-109).
Physical catalog type and exact registered relation digest are checked again at
resolution and admission. Date/timestamp fields use explicit range controls and
do not advertise discrete option lookup. The service performs a bounded validated
distinct read over the full exact authorized population, excluding NULL, without implicitly applying
other staged chart/filter selections. Limits are 1–199 returned options and
256 UTF-8 search bytes. One sentinel row fits inside the unchanged 200-row native
preview cap; option execution stays within 512 KiB. Text search is an escaped bound
substring pattern; other physical types use a bound exact typed value. Empty search
browses the field. Every physical option is transported as an exact string,
including integer/decimal/boolean values. Native typed ordering and cursor bind
values prevent lexicographic numeric paging or browser rounding. Cursor,
locale, authority, source revision and resolution pins are checked in the shared
domain; callers treat continuation cursors as opaque. No model work or arbitrary
client expression is introduced.

The source operations are mutation-capable, paid/open-world for MCP annotation
purposes: they reserve custody and can incur source cost. They audit
`authoring.option_reserved` / `authoring.option_finished` plus native read events.
Control audits `authoring.option_control` plus native control events and is
mutation-capable/open-world. Status is read-only/idempotent metadata with no
new domain audit. Ordinary audit never contains search text, cursor payload, SQL,
option values or bearer credentials.

All four HTTP operations retain `Replay=never`; typed SDK methods issue exactly
one request and preserve caller keys. MCP uses the registered structured error
inventory and conservative `not_started` / `unknown` outcomes. Shared errors
include invalid request, forbidden/not-found, conflict/stale, expired, budget,
busy, unavailable and cancelled/timed-out without native source diagnostics.
Transport errors never authorize another query.

The assembled inventory is 91 default tools and 96 with all five optional
rendition tools. The 96-tool registry ceiling, schema-size limits, 10 MiB MCP
request cap, 16 MiB response cap, 16 concurrent calls, 65-second MCP deadline and
existing HTTP/SDK/source execution limits are unchanged. Registration tests check
both complete factory inventories and strict HTTP/MCP schema/action/error/audit/
effect parity; typed SDK tests check exact request/state round trips and no retry.

## Physical origin custody (D-109)

Migration 090 preserves legacy record/request bytes and topic foreign keys while
allowing an exclusive physical origin. It constrains column coordinates to the
record source/context/dataset/revision and retains the complete column pin in
protected operation custody. The source journal trigger still requires the exact
validator receipt, original actor/session, deadline, current source revision and
report/block fences before issuing a read. Topic publication/rule locks apply to
actual topic origins. No synthetic topic is created for a registered table.

Dependency discovery derives source-only or reviewed reach from the same origin
or original custody. Pengui checks complete manifests, bounds and fresh policy;
raw dataset manifests cannot add another source, dataset, context or any topic.
Shared controls stage selections until Done, preserve defaults on Cancel and
validate returned physical values before accepting them into UI custody. An
invalid or unconfirmed response is never an empty successful result or an
instruction to replay the source query. Local browser fixtures are separate from
signed-in host and independent audience-management qualification.
