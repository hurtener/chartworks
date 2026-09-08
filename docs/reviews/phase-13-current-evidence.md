# Phases 13/14 current acceptance evidence

Status: phases 13 and 14 are accepted and shipped at exact committed head
`6883bc2103b2b870595222e623cd4d79bc01bc41` by [qualifying hosted CI run
34182486766](https://github.com/hurtener/chartworks/actions/runs/34182486766).
PR #9 merged the phase 01–12 baseline at
`f4b159430d8c365c7348bea0bdcc04ba6193262f`; that merge is not phase 13 or 14
evidence. The phase 25 full-release gate remains unimplemented, and recorded
cloud fixtures do not claim live-cloud qualification. Review closure is recorded separately in the
[phase 13/14 adversarial review](phase-13-14-adversarial.md).

## Reproducible runner provenance

The current immutable D-067 Bruin source is
`5f562c2959496a04d57f5f199f5e3ad22159fa9f`, derived from v0.11.749. It
supersedes `02638a8f05342f6c18dd835a8d57301d0af94f77` for the acknowledged MySQL
cancellation correction. The Rust source tree is identical at both commits
(Git tree object ID `6f4405e8026d6f2fa7e32fa806ecba8ae1a4906e`), so the previously built Rust
1.98.1 `libbruin_rustsqlparser.a` remains identified by SHA-256
`82736d93282fe2f572715a25fcecce8a5c911c1fc5589b43881e295700be3243`.
The earlier Linux arm64 Go 1.26.4 runner built from `02638a8f0534` had SHA-256
`a724de2b5a2c3230e5f32bf90a85446cba221b5d2e5f8a6754d6840ee310be6d`.
That executable digest is historical evidence for its named source. The current
Linux arm64 Go 1.26.4 runner built with the reference flags from `5f562c295949`
has SHA-256
`4fb5c4f12057c3240b7a641f408a329b15d0b2c938b786ba978938196cee5405`;
its network-disabled, read-only version check reported the exact full source
revision. None of these local artifacts constitutes live cloud qualification.

## Accepted local runtime results

| Scope | Exact source | Observed result | Remaining boundary |
|---|---|---|---|
| Phase 13 acceptance | `7a69610bb5003c9aaf11fd5645d591c5481cc273` | `TestPhase13/AC01`–`AC06` passed under race detection in 99.84s, including all six strategy branches. | This predates later fixture-only and phase 14 fixes; final-head cumulative acceptance remains pending. |
| Pipeline authority and lifecycle | `7a69610bb5003c9aaf11fd5645d591c5481cc273` | The exact non-wildcard Draft–Publish–Run and control regression passed in 8.63s; enabled-runner startup passed in 1.86s; one-pool/two-output publication passed in 8.52s; SDK coverage passed in 4.24s. | These focused results do not replace the cumulative gate. |
| Phase 13 package races | `7a69610bb5003c9aaf11fd5645d591c5481cc273` | `internal/exec` passed in 36.895s, `internal/sources` in 39.706s, `internal/engineering` in 40.698s and `internal/store/postgres` in 1.260s. | Package passes are not a repository coverage result. |
| Cross-database proposal regression | `38823a4eddef456eacca4b035f452d79c483bb0a` | The real PostgreSQL/TLS/race fixture passed in 12.35s after the test service retained its explicitly configured model gateway. | The earlier whole acceptance command stopped on the fixture's discarded gateway; it was not an accepted whole-suite pass. |
| Compiled service lifecycle | `38823a4eddef456eacca4b035f452d79c483bb0a` | The compiled binary passed live foundation HTTP smoke: 221.24ms liveness, 51,604 KiB `VmRSS`, seven threads, command/health/protected-negative-route checks, redaction and clean SIGTERM shutdown. | This is a local service lifecycle check, not cumulative acceptance or live cloud evidence. |
| SQL Server parameter and cancellation behavior | `d54573d5aebc116fec5ab7216a7c518d4a6f8bb4` and `71b9bd6a872ccf70426c7013fe1c3d345fe8eb35` | Local SQL Server emulation passed the source unit race suite in 1.181s, ten parameter cases in 6.06s and `TestPhase14/AC04` under race detection in 23.333s. Hosted Linux-amd64 native SQL Server later passed all six acceptance branches at `38823a4eddef456eacca4b035f452d79c483bb0a`. | Both runs predate the final integrated head. |
| Self-contained MySQL probe fixture | `7ead982755c96e32471d450bc9c5e4e3e8b33a47` | The focused race test passed in 1.558s with a random reader role and database. | The preceding full coverage run at `38823a4eddef456eacca4b035f452d79c483bb0a` failed the old environment-dependent probe and a separate reachable MySQL acknowledged-cancellation case. Both fixes are included in the later passing `b4942c1042ff` cumulative gate. |
| MySQL acknowledged cancellation correction | Chartworks `6ff29d9bf30fab3bc433019b6131c7ab72a686a4`; Bruin `5f562c2959496a04d57f5f199f5e3ad22159fa9f` | Frozen Go 1.26.4 source passed the real MySQL `TestPhase14/AC04/mysql` five times under race detection in 21.275s. The fork's native `TestOpenReadMySQLIntegration` received `BRUIN_MYSQL_READ_TEST_DSN` and passed under race detection in the same run. Unit regressions distinguish acknowledged rollback from an unobserved `ErrTxDone` or grace-forced drain. | The later `b4942c1042ff` cumulative gate passed; final exact-head hosted qualification is recorded below. |
| Integrated runtime and package gates | `62f0362d458e46e697b638ff916c47759020f55b` | Every runtime test passed under race detection; the acceptance package completed in 360.765s. Full Linux lint reported zero issues. Coverage passed every configured band except `internal/sources`. | This run did not qualify because its source coverage band failed; the later `b4942c1042ff` row records the corrected cumulative result. |
| Source failure contracts and row-width guard | Tests `347a649f8f3ecd0acdf710df379f52da06fc56c0`; guard and integrated head `b4942c1042ffa971286bb4fb13fbc09f2f28e2b9` | The full `make coverage` gate passed with all production packages instrumented and all runtime tests under race detection; acceptance completed in 266.499s. `make preflight-full` also passed: all 88 named criteria across phases 01–14 passed with no missing or skipped implemented case; it reported the twenty later planned phases as development-only unimplemented skips. Full Linux golangci-lint 2.12.2 reported zero issues. The guard rejects shorter or longer cloud rows with `ErrType` before schema indexing and exposes no partial result. The compiled service passed foundation smoke, including command, health, protected negative routes, redaction and clean SIGTERM; fresh liveness was 154.21ms with 50,808 KiB `VmRSS` and seven threads. | Hosted CI run `34171583231` failed its bounded fuzz step. Local runtime, coverage, lint, preflight and lifecycle evidence does not substitute for that repository gate; this is not all-product release acceptance. |
| Pipeline graph fuzz setup | `811c7b47ed193d30be35a86b2ecc13c65693fe01` | Narrow review confirmed that only deterministic test setup changed. A frozen Linux Go 1.26.4 race run executed the existing valid seed twice and passed in 14.510s: the cold pinned-WASM setup took 13.383977132s outside the timed callback, both seed callbacks took 0.00s and the second setup took 269.583µs. | These timings support the cold-initialization diagnosis; the corrected harness also passed the hosted fuzz gate in the final exact-head run below. |
| Hosted CI coverage result | `34174537836` at `811c7b47ed193d30be35a86b2ecc13c65693fe01` | The acceptance package and all runtime tests passed in 204.777s. The other five hosted jobs, native container fixtures, vet, image and leaf SDK fixtures passed. | `internal/store/postgres` measured **2010/2379 (84.49%)**, below the exact approved 84.5% floor; later named, fuzz, preflight, benchmark and hygiene gates were skipped. This run is a coverage failure, not a hosted pass. |
| Qualifying hosted CI | [`34182486766`](https://github.com/hurtener/chartworks/actions/runs/34182486766) at `6883bc2103b2b870595222e623cd4d79bc01bc41` | All six hosted jobs passed on the exact committed source, including race/coverage, strict implemented-phase criteria, compiled binary smoke, the corrected fuzz gate, cumulative development preflight, microbenchmark baseline and hygiene. Hosted `internal/store/postgres` coverage was **2013/2379 (84.62%)**. | The exact-head local `make coverage` result is separately **2011/2379 (84.53%)**; phase 25 full release remains unimplemented and recorded cloud fixtures remain non-qualifying for live-cloud cutover. |
| Deterministic queue rollback regression | `fcf57f2`, follow-up `6883bc2` | The retained test synchronizes a real PostgreSQL advisory-lock wait, cancels the actual `ClaimJob` context, checks zero lease plus unchanged pending job/attempt/audit state, and proves a subsequent claim succeeds. The follow-up removed the artificial schema-corruption scan fixture. Exact Linux Go 1.26.4/race coverage passed this test five times in 1.627s; the advisory-lock block was instrumented on all five runs and the scan-error block remained uncovered as intended. | The exact-head local and qualifying hosted coverage runs both pass their configured bands. |

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
`4ace97e1471643f2de0fad7df111249f57041975`. It supports the approved threshold,
not a whole-suite pass.

At `62f0362d458e46e697b638ff916c47759020f55b`, the complete race-enabled runtime
suite passed, but the coverage gate failed only `internal/sources` at
**2011/2532 statements (79.42%)** against its unchanged 80% minimum.
`internal/store/postgres` passed its approved minimum at **2012/2379 (84.57%)**,
`internal/engineering` passed at **2681/3330 (80.51%)**, and every other
configured band passed. The source contract tests in `347a649f8f3` and the local
row-width guard in `b4942c1042ff` address that measured source gap.

At exact `b4942c1042ffa971286bb4fb13fbc09f2f28e2b9`, `make coverage` passed with
exit zero. Acceptance completed under race detection in 266.499s and every
production package was instrumented. The measured bands included
`internal/sources` at **2035/2534 (80.31%)**, `internal/store/postgres` at
**2012/2379 (84.57%)**, `internal/engineering` at **2681/3330 (80.51%)** and
`internal/jobs` at **325/382 (85.08%)**; all other configured package bands also
passed.

Hosted CI run `34174537836` tested `811c7b47ed193d30be35a86b2ecc13c65693fe01`.
Its runtime and acceptance tests passed, but the exact store band was
**2010/2379 (84.49%)**, so the coverage gate failed. The profile comparison
identified the two local-only statements as the advisory-lock `Exec` error
return at `internal/store/postgres/jobs.go:30.84–32.3` and the `scanJob` error
return at `internal/store/postgres/jobs.go:267.15–269.4`. The committed
follow-up retains only the synchronized advisory-lock cancellation regression.
Exact `6883bc2103b2b870595222e623cd4d79bc01bc41` then passed the full Linux Go
1.26.4/race `make coverage` run: acceptance completed in 141.724s, with
`internal/store/postgres` at **2011/2379 (84.53%)**, `internal/sources` at
**2035/2534 (80.31%)** and `internal/engineering` at **2681/3330 (80.51%)**;
all configured bands passed. The profile is
`/tmp/chartworks-phase13-gates-2cdf7ef/coverage-6883bc2.out`. This local pass
does not replace the remaining hosted completion gates.

The exact-head `make preflight-full` run also passed with exit zero. Its strict
acceptance runner reported all **88** named criteria across implemented phases
01–14 passed, including all six phase 13 and all six phase 14 criteria, with no
missing or skipped implemented case. `ACCEPTANCE` reported `passed_phases=14`
and `unimplemented_skips=20`; those twenty are the later planned phases in this
development branch and do not constitute release acceptance. The final line was
`PREFLIGHT OK`.

## Final closure evidence

Hosted CI run `34171583231` tested exact
`b4942c1042ffa971286bb4fb13fbc09f2f28e2b9`. Mirror, drift, lint and both native
build jobs passed. The build-test job passed the pinned dependency and runner
build, recorded leaf lifecycle contracts, source/lock check, application build,
reference image, vet, full race/coverage, per-function diagnostics, all
implemented criteria and compiled lifecycle smoke. It then failed the bounded
fuzz step while gathering the seed corpus for `FuzzPipelineDerivedGraph/seed#0`:
the fuzzing process "hung or terminated unexpectedly" with exit status 2. The
later preflight, benchmark and hygiene steps were consequently skipped. This run
is not a hosted CI pass.

The test-only correction at `811c7b47ed193d30be35a86b2ecc13c65693fe01`
validates the existing positive pipeline graph query during deterministic fuzz
setup, before the timed input callback. It retains both seeds, the callback and
its 65,536-byte input limit. The loaded frozen-source check above showed the cold
setup itself can exceed the worker watchdog while both callbacks remain
immediate, supporting the initialization-timeout diagnosis. The unchanged hosted
gate has not yet passed on this commit.

The later hosted run `34174537836` tested exact
`811c7b47ed193d30be35a86b2ecc13c65693fe01`. Runtime acceptance passed in
204.777s and the other hosted jobs plus native container, vet, image and leaf
SDK fixtures were green, but `internal/store/postgres` was **2010/2379
(84.49%)**, below the exact 84.5% threshold. Later named, fuzz, preflight,
benchmark and hygiene gates were skipped. The follow-up test-only commits
`fcf57f2` and `6883bc2` retain the deterministic advisory-lock cancellation
coverage correction and remove the artificial scan-error fixture. The final
coverage artifact for that committed source is still required.

The qualifying hosted run
[`34182486766`](https://github.com/hurtener/chartworks/actions/runs/34182486766)
completed every required phase 13/14 repository gate on exact head
`6883bc2103b2b870595222e623cd4d79bc01bc41`. Its hosted store coverage was
**2013/2379 (84.62%)**. The exact-head local Linux coverage result above remains
separately accepted evidence at **2011/2379 (84.53%)**. The cumulative preflight
reported `passed_phases=14` and `unimplemented_skips=20`; those skips are later
planned workstreams, including phase 25, and do not constitute release
acceptance. Phase 13 and phase 14 are shipped while the full release remains
unimplemented. Recorded BigQuery, Snowflake and Databricks fixtures remain
implementation evidence only; no live-cloud qualification is claimed.
