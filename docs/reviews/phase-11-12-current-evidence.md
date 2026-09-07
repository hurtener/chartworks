# Phase 11/12 current acceptance evidence

Status: local acceptance complete at runtime commit `49ac0cd`; final documentation-head cloud CI rerun pending. This ledger is the current record for shipped phases 11/12; the earlier adversarial and request-recovery reports remain historical evidence and are not rewritten.

The accepted local environment was Linux arm64 with Go 1.26.4, PostgreSQL 17 and pgvector 0.8.2. Runtime commit `49ac0cd` passed build, vet, the full uncached race suite, every phase 11/12 criterion and all configured package coverage gates. Measured coverage was 81.53% for `internal/engineering` and 85.14% for `internal/store/postgres`.

| Gate | Required current-head evidence | Current record |
|---|---|---|
| Phase 11 acceptance | `TestPhase11/AC01`–`AC06`, no skips | pass at `49ac0cd` |
| Phase 12 acceptance | `TestPhase12/AC01`–`AC06`, no skips | pass at `49ac0cd` |
| Race/package suite | uncached race-enabled repository suite | pass at `49ac0cd` |
| Coverage | unchanged configured package bands | pass: engineering 81.53%, postgres 85.14% |
| Parser boundaries | focused CSV/XLSX/Parquet checks and committed fuzz seed regressions | pass at `49ac0cd`; mutation campaigns remain a final-CI gate |
| PostgreSQL boundary | real PostgreSQL 17 metadata/workspace/source fixtures | pass at `49ac0cd` |
| Compiled smoke | compiled foundation lifecycle smoke | pass at `49ac0cd` |
| Optional model boundary | recorded Bifrost response validation; live evidence only if claimed | pass for recorded boundary; no live-provider claim |
| Cumulative preflight | strict runner: 12 implemented phases / 76 criteria; 22 explicit unimplemented later skips | pass at `49ac0cd` |
| Documentation | planning checker, manifest parity, mirrored rules | rerun required on final documentation head |
| Cloud CI | run `34141517671` found one lint-only `ifElseChain` style failure; corrected by `26c223b` with narrow race pass | final documentation-head rerun pending |

Two independent adversarial reviewers found no P0/P1 issues. Reviewer A's reachable P2 lifetime-cap finding was fixed in `d4edba9`: only pending/retry/running requests consume capacity, while terminal receipts remain available for replay; the narrow rereview found no regression. Reviewer B suggested trimming and NFKC-normalizing arbitrary text. That change was declined because the active contract normalizes source types while intentionally preserving original text; screening normalization remains separate.

The shipped label rests on the accepted required local gates above, including cumulative preflight. Final cloud CI remains explicit merge evidence; its run URL and conclusion belong to authoritative PR/CI metadata rather than a follow-up commit that would create another unverified head. Platform-specific fixture failure is neither a code pass nor a code failure; environment failures must remain distinguishable from assertions.
