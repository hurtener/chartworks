# Delivery: MCP Apps, embedded viewing and real server rendering

Status: proposed architecture, 2026-09-04. The protocol references were checked on that date; actual Harbor/Pengui host interoperability remains an explicit implementation gate.

## 1. One semantic result, several delivery adapters

```
Approved definitions -> governed run -> retained artifact + evidence
                                      |
                         versioned presentation contract
                          /             |              
                     API data      shared viewer     static renderer
                                   /          \       HTML + SVG
                              MCP bridge    embed bridge
```

The renderer does not select source tables, generate SQL, calculate authority, or rerun a query because it was opened. A request to change a business filter is an explicit new execution, not a cosmetic redraw of a supposedly identical snapshot. Sorting already retained rows, changing a legend or switching an authorized output can remain local.

The renderer may be a small Svelte/TypeScript or framework-light bundle. No dashboard-builder application, client-specific frontend fork, or React dependency is required by this design. Share the interpretation of the presentation schema, formatting rules and theme tokens; do not create separate chart grammars for the API, MCP and iframe paths.

## 2. Delivery capability levels

| Level | What it really provides | Release claim |
|---|---|---|
| Structured API | Authorized manifest and bounded result/spec data | Headless integration; not a visual application. |
| Interactive viewer | Browser-rendered charts/tables/KPIs/text over a retained artifact | MCP App and iframe interaction; not chart SSR. |
| Go-generated static HTML | Server-generated tables, KPI labels and safe text | Genuine SSR for these primitives only. |
| Static chart rendition | Server-generated chart SVG plus HTML composition | Genuine chart SSR, usable without running chart JavaScript in the client. |
| Paginated document export | Print layout, page breaks, fonts and export fidelity | Separate future capability; do not imply this from SVG support. |

An iframe is a containment/delivery mechanism, not a rendering strategy. It can show any of these levels. Do not label an empty HTML shell plus a browser chart library as SSR.

## 3. MCP Apps protocol contract

The official Apps overview specifies a tool-declared UI resource, resource retrieval, sandboxed host rendering and a UI JSON-RPC bridge. Host support varies. The current tool metadata uses nested `_meta.ui.resourceUri`; the HTML resource carries its own UI CSP/permission metadata. The build guide uses `text/html;profile=mcp-app`. These are extension contracts, not ordinary chart fields in a tool result. [M1][M2]

Illustrative tool declaration:

```json
{
  "name": "reporting_view",
  "description": "View an authorized retained reporting result without running a new query.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "run_id": {"type": "string", "minLength": 1, "maxLength": 128},
      "cursor": {"type": "string", "maxLength": 2048}
    },
    "required": ["run_id"],
    "additionalProperties": false
  },
  "annotations": {"readOnlyHint": true},
  "_meta": {
    "ui": {"resourceUri": "ui://chartworks/report-viewer/v1"}
  }
}
```

The resource is a versioned bundled HTML/CSS/JS application, not a different untrusted page for every result. Result data contains a bounded manifest/projection and authorized reference; private values are not baked into a globally cacheable UI resource. Resource CSP and requested capabilities are minimized. Tool annotations and app visibility are hints, never authorization boundaries.

Suggested narrow reporting tool additions:

| Tool | Purpose |
|---|---|
| `reporting_search` | Search permitted block/report/dashboard discovery metadata; no SQL. |
| `reporting_describe` | Read exact allowed definition/output/parameter information. |
| `reporting_run` | Explicitly create a block/report execution under current authority and budgets. Persisting a run is a side effect; do not label it read-only merely because warehouse SQL is read-only. |
| `reporting_runs` | List authorized retained run summaries, including scheduled results. |
| `reporting_view` | View/page an exact retained artifact and expose its UI resource. Never executes SQL/model work. |

Authoring, publication, certification and schedule mutation remain fully available through the API. Add MCP mutation tools only with an actual consumer, clear confirmation semantics and the same domain authorization. Do not wrap all API routes in one opaque do-anything tool or expose every administrative operation to the default agent.

The app uses the host bridge for provider tool calls. It never receives the Pengui signing key, shared capability bearer, refresh token, or warehouse credential. Requests from the app pass through the same authorized provider operation as requests from the agent. A model-visible summary is bounded and deliberately excludes unnecessary raw rows; the app receives only data the current user may see.

