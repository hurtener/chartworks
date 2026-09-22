# Output specifications v1, v2 and v3

This is the current shared chart contract. Its filename remains stable for older
links. `internal/charts` owns deterministic binding, suitability and retained
transformation; `internal/chartservice`, the closed HTTP API, public SDK, saved
reporting definitions and the current Apps viewer consume that same contract.
The [operation manifest](chartworks-chart-operations.json) is checked against the
registry that generates `/openapi.json`.

## Authority and data boundary

Every chart operation requires its exact `charts.read`, `charts.select` or
`charts.bind` action and `cw.tenant.read:<signed-tenant>`. The existing verified
identity/issuer and signed source-context boundaries are unchanged. Requests do
not accept credentials, SQL, executable formatters, remote resources or replacement
authority. Chart provenance is metadata, not proof of permission or certification.
The separately authorized query path obtains data; saved reporting publication and
execution independently validate the exact dependencies under current authority.

Standalone chart operations transform caller-supplied bounded typed results.
They neither persist a second chart-state store nor query a source to draw a chart.
Saved reporting definitions persist the same mapping in immutable revisions.
Source validation, publication, current health and current authority remain
separate facts. A frozen refresh builds the saved outputs without question
interpretation, chart selection or optional chart ranking.

## Operations, versions and closed bindings

| Operation | HTTP | SDK | Behavior |
|---|---|---|---|
| Catalog | `GET /v1/charts/catalog` | `ChartCatalog` | Fourteen kinds, separately described binding variants, mapping versions and limits. |
| Selection | `POST /v1/charts/select` | `SelectChart` | Rules use bounded intent and retained-data signals before sealing candidates. |
| Explicit authoring | `POST /v1/charts/specify` | `SpecifyChart` | Bind and build the chosen shape; unsuitable shapes do not silently become tables. |
| Saved output | `POST /v1/charts/build` | `BuildChart` | Exact saved pins and compatible data, without a source, selector or model call. |
| Authoring rebind | `POST /v1/charts/rebind` | `RebindChart` | Detached `review_required` proposal, not approval or mutation. |

Data, column provenance and old scalar mappings/outputs remain version 1. Rich
bindings and their outputs use version 2. Reviewed KPI/table display intent uses
version 3. `mapping_versions` advertises `[1,2,3]`;
`BuildVersion` separately identifies the transformation engine in frozen-result
reuse keys. A retained v1 artifact is still readable; a new engine does not
rewrite it or reuse an incompatible older build as a new result.

All structures use closed lower-snake-case JSON. A cell carries `null` and an
exact string `value`; a null requires `value:""`, while a non-null empty string is
real data. `ChartDataFromReadResult` preserves qualified read-result strings and
numeric tokens without passing exact values through floating point. It does not
infer units, additive semantics or authority.

The binding object permits scalar column IDs `category`, `value`, `series`, `x`,
`y`, `parent`, `size`, `comparison`, `target`, and ordered ID arrays `values`,
`hierarchy`, `columns` only. Comparison/target are restricted to v3 KPI.
Arrays contain column IDs, not arbitrary expressions, objects or rendering code.
The applicable variant closes the allowed combination. `value` and `values` are
exclusive; `hierarchy` replaces scalar `parent`/`category` for a rich treemap.
All used IDs are distinct, including between slots. Columns are pinned in this
canonical order: category, value, series, x, y, parent, size, values, hierarchy,
columns. Absent slots contribute nothing. Repeated-slot order is never sorted away.
Unknown fields/versions, malformed arrays, repeated IDs and mismatched version/shape
combinations reject. A rich binding cannot be relabeled v1 to drop its extra slots.

## Supported variants, not merely catalog names

The catalog is **12 plots plus KPI and table**, not a claim that every possible
variation of fourteen names exists. Its `variants` list describes the actual
supported alternatives independently from legacy scalar `required_slots`.

| Kind | Compatible v1 | Rich v2 and restrictions |
|---|---|---|
| Line / area | Temporal category + scalar value | Ordered values, or scalar value with categorical series, or values with series. Temporal category leads explicit order; default ascending. |
| Bar / column | Category + scalar value | Ordered values without a series dimension; scalar value plus real categorical series; or values plus series. |
| Grouped bar | Category + value + series | Category + at least two values without an artificial dimension, or repeated values with real series. |
| Scatter | Numeric x/y, optional categorical series | Explicit numeric size, with optional categorical series; bubble area encodes size. |
| Treemap | Category + value, optional one-level parent | Ordered hierarchy of 1–8 levels plus a declared additive value; no scalar category/parent mixed in. |
| Stacked bar / column | Category + series + one value | Existing exact tuple contract is preserved; no repeated-measure stacking extension. |
| Heatmap | Categorical x/y + value | Existing unique-cell contract; not numeric scatter axes. |
| Pie / donut | Category + value | Existing nonnegative composition contract. |
| KPI | One scalar value, at most one row | V3 selects first/last ordered value, comparison column or previous row, exact delta/percent delta, target difference, ordered thresholds and retained sparkline values. |
| Table | Ordered columns | V3 retains visibility/order, labels, page size and explicit eligible totals policy. Hidden columns may remain in saved sort intent but are omitted from retained display rows. |

