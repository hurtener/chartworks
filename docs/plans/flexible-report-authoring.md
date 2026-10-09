# Flexible report authoring and audience management

Status: in progress, 2026-10-09. Owners: phases 08/11/15/20/27/29/31 and the
existing Pengui companion. Follows the locally qualified team-reporting pass.
This is implementation work; unchecked criteria are not shipped capability.

## Product contract

Clients choose their own data and analytical questions. Production field names,
labels, types, units, roles, calendars, aggregations and relationships come from
authorized source or semantic metadata. Never infer business meaning from a
column name, assume a sales schema, or select an analysis for the user. Business
examples belong in synthetic fixtures and demo datasets. Starting layouts are
optional, editable arrangements with no attached data or prescribed analysis.

Expose authorized loaded tables, including uploads, and topic datasets through
one schema-driven field selection experience. Distinguish physical columns from
reviewed dimensions and metrics. Physical columns offer type-compatible grouping,
aggregation and unaggregated tables; reviewed metrics retain their definitions.
Support multiple dimensions and measures within advertised execution/render
budgets, not an arbitrary analytical shape. Date grouping uses explicit grain,
calendar and timezone; a numeric year or text period is not a timestamp.

Read predecessor behavior privately and record only neutral conclusions. Reuse
schema discovery and typed intent patterns; do not port domain-specific aliases,
name heuristics, silent fallback aggregation or unverified joins.

## Delivery sequence and acceptance

- [x] FA01: audit production assumptions and compare physical/semantic field
  discovery with current predecessors; optional layouts contain neutral copy.
- [ ] FA02: versioned typed field compiler supports multi-dimension/multi-measure
  aggregates, count, raw rows and explicit date grouping. Exact physical schema,
  safe columns, current source/context and configured column limits are checked
  before source work; incompatible types/policies fail with useful dispositions.
- [ ] FA03: authorized table/upload and topic discovery use existing public
  services; no mandatory topic creation and no extra access-management service.
- [ ] FA04: searchable field UI exposes types, physical versus reviewed meaning,
  selected order, aggregation and date controls; no hidden business defaults.
  Both transports use the same implementation and deliberate prepare lifecycle.
- [ ] FA05: old definitions, omitted-field canonical hashes, original preparation
  custody, private amendments and immutable publications remain compatible.
- [ ] FA06: Pengui-owned report access management grants people/Teams independent
  view/edit/publish/manage permissions. Complete dependency preflight prevents
  accidental data grants; amendments and revocations preserve independent grants.
- [ ] FA07: real PostgreSQL author-to-reader-to-amendment acceptance through HTTP
  iframe and restricted no-chat MCP on desktop/mobile. Include at least two
  unrelated synthetic schemas and renamed identifiers; equivalent typed choices
  produce equivalent results without source-specific code changes.
- [ ] FA08: focused Go/race/browser/contract/planning gates, bounded adversarial
  review, exact-source evidence, draft PRs and standalone walkthrough are updated.
  Owned temporary services are stopped; local results are not hosted CI evidence.

## Ownership, migration and boundaries

Keep one validator-issued execution plan, existing operation custody and native
publication lifecycle. Pengui alone decides sharing and signs authority. This
pass adds no issuer, identity database, client SQL execution, silent semantic
rewrites or automatic data-access widening. Existing published reports remain
readable. New optional request fields are omitted on legacy requests to preserve
their digest. Any new durable access state requires a forward-only Pengui
migration and concurrency tests with its first consumer.

Use [COMMON.md](COMMON.md), [the preparation contract](../contracts/manual-chart-preparation-v1.md)
and [the authority contract](../contracts/pengui-authority.md). Source metadata,
report dependencies and shared data permissions remain independent of layout.
Single-agent implementation and review follow the user's explicit instruction.
Hosted CI is billing-blocked; keep companion PRs draft and unmerged. This pass
does not close phase 34 migration or phase 25 whole-product release.

## Typed-topic checkpoint

The additive compiler and metadata-driven picker are implemented. Local native
acceptance `TestReportAppTypedFieldsNative` uses two unrelated, freshly profiled
synthetic schemas and exercises preparation, replay, revoked dataset reach,
private consume and preview. It includes six selected fields, duplicate-preserving
raw rows, row count and a multi-value chart with an explicit timezone boundary.
`TestAuthoringFields*` covers renaming, physical type modifiers, schema drift,
unsafe columns, reviewed-policy refusal, calendar/type mismatches, configured
pre-execution budgets and absence of predefined measures. Original preparation
tests remain applicable. Both compiled-browser transports pass 378 checks each;
their host is synthetic, with separate desktop/mobile picker captures.

Self-review found and fixed row-count provenance pretending to be a reviewed
field, qualified native-type handling, oversized aggregate display labels,
legacy no-measure filtering and horizontal picker clipping. No independent
reviewer was used. Full host acceptance, topic-independent tables/uploads and
Pengui sharing remain unfinished; FA02–FA08 stay open until their full evidence
and lifecycle requirements are met.


## Registered-source lifecycle checkpoint

D-107 and migration 089 add topic-independent source/dataset pins to the existing
block lifecycle and metadata dependency carrier. `TestReportAppSourceDatasetNative`
uses actual PostgreSQL, schema-checked HTTP and an actor with no topic permissions. It covers metadata
reads without execution, changed origin rejection, source/dataset/context revocation,
private actor custody, exact retained aggregate values, validation/publication,
frozen execution, report composition, private amendment and cleanup/replay.
Pengui's closed authority and iframe request parsers accept the explicit source
origin and typed field carrier; mapping/copy uses source read without source write.
These tests do not establish the table picker or a signed-in host journey.

FA03 still requires usable paged table/upload discovery through the existing
public source services. Physical-field filters, audience management and the final
visual/live acceptance remain open. No criterion is closed by this checkpoint.

## Paged catalog and picker checkpoint

The source page is implemented through the existing source core and registered
HTTP/MCP/SDK surfaces. Both Pengui hosts expose bounded, policy-filtered source
and dataset catalogs with explicit continuation, including sparse pages. The
shared Builder offers Tables & uploads alongside Reviewed topics, carries the
server's source/context/revision/schema pin and uses the same physical-field
picker. Topic and source catalogs no longer silently stop at a UI item ceiling.
Catalogs and selection perform no source/model work. Denied/closed reads erase
metadata. Empty mobile canvases now fit their message and preserve space for the
controls.

This closes the implementation gap in the prior source checkpoint, but FA03/FA04
remain open until actual uploaded data and the signed-in host journeys are
qualified with physical filters. FA06/FA07 still require audience management and
the author-to-reader-to-amendment walkthrough. A broad Phase23/AC01 run uncovered
an analytical-mismatch/error-registration regression outside catalog reads; it
is retained as an open qualification finding, not a passing product gate.

## Physical-filter native checkpoint

D-108 adds explicit physical column parameters to the typed compiler, shared
binder and report compatibility checks. Exact numeric values, text/UUID/boolean
choices and half-open numeric/date/timestamp ranges use bound values. Temporal
input has explicit calendar/zone semantics and rejects DST gaps/folds. Both raw
source and reviewed-dataset physical columns are checked against exact registered
schema identity. Saved defaults and temporary report selections share one core.

Native acceptance covers an author without topic permissions, schema-checked
HTTP preparation, private custody, malformed/reference/authority rejection,
validation/publication, exact frozen results, report filter overrides across a
23-hour calendar day, private amendment and compacted replay. Physical option
search, Builder/Consumer controls and signed-in host evidence are still open.
The Phase23 analytical error registration finding and audience work remain open.
No FA criterion closes from this checkpoint alone.
