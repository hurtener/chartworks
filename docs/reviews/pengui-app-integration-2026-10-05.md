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
- [ ] Complete topic/preparation/run discovery and projections for every Builder
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
