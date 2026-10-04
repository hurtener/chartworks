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
  generation, multi-selection layout operations or export in this checkpoint.
  Existing unsupported documents are inspection-only. Source execution occurs
  only after an explicit data-validation or run/preview action.

Capabilities come from current server checks. The UI does not decide authority.
For creation the user supplies a title. The app asks its negotiated trusted host
for an opaque target through `app/allocate-target`, then checks native exact-target
capabilities before opening a new report buffer. Internal IDs are never typed. Buffer changes do not persist until
explicit save. CAS conflicts and uncertain mutations preserve the local buffer
and block retries until authoritative reload. Leaving dirty work asks whether to
discard it. Close/pagehide clears data and fences delayed results.

A private preview's returned run ID is retained as an inspection coordinate.
The host may need fresh exact run-read/preview authority before `reporting_view`
can read it. **Inspect last retained run** performs only that read; it never
executes the preview again. An expired/denied artifact does not regenerate itself.

## Bounded visual editing and independent pages

The Builder starts new reports as version 3 with one empty `Summary` page.
A title-only new report can save without a fabricated component. Reopened
version 2 documents retain their exact shape and need a heading or approved
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

Public or noncurrent revisions use a private copy with a distinct target allocated
by the trusted host from the exact checked source reference. A current private draft is amended under version/revision/digest
CAS. Successful save immediately stages only the selected report widget's exact
`private_preview` revision/digest reference and labels it unvalidated. **Save
report** can preserve it without any source read. Other widgets keep their pins.
**Validate data** is a separate explicit native source read. Validation uses typed
widget literals and the selected page's bound defaults; result rows are not sent
through this authoring lane. **Check chart status** is metadata-only. Private
preview requires fresh exact validation evidence and still rechecks access and
dependencies server-side. Mapping edits never publish implicitly; the separate
publication and review controls below require explicit confirmation.

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

## Deterministic build-time packaging

Readable authored `.js` modules remain the source of truth. The report resource
embeds `generated/report-app.js`, a single minified IIFE built from `app.js` by
exactly pinned esbuild 0.21.5. This is a build-only dependency: production Go and
the iframe do not install npm, compile scripts, fetch chunks, or evaluate strings.
No source maps are generated or served. Both app and read viewer continue to
consume the same `../report-viewer/presentation.js`; the report bundle excludes
the read viewer's host/controller. The embedded resource prepends its registered
public parent list before the same bundle, preserving its `typeof` entry switch.

To install the integrity-locked official registry dependency and regenerate:

```sh
cd web/report-app
npm ci --ignore-scripts --no-audit --no-fund --registry=https://registry.npmjs.org/
npm run build
npm run check
npm run test:bundle
```

Commit both generated files alongside every authored change. `build.mjs` writes
no timestamps or absolute paths. `generated/manifest.json` records the exact tool
version, source/import graph, source/style/build-configuration SHA-256 hashes,
and emitted script digest. Go tests verify all hashes and independently traverse
reachable local static imports, so source, CSS, build configuration, and output
drift fail without needing npm in a Go build. The exact-head browser CI lane
installs from the lockfile and runs `npm run check`, which regenerates in memory
and fails any byte difference without repairing the checkout. It then exercises
the actual embedded resource in VM and Chromium fixtures; it makes no live
provider/model call.

The existing 262,144-byte HTML ceiling and restrictive hash-based CSP remain.
CSP hashes cover the exact inline bytes, including the embedded parent prefix;
oversized embedded registration lists fail closed. Regenerate again after later
source integrations. An earlier bundle's passing tests never establish that it
matches subsequent authored changes.

## Verification

Pure controller/bridge tests (including a deliberately small fake DOM) are not
browser evidence:

```sh
node --test web/report-app/model.test.mjs web/report-app/bridge.test.mjs web/report-app/retained.test.mjs web/report-app/app.test.mjs
```

The Go resource test checks compilation, exact CSP hashes, public-only bytes,
canonical parent origins, bundle/source identity and the shared renderer. The
VM bundle suite executes the generated IIFE through both bootstrap adapters,
checks capability narrowing/teardown and renders exact retained values. A VM with
a small fake DOM is not browser evidence. To generate actual resources:

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

