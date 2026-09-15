# Phase 29 — reports/dashboard runtime adversarial review

## Scope and provenance

Continuation of `feat/phase-29-reports-dashboards` from
`3a4028987e6ffae863ccbb28c5ad7de08631a1a1`, based on merged main
`6f0001dbd6370dff300cab472405088d7b85fead`. This supersedes the unfinished-runtime
statements in [the recovery ledger](phase-29-development-recovery.md), not its
historical evidence. Phase 29 remains `in_progress` until reviewed/merged.

The implementation adds document revision/lifecycle storage, exact dashboard
pages, immutable composition admission, shared-block output unions, opt-in saved
query execution, retained widget reads, cancellation and expiry. The existing
HTTP registry, host wiring and typed SDK consume all 26 new operations; cumulative
Phase 21 transport tests cover that inventory. No new authority issuer, SQL
executor, gateway client, source driver or queue was introduced.

## Findings and disposition

| Adversarial question | Disposition and executable evidence |
|---|---|
| Can an unavailable optional narrative discard a successful table sharing its query? | Fixed admission to permit explicit failed narrative receipts in the partial child lane. Parent strict/partial policy remains independent. `TestReportingCompositionOutputSubsets`, `TestReportingCompositionBoundaries/independent-output-failure`. |
| Does partial output mode bypass narrative consent, schema compatibility or declared budgets? | No. Explicit opt-in remains required; unavailable/version-mismatched outputs fail before model I/O, and declared call/token ceilings still apply. `TestFrozenOutputSelectionAvailabilityAndConsent`, `TestFrozenPartialNarrativeDeclaredBudgets`, `TestFrozenUnavailableNarrativeReceipt`. |
| Can uncertain storage cause completed source/model work to repeat? | Child and parent reply-loss tests preserve durable results. A lost dynamic-plan reply recovers the actual saved NLQ plan; absent evidence stays indeterminate without fresh generation. `TestReportingCompositionRecovery`, `TestReportingCompositionDynamicRecovery`. Invalid operation coordinates and changed recovery evidence are denied. |
| Are lifecycle pointers, reference graphs, operation admission and quotas atomic with audit? | Real PostgreSQL insert/update/audit faults roll back all affected state. `TestDocumentAuditRollback`, `TestDocumentGraphRollback`, `TestReportingCompositionAdmissionRollback`, `TestReportingCompositionCheckpointRollback`. These exercise real typed domain proofs, foreign keys, immutable guards and common operation fences, not success-returning repository stubs. |
| Can cancellation without current action, target, dependency or preview reach alter work? | Denied independently. A late cancellation-audit failure rolls back both common operation intent and composition state; successful replay emits one audit. `TestReportingCompositionControlBoundaries`. Cancellation intent does not claim native termination. |
| Do expiry triggers prevent actual erasure, as previously suspected? | The recovered implementation already uses expiry-aware DELETE guards. No migration change was needed. `TestReportingCompositionRetention` waits for the supported real one-minute expiry, erases completed/private/sealed/cancelled values, retains a live comparison artifact, and verifies late-audit failure rolls back erasure and byte accounting. No clock rewrite, disabled guard or manufactured proof is used. |
| Can publication or retained reads widen authority or expose hidden pages? | Private artifacts stay private; current page/context reach gates metadata and values. Missing sessions, same-tenant different-context reads, zero-visible-page projections and cross-tenant denial remain covered by the original storage/boundary suites. Metadata reads do no source/model work. |
| Are examples and cumulative schema expectations executable? | The checked-in report/dashboard examples are submitted as their original bytes through closed HTTP schemas, reviewed/published, executed and read through the real SDK. `TestDocumentHTTPContracts`. `TestSafeErrors` now expects migrations 29–30 while preserving the existing migration-28 schedule-admission checks. |

The five outstanding lint findings were corrected without disabling analyzers or
changing runtime semantics. Coverage bands, acceptance criteria, signed-scope
ceilings and runtime phase selection were not lowered or bypassed.

## Verification procedure

Local tests use Go 1.26.4, the pinned native Bruin/Rust parser, PostgreSQL 17.10 with
pgvector 0.8.2 and golangci-lint 2.12.2. Those binaries were recovered through
read-only preparation artifacts. Preparation success is not implementation
verification. Recorded gateway fixtures are not live provider qualification.

The continuation ran the originally failing race-enabled regressions, the new
narrative-selection tests, real retention and audit-rollback tests, dynamic
recovery, imported-reference graph rollback, first-seal rollback, cancellation
and checkpoint rollback, and the named Phase 29 acceptance runner. The current
`TestPhase29/AC01`–`AC08` mapping includes these scenarios instead of merely
counting new test names.

Commands for repeatable validation, after installing the repository's pinned
native dependencies and supplying disposable fixture connections:

```sh
go test -race -count=1 -timeout=15m ./internal/reporting ./internal/reportingapi \
  ./internal/nlqexec ./internal/nlqroute ./internal/config ./sdk/chartworks
go test -race -count=1 -timeout=15m ./test/acceptance \
  -run 'TestDocument|TestReportingComposition|TestSavedQuestion'
python3 scripts/run_phase_acceptance.py --phase 29
golangci-lint run --timeout=10m
make check-mirror planning-check drift-audit
make coverage
make preflight-full
```

The first local full-coverage attempt exposed the stale schema-version assertion;
it was stopped after that failure and is **not** a passing full-suite result.
Targeted instrumented tests were used to find untested failure paths. Combining
older instrumentation with those tests is diagnostic only, never the full-suite
coverage gate. The final CI run must execute the unmodified exact committed tree,
all applicable native/source fixtures and the existing coverage thresholds. The
PR records that exact SHA and its check results separately; this document does
not pre-declare an unobserved CI result.

## Non-goals and remaining release boundaries

Phase 29 is API-first, not a visual report builder. Phase 30 owns report schedules;
31 owns the Apps viewer; later phases own static rendering/export and cutover.
The phase-25 final release gate remains planned. Report publication does not
certify dynamic SQL. Parent expiry is not a purge of independently retained child
frozen runs. No platform build or fixture pass is a production cutover claim.

## Cumulative schema and failure-surface continuation

The full race/coverage run for source tree
`d27e05984ec77569d787eb135ece2352178e72b9` failed at `TestPhase02/AC04`:
its explicit foundation schema inventory omitted the 17 domain tables added by
migrations 29–30. The saved instrumentation met all 42 unchanged package bands,
but a failed Go test invocation is not a passing coverage gate.

The cumulative inventory now explicitly includes the document and composition
tables. Exact table equality and the separate ban on local IAM/issuer material
remain enforced; the expected list is not derived from the implementation being
tested. CI also runs Phase 02 before the expensive container/coverage steps so a
future cumulative migration mismatch fails with named acceptance diagnostics.

Additional acceptance exercises caller JSON against the sealed document,
quarantine, composition-admission and checkpoint proof types. Ordinary verified
scope does not turn caller data into a service-prepared proof or a leased fence;
rejected attempts leave no document/quarantine/composition records. A real
PostgreSQL retained text artifact supplies the positive control for a matrix of
unverified authority, cancelled requests and a closed actual connection pool.
Document reads/listing, execution recovery, artifact metadata, widget payloads,
cancellation, retention and creation return the precise failure category and no
payload. Independent SQL inspection verifies unchanged durable state and audit
counts; source/model counters stay unchanged. The new cases are included in
Phase 29 AC01/AC08, not only discovered as unrelated test names.

These regression additions do not change production execution behavior, coverage
thresholds, package inventory, timeouts or race instrumentation. The PR records
observed verification against the final source separately.
