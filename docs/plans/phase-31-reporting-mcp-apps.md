# Phase 31 — reporting-mcp-apps

Status: in_progress. Owner: internal/mcpserver, internal/reporting, web/report-viewer. Hard dependencies: 22, 23, 28, 29.

## Authority and design

[D-107](../decisions/2026-10-09-source-dataset-blocks.md) adds an exact registered
source/dataset origin without topic creation. Migration 089 retains native custody;
the active flexible-authoring pass still owns discovery and host qualification.

The active [flexible authoring pass](flexible-report-authoring.md) owns the shared
schema-driven picker for both transports. D-106 preserves explicit preparation
and retained custody. Compiled-browser controls and real host acceptance are
separate evidence; the pass does not close whole-product migration or release.

RFC-002 §8, `docs/reporting/delivery.md`, D-046/D-047/D-054 and [COMMON.md](COMMON.md) apply. Harbor and Pengui support MCP Apps end to end. Build Chartworks' tools/resources/viewer, not a host qualification project. Viewing does not depend on phase 30 scheduling; later scheduled artifacts use the same result contract.

## Brief findings incorporated

Briefs 06, 13 and 14: portable outputs, selected artifact consumption, safe UI metadata and useful structured results. Historical library/middleware discussion is background, not a framework-selection checkpoint.

## Findings I'm departing from

No compatibility spike, host transcript requirement or unrelated protocol upgrade. Remove the accidental schedule dependency. Test real Chartworks behavior rather than spending a runtime acceptance criterion merely proving that a compatibility task is absent.

## Scope and implementation tasks

1. Register reporting_search/describe/run/runs/view/filter_options over the shared domain/API and selected published outputs; supply the versioned bundled Apps resource.
2. Build the shared small read viewer for block/report/dashboard outputs, filters, pagination, locale/theme and run/approval/freshness/error states.
3. Use the established host bridge; keep resources free of credentials and tenant values. A data-changing filter explicitly invokes an authorized run; retained-result pagination/redraw makes no query/model call.
4. Add content escaping, message/data size limits and component/provider tests. The viewer stores no authoritative duplicate report state.

The view tool advertises `_meta.ui.resourceUri`, uses the supported Apps HTML MIME and resource CSP/permission metadata, and receives bounded authorized manifest/result references. UI visibility hints are not authorization. Scheduled provenance appears as ordinary catalog metadata after phase 30; no new viewer-specific schedule engine is introduced.

AC01 also requires `ui.visibility` for the actual manual-report callback set,
including authoring, topic description and chart catalog. The complete factory
inventory test covers this dispatch metadata without attaching duplicate HTML or
exposing unrelated query tools. Real local both-mode evidence is recorded in
[the 2026-10-07 qualification](../reviews/manual-app-two-mode-2026-10-07.md).

## Non-goals

No host compatibility project, unrestricted builder, second authorization protocol, token bridge, local inference or general-purpose mutation tool exposed by default. D-096 adds a separately versioned optional manual application and bounded authoring tools; the existing viewer contract stays read-only.

## Config and persistence

Viewer row/message/output bounds, constrained theme tokens and resource version. No embed secrets or host qualification flags. UI assets are public-data-free; private data remains under Pengui-issued authority through the existing bridge/BFF. No inference credentials are delivered to the viewer, and it never calls Bifrost directly.

## Acceptance criteria

1. **AC01** — Tools advertise actual UI resources with standard metadata/MIME/CSP; tests validate Chartworks declarations, not established host compatibility.
2. **AC02** — Search/describe omit unauthorized resources and SQL; run is explicitly side-effecting while view/runs make zero data/model executions.
3. **AC03** — Artifact loading/error/partial/expired/private states and output/page navigation work without shared bearers in resource code, arguments or storage.
4. **AC04** — Every supported chart/KPI/table/narrative displays exact labels/units/order and trust state; redraw does not re-execute a query.
5. **AC05** — Revision-bound filter choices use an explicit bounded authorized source read, while data-changing filters create explicit authorized runs; app visibility cannot bypass provider-side signed scope checks.
6. **AC06** — Theme/locale/resize/accessibility and bounded paging work through established bridge component/provider fixtures.
7. **AC07** — API/MCP/SDK result/error/selection schemas agree; structured/text fallback remains useful and interactive results work without a scheduler.
8. **AC08** — Untrusted data/labels/narratives cannot inject scripts, remote resources or credentials; resource/message/output limits and safe errors are enforced in the actual viewer/provider implementation.

