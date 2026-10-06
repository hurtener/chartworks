# Pengui manual application integration checkpoint

Status: incomplete. Chartworks's manual application already implements the
native Builder and Consumer operations; the remaining product gap is trusted
host admission and central interactive policy. No new local issuer, grant store
or renderer implementation is required by this checkpoint.

## Verified baseline and boundaries

Draft PR76 remains open at `8bfe9a196dc11df8a20104f68c5c7c34875824c6` against
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

## Cross-repository gap checklist

- [x] Shared native report application, named resource selection and existing
  HTTP/MCP operations for manual editing, pages, filters, preparation, validation,
  private preview, lifecycle and presentation.
- [x] Negotiated `app/allocate-target`, `report-app-allocation-v1` request and
  supported-operation presentation hints; neither is permission to create.
- [x] Pengui central App/reference grants, five-operation HTTP Consumer host,
  document digest binding and expiry/navigation teardown.
- [x] Source report/chart ID allocation under explicit central creation permission,
  with atomic server-owned idempotent targets and creator grants in all three
  Pengui stores (PD-237, migration 0107; companion draft PR367).
- [ ] Native pinned-source validation for copy allocation and real host negotiation
  of all three creation intents. Allocation alone does not create a provider draft.
- [x] Authenticated native report/block dependency discovery and the first exact
  block-read and report-reopen authority consumers (D-098 / Pengui PD-238–239).
- [ ] Complete run discovery and remaining projections for every Builder
  operation. Operator-maintained references and browser definitions are not
  substitutes for this contract.
- [ ] Exact interactive read/edit/query/execute/preview/publish authority through
  Pengui's existing session, Team, ShareGrant, allowance and issuer machinery.
- [ ] Fresh exact-run authority after preview, without rerunning a denied read.
- [ ] Restricted no-chat MCP admission through the existing framework and the
  same central policy as HTTP. No borrowed conversation/full-agent bearer.
- [ ] Real dev Builder create/edit/save/reopen/preview/publish to independent
  Consumer read on both delivery modes, with negative/interruption checks.

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
