# Optional manual report app

`reportapp.HTML()` is the versioned MCP Apps resource
`ui://chartworks/report-app/v1`. `reportapp.EmbeddedHTML(parents)` is an explicit,
separate embedded entrypoint for a bounded list of registered canonical HTTPS
parent origins. Neither entrypoint falls back to the other. Both mount the same
`ReportApp`, `DraftSession`, canonical `DocumentDefinition` and shared retained
`Viewer`. The existing `ui://chartworks/report-viewer/v1` stays unchanged.

## First slice

- Consumer: authorized published report catalog, retained runs, exact retained
  values and explicit approved runs with typed business filters.
- Builder: exact-target creation; private draft catalog/reopen; headings;
  published KPI/chart/table output selection; report/widget titles; bounded
  one/two/three-column layout; declared block-parameter filters and bindings;
  full-report CAS save; narrow selected-widget text/presentation save; reload;
  explicit private preview and independent retained read.
- No chart-mapping replacement, arbitrary query, dynamic generation, narrative
  generation, publication, advanced drag/drop, multi-page editing or export.
  Existing unsupported documents are inspection-only. Source execution occurs
  only after an explicit run/preview action.

Capabilities come from current server checks. The UI does not decide authority.
For creation the host first allocates/authorizes an exact report ID; the app
checks that ID before opening a new buffer. Buffer changes do not persist until
explicit save. CAS conflicts and uncertain mutations preserve the local buffer
and block retries until authoritative reload. Leaving dirty work asks whether to
discard it. Close/pagehide clears data and fences delayed results.

A private preview's returned run ID is retained as an inspection coordinate.
The host may need fresh exact run-read/preview authority before `reporting_view`
can read it. **Inspect last retained run** performs only that read; it never
executes the preview again. An expired/denied artifact does not regenerate itself.

## Embedded integration boundary

The serving host must enforce matching `frame-ancestors` response headers. The
resource's explicit origin registration is public routing configuration, not
credentials or grants. CORS registration alone does not authorize embedding.
Pengui host deployment/admission remains an independent integration gate.

After the registered resource is loaded, its exact parent sends:

```json
{
  "protocol": "chartworks-report-app-v1",
  "method": "bootstrap",
  "frame": "frame-instance-17",
  "generation": 1,
  "params": {"challenge": "one-use-correlation-17"}
}
```

Only an exact configured origin and `window.parent` are accepted. Extra bootstrap
fields are rejected. The correlation challenge is not a host-only nonce and must
not carry authority. The app responds on the same protocol/frame/generation with
an `initialize` request and `params.challenge`; the parent returns the matching
request ID and `result: {challenge, tools: true, context: {locale, theme}}`.
After that the challenge is discarded. Every request/reply pins the frame and
generation. Requests use `method: "tools/call"` with `{name, arguments}`; names
are the fixed versioned authoring tools and existing reporting delivery tools.
Results preserve the MCP structured-content envelope across both adapters.

The parent handles provider communication and fresh operation-specific authority.
It must not send a bearer or host-only nonce into the iframe. The app has no
network calls, token URL, browser storage or provider destination selector. The
parent sends `method: "close"` on teardown/revocation and fences outstanding work
on its own side. `context` updates carry only presentation hints. The resource
closes pending requests on teardown/pagehide and does not resume after Back.

Initialization is capped at 65 seconds; the transport caps pending requests at
16 and bounds JSON depth/size. These are transport limits, not credential TTLs.
Unrecognized methods and wrong source/origin/frame/generation messages cannot
settle requests or widen the fixed tool allowlist.

## Verification

Pure controller/bridge tests (including a deliberately small fake DOM) are not
browser evidence:

```sh
node --test web/report-app/model.test.mjs web/report-app/bridge.test.mjs web/report-app/app.test.mjs
```

The Go resource test checks compilation, exact CSP hashes, public-only bytes,
canonical parent origins and the shared renderer. To generate actual resources:

```sh
CHARTWORKS_REPORT_APP_HTML_OUT=/tmp/report-app-mcp.html \
CHARTWORKS_REPORT_APP_EMBEDDED_HTML_OUT=/tmp/report-app-embedded.html \
go test -count=1 ./web/report-app ./web/report-viewer
```

The separate Chromium runner exercises both real compiled entrypoints against a
synthetic host/provider. It needs Node 22+ and a working Chromium/Chrome binary:

```sh
CHARTWORKS_CHROME_BIN=google-chrome node web/report-app/browser.test.mjs /tmp/report-app-mcp.html /tmp/report-app-mcp.png mcp
CHARTWORKS_CHROME_BIN=google-chrome node web/report-app/browser.test.mjs /tmp/report-app-embedded.html /tmp/report-app-embedded.png embedded
```

Embedded browser fixtures use CDP request fulfillment at the exact configured
synthetic HTTPS origin. They do not change certificate trust or bypass browser
warnings. The suite covers create/save/reload, a published output and typed
filter, retained exact values, explicit consumer run, private preview read denial
and authority-refresh retry, conflict preservation, cancelled navigation,
repeated save/navigation, closure with a delayed response, Consumer-only mode,
and no browser storage. Failure screenshots are saved separately. These fixtures
are not deployed host, live warehouse/provider or real-grant qualification.

Browser evidence uses a fixed 1440×1000 desktop viewport and a fixture iframe
that follows the app's bounded size notifications. Full-page screenshots capture
the Builder editor at the requested path and Consumer retained view in the sibling
`.consumer.png` file. Both show synthetic fixture data, not a production session.
The harness requests graceful browser shutdown, joins its owned child process,
and uses bounded profile-removal retries; cleanup failure still fails the run.
Its deterministic cleanup regressions need no browser:

```sh
node --test web/report-app/browser_cleanup.test.mjs
```
