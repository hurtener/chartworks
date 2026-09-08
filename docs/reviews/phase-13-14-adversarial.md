# Phases 13/14 adversarial review closure

Status: two full independent review rounds, the permitted narrow fix reviews and
the exact-head hosted gates are complete for the phase 13/14 implementation. The
reviewed scope has no open concrete P0 or P1 finding. This closes the bounded
review cycle and supports phase 13/14 shipped status; it does not mark the phase
25 full-release gate complete.

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

During the later coverage follow-up, a narrow local review found one reachable
P2: an over-wide cloud row could index past its verified schema. The local guard
at `b4942c1042ffa971286bb4fb13fbc09f2f28e2b9` now rejects both shorter and
longer rows with the existing typed `ErrType` before indexing and returns no
partial result. Its narrow diff review passed and did not reopen a full review
round or change the closed P0/P1 count.

Hosted CI later exposed a test-harness startup regression in the bounded
pipeline graph fuzz target. Commit
`811c7b47ed193d30be35a86b2ecc13c65693fe01` moves the existing valid-query WASM
parser initialization before the timed input callback without changing the
seeds, callback or input limit. Its narrow review passed. A frozen Linux Go
1.26.4 race check passed the known seed twice: cold setup took 13.383977132s
outside the callback, both callbacks took 0.00s and repeated setup took
269.583µs. This supports the cold-initialization diagnosis without reopening the
closed implementation review. The corrected harness passed the hosted fuzz gate
in qualifying run
[`34182486766`](https://github.com/hurtener/chartworks/actions/runs/34182486766)
at exact head `6883bc2103b2b870595222e623cd4d79bc01bc41`.

## Verification boundary

The [current evidence ledger](phase-13-current-evidence.md) records the exact
runner provenance, accepted local phase 13 and focused phase 14 results, the
owner-approved store-only coverage exception and the qualifying final hosted
run.
The complete runtime suite and Linux lint passed at `62f0362d458e`, but its
source coverage band failed. After the subsequent source contract tests and
local row guard, exact `b4942c1042ff` passed the cumulative race/coverage gate,
Linux lint and compiled-service foundation smoke. Hosted run `34171583231` then
failed when `FuzzPipelineDerivedGraph/seed#0` hung or terminated unexpectedly
during its bounded fuzz step. The later hosted run `34174537836` passed runtime
acceptance in 204.777s and its other jobs, but failed the exact
`internal/store/postgres` coverage floor at **2010/2379 (84.49%)**; later named,
fuzz, preflight, benchmark and hygiene gates were skipped. The follow-up
`fcf57f2`/`6883bc2` test change retains only the synchronized advisory-lock
cancellation regression; exact `6883bc2103b2b870595222e623cd4d79bc01bc41`
then passed the full Linux Go 1.26.4/race local coverage run with
`internal/store/postgres` at **2011/2379 (84.53%)**, `internal/sources` at
**2035/2534 (80.31%)** and `internal/engineering` at **2681/3330 (80.51%)**;
all configured bands passed. The exact profile is
`/tmp/chartworks-phase13-gates-2cdf7ef/coverage-6883bc2.out`. Hosted run
`34182486766` then passed all six hosted jobs on the exact committed source,
including the corrected fuzz gate, strict implemented-phase criteria, compiled
smoke, cumulative development preflight, microbenchmark baseline and hygiene.
Hosted store coverage was **2013/2379 (84.62%)**. Phase 13 and phase 14 are
shipped; phase 25 remains an unimplemented full-release gate, and recorded cloud
fixtures do not qualify live-cloud cutover.
