# Phase 11 — uploads-workspace

Status: planned. Owner: internal/engineering, internal/sources. Hard dependencies: 06, 08.

## Authority and design

RFC-001 §7, D-045/D-052 and [COMMON.md](COMMON.md) apply. Uploads become governed datasets in a workspace separate from the metadata store. Downstream query/report use waits for their actual owning phases; do not create a parallel query mode.

## Brief findings incorporated

Briefs 02, 04, 14: bounded parsing, normalized source types, tenant separation, dataset lifecycle and source-mode parity.

## Findings I'm departing from

No alternate upload authentication, source token in a file, arbitrary file execution or DuckDB escape path. Interrupted ingestion cannot look like a complete active dataset.

## Scope and implementation tasks

1. Parse bounded CSV/XLSX/Parquet to staging; validate headers/cells/types, archive expansion and path hygiene.
2. Provision a tenant-scoped managed workspace separate from metadata-store tables, register datasets and activate them atomically after successful load.
3. Reuse source/query/semantic/report paths and recover interrupted load/cleanup without duplicating uploads.

## Non-goals

No spreadsheet application, general file-format platform, parallel IAM or source execution bypass.

## Config and persistence

Workspace DSN/managed namespace, format allowlist, upload bytes=100MiB/rows=1M plus decompression/cell/sheet limits and staging expiry. Keep staged file state, checksum/idempotency and dataset activation pointers; failed/incomplete loads retain bounded recoverable state, not queryable partial tables.

## Acceptance criteria

1. **AC01** — All three formats preserve declared types, nulls, multi-sheet selection and deterministic header/error handling.
2. **AC02** — Zip/decompression growth, paths, cells/rows/bytes and spreadsheet executable content are bounded or rejected.
3. **AC03** — Cross-tenant upload/status/workspace access is denied from signed scope before data exposure.
4. **AC04** — Interrupted/retried upload leaves recoverable staging or one active dataset, not duplicate/orphan active tables.
5. **AC05** — Uploaded datasets use the same validated query, execution context and reporting contract as warehouse datasets.
6. **AC06** — Retention/erasure removes staged bytes/workspace artifacts while protecting unrelated data; no parallel auth or DuckDB bypass is introduced.

## Tests, coverage and smoke

Implement `TestPhase11/AC01` through `TestPhase11/AC06`; malformed/archive/cell fuzz seeds, real workspace load and kill/retry/cleanup tests are required. AC05 closes with the common execution/report consumer before migration acceptance. COMMON.md supplies coverage; `scripts/smoke/phase-11.sh` requires all six results.

## Glossary, decisions and deviations

Managed upload workspace is customer data, not metadata-store data. D-052 applies. No runtime completion is claimed.
