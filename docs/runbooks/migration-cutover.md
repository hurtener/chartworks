# Migration cohort runbook

1. Produce a neutral `chartworks-migration-v1` manifest outside the service. Remove
   credentials and identity policy. Include every object field disposition, all 63
   feature evidence rows, exact destination mappings, retention and one schedule
   occurrence boundary.
2. Call `migrationDryRun`. Resolve every rejected mapping and every failed or
   unsupported required row. Save the returned digest and loss ledger with the
   operator change record. Dry run performs no import.
3. Call `migrationImport` with `expected_revision: 0`. If the call ends without a
   result, read the operator record and call `migrationResume` with the last returned
   exact revision. Do not create a new batch ID to conceal an uncertain attempt.
4. Export bounded pages and compare the stored digest, object count, quarantine
   count, retention, dialect/source mapping and occurrence boundary. Exercise normal
   domain reads with current scoped bearers; migration authority is insufficient.
5. Stop the old cohort dispatcher after its recorded `last_accepted` occurrence.
   Confirm the target scheduler will resume strictly after the manifest boundary.
   Call `migrationCutover` with generation 0 (or the current exact generation), the
   target route and the external change/drill reference.
6. Observe one complete target occurrence and verify no duplicate logical occurrence
   was admitted. Keep Phase 25 release status unchanged until its full gates pass.
7. For rollback, stop the target dispatcher, enumerate already delivered or committed
   effects, then call `migrationRollback` with the current exact generation and that
   effect list. Verify the prior route resumes after the retained boundary. Never
   report listed effects as undone.
8. After retention and when no active cutover references the batch, call
   `migrationErase` in bounded pages until `remaining` is zero. Record that online
   payload was removed and apply the operator backup/replica/WAL retention policy.

Abort cutover when the plan is not ready, a destination mapping changes, current
authority is missing, a source/profile revision moved, an unsupported engine is
present, the batch is incomplete, or the scheduler boundary cannot be proved.