### Host compatibility gate

Capture exact versions and an end-to-end transcript for: Pengui capability registration and scope configuration -> Harbor tool discovery -> preservation of tool UI metadata -> UI resource fetch -> resource metadata/CSP -> artifact result delivery -> app-originated tool call -> scoped token renewal -> denial after revocation. Verify theme/locale, resize, paging, error rendering, no-App fallback and the chosen external MCP host as applicable.

The source review verified relevant Pengui minter seams, not that every step above already works. Ordinary MCP tools, an artifact renderer, or a prior Dockyard investigation are not proof of this chain.

### Core protocol version is a separate choice

The July 28, 2026 MCP release introduced a stateless core and moved tasks into an extension. Consequently, the earlier planning baseline's transport/session assumptions must be rechecked. This is not a demand to upgrade all Harbor installations in the reporting PR: pin the actual tested deployment profile and implement compatibility only for real supported consumers. Keep application run identity independent of any transport session. The Apps UI dialect still has its own lifecycle. [M3]

## 4. MCP authorization versus internal service JWTs

For controlled Pengui-to-capability calls, use the existing server-derived asymmetric issuer/minter path and exact intended audience. Preserve the distinction between a shared service principal and a real-user/delegated call; never pretend that an unsigned actor in tool arguments turns a shared bearer into user authorization.

For general remote MCP clients, a JWT verifier alone is not a complete interoperable OAuth integration. The current MCP authorization specification describes protected-resource discovery, authorization-server discovery and resource-bound token usage. Chartworks can be a resource server relying on Pengui or an approved authorization server; it need not build its own login/consent/authorization server. Tokens are sent in the Authorization header on every request, not in URI query strings, and tokens for other resources must not be accepted or forwarded. [M4]

Publish exact audience configuration for HTTP and MCP. An audience array is accepted only under the configured issuer profile and intended-resource check; Pengui's explicit canonical-resource audience handling must not become a wildcard across different capability servers. Scope negotiation is not the resource-grant system.

## 5. Iframe strategy: two supported integration patterns

### 5.1 Preferred first integration: client-owned same-origin backend proxy

The client authenticates its user normally. Its backend maps that identity to a scoped Chartworks request, fetches an authorized retained artifact/rendition, and serves it under the client's own origin. The iframe URL carries only a non-secret resource reference. Authorization remains on every backend request; a public proxy that fetches arbitrary run IDs would defeat the design.

This pattern works for static HTML/SVG and avoids requiring a browser to send a server credential to Chartworks. It also avoids making third-party cookies the authentication dependency. Document cache headers and the client's responsibility for access checks, data retention and export.

### 5.2 Cross-origin interactive viewer: one-time bootstrap exchange

Where a hosted viewer is genuinely useful:

1. The authenticated client backend requests an embed grant for an exact retained run/output subset, with a registered embedding origin and an allowed policy partition. Scope is derived from the caller's verified authority and can only narrow it.
2. Chartworks returns a short-lived, single-use opaque bootstrap code. Store only its hash and bind it to target, viewer configuration, policy epoch, permitted origin and expiration. A proposed default is 60 seconds to redeem; this is product policy, not an MCP requirement.
3. The iframe loads a data-free public viewer shell under a registered configuration identifier. No JWT or bootstrap code is placed in the URL. The parent and viewer exchange the code using exact-window, exact-origin and nonce checks; never wildcard `postMessage` destinations for credentials. The dedicated viewer origin and supported sandbox settings are part of the integration contract.
4. The viewer redeems by POST and receives a narrowly scoped in-memory view credential for the embed-only endpoint family. It is bound to the exact retained artifact/output subset, has a short lifetime, and cannot run SQL, author, certify, schedule or call the general API. Do not put it in localStorage or persist it across sessions.
5. Each data/rendition fetch checks current grant and artifact policy. Renewal repeats the authenticated grant process; revocation is not delayed until a long-lived signed URL expires.