## Tests, coverage and smoke

Implement `TestPhase31/AC01` through `TestPhase31/AC08` with real viewer component tests invoked by acceptance plus provider tool/resource tests. Exercise all output kinds, injected labels, long/oversized data, private/expired results, filter actions and repeated zero-query reads. No host qualification is required. COMMON.md sets coverage; `scripts/smoke/phase-31.sh` requires all eight results.

## Glossary, decisions and deviations

MCP Apps is an established dependency. D-046/D-054 remove host qualification and the accidental schedule prerequisite; phase 30 later supplies scheduled results without a viewer rewrite. The implementation and concrete representation choices are documented in
[reporting delivery v1](../contracts/reporting-delivery-v1.md). All eight named
criteria have executable real-consumer tests. Exact-source results and review
findings are recorded in the [adversarial ledger](../reviews/phase-30-31-adversarial.md);
status remains in_progress pending review/merge, not a planned-phase skip.

## CW-02 retained rich shapes, 2026-09-16

The current viewer consumes the [shared v1/v2 chart contract](../contracts/chart-specifications-v1.md):
ordered multi-measure/category-series lines and bars, independently labeled unit
scales, area-sized bubbles and ordered deep hierarchies. Exact wide rows, series
identities, original row provenance and hierarchy aggregates remain accessible
when geometry omits null, zero or subpixel values. Retained paging/redraw does no
source or model work. The 22 rich synthetic cases are separate from the fourteen
kind count and are exercised by the actual `TestPhase31/AC04` browser component;
`TestCW02RichCharts` also asserts persistence/publication/execution/delivery effects.
No new framework, host protocol, standalone builder or dashboard-grid redesign is
introduced, and the broader phase status/acceptance ownership is unchanged.

## CW-10 selectable filter options

The viewer/provider exposes `reporting_filter_options` only when validated source
execution is mounted. Its closed request contains report/revision/filter/search/
cursor/limit/locale, never SQL or a physical relation. D-082 binds each page to
current signed reach and source revision; retained view/redraw remains source-free.

## CW-03 output-intent and evidence-policy continuation

AC02/AC03/AC05/AC08: localized display-ordered selectors expose selected/omitted/disabled metadata independently of selected data; retained navigation and explicit filter runs preserve accepted selection order and caps; actual browser assertions verify both.

Use [D-073](../decisions/2026-09-16-reporting-output-policies.md) and the
[v2 field-level contract](../contracts/reporting-output-intent-v2.md). The
[scoped adversarial record](../reviews/cw-03-adversarial.md) links real PostgreSQL,
source execution, provider-fixture and browser checks. Keep the existing named
phase criteria and phase status; this assignment closes only its three owned gap
entries, not the whole phase, other reporting work, or full migration/release.

## CW-05 display consumer continuation

AC04/AC06/AC08: the Apps viewer consumes v3 KPI/table output and applies retained
display labels, fraction digits, locale/date and currency fallback as inert text.
KPI comparison, target, threshold and sparkline evidence is shown without source or
model work. The closed formatter and actual retained shapes are shared with the
bounded static/export slice described by [D-079](../decisions/2026-09-22-rich-output-display.md).

## Visual authoring continuation

[D-097](../decisions/2026-10-03-visual-chart-authoring.md) requires direct-grid, chart/field and genuine private-page authoring beyond D-096. Preserve existing criteria and historical evidence; qualify new domain, HTTP/MCP/SDK and browser paths separately. Scope approval does not change phase status or make unsupported controls available.

## Consumable authoring documentation continuation

