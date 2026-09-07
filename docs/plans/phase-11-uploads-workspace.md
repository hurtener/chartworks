# Phase 11 — uploads-workspace

Status: in_progress. Owner: internal/engineering, internal/sources. Hard dependencies: 06, 08.

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

The current implementation uses the operator-approved source connection alias with separate read/write environment references and a deterministic tenant workspace namespace. It conservatively rejects reuse of the metadata database name. `uploads` is opt-in; defaults include 256 columns, four million cells, 64KiB per cell, 256MiB expanded data, 1,024 archive entries, 32 sheets, a 100-fold expansion ceiling, 8MiB pages, 64MiB row groups, two concurrent upload operations, a 60-second work timeout and 24-hour staging expiry. These are explicit decoder/work bounds, not a claim of a physical database disk quota.

All formats, including CSV, reserve `uploads.max_expanded_bytes` against tenant accounting before staging; normalization can increase the original file's byte count. Activation atomically replaces this conservative reservation with measured decoded bytes. This can admit fewer simultaneous staged uploads than a reservation based only on compressed/original bytes. Failed or interrupted work remains counted until successful activation or erasure.

Declared integer/decimal values remain exact. Parquet float32 values are promoted without changing the original binary value. Managed timestamps must be exactly representable at microsecond resolution and within the supported UTC year range; nonzero excess fractional precision is rejected rather than rounded. XLSX requires an explicit sheet choice. The current parser is a qualified bounded subset, not a promise to support arbitrary Parquet encodings or executable spreadsheet features.

Forward migrations 007–008 add the consumers' metadata; migrations 001–006 are unchanged. A successful load records the actual workspace table OID. Source activation re-proves that OID through the ordinary read-only adapter and retains its table lock during the fenced metadata publication. Metadata activation is atomic locally; it is not a distributed transaction with the workspace. Owned-object erasure repeats its catalog/OID proof after acquiring a DDL lock and never uses `CASCADE`.

An upload's final erasure transaction also removes complete and checkpoint profile values, active-profile/dependency/health references, and blocks pending derived work for that exact tenant/source. Minimal immutable manifests and tombstones remain for recovery/replay safety. This is logical live-data erasure, not secure overwriting of database pages, WAL or backups. Expired staging cleanup is explicit and bounded under current signed authority; the existing retention-only broker is not silently given upload permissions.

## Acceptance criteria

1. **AC01** — All three formats preserve declared types, nulls, multi-sheet selection and deterministic header/error handling.
2. **AC02** — Zip/decompression growth, paths, cells/rows/bytes and spreadsheet executable content are bounded or rejected.
3. **AC03** — Cross-tenant upload/status/workspace access is denied from signed scope before data exposure.
4. **AC04** — Interrupted/retried upload leaves recoverable staging or one active dataset, not duplicate/orphan active tables.
5. **AC05** — Uploaded datasets use the same validated query, execution context and reporting contract as warehouse datasets.
6. **AC06** — Retention/erasure removes staged bytes/workspace artifacts while protecting unrelated data; no parallel auth or DuckDB bypass is introduced.

## Tests, coverage and smoke

Implement `TestPhase11/AC01` through `TestPhase11/AC06`; malformed/archive/cell fuzz seeds, real workspace load and kill/retry/cleanup tests are required. AC05 closes with the common execution/report consumer before migration acceptance. COMMON.md supplies coverage; `scripts/smoke/phase-11.sh` requires all six results.

The current candidate contains the six named tests, HTTP/SDK/binary integration fixtures, a checked [engineering operation manifest](../contracts/chartworks-engineering-operations.json), and exact-value, normalization-budget, publication-lock, replacement-race and derived-erasure regressions. Historical failures remain in the [adversarial review](../reviews/phase-11-12-adversarial.md) and [request-recovery record](../reviews/phase-11-12-request-recovery.md). The [current evidence ledger](../reviews/phase-11-12-current-evidence.md) owns exact-head gate results. Missing evidence keeps this phase in progress.

## Glossary, decisions and deviations

Managed upload workspace is customer data, not metadata-store data. D-052 applies. No runtime completion is claimed. Later reporting/semantic integration and final migration acceptance remain obligations of their actual owning phases, not satisfied by declaring an upload active.
