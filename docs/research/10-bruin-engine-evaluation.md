> Status: draft · 2026-07-06 · source: external_refs/bruin-cli + upstream (pinned refs)

# Brief 10 — Bruin as a candidate embedded engine (D-018)

**Backs:** D-018 (Bruin is a candidate embedded engine, adopt-or-ditch in the RFC).
**Scope:** query execution against customer warehouses (P1 SQL-safety, D-017) and the
data-engineering pipeline stage (D-013). Not a general Bruin feature tour — every section
below is evaluated strictly against those two candidate roles and against CLAUDE.md §4.4
(seams), D-005 (CGo-free), and P5/P7.

**Method note.** The staged copy at `external_refs/bruin-cli` contains only an unvalidated
internal draft (`draft_idea_not_validated.md` — a positioning memo, mined for framing but
not evidence). It does **not** contain the actual Bruin source tree, so every claim below
about code, packaging, and licensing was verified directly against the real upstream
repository (`github.com/bruin-data/bruin`) via the GitHub API — file contents, `go.mod`,
`Dockerfile`, `.goreleaser.yaml`, and doc pages — as of the pinned refs in the table below.
No Bruin source was copied; only structural facts (package names, import lists, build
flags, license text) are reported.

| Artifact | Pinned ref | Verified via |
|---|---|---|
| `bruin-data/bruin` (main repo) | commit `2dd151b95c575b6404fe60b3c6247c204729b61b`; latest tagged release **`v0.11.666`** (published 2026-07-06) | GitHub API contents/releases |
| `bruin-data/bruin/semantic-engine` (nested module) | same commit, `semantic-engine/go.mod` | GitHub API contents |
| `bruin-data/ingestr` (separate ingestion CLI, Python) | HEAD as of 2026-07-06 | GitHub API repo metadata |
| `github.com/kluctl/go-embed-python` | `v0.0.0-3.13.1-20241219-1` (pinned in bruin's `go.mod`) | GitHub API repo metadata |
| `pkg/sqlparser/rustffi` → `polyglot-sql` crate | `=0.2.0` (pinned in `Cargo.toml`) | GitHub API contents |
| `tobymao/sqlglot` (Python, used by the legacy parser path) | `sqlglot[c]==30.7.0` (pinned in `internal/requirements.txt`) | GitHub API contents + repo metadata |

---

## Summary

Bruin is a real, actively developed (daily releases), Apache-2.0-licensed Go project with
genuinely broad connector coverage and a mature pipeline/quality/lineage model — the
positioning memo in `external_refs/bruin-cli` undersold how much is actually built. But
"written in Go" does not mean "CGo-free, embeddable, single-binary." The parts of Bruin
that do the two jobs D-018 asks about — safe SQL parsing/rewriting and heterogeneous data
movement — are **not pure Go**: the official binary requires `CGO_ENABLED=1` plus a Rust
toolchain to statically link a Rust SQL-parser FFI, and both ingestion (delegated to the
separate `ingestr` Python/dlt tool) and Python/R asset execution route through an
embedded or `uv`-managed Python runtime. That is a direct collision with D-005. One piece
is a genuine standout: `semantic-engine`, a decoupled, pure-Go, low-dependency submodule
implementing a metrics/dimensions/joins semantic layer strikingly close to Chartworks'
topic-pack concept — deliberately built for exactly this kind of external reuse.

**Provisional recommendation: mine-ideas-only for the pipeline/ingestion/SQL-safety
engine (confidence: high) — do not adopt as a library or a CLI subprocess for D-017/D-013's
core paths. Carve out `semantic-engine` for a separate, narrow adopt-as-library spike
(confidence: medium — promising on paper, unproven against Chartworks' actual topic-pack
requirements).** The RFC should treat these as two different decisions, not one.

---

## 1. Library consumability

Bruin **is** structured as a library, not just a CLI: nearly all functional code lives
under `pkg/` (public, importable) with a thin `internal/` (`bootstrap`, `data`,
`generate` — mostly embedded asset scaffolding). Module path `github.com/bruin-data/bruin`,
`go 1.25.0` (compatible with Chartworks' pinned Go 1.26 toolchain). `pkg/` holds roughly
150 sub-packages: per-connector packages (`postgres`, `bigquery`, `snowflake`,
`databricks`, `clickhouse`, `duckdb`, …), plus core mechanics (`pipeline`, `executor`,
`query`, `connection`, `scheduler`, `jinja`, `sqlparser`, `lineage`).

Two consumability shapes exist side by side:

- **The mega-factory (`pkg/connection`).** `connection.go` imports essentially every
  connector package in one file to build a universal "give me a client for connection
  type X" factory. Importing this package pulls in the full transitive dependency graph —
  Azure/GCP SDKs, Spark, Kafka clients, Mongo drivers, R support, etc. — regardless of
  which connectors Chartworks would actually use. This is the "consume Bruin as an
  ingestion/connection framework" path, and it is heavy.
- **Leaf packages.** Each connector (`pkg/postgres`, `pkg/bigquery`, `pkg/snowflake`,
  `pkg/databricks`, …) can plausibly be imported directly, bypassing the mega-factory —
  at the cost of reimplementing the connection-resolution/config layer Chartworks would
  otherwise get from `pkg/connection`. Not evaluated end-to-end (out of scope for this
  brief) but flagged as the only consumability path that doesn't drag in ~150 packages'
  worth of SDKs.
- **`semantic-engine` — the standout.** This is a genuinely separate Go module
  (`github.com/bruin-data/bruin/semantic-engine`, own `go.mod`/`go.sum`) with a
  self-declared purpose in its `go.mod` header comment: *"Standalone module so the
  semantic engine can be consumed by lightweight tools (e.g. dac) without pulling in the
  full bruin CLI dependency tree."* Its only dependencies are `jsonschema/v6`, `afero`,
  `yaml.v3`, and `x/text` (indirect) — no CGo, no connector bloat, no Python. This is
  strong evidence the Bruin maintainers already anticipated "reuse just the semantic
  layer" as a use case.

**Shelling out to the CLI instead:** viable as a fallback for the pipeline/ingestion
surface (subprocess boundary, structured JSON I/O), but it reintroduces exactly the
external-process management, binary distribution, and failure-mode complexity a
"library" evaluation is supposed to avoid — and does nothing to fix the CGo/Python
runtime requirements of §6, which live in what the subprocess itself needs to run.
Acceptable only as a stopgap, not a target architecture; CLAUDE.md's interface+factory+
driver seam pattern (§4.4) expects a Go interface at the boundary, and a CLI subprocess
gives up type safety, error typing (P4), and in-process context propagation for that
interface.

## 2. License

**Apache License 2.0**, verified directly from `bruin-data/bruin`'s `LICENSE.md` and the
GitHub API's license detection (`license.spdx_id: "apache-2.0"`). Permissive, commercial
embedding is permitted (attribution + `NOTICE` preservation only) — no license blocker to
adopting Bruin's Go code itself, including `semantic-engine`.

Two adjacent, non-Go dependencies need their own license check before any adoption that
touches them:

- **`kluctl/go-embed-python`** (embeds a Python 3.13 distribution in the Go binary,
  pinned `v0.0.0-3.13.1-20241219-1`): Apache-2.0 — no blocker.
- **`tobymao/sqlglot`** (the Python SQL-parsing library the legacy embedded-Python parser
  shells out to, pinned `sqlglot[c]==30.7.0`): MIT — no blocker.
- **`bruin-data/ingestr`** (the separate Python/dlt-based ingestion CLI Bruin delegates
  all data-source ingestion to): GitHub's API reports `license.spdx_id: "NOASSERTION"`
  even though the repo does carry a `LICENSE` file (plus `THIRD_PARTY_LICENSES.txt` and a
  `licenses.lock.yml`, suggesting a generated third-party-license manifest). SPDX
  non-detection is not itself disqualifying, but it means the license text was **not**
  independently confirmed in this pass — flagged as an open question (§10) before any
  path that runs `ingestr` in Chartworks' shipped product.

Net: the Go code (Bruin core + `semantic-engine`) is unambiguously embeddable. The
Python-runtime dependencies used for legacy parsing are unambiguously embeddable. The
`ingestr` ingestion dependency needs its license text read before it can be called
cleared.

## 3. Connector coverage

`pkg/` lists ~150 directories, but they serve two very different roles and conflating
them overstates "connector coverage" for D-017's purpose:

- **SQL warehouses/databases usable as query-execution destinations** (the relevant set
  for NLQ execution): `postgres`, `bigquery`, `snowflake`, `databricks`, `clickhouse`,
  `mysql`, `mssql`, `redshift`, `athena`, `trino`, `duckdb`, `oracle`, `db2`, `hana`,
  `vertica`, `cratedb`, `spanner`, `synapse`, `starrocks`, `sqlite`, `dremio`, `fabric`,
  `sail`. This is a genuinely strong, production-grade list (roughly 20 engines) —
  Chartworks' predecessors covered Databricks/Postgres/BigQuery/Snowflake; Bruin already
  covers those plus most of what a customer base would ask for next.
- **SaaS/API ingestion sources** (the majority of the ~150 — `salesforce`, `hubspot`,
  `shopify`, `stripe`, `zendesk`, `googleads`, `facebookads`, `notion`, `jira`, `slack`,
  dozens more): these are **not query engines**. They exist because Bruin's ingestion
  path shells out to the separate `ingestr` Python tool, and these `pkg/` directories are
  thin Go-side connection/config definitions consumed by the `ingestr` bridge
  (`pkg/ingestr/operator.go`), not independent Go client implementations doing the actual
  data movement. Counting these toward "Bruin's Go connector coverage" would be
  misleading — the real work happens in a separate Python project.

Credential/connection modeling: connections are declared in a project's `.bruin.yml`
under an `environments.<env>.connections.<type>` map, keyed by connection type, with
plaintext field examples in the official docs (username/password inline in YAML;
env-var/secrets injection exists as an opt-in mechanism, not the default). This is a
materially weaker default posture than D-016 (credentials encrypted at rest in
Chartworks' own `Store`) — adopting Bruin's connection model as-is would require wrapping
it entirely; it cannot be used unmodified.

## 4. Pipeline model

Bruin's DE model is genuinely mature and maps closely onto D-013's scope:

- **Assets**: SQL, Python, R, and YAML (ingestion) assets under a `pipeline.yml` +
  `assets/` folder convention; a pipeline is a DAG of assets executed in dependency order.
- **Materializations**: table/view materializations with incremental strategies (the
  `pkg/pipeline/materializer.go` + per-connector materialization mapping in
  `pkg/python/materialization_mapping.go` handle the SQL rendering per target dialect).
- **Quality checks**: built-in column-level checks (`unique`, `not_null`, `accepted_values`,
  `positive`/`negative`, `pattern`, `min`/`max`) plus custom checks, with a `blocking`
  flag (fail the asset + downstream vs. record-and-continue) and independent retry counts
  per check — a genuinely more granular quality model than a green/red boolean gate.
- **Lineage**: column-level lineage is derived by the SQL parser (either the Rust FFI or
  the Python/sqlglot legacy path — see §6), exposed via `bruin lineage`.
- **Scheduling**: `pipeline.yml` carries `schedule` (cron-like, `@daily` etc.),
  `start_date`, `catchup`, `retries`, `concurrency`, `max_active_steps` — a real scheduler
  contract, though Bruin itself does not appear to run a persistent scheduler daemon; it
  is invoked per run (locally, on a VM, or via GitHub Actions in the documented deployment
  model) rather than owning long-running orchestration.
- **Semantic layer**: a genuine metrics layer exists — see §1 and §7 — defined in a
  repository-level `semantic/` directory (YAML models with `dimensions`, `metrics`,
  `joins`, `segments`, `windows`), queryable via `bruin query --semantic-model`.
- **Variants**: one `pipeline.yml` can template out multiple concrete pipelines (per
  client/region/environment) via a `variables` + `variants` block — a genuinely useful
  multi-tenant-pipeline-definition idea, orthogonal to (and worth studying independently
  of) whether the engine itself is adopted.

How much of D-013's DE stage this could absorb, if the packaging problems in §6 were not
disqualifying: potentially most of it — connectors, materializations, quality checks,
lineage, and scheduling contracts are all here. That is precisely why this evaluation
matters: the DE-stage cost savings on offer are real, which makes the CGo/Python findings
in §6 a proportionately expensive thing to discover late.

## 5. Query-execution fit

Bruin ships a real cross-warehouse **dry-run/validation abstraction** (`pkg/query`):
`DryRunResult` normalizes cost estimation, referenced tables, and output schema across
backends (BigQuery-specific fields like `TotalBytesProcessed`/`EstimatedCostUSD` are
populated where the backend supports them; others fall back to `EXPLAIN`-based rows).
There is also a lightweight, pure-Go, string-based classifier
(`query.IsLikelyResultQuery`, `query.SQLStatementType`, `query.StripLeadingSQLComments`) —
useful as a cheap first-pass filter, but not a substitute for real parsing (it does not
detect e.g. a `SELECT` wrapping a CTE with an `INSERT` side-effect, or validate table
references against an allowlist).

The primitives D-017 actually needs — **single-SELECT enforcement**
(`IsSingleSelectQuery`), **LIMIT injection** (`AddLimit`), **table extraction for schema
allowlisting** (`UsedTables`/`GetTables`), and **column lineage** — all live behind the
`sqlparser.Parser` interface, which has exactly two implementations (§6): the CGo+Rust
FFI parser, or the legacy Python-subprocess parser. There is no pure-Go implementation of
these safety-critical operations. That means: **Bruin's query-execution layer can serve
as the NLQ execution/safety engine only if Chartworks accepts one of the two non-Go
runtime dependencies in §6** — it is not "pipeline-only in practice" (it genuinely has a
usable query/dry-run path), but the specific safety gates P1/D-017 requires are gated
behind the packaging problem, not absent from the design.

## 6. CGo / binary implications (D-005)

This is the central finding of the brief, and it is unambiguous once the build tooling is
read directly (`.goreleaser.yaml`, `Dockerfile`):

- **Official release builds set `CGO_ENABLED=1`** on every platform (darwin, linux
  amd64/arm64), with per-OS C/C++ cross-compilers configured (`o64-clang` for darwin,
  `x86_64-linux-gnu-gcc`/`aarch64-linux-gnu-gcc` for linux).
- **A Rust toolchain is a build-time prerequisite.** `scripts/build_rustsqlparser_release_lib.sh`
  installs `rustup` if absent and runs `cargo build --release` against
  `pkg/sqlparser/rustffi/Cargo.toml` (which wraps the `polyglot-sql` crate, pinned
  `=0.2.0`) to produce a static library (`libbruin_rustsqlparser.a`) that the Go build
  links via cgo (`#cgo LDFLAGS: ... -lbruin_rustsqlparser`, in
  `pkg/sqlparser/rustsqlparser_ffi_{darwin,linux}.go`, each gated `//go:build cgo && …`).
  There is a build-tag fallback (`rustsqlparser_ffi_stub.go`, `//go:build !cgo || !(darwin
  || linux)`) but it **returns an error at runtime** ("rust sql parser ffi requires cgo on
  darwin or linux") rather than a working pure-Go parser — i.e., a `CGO_ENABLED=0` build
  of Bruin does not get a degraded parser, it gets a non-functional one for this path.
- **DuckDB support requires cgo too** — `pkg/duckdb` has parallel `db.go`/`db_no_duckdb.go`
  (build-tag-gated `!bruin_no_duckdb` / `bruin_no_duckdb`) implementations, confirming the
  maintainers themselves treat "no DuckDB" as a distinct, reduced-functionality build
  variant, not a transparent fallback.
- **A legacy, still-present code path embeds a full Python runtime.** `pkg/sqlparser`'s
  older `SQLParser` type (as distinct from `RustSQLParser`) uses
  `github.com/kluctl/go-embed-python` to extract an embedded Python 3.13 distribution
  plus a bundled `sqlglot`-based parser and a Jinja2-based renderer to a temp directory at
  process start, then drives it as a **long-lived subprocess** over stdin/stdout pipes.
  Current CLI entry points (`cmd/run.go`, `cmd/render.go`) invoke `RustSQLParser`, so this
  is not necessarily the default hot path in the current release — but it is shipped,
  tested, and depended on by the module graph regardless of which parser is selected at
  runtime; embedding "just the Go module" does not opt out of it.
- **Ingestion and Python/R asset execution are Python-runtime-dependent by design, not by
  accident.** The official `Dockerfile` installs a C/C++ toolchain, `python3-dev`,
  `unixodbc`, and — after building the Go binary — bootstraps `uv` (astral-sh/uv) and
  pre-installs **Python 3.9 through 3.14** into the runtime image, then runs
  `bruin init bootstrap && bruin run bootstrap` to seed an `ingestr` installation. This is
  the documented, supported deployment shape: Bruin's ingestion connectors and Python/R
  assets are not Go code at all — they are Python packages (`ingestr`, user pipeline code)
  executed inside `uv`-managed virtual environments that Bruin provisions at runtime.

**Conclusion: Bruin is not CGo-free, not a single static binary in its supported
configuration, and depends on a provisioned Python runtime for two of its three core
capabilities (ingestion, Python/R assets).** Only the SQL-warehouse query/materialization
path and the `semantic-engine` submodule are plausibly compatible with a CGo-free,
single-binary target — and even the query path loses its safety-relevant parsing
primitives (§5) without cgo+Rust or an embedded Python runtime. Adopting any of the
CGo/Python-dependent pieces would require a new decision entry reversing D-005, exactly as
CLAUDE.md §4.4/D-005 requires — this brief does not recommend making that reversal for
Bruin's DE/ingestion/parsing engine.

## 7. Architectural fit

- **Seam pattern (CLAUDE.md §4.4).** Bruin's connector story is closer to "one mega-package
  that imports everything" (`pkg/connection`) than to Chartworks' interface+factory+driver
  seam with independently registering drivers. The individual connector packages
  (`pkg/postgres`, `pkg/snowflake`, …) could be wrapped behind Chartworks' own `sources`
  seam as drivers, but that means writing Chartworks' own factory/registration layer
  around Bruin's clients — Bruin doesn't hand you the seam, just the clients.
- **One-binary posture.** Directly contradicted by §6 for the ingestion/parsing paths; not
  contradicted for the pure-Go `semantic-engine` submodule.
- **Context-first design.** Good alignment where checked: `pkg/executor`'s concurrent DAG
  runner and `pkg/connection` take `context.Context` idiomatically; this part of Bruin's
  Go code reads like code Chartworks' own conventions (§5) would accept in review.
- **Concurrency safety if embedded long-lived.** Bruin is designed and shipped as a
  **per-invocation CLI tool** (run once per pipeline execution, exit), not as a long-lived
  in-process component of a multi-tenant server. Evidence for this framing: the embedded
  Python extraction path writes to shared, content-hash-keyed temp directories on process
  start (a ~3-second one-time cost per the code's own comments, mitigated by a "cached"
  parser constructor reusing a stable path) — a pattern built for "run once, exit," not
  for "hundreds of the this per second across tenants inside a persistent server," which
  is exactly the shape D-017's read-only execution path needs. No outright thread-safety
  bug was found in the code inspected, but no evidence of production use as a persistent,
  concurrently-invoked in-process library was found either — this is an unverified
  assumption shift, not a confirmed safe pattern, and is called out as an open question
  (§10).
- **P5 (one intelligence seam) / P7 (one primitive family).** Not directly violated —
  Bruin's SQL parsing is deterministic (Rust/sqlglot), not an LLM call, so P5 doesn't
  apply to it directly. But P7's "one primitive family, no parallel paths" is worth
  flagging: adopting Bruin's execution engine while keeping the `internal/exec` seam
  CLAUDE.md's layout already reserves would create exactly the "two of anything" P7
  warns against — Bruin's executor plus Chartworks' own — unless Bruin fully replaces
  that package rather than sitting beside it.
- **Bruin already ships its own MCP server** (`cmd/mcp/mcp.go`, present in the current
  tree). Not evaluated in depth here (out of scope), but worth the RFC knowing about: it
  is not a substitute for Chartworks' own MCP tool surface (different contract, different
  auth model, no P2/P3/P1 access enforcement of Chartworks' shape) and should not be
  conflated with it if any part of Bruin is adopted.

## 8. The honest alternative — build vs. adopt

| Dimension | Adopt Bruin (full engine) | Adopt `semantic-engine` only | Build on plain drivers (pgx, `cloud.google.com/go/bigquery`, `gosnowflake`, `databricks-sql-go`, …) |
|---|---|---|---|
| CGo-free / single binary | No — cgo+Rust required for safety-relevant parsing; Python runtime required for ingestion/Python-R assets | Yes — pure Go, minimal deps | Yes — these are the same driver packages Bruin itself uses under the hood |
| Warehouse connector breadth | ~20 SQL engines, production-tested | n/a | Each engine is its own integration; ~20 engines is a multi-quarter effort if all are wanted at once, but Chartworks likely needs 3-5 for V1 (the predecessors' set: Databricks/Postgres/BigQuery/Snowflake) |
| SQL-safety primitives (single-SELECT, LIMIT injection, table/column extraction) | Present, but gated behind cgo+Rust or Python subprocess | Not its job (semantic-engine doesn't parse arbitrary SQL) | Must be built — a Go SQL parser (e.g. a pure-Go dialect-aware parser) or a narrower allowlist-based validator scoped to generated SQL shapes, which D-017 arguably wants anyway (schema-allowlisted, not general-purpose parsing) |
| Semantic/metrics layer (measures, dimensions, joins) | Comes bundled with the above cost | Directly reusable, closely matches topic-pack shape | Built from scratch — this is the predecessors' "crown jewel" (D-013) Chartworks already plans to keep/enhance, so likely rebuilt as Chartworks-native regardless |
| Ingestion (SaaS/API sources) | Delegates to `ingestr` (Python, separate license to verify) | n/a | Out of D-013's V1 scope framing beyond "uploaded CSV/XLSX/Parquet and warehouse connections" — the ~100 SaaS connectors are arguably not needed for V1 at all |
| Ops/packaging cost | High — Rust+C toolchain in CI/release pipeline, Python runtime provisioning (`uv`, multiple Python versions) in the shipped artifact, credential model needs full rewrite for D-016 | Low — one more pure-Go dependency | Medium — each new driver is a normal Go dependency addition; no new toolchain classes |
| Time-to-first-warehouse-query | Fast if the cgo/Python cost is accepted | n/a (doesn't execute queries) | Slower per-connector, but each connector is delivered CGo-free and fits the existing seam pattern from day one |

**Reading the table:** the "build it ourselves" column is not actually more expensive for
the specific V1 scope D-013 describes (uploaded files + a handful of warehouse
connections) — pgx/BigQuery/Snowflake/Databricks Go drivers are the same underlying SDKs
Bruin itself wraps, with none of the CGo/Rust/Python packaging tax. The place Bruin's cost
genuinely looks lower than building it is the **full DE pipeline surface** (materializations,
quality-check DSL, lineage, scheduling contract, variants) — but that surface is
inseparable, in Bruin's design, from the cgo+Python runtime dependencies this brief flags.

## Risk table

| Risk | Likelihood | Impact | Notes |
|---|---|---|---|
| Adopting Bruin's engine silently reverses D-005 (CGo-free) without a decision entry | Medium | High | The cgo+Rust requirement is easy to miss if evaluated only via `go.mod` (which shows no CGo-only entries) rather than the actual build scripts — this brief exists partly to prevent that miss |
| `ingestr`'s license is not independently confirmed | Low-medium | Medium | GitHub reports `NOASSERTION`; a `LICENSE` file exists but its text was not read in this pass — must be resolved before any ingestion-path adoption |
| Bruin's connection/credential model (plaintext YAML fields, opt-in secrets injection) gets adopted uncritically alongside connectors | Medium | High | Directly conflicts with D-016 (encrypted at rest); any connector reuse must strip and replace the credential layer entirely |
| Embedding Bruin's executor as a long-lived, concurrently-invoked library (rather than per-run CLI) hits untested concurrency assumptions | Medium | Medium | No confirmed bug found, but no evidence of this usage pattern either — treat as unproven, not proven-safe |
| `semantic-engine` looks good on paper but doesn't actually fit Chartworks' topic-pack requirements once compared in detail | Medium | Medium | This brief did not do a field-by-field topic-pack-vs-semantic-model comparison — that is the necessary next step before "adopt" becomes a real verdict for this piece |
| Two "run a DAG of data assets" engines end up coexisting (Bruin's executor + Chartworks' own `internal/exec`) if adoption is partial | Low-medium | Medium | Would violate P7 ("one primitive family, no parallel paths") — any partial adoption must fully replace, not sit beside, the corresponding Chartworks package |

## Open questions for the RFC

1. Should `semantic-engine` get its own follow-up spike (field-by-field comparison against
   the topic-pack / measures-dimensions-KPIs-join-graph concept from the predecessor diff
   brief) before the RFC commits to a semantic-model schema, given how close the fit looks
   on paper?
2. Is a CGo+Rust build toolchain in CI/release ever acceptable for Chartworks under any
   circumstance (per D-005's "genuine CGo need... requires its own decision entry"), or is
   the CGo-free posture treated as absolute for V1 regardless of the engine evaluated?
3. If Chartworks builds its own SQL-safety validator instead of adopting Bruin's parser,
   should it aim for general SQL parsing (a heavier lift) or a narrower validator scoped
   to the shapes generated SQL can actually take (single SELECT, from an allowlisted
   schema, no DDL/DML keywords) — the latter may need much less machinery than either of
   Bruin's two parser implementations?
4. Does `ingestr`'s actual license (once read in full) permit the kind of embedding
   Chartworks would need, and if not, does that foreclose "adopt Bruin's ingestion path"
   entirely, or just require Chartworks to write its own ingestion layer against the same
   category of SaaS/API sources later, out of V1 scope?
5. Given D-013 frames V1 ingestion as "uploaded CSV/XLSX/Parquet **and** warehouse
   connections" (not the ~100 SaaS/API sources), should the ~100 non-warehouse `pkg/`
   connector directories be considered relevant to this decision at all, or descoped from
   the comparison entirely?
6. Should the RFC record a lighter-weight decision to track Bruin as a project (its daily
   release cadence and MCP server suggest it is moving toward exactly the space Chartworks
   occupies) even after a "ditch as engine" verdict — i.e., a competitive-watch note
   distinct from the adopt/ditch call?
