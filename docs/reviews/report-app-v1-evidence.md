# Report application v1 evidence

Date: 2026-10-03. Status: provider/application implementation checkpoint; hosted
browser qualification and real Pengui launch integration remain pending.

This change is isolated from the frozen core at
`89fb6ea9bd8292172f44b8b50f359e48eb36ae9c`. It depends on that core; it does not
replace its canonical integration PR or expand renderer isolation budgets.

## Verified locally

- Real PostgreSQL `TestReportAppAuthoring`: create/save/reopen, immutable revision
  CAS and concurrent conflict, private draft catalog filtering, narrow selected
  widget preservation, publication separation, private preview and HTTP/MCP schema
  and authority parity. The new suite also passes under the Go race detector.
- Real PostgreSQL `TestReportAppAuthoringBlockReach`: approved published block
  composition, denied missing block/context authority before source work, one
  frozen source attempt, exact retained decimal values and fresh exact run-read
  authority. Reading retained output makes no additional source/model calls.
- Real `TestWorkAssemblyLifecycle`: all default service/tool groups assemble.
  `TestReportAppFullFactoryInventory` composes 79 actual factory bindings,
  including five optional durable-rendering bindings, without invoking rendering.
- Focused race checks across configuration, authoring/bootstrap registration,
  MCP catalog bounds, foundation mounts and typed SDK/catalog consumers pass.
  Registry tests accept 64, 74, 79 and 96 tools; reject 97, duplicates and invalid
  names. Existing request/response byte, concurrency and timeout budgets remain.
- Shared app/controller/bridge Node tests and both Go web packages pass. They
  cover CAS/conflict buffers, repeated mutation attempts, closed/late replies,
  exact origin/frame/generation correlation, explicit embedded bootstrap,
  capability-derived controls and existing exact-number/disclosure rendering.
- Planning coherence, mirrored contributor rules, workflow YAML parsing and diff
  checks pass. These are not runtime or security substitutes.

The tests use synthetic issuer/source/host fixtures. No live provider, deployed
identity policy, customer source, token minting service or external account was
used. The existing narrative-consent model trap confirms a saved narrative output
ID cannot bypass the manual lane's disabled narrative intent.

## Browser and host boundaries

Local Chromium launch is blocked by the execution environment; the cloud browser
also blocks its local fixture URL. Neither restriction is bypassed. Actual browser
success has not been inferred from Node tests or compiled HTML.

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

## Scope limits

The initial slice covered manual heading/published-block composition and retained
consumption. The following checkpoints add direct grid manipulation, inline page
lifecycle and private chart mapping. Dataset-first creation, complete chart/page
browser journeys, visual-quality closure, production host activation and whole-
catalog dynamic authority remain separately qualified work. Existing broader
release gates remain open independently.

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
They are not claimed byte-identical to the lost later local commits. Current
combined frontend checks pass 98 Node tests, source formatting and planning checks
pass, and all operation budgets remain unchanged. Current native domain, database,
transport integration and actual-browser qualification must rerun on this tree.
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
were inspected: the grid interaction is functional, while excessive chrome,
oversized-number wrapping, dense provenance and small chart labels still need
visual refinement. Mapping-inspector and multi-page browser journeys remain open.

Fresh native race qualification on reconstructed local `4cc874c` (the backend
tree published in `8a55cfe`) passed reporting, HTTP/MCP, PostgreSQL package,
factory inventory, SDK and app checks. A separate actual PostgreSQL acceptance
run passed all four manual report/canvas/block-authoring tests, all six page
tests and the private-block composition bridge in 41.534 seconds. The synthetic
cluster stopped cleanly. These are post-recovery results, not inherited passes.

Dataset preparation is a subsequent increment. Its focused unit/schema/registry
tests have passed locally, but its first actual PostgreSQL journey exposed an
unsupported fixture measure and remains unqualified. Current-rule activation and
preprojection custody authorization are under explicit review; no completed
dataset-first creation-to-preview journey is claimed here.

Fresh full Phase29 report/dashboard AC01–08 and Phase30 scheduling AC01–08
regression tests also passed on that immutable reconstructed backend under real
PostgreSQL and the race detector (236.127 seconds aggregate, no skipped/failed
criteria). The fixture cluster was stopped after the run.
