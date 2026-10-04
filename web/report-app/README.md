# Optional manual report app

`reportapp.HTML()` is the versioned MCP Apps resource
`ui://chartworks/report-app/v1`. `reportapp.EmbeddedHTML(parents)` is an explicit,
separate embedded entrypoint for a bounded list of registered canonical HTTPS
parent origins. Neither entrypoint falls back to the other. Both mount the same
`ReportApp`, `DraftSession`, canonical `DocumentDefinition` and shared retained
`renderRetainedOutput` presentation. The existing `ui://chartworks/report-viewer/v1` remains supported and uses the same presentation function.

## Implemented manual checkpoint

- Consumer: authorized published report catalog, retained runs, exact retained
  values and explicit approved runs with typed business filters.
- Builder: exact-target creation; private draft catalog/reopen; headings;
  published KPI/chart/table output selection; report/widget titles; bounded
  12-column move/resize grid with keyboard/numeric alternatives and explicit arrangement; declared block-parameter filters and bindings;
  full-report CAS save; narrow selected-widget text/presentation save; reload;
  explicit private preview and independent retained read. The continuations below
  add independent inline pages, private chart mapping and finite dataset-first
  creation.
- No arbitrary query, dynamic generation, narrative
  generation, publication, multi-selection layout operations or export in this checkpoint.
  Existing unsupported documents are inspection-only. Source execution occurs
  only after an explicit data-validation or run/preview action.

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

## Bounded visual editing and independent pages

The Builder starts with an empty new draft. Version 2 needs a heading or approved
output before save. **Enable pages** is an explicit local upgrade to version 3;
no read automatically rewrites a legacy document. Empty version 3 pages can save.
The Components and Selected rail stays beside the canvas; the report list can
collapse. Components use schematic icons, never fabricated data.

The saved layout has twelve columns, 80px row tracks and 12px gaps. Fine squares
are decorative. Selected cards have move and eight-edge resize handles. Pointer
motion previews only DOM geometry; release commits one valid rectangle. Invalid
placements preserve all cells. Escape, cancellation, capture loss, rerender and
close roll back the gesture. Arrow keys move; Shift + arrows resize; numeric
row/column/width/height controls provide another path. Duplicate finds free space;
explicit arrangement preserves individual heights. Title editing remains inert.

Version 3 tabs use authored page IDs. Add, rename, reorder and remove are unsaved
page changes until full report CAS save. Keep at least one page. Filters/defaults
are page-local, widget IDs report-global, and the narrow content patch carries
both exact page and widget. Other page bytes remain unchanged. Page switching
cancels gestures and fences delayed metadata and detached form controls.
Consumer tabs use exact retained page IDs without editing chrome or queries.
Page-aware compatibility includes every page's semantic settings. Empty initial
retained pages are verified metadata, not cached payloads. Healthy sibling pages
remain readable under the original fanout, buffer and lifetime bounds.

## Staged chart mappings

On version 3 pages, **Edit chart** opens exact SQL-free block metadata and the
native chart catalog. Type, field, variant, sort, KPI and table settings are
staged locally with **Save chart** and **Cancel chart edits**. All fourteen
native kinds retain their real slot requirements. Field IDs come from the
selected output's server-owned candidates; units, aggregation and provenance
cannot be invented. Table column order/visibility/page size/totals and supported
KPI row/comparison/target/sparkline controls remain explicit metadata.

Public or noncurrent revisions use a private copy with a distinct host-authorized
new block ID. A current private draft is amended under version/revision/digest
CAS. Successful save immediately stages only the selected report widget's exact
`private_preview` revision/digest reference and labels it unvalidated. **Save
report** can preserve it without any source read. Other widgets keep their pins.
**Validate data** is a separate explicit native source read. Validation uses typed
widget literals and the selected page's bound defaults; result rows are not sent
through this authoring lane. **Check chart status** is metadata-only. Private
preview requires fresh exact validation evidence and still rechecks access and
dependencies server-side. No publication occurs in this editor.

Unknown save or validation outcomes stay fenced, including after editor close or
a status read. An inspection does not attribute a hidden definition to an earlier
request or authorize execution replay. Closing the app fences late replies and
clears editor metadata. Changing a staged mapping makes retained values visibly
stale; Cancel restores compatible values without a new query.

The bridge allows `chart_catalog` and the four `reporting_authoring_block_*_v1`
operations; native action and dependency checks remain decisive. Dataset-first
creation is implemented through its separate preparation flow below; changing a
saved mapping does not construct a new query.

