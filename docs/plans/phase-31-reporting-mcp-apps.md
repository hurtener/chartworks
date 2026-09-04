# Phase 31 — reporting-mcp-apps

Status: planned. Owner: internal/mcpserver, internal/reporting, web/report-viewer. Hard dependencies: 22, 23, 28, 29, 30.

## Authority and design

RFC-002 §8, `docs/reporting/delivery.md`, D-046/D-047 and [COMMON.md](COMMON.md) apply. Harbor and Pengui support MCP Apps end to end. This phase builds Chartworks' tools/resources/viewer; it does not requalify either host.

## Brief findings incorporated

Briefs 06, 13, 14: portable output contracts, selected artifact consumption, safe UI metadata and useful structured results. Historical middleware/library discussion is background, not a new framework-selection project.

## Findings I'm departing from

Remove the former compatibility spike, transcript/host qualification and unrelated core-protocol upgrade gates. Keep ordinary functional tests for the new Chartworks code. No bearer in a UI resource, tool argument or browser storage.

## Scope and implementation tasks

1. Add reporting_search/describe/run/runs/view over the same API/domain contracts and selected published outputs; supply the bundled Apps UI resource.
2. Build a small shared viewer for blocks/reports/dashboard pages with filters, output selection, pagination, locale/theme and clear run/approval/freshness/error states.
3. Use the established host bridge for tool calls and authorized data; keep UI resources tenant-data-free and mutations behind explicit scope/intent.

The view tool advertises `_meta.ui.resourceUri` and serves versioned bundled Apps HTML with safe resource metadata/CSP. Tool outputs carry bounded result/manifest references; assets do not embed tenant data. The standard bridge invokes the same authorized provider functions. Query-changing filters are explicit run actions; local redraw/paging of retained data must not invoke generation or SQL. Text/data fallbacks remain useful without visual rendering.

## Non-goals

No host compatibility project, standalone builder, second authorization protocol, custom token bridge or general-purpose mutation tool exposed by default.

## Config and persistence

Viewer output/row/message bounds, allowed theme tokens and resource version; no embed secrets or host compatibility feature flags. Persist no duplicate report state in the viewer. UI assets are versioned and public-data-free; sensitive outputs remain under ordinary Pengui JWT enforcement via the host/BFF path.

## Acceptance criteria

1. **AC01** — Tools advertise the actual UI resource and standard resource metadata/MIME/CSP; this tests Chartworks output, not Harbor/Pengui compatibility.
2. **AC02** — Metadata search/describe omit unauthorized resources and SQL; run is explicitly side-effecting while view/runs never execute data/model work.
3. **AC03** — Viewer handles artifact references, loading/error/partial/expired/private states and output/page navigation without a shared bearer in code or arguments.
4. **AC04** — Every chart/KPI/table/narrative kind displays exact values/units/order and trust state; local redraw does not rerun a query.
5. **AC05** — A business-filter change requiring data creates an explicit authorized run; app visibility hints cannot bypass provider scope checks.
6. **AC06** — Theme/locale/resize/accessibility and bounded table paging work through the existing bridge with functional component/provider fixtures.
7. **AC07** — API/MCP/SDK reporting result/error/selection schemas agree; clients without Apps still receive useful bounded data/text.
8. **AC08** — No compatibility spike, framework-selection checkpoint, host requalification transcript or mandated core-protocol migration blocks delivery.

## Tests, coverage and smoke

Implement `TestPhase31/AC01` through `TestPhase31/AC08` with provider tool/resource tests plus a real viewer/component test command invoked by acceptance. Exercise the existing bridge fixture contract and all output kinds, private/partial/expired states, filters and zero-query reads. AC08 is a scope/documentation check, not a replacement for functional tests. COMMON.md sets coverage; `scripts/smoke/phase-31.sh` requires all eight results.

## Glossary, decisions and deviations

MCP Apps support is an established dependency. D-046 is binding. No runtime completion of Chartworks' new app is claimed by this plan.
