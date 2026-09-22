# Rich output display and retained static export v1

This contract closes VIS-01 and VIS-03 for native Chartworks definitions and
retained consumers. The core structures are chart mapping/output version 3.

## Authored intent

KPI definitions bind one numeric value and may bind a numeric comparison and target.
They choose the first or last ordered row, no comparison, the previous row, or the
comparison column. Delta, percent delta, target difference, sparkline and threshold
states are explicit independent choices. Derived values use exact decimal arithmetic;
division by zero yields no percent delta rather than infinity or a fabricated zero.
Thresholds are evaluated in reviewed order against exact value text.

Table definitions bind ordered columns. Every bound column has one visibility entry,
at least one is visible, page size is 1–1000, and totals are explicit. Only visible,
eligible additive columns produce totals. The presentation page size narrows retained
view paging and never expands artifact/source limits.

Every column may carry a human display label, bounded locale tag, one closed date
pattern (`date_short`, `date_medium`, `date_long`, `datetime_short`, `year_month`),
0–20 fraction digits, currency code and currency-symbol fallback. The exact stored
cell is never replaced by formatted text. Formatter code, arbitrary date patterns,
URLs and scripts are not fields in the contract.

## Persistence and compatibility

Immutable block revisions already store chart mappings as JSON and retained outputs
store the matching typed chart payload. Version 3 display fields therefore join the
existing definition, manifest, output and reuse digests. Frozen execution applies
the saved mapping to the one normalized retained result; it does not interpret a
question, choose a chart, query a source or call a model.

Migration 043 validates version-3 KPI/table shape and bounded formatter fields on
new block revisions. It does not rewrite v1/v2 publications. Unknown mapping versions,
missing visible table columns, unbound KPI comparison/target fields and executable or
unbounded formatter values reject. Foreign mappings remain Phase 34 work.

## Consumers

The bundled Apps viewer renders exact rich KPI fields and the authored table policy.
Its closed formatter consumes fraction digits, locale/date pattern, currency fallback,
unit and percent semantics. It treats all labels and cells as inert text.

`POST /v1/reporting/export`, MCP `reporting_export`, the Go SDK and the generic CLI
expose retained-only JSON, CSV, static HTML and static SVG. The service first requires
both `reporting.read` and `reporting.export`, plus `cw.run.export:<run>`, then delegates
the retained read to the normal artifact authority path. CSV neutralizes spreadsheet
formula prefixes in headers and cells, including when Unicode BOM, bidi marks or
control characters precede the formula. HTML permits only its generated inline style
under CSP; all other content and network sources are denied. HTML/SVG escape data,
carry no client JavaScript and make no network, source or model call. Date/time fields
interpret offset-bearing timestamps in the sealed report timezone; naive dates and
date-times remain wall-clock values.

Every rendition distinguishes the full retained-output digest from a projection
digest over the exact output window, page bounds, locale and timezone. Table exports
report offset, limit, total, continuation, truncation, completeness and warnings.
Static HTML also prints returned-row totals with their declared scope and the visible
page range. A missing continuation for an incomplete table is rejected rather than
presented as a complete export. Static KPI HTML and SVG include the retained value,
comparison, delta, percent delta, target, target difference, threshold and sparkline
when present. Static chart values use the same closed display formatter as tables and
KPIs. Theme and viewport dimensions affect the generated HTML/SVG bytes.

The current static slice renders tables and KPI as HTML, chart/KPI as SVG, and tables
as CSV. Full report/dashboard layout composition, durable rendition rows, PDF, worker
isolation and BFF/embed integration remain Phase 32 work and are not implied here.
