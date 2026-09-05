# Phase 11 — `uploads-workspace` (Wave 3)

> **Status:** draft
> **Owner:** orchestrator (Wave 3)
> **Depends on:** phase-08-sources-core (transitively: phase-02, phase-04, phase-06)

---

## RFC / request sections

- **RFC-001 §7.4** — Uploads as first-class data sources via a managed Postgres upload
  workspace (the phase's charter, D-024).
- **RFC-001 §7.1** — Registration & discovery: an upload registers as a candidate
  **dataset** with normalized, dialect-agnostic `TypeCategory` classification computed
  **once**.
- **RFC-001 §6.1–6.2** — The `postgres` data-source adapter (which "also serves upload
  workspaces") + the connections registry the workspace's `data_sources` row lands in.
- **RFC-001 §3.2 / §7.5** — Package placement (`internal/engineering`), origin/lineage/
  freshness on the registered dataset.
- **RFC-001 §12** — Store inventory: `datasets.origin = upload`; uploaded file bytes live
  on disk under a tenant-scoped path, **not** in the store; workspace tables are
  customer-data territory, not `Store` territory (D-004).
- **RFC-001 §14** — the `workspace` config domain (DSN, upload limits, formats).
- **RFC-001 §15** — `chartworks admin erase --tenant` removes workspace database contents
  and uploaded files (the erasure hook this phase provides).
- **RFC-001 §17** — dev shape: the workspace is a *second database in the same instance*.
- **Decisions:** D-024 (uploads mechanism), D-004 (store ⁄ workspace boundary), D-005
  (CGo-free — the parser-library constraint).

## Depends on

- **phase-08 `sources-core`** — required. It ships the `postgres` adapter (the read path
  the uploaded dataset is queried through), the connections registry (where the
  workspace's `data_sources` row is created, secret-free), the `ValidatedSQL`-only query
  signature, dialect-agnostic `TypeCategory` classification (reused for inference), and
  the credential-custody patterns. This phase closes the seam phase-08 opened: an upload
  becomes a queryable dataset through *that* adapter.
- Transitively already shipped (Waves 1–2), relied on but not newly opened here:
  **phase-02** (`datasets`/`data_sources` store domains + the inescapable tenant
  predicate), **phase-04** (the one access resolver + grants — the read path intersects
  the caller's dataset grants), **phase-06** (the generic leased job queue — the async
  `upload_load` handler runs here so a large file never sits on a request's hot path,
  RFC §3.3).

## Informing briefs

Per `docs/research/INDEX.md`: **02** (primary — predecessor data & execution), **01**
(secondary — predecessor architecture).

## Brief findings incorporated

- **Brief 02 — dataset-ingest hardening (the client predecessor's fixes).**
  - *Cell sanitization.* Every parsed cell is neutralized before load: leading
    formula-injection sigils (`= + - @`, and a leading tab/CR that Excel re-interprets)
    are defused so `=cmd()` lands as inert text; control characters are stripped; values
    are length-capped; encoding is normalized to UTF-8 (BOM stripped). This is RFC §7.4's
    "cell sanitization — the client predecessor's hardening."
  - *Header detection.* First non-empty row is the header for CSV/XLSX (Parquet carries
    column names natively); empty and duplicate header names are sanitized to unique,
    safe column identifiers (never silently collided).
  - *Type classification computed once.* Column types are inferred **once** at load into
    the dialect-agnostic `TypeCategory` (numeric / temporal / boolean / text / structured
    / binary / unknown) and stamped onto the dataset schema — the "`money`-column typed
    twice" bug is a standing regression guard (brief 02 / RFC §7.1).
  - *Secret-free registry read shape.* The workspace `data_sources` row reuses phase-08's
    no-secret-field read shape; the workspace DSN is a config secret, never echoed.
- **Brief 01 — the parallel weak-mode scar the design must kill.** The predecessors grew
  a weak-auth **"Spreadsheet Mode"**: a parallel ingest/query path with its own, laxer
  identity and access story. This phase **kills it by construction** — an upload is
  parsed, loaded into the tenant's workspace, and registered as an ordinary `dataset`;
  from that point it flows through the *identical* identity → grants → read-only
  `postgres` adapter → `exec` route as a warehouse table. There is no second query path,
  no second auth story, no `origin`-switched code branch on the read side (P7).

## Findings I'm departing from

None from the cited briefs. Two deliberate departures from the **predecessors** (not from
a brief's recommendation): (a) uploaded data does **not** share a loose common store — it
is isolated per tenant in a *separate* workspace database (D-004/D-024), preserving the
store⁄warehouse boundary; (b) no DuckDB / embedded analytics engine for uploads — its Go
driver requires CGo and would break D-005 (D-024). Both are RFC-settled, restated here so
the departure is visible, not silent.

## Scope

New package tree under the RFC-named `internal/engineering` (this is the first engineering
phase; phases 12–13 extend the same package in Wave 4):

- **`internal/engineering/uploads`** — the upload core:
  - **Format parsers** behind a `FormatParser` seam (interface + factory + driver, §4.4;
    drivers register by format via `init()`): `csv` (stdlib `encoding/csv`), `xlsx`
    (`github.com/xuri/excelize/v2`, streaming `Rows()` iterator), `parquet`
    (`github.com/parquet-go/parquet-go`, row-group streaming). All three pure-Go /
    CGo-free (D-005). Each parser yields a streamed `(header, rows)` sequence so
    byte/row caps are enforced without buffering the whole file.
  - **Header detection**, **type inference** (sample-bounded → `TypeCategory` →
    concrete Postgres DDL type), and **cell sanitization** (above).
  - **`Loader`** — orchestrates: validate limits → parse (stream) → sanitize → infer
    types → ensure workspace → `CREATE TABLE` + bulk `COPY` into the tenant's workspace
    schema → register the dataset (`origin = upload`) → write custody bytes. Exposed as a
    core service and wrapped by an `upload_load` **jobs handler** (phase-06 queue) for
    large files.
  - **Custody** — original uploaded bytes written to disk under a tenant-scoped path
    `{workspace.custody_dir}/{tenant_id}/{upload_id}.{ext}`; the path is derived from the
    frozen envelope's tenant, never from caller input (no traversal). An object-storage
    backend is a future custody driver behind the same interface.
  - **Erasure hook** — `EraseTenant(ctx, tenant)` drops the tenant's workspace schema
    (CASCADE) and removes its custody directory; loud on partial failure (P4). Wired into
    `chartworks admin erase --tenant` by phase-23; provided here.
- **`internal/engineering/workspace`** — the **workspace-provisioner seam** (interface +
  factory + driver): `EnsureWorkspace(ctx, tenant) → WorkspaceLocator` provisions/ensures
  the tenant's isolation unit and ensures the secret-free `data_sources` row (kind
  `postgres`) that the read path queries through. **V1 driver `local_pg`** (dev default,
  RFC §17): one workspace database (`workspace.dsn`, a *separate database* from the store)
  with a per-tenant schema `t_<sanitized-tenant>`. A managed-DB provisioner (per-tenant
  database on RDS/CloudSQL) is a future driver — no core surgery (risk-register mitigation).

Schema: **no new store tables** — reuses the budgeted `datasets` and `data_sources`
inventory (RFC §12). Writes into the workspace database are DDL/`COPY` against
customer-data territory via a privileged provisioner connection, never the `store` seam
and never the `exec`/`Query` read path.

## Non-goals

- The `POST /uploads` multipart HTTP endpoint and its status surface — **phase-21**
  (`http-api`). This phase delivers the core `Loader` + jobs handler; the surface wires it.
- Profiling / quality assessment of the uploaded dataset — **phase-12**. This phase
  registers the dataset (`origin = upload`); profiling is a separate job.
- Topic modeling of the dataset — **phase-15**.
- Incremental / append uploads, upload versioning beyond dataset `version = 1`, and
  re-upload-to-replace semantics — post-V1.
- A managed-DB (non-`local_pg`) provisioner driver, object-storage custody driver — future
  drivers behind the shipped seams, not V1.
- Any write reachable from NLQ/BYO SQL — structurally excluded (P1c); the loader's write
  path is admin/provisioning only.

## Design

### Data flow

```
bytes + filename + envelope
   │  1. limit gate         bytes ≤ max_bytes, format ∈ workspace.formats  (else typed error)
   ▼
FormatParser.Stream()       header row + streamed data rows (row cap enforced mid-stream)
   │  2. header detect       first non-empty row → sanitized unique column ids
   │  3. sanitize            defuse formula sigils, strip control chars, cap length, UTF-8
   │  4. infer (once)        sample rows → TypeCategory per column → Postgres DDL type
   ▼
WorkspaceProvisioner.EnsureWorkspace(tenant)     → schema t_<tenant>, ensured data_sources row
   │  5. CREATE TABLE t_<tenant>.<name> (...)     privileged provisioner conn (NOT store, NOT exec)
   │  6. COPY rows                                 pgx CopyFrom, batched
   ▼
datasets.Register(origin=upload, locator, schema_json, version=1, health=ok, freshness=fresh)
   │  7. custody             write original bytes → {custody_dir}/{tenant}/{upload_id}.{ext}
   ▼
queryable dataset  ──(later, read-only)──▶  postgres adapter + grants + exec   (P1a / P7)
```

Steps 5–6 (workspace writes) are **privileged provisioning**, isolated from both the
`store` seam (D-004) and the read path: a distinct interface on a distinct connection. The
subsequent *query* of the dataset (the round-trip criterion) uses the ordinary read-only
`postgres` adapter with `ValidatedSQL` and the caller's intersected dataset grants — the
same machinery a warehouse table uses.

### Key types / interfaces

- `FormatParser interface { Format() string; Stream(ctx, io.Reader, Limits) (Header, RowSeq, error) }`
  — one per format, registered by `init()`; `Supports`-style gating, never a type switch.
- `Loader` — the orchestration core; pure of surface concerns (callable by the jobs
  handler and, later, the HTTP handler). Takes a scope parameter derived from the frozen
  envelope (P1a) on every store touch.
- `WorkspaceProvisioner interface { EnsureWorkspace(ctx, tenant) (WorkspaceLocator, error); DropTenant(ctx, tenant) error }`
  — the one interface the risk register names; `local_pg` is the V1 driver.
- `TypeInference` — maps sampled cell values → phase-08 `TypeCategory` → concrete Postgres
  type; a single table-driven `categoryToDDL` map (numeric→`numeric`/`bigint`,
  temporal→`timestamptz`/`date`, boolean→`boolean`, text→`text`, else→`text`). Mixed
  columns degrade to `text` (never a lossy guess).

### Seams & properties

- **P1a / P7** — the read path is the shared adapter + resolver; an upload has no private
  query route. Empty dataset grant ⇒ typed `access.none`, no query issued.
- **P1c** — the workspace write path (DDL + `COPY`) is a distinct provisioning interface,
  unreachable from `internal/nlq` and `internal/exec` (architecture test). NLQ stays
  structurally read-only.
- **P3** — tenant isolation: per-tenant workspace schema + the store's inescapable tenant
  predicate on `datasets`/`data_sources`; custody paths are tenant-derived.
- **P4** — every failure (limit exceeded, unparseable, unsupported format, partial erase)
  is a typed error + metric, never a partial/silent load.
- **P5** — none: parsing and type inference are deterministic Go, no gateway call. (Optional
  `profile_summary` descriptions are phase-12, gateway-gated.)
- **P6** — no plumbing nouns on the wire: the dataset presents as origin `upload`; error
  codes are domain-named (`upload.malformed`, not "parse failure of the ingest shard").
- **D-004** — the workspace database is customer-data territory reached via the adapter +
  provisioner, **never** the `store` seam (architecture test).
- **D-005** — CSV = stdlib; XLSX = excelize (pure Go); Parquet = parquet-go (pure Go). A
  `CGO_ENABLED=0` build of the package is a criterion.

### Library selection (convention 8 — pin-and-verify)

Verified against real release assets via the Go module proxy (`@latest` + `@v/list` +
`@v/<v>.mod`) on 2026-07-06; both dependency trees are pure-Go (no CGo-linked package):

| Format | Library | Pinned tag | Verified | Posture |
| --- | --- | --- | --- | --- |
| CSV | stdlib `encoding/csv` | (Go 1.26) | n/a | pure Go |
| XLSX | `github.com/xuri/excelize/v2` | `v2.11.0` (tag `refs/tags/v2.11.0`, 2026-07-06) | proxy `@latest` | pure Go, CGo-free |
| Parquet | `github.com/parquet-go/parquet-go` | `v0.30.1` (tag `refs/tags/v0.30.1`, 2026-05-18) | proxy `@latest` | pure Go, CGo-free |

Both pins were **ratified as part of D-035** during the planning review (see
*Decisions filed*), mirroring how D-003 / D-019 pin a library. The implementing PR runs
`go mod tidy` and commits the exact tags; criterion 2 bakes in the `CGO_ENABLED=0` build
proof so a future CGo-carrying transitive bump fails CI.

## Config keys added

RFC §14 `workspace` domain. Contractual defaults from §14: 100 MiB / 1M rows;
csv+xlsx+parquet on.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `workspace.dsn` | string (`env:`) | — | yes (if uploads enabled) | Upload-workspace Postgres DSN — a **separate database** from `store.dsn` (D-004); dev = second DB in the same instance (RFC §17). Secret; never logged/echoed. |
| `workspace.provisioner` | string | `local_pg` | no | Workspace-provisioner driver (seam). V1: `local_pg`. |
| `workspace.custody_dir` | string (path) | — | yes (if uploads enabled) | Root for tenant-scoped custody of original upload bytes (`{dir}/{tenant}/{upload_id}.{ext}`). Not the store (RFC §12). |
| `workspace.upload_max_bytes` | int (bytes) | `104857600` (100 MiB) | no | Hard byte ceiling; exceed ⇒ `upload.too_large`. |
| `workspace.upload_max_rows` | int | `1000000` | no | Row ceiling; exceed ⇒ `upload.too_many_rows`. |
| `workspace.formats` | []string | `[csv, xlsx, parquet]` | no | Enabled formats; anything else ⇒ `upload.unsupported_format`. |
| `workspace.infer_sample_rows` | int | `1000` | no | Rows sampled per column for type inference (bounded; no full-file scan for typing). |

All keys are validated fail-loud at boot by the `workspace` sub-validator (RFC §14): if
any format in `workspace.formats` is unknown, or uploads are enabled with `dsn`/
`custody_dir` unset, boot is refused. Keys land in the example config + a smoke check
(§4.2).

## Acceptance criteria

1. **Round-trip through the shared path.** A CSV upload is parsed, loaded into the tenant
   workspace, and registered as a `dataset` with `origin = upload`; querying it goes
   through the *same* read-only `postgres` adapter + grants path as a warehouse table
   (integration, Docker Postgres). With a matching `dataset` `query` grant the rows
   return; with **no** grant the read short-circuits to typed `access.none` with **no
   query issued** (store-call-count assertion).
2. **XLSX + Parquet load CGo-free.** XLSX and Parquet uploads load through the identical
   `Loader` (header detected, types inferred, rows loaded), asserted per format; the
   `internal/engineering/uploads` package builds and tests under `CGO_ENABLED=0`
   (build-proof assertion).
3. **Malformed ⇒ typed error.** A malformed / unparseable / bad-encoding file yields a
   typed `upload.malformed` error and loads **no** rows and registers **no** dataset
   (no partial load — P4).
4. **Limits ⇒ typed errors.** An over-`upload_max_bytes` file ⇒ `upload.too_large`; an
   over-`upload_max_rows` file ⇒ `upload.too_many_rows`; a format ∉ `workspace.formats` ⇒
   `upload.unsupported_format`. All fail-loud, mid-stream where applicable.
5. **Cell sanitization.** A golden fixture proves formula-injection sigils and control
   characters are neutralized (`=cmd()` loads as inert text) and that empty/duplicate
   header names are sanitized to unique, safe column identifiers.
6. **Type inference computed once.** A mixed/`money`-style column is classified exactly
   once into a `TypeCategory` and mapped to its Postgres DDL type via the table-driven
   `categoryToDDL` map (brief 02 regression guard); no second re-typing pass exists.
7. **Workspace unreachable via the `store` seam.** An architecture test asserts no path in
   `internal/engineering/uploads`/`workspace` reaches a workspace table through
   `internal/store`; the workspace is reached only via the `postgres` adapter (read) and
   the provisioner (privileged write) — the D-004 boundary.
8. **Loader write path unreachable from NLQ/exec.** An architecture test asserts the
   privileged load / DDL / `COPY` path is not importable from `internal/nlq` or
   `internal/exec` (P1c — the read path stays structurally read-only).
9. **Tenant erase removes tables + files.** `EraseTenant(tenant)` drops the tenant's
   workspace schema/tables **and** removes its custody files; a partial failure is loud
   (typed error + metric), never silent (integration, Docker Postgres).
10. **Provisioner seam + tenant-derived paths.** The workspace provisioner is a seam with
    the V1 `local_pg` driver; custody and workspace-schema names derive from the frozen
    envelope's tenant (a `../`-style or cross-tenant filename cannot escape the tenant
    directory) — asserted structurally + by an adversarial path probe.

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven parser tests (header detection, sanitization corpus, per-format
  streaming); `categoryToDDL` mapping table; the `workspace` config validator (unknown
  format, missing dsn/custody_dir with uploads on ⇒ refused). Golden: the sanitization
  fixture (criterion 5) and the inferred-schema shape.
- **Integration (required — Deps name phase-08 and this closes phase-08's seam, §17):**
  the full round-trip against Docker Postgres (`make pg-up`) with a real token + real
  `dataset` grant — parse → provision (a real second database + per-tenant schema) → load
  → register → query via the read-only adapter; plus the erase-cascade test (criterion 9).
  Real drivers on the seam; **no** boundary mock (the gateway `mock` is not used here — no
  gateway call).
- **Adversarial (required — the read path is an access path):** cross-tenant probe on the
  uploaded-dataset read (tenant A cannot query tenant B's upload); empty-access-set
  short-circuit (criterion 1); fetch-then-filter regression guard (the grant is
  intersected inside the query, proven by store-call-count); custody path-traversal probe
  (criterion 10). Cross-tenant probes derive from the registration tables, not a
  hand-list (convention 5).
- **Fuzz (required — upload parsing is a parse/decode surface):** `FuzzParseUpload` (per
  format: `FuzzParseCSV` / `FuzzParseXLSX` / `FuzzParseParquet`) with a seed corpus;
  invariant: never panics, never loads more than `upload_max_rows`, never emits an
  un-sanitized cell. The corpus runs as an ordinary CI test.
- **Bench:** `BenchmarkLoadCSV` — the loader is a hot reusable artifact; a throughput
  baseline (not a CI gate). Plus a `-race` concurrent-reuse test proving one `Loader` /
  provisioner instance is safe under concurrent uploads.

## Coverage targets

Default 80% for new `internal/` packages (convention 4). Added to
`scripts/coverage-bands.conf` in the same PR:

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/engineering/uploads` | 80% | New package (default). |
| `internal/engineering/workspace` | 80% | New package (default). Provisioner-driver live halves that need a real managed DB are out of V1 (`local_pg` only); no override needed for `local_pg`. |

## Smoke checks

`scripts/smoke/phase-11.sh` sources `scripts/smoke/lib.bash` and uses `run_group` over
`internal/engineering/uploads` (and the architecture tests). It SKIPs cleanly when the
package does not exist yet (unbuilt surface) and surfaces integration tests gated on the
Docker-Postgres URL as genuine SKIPs when it is unset.

| Acceptance criterion | Smoke assertion (test name) |
| --- | --- |
| 1 | `TestUploadCSVRoundTrip_Integration` |
| 2 | `TestUploadXLSXParquetLoadCGoFree` |
| 3 | `TestUploadMalformedTypedError` |
| 4 | `TestUploadLimitsTypedError` |
| 5 | `TestUploadCellSanitization` |
| 6 | `TestUploadTypeInferenceOnce` |
| 7 | `TestWorkspaceUnreachableViaStore` |
| 8 | `TestUploadLoaderUnreachableFromNLQExec` |
| 9 | `TestEraseTenantRemovesTablesAndFiles_Integration` |
| 10 | `TestWorkspaceProvisionerSeamAndTenantPaths` |

## Glossary additions

Existing entries already cover **Upload workspace**, **Dataset** (`origin = upload`), and
**Data source**. New terms this phase introduces, pre-written for `docs/glossary.md` (same
PR, §14):

- **Upload custody** — the on-disk retention of a tenant's *original* uploaded file bytes
  under a tenant-scoped path (`{custody_dir}/{tenant}/{upload_id}.{ext}`); config-located,
  never in the `store` (RFC §12). An object-storage backend is a future custody driver.
- **Workspace provisioner** — the seam that ensures a tenant's upload-workspace isolation
  unit (V1 driver `local_pg`: a per-tenant schema in the separate workspace database) and
  the secret-free `data_sources` row the read path queries through (RFC §7.4, D-024).
- **Cell sanitization** — the ingest step that defuses formula-injection sigils and
  control characters and normalizes encoding on every uploaded cell before load, so a
  spreadsheet cell can never carry an executable payload downstream (brief 02, RFC §7.4).

## Decisions filed

- **Relies on (no re-litigation):** D-024 (uploads via a managed Postgres workspace
  through the standard adapter), D-004 (workspace ⁄ store boundary), D-005 (CGo-free
  posture — the parser-library constraint), D-020 (the grant path the read uses).
- **Proposal for the orchestrator** (a new `D-0NN`, orchestrator-numbered — this plan does
  not edit the append-only log): *ratify the upload-parser library pins* —
  `github.com/xuri/excelize/v2 v2.11.0` (XLSX) and
  `github.com/parquet-go/parquet-go v0.30.1` (Parquet), both verified pure-Go / CGo-free
  against real release assets (Go module proxy, 2026-07-06); CSV on stdlib `encoding/csv`.
  Rationale mirrors D-003/D-019 (a pinned, verified external dependency baked into a plan
  gets a decision entry). No CGo need arises, so D-005 is **not** reversed.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). Empty at authoring time. -->
