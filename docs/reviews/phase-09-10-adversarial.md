# Phases 09/10 — adversarial self-review

Based on merged phases 01–09 at `280068cef978df9fc4165b38dd6596634d4a4c4c`.
This is an incremental implementation and self-review, not an independent audit.
Phase 09 was already implemented in PR #7 and is retained as the read prerequisite.

## Findings and executable evidence

| Finding | Correction and regression |
| --- | --- |
| Five-second metadata transaction outlives neither a 60-second read nor cleanup | Separate bounded source-revision fence; `TestReadRevisionFenceExceedsMetadataTimeout` holds a real read beyond five seconds, proves a competing revision update remains blocked, then completes the read. |
| Prepared FETCH metadata can describe a previous cursor on a reused pooled connection | Obtain each actual description with DescribeExec; `TestReadSameConnectionAlternatesActualResultSchema` alternates integer, money/decimal and boolean/text shapes on one connection. |
| PID-based cancellation can target a different query after reuse | No external pg_cancel_backend path; persist cancellation intent, and the live owner alone cancels its original connection. Exact tagged backend observation is separate from signaling. |
| Cancellation may arrive after result collection but before receipt commit | Database CASE/fence resolves the final status; executor reloads it and drops values. `TestReadLateCancellationCannotPublishRows` injects a real cancellation at finalization. |
| Missing audit registration prevents all read admission | Migration 006 extends the closed audit vocabulary without changing 001–005; real attempted admission exposed the failure instead of a fake in-memory ledger passing. |
| Final audit/receipt failure can leak unrecorded success | `TestReadJournalFailureDropsValues` injects a real audit failure and verifies rollback, no returned values, uncertainty and later observed reconciliation. |
| Lost response can cause hidden physical retries | Actor/session-bound operation lookup and explicit attempt numbering; `TestReadUncertaintyRequiresObservedReconciliation` loses a real native reply, blocks retry until observation, and never recreates successful rows. |
| Same logical key can race into multiple queries | Tenant-serialized admission, immutable manifests and unique attempt keys; `TestReadDuplicateAdmissionAndJournalPrivacy` races twelve callers and requires one physical read. |
| Decimal/large integer/money data can silently round or nonfinite values can become NULL | Native ordered type metadata and exact encodings; phase10 AC03 plus native-money extrema tests, with whole-result rejection for malformed/unqualified/nonfinite values. |
| Client row/byte limits can be mistaken for warehouse scan budgets | Actual serialized response cap plus explicit optimizer estimate and unknown actual scan bytes; phase10 AC04 and capability receipts. |
| Empty output can trigger unapproved SQL widening | No model or rewrite dependency in the core, no automatic retries; phase10 AC06 preserves the empty schema and original operation. |
| HTTP/SDK transport can accept conflicting authority/limits or truncate large valid responses | Closed typed decoder, negative transport tests, route-inventory parity and a real >1 MiB response in `TestReadAPIAndSDK`. |

## Additional final-review findings

- `TestReadReconciliationCannotSwitchCredentialContext` changes the actual
  credential/database without rotation. Reconciliation re-probes the current
  technical binding under the source-revision fence before observing an old
  query; a foreign database cannot falsely prove termination.
- `TestReadOldCancellationCannotSignalReusedBackend` proves actual PID reuse,
  cancels the old uncertain attempt and verifies the new tagged transaction is
  still active. Only its own owner can cancel it.
- Retired credential pools are bounded to one per alias and joined on shutdown;
  a short control request no longer synchronously waits for an old 60-second read.
- Late cancellation now also controls the committed audit action. The regression
  requires zero `read.succeeded` records and one `read.cancelled` record.
- Request ceilings are checked before HTTP-triggered native planning, and
  PostgreSQL-17 transaction/idle-transaction SQLSTATEs retain typed timeout errors.
- The whole-suite schema-version assertion now includes actual migration 006;
  no migration/acceptance check or coverage threshold was bypassed.

## Preserved authority and lifecycle checks

The existing phase-09 six named criteria, whole-tree parser negatives, hidden-column
alias oracles, unknown AST-node mutations, native dry planning and zero-plan/JSON
unforgeability remain. Phase10 AC05 adds execution-context-only and cross-tenant
negatives, actor-bound control, stale rotation and zero warehouse work on denial.
No token, source credential, SQL or parameter values are retained by the attempt
journal. Source metadata and terminal attempt inspection make no learned-model
calls; no model gateway is injected into the executor.

## Executed iterations and final gate

Development run `34010111971` tested `738940f612d246f680e444fc9dc802699e87b901`.
It passed phase08/09 and the real extended revision-fence regression, then failed
phase10 admission because the closed audit constraint lacked the new read actions.
That run is not represented as a completed implementation or coverage pass.
Subsequent corrections are verified against their actual committed source.

Final readiness requires all six phase10 criteria, all 58 preceding criteria,
whole-repository uncached race-enabled coverage at unchanged thresholds, native
PostgreSQL/pgvector recovery, build/vet/lint, preserved SQL/authority fuzz campaigns,
configuration and schema parity, compiled lifecycle smoke, planning/drift/mirror,
CGo-free Linux amd64/macOS arm64 builds and an unchanged checkout. Final exact-head
and PR integration run links, outcomes and measurements belong in the PR after
those runs complete; this document does not pre-claim their results.

## Boundaries

Qualified PostgreSQL 17 only. This is not a multi-engine or full-product release,
a paid-provider test or a deployment. Scheduled reporting handlers and retained
result artifacts remain phases 30 and 28. Uncertain observations remain unknown,
optimizer cost is not actual scanned bytes, offline authority ends at its JWT
boundary, and explicit replay protection is bounded by the retained receipt window.

## Finalization discipline

All new runtime assertions are retained, including actual PID reuse, credential
context substitution, late-cancellation audit truth and the reference configuration
through the closed decoder with required Pengui verifier settings. Migration 006
is reflected in the cumulative schema-version assertion. No threshold, criterion,
fuzz seed or race instrumentation has been removed to resolve a failure.

Permanent CI now requires all 64 implemented criteria, including phase10, and
rejects source preparation scripts and temporary development/workspace workflows.
Those files are removed before the final verification commit. The final PR records
exact tested source, completed runs and any remaining qualified boundaries.
