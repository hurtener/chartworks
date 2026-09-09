# Output specifications v1

Phase 20 produces portable drawing input, not rendered pixels, SQL or a persisted
chart object. `internal/charts` is deterministic standard-library code;
`internal/chartservice` applies current Pengui authority and optional existing
Bifrost rank assistance. `internal/chartapi` and `sdk/chartworks` consume those same
services. The [operation manifest](chartworks-chart-operations.json) is checked
against the actual registry, which also generates `/openapi.json`.

## Authority and data boundary

Every operation requires its exact `charts.read`, `charts.select` or `charts.bind`
action **and** `cw.tenant.read:<signed-tenant>`. No request accepts a tenant, actor,
DSN, source handle, SQL, credential, formatter or remote resource location. These
operations transform **caller-supplied** bounded data; column provenance is
metadata, not proof of source access or reviewed meaning. Supplying a source/topic
identifier never causes a lookup, validates a permission, certifies a semantic
model, or grants the right to query that resource. The caller must obtain data
through the separately authorized source/NLQ/BYO path. Future block/report owners
must resolve exact definitions and data dependencies under their own authority.

The service independently enforces the signed tenant/action before processing;
HTTP denies unauthorized calls before decoding. HTTP decoding and in-process work
have bounded, nonqueued admission. A definition is not an execution capability and
is never implicitly made current after schema drift. Nothing is stored or logged
as chart data, SQL, model prompt or result content. These operations have no domain
mutation audit; an attempted optional inference has an explicit usage receipt.

## Operations and public SDK

| Operation | HTTP | SDK | Behavior |
|---|---|---|---|
| Catalog | `GET /v1/charts/catalog` | `ChartCatalog` | Fourteen kinds, required/optional slots, negative/null policies, configured limits. |
| Selection | `POST /v1/charts/select` | `SelectChart` | Deterministic suitable candidates; explicitly requested optional remote reordering. |
| Explicit authoring | `POST /v1/charts/specify` | `SpecifyChart` | Bind one chosen kind and return its output; unsuitable kinds are rejected, not replaced. |
| Saved output | `POST /v1/charts/build` | `BuildChart` | Exact saved definition applied to compatible supplied data; no selector/model/source call. |
| Authoring rebind | `POST /v1/charts/rebind` | `RebindChart` | Detached `review_required` proposal; never approval, persistence or mutation. |

All structures are closed lower-snake-case JSON. Required scalars are non-null;
Go nil collections are permitted where the DTO has a collection. Every cell has
`null` and `value`; a null cell must have `value:""`. `{null:false,value:""}` is a
real empty text value, not missing data. Binary values use even-length hexadecimal
text; structured values contain valid JSON text (not an executable object). `ChartDataFromReadResult` preserves the
qualified reader's exact string encodings, booleans, nulls and numeric tokens,
without a `float64` intermediary. It does not infer units, additive aggregation,
source authority or semantic approval. `DefaultChartOptions` supplies explicit
safe options for authoring.

```go
// client already has a trusted server URL and current Pengui TokenProvider.
// report comes from ExecuteRead / RunNLQ / the caller's explicit BYO step.
if report.Result == nil { return errors.New("read did not return data") }
catalog, err := client.ChartCatalog(ctx)
if err != nil { return err }
data, err := chartworks.ChartDataFromReadResult(ctx, *report.Result, catalog.Limits)
if err != nil { return err }
selection, err := client.SelectChart(ctx, chartworks.ChartSelectRequest{Data: data})
if err != nil { return err }
output, err := client.BuildChart(ctx, chartworks.ChartBuildRequest{
    Data: data, Mapping: selection.Selection.Selected.Mapping,
})
// output.Output is declarative input for a renderer, not image bytes.
```

## Catalog, selection and shapes

The fourteen distinct kinds are `area`, `bar`, `column`, `donut`, `grouped_bar`,
`heatmap`, `kpi`, `line`, `pie`, `scatter`, `stacked_bar`, `stacked_column`, `table`
and `treemap`. `kpi` is the KPI-card wire spelling. Selection uses stable first-fit
qualified roles, deterministic scores and catalog tie order. It returns at most
`max_alternatives` in addition to the primary, excluding every candidate below
`selection_floor`. A labeled `table_fallback` is used only when no chart qualifies;
it is not counted as implementation of any other kind. An explicitly requested
kind never silently falls back. The frozen mapping includes its exact column pins,
slot order, formatting and explicit sort definition.

Categorical plots require unique category/series cells rather than inventing an
aggregation. Grouped and stacked plots require a separate series column. Heatmaps
require two categorical axes and a numeric value; their category bound covers the
combined axis labels. Scatter preserves repeated observations. Line/area require
parseable temporal categories, detect identical instants across different offsets,
and retain null-value gaps. Treemap supports a flat or one-parent-level hierarchy;
leaf/parent self-links, duplicate leaves in one parent, negative parts and excessive
series are rejected. Pie/donut/treemap reject negatives and report
`no_positive_values` for nonempty all-zero values. KPI has at most one row. Empty
inputs produce `empty`, without invented zeroes. Null coordinates omitted from
other plots are reported by count and warning; table nulls remain explicit.