Builder requires an explicit schema-v3 page and negotiated host target allocation.
Field/type/page-size edits stay local. Prepare chart explicitly performs the
bounded actual-schema source read. Create private chart consumes its exact
preparation/digest and inserts an unvalidated private reference on the selected
page. Save report remains source-free; Validate data and Private preview are
separate deliberate actions. Preparation and private creation do not publish.

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
dimensions and eight initial kinds. It supports up to four reviewed text-select,
text-multiselect or date-only range filters. Joins, calendar buckets, arbitrary
expressions and unsupported reviewed policies remain unavailable. Mapping over an
existing result offers all fourteen real native kinds; Combo remains unsupported.

The manual filter UI stages saved defaults separately from temporary preview/run
selections. Text options require explicit governed Search; multiselect retains
1–16 exact values, including empty text, and excludes SQL NULL. Date ranges commit
only on Done and convert inclusive civil end dates to half-open wire bounds.
Private chart parameters use SQL-free authoring metadata. Shared filter bindings
can be selected for compatible widget parameters and removed explicitly.
Report-filter defaults update `Filters[].Parameter.Default`; the separate
block-parameter `Defaults` fallback layer is preserved. Lookup status/control
never reconstruct lost values or restart a source query. These controls have
local native/Node/bundled-code qualification; current hosted browser screenshots
are still pending. Field labels, numeric/date/currency formatting, legend settings
and data-point limits are not yet fully editable.

The separate in-app publication workflow below uses the existing independently
authorized native lifecycle for whole-chart publication, selected reference
rebinding, review and report publication. Unsupported hosts do not offer these
operations. Its current hosted-browser qualification is pending.
Raw target-ID entry has been removed. The versioned host allocation
extension is optional; unsupported hosts explain that creation is unavailable.
The actual Pengui allocation endpoint, real no-chat launch, authority
projection/refresh and production registration remain host integration gates. Synthetic browser proof does not qualify them or mobile layouts.
The [page contract](../../docs/contracts/report-pages-v3.md) and
[preparation contract](../../docs/contracts/manual-chart-preparation-v1.md) retain
their precise domain and evidence boundaries.

## Trusted host target allocation

MCP hosts advertise `hostContext['chartworks/target-allocation'] = {version:
'report-app-allocation-v1'}`; embedded hosts advertise
`capabilities.target_allocation = {version:'report-app-allocation-v1'}` in their
initialization result. The app sends `app/allocate-target` only
after an explicit report create, chart copy save or dataset preparation action.
Requests bind kind, intent, display title and an opaque idempotency key; copies
also pin the checked block/revision/version/digest/output. Replies must echo the
exact version, kind, intent and key and contain only an opaque target ID. No
tenant, grants, provider URL or credentials are supplied by the iframe. Native
capability and mutation checks remain authoritative. This extension does not
implement the still-pending Pengui allocation service.

At most 32 allocation records remain in the live app window. An unconfirmed
allocation can be explicitly resumed with its identical frozen request and key;
no timer, navigation, metadata read or editor change retries it. Chart recovery
only resolves allocation, then returns to explicit Save chart or Prepare chart.
An allocation reply never proves that a native chart/report mutation committed.
Navigation fences late adoption; host teardown closes all records and clears IDs.
Unsupported hosts keep existing-item selection and reading available without a
raw-ID fallback.


## Initialized host tool availability

The trusted host can narrow the app's provider operation inventory with the exact
registered names. Embedded initialization uses
`capabilities.supported_tools = {version:'chartworks-host-tools-v1',names:[...]}`;
MCP initialization preserves the same object in
`hostContext['chartworks/supported-tools']`. This is presentation metadata, never
a grant or permission to use an unregistered app tool. Native checks remain
independent and authoritative on each request. Allocation is still negotiated
separately; `app/allocate-target` is not a provider tool name.

The object is closed to `version` and `names`, at most 16 KiB, with at most 96
unique names of 1..128 ASCII letters, digits, underscores, dots, colons or hyphens.
Malformed values, duplicates, unknown versions and excessive bounds fail closed
for all provider calls. Syntactically valid unknown names enable nothing outside
the app's fixed allowlist. An explicit empty list supports no provider tool.
Omitting the hint preserves existing full-capable host compatibility, still
subject to the adapter's ordinary tool capability and native authorization.
The detached inventory is immutable for the initialized frame/session. Later
context notifications cannot narrow or widen it; an inventory change needs a
fresh frame/session. Ordinary theme/locale context changes remain supported.

