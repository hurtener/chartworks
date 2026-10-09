# Report application v1 evidence

Updated: 2026-10-04. Status: finite manual-authoring implementation with hosted
browser and inspected-pixel evidence at the exact checkpoint below. Full manual
filters, presentation controls, in-app publication and real Pengui launch remain
unfinished. Native, browser and protected-renderer evidence are separate gates.

This change is isolated from the frozen core at
`89fb6ea9bd8292172f44b8b50f359e48eb36ae9c`. It depends on that core; it does not
replace its canonical integration PR or expand renderer isolation budgets.

## Current hosted browser checkpoint — 2026-10-04

Source `fb936b1aa20e40790d5a6bd713198425e50669f8`, tree
`d2b8b3acf7c0f4346f8412d4d17f3e267d502f98`, passed ordinary CI
[37171160234](https://github.com/hurtener/chartworks/actions/runs/37171160234).
The actual compiled MCP and explicitly registered embedded resources each passed
**279 Chrome assertions**. The [source/log/image artifact 11291232818](https://github.com/hurtener/chartworks/actions/runs/37171160234/artifacts/11291232818)
has SHA256 `ee821ddd71e67714913b082f84e4be110407ff5625760675e978166f16d63f77`.
Its source identity, both logs and Google Chrome 154.0.8037.57 version were checked.

The recorded pixel review covers seventeen MCP/embedded image pairs, all
pixel-identical across the two adapters. Five new or changed distinct states were
visually inspected; the other twelve matched previously inspected RGB pixels.
The main Builder and native-precision states were also reinspected. The qualified
viewport is 1440×1000 at device scale 1, not mobile or arbitrary host dimensions.

Covered journeys include direct grid move/resize/cancel and exact save/reopen;
independent inline pages and empty-page navigation; real chart type/field editing;
staged table order/visibility/page-size controls and Cancel; exact private chart
copy/amendment, explicit validation and private preview; reviewed-dataset Prepare,
Create recovery and native DTO consumption; retained paging and precision;
authority-withdrawal teardown and delayed-response fencing. These are finite
journeys, not every chart/compiler combination or full product parity.

The published Consumer images use a fresh read-only host profile and an
independently constructed published revision. They contain no private draft
identifiers or editing chrome. Separately named private-history images show an
authorized Builder browsing an explicitly selected private retained preview;
that preview is visibly distinct from the current publication. Changing UI mode
never changes authority or publishes a private chart.

The host is synthetic and uses generated fixtures plus recorded native public
DTOs. Browser proof does not establish live warehouse execution, deployed Pengui
policy/launch, provider calls or protected static-worker isolation.

## Exact local native checkpoints

These results qualify their own immutable source checkpoints, not a later head:

- `4cc874cad9b4faa84de6fe38397f0eed211763ae`: reconstructed reporting, transport,
  PostgreSQL, factory, SDK and app race qualification; four manual authoring/canvas/
  mapping tests, six page tests and private-block composition passed in 41.534 s.
  Phase29 and Phase30 AC01–08 also passed with real PostgreSQL/race in 236.127 s.
- `2707b50bfb7e158fce1b0bdfcbdedf159be7882d`: focused deterministic preparation
  and rule-absence/custody tests passed with real PostgreSQL and race. The native
  creation-to-preview journey made three deliberate source reads and no model
  calls, retaining the exact sum `9007199254740998.625`.
- `0d27a36e0d655f6556d70af4a8ad92858f933e30`: seven affected packages passed full
  race suites; real PostgreSQL Phases27–30 passed AC01–08 (32 roots, no failed or
  skipped criteria) in 305.471 s. Separate source checks passed 144 app and six
  shared-viewer Node contracts.

The [dataset evidence record](report-app-dataset-preparation.md) owns the detailed
compiler, custody, rule-absence and remaining adversarial-test boundaries. No local
native result is inferred from hosted screenshots or a passing planning check.

## Open verification and product work

At the 2026-10-04 02:31 UTC status check for the hosted checkpoint,
[SQL recovery 37171160237](https://github.com/hurtener/chartworks/actions/runs/37171160237)
and [reporting 37171160354](https://github.com/hurtener/chartworks/actions/runs/37171160354)
were still running. This record does not report either as passing. A current-head
protected renderer proof covering the page continuation is also pending; prior
kernel or renderer checkpoints do not qualify a changed head automatically.
Later source or test-only changes require their own applicable checks.

The current app still lacks governed option search, single/multiselect and staged
date-range controls, complete widget/filter applicability, and parameterized
manual dataset creation. Private parameter discovery must use authoring metadata
rather than published-only delivery. Temporary run values must remain separate
from saved defaults. Formatting/field-label/legend/data-point controls and broader
qualified compiler shapes remain unfinished. Combo is not a native supported kind.

Private creation, validation and preview are implemented. In-app block publication,
explicit private-to-published reference rebind and report review/publication are
not. Preparation cleanup remains unimplemented under fail-closed quotas; unresolved
attempt liability cannot be discarded. Real no-chat host launch, exact target
allocation, current dependency projection, refresh/withdrawal and production
embedded registration remain integration work. None is closed by this browser
checkpoint. No deployment or external configuration change is implied.

## Historical initial local record — 2026-10-03

The following records describe the initial slice and preserve its original test
scope. They are not independent proof for the current source head.

- Real PostgreSQL `TestReportAppAuthoring`: create/save/reopen, immutable revision
  CAS and concurrent conflict, private draft catalog filtering, narrow selected
  widget preservation, publication separation, private preview and HTTP/MCP schema
  and authority parity. The initial suite also passed under the Go race detector.
- Real PostgreSQL `TestReportAppAuthoringBlockReach`: approved published block
  composition, denied missing block/context authority before source work, one
  frozen source attempt, exact retained decimal values and fresh exact run-read
  authority. Reading retained output makes no additional source/model calls.
- Real `TestWorkAssemblyLifecycle`: the then-default service/tool groups assembled.
  `TestReportAppFullFactoryInventory` composed 79 actual factory bindings,
  including five optional durable-rendering bindings, without invoking rendering.
- Focused race checks across configuration, authoring/bootstrap registration,
  MCP catalog bounds, foundation mounts and typed SDK/catalog consumers passed.
  Registry tests accepted 64, 74, 79 and 96 tools and rejected 97, duplicates and
  invalid names. Existing request/response byte, concurrency and timeout budgets remain.
- Shared app/controller/bridge Node tests and both Go web packages passed. They
  covered CAS/conflict buffers, repeated mutation attempts, closed/late replies,
  exact origin/frame/generation correlation, explicit embedded bootstrap,
  capability-derived controls and existing exact-number/disclosure rendering.
- Planning coherence, mirrored contributor rules, workflow YAML parsing and diff
  checks passed. These are not runtime or security substitutes.

The tests used synthetic issuer/source/host fixtures. No live provider, deployed
identity policy, customer source, token minting service or external account was
used. The existing narrative-consent model trap confirms a saved narrative output
ID cannot bypass the manual lane's disabled narrative intent.

## Historical browser restriction and continuing host boundary

During the initial local checkpoint, Chromium launch was blocked by the execution
environment and the cloud browser blocked its local fixture URL. Neither restriction
was bypassed. Later hosted Chrome evidence is recorded above; it was not inferred
from Node tests or compiled HTML.

The ordinary CI `report-app-browser` job checks out the exact proposed head,
compiles both resource modes, then drives the shared app in installed Chrome.
It has a ten-minute bound and uploads source identity, logs and synthetic-host
screenshots. Those artifacts require inspection before visual-quality claims.
The synthetic host tests do not establish a deployed Pengui session.

The explicit embedded entry remains disabled until operator registration. It
requires its own exact HTTPS parent list, independently from data-plane CORS.
Real Pengui integration still needs central no-chat admission, exact new-target
allocation, authoritative resource/context projection, selected-widget host intent
binding, private-run authority refresh, and renewal/withdrawal lifecycle proof.
No app profile, bootstrap mode, caller target or iframe message supplies grants.

## Initial scope and later continuations

The initial slice covered manual heading/published-block composition and retained
consumption. Subsequent checkpoints added direct grid manipulation, inline pages,
private chart mapping and finite dataset-first creation. Their current evidence
and remaining product/host boundaries are listed above. Existing broader release
gates remain open independently.

## Reconstructed visual-authoring checkpoint (2026-10-03)

The retained-canvas checkpoint was recovered from verified Git objects and blobs
with tree `507f60f6904567aa593d67bed53584f551f0eda3`, then published as
`be639041004deea7703d37fb371e790d7bb2b416`. Its fresh local checks passed 67 Node
cases and both web package race suites. Hosted fast build/lint/planning/mirror
passed; the browser job stopped before Chrome because an expiry test awaited an
unreferenced timer. A bounded referenced watchdog fixes that test without changing
production expiry behavior or weakening assertions.

The subsequent visual grid, inline pages, private-block bridge and four manual
chart operations were reconstructed after the execution workspace was replaced.
They were not claimed byte-identical to the lost later local commits. At that
reconstruction checkpoint, combined frontend checks passed 98 Node tests and source
formatting/planning checks passed, with operation budgets unchanged. Native domain,
database, transport and browser requalification was still required at that point.
Pre-reset PostgreSQL/race results are historical evidence only. The real Pengui
launch/admission path remains pending; hosted browser fixtures are synthetic.

## Requalified grid and private-authoring foundation (2026-10-04)

Remote `51d097d0af60a837d50122a047f93c1ace742435`, tree
`007c378538221dfe843be62085bee7d6a5039b93`, has green ordinary CI
[37163540103](https://github.com/hurtener/chartworks/actions/runs/37163540103).
Hosted Chrome passed 106 assertions in MCP and 106 in the explicitly registered
embedded synthetic host: real pointer/keyboard/numeric geometry, collision and
cancel, exact CAS save/reopen, genuine KPI/trend/table retained output, scoped
denial clearing and query-free redraw. The 123 deterministic UI contracts also
pass on hosted Node 22.14.0. No claim of deployed Pengui integration follows.

The complete artifact `11288243918` has SHA256
`50c98cafee20647e5ce9508815ad35b8c1263d363fe178bc7f0668f5241c389a`.
Its source identity and both logs were verified. Actual Builder/Consumer pixels
were inspected: grid interaction was functional, while excessive chrome,
oversized-number wrapping, dense provenance and small chart labels required
refinement. Mapping-inspector and multi-page journeys were still open at this
checkpoint; the later hosted evidence above covers those subsequent changes.

Fresh native race qualification on reconstructed local `4cc874c` (the backend
tree published in `8a55cfe`) passed reporting, HTTP/MCP, PostgreSQL package,
factory inventory, SDK and app checks. A separate actual PostgreSQL acceptance
run passed all four manual report/canvas/block-authoring tests, all six page
tests and the private-block composition bridge in 41.534 seconds. The synthetic
cluster stopped cleanly. These are post-recovery results, not inherited passes.

The first dataset-preparation attempt was a subsequent increment. Its focused
unit/schema/registry tests passed, but the initial PostgreSQL journey exposed an
unsupported fixture measure and was not qualified at that checkpoint. Later
preparation, current-rule and custody evidence is recorded at the exact local
checkpoints above and in the dedicated dataset evidence record.

Fresh full Phase29 report/dashboard AC01–08 and Phase30 scheduling AC01–08
regression tests also passed on that immutable reconstructed backend under real
PostgreSQL and the race detector (236.127 seconds aggregate, no skipped/failed
criteria). The fixture cluster was stopped after the run.