## Saved report canvas

After an explicit retained opening or private preview, Builder and Consumer show
heading and retained KPI/chart/table outputs together in the canonical grid.
Consumer uses the authorized retained page summary and can inspect an existing query widget’s fixed retained `result` table without executing it; Builder can overlay current
presentation and layout only when its execution fingerprint matches the immutable
definition captured at that exact preview admission. Saving does not replace that
snapshot. Semantic changes hide values behind a visible stale notice until an
explicit saved preview; title, heading and layout redraws reuse existing values.
An uncaptured preview is not treated as evidence for draft defaults, even at the same revision, because other callers can supply runtime filter overrides.

The disposable canvas reads only currently authorized `reporting_view` selections
from retained widget/output summaries, reuses the initial exact payload, and caps
fanout at four concurrent reads, 100 selected outputs including headings, and
16 MiB cached responses. Each response keeps the existing 4 MiB value bound.
Paging replaces one table window while preserving sibling output values and the
whole-output retained digest. Overflow, denial, changed authority projection,
expiry, navigation or close clears retained buffers. Redraw performs no tool call.
Every view stays pinned to its run, exact target/revision, privacy, page, widget,
output, paging window and authorized layout. Private read denial requests host
refresh rather than another execution. Values never become a new definition.

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
node --test web/report-app/model.test.mjs web/report-app/bridge.test.mjs web/report-app/retained.test.mjs web/report-app/app.test.mjs
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

### Dataset-first private chart setup

`dataset.js` owns one immutable preparation operation; `dataset-editor.js` renders
reviewed field controls without chart samples. Topic and dataset selection use the
existing `list_topics` / `describe_topic` metadata tools and the authoring dataset
projection. The projection supplies exact reviewed field IDs, binding aliases,
measure aggregation/units, eight initial chart kinds and explicit unsupported
reasons. No SQL, arbitrary expressions, client-supplied types or authority enter
preparation input.

Builder requires an explicit schema-v3 page and a host-authorized new block ID.
Field/type/page-size edits stay local. Prepare chart explicitly performs the
bounded actual-schema source read. Create private chart consumes its exact
preparation/digest and inserts an unvalidated private reference on the selected
page. Save report remains source-free; Validate data and Private preview are
separate deliberate actions. No publication occurs.

Once preparation dispatches, the operation and target stay pinned. All uncertain
or failed dispatch replies retain custody; Inspect preparation reads metadata by
exact operation/preparation and cannot rerun a query. Original-attempt inspection,
cancellation and reconciliation are separate explicit controls. Unknown create
requires exact consumed custody before the explicit Recover created chart action
can use the server's consumed read branch. Closing setup retains at most16 bounded
recovery records in the current app window, discarding topic/dataset/schema
buffers; reopening requires a new authorized status read. Closing the app clears
all client state. There is no automatic retry or source execution on control edits.

Source tests use clearly synthetic DTOs. Actual PostgreSQL preparation, hosted
browser interaction and pixels qualify this lane separately. The current
[evidence record](../../docs/reviews/report-app-v1-evidence.md) pins hosted
279-assertion runs in each adapter and inspected image pairs to source
`fb936b1aa20e40790d5a6bd713198425e50669f8`; later revisions need their own evidence.

## Remaining product and integration limits

Dataset creation is PostgreSQL-only, with one reviewed measure, at most two direct
dimensions and eight initial kinds. It cannot author filters, joins, calendar
buckets, arbitrary expressions or unsupported reviewed policies. Mapping over an
existing result offers all fourteen real native kinds; Combo remains unsupported.

The manual filter UI currently copies existing block parameters and edits text
defaults, while Consumer keeps temporary runtime values separately. Governed
option search, single/multiselect, staged date-range controls and explicit shared
filter applicability still need implementation. Private chart parameter discovery
must use the SQL-free authoring read instead of published-only delivery. Field
labels, numeric/date/currency formatting, legend settings and data-point limits
are also not fully editable here; stored semantics remain protected.

There is no in-app block/report publication or private-to-published reference
rebind workflow. These require the existing independently authorized native
lifecycle. Host-authorized IDs are still entered manually; real no-chat launch,
allocation, authority projection/refresh and production registration remain host
integration gates. Synthetic browser proof does not qualify them or mobile layouts.
The [page contract](../../docs/contracts/report-pages-v3.md) and
[preparation contract](../../docs/contracts/manual-chart-preparation-v1.md) retain
their precise domain and evidence boundaries.