All variants apply type, cardinality, null and numeric-range validation before
building. Categorical/series tuples and heatmap cells must be unambiguous even if
one duplicate has a null value. Equivalent numeric spellings and temporal instants
are the same coordinate, not a way to evade uniqueness. Duplicate leaf paths also
reject. No path silently takes the last row or performs implicit duplicate-row
aggregation. Scatter observations may repeat because they are individual points.

Negative pie/donut/treemap components are unsuitable, including negatives on rows
that another missing coordinate would otherwise omit. Existing signed comparison
and diverging stacked geometry remains supported; stacks do not become a positive
composition percentage. KPI and table retain signed values. Invalid numeric data,
nonfinite geometry and true underflow/overflow reject rather than becoming zero.

## Ordered series and retained normalization

For repeated measures, declared measure order is outermost; within each measure,
real categorical breakdowns follow first appearance after the declared stable row
order. Each series retains its measure, original breakdown cell, full name, format
and stable identity. Column ID/type/grain/unit/provenance pins remain in the mapping.
Hashed typed coordinate/series keys are separate from exact human-readable labels.

Line/area align series to the union of observed category positions. A null measure
stays null. An absent observation becomes a null gap with `row:-1`, not a fabricated
source row or zero. The current viewer breaks both kinds of gap. It uses ordered
observed positions, not elapsed-time-proportional spacing, interpolation, resampling
or inference of missing calendar buckets. Declared grain and order are exposed.

Bar/column/grouped plots normalize every declared measure and meaningful breakdown
from the already retained typed rows; no new SQL is generated. Distinct units,
currencies or percentage representations use labeled independent scale panels in
the viewer, not a misleading shared axis or an implicit conversion. Dense glyphs
stay inside their allocated slots; subpixel geometry may be invisible and is not
rounded up into a false value.

Rich output retains exact wide `rows`, canonical `columns` and original
`row_indices` separately from normalized `points`. Each observation carries a
measure, series identity, exact value and approximate drawing coordinate. The
versioned `transformation` records method, null/duplicate policy, scope and missing,
absent-gap and zero-size counts. Omitted coordinates do not erase their wide rows.
Empty or undrawable states do not erase retained evidence either.

## Bubble size and ordered hierarchy

Size is a numeric measure, not a dimension, a percentage or an implicit third
coordinate. It must be finite and nonnegative within geometry bounds. Positive
size maps to bubble area, so radius is proportional to its square root. Zero has
zero area and is explicitly counted as undrawn; null x/y/size/series also prevents
geometry without replacing a value. Exact x/y/size strings, labels and returned-row
identities remain accessible independently from approximate drawing coordinates.

Hierarchy order is explicitly root-to-leaf. Default ordering follows these levels
ascending; custom order must preserve the declared hierarchy prefix. Complete leaf
paths are unique. A declared `sum` or `count` numeric, non-percentage measure permits
named `sum` internal-node aggregation over contributing returned leaf rows.
Each node has a stable path ID, parent, depth, full path, exact value and original
contributing row indices. Prefix aggregation is not duplicate-leaf aggregation.

Null path elements and null values are omitted from tree geometry and internal
aggregation, not placed under an invented unknown category. Negative values reject.
Node scope is `returned_complete_paths`, which is intentionally different from a
total over every returned row. A truncated result cannot establish a whole-source
hierarchy. The viewer draws nested levels and exposes exact aggregate paths,
values, membership and scope; zero or subpixel regions remain truthful omissions.

## Exactness, totals and completeness

Integer/decimal labels remain exact strings. Only numeric geometry approximates
to finite floating point; `approximate` and visible warnings disclose it. Currency,
unit, percentage, fraction digits, locale, closed date pattern and currency-symbol
fallback do not rewrite stored exact values. Display labels are separate from
meaning/provenance pins. Version 3 carries these fields through immutable
definitions and retained output. Interactive and retained-only static/export
consumers apply the closed formatter vocabulary; arbitrary locale code, date
patterns, scripts and URLs reject.

Totals use exact arithmetic over the supplied result only for explicitly additive
`sum`/`count` numeric columns without percentage formatting. Average/minimum/maximum,
distinct-count and unknown aggregation are not silently summed. Totals include
returned rows whose missing chart coordinates prevent plotting. All-null totals
remain null. `complete_result` means the complete supplied query result, not an
entire source; a truncated result records its reason, warns
`truncated_result_not_full_source`, and labels totals `returned_rows`. Tree-node
`returned_complete_paths` sums never masquerade as these all-row totals.

## Saved compatibility, publication and portability disposition

Saved builds compare every bound column against exact ID/name/type/role/aggregation,
unit/currency/percent/format/grain and source/topic/semantic/revision pins. Drift is
`409 mapping_changed`, not automatic rebinding. Explicit authoring rebind requires
one unique same-meaning match with compatible source/type/role/grain/aggregation and
format; changed ID/name/revision pins appear in a detached proposal. Missing,
ambiguous or incompatible meanings reject, and even an unchanged proposal remains
`review_required`. Approving it requires the existing authoring/publication path.

