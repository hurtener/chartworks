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

The private predecessor's client-facing report builder is the primary experience
benchmark. Capability migration does not require carrying over an analyst or
operator interface. Readers see the report, filters, refresh, sharing and history;
authors choose data, fields and chart styles directly. Internal operations,
identifiers, digests, provenance terminology and authority machinery do not belong
in the ordinary workflow. Keep meaningful access, privacy, incomplete-data,
expiry and uncertain-action explanations in plain language. This is a product
experience requirement, not a lexical check or permission to weaken enforcement.

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
- [x] FA02: versioned typed field compiler supports multi-dimension/multi-measure
  aggregates, count, raw rows and explicit date grouping. Exact physical schema,
  safe columns, current source/context and configured column limits are checked
  before source work; incompatible types/policies fail with useful dispositions.
- [x] FA03: authorized table/upload and topic discovery use existing public
  services; no mandatory topic creation and no extra access-management service.
- [ ] FA04: searchable field UI exposes types, physical versus reviewed meaning,
  selected order, aggregation and date controls; no hidden business defaults.
  Both transports use the same implementation and deliberate prepare lifecycle.
- [x] FA05: old definitions, omitted-field canonical hashes, original preparation
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
- [ ] FA09: reader, Builder, chart setup, publication and Pengui sharing use
  client-facing language and a clear action hierarchy. Technical references are
  absent from the ordinary experience; status and recovery still explain what
  happened, what is safe to do next, and whether existing readers are affected.
  Preserve existing English/Spanish support and keyboard/screen-reader labels.
- [ ] FA10: the signed-in walkthrough demonstrates a composed report with
  meaningful charts, a summary value, a table and editable text, using synthetic
  data selected through the same schema-driven workflow. Show authoring and
  reading on desktop/mobile and both transports; a table-only fixture or a
  renderer catalog is insufficient evidence of the complete client experience.

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
the author-to-reader-to-amendment walkthrough. The analytical error-registration
finding from Phase23/AC01 is resolved in the physical-filter UI checkpoint below.

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
23-hour calendar day, private amendment and compacted replay. The shared
Builder/Consumer now has typed value/set/range controls with explicit calendar
and timezone policy, staged Done/Cancel behavior and exact numeric entry.
JavaScript/resource contracts and 396 browser assertions per transport pass,
including desktop/mobile physical controls. Physical option search and signed-in
host evidence are still open.
The NLQ/BYO error registry now includes the analytical refusals already emitted
by handlers. The shared surface fixture preserves the question's actual reviewed
aggregation instead of substituting raw rows. The full `internal/nlqapi` suite and
Phase22/Phase23 AC01–AC03 pass against local PostgreSQL/native execution. This
closes the recorded analytical error-registration finding; audience work remains
open. No analytical guard was loosened.
No FA criterion closes from this checkpoint alone.

## Physical option search checkpoint

D-109 completes explicit physical option search in the same native service and
shared controls. Safe text/UUID/integer/numeric/boolean fields advertise lookup;
temporal fields retain ranges. Both raw and reviewed dataset origins resolve
actual registered field identity. Private and published report filters derive
that same identity from their complete saved bindings. Native types govern exact
search and keyset ordering, with exact strings at the browser boundary.

Migration 090 preserves legacy request/record bytes, actual topic foreign keys,
source revision and original native receipt fences. Pengui's closed host carrier
accepts exclusive physical selectors and checks source-only manifests without
inventing topic grants. Unknown lookups retain original custody. Typing, selecting,
Done and Cancel perform no source work; explicit Search/Next does.

Local qualification includes physical text/empty/SQL-looking strings, booleans,
integers, exact decimal ordering, UUIDs, floats and NULL exclusion through actual
PostgreSQL; private/published full-field populations; denied/stale reach and
non-reexecuting replay. Existing cancellation, receipt/rule fences, retention and
dependency checks pass with the race detector. The full reporting/API suites pass.
Shared resource tests and 404 Chromium assertions per HTTP iframe/MCP transport
pass, with desktop/mobile snapshots inspected. These browser hosts are synthetic.
Pengui host race tests, 32 embedded-host tests and Svelte check also pass.

FA03/FA04 still require uploaded-data and actual signed-in host acceptance.
Independent people/Team view/edit/publish/manage authority, dependency eligibility,
revocation/amendment and the final author-to-reader walkthrough remain open under
FA06/FA07. Hosted CI remains billing-blocked, not green. No deployment or merge.

## Independent permission and sharing checkpoint

D-110 implements native read-only publication review: exact read intersects
write/publish reach before worklist projection; report opening and lifecycle
inspection do not lend write authority to publishers. Save/rebind/review and
execution remain independently gated. Actual PostgreSQL lifecycle acceptance
publishes as a separate reviewer with no write/execute/source-query permission;
negative cases retain data context and private custody. Both HTTP/MCP registration
and the shared UI support the publisher-only lane.

