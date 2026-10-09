# Typed field authoring, version 3

Implementation is in progress under [the flexible authoring plan](../plans/flexible-report-authoring.md).
This additive compiler uses the existing dataset/preparation/consume operations;
the legacy v1/v2 request and retained hashes remain unchanged when `fields` is
omitted. It does not introduce SQL supplied by the browser or model execution.

## Data contract

The authorized dataset projection adds `fields`, containing a compiler ID,
availability, the configured `max_columns`, physical column metadata and reviewed
dimension metadata. Unsupported fields remain visible with reasons. A physical
column has its stable ID, actual source name, display label, native type,
category, nullability, allowed aggregations and date grains. These come from the
immutable topic dataset matched against the current source/context binding.
Metadata reads do not query warehouse rows. No name-based aliases or type guesses
are used. The legacy `supported` flag still describes the legacy compiler;
`fields.supported` describes the new branch independently.

`intent.fields` selects `aggregate` or `rows`, an ordered `dimensions` list and
an ordered `measures` list. Legacy `dimensions` must be empty and `measure` blank
when this object is present, so conflicting interpretations cannot be admitted.

- A grouping explicitly names a `column` or reviewed `dimension`. Optional
  `grain`, `calendar` and `timezone` carry date intent.
- A measure explicitly names a physical `column` and an advertised aggregation,
  a reviewed `measure` with no aggregation override, or `count` with no field.
- `count` means `count(*)`; a column count excludes nulls. Numeric columns expose
  sum, average, minimum, maximum, count and distinct count. Other supported scalar
  columns expose count and distinct count. Unsupported aggregates are refused,
  not replaced with another calculation.
- Row count retains source provenance only; it does not invent a reviewed topic
  field. Physical and reviewed selections retain their real published identities.
- `rows` projects selected columns into a table without aggregation, grouping,
  implied distinctness or a hidden measure. It preserves duplicate rows. Normal
  execution/result bounds apply; truncated preparation cannot be consumed.

The configured reporting column/SQL limits apply before source execution, beneath
the existing absolute 256-column schema ceiling. This is a resource budget, not a
business-analysis limit. Table columns bind in selected order; chart category,
series and multiple-value slots use the existing renderer contracts. Positional
output aliases are `group_1` and `value_1` families. Display labels and provenance
retain selected identities. Observed native schema supplies output types.

## Dates and retained custody

Only actual date/timestamp types offer calendar grouping. A numeric year or
text-encoded month is not coerced. The implemented calendar is Gregorian; weeks
start Monday under PostgreSQL's calendar truncation. Civil dates/timestamps have
no timezone conversion. Instant timestamps require an explicit IANA timezone,
passed to native three-argument truncation. A reviewed temporal policy constrains
the selected calendar, grain and timezone and cannot be overridden silently.
Unknown calendar/timezone/type combinations fail with an explicit disposition.

Preparation keeps the original actor/session/target/input digest and current
topic/source/context fences. Active reviewed rules, mandatory dimension/measure
filters and other unsupported semantic policies retain explicit refusals. Choosing
a physical column is an explicit raw-field operation, never an assertion that it
implements a similarly named reviewed metric. No implicit joins or formula
inference are added. Existing typed reviewed filters retain their own contract.

The UI stages selections without source work. Prepare, consume, validate, save,
preview and publish remain separate lifecycle operations. Changing a field after
preparation requires a new deliberate preparation; replay inspects original
custody and cannot run again. New fields are optional in the existing JSON record,
so this compiler branch requires no storage migration.

## Qualification boundary

Unit compiler/client checks, real PostgreSQL preparation/consume/preview and
both compiled-browser transports pass locally. Native cases cover two unrelated
synthetic schemas, six selected outputs, raw-row duplicate preservation, row
count, a multi-value chart and explicit timezone calendar boundaries. The browser
host is synthetic; this is not signed-in host acceptance. Host journeys remain
tracked separately in the owning plan. Topic-free loaded
table/upload discovery and Pengui audience management remain unfinished work in
that plan; this document does not claim those paths are already available.

The Pengui companion must accept the optional `intent.fields` carrier in its
closed preparation request. It treats this object as Chartworks input and derives
no scopes from it; native dependency discovery and original custody remain the
authority source. A legacy host parser may reject the new request until updated.

## Registered table origin

D-107 adds optional `source_dataset` to dataset metadata requests, typed intents,
block definitions and safe block projections. Its fields are `source`, `context`,
`dataset`, `source_revision` and `schema_digest`. The source catalog supplies the
relation digest; the service checks it against the actual retained binding.
`topic` must be empty and `dataset` must match the pin. This branch requires
`fields`; it cannot use the legacy predefined-measure compiler. The physical
catalog excludes unsafe columns and contains no invented reviewed dimensions or
measures. Physical provenance contains only its actual source/revision.

Version-two block definitions select exactly one origin. Source-backed definitions
have no topics, rules, templates or reviewed amount declarations. They retain one
registered dataset and its safe columns within the existing validator. Mutation,
validation, publication, frozen execution and report composition reuse the native
lifecycle. A private amendment does not alter the previous publication.

The source parent requires source read, independently of block create/edit/publish
authority; it never requires or grants source write. Dataset query and execution
context use remain required references. Query effects additionally require source
query authority. Dependency discovery returns only these coordinates. Preparation
lookup derives them from original actor/session/target custody, including after
SQL-bearing preparation cleanup; it cannot reinterpret a supplied replacement pin.

Migration 089 makes the parent exclusive and immutable, preserves tenant foreign
keys and the guarded native admission receipt, and retains the source pin in the
compact consumed receipt. New source definitions change execution identity; legacy
omitted fields and preparation records retain their hashes. A registered revision
or relation digest mismatch is stale and cannot silently rebase a definition.

`TestReportAppSourceDatasetNative` qualifies metadata-only discovery, exact native
aggregate execution, revoked data access, actor isolation, validation/publication,
report composition, a private amendment and compacted Create replay using an actor
without topic permissions. Companion tests exercise exclusive origins and missing
or withdrawn source/dataset reach. Discovery UI, physical-field filters and both
signed-in host journeys are still pending; this is a native lifecycle checkpoint.

## Source and dataset discovery

The Builder offers reviewed topics and registered tables/uploads. The latter do
not require a topic. `POST /v1/sources/list`, MCP `list_source_page`, and SDK
`ListSourcePage` accept `after` and `limit` (1–32), returning secret-free `items`
and an explicit optional `next`. Current source reach and byte-ordered cursors
apply in storage before the limit. The original source list remains compatible.
Existing dataset list/describe operations supply the exact registered context,
revision and schema digest; selection sends this `source_dataset` pin to the
native authoring metadata operation. It sends neither a client schema nor SQL.

Both Pengui host transports compose these catalogs under current exact policy.
Each page visits at most eight permitted roots. A sparse or empty visible page
can still have a next cursor. Source names additionally require the source's
current execution context; dataset metadata requires source/context/dataset
reach. The final policy/session fence suppresses withdrawn or late responses.
No catalog operation queries a warehouse, resolves source credentials, invokes
a model, grants data access or creates a semantic topic.

The UI keeps bounded pages, offers previous/next navigation, checks origin and
revision before showing fields, and erases catalog metadata on denied reads or
closure. Legacy hosts retain the topic entry point. The first physical field or
aggregate is always an explicit author choice; display names have no analytical
meaning. Physical filters and independent audience management remain open in the
[active plan](../plans/flexible-report-authoring.md).
