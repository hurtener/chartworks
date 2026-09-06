# Phase 10 — exec-read

Status: shipped. Owner: internal/exec. Hard dependencies: 08, 09.

## Authority and design

RFC-001 §9, RFC-002 §4/6, D-045/D-051 and [COMMON.md](COMMON.md) apply. One read execution core serves generated, submitted and frozen approved SQL; only exploration orchestrators may request a separately validated correction.

## Brief findings incorporated

Briefs 02, 03, 04, 14: read-only source behavior, ordering-preserving caps, cancellation, distinct empty/error outcomes and safe result contracts.

## Findings I'm departing from

No execution-time auto-rewrite inside this core, floating-point coercion of exact data, tenant-only result reuse or universal exactly-once claims.

## Scope and implementation tasks

1. Execute only validator-issued plans under real read-only credentials/session enforcement and current signed source/dataset/context reach.
2. Implement server/context timeouts, query IDs/cancellation/reconciliation and cursor row/byte caps without changing SQL ordering.
3. Normalize ordered schemas/rows with exact decimal and large-integer encoding; separate valid empty/truncated/error outcomes and collect attempts/cost evidence.

## Non-goals

No write interface, SQL string wrapper to inject LIMIT, hidden self-curation, local grants or permission expansion on retry.

## Config and persistence

`exec.rows_default=10000`, `rows_ceiling=100000`, `preview_rows=200`, `timeout=60s`; explicit result-byte/scan-cost ceilings and cancellation capability metadata. Attempt records distinguish issued query, accepted remote query ID, uncertain outcome and reconciled result. Persistent retention is phase 28, not an implicit unlimited row cache.

## Acceptance criteria

1. **AC01** — Real read-only sessions reject test writes; broader function/external access remains independently blocked by validation.
2. **AC02** — Cancellation/timeouts terminate or reconcile remote work where supported and expose unsupported/uncertain cancellation honestly.
3. **AC03** — Decimal values and 9007199254740993 survive transport; null/boolean/nonfinite/type errors are not silently coerced.
4. **AC04** — Default/ceiling row and byte limits apply to interactive and scheduled work; no LIMIT string wrapper changes ORDER BY or semantics.
5. **AC05** — Plan-only/context-only authority cannot execute; changed/mismatched source/context/restrictions require revalidation or denial.
6. **AC06** — Real-driver and race/failure fixtures record logical operations versus physical attempts; empty results do not trigger silent filter widening.

## Tests, coverage and smoke

Implement `TestPhase10/AC01` through `TestPhase10/AC06` against real read-only PostgreSQL, cancellation and large/exact data fixtures. Driver-specific suites extend the contract in phase 14. COMMON.md requires 85% exec coverage; `scripts/smoke/phase-10.sh` requires all six results.

## Glossary, decisions and deviations

Attempt and indeterminate outcome are not successful exactly-once execution. D-051 applies. Runtime implementation and all six executable acceptance criteria are supplied here; final exact-source CI is required before PR readiness.

## Implemented contract and review

D-065 and [read-execution.md](../contracts/read-execution.md) specify the actual
PostgreSQL cursor, exact type encodings, server/client/JWT deadlines, separate
source-revision fence, bounded response/optimizer admission and content-free
attempt journal. Migration 006 accompanies its consumers; applied 001–005 remain
unchanged. The [adversarial review](../reviews/phase-09-10-adversarial.md) records
real failures and fixes. HTTP and Go SDK operations use the same plan-only core.

`exec.bytes_default=4194304`, `bytes_ceiling=16777216`, `cancel_grace=2s`,
`planner_cost_ceiling=10000000`, `execution_concurrency=2` and
`max_read_attempts=3` supplement the row/time settings above. Optimizer cost is an
estimate; PostgreSQL does not implement a hard scan-byte ceiling and reports actual
scan bytes as unknown. There is no misleading scan_bytes setting. Attempt records
have a bounded 24-hour replay window and fixed admission capacity; uncertain
records require reconciliation rather than automatic erasure. Values are not retained.

The common core's caps apply to later scheduled/frozen consumers without a mode
bypass. Actual scheduled reporting targets remain phase 30 and are not fabricated
to close phase 10. Phase 09 was already merged and all six of its criteria remain.