AC01/AC07/AC08 also cover the bounded full-text authoring resources described in
[the public workflow guide](../contracts/report-authoring-guide-v1.md). The existing
guide tool retains its compact index and gains typed immutable document references;
no tool count or execution/transport cap changes. Maintained public contracts are
embedded directly, with exact versioned URI dispatch, digest/source references,
native read authority, HTTP/MCP shared-core parity and no tenant/source/model data.
Focused mounted-wire and immutable-catalog tests establish this resource slice;
they do not replace the phase's domain/browser/deployed-host acceptance evidence.

## Native dependency discovery continuation

AC02/AC07/AC08 include the [D-098 BFF metadata seam](../contracts/report-dependencies-v1.md), its native complete-reference projection and private/tenant/input negatives. `TestReportAppDependencyDiscovery` and `TestReportAppDependenciesResolveCurrentPublishedPin` are the focused acceptance tests. The metadata route is HTTP-only control plane, so the App tool inventory is unchanged. This does not close the full Pengui Builder or restricted MCP journeys.

AC02/AC07/AC08 additionally include D-099 proposed-write discovery and
`TestReportAppWriteDependencyDiscovery`: native create/save consumption, removed
baseline requirements, private custody and closed HTTP inputs. This remains a
BFF metadata route outside the MCP/App tool inventory; no new migration or
phase-completion claim is introduced.

AC02/AC07/AC08 also cover D-100's native publication/preparation requirement
projection via `TestReportAppDataDependencyDiscovery`, preserving exact original
custody, full unselected-dataset dependencies and zero source/model metadata work.
This HTTP control-plane consumer changes no MCP tool inventory or framework gate.

AC02/AC07/AC08 additionally cover D-101: saved validation/preview dependencies,
original private run custody, and fresh retained read projection. The named
`TestReportAppEffectDependencyDiscovery` is required for this implementation.
Discovery is metadata only; existing native effects retain all safety/CAS checks.
No migration is required. Restricted no-chat MCP and full published Consumer
acceptance remain open; local source evidence does not close those gates.

AC02/AC07/AC08 also cover D-102 published execution and original block/report run
dependencies, including the independent-reader subtest of
`TestReportAppEffectDependencyDiscovery`. Exact summary selection and bounded
BFF-only candidate discovery preserve private custody and source/model counters.
Real-service Consumer and restricted MCP qualification remain separate gates.

The local real-service HTTP Consumer gate now has separate-reader evidence in
[the integration review](../reviews/pengui-app-integration-2026-10-05.md): canonical
issuer, actual PostgreSQL, original context withdrawal and retained iframe reads.
Restricted no-chat MCP and the overall phase/integration gates remain open.


## Service connection discovery — D-104

The existing Pengui-signed service connection bearer with exactly
`capability:connect` may initialize/ping and list static MCP tool, resource and
template descriptors for enabled services. It cannot read resources (including
App HTML) or invoke tools. Interactive App resource/tool requests still require
`mcp.use`, their exact projected domain scopes and complete native resource reach.
No service name alone authorizes a request; the verifier, configured MCP audience,
expiry and exact scope profile remain mandatory. No new issuer, credential,
identity table, domain endpoint or migration is introduced.

## Manual application product-quality continuation

[The bounded quality plan](manual-report-product-quality.md) owns compact host
composition, retained-result opening, published return/edit/republish, metadata-only
chart status checks, retained formatting previews and responsive reading. Saved
column/row/span coordinates remain authoritative. Builder rows stay exactly 80px;
reading rows use that minimum and expand for content, keeping disclosures and table
rows visible without nested card scrolling. This is a screen-reading adjustment,
not a rewrite of saved definitions or portable export geometry.


## Team-ready reporting capacity

[D-105](../decisions/2026-10-09-reporting-capacity.md) coordinates bounded
exact provider authority and a 512 KiB self-contained App ceiling. The small
read viewer remains bounded at 256 KiB. The [active product pass](team-ready-reporting.md)
tracks twelve-chart/two-page runtime and browser qualification separately.

AC02/AC07/AC08 additionally consume D-110 independent report review and the
Pengui parent sharing method. `TestReportAppManualPublicationLifecycle` includes
an independent publisher with no edit/execute scopes; HTTP/MCP registry tests
and `publication-controls.test.mjs` cover native admission and read-only controls.
The flexible-authoring plan owns full signed-in sharing and visual qualification.
