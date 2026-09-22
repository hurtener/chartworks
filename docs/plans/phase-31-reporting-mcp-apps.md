# Phase 31 — reporting-mcp-apps

Status: in_progress. Owner: internal/mcpserver, internal/reporting, web/report-viewer. Hard dependencies: 22, 23, 28, 29.

## Authority and design

RFC-002 §8, `docs/reporting/delivery.md`, D-046/D-047/D-054 and [COMMON.md](COMMON.md) apply. Harbor and Pengui support MCP Apps end to end. Build Chartworks' tools/resources/viewer, not a host qualification project. Viewing does not depend on phase 30 scheduling; later scheduled artifacts use the same result contract.

## Brief findings incorporated

Briefs 06, 13 and 14: portable outputs, selected artifact consumption, safe UI metadata and useful structured results. Historical library/middleware discussion is background, not a framework-selection checkpoint.

## Findings I'm departing from

No compatibility spike, host transcript requirement or unrelated protocol upgrade. Remove the accidental schedule dependency. Test real Chartworks behavior rather than spending a runtime acceptance criterion merely proving that a compatibility task is absent.

## Scope and implementation tasks

1. Register reporting_search/describe/run/runs/view over the shared domain/API and selected published outputs; supply the versioned bundled Apps resource.
2. Build the shared small read viewer for block/report/dashboard outputs, filters, pagination, locale/theme and run/approval/freshness/error states.
3. Use the established host bridge; keep resources free of credentials and tenant values. A data-changing filter explicitly invokes an authorized run; retained-result pagination/redraw makes no query/model call.
4. Add content escaping, message/data size limits and component/provider tests. The viewer stores no authoritative duplicate report state.

The view tool advertises `_meta.ui.resourceUri`, uses the supported Apps HTML MIME and resource CSP/permission metadata, and receives bounded authorized manifest/result references. UI visibility hints are not authorization. Scheduled provenance appears as ordinary catalog metadata after phase 30; no new viewer-specific schedule engine is introduced.

## Non-goals

No host compatibility project, full builder, second authorization protocol, token bridge, local inference or general-purpose mutation tool exposed by default.

## Config and persistence

Viewer row/message/output bounds, constrained theme tokens and resource version. No embed secrets or host qualification flags. UI assets are public-data-free; private data remains under Pengui-issued authority through the existing bridge/BFF. No inference credentials are delivered to the viewer, and it never calls Bifrost directly.

## Acceptance criteria

1. **AC01** — Tools advertise actual UI resources with standard metadata/MIME/CSP; tests validate Chartworks declarations, not established host compatibility.
2. **AC02** — Search/describe omit unauthorized resources and SQL; run is explicitly side-effecting while view/runs make zero data/model executions.
3. **AC03** — Artifact loading/error/partial/expired/private states and output/page navigation work without shared bearers in resource code, arguments or storage.
4. **AC04** — Every supported chart/KPI/table/narrative displays exact labels/units/order and trust state; redraw does not re-execute a query.
5. **AC05** — Data-changing filters create explicit authorized runs; app visibility cannot bypass provider-side signed scope checks.
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
bounded static/export slice described by [D-078](../decisions/2026-09-22-rich-output-display.md).