Both versions round-trip through JSON, the SDK, reporting draft persistence,
immutable publication, frozen execution and actual retained delivery. There is no
new schema table, chart-specific IAM or mutable published definition. Frozen reuse
includes the builder version as well as existing revision, context, authority,
privacy, policy and execution bounds.

JSON save/read/build compatibility is implemented; a standalone portable
import/export feature is not. Any later importer must preserve ordered rich slots
and exact pins, reject unknown versions or unsupported downgrade, and resolve
current dependencies/authority through its existing governed authoring path.
Dropping a v2 slot to manufacture v1 compatibility is not an accepted migration.
Static rendering, portable export and broader display policies remain separately
owned interfaces, not completion claims from these fixtures.

## Deterministic selection before sealing

The selector classifies at most 1024 characters of supplied author intent into
closed comparison/trend/composition/relationship/bubble/hierarchy/intensity/table
cues, with bounded English/Spanish lexical handling of negation and conflicting
cues. This is not an NLQ parser or a clarification policy. Unspecified or mixed
intent preserves conservative rules rather than pretending certainty.

Before floor/alternative limits seal the set, rules inspect typed roles, additive
semantics, temporal fields and exact cardinality/null counts over retained rows.
Multi-measure candidates retain every eligible measure rather than retrying the
first measure alone. Bubble and deep-hierarchy intent can change candidate bindings
before suitability is evaluated. Composition cardinality changes relative scores;
semantic nonadditivity is explicit. Unsupported or excessive shapes reject with
bounded evidence. Column order selects otherwise ambiguous axes; unused columns
remain visible in candidate evidence, not silently claimed as represented.

Stable catalog order breaks score ties. Evidence includes rules version, classified
intent, retained row/cardinality counts, all kind admission outcomes/signals,
suitability/floor counts and tie policy. Each candidate records its variant,
rationale and unused columns. Below-floor/limited/unsuitable candidates cannot be
recovered by a ranker after sealing. Only when no plot qualifies is a labeled table
fallback used; explicit tabular intent is separately identified.

Optional ranking requires `rank:true`, enabled configuration and the existing
`visual_rank` gateway. Inputs contain only the closed classified intent and bounded
kind/rule/variant/signals, never the raw question, rows, labels, source/topic pins or
saved definitions. Ranking may permute only the sealed set. Invalid IDs/order/scores,
usage warnings, unavailable models, exhausted budgets or timeout preserve the
deterministic result with honest ranking provenance and receipts. Observed usage is
not fabricated from reservations. Specify/build/rebind and retained viewing cannot
invoke ranking. No fixture establishes live model quality or measured performance.

## Bounds, errors and actual viewer

Configured input rows/columns/bytes, category/series counts, operation/rank deadlines
and admission limits remain enforced. Rich normalization additionally caps expanded
points/nodes at 10,000 and hierarchy depth at eight; cross-products count against
series/expansion limits. Incremental output accounting checks bounded work before
large rich allocations. The existing final-output limit is four times the configured
data cap. HTTP and SDK ceilings remain independent. The Apps provider/viewer has
its own bounded message/result limits; an oversized retained shape fails honestly,
not through truncating bindings or dropping measures.

Existing status classes remain: 400 invalid input, 401 invalid bearer, 403 missing
action, 404 inaccessible/unregistered resource, 405 method mismatch, 409 mapping
drift, 413 bounds, 422 unsuitable binding, 429 saturation, 503 unavailable and 504
cancellation/timeout. Error text does not echo data. Signed authority still bounds
execution even after an optional ranking attempt.

The established Apps protocol is unchanged. Its actual component consumes v1/v2,
exposes exact wide rows, named-series observations and full hierarchy aggregates in
accessible tables with full labels. Null/zero/subpixel omissions and total scope
are visible. Local chart-table pagination, redraw and theme changes do no provider,
source or model work; retained output/page selection uses only authorized artifact
reads. Inconsistent versions, tuples or tree references fail instead of silently
falling back to a scalar rendering.

## Verification boundary

The original 84 kind/behavior goldens and `TestPhase20/AC01`–`AC06` preserve the
scalar catalog. Rich regressions cover ordered slots, exact values, missing gaps,
bubble/hierarchy semantics, negative/null/duplicate data, strict drift, safe detached
rebinding, concurrent reuse, expansion budgets and pre-seal intent/cardinality.

`TestCW02RichCharts` exercises the actual closed API/SDK and all rich fixture
bindings, then saves 13 outputs through PostgreSQL draft/read/validation/publication,
one frozen query, exact rebuild and retained Apps delivery. It checks v1 coexistence,
source-revision rejection, builder reuse identity and zero source/model work during
retained reads. The actual `TestPhase31/AC04` component suite builds the shared 22
rich synthetic variants from deserialized mappings, renders them in Chromium and
asserts geometry, every retained measure, exact values, full labels, gaps, bubble
area, three hierarchy levels, dense slot separation and local paging. AC05/AC06
retain interaction and hostile-message/authority-read regressions. CI receipts,
not this document or fixture count, determine which commands actually passed.
