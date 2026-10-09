# Manual report App upgrade and activation

Status: local synthetic rehearsal; **no production activation authorized or
performed**. Both registered HTTP and restricted no-chat MCP have
[real local acceptance](../reviews/manual-app-two-mode-2026-10-07.md). Production
release and activation remain separate operator actions.

## Record the intended release

Pin Chartworks, Pengui and the compatible Harbor app-operation-v1 commit and image digest.
Record the registered App, capability, organization, runtime tenant mapping, HTTP
and MCP audiences, canonical MCP sink, enabled surfaces, exact parent origins,
document hash and configuration generations. Keep credentials in the protected
deployment configuration, not this record. Obtain the build's schema count/hash
from `chartworks schema-manifest`; compare the ordered migration history with
the exact embedded source checksums. `config-check` does not connect to a database
and is not migration or readiness proof.

Inventory existing generic MCP integrations using the chosen capability before
creating any host row. Even a disabled host permanently opts the capability into
App governance until capability deletion. Use an isolated capability for rehearsal;
do not register a disabled host against an existing production integration merely
to test the UI. App role, platform admin, entitlement and native resource/action
permissions remain separate decisions.

## Quiesce and upgrade

1. Stop new App admission and external ingress to all Chartworks writers. Drain
   source reads, worker attempts, schedules, preparation and option operations;
   stop every old binary, worker and maintenance writer. Inventory unresolved
   accepted/uncertain preparations and native read attempts by protected IDs and
   state. Do not settle, delete or retry unknown physical work to make a gate pass.
   Quiescence is an operator obligation; migration advisory locking does not stop
   old application writers.
2. Take a restorable database backup after draining and record its recovery point.
   Rehearse restoration and upgrade on an isolated copy with source/model/network
   effects disabled. Preserve original preparation JSON, hashes, custody, source
   coordinates and report/block definitions. Do not use production credentials in
   the rehearsal.
3. Apply the new Chartworks schema through the normal forward migrator with the
   exact candidate binary and writers still stopped. Migration 087 adds guarded
   timestamped preparation admission and positive settlement/retention evidence.
   Existing unproved rows retain liability. A pre-087 preparation writer is
   incompatible: missing `prepare-v1` is rejected with `CW001`; expired operation
   keys are rejected with `CW002`. Mixed-version writing is prohibited.
4. Migration 088 validates the optional presentation shape across every retained
   block revision without rewriting the JSON. An invalid preexisting presentation
   stops the upgrade. The migrator applies pending migrations in one transaction:
   an 088 failure rolls back 087 and their history rows as well. Keep ingress
   closed, investigate the exact protected revision and produce a reviewed
   forward-compatible repair; do not disable constraints or mutate immutable
   evidence as an ad hoc workaround.
5. Upgrade Pengui with its writers stopped and the new host still disabled. The
   populated 0103/0104/0106/0107 checks preserve existing catalog grants, reference
   contexts and generations. 0106 leaves old unpinned hosts unavailable until an
   explicit digest is registered. 0107 preserves legacy references with an empty
   permission, adds independently granted permission references and target
   allocation receipts, and expires all old App read sessions and their joins.
   This is intentional: users must obtain a new admission after the upgrade.
   0108 adds bounded single-use App operation intents; 0109 adds explicit MCP
   mode/agent/document pins. Existing hosts remain MCP-disabled. Intent custody
   is ephemeral, bounded by its parent session and never a reusable credential.
6. Start only compatible Chartworks binaries in `store.migration_policy: check`
   mode and compatible Pengui binaries. Verify actual `/readyz` HTTP status and
   readiness body, build/image identity, schema history and required relations.
   Restart the same candidate once to prove idempotent schema checking. Health
   alone does not prove that any user can consume the App.

## Activate the isolated App and verify policy

Use the existing canonical Pengui administration API. Register separate surface
audiences, exact HTTPS provider/parent origins and CORS policy, the actual rendered
HTML digest, and the chosen runtime mapping under a generation CAS. Grant a named
Builder only the required create/action/resource/Project/Team reach. Grant an
ordinary Consumer App visibility and independent published-resource/context reads.
Do not grant implicit publish, execute, source query or tenant wildcards.

Qualify each enabled mode independently. For MCP, activate the capability on the
existing intended agent through canonical signed activation; static discovery
uses the service connection profile and grants no execution. Pin the separate
MCP HTML digest, agent and host generation. Use MCP-only mode to verify the Apps
route; when both modes are enabled it selects HTTP. Open a fresh admission, select named data,
create and edit a chart/report, save, close/reopen, validate, preview, obtain fresh
authority for the returned private run, explicitly publish, then open the published
result as the ordinary Consumer. Recheck denied edit/query/publish, missing
dependencies/contexts, cross-user/org access, guessed IDs and altered inputs.
Repeat the documented navigation, stale revision, lost-response, expiry,
revocation, frame/origin/document/generation and late-response checks. Inspect
actual desktop and narrow-screen screenshots and use the keyboard. Count physical
source attempts and model calls; retained redraw/history must add none.

MCP requires the actual existing runtime/host/broker lane to have an exact
App operation binding, current policy projection and no cross-operation credential
reuse, plus the same real Builder-to-Consumer journey and denials. The local rehearsal
records this evidence; repeat it for the intended release/configuration. An MCP resource
rendered by a synthetic adapter or an HTTP proxy is not this evidence.

## Withdrawal and recovery

On a failed user journey, close App ingress, disable the host and withdraw the
relevant canonical grants. Already-issued 30-second tokens can remain acceptable
for about 60 seconds with the default 30-second Chartworks verifier leeway. Track
that window; disabling a host is not instantaneous offline token revocation.
Existing unknown native work remains a reconciliation obligation.

Do not restore broad generic MCP credentials by deleting a host. App governance
persists independently of its host. Do not delete a shared capability as a rollback
shortcut. Keep a dedicated failed rehearsal capability disabled, and remove only
task-owned temporary services/data after capturing evidence.

Prefer a compatible forward fix with all writers stopped. Reverting to a pre-087
binary against the upgraded database is unsafe. Restoring the pre-upgrade backup
also restores its older data and cannot undo external source effects, delivered
artifacts or publication already observed elsewhere. Any destructive restore
requires an explicit incident decision and a reconciled recovery point. This
local rehearsal does not assert a production restore or mixed-writer guarantee.