Origin binding supplements authorization; an Origin/Referer value alone never proves the viewer's identity. A client outside a browser can forge such a field, so possession, expiry, one-time redemption, policy scope and online checks remain essential. Do not claim arbitrary opaque-origin/sandbox host support without a tested bridge. MCP Apps use their host's bridge, not this custom embed bootstrap.

A read-only embed may allow only local filters over already authorized retained data. A true data filter needing another query requires a separate explicit, currently authorized run operation; a view credential never silently upgrades to execute permission.

## 6. Browser and content security

Set `frame-ancestors` as an HTTP response header for the embedded viewer, using configured approved parent origins and testing nested ancestors. It controls who embeds the page; `frame-src` controls which frames a page can load. The directive is not effective in a meta tag and does not inherit a safe default from `default-src`. [B1]

For custom embed messaging, validate both `event.origin` and `event.source`, protocol version, message type, nonce and payload size. Disallow open redirects, arbitrary navigation and credential-bearing URL parameters. Use no-referrer policy and no-store on sensitive responses. Public versioned viewer assets can be cached separately because they contain no tenant data.

CSP is deny-by-default for scripts, connections, images, fonts, frames and forms, then opens only the required bundled assets/approved data endpoint. For MCP Apps, express the policy through the resource metadata understood by the host; do not assume an HTTP header on a resource fetch becomes the sandbox's policy automatically.

Escape SQL-derived labels and table text. Sanitize permitted Markdown and generated SVG; prohibit event handlers, scripts, foreign-object embedding, external references and arbitrary CSS/formatter functions. Approved branding consists of constrained theme tokens and controlled assets, not remote CSS or URLs supplied with a report definition. Do not mark untrusted strings as trusted Go template HTML.

Exported files can outlive revocation; only grant export where that is acceptable. A report-view permission is not automatically a permission to email the underlying dataset or download every row.

## 7. Genuine server rendering

Apache ECharts documents server SVG rendering using its JavaScript runtime with an SSR configuration and SVG-string output. Its DOM-free SVG mode is not a native Go renderer. Lightweight client interaction and full browser rendering are separate options. [V1]

Recommended topology: Go handles identity, query execution, composition, scheduling and artifact access. A small optional render worker consumes a service-produced, typed rendering job over an internal bounded interface and returns static SVG. The reference deployment can colocate it as a supervised process/sidecar; it need not be a separately scaled microservice. Basic tables/KPIs/text remain renderable by Go alone.

The renderer receives only authorized sealed data/spec/theme/viewport/version inputs, never a user-supplied URL or arbitrary script. It has no warehouse credentials, no model token, no outbound network, bounded CPU/memory/concurrency/time/output size, and a clean temporary directory. Validate and sanitize its output before retaining or serving it.

Persist renderer version and theme version. Do not promise pixel-identical output across changed fonts/library versions. A supported compatibility rendition must retain the original data and clearly identify its rendition version. Full-feature interactive charts may use the browser bundle; static SSR need not emulate every hover interaction.

PDF/PNG exports are not free consequences of SVG support. Add rasterization or paginated document rendering only with a concrete requirement, isolated dependencies, licensing review and fidelity tests. Do not use a full headless browser as the default source of every chart image.

## 8. Cross-surface acceptance

For each supported output, feed the same sealed synthetic artifact to the API, browser viewer and static renderer. Compare exact labels, table values, units, precision, sorting, filters, evidence, errors and current approval state. Visual regression tests cover every catalog entry, light/dark/white-label themes, empty/single/many rows, negative and missing values, long labels and small viewports.

The decisive SSR test disables client JavaScript and still observes the promised chart/table/KPI content. The decisive security test revokes access after the initial page loads and verifies denial on every subsequent data/rendition request without leaking a broader cached result. The decisive economics test repeatedly opens an existing artifact and observes zero warehouse/model calls.

## References (official specifications/documentation; checked 2026-09-04)

- M1: https://modelcontextprotocol.io/extensions/apps/overview
- M2: https://modelcontextprotocol.io/extensions/apps/build
- M3: https://blog.modelcontextprotocol.io/posts/2026-07-28/
- M4: https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization
- B1: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Security-Policy/frame-ancestors
- V1: https://echarts.apache.org/handbook/en/how-to/cross-platform/server/
