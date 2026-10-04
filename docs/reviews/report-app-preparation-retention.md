# Bounded preparation liability and retention

Source-only continuation, 2026-10-04. This is not production rollout, secure
erasure, renderer qualification or real Pengui host integration. The public
[preparation contract](../contracts/manual-chart-preparation-v1.md) defines the
versioned key, explicit migration/expiry errors and required quiesced-writer rollout.

## Implemented boundary

- Reservation no longer releases an aged accepted/uncertain target because the
  native journal is absent. One shared preparation liability predicate protects
  target replacement, conservative quota charge and payload deletion.
- Guarded no-dispatch settlement serializes with native admission. Otherwise the
  exact original attempt, complete manifest, current terminal status and positive
  stopped/not-issued observation must match. Missing legacy evidence stays held.
- Native admission seals validator evidence and current semantic/source pins;
  preparation-specific NOWAIT row fences avoid waiting while holding the native
  journal-retention fence. A compatible queued SHARE can legitimately succeed.
- Native journal pruning preserves unwitnessed preparation evidence in upgraded
  writers. Explicit recovery never reconstructs values or successful preparation.
- Fresh Prepare can commit one actor/tenant-scoped batch of at most 100 terminal
  payloads before final admission, so later capacity refusal cannot undo progress.
  Consumption expiry, 24-hour terminal/consumption clocks, encoded key expiry,
  exact consumed receipts and contradictory-evidence checks are independent gates.
- Immutable consumed receipts are closed and capped at 64 KiB, retain exact native
  revision 1 digests/authority references, and remain byte-charged. Native revision,
  origin provenance, validation, publication and report/result custody are untouched.
- Fresh admission reserves 2 MiB payload plus 132 KiB bounded lifecycle growth.
  Accepted, uncertain and contradictory liability cannot evade those byte charges.

## Named qualification

The acceptance suite contains actual PostgreSQL tests for:

- TestReportAppPreparationLiabilityAndNativeGuard: aged missing legacy journals,
  exact old-body replay/conflict, deletion denial, concurrent BeginRead versus
  guarded no-dispatch settlement and delayed original admission rejection.
- TestReportAppPreparationJournalRetentionAndPositiveRecovery: ordinary native
  pruning retains an unwitnessed attempt, explicit recovery starts a new retention
  clock, later native pruning cannot erase durable custody, and a still-fresh
  encoded key remains fenced even when fixture timestamps are old.
- TestReportAppPreparationExistingWitnessSettlesUncertain: explicit recovery can
  finish logical uncertainty using an already verified terminal witness.
- TestReportAppPreparationContradictoryLiabilityRemainsReserved: conflicting native
  evidence survives cleanup, blocks replacement, keeps the full byte reservation
  and makes cancel/reconcile return uncertainty rather than a terminal claim.
- TestReportAppPreparationBoundedRetentionAndQuota: max-100 committed batches,
  no quota rollback starvation, actor isolation, unknown/unproved retention,
  expired legacy rejection and canonical-looking future legacy key protection.
- TestReportAppPreparationConcurrentQuotaReservesLifecycle: concurrent saturation
  plus later receipt sealing/native admission cannot oversubscribe lifecycle bytes.
- TestReportAppPreparationConsumedReplayAfterCleanup: atomic receipt/delete/audit
  rollback, exact original revision after native edits, no source reexecution,
  actor/session/tenant/revoked-dependency negatives and receipt immutability.
- TestReportAppPreparationRetentionBoundariesAndLockedRows: microsecond retention
  and consumption boundaries, deterministic locked-row skipping and later progress.
- TestReportAppPreparationExpiryAtLockedAdmissionRemainsTyped: a key expires while
  waiting at the topic fence and still receives the promised expiry error.
- TestReportAppPreparationNativeGuardDoesNotWaitBehindRotation: an existing source
  SHARE owner, queued rotation and another admission complete without trapping
  the source owner's journal fence. This does not claim an observed prior deadlock.
- TestReportAppPreparationNativeGuardRejectsHeldConflictingLocks: held custody,
  topic and source conflicts return busy with no durable native admission.

Synthetic already-aged metadata exercises retention without waiting a day. Such
fixtures are not claimed as physical source observations. The actual deterministic
dataset journey independently preserves its three deliberate source reads, exact
numeric value, private native validation and zero incremental model calls. The
new wire fixtures are exact safe DTO exports from that journey, not edited JSON.

## Verified exact-source checkpoint

On `fda04bb254a5a6c9408242df8342cf1cddf53802`, all seven complete package suites
passed with `go test -race -count=1`: `internal/reporting`, `internal/reportingapi`,
`internal/store/postgres`, `internal/foundation`, `sdk/chartworks`,
`web/report-app`, and `web/report-viewer`. Actual Go resource checks include CSP,
compiled-source parity, both registered preparation Node contracts, and the
maximum sixteen-parent embedded HTML case under the unchanged 262,144-byte cap.
The separately executed full non-browser frontend suite passed 366 tests.

The same unchanged checkout then passed every `TestReportApp*` PostgreSQL/race
root: 25 roots, including the eleven preparation tests above, zero failures and
zero skips, in 97.036 seconds. This also exercises existing authoring, filters,
options, canvas, publication and prepared-origin rule fences. The dedicated
PostgreSQL fixture stopped cleanly. Before/after commit and worktree checks match.
Planning coherence passed 61 tests; it is not runtime acceptance.

Earlier failing fixture iterations and the stale pre-migration schema-count
assertion are not counted as aggregate passes. The final migration-count test
pins 087 and preserves every earlier migration identity. Independent scoped
backend review reported no open blocking static finding at this checkpoint.

Native capture hashes are unchanged from the actual exporter output:

- `dataset-native-public.json`: `5d6e8c2a185ab346098478d4edb7eeaa7effd1a8ed0af5fafb8c5cf5f7d3e532`
- `dataset-prepare-contract.json`: `156aee733fa735e1fbc273710448e4afad654e2cd55eb6ffddef745cadd8a7b6`

This evidence does not claim a full release, new Chromium-host qualification,
protected renderer/kernel validation, real Pengui integration or mixed-writer
rollout. No new scheduler, tool, local authority, production cleanup or deployment
is part of this change. Remote source checkpoints are separate from runtime
qualification.