Text labels are literal UTF-8, not HTML or code. Full long labels survive; a
renderer may use `label_max_runes` only as a visible layout hint and must retain
accessible exact labels. Column order follows bound IDs, not original data order.
Explicit ordering is stable and exact-numeric aware, with nulls last.

## Exactness and completeness

Integer/decimal labels remain strings. Numeric geometry alone may approximate to
finite `float64`, with per-value `approximate` and a visible aggregate warning;
underflow/overflow is unsuitable for geometry, while a table can retain the exact
value. Currency/unit/fraction-digit and percentage hints never rewrite exact
labels. Percent `fraction` means 0.1 represents 10%; `whole` means 10 represents
10%. Date/time grain, aggregation, role and versioned provenance remain portable.

Totals use exact rational arithmetic over the **supplied result**, only for
explicit `sum`/`count` numeric columns without percentage formatting. Unknown,
average, minimum, maximum and distinct-count aggregations are not silently summed.
Totals include supplied rows even when null chart coordinates prevent plotting;
`omitted_rows` remains visible. Renderers must label the total's scope rather than
presenting it as a total of plotted points. `complete_result` describes the full
supplied query result, not the entire underlying source. Truncated results retain
the reason, warn `truncated_result_not_full_source`, and mark every total
`returned_rows`. All-null totals are null, never a fabricated zero bill/value.

## Rebinding and immutable meaning

A saved build compares every bound column with the exact stored metadata, including
source and topic revisions, type, role, grain, aggregation, format and name. Drift
returns `409 mapping_changed`. Explicit rebind may associate one and only one
same-meaning column using stable topic/semantic identity and the same source,
type/role/grain/aggregation/format. No fuzzy name matching or ranker is used.
Changed revision/name/ID pins appear in a detached proposal; ambiguity, missing
columns and incompatible units/types are rejected. Even an unchanged proposal is
`review_required`. Only a later owner-controlled authoring workflow may approve and
persist it. Future block revisions own saved definitions; phase 20 adds no store.

## Optional ranking and failure receipts

Ranking requires `rank:true`, `charts.rank_enabled:true`, an enabled existing
Bifrost gateway/`visual_rank` role and more than one suitable candidate. The ranker
receives only explicitly supplied bounded author intent and kind/rule descriptors,
not rows, labels, source/topic coordinates or saved definitions. Schema-constrained
Bifrost output can permute only the sealed suitable set; it cannot introduce a
kind/binding or change deterministic suitability scores. Unknown/duplicate/missing
IDs, nonfinite scores, unavailable models, warnings, exhausted reservations and
rank timeouts preserve the deterministic result with visible provenance.

`ranking` is `not_requested`, `disabled`, `not_applicable`, `rules_preserved`,
`invalid_response` or `gateway_ranked`. Reserved calls/tokens and maximums are
separate from observed `receipt.calls`; missing provider usage/cost remains unknown.
An interrupted or expired request never returns candidates after authority ends,
but an already attempted call's receipt accompanies the safe error. The SDK exposes
that bounded metadata as `StatusError.Receipt`; `Error()` never echoes server
content. Build, specify and rebind have no optional rank field and make zero calls
at every inference seam. No warehouse is queried to draw a chart.

## Bounds and HTTP outcomes

The [configuration reference](../configuration.md) and
[mergeable excerpt](../../examples/chartworks.charts.json) define every limit. HTTP
JSON bodies have an additional 10 MiB ceiling, independently of encoded data and
closed option limits. Final serialized output is at most four times the configured
data byte cap (at most 32 MiB). The SDK uses a 40 MiB response ceiling for that
bounded envelope, with smaller catalog/proposal caps. There are no query parameters,
cookies, alternate media types or content encodings for these routes. GET bodies
are rejected. Context/authority and operation/rank deadlines cannot be extended.

Safe outcomes are 400 invalid input, 401 missing/expired bearer, 403 missing action,
404 inaccessible tenant or unregistered path, empty 405 method mismatch, 409 saved
mapping drift, 413 size/bounds, 422 unsuitable binding, 429 admission saturation,
503 unavailable dependency and 504 cancellation/timeout. Literal errors do not echo
source data. The composed registry guard blocks an unregistered protected handler
before dispatch; resource enforcement remains the responsibility of actual services.

## Verification boundary

`TestPhase20/AC01`–`AC06` exercise the six owning criteria, 84 static kind/behavior
goldens, all five HTTP/SDK operations, saved-map drift, numeric exactness, rank
failures/receipt honesty, recorded actual Bifrost usage and admission/cancellation.
`TestChartsFromQualifiedReadExecution` additionally exercises actual PostgreSQL
validated reads through HTTP/SDK into chart/table outputs, with no source reaccess.
`TestPhase21` and the cumulative guard test enumerate all implemented registrations.
These are specification and recorded integration tests, not phase-31/32 pixel
parity, live-provider quality, deployed Pengui configuration or full-release proof.