Controls intersect native capability hints with their required registered tools.
New-report intent additionally requires host allocation and native capability,
create, read and save tools; after allocation the exact target's `can_create`
is checked again. No raw-ID entry or synthesized native authority is added.
The retained-only five-tool host (capabilities, search, describe, runs, view)
keeps catalog/history/retained inspection and hides creation, execution and
advanced authoring. The text-only host adds drafts/read/create/save and supports
blank v3 save, headings, report titles, layout, pages, full CAS save and reopen.
It does not require the separate widget-patch tool. That narrow save control is
hidden unless its actual tool is registered.

Block metadata registration is required before composing approved outputs or
filters; chart mapping and dataset preparation require their complete native
operation groups. A text-only host never offers chart/source/filter/publication
actions or attempts block-shaped catalog calls. Existing block-bearing documents
or nonempty filters/defaults/bindings/literals are inspection-only in that host.
Absent or empty filter/default arrays on existing text documents are preserved,
with no automatic schema conversion. Retained charts remain readable through
`reporting_view`; availability of retained data never enables source execution.

The adapter/profile tests are deterministic source tests, including both actual
adapter classes and a small fake DOM. They do not qualify a deployed host or
browser pixels; the separate hosted-browser gate remains necessary.


## Explicit manual publication and recoverable review

When their exact optional host tools are available, `publication.js` and
`publication-controls.js` add metadata-only lifecycle inspection, chart
publication, selected-widget rebinding, report review and reviewed-report
publication. Every mutation requires an independent user confirmation bound to
its inspected immutable coordinates and native CAS. Report transitions also
require current native lifecycle hints; those hints never replace server checks.

Chart disclosure includes **every output of the entire revision**, including
outputs absent from the report. The audience is existing centrally authorized
readers; no names, counts, sharing changes, grants or certification are inferred.
A published chart stays published if a subsequent report rebind conflicts.
Rebinding selects exact existing widget pins and delegates the narrow edit to the
server, preserving untargeted content. The report remains private until its
separate review and publication transitions succeed. Reopening a review is
explicit, and saving from it creates a new draft while preserving pending review.
Native Return review uses the independent rejection operation, requires current
`can_reject` authority, a bounded note and explicit confirmation. It preserves
newer draft/publication pointers and requires a new amended revision before
resubmission; no withdrawal is fabricated.

Unknown outcomes retain their original request across navigation. Recovery reads
the original exact revision, including after publication clears private pointers.
An unchanged head does not establish that an earlier request stopped. After exact
inspection confirms unchanged CAS, digest and current eligibility, a separate
confirmation may retry only the identical metadata-only request; native CAS
prevents a second commit. A retry conflict remains unresolved until another exact
inspection. No source-query, validation or run retry is implied by this path.

Publication never relabels a retained private preview or runs public data. Browse
loads the actual published catalog; its explicit Run action is independent.
Focused Node lifecycle/controller/bridge tests cover confirmations, all-output
scope, selected-widget rebinding, CAS conflict, unknown success/retry, stage
recovery, missing tools, double clicks and late navigation/teardown responses.
These tests are synthetic source tests, not browser or live-source evidence.

Lifecycle metadata is bounded at 3 MiB without changing bridge or request limits.
Recovery custody is bounded at 16 MiB across at most 64 records. Only terminal
known records may be pruned; unresolved requests retain their original arguments
and native report bases. Exhaustion rejects new mutations before dispatch.
Frozen chart disclosures retain every output ID, kind and title; native digest
and evidence pin complete content. Retry snapshots retain exact eligibility
coordinates rather than copying full chart mappings or report bodies again.

A successful Return review opens the preserved current draft, including a newer
independent draft when one exists. Exact unknown-outcome inspections can display
an older private revision only as read-only history, with an explicit action to
open the current draft. The native `rejected` receipt distinguishes a returned
revision from a publication even after reopening; missing receipts never turn
changed pointers into a claimed rejection.
