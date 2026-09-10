# Phase 26/28 runtime completion evidence

Status: implementation and direct adversarial review in progress. No independent
subagents were used, as requested by the owner. This document is not a runtime
completion claim.

The starting branch `1d412d9` contained domain services, PostgreSQL migrations and
named acceptance tests, but lacked application wiring, registered HTTP operations
and SDK integration. The runtime completion adds those consumers and changes the
phase ledger to `in_progress` so cumulative preflight cannot skip them.

Initial real PostgreSQL Phase 28 execution found an output-order mismatch and a
narrative test fixture that put the same field in both allowed and redacted lists.
The frozen lane now seals explicit request output order without changing the
existing authoring-preview order contract. The narrative fixture uses disjoint
lists. HTTP testing also exposed empty enum values in default run requests;
optional policy fields now omit empty values so the core's defaults apply.

Verification environment: macOS arm64, Go 1.26.4, PostgreSQL 17.11 and pgvector
0.8.2, native pinned SQL parser. Docker returned a local storage I/O error and its
existing PostgreSQL fixture was unreachable, so an isolated native database was
created for read-path tests. The managed runner intentionally requires Linux
process isolation and executable tmpfs; its evidence must come from Linux CI.

Phase 28 local race acceptance passed all eight criteria after the fixes, including
HTTP/SDK admission, execution, paging, cancellation and three additional lost-reply
checkpoints. This was a working-tree run, not hosted exact-commit evidence.

Pending: Phase 26 Linux runtime acceptance, direct adversarial review completion,
coverage, full cumulative checks and exact committed-source hosted CI. Phase 25,
34, live provider quality and production deployment are not claimed.
