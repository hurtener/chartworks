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

The first slice is manual heading/published-block report composition and retained
consumption. It does not claim advanced drag/drop, multi-page authoring, arbitrary
NLQ widget creation, whole-catalog dynamic authority metadata, production host
activation or complete migration parity. Existing broader release gates remain
open independently.
