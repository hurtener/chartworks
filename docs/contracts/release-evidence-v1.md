# Phase 25 release evidence v1

This contract supplies machine-checkable inputs to `TestPhase25`. It does not
turn a successful local fixture into production qualification. The only release
entry point is `make release-check` on a clean, committed candidate. It runs all
phases in dependency order with uncached race-enabled Go JSON results and passes
the validated earlier results to Phase 25. Running a Phase 25 test alone, setting
an environment variable by hand, or editing a planning status is not a release.

Keep the bundle **outside the repository** and set
`CHARTWORKS_RELEASE_EVIDENCE_DIR` to its absolute directory. It contains
`release.json` (at most 1 MiB), hashed raw Go JSON logs, a Phase 34 cohort
inventory and source manifest, the candidate binary, an OpenAPI response from
that candidate, and the cumulative review record. No tokens, SQL, prompts,
rows, customer schema or credential material belongs in these files. Retain any
sensitive underlying evidence in the owner-controlled system named by `run_ref`.

`release.json` uses these exact top-level keys:

| Key | Required value |
| --- | --- |
| `schema_version` | `1` |
| `head` | full 40-character source commit; every record repeats it |
| `cohort_inventory` | relative `path` and SHA-256 of the Phase 34 inventory |
| `records` | named real-boundary Go JSON test logs, described below |
| `artifacts` | binary, image, source tree, migration, OpenAPI and active-file identities |
| `cumulative_review` | relative `path` and SHA-256 of a reviewed finding ledger |

A record has `kind`, `id`, `mode`, `head`, `run_ref`, `exit_code` and `events`
(relative path plus SHA-256). The exit code must be explicitly present and zero.
The log must contain one pass for the exact named Go test, a package pass, and
no fail or skip events, including children. The record and its hash do not by
themselves prove that a real account/source was used; the owner reviews the
external run referenced by `run_ref` and its environment before accepting it.

| Kind | Required IDs | Required Go test | Accepted mode |
| --- | --- | --- | --- |
| `authority` | `pengui` | `TestReleaseAuthority` | `live` |
| `operation` | `container_boot`, `dependency_readiness`, `shutdown_recovery`, `backup_restore`, `jwks_rotation`, `retention_erasure` | `TestReleaseOperation/<id>` | `live` |
| `engine` | `postgres`, `mysql`, `sqlserver` | `TestReleaseEngine/<id>` | `native` or `live` |
| `engine` | `bigquery`, `snowflake`, `databricks` | `TestReleaseEngine/<id>` | `live` |
| `cohort` | every ID from Phase 34 inventory | `TestReleaseCohort/<id>` | `live` |

`fixture`, `recorded`, `unknown`, missing, duplicated, stale-head and skipped
records fail. The Phase 34 inventory must have the same head, a nonempty
`phase34_run_ref`, a hashed external `source_manifest`, and a nonempty unique
`cohorts` list with each cohort's required engines. The owner must verify that
this inventory is the complete output of the accepted Phase 34 dry run; Phase
25 cannot infer missing cohorts from the manifest alone. The actual Phase 34
criterion results must also pass the strict runner before Phase 25 begins.

`artifacts` contains `binary` (hashed file in the bundle), `image_ref`,
`image_digest` (`sha256:` plus 64 hex characters), `source_tree` (Git tree
hash), `migrations_sha256`, `api_schema` (hashed live `/openapi.json` response),
and `files` (the exact SHA-256 of every active required source file). The
verifier requires a clean checkout at `head`, compares the Docker image ID,
executes the binary's `version` and `schema-manifest` commands, compares the
embedded ordered-migration digest with this source build, and checks the
specified docs, operation schemas, examples and migrations. `make build` now
embeds the full commit, so an older short-commit binary cannot pass.

`cumulative_review` is a hashed JSON object with `head`, nonempty `reviewer`
and `run_ref`, and `findings` entries with unique `id`, `priority` (`P0`–`P3`)
and `state: "resolved"`. A human reviewer must verify the underlying review
and supported-feature declaration; a JSON field is a pointer to that review,
not a substitute for it. The current support matrix distinguishes native
PostgreSQL/MySQL/SQL Server checks from recorded cloud fixtures, and a cloud
engine needs separate live evidence before cutover.

AC03 remains separate under the performance evidence contract. Neither this
bundle nor its release verifier can close that criterion. This contract is
implemented before Phase 34 is shipped; absent Phase 34 evidence is a hard
release failure, and the current Phase 25 branch is not a release candidate.