The Pengui companion implements people/Team sharing, full dependency preflight,
canonical grant contributions, safe revocation and independent permissions under
PD-253/migration 0110. Local three-store, HTTP and UI tests are distinct from
signed-in proof. Self-review corrected a delayed dialog-response race on report
switches and added explicit search/preflight/mutation/reload regressions.
FA06/FA07/FA08 remain open for complete signed-in author/Team/amendment journeys,
uploaded data, current desktop/mobile walkthrough, exact-head gates and cleanup.

## Uploaded-data API journey checkpoint

Two synthetic CSV files with unrelated physical schemas were reserved, staged
and loaded through the real engineering service into isolated PostgreSQL. The
running reference binary discovered those uploaded tables without topics or
predefined measures. An authenticated author selected three groups and three
measures, including explicit zoned daily grouping, prepared/validated the chart,
and submitted a report for a separate publisher. A viewer-admitted reviewer
published without write or source-query permission. A Team-only reader explicitly
ran the report and read six exact retained rows. This sequence passed through
both HTTP authority/BFF APIs and restricted no-chat MCP with the current resource.

An amendment added the second uploaded schema. HTTP preflight reported missing
source/dataset/context authority before sharing; it granted no data permissions.
The old retained result remained readable while the new publication was hidden.
After explicit administrative data prerequisites, the manager rechecked sharing
and the Team ran both pages. Revoking the report contribution denied an existing
reader session. MCP repeated amendment/publication/refresh/revocation and verified
that removing one report's contribution preserved a chart shared by another report.
`TestReportAppUploadedFieldsNative` covers the real upload/typed preparation/private
consumption/preview seam without models or caller-supplied schema.

These are authenticated API journeys, not live browser acceptance. The browser
certificate hand-off, desktop/mobile visual review, standalone walkthrough,
exact-source final gates and owned-service cleanup remain open. Local fixture
setup retained the original source address because execution-context fingerprints
correctly reject a changed warehouse location. Hosted CI is still billing-blocked.

## Precision review correction

Offline inspection of the actual compiled UI over captured uploaded results found
that new decimal measures inherited zero fraction digits. D-111 changes new typed
numeric selections to preserve exact retained precision until explicit formatting.
Legacy definitions remain unchanged; explicit zero and reset have distinct native
and browser semantics. The offline fixture contains no credentials or live relay
and does not substitute for signed-in browser acceptance.

## Current local qualification, 2026-10-09

FA02/FA03/FA05 now have native and authenticated public-seam evidence: actual
uploads, typed multi-field selection, legacy preparation compatibility, immutable
publication and independent reviewer/Team amendment journeys. The final numeric
default is also exercised by newly prepared HTTP and MCP reports. Native charts,
static rendering, full reporting/API race suites and focused real-PostgreSQL
acceptance pass. Shared resource/module tests and planning/docs checks pass.

Desktop and 390-pixel mobile inspection used the actual compiled resource with
captured public synthetic DTOs in a read-only offline host. Six fields, ordered
multi-measure selection, explicit day/timezone controls and exact retained values
were inspected. This exposed and corrected decimal rounding and compressed mobile
table columns; tables now scroll within their container. This is visual evidence,
not an authenticated browser journey. The local certificate warning requires a
human browser hand-off. FA04/FA06/FA07/FA08 remain open for final host visual
qualification, delivery/cleanup and any concrete issues it exposes. No hosted CI,
production deployment or merge is claimed.


## Client experience checkpoint

The reader, chart setup, formatting, publication and recovery controls now use
plain language. Readers get a visible Refresh report action, filters/history
before the canvas, and optional details instead of raw status codes, revision
coordinates or validation identifiers. Data warnings, draft privacy, original
numeric values and uncertain-action fences remain. Pengui sharing explains data
prerequisites and separately granted access in English and Spanish. Field choices
remain schema-driven; useful source field names still distinguish display labels.

A composed synthetic report was created through the public services, published
by an independent reviewer, shared with a Team and read through both HTTP and
restricted MCP. It contains text, a summary number, bars, a daily line and a table
on two pages. This is authenticated API evidence, not signed-in visual acceptance.
The date chart exposed PostgreSQL's abbreviated whole-hour timezone offsets; the
temporal parser now accepts those actual source values without rewriting retained
cells. Native line/area regressions and browser-formatting checks cover those
offsets and the daylight-saving boundary.

Local module/compiled-resource checks, chart race tests, Pengui sharing tests and
the console build pass. Browser fixtures have updated expectations but the new
visual pass remains open: Chrome displayed ERR_BLOCKED_BY_CLIENT when reopening
the app. The owner was asked to restore the browser page. FA09/FA10, desktop/mobile
visual sign-off, the refreshed walkthrough and final service cleanup remain open;
this checkpoint does not establish predecessor visual parity or goal completion.
