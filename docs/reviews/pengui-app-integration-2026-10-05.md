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
- [ ] Explicit central permission to create, separate from consumption; durable
  server-owned idempotent targets and consistently persisted creator grants.
- [ ] Authenticated provider metadata that discovers each operation's complete
  trusted dependency/context closure. Operator-maintained references and browser
  definitions are not this contract.
- [ ] Exact interactive read/edit/query/execute/preview/publish authority through
  Pengui's existing session, Team, ShareGrant, allowance and issuer machinery.
- [ ] Fresh exact-run authority after preview, without rerunning a denied read.
- [ ] Restricted no-chat MCP admission through the existing framework and the
  same central policy as HTTP. No borrowed conversation/full-agent bearer.
- [ ] Real dev Builder create/edit/save/reopen/preview/publish to independent
  Consumer read on both delivery modes, with negative/interruption checks.

The handoff reports a prior platform restriction on Builder authorization. Its
exact rejected action and reason were requested before expanding that path.
This is an unresolved reported restriction, not a newly observed platform denial.
The isolated dev-service target was also requested. Independent Consumer
teardown corrections and documentation do not bypass either prerequisite.

## Contract and migration requirements

Pengui retains policy ownership. Allocation must derive identity/org/provider
tenant from the current verified session, return the original target after a lost
response, conflict on changed idempotency intent, and independently authorize an
exact copy source. Allocation and central creator grants need consistent local
persistence; provider draft creation remains a separate cross-service effect.

Dependency discovery requires a reviewed authenticated metadata entry point,
exact operation/target/revision/digest binding, independent central checks of
every resource and versioned context, and provider revalidation at execution.
No endpoint or bootstrap credential is asserted to exist yet. The actual
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
