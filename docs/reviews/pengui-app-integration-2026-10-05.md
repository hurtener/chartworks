# Pengui manual application integration checkpoint

Status: incomplete overall. The registered HTTP lane now implements and locally
qualifies 32 operations: named data selection, creation, save/reopen, preparation,
validation, private preview, publication, independent Consumer reads, chart
copy/presentation and governed options. Restricted no-chat MCP and final activation
remain open. Later dated checkpoint sections below contain the detailed evidence.

Current implementation checkpoints before migration rehearsal: Chartworks
`840830fc52b2f8f4caa0cdbb65e42f96eb30373b`, Pengui
`47d314272a87b3af9e1dbdc7b7eafe3587d8362b`. Both PRs remain draft/unmerged.

## Historical starting baseline and boundaries

At the start of this continuation, draft PR76 was at `8bfe9a196dc11df8a20104f68c5c7c34875824c6` against
remote main `650356f2f37e67910922febc43bdcec5835cf5eb`. The ordinary reporting
recovery run 37239733242 failed renderer kernel tests and Phase32 AC03–AC05.
The later protected [run 37263329639](https://github.com/hurtener/chartworks/actions/runs/37263329639)
passed with downloaded evidence binding **8bfe9a1** as tested source and
**650356f** as trusted provisioner. The workflow run head alone identifies the
provisioner and is not enough to establish candidate qualification.

Pengui's `feat/chartworks-apps-replayed` candidate is
`ea9f6d3a88fce40f3857600a3cb01a587965fb94`, based on main
`53f7515342a92aa63b4af03b27e7bc3f663b0d52`. Its exact-version Consumer
[run 37312738341](https://github.com/pengui-ai/pengui/actions/runs/37312738341)
passed 16 synthetic browser checks using the actual 8bfe9a1 HTML, native-recorded
payloads and synthetic publication coordinates. Desktop and 375-pixel images
were inspected. This proves neither a real provider connection nor Builder
authority. The narrow layout uses horizontal workspace scrolling, including
keyboard navigation; it is not a responsive reflow of the saved report layout.

## Current cross-repository gap checklist

- [x] Shared native Builder/Consumer application and bounded HTTP host with exact
  document binding, operation hints, host theme and keyboard/narrow-screen checks.
- [x] Central create-report/create-chart/copy-chart policy, server-owned idempotent
  allocation, atomic creator grants and provider-native pinned-source copy checks.
- [x] Native dependency discovery and independent read/write/query/validate/
  execute/preview/publish grants for the 32-operation HTTP lane.
- [x] Fresh authority for the exact private run; original custody and retained
  output survive later draft changes without query replay.
- [x] Real local registered HTTP Builder preparation/save/reopen/preview/publish
  and ordinary-user Consumer journeys, plus chart editing and governed searches.
- [x] HTTP negative/interruption cases, exact values/provenance, and source/model
  counters are recorded in the checkpoint sections; no paid model validation.
- [ ] Restricted no-chat MCP through the existing runtime/host framework and the
  same central policy. Current runtime broker/cache lacks an App operation binding.
- [ ] Full both-mode end-to-end acceptance against the final combined release head.
- [x] Populated synthetic 087/088 and 0103/0104/0106/0107 upgrade rehearsal; explicit cutover and recovery runbook.
- [ ] Restricted MCP activation and final combined-head both-mode verification.

On 2026-10-05 the owner clarified that no rejection/review text is known and
authorized dev administration or an isolated local Docker environment. No platform
rejection has occurred in this continuation. The unspecified historical report
is not a current gate. A separate local database is being used; production and
other workstreams remain unchanged.

## Contract and migration requirements

Pengui retains policy ownership. Allocation must derive identity/org/provider
tenant from the current verified session, return the original target after a lost
response, conflict on changed idempotency intent, and independently authorize an
exact copy source. Allocation and central creator grants need consistent local
persistence; provider draft creation remains a separate cross-service effect.

Dependency discovery requires a reviewed authenticated metadata entry point,
exact operation/target/revision/digest binding, independent central checks of
every resource and versioned context, and provider revalidation at execution.
The metadata-only endpoint is specified in
[report-dependencies-v1](../contracts/report-dependencies-v1.md); it creates no
bootstrap credential. The actual
`internal/reportingapi/authoring*.go` operation registry remains authoritative for
actions/effects/loaders. Scope overflow denies under the existing 32-entry and
4096-byte limits; no omitted dependency or wildcard fallback is permitted.

Before applying 087, stop and drain all older writers, including source/reporting
and native retention workers, then migrate while quiesced and start compatible
new binaries only. Follow [preparation retention](../contracts/manual-chart-preparation-v1.md).
Migration 088 adds a structural presentation constraint without rewriting old
JSON. Rehearse against retained definitions and qualify new presentation payloads
through current readers/SDK/renderers. Old-binary compatibility is unproven;
pre-087 admission/retention rollback is unsafe. Use a compatible forward fix.

For activation, use an isolated capability/org/provider with known existing
integration impact. Even a disabled Pengui host registration makes that capability
App-governed until capability deletion. Register exact surface audiences,
tenant mapping, HTTPS parent/CORS origins and the actual embedded document hash.
Prove live usability separately from enabled configuration. A new document build
requires a new registered digest and invalidation of prior sessions.

The real acceptance matrix must cover distinct actions/grants, wrong identities
and contexts, lost responses/CAS, withdrawal/team/org changes, frame/origin/digest
mismatch, close/reopen/history/late replies, exact output/provenance and zero
source/model calls on retained redraw. Thirty-second Pengui read tokens plus
Chartworks's default thirty-second verifier leeway can permit roughly sixty
seconds of offline acceptance. No instantaneous revocation is promised.

No production deployment, production grants, persistent credentials or paid
provider calls are part of this checkpoint. Pengui remains unmerged. Synthetic
browser, local unit/race, hosted CI and real-service evidence remain separate.

Local checks for this documentation continuation: `go test -race ./docs
./web/report-app -count=1`, mirrored rules and diff checks passed. The initial
planning test run failed five harness path comparisons because macOS aliases
`/var` to `/private/var`; `TMPDIR=/private/tmp make planning-check` passed all
61 checker tests and the planning DAG/coverage coherence check. No test was
skipped to repair that path mismatch. These checks do not replace the native
database, protected-renderer or real-service gates described above.


## Allocation continuation

Pengui's companion [draft PR367](https://github.com/pengui-ai/pengui/pull/367)
at `81e0a89d4af5093eac04df958bd11f0b1900b5e5` adds exact permission references to
the canonical ShareGrant model and an
HTTP allocation endpoint for `create_report` and `create_chart`. Current App
editor reach and an independent creation grant are required; independent
preview/execute/publish decisions constrain creator grants. Same-key replay does
not restore revoked authority. The old HTTP Consumer continues to expose only
its five operations. `copy_chart` remains rejected pending the provider-native
source projection; no manual Builder journey is claimed.

Local real PostgreSQL/SQLite and in-memory race tests qualify the allocation
transaction, migration preservation, generation/identity fences, concurrent
retries and revoked-grant behavior. A Project-deletion deadlock was reproduced
and fixed by locking the parent before the reference. These are Pengui backend
checks, not provider or browser acceptance of the Builder.

Pengui Actions at checkpoint `d68cf20ade0e96068fa61898a4ed69237ccb86e7` did not
start: GitHub reports failed recent payments or a required spending-limit increase.
This is a confirmed hosted-runner/account limitation, not a source-test result.
Chartworks source checkpoint `edc156b21f31125f1eef834bae7e8334a8c2df5f` passed
ordinary CI 37330388517, SQL recovery 37330388699 and protected renderer
37330564330 (22 events, 100 renders); ordinary reporting recovery 37330389435
failed the missing cgroup-root prerequisite. The protected log binds the tested
candidate explicitly. No all-CI-green or real-service completion claim follows.


## Native dependency continuation

D-098 adds the HTTP-only `reportAppDependenciesV1` metadata seam over existing
native PostgreSQL indexes. It returns complete bounded IDs, revision/digest and
privacy, resolves current publication pins, and retains private custody. Ordinary
content reads still require all dependencies. There is no migration or new MCP
App tool; the 96-tool ceiling is unchanged. Pengui PD-238 uses this seam for the
exact block-read and report-reopen projections, with independent
canonical permission grants and a final policy-generation check after minting.

Direct adversarial review found two dependency-completeness hazards and added
regressions before qualification: replayable query widgets are not necessarily
in the session-query index, so the manual discovery lane explicitly rejects all
query widgets; latest-publication pins must resolve the current block revision
and its context requirements rather than the report's older snapshot. Missing
selected revisions fail instead of disappearing from a dependency join.

The owner requires local testing because hosted billing prevents runners from
starting. Historical CI results above remain historical; they do not qualify
these new sources. Local `go test -p 2 -race ./test/acceptance -run '^TestReportAppDependenc'
-count=1` passed with PostgreSQL 17/pgvector, the pinned patched native parser and
its freshly built executable. Both dependency tests ran (including native
draft/review selection); no source/model work occurs during metadata discovery.
Targeted API, SDK consumer/schema parity and full App tool-inventory checks also
passed. Native artifact hashes and full local logs are retained outside source.
These are local native acceptance results, not a deployed Builder journey.

Existing native authoring/block-mapping regressions also passed locally under
`-race` against PostgreSQL (`TestReportAppAuthoring`,
`TestReportAppAuthoringBlockReach`, `TestReportAppBlockMappingAuthoring`). This
checks the existing content/authoring domain alongside the new metadata seam.


## Reviewed data projection candidate

D-100 adds provider-native reviewed-topic and original preparation dependency
metadata. The Pengui candidate consumes it for named topic pages, topic/dataset
reads and allocated chart Prepare/Inspect/Consume/Control. Native custody is
stable across iframe admissions inside one canonical Pengui login and remains
partitioned by App, user, organization, recipient and tenant. Both implementations
are under local qualification; no real-service chart or MCP acceptance is claimed
by this source checkpoint. Billing CI is intentionally not requested.

The opt-in `chartworks_live_fixture` build tag supplies
`TestReportAppLiveFixture` for the separate real-service lane. With an isolated
`CHARTWORKS_TEST_STORE_URL` and a fresh absolute
`CHARTWORKS_LIVE_FIXTURE_OUTPUT`, it seeds synthetic reviewed data through the
existing domain services, exports a private read-only connector configuration
and a public logical selection descriptor, then keeps its disposable databases
alive for at most two hours. The reference service uses the real platform issuer;
the fixture's test issuer and recorded setup gateway are never exported to it.
Creating `OUTPUT/stop` records source/model deltas and runs normal database/role
cleanup. Private connector output must never enter evidence or source control.
This harness does not itself establish browser, publication or MCP acceptance.

Local source gates: reporting, reporting API, PostgreSQL and SDK focused race
checks passed. `TestReportAppDataDependencyDiscovery` and
`TestReportAppPreparationConsumedReplayAfterCleanup` passed on real PostgreSQL 17
with verified TLS (61.039s together). The first run used an unencrypted Docker
hostname and correctly failed source registration; only the test database
transport/configuration changed. Assets (33 assertions), generated bundle checks,
planning and mirrored rules passed. Direct diff review checked exact discovery
roots, complete publication reach, original actor/session/target custody,
compact consumed receipts, closed wire shapes and absence of execution authority
in discovery seeds. No P0/P1 finding remains in this bounded source change.
The live fixture has reached ready state; HTTP/browser results are still pending.

### Real-service reviewed-data checkpoint

Runtime source `438f91fbb6c5ae243048e50f5e938c539a9c52a5` was exercised with
Pengui runtime/Console `561c38037212eb794b5cdca0c59d5bec26a237f0`. The native image
is `sha256:b23f939b607f912a09f901ef6fa24ef17da9365607042410aeb1226624e3fc2b`.
Fourteen real HTTP checks passed, including named topic/dataset selection,
independent dataset/source/preview withdrawal, allocation/prepare/consume retry,
close/reopen custody and guessed-target denial. Five registered-iframe checks
passed in sandboxed Playwright Chromium 148.0.7778.96: named keyboard selection,
explicit KPI preparation, 375-pixel inspection/recovery, private chart creation
and report save, then named reopening through a fresh App admission.

Desktop, narrow preparation, saved-chart and reopened-chart screenshots were
visually inspected. The chart is honestly displayed as unvalidated; this proves
its saved native reference, not rendered values or publication. Narrow canvas
navigation remains horizontal. External Chrome 154 is still unqualified.

The complete live campaign recorded four source attempts for four explicit
preparations (two HTTP and two browser attempts, including probe corrections),
with zero fixture model calls and the runtime gateway disabled. Retry, inspect,
consume and retained navigation added no source attempt. Probe corrections
compared immutable revision fields rather than current-health envelopes, used
the current default paged report, and explicitly inspected resumed custody.
They changed no runtime code. Fixture shutdown passed and left zero disposable
databases and zero reader/writer roles. Only this task's containers are stopped.

The local `real-services-reviewed-data/source-manifest.json` binds binary hashes,
document digest, source versions, logs, counters and screenshot results. Hosted
CI was not requested. Private preview/run authority, validation/mapping/copy and
publication projections, independent Consumer and restricted MCP journeys, and
the migration/activation rehearsal remain open; this is not completion of the
cross-repository goal.

## Private preview integration checkpoint — 2026-10-05

Runtime Chartworks `4e551e94b26e8e20e61a2d9133c8c1365d11ec2e` and Pengui
`11963d644040c38f3c1faf63ef927e75e4a7e237` pass 25 real HTTP checks and four
registered-iframe checks using the actual local issuer/services. Twenty closed
HTTP host operations now include explicit block validation, saved report preview
admission/execution and fresh exact retained report-run reads. Public-viewer
projection has a focused policy test; full published Consumer acceptance remains
open. Provider metadata and each effect retain independent canonical policy.

The exact synthetic decimal `9007199254740998.625` survives native preparation,
validation, private execution and retained reads. The browser verifies its visible
unrounded disclosure, configured rounded headline, provenance, saved sibling page,
375-pixel redraw and fresh iframe reopening. Final desktop/narrow/widget screenshots
were inspected. Narrow canvases and widget contents retain horizontal/vertical
scrolling. Browser proof uses sandboxed Playwright Chromium 148.0.7778.96; external
Chrome 154 remains unqualified. No screenshot is treated as an authority test.

Local source gates: Pengui App/ConnectedApp race tests 7.923s, additional public
viewer case 2.625s, vet and Linux arm64 build; 57 host/client assertions, ESLint,
Svelte zero errors/warnings, production Console and both bundle gates. Chartworks
reporting 24.463s, HTTP 59.372s, PostgreSQL 18.241s and SDK 81.084s full package race
checks pass. Real PostgreSQL `TestReportAppEffectDependencyDiscovery` passes in
25.165s using only returned exact projections for validation, preview, execute and
retained values; missing scopes and foreign tenant/actor/session reject. Planning
and mirrored contributor rules pass. Hosted CI was not used because of billing.

Direct bounded adversarial review checked metadata-only seeds, original private
run custody, frozen dependencies, independent source/execute/preview grants, scope
limits, closed selectors, canonical final fences and no read-triggered source work.
No P0/P1 issue remains in this increment. Subsequent changes correct probes and
add evidence/test coverage; runtime source stayed unchanged throughout live runs.

The full live campaign records 17 source attempts: three explicit preparations,
eight validations and six preview executions, including early probe failures.
Retries, stale validation rejection, admission, inspection, denied reads, retained
reads, page changes and reopening add no source attempts. Fixture model delta is
zero and runtime models are disabled. Probe corrections reused allocation-created
grants, supplied the required narrative:false DTO field, limited response capture
to successful POSTs, and selected the visible raw KPI disclosure rather than a
closed nested Precision disclosure. Earlier failed evidence is retained. The
final assertion compares the retained decimal against the fixture's expected value.

The local `real-services-private-preview/source-manifest.json` records binary,
image/document identities, results and screenshots. Fixture cleanup passes
(481.592s including service hold), with zero residual test databases/roles.
This task's services/containers are stopped; unrelated running work is preserved.
Both PRs remain draft/unmerged, with no production change or live model call.

Remaining product work: chart mapping/catalog/copy, governed options and lifecycle
publication/rebinding, native projections replacing legacy non-report Consumer
selectors, independent published Consumer acceptance and restricted no-chat MCP
through the existing framework. Migration 087/088 plus Pengui 0107 still require
quiescence/compatibility and activation rehearsal; D-101/PD-244 add no migration.

## Explicit publication integration checkpoint — 2026-10-05

The Pengui HTTP host now exposes 24 closed operations, including the existing
native lifecycle inspection, whole-chart publication, selected-widget rebind and
report review/publication/rejection seam. This stage changes Pengui projection and
transport only; Chartworks runtime remains `4e551e94`. It reuses native definition
metadata and independent canonical publish grants, with optional inspection hints,
strict scope bounds and the existing final live policy fence. No source/model
permission is inferred from publication or editor status.

Local qualification passes 36 real HTTP checks and four registered-iframe
publication checks; a separate 25-check setup creates the browser fixture. The
actual issuer and real PostgreSQL services exercise missing publication grants,
inspect-without-publish, whole-revision chart publication, unchanged private report
pins, stale report CAS, separate exact rebind, recoverable private review, explicit
report publication and the original private run's unchanged privacy/values. The
iframe requires each separate confirmation, preserves both saved pages and exact
published pins, and issues no validation/prepare/preview/execute call during the
publication flow. Desktop, pending-review and 375-pixel screenshots were inspected.
Browser evidence is sandboxed Playwright Chromium 148.0.7778.96, not external Chrome.

The two synthetic setup journeys account for exactly six source attempts: two
preparations, two validations and two private-preview executions. Publication adds
zero. Fixture model-call delta is zero and live model services are disabled. The
real-PostgreSQL harness passes in 335.324s including its hold; cleanup confirms zero
leftover databases/roles. Owned services are stopped and the disposable runtime
container removed; unrelated containers and primary checkouts are preserved.

Local gates: full App/ConnectedApp race suite 13.978s; focused lifecycle/dependency
race suite 3.769s; 62 host/client assertions; vet; ESLint; Svelte zero errors/warnings;
Linux arm64 build; production Console and both bundle gates. Existing Chartworks
runtime was reused; no new full provider suite is claimed. One direct bounded
adversarial review inspected optional hints versus actual publish authority,
exact root/dependency policy, native CAS/custody preservation, scope overflow,
withdrawal during signing, closed frame DTOs, response bounds and unknown-outcome
recovery. No unresolved P0/P1 was found in this increment. Initial unit fixture
errors (admission setup and a non-native root reference) were corrected in tests;
real HTTP and iframe publication probes passed on their first attempts.

Evidence is recorded in the local `real-services-publication/source-manifest.json`
and sanitized logs/results/screenshots. Binaries were built before committing this
stage from the same runtime source content; the manifest records that distinction,
source hashes and binary digests, without claiming a clean-commit release build.
No hosted CI, production deployment, paid model, merge or tag was performed.
Both PRs remain draft/unmerged. Full independent published Consumer projection/run
acceptance, mapping/copy/governed options, restricted no-chat MCP and activation/
migration rehearsal remain open; this checkpoint does not close the overall goal.

## Published Consumer integration checkpoint — 2026-10-05

D-102 / PD-246 complete native published report/block dependencies, explicit
published execution, original retained reads and metadata-only history projection.
Pengui's 25 closed operations use canonical independent policy checks; published
search resolves full native dependencies before returning names. Hidden run
candidates/cursors stay in the BFF. Exact summaries use retained read eligibility,
not execution authority. Ordinary readers need no write, preview or source-query
grant. Private previews keep their original custody after publication.

Local real-service qualification passes 36 Builder/publication HTTP checks,
13 independent Consumer HTTP checks and four Consumer iframe checks. A separate
ordinary user opens the named report, reads the exact decimal
9007199254740998.625 and native provenance, changes saved pages, and reopens through
reload and Back/Forward. Edit, execute, lifecycle and guessed-run attempts reject.
Revoking original context access removes names/history/values; restoring it allows
the same retained values without execution. Final sandboxed Chromium 148.0.7778.96
desktop and 375-pixel screenshots were inspected. The catalog button collision and
narrow reading-canvas overflow were corrected in shared CSS.

The complete fixture campaign records six source attempts: one preparation,
one validation, one private-preview execution and three explicit published runs
during probe development. Same-key recovery, denied effects, metadata, retained
reads and navigation add no attempts. Fixture model-call delta is zero; runtime
models are disabled. The native hold test passes in 1649.853s; cleanup confirms
zero residual test databases/roles. Task containers are stopped, the disposable
runtime container removed, and unrelated containers/checkouts are preserved.

Local gates pass: native real-PostgreSQL effect-dependency acceptance (27.617s),
focused reporting/API/SDK race checks, planning and rule mirrors; App/ConnectedApp
race suite (12.560s), focused Consumer policy/run checks, vet and Linux arm64 build;
66 host/client assertions, ESLint, Svelte zero errors/warnings, production Console
and both bundle gates. Chartworks resource/viewer tests pass with Node 24.
The synthetic browser suites pass 359 assertions per adapter (MCP and embedded);
these are not real no-chat MCP acceptance. Unchanged JavaScript bundle checks
cover 174 assertions. The 16-parent embedded resource is 262141 bytes, only three
bytes below the existing 256 KiB limit; further growth requires deliberate size
work, not a silent limit increase.

Direct bounded review inspected complete published closure, exact run selectors,
private custody, native metadata-only summaries, independent execution policy,
candidate/cursor privacy, canonical final fences and no read-triggered effects.
No unresolved P0/P1 was found within this increment. The initial exact-summary
implementation incorrectly used execution eligibility and was corrected to native
retained-read eligibility before passing acceptance. Probe fixes use the original
canonical Builder session and stable operation key, explicit invited-user
activation, the native completed state, and the selected exact history row.
Stale synthetic UI text assertions were updated without weakening effect checks.
Planning tests require TMPDIR=/private/tmp to avoid macOS temporary-path aliases.
CDP can discard an already-consumed iframe response body during navigation:
only that exact capture diagnostic is tolerated, every reopened frame still must
render the exact value, and native captured output must match. The final run has
zero page errors and zero capture diagnostics.

Local evidence is in real-services-consumer/source-manifest.json and its sanitized
results, logs and screenshots. Runtime content was committed as Chartworks
f997f598 and Pengui 37efe007; binaries were built before those commits from matching
source, with the subsequent Chartworks CSS correction. This is not a clean-commit
release build. No hosted CI was used because of billing; no production, paid model,
merge or tag was performed. Both PRs remain draft/unmerged.

Remaining integration: mapping/catalog/copy and governed options, restricted
no-chat MCP using the existing framework, both-mode full journeys, and migration/
activation rehearsal for Chartworks 087/088 with Pengui 0107. The locally verified
HTTP Consumer increment does not close the overall goal.

## Chart catalog, copying and formatting checkpoint — 2026-10-05

PD-247 admits three existing native operations, bringing the registered HTTP host
to 28 closed operations. Catalog reads are metadata-only. Mapping and formatting
require independent read/write/preview, exact native dependencies and parent-topic
write; copying additionally requires canonical copy permission and an allocated
target, without source-block write. Allocation verifies current native source CAS
and editable output before reserving the identity and creator grants atomically.
Fresh copy authority and native CAS remain required after allocation.

Local qualification passes 25 Builder setup HTTP checks, ten chart HTTP checks and
three real iframe chart checks. The iframe selects a named published KPI, explicitly
copies it to a private table, saves the report, changes the header/precision in one
private amendment, and saves again. Both pages, exact private revision pins and
formatting survive fresh admission/reload. Narrow 375-pixel and desktop screenshots
were inspected. The iframe issues one allocation, one copy and one amendment, with
no preparation, validation, preview or execution. The original published metadata
and retained exact decimal remain unchanged in the HTTP proof.

A real click-after-typing failure revealed that text-field blur rerendered and
removed the clicked Save button. Shared text inputs now commit on input and ignore
a duplicate change event. A focused regression and the actual single-click iframe
journey pass; no Tab/double-click workaround is used. Equivalent transparent CSS
values and an empty rule were compacted to preserve the existing resource limit.
The 16-parent resource is 262052 bytes, 92 bytes below 256 KiB; no cap was raised.
Synthetic browser suites pass 359 assertions per adapter after this fix.

Pengui local gates pass: full App/ConnectedApp race suite (13.715s), focused chart
policy/CAS/copy tests, vet, Linux arm64 build, 69 host/client assertions, ESLint,
Svelte zero errors/warnings, production Console and both bundle gates. Chartworks
resource/viewer tests, planning and rule mirrors pass. Native fixture acceptance
passes in 1114.797s including its hold: exactly three source attempts from setup
(preparation, validation, private preview) and zero model calls. Chart editing,
publication/rebind setup, retained reads and failed/repeated operations add none.
Cleanup leaves zero test databases/roles; owned containers are stopped and the
disposable runtime removed. Unrelated containers/checkouts are preserved.

Direct bounded review checked independent policy before metadata, native source
and parent identity, final policy fences, allocation input replay, source-write
exclusion on copies, closed frame arguments, exact destinations and unknown-outcome
handling. The source-preview guard was tightened before metadata during this
review. No unresolved P0/P1 remains in this increment. An early probe attempted
unsupported formatting on a legacy KPI and correctly failed; the final probe uses
a catalog-supported table. Local setup also corrected an obsolete CA path and
reused the existing isolated App registration. None required weaker authority.

Evidence is in real-services-chart/source-manifest.json and sanitized results,
logs and screenshots. Binaries were built before committing from matching runtime
source; this is not a clean-commit release build. No hosted CI (billing), production,
paid model, merge or tag was used. Both PRs remain draft/unmerged. Governed option
lookups, restricted no-chat MCP, full both-mode journeys and migration/activation
rehearsal remain open; this checkpoint does not complete the overall goal.

## Governed options integration checkpoint — 2026-10-05

D-103 / PD-248 add metadata-only option dependency discovery and wire the four
existing native option operations through Pengui's registered HTTP host (32 closed
operations). Original tenant/actor/login/target/key custody takes precedence over
current definitions. Status/control never fall back to a new query; fresh searches
reuse the existing complete publication/report discovery cores. Every action,
source, dataset and context requires current independent canonical policy. Dataset
targets require same-actor/org/App creation allocation; report options do not
acquire report write. Existing scope limits and final policy fences remain intact.

Local qualification passes 25 Builder setup HTTP checks, ten governed-option HTTP
checks and three real iframe checks. Dataset Search returns actual bounded native
choices and opaque keyset continuation. Changed-input replay, guessed allocation,
retry control and withdrawn source-query policy reject. Replay/status/reconcile
return no reconstructed choices. Original report status/control survive a newer
saved draft and fresh App admission within the original canonical login.
The native retained value remains exactly 3.750. The real iframe performs one
explicit Search for East, stages and saves a default, changes pages and reopens
with that choice; opening, typing, selecting, save and navigation do not repeat
Search or execute a report. Sandboxed Chromium 148.0.7778.96 desktop and 375-pixel
screenshots were visually inspected; the final run has zero page/capture errors.

The live native fixture passes in 344.277s including the hold and records ten
source attempts: six setup attempts (two preparation/validation/private-preview
journeys) plus four explicit option reads (two HTTP dataset pages, one HTTP report
search, one iframe report search). Replay, denied requests, metadata, status,
control, saves and retained navigation add none. Live model-call delta is zero;
runtime models remain disabled. Cleanup leaves zero residual databases/roles,
stops owned containers and removes the disposable runtime, preserving unrelated
containers and checkouts.

Native final-source race gates pass in the reference Linux container: focused
real-PostgreSQL acceptance 86.141s, reporting 25.046s, reportingapi 128.081s and SDK
76.574s. The native tests include original recovery after draft advancement and
independent published-reader discovery. Pengui passes the full App/ConnectedApp
race suite (14.486s), the final 144-case option policy/bounds matrix (6.388s), vet,
Linux arm64 build, 73 host/client assertions, ESLint, Svelte zero errors/warnings,
production Console and both bundle gates. Planning, rule mirrors and diff checks
pass. Chartworks UI assets are unchanged from the preceding visually qualified
checkpoint; the same registered HTML digest is used. No new synthetic-browser
suite run is claimed for unchanged assets.

Direct bounded review covered exact root guards before metadata, source/context
permissions, original custody selection, no status fallback, conservative initial
report closure, output bounds, closed frame targets, final withdrawal fences and
unknown-outcome handling. No unresolved P0/P1 remains in this increment.
Development failures were recorded: the host macOS linker lacks the native
libraries, so native gates ran in the configured Linux container; a test reused
already allocated grant rows with generation zero and was corrected; the expected
frontend inventory needed the four new operations. The live setup initially
assumed that denying one dataset emptied a catalog containing another eligible
topic; its check now targets only the selected topic. The initial fixture expected
3.75 instead of the native exact decimal 3.750, causing the first completed setup
journey to fail its assertion. The expected fixture scale was corrected, a second
explicit setup passed, and both journeys remain included in the source count.
Neither failure weakened runtime policy or changed stored values.

Evidence is in real-services-options/source-manifest.json with sanitized results,
logs and screenshots. Runtime binaries were built before commits from matching
source, not from a clean release commit. The opt-in fixture's expected decimal
scale was corrected after startup; that descriptor change is test-only.
No hosted CI (billing), production change, paid model, merge or tag was used.
Both PRs remain draft/unmerged. Remaining work is restricted no-chat MCP admission
through the existing framework, complete both-mode journeys, and the explicit
migration/activation rehearsal. This checkpoint does not complete the goal.


## Populated migration rehearsal and restricted MCP boundary

This checkpoint changes tests and documentation only. It preserves the qualified
HTTP runtime and UI. No new browser qualification, binary release, hosted CI,
production deployment or MCP capability is claimed.

`TestReportAppPopulatedMigration087088` now creates an actual populated schema
through migration 086, using synthetic native parent metadata and an archived
historical topic. It keeps an expired, unsettled legacy preparation and a retained
chart definition. It proves current check-only startup refuses the old schema,
normal 087/088 upgrade preserves exact JSON/hashes/custody, the current reader
recovers that custody, legacy liability remains charged, old inserts fail with
`CW001`, expired new keys fail with `CW002`, and current admission is guarded.
A malformed preexisting presentation makes the whole pending upgrade roll back
to 086 without partial columns or changed evidence. Same-version reopen passes;
the migration adds no source attempts. This is a synthetic store upgrade proof,
not a production backup/restore or mixed-writer endorsement.

Final-source local gates:

- Reference Linux, real PostgreSQL, `-race -p 1 -count=1`: the populated migration
  test, preparation liability/native guard, bounded retention/quota, consumed
  replay after cleanup and presentation authoring all pass; package 32.249 s.
- Pengui populated 0103/0104/0106/0107 migrations pass on PostgreSQL (4.426 s) and
  SQLite (32.842 s), with no skips. Existing grants/contexts/generations survive;
  stale read admissions expire and unpinned documents remain unavailable.
- Six Pengui governance/withdrawal test groups pass under `-race -count=1`
  (2.514 s), including disabled-host governance after successful generic OAuth,
  derived/receiver credential denial, post-mint withdrawal, close and recipient
  changes. Generic MCP authority stays denied.
- Planning passes with `TMPDIR=/private/tmp`; mirrored instructions and diff
  checks pass. The first planning run hit the macOS `/var` versus `/private/var`
  test-fixture path mismatch, resolved by the environment override. The first
  migration run failed on incomplete synthetic topic/facet state; the fixture
  now represents an archived historical topic and leaves all constraints active.

Direct bounded self-review covered the final fixture, full pending-transaction
rollback, old-writer rejection, evidence preservation and activation instructions.
No unresolved P0/P1 was found in this increment. New work has no UI/runtime change,
so the earlier inspected screenshots remain time-bound evidence rather than a
new browser pass. Test cleanup reports zero disposable databases and roles.

Harbor's fetched `origin/main` is
`6305c88fce1fcacc6ff2fa34440ac037c5de63f3`. The existing resource/tool requests and
broker exchange lack an App operation binding, and signed-capability credentials
are cached per identity session/source/binding rather than per App operation.
The current MCP host and fresh render admission are reusable; they do not fill
that missing credential-policy contract. Pengui records the exact inspected code
and required joint extension in `docs/contracts/connected-app-mcp-runtime-gap.md`.
No alternative broker endpoint, broad credential fallback or second MCP host was
introduced. MCP remains disabled and the overall goal remains active.

The Chartworks `docs/runbooks/manual-report-app-activation.md` now specifies the
upgrade order, writer drain, snapshot/restore rehearsal, 087/088 failure handling,
Pengui admission expiry, exact document/audience binding, Consumer verification,
withdrawal window and forward recovery. Local synthetic migration qualification
is complete; actual MCP activation and final both-mode journeys are still open.
Evidence is retained under `chartworks-pengui-evidence/migration-runtime-boundary`
in the task artifacts; the manifest records source hashes and initial failures.
