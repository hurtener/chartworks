# Phases 11–12: request recovery and source-fence follow-up

Status: fixes and regressions prepared against `630f9e098fbf15aa823dbf9a452b8539c547066b`; runtime verification is still required. This supplements the [original adversarial self-review](phase-11-12-adversarial.md), not an independent audit or a declaration that the owning phases have shipped.

## Findings addressed

1. **Expired running owners could not be explicitly resumed.** `ClaimRequest` already supports an expired running lease, but `ResumeRequest` rejected the state before the claim path could run. Resume now checks expiry under the operation row lock using the database clock. It leaves the old lease, acquiring attempt, manifest, timestamps and attempt count intact. The existing claim helper alone abandons that attempt and increments the fence. Live owners and exhausted attempt budgets still deny resume. Lease expiry is not evidence that warehouse work stopped; the upload/profile reconciliation paths are unchanged.
2. **A lost post-commit reply could contradict confirmed success.** The runner could return an error after its final authorized ledger read proved that domain publication and operation completion had committed. A readable `succeeded` receipt now resolves that ambiguity; a nil handler return, unavailable receipt, expired caller or uncompleted operation does not. The cancellation/publication transaction fence remains the decision point.
3. **Dependency insertion lacked the source-deletion fence.** The profile-head advisory lock serializes publication but not source tombstoning. Registration now locks the live source row before the head lock and retains that lock through reference insertion. It does not add authority or change the existing stale-head conflict behavior. PostgreSQL's `FOR SHARE` row lock conflicts with the UPDATE used to tombstone the source; an advisory lock alone does not provide that exclusion.
4. **Profiling erased specific native failure reasons.** The common executor deliberately returns sealed failed attempts with a nil Go error. The profile consumer now translates the fixed receipt codes for limits, context changes, unsupported capabilities, result types, cancellation and uncertainty instead of flattening them into a dependency outage. Unknown driver messages are not copied to public output. The executor itself and its limits remain unchanged.
5. **Cell normalization could exceed the configured per-cell limit.** The parser checked cell length before normalization only. Canonical number text can be longer than the input; it is now checked again before emitting the row to the managed writer. The exact limit is accepted and an over-limit normalized cell is rejected without emission.

## Added executable evidence

- `TestUploadExplicitResumeReclaimsExpiredOwner`: real staged upload and leased operation, live/foreign-caller denial, observed database-clock lease expiry, preserved pre-claim evidence, and one fenced load into the common reader.
- `TestProfileExplicitResumeReusesExpiredOwnersCheckpoint`: real sampling/checkpoint plus injected process-loss state; recovery keeps the deterministic version and does not repeat the native read. This is a failure-injection test, not an OS process-kill claim.
- `TestExpiredRequestResumeCannotResetAttemptBudget` and `TestRequestHandlerCannotInventPublicationSuccess`: no attempt-limit reset and no fabricated success from a nil handler return.
- `TestEngineeringCommitReplyLossUsesConfirmedReceipt`: real upload activation, profile publication and erasure commits followed by injected reply loss; the final ledger read must resolve each outcome accurately.
- `TestProfileDependencyRegistrationFencesSourceDeletion`: observe registration blocked on the real head lock, prove the source row is already locked using a conflicting `NOWAIT` probe, complete registration, then erase and reject late references.
- `TestProfileFailurePreservesSealedReadReason` and `TestProfilePlannerLimitPreservesExecutorFailureCode`: fixed-code unit cases and a real planner-limit failure matched to its stored native receipt.
- `TestUploadCellLimitAppliesAfterNormalization`: canonical-value growth, zero emission on rejection, and exact-boundary acceptance.

## Verification boundary

The local container and Python probes returned `ClientError` before execution during this follow-up. Source review and Git object comparison can confirm the patch but cannot establish a successful build, formatting, coverage, database race behavior or acceptance results. Run these regressions together with all 76 implemented phase criteria and the full uncached race-enabled suite using the existing permanent CI. No existing test, migration 001–006, coverage threshold, permission check or phase-09/10 executor is relaxed to obtain a green result. The phase registry remains in progress until runtime evidence exists.

References: PostgreSQL 17 explicit-locking contract (`https://www.postgresql.org/docs/17/explicit-locking.html`) and the repository's [shared completion contract](../plans/COMMON.md). Database fixtures, not this description, must demonstrate the lock and recovery properties.
