# Reporting delivery implementation

Status: current design, 2026-09-04. Phases 31 and 32 own delivery. Harbor/Pengui MCP Apps compatibility is established by the owner; remove the earlier compatibility qualification, protocol-upgrade and framework re-evaluation work.

## Shared contract, separate adapters

A sealed authorized artifact feeds the versioned presentation contract. API data, the Apps viewer, a BFF-backed iframe and static HTML/SVG use it. The viewer/renderer cannot choose source tables, generate SQL or fetch new warehouse data simply because it opens. A data-changing filter creates an explicit authorized run; local sorting/redraw is local.

Build a small Svelte/TypeScript or framework-light read viewer, not a full builder application. Share output interpretation, exact labels/units, theme/locale, errors, current business trust and partial/freshness indicators. An unsupported chart fallback is visible and does not satisfy chart parity.

## MCP Apps work to implement

Register `reporting_search`, `reporting_describe`, `reporting_run`, `reporting_runs`, `reporting_view` against the existing core. The view tool advertises `_meta.ui.resourceUri`, for example `ui://chartworks/report-viewer/v1`. Serve the bundled UI resource using the supported Apps HTML MIME and resource `_meta.ui` CSP/permissions. These are Chartworks tool/resource outputs, not reasons to re-test host compatibility. [M1][M2]

Keep versioned UI assets free of tenant values and credentials. Supply bounded manifest/result references via the existing host bridge and use its tool call facility. No shared bearer appears in HTML, tool arguments or localStorage. UI visibility and tool annotations are not authorization. Mark run creation as side-effecting; pure history/view operations do not execute SQL/models. Return useful structured/text output as well.

Functional tests cover new tool schemas and results, safe resource metadata, viewer output/paging/filter/error/private/expired states, locale/theme/resize/accessibility and calls to the existing provider bridge. The test target is Chartworks' new code. There is no compatibility spike, production-adopter check, host qualification transcript or mandated protocol release migration.

## Iframe work to implement

Use an authenticated Pengui or client-owned BFF URL as the iframe source. The BFF authenticates its user with Pengui and forwards a scoped Pengui-issued bearer server-side to Chartworks' result/render endpoints. Chartworks validates/enforces it normally and returns HTML/SVG/data. BFF caching must remain private and keyed by the authorized result/context.

No JWT in the iframe URL; no Chartworks-issued embed credential, grant, one-time bootstrap code, cookie session, signing key or renewal endpoint. Other API consumers get credentials from Pengui as well. A future direct-browser access pattern must use the same issuer and existing platform delivery mechanism, not quietly add local auth.

The serving BFF response applies approved frame-ancestor policy and ordinary browser protections. A URL/run identifier is not a secret that substitutes for authorization. Every subsequent private data/rendition request still goes through an authenticated request. Offline-valid JWTs are bounded by expiry; do not claim immediate revocation of bytes already delivered or downloaded.

## Genuine SSR work to implement

Go generates safe tables, KPI labels and text. An optional pinned ECharts SVG worker renders chart SVG from sealed typed input; ECharts documents a JavaScript server SVG path, not a native Go renderer. [V1] The worker has no warehouse/model tokens, network egress, arbitrary URL loading or user script execution. Use bounded IPC/argv, CPU/memory/time/concurrency/output size and a supervised lifecycle; return typed failures without taking artifact reads down.

Render input includes exact data/spec, artifact ID, renderer/theme version and viewport. Sanitize labels/Markdown/SVG and reject scripts/events/foreign objects/external references/unsafe formatter and CSS options. Avoid trusting a string as HTML just because a model produced it. Retained rendered bytes inherit artifact privacy/context/expiry.

SSR acceptance disables client chart JavaScript and still observes the promised chart/table/KPI content. An HTML shell with browser chart rendering alone fails. Interactive hover behavior is not a requirement for static SVG. Preserve exact numeric labels even when graph coordinates are approximate.

## Export and side effects

Deliver explicitly authorized JSON/CSV/HTML/SVG exports with row/byte limits, spreadsheet-injection protection and exact metadata. CSV safety must not silently change the canonical data; record escaping/export representation. Full PDF/PNG/paginated document rendering is separate later scope and is never implied by SVG support.

Catalog delivery is already useful. Outbound notifications are Pengui integration effects with a durable intent/receipt, not an email subsystem in the renderer. Never claim delivery because recipients were stored. A downloaded authorized file cannot be recalled; apply the separate export permission accordingly.

## Release evidence

Phase 31 proves Chartworks' real tools/resources/viewer behavior and zero-execution views. Phase 32 proves BFF credential boundaries, static rendering fidelity, renderer isolation, output limits and retention. All output kinds share data/label/unit/order/partial/trust fixtures across API/viewer/static paths. No release gate asks whether the owner-confirmed hosts support Apps.

## Official implementation references

Checked 2026-09-04 for implementation syntax, not host qualification:

- M1: https://modelcontextprotocol.io/extensions/apps/overview
- M2: https://modelcontextprotocol.io/extensions/apps/build
- V1: https://echarts.apache.org/handbook/en/how-to/cross-platform/server/
