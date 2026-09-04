# Phase 32 — reporting-rendering-embed

Status: planned. Owner: internal/rendering, web/report-viewer. Hard dependencies: 28, 29, 31.

## Authority and design

RFC-002 §8, `docs/reporting/delivery.md`, D-044/D-047 and [COMMON.md](COMMON.md) apply. Rendering consumes sealed authorized data. Iframe authentication stays in the existing Pengui/client BFF; Chartworks is only the JWT-protected content provider.

## Brief findings incorporated

Briefs 06, 14: shared output semantics, exact formatting, safe content and artifacts that render without re-execution.

## Findings I'm departing from

Delete the earlier Chartworks embed-grant/bootstrap-code/view-token proposal. No local browser session or issuer. An HTML shell that needs client chart JavaScript is not chart SSR.

## Scope and implementation tasks

1. Add authenticated artifact HTML/SVG/rendition endpoints; reuse the viewer contract and a Pengui/client-owned BFF iframe integration.
2. Render tables/KPIs/text in Go and charts using an optional isolated pinned ECharts SVG worker; consume sealed values, never a URL/query.
3. Retain rendition version/theme/viewport metadata, sanitize content and bound resources; declare supported exports separately from paginated documents.

Render jobs contain artifact/spec/data/theme/viewport/version only. The renderer has no source/model credentials, arbitrary script or network egress. The process boundary is supervised with resource/time/output limits, returning typed failures. Canonical exact numbers feed labels/tables/exports even when chart geometry uses numeric approximation. The BFF forwards a scoped Pengui bearer server-side and serves the authorized iframe route with approved parent policy; Chartworks issues nothing.

## Non-goals

No standalone page builder, headless browser for every chart, arbitrary remote URLs/CSS/JavaScript, embed-auth service or implied PDF/PNG export support.

## Config and persistence

`rendering.enabled`, worker_path/version, max time/memory/output/concurrency, safe theme/assets; trusted BFF frame-ancestor configuration. No signing/bootstrap cookie configuration. Persist rendition version and artifact reference plus bounded bytes in the existing artifact retention path. A BFF example is a concrete consumer, not a new IAM endpoint.

## Acceptance criteria

1. **AC01** — HTML/SVG/result reads require Pengui-issued target/context reach and do zero model/warehouse work.
2. **AC02** — BFF iframe example keeps bearer credentials server-side and forwards scoped authority; Chartworks has no bootstrap-code/embed-token/session/signing endpoint.
3. **AC03** — With client chart JavaScript disabled, promised tables/KPIs/text/chart SVG remain visible; a shell alone fails SSR acceptance.
4. **AC04** — Label/Markdown/SVG/theme injection, external URLs and executable formatter options are rejected; renderer has no source/model secrets or network access.
5. **AC05** — CPU/memory/time/concurrency/output limits, crash isolation and cancellation protect the Go service and retain honest failure states.
6. **AC06** — Viewer/static renditions agree on exact values, ordering, units, omissions and provenance across the full output catalog.
7. **AC07** — Renderer/theme version and artifact retention/deletion are coupled; a newly rendered rendition does not pretend to be a historical byte-identical output.
8. **AC08** — JSON/CSV/HTML/SVG exports enforce explicit export reach and spreadsheet/text safety; PDF/PNG/page-layout support is not falsely advertised.

## Tests, coverage and smoke

Implement `TestPhase32/AC01` through `TestPhase32/AC08` using the real worker and BFF sample. Run client-JavaScript-disabled static tests, injection/egress probes, render timeout/crash scenarios and exact-value comparisons. Check retention across every output file. COMMON.md sets coverage; `scripts/smoke/phase-32.sh` requires all eight results.

## Glossary, decisions and deviations

Iframe is delivery; SSR is where content is rendered. Neither requires Chartworks to issue credentials. D-044/D-047 apply. No runtime completion is claimed.
