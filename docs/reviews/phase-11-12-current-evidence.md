# Phase 11/12 current acceptance evidence

Status: pending exact-head verification. This ledger is the mutable current record for the phase 11/12 acceptance candidate; the earlier adversarial and request-recovery reports remain historical evidence and are not rewritten.

Before either phase moves from `in_progress` to `shipped`, record one immutable candidate commit and its results for:

| Gate | Required current-head evidence | Current record |
|---|---|---|
| Phase 11 acceptance | `TestPhase11/AC01`–`AC06`, no skips | pending exact head |
| Phase 12 acceptance | `TestPhase12/AC01`–`AC06`, no skips | pending exact head |
| Race/package suite | uncached race-enabled repository suite | pending exact head |
| Coverage | unchanged configured package bands | pending exact head |
| Parser boundaries | bounded CSV/XLSX/Parquet focused and fuzz checks | pending exact head |
| PostgreSQL boundary | real PostgreSQL 17 metadata/workspace/source fixtures | pending exact head |
| Optional model boundary | recorded Bifrost response validation; live evidence only if claimed | pending exact head |
| Documentation | planning checker, manifest parity, mirrored rules | planning checker passed locally before the final candidate was fixed; rerun required |
| Cloud CI | exact commit, workflow run and conclusion | pending exact head |

Platform-specific fixture failure is not a code pass or failure. Record the environment and distinguish harness failures from assertions. Do not promote the registry based on this ledger until every required exact-head result is attached and reviewed.
