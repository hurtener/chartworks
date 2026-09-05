# Phase 01–02 verification evidence

Date: 2026-09-05. Source implementation verified before PR creation: `f5c5bc48c71b4dcbed067d165c4384d62bf75f53`. GitHub Actions run `33971906736`, job `101321791607`, printed that exact `TESTED_COMMIT` after source formatting and dependency lock normalization, then passed build, vet, race-enabled real-store coverage, both phase acceptance suites, planning/tool tests, benchmarks and golangci-lint v2.12.2 with zero issues.

This evidence is not based on skipped planning tests: every `TestPhase01/AC01`–`AC06` and `TestPhase02/AC01`–`AC06` passed; each phase runner reported `unimplemented_skips=0`. Thirty-one Python planning/archive/coverage regression tests passed. PostgreSQL 17 and real version-17 dump/restore clients were used. No live LLM or customer warehouse was involved or needed by these phases.

## Measured package statement coverage

| Package | Covered/total statements | Measured | Required |
|---|---:|---:|---:|
| `cmd/chartworks` | 3/4 | 75.00% | 70% |
| `internal/config` | 210/219 | 95.89% | 80% |
| `internal/foundation` | 297/312 | 95.19% | 80% |
| `internal/maintenance` | 15/17 | 88.24% | 80% |
| `internal/store` | 16/16 | 100.00% | 85% |
| `internal/store/postgres` | 243/276 | 88.04% | 85% |
| `internal/telemetry` | 38/40 | 95.00% | 80% |

Coverage combines executed blocks from the full race-enabled suite, including actual cross-package PostgreSQL callers. It is not an average of function percentages or a silent relaxation of store thresholds.

## Microbenchmarks

Environment: Ubuntu 24.04 GitHub hosted runner, Go 1.26.4 linux/amd64, Intel Xeon 6973P-C, GOMAXPROCS=2; 100 iterations. Configuration parse: 13,209 ns/op, 6,656 B/op, 122 allocations. In-process health handler: 2,968 ns/op, 6,282 B/op, 21 allocations including the test request/recorder. These are baselines, not full-process startup, warehouse performance or user-facing response-time guarantees. The compiled-process smoke additionally records observed startup/RSS in its actual CI environment without a fabricated target.

## Final-tree verification and scope

The temporary write-enabled branch formatter has been removed from the delivered tree. The final CI is read-only, does not generate commits, checks go.mod/go.sum and formatting for drift, builds the actual binary, runs the same real-store coverage and named acceptance, exercises compiled commands/HTTP/SIGTERM, reruns full preflight, and cross-compiles linux/amd64 and darwin/arm64. The PR body identifies the exact final head and successful read-only workflow run.

The [adversarial self-review](phase-01-02-adversarial.md) was performed before opening the PR; its discovered bugs have executable regression coverage. The reviewer is the implementer, not an independent certification authority. Phase status means implemented/verified, not merged, deployed or tagged. The other 32 numbered phases remain planned and the all-product release gate must still reject them.
