# Phases 13/14 adversarial review closure

Status: two full independent review rounds and the permitted narrow fix reviews
are complete for the phase 13/14 implementation. The reviewed scope has no open
concrete P0 or P1 finding. This closes the bounded review cycle; it does not mark
either phase shipped or convert pending runtime and hosted gates into passes.

## Reviewed boundaries

The reviews covered the managed-pipeline definition, validation, authority,
runner, store-transition and atomic-publication paths; PostgreSQL, MySQL and SQL
Server source planning/execution; cloud adapter lifecycle and context fencing;
and the pinned fork protocol changes consumed by Chartworks. Phase 15 work and
unrelated earlier capabilities were outside this review scope.

The final fixes retained these required properties:

- pipeline validation, publication and execution bind the managed writer to the
  validated source database and the held object manifest;
- multi-output publication uses one metadata transaction and does not borrow one
  pool connection per output;
- private pipeline stages use exact accepted pipeline authority while ordinary
  source reads retain their source/query/context requirements;
- pipeline job inspection and cancellation enforce their registered job actions
  and scoped domain reach;
- an enabled service verifies its configured runner at startup, while disabled
  deployments retain metadata access;
- cloud lifecycle state is fenced to the verified execution context, and fork
  fixtures remain distinct from live cloud qualification;
- SQL Server parameter declarations and cancellation start from the actual
  dispatched operation without broadening supported syntax or claiming cleanup
  that the driver did not acknowledge; and
- MySQL acknowledged cancellation reports `stopped` only after the original
  owned transaction returns an explicit rollback acknowledgement. A bare
  `ErrTxDone`, socket loss or bounded grace fallback remains `unknown` and
  exposes no partial result.

The last SQL Server fixes were
`d54573d5aebc116fec5ab7216a7c518d4a6f8bb4` and
`71b9bd6a872ccf70426c7013fe1c3d345fe8eb35`. Reviewer B's narrow diff-only
rereview passed with no P0/P1 finding. Earlier P1 fixes received their permitted
narrow rereviews; no full review round was reopened.

The later MySQL cleanup correction is Chartworks
`6ff29d9bf30fab3bc433019b6131c7ab72a686a4` with fork
`5f562c2959496a04d57f5f199f5e3ad22159fa9f`. The narrow review accepted its
explicit rollback receipt, caller-cancellation handoff and bounded slow-drain
fallback without reopening the full review cycle.

## Verification boundary

The [current evidence ledger](phase-13-current-evidence.md) records the exact
runner provenance, accepted local phase 13 and focused phase 14 results, the
owner-approved store-only coverage exception and every unfinished final gate.
The most recent focused fixes are not yet covered by a final-head cumulative
race/coverage result. Linux-amd64 native SQL Server CI, final lint and hosted CI
also remain pending. Those results, rather than this review record, determine
whether the in-progress phase statuses can advance.
