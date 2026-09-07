# Phases 13/14 current acceptance evidence

Status: implementation and review are in progress; final exact-head gates remain
pending. PR #9 merged the phase 01–12 baseline at
`f4b159430d8c365c7348bea0bdcc04ba6193262f`; that merge is not phase 13 or 14
evidence. Review closure is recorded separately in the
[phase 13/14 adversarial review](phase-13-14-adversarial.md).

## Reproducible runner provenance

The current immutable D-067 Bruin source is
`02638a8f05342f6c18dd835a8d57301d0af94f77`, derived from v0.11.749. Its native
`libbruin_rustsqlparser.a`, built with exact Rust 1.98.1, had SHA-256
`82736d93282fe2f572715a25fcecce8a5c911c1fc5589b43881e295700be3243`. The Linux
arm64 Go 1.26.4 runner, built with the documented reference flags, had SHA-256
`a724de2b5a2c3230e5f32bf90a85446cba221b5d2e5f8a6754d6840ee310be6d`.
These digests identify the locally exercised source and executable. They do not
constitute live cloud qualification.

## Accepted local runtime results

| Scope | Exact source | Observed result | Remaining boundary |
|---|---|---|---|
| Phase 13 acceptance | `7a69610bb5003c9aaf11fd5645d591c5481cc273` | `TestPhase13/AC01`–`AC06` passed under race detection in 99.84s, including all six strategy branches. | This predates later fixture-only and phase 14 fixes; final-head cumulative acceptance remains pending. |
| Pipeline authority and lifecycle | `7a69610bb5003c9aaf11fd5645d591c5481cc273` | The exact non-wildcard Draft–Publish–Run and control regression passed in 8.63s; enabled-runner startup passed in 1.86s; one-pool/two-output publication passed in 8.52s; SDK coverage passed in 4.24s. | These focused results do not replace the cumulative gate. |
| Phase 13 package races | `7a69610bb5003c9aaf11fd5645d591c5481cc273` | `internal/exec` passed in 36.895s, `internal/sources` in 39.706s, `internal/engineering` in 40.698s and `internal/store/postgres` in 1.260s. | Package passes are not a repository coverage result. |
| Cross-database proposal regression | `38823a4eddef456eacca4b035f452d79c483bb0a` | The real PostgreSQL/TLS/race fixture passed in 12.35s after the test service retained its explicitly configured model gateway. | The earlier whole acceptance command stopped on the fixture's discarded gateway; it was not an accepted whole-suite pass. |
| Compiled service lifecycle | `38823a4eddef456eacca4b035f452d79c483bb0a` | The compiled binary passed live foundation HTTP smoke: 221.24ms liveness, 51,604 KiB `VmRSS`, seven threads, command/health/protected-negative-route checks, redaction and clean SIGTERM shutdown. | This is a local service lifecycle check, not cumulative acceptance or live cloud evidence. |
| SQL Server parameter and cancellation behavior | `d54573d5aebc116fec5ab7216a7c518d4a6f8bb4` and `71b9bd6a872ccf70426c7013fe1c3d345fe8eb35` | Local SQL Server emulation passed the source unit race suite in 1.181s, ten parameter cases in 6.06s and `TestPhase14/AC04` under race detection in 23.333s. | Linux-amd64 native SQL Server CI remains pending; emulation is not a substitute. |
| Self-contained MySQL probe fixture | `7ead982755c96e32471d450bc9c5e4e3e8b33a47` | The focused race test passed in 1.558s with a random reader role and database. | The preceding full coverage run at `38823a4eddef456eacca4b035f452d79c483bb0a` failed the old environment-dependent probe and a separate reachable MySQL acknowledged-cancellation case. Full coverage must be rerun after both fixes. |

Fork leaf-client SDK/HTTP fixtures and injected Chartworks Service lifecycle tests
remain separate evidence. Recorded BigQuery, Snowflake and Databricks protocol
inputs verify implemented behavior only; no live cloud-engine pass or migration
cutover qualification is claimed.

## Owner-approved package coverage band

On 2026-09-07 the owner explicitly approved: "84.5% is approved by my standards".
The configured exception is exactly **84.5% for `internal/store/postgres` only**;
all other package thresholds and the default coverage policy remain unchanged.
The gate compares integer statement counts against exact basis points, so a
lower value that merely displays as 84.50% cannot pass this threshold.

The supporting measurement was **1974/2336 statements (84.50% displayed)** from
the partial cumulative race suite at
`4ace97e1471643f2de0fad7df111249f57041975`. Phase 14 native acceptance was
excluded, and the run stopped on an outdated assertion that expected MySQL to
remain unsupported. This is measured coverage evidence, not a full-suite pass.
The exception changes only the package minimum; complete native CI and the full
race-enabled coverage execution remain required on final committed source.

The later full race/coverage run at
`38823a4eddef456eacca4b035f452d79c483bb0a` completed its acceptance portion in
404.836s. SQL Server's six acceptance branches and all remaining phase 13/14 and
earlier acceptance branches passed, but the run failed the superseded MySQL
probe and `TestPhase14/AC04/mysql`: cancellation returned `ErrCancelled` while
the acknowledged remote state remained `running`. The latter reachable behavior
was still under investigation. Diagnostic coverage was 75.00% (1890/2520) for
`internal/sources`, 84.45% (2009/2379) for `internal/store/postgres` and 80.51%
(2681/3330) for `internal/engineering`; all other configured package bands
passed. The sources and store measurements remain below their configured gates,
so this is not a coverage pass.

## Pending final evidence

Hosted CI run `34167795786` targets
`38823a4eddef456eacca4b035f452d79c483bb0a` and was still running when this ledger
was written. It predates the self-contained MySQL fixture commit and therefore
cannot qualify the final head. Earlier hosted run `34161905964` passed the
reference image, vet, lint and native build jobs, but its superseded test
failures prevent it from qualifying current source.

The final-head full race/coverage run, MySQL cancellation repair, bounded lint
rerun and Linux-amd64 native SQL Server job are unfinished. Do not change either
phase registry entry to shipped until current committed source supplies every
required named result, coverage band and repository gate. Review closure and
local artifact provenance do not make a pending gate pass.
