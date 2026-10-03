# Renderer charged-memory recovery checkpoint

Date: 2026-10-01. Stacked on reporting checkpoint `608f2d98f42a895927e551bf403b24bb8fdbd382`
and SQL/topic checkpoint `9ada852c5a78490dff5ba9aeb300e9d927031522`.
This is an incremental implementation and local evidence record, not release or
deployment qualification. [D-093](../decisions/2026-10-01-renderer-charged-memory.md)
is the normative containment contract.

## Recovered implementation and review corrections

The interrupted thirty-file implementation was recovered from exact saved source
and every file checked against its SHA256 review manifest before modification.
No source was reconstructed from a prose description. The approved decision-log
cleanup removes historical identifiers/paths without changing technical decisions.

Every isolated render requires a fresh cgroup v2 leaf with charged-memory, swap,
HugeTLB, CPU and task controls. The fixed 3 GiB virtual-address ceiling is separate
from the at-most 1 GiB charged-memory budget. Missing kernel support fails before
worker execution; no ordinary-process or in-process fallback was introduced.

Review identified and corrected two implementation defects:

- Arbitrary configured byte budgets are rounded down to whole kernel pages before
  programming and exact readback. Kernel rounding cannot increase the requested
  budget or make a valid nonaligned configuration fail every job
- Cleanup/accounting retain preopened CLOEXEC controller descriptors. A worker
  with the manager's mapped host UID cannot disable supervisor cleanup merely by
  revoking control-file or directory modes after launch. Offset-reset bounded
  reads and serialized control access preserve repeated use. Setup and terminal
  cleanup failures remain distinct errors and disable further admission

Pure tests cover retained descriptors after file/directory mode changes, CLOEXEC,
page alignment at multiple page sizes, and prelaunch cleanup failure reporting.
The mandatory kernel suite additionally includes a hostile permission-revocation
probe under an unprivileged manager; local pure tests do not reproduce that kernel
attack or prove isolation.

Both reporting and final-core workflows accept an optional existing trusted Linux
runner label and delegated cgroup root through repository variables. These select
already provisioned infrastructure only; the workflows do not create delegation,
remount controllers, or change security settings. The hosted default remains
fail-closed if it cannot satisfy the contract.

## Executed local checks

Go 1.27.1, Linux amd64, pinned native parser base and patch, PostgreSQL 17.6 with
pgvector 0.8.2, synthetic disposable databases, recorded provider responses only.
Native executable and library checksums matched their saved manifest.

- Recovered affected-package baseline: 564 passing race-test events in seven
  packages; zero failures or skips
- Final renderer, containment, BFF and configuration suites after review fixes:
  230 passing race-test events; zero failures or skips
- Retained PostgreSQL/native lifecycle suite: all fourteen required families,
  44 passing test/subtest events, zero failures or skips; 157.414 seconds
- Functional Phase32 AC01/AC02/AC06/AC07/AC08: all five selected criteria passed
  (six events including the parent), zero skips; 37.528 seconds. The three
  isolation-dependent criteria remain unqualified here
- Full `go build ./...` and `go vet ./...`: passed
- Viewer disclosure/display unit checks: six passes, zero skips
- Planning/document checks: passed; these are not runtime acceptance
- Kernel integration tests compile and fail both top-level gates because this
  executor has no preconfigured cgroup hierarchy. No kernel-enforcement,
  hundred-render stability, simultaneous isolation or hostile-mode-change pass is
  claimed

The broad affected-package baseline above predates the review fixes; the final
renderer suite and retained-source checks cover the changed implementation. A
prior source run was interrupted by host memory pressure after 43 passing events;
it is not counted as a suite pass. The full rerun above completed cleanly.

## Open qualification

A suitable already authorized host must pass the full Phase32 and mandatory kernel
suites, including unprivileged mode revocation, per-job and aggregate memory,
address-space independence, leak/tamper probes, cancellation, and one hundred real
representative concurrent renders. The prior hosted reporting checkpoint passed
actual browser/scheduling/retention contracts but failed isolated worker launch.
Live model quality, remaining analytical software/engine breadth, and final
Phase34/25 evidence remain separate open gates in the
[completion tracker](sql-recovery-completion.md). No merge or deployment is implied.
