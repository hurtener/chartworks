# Phase 13 current acceptance evidence

Status: implementation in progress; final exact-head verification pending. PR #9
merged the phase 01–12 baseline at
`f4b159430d8c365c7348bea0bdcc04ba6193262f`. It is not phase 13 evidence.

| Source | Verified result | Acceptance boundary |
|---|---|---|
| `e3536986395766ef8e33e777591808eb52062eb8` | The full race-enabled instrumented test execution passed. | The repository coverage gate then failed because `internal/store/postgres` measured 83.68% (1954/2335), below the unchanged 85% requirement. |
| `2cdf7ef9c190a04d27d4170749fba9e0d173e14e` | `TestPhase13PipelineStoreTransitions` passed in 33.14s; `TestPipelineRejectsDifferentInputDatabase` passed in 7.09s; `go vet ./...` passed. | `TestPipelinePublishesTwoOutputsWithOneReadConnection` later exposed a source-binding failure, so this commit is not an accepted head. |
| `474b5d0ed167ed3c03cd84359f4aaadb3fa257f0` | `TestPhase13/AC01`–`AC06` all passed under race detection in 73.73s against PostgreSQL 17 over verified TLS and the pinned Bruin CLI; AC06 exercised merge, interval, SCD2, replace, append and incremental. `TestPipelinePublishesTwoOutputsWithOneReadConnection` separately passed in 33.96s on Linux arm64 with Go 1.26.4 and CGo, using one read-pool connection, normal discovery plus validated read, two outputs and both retained values. The exact-head Chartworks binary passed a live foundation smoke: 185.66ms liveness, 54,996 KiB `VmRSS`, 11 threads and clean shutdown. | The named phase acceptance, focused regression and binary/runtime checks passed. The full repository race/coverage gate has not yet passed on this exact head; the available diagnostic profile covered only three packages. |
| Bruin fork `cbde66f665090a2848f583f9d1043d3299d5363d` | Race-enabled tests passed for the BigQuery, Snowflake, Databricks, MySQL and SQL Server leaf packages. | Cloud protocol inputs were synthetic/recorded. This is neither live cloud qualification nor phase 14 closure. |
| Pinned Bruin Linux arm64 runner CLI | Version `v0.11.749` at fork commit `cbde66f665090a2848f583f9d1043d3299d5363d`; SHA-256 `558a8ac85952c0a56bb22fc4efb010e16b66f8f4638a0230d57072b311926546`. | This identifies the locally exercised runner artifact; the final release environment must retain the same explicit provenance. |

Hosted CI run `34159347833` targeted remote commit
`9fa9e86f502d75adb9d732437cf978679bfa3360`. Its native Linux Go/Rust job passed
using Rust 1.98.1 through rustup, while the reference-container job failed because
the configured `rust:1.98.1-bookworm` image tag does not exist. The run predates the
current source fixes and cannot qualify the final head.

## Owner-approved package coverage band

On 2026-09-07 the owner explicitly approved: "84.5% is approved by my standards".
The configured exception is exactly **84.5% for `internal/store/postgres` only**;
all other package thresholds and the default coverage policy remain unchanged.
The gate compares integer statement counts against exact basis points, so a
lower value that merely displays as 84.50% cannot pass this threshold.

The supporting measurement was **1974/2336 statements (84.50% displayed)** from
the partial cumulative race suite at
`4ace97e1471643f2de0fad7df111249f57041975`. Phase 14 native acceptance was excluded,
and the run failed an outdated phase 09 assertion that expected MySQL to remain
unsupported. This is measured coverage evidence, not a full-suite pass. The
exception changes only the package minimum; the complete native CI and
race-enabled coverage execution must still pass on the final committed source.

Do not change the phase registry to shipped until current committed source supplies
every required named result, coverage band and repository gate. Local runner
evidence must retain the exact executable SHA, embedded parser/runtime dependency
and private bounded cache/scratch environment; it is not cloud-engine evidence or
phase 14 completion.
