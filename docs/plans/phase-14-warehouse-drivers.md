# Phase 14 — `warehouse-drivers` (Wave 4)

> **Status:** draft
> **Owner:** orchestrator (staff Opus for the CGo/read-only posture + seeding design, Sonnet for driver wiring)
> **Depends on:** phase-08 `sources-core`, phase-10 `exec-read`

Authored per CLAUDE.md §16. This phase adds **five** real customer-engine drivers
behind the `internal/sources` adapter seam that phase 08 opened, each enforcing
the read-only execution obligations phase 10 defined. Per **D-032** the V1 real
driver set is `postgres` (phase 08) + `mysql`, `sqlserver`, `bigquery`,
`snowflake`, `databricks` (this phase). It is additive behind the seam — no core
surgery (RFC §6.1) — and may slip into Wave 5 without blocking anything.
**Difficulty: medium-high.**

Two engine classes, two validation strategies (D-032):

- **Self-hostable** — `mysql`, `sqlserver` (joining phase-08's `postgres`): full
  adapter conformance against **dockerized instances loaded with public
  (Kaggle-class) datasets**, locally and at wave ends — real-engine proof without
  cloud credentials.
- **Cloud** — `bigquery`, `snowflake`, `databricks`: hermetic recorded-fixture
  validation in CI + full validation under the D-010 live gate.

---

## RFC / request sections

- **RFC §6.1 (D-032)** — the adapter seam; the six-driver V1 set with the
  per-engine-class validation strategy; "All six real drivers are pure-Go — the
  CGo-free posture holds (D-005)"; the `ansi` dialect sentinel; "Adding a driver
  is a driver package + conformance run, never core surgery".
- **RFC §6.2–6.3** — the connections registry (secret-free read shape) and
  credential custody (envelope-encrypted, decrypted only at construction) each
  driver consumes (owned by phase 08; this phase supplies each credential-shape
  descriptor).
- **RFC §9.6** — the read-only execution obligations each driver satisfies:
  independent read-only enforcement at the adapter, server-side statement
  timeouts + context deadlines, cursor-level row caps (never LIMIT-wrapping),
  `QueryResult` shaping. Phase 10 owns the layer; this phase implements the
  per-engine mechanisms it drives.
- **RFC §9.5 / §6.1** — the `ValidatedSQL`-only `Query` signature (no raw-string
  execute) and dialect-aware parsing (recorded fixtures per dialect, incl. mysql +
  T-SQL).
- **RFC §17** — the docker-compose dev shape (Postgres already on 5434); this phase
  adds the mysql + sqlserver services + a seeding script.
- **Decisions:** **D-032** (five-driver set + dockerized-engine validation),
  D-004 (customer sources ≠ the `Store`), **D-005** (CGo-free — load-bearing),
  D-021 (defense-in-depth read-only + split read/write), D-023 (Bruin
  mine-ideas-only — drivers are native pure-Go, never an engine dependency).

## Depends on

- **phase-08 `sources-core`** — the adapter interface (`Kind`/`Dialect`/
  `Capabilities`/`Supports`/`TestConnection`/`DiscoverSchema`/`SampleValues`/
  `Query`), the factory + `init()` registration, the connections registry,
  credential custody, `TypeCategory` classification, the conformance suite, and the
  `null`/`mock`/`postgres` reference drivers. This phase implements five more
  drivers against that seam and **extends** its conformance suite; it closes no new
  seam of its own.
- **phase-10 `exec-read`** — the read-only execution layer that calls
  `adapter.Query(ctx, ValidatedSQL, QueryOpts)`. This phase supplies the per-engine
  read-only session, server-side timeout, and cursor-cap mechanisms `exec` drives;
  the exec ceilings (row cap 10 000 / 100 000, statement timeout 60s — RFC §14)
  flow *in* through `QueryOpts` and are not re-declared here.

## Informing briefs

Per `docs/research/INDEX.md`, `internal/sources` is informed by **brief 02** and
**brief 05** (primary), 04 + 10 secondary.

- **`docs/research/02-predecessor-data-and-execution.md`** — warehouse adapters,
  the encrypted Connections registry, the SQL validator's **single-gate read-only
  scar**, the **Databricks pooling difference**, result shaping.
- **`docs/research/05-predecessor-diff.md`** — the fork keepers: the **shipped
  `sqlserver` adapter** (its cursor-level capping + DSN-vs-discrete-fields config),
  the `null` fail-loud placeholder, the `ansi` dialect sentinel, capability gating.

## Brief findings incorporated

- **Brief 02 — the single-gate read-only scar (headline).** The predecessors'
  validator was the *entire* read-only guarantee; a caller could reach `execute()`
  around it. Every driver carries an **independent, engine-native read-only
  posture** (per-driver §Design). The validator (phase 09) is the primary gate; the
  engine posture is defense-in-depth; **both hold independently** (risk register
  row 10/14, RFC §9.6, D-021). For the self-hostable trio this is a **dockerized
  probe**, not live-gate-only evidence.
- **Brief 05 — the fork's `sqlserver` adapter (carried).** The fork shipped SQL
  Server in production; two specifics carry: **cursor-level capping** (`rows.Next()`
  stop at the ceiling — never LIMIT-wrapping, so `ORDER BY` survives) and
  **DSN-vs-discrete-fields config** — the adapter accepts discrete connection
  fields (host/port/database/user) and assembles the go-mssqldb DSN internally,
  with a full-DSN override supported.
- **Brief 02 — the Databricks pooling difference.** Databricks SQL warehouses are
  session/operation-oriented over HTTP-Thrift; a connection is a warehouse session,
  establishment is expensive, and a warehouse auto-suspends / cold-starts. The
  `databricks` driver takes a distinct pool shape: small `MaxOpenConns`, long
  `ConnMaxLifetime`, first-use health check + single retry — a per-kind default,
  not the shared one.
- **Brief 02 — encrypted registry + TypeCategory-once.** Drivers read credentials
  only from phase-08 custody; `DiscoverSchema` classifies each column to the
  dialect-agnostic `TypeCategory` **once** at discovery, never re-derived per query.
- **Brief 05 — `null` fail-loud (extended).** A driver that cannot support an
  operation fails **loud** (typed error + metric), never silently degrades — e.g.
  none of the five implements `sources.Materializer`, so the write path is *absent*,
  not a no-op (P1c).
- **Brief 05 — `ansi` sentinel + capability gating.** Unknown-dialect constructs
  degrade to **typed rejections, never silent passes** (risk register row 09/14);
  features are reported via `Capabilities()` / gated by `Supports(x)`, never a
  type-switch on the concrete adapter.

## Findings I'm departing from

- **Adopting an execution engine (Bruin) instead of native drivers — rejected per
  D-023.** Bruin's SQL/ingestion core needs CGo + a Rust FFI (or embedded Python),
  colliding with D-005; all five drivers are consumed as native pure-Go libraries.
- **Trusting each driver's default build to be CGo-free — rejected.** Convention 8
  caught that `gosnowflake`'s *default* build links a native `minicore` probe via
  `import "C"`; this plan pins the `-tags minicore_disabled` build (§Design,
  criterion 3) rather than assuming "pure Go driver" means the default build is
  static. No other brief recommendation is set aside.

## Scope

Five driver packages under `internal/sources/` + the conformance/dialect
extensions + the self-hostable dockerized harness:

- `internal/sources/mysql` — `github.com/go-sql-driver/mysql` **v1.10.0**
  (pure-Go `database/sql`, name `"mysql"`).
- `internal/sources/sqlserver` — `github.com/microsoft/go-mssqldb` **v1.10.0**
  (pure-Go `database/sql`, name `"sqlserver"`; the fork's shipped engine).
- `internal/sources/bigquery` — `cloud.google.com/go/bigquery` **v1.77.0**
  (pure-Go client, Jobs API; not `database/sql`).
- `internal/sources/snowflake` — `github.com/snowflakedb/gosnowflake` **v1.19.1**
  (pure-Go `database/sql`, name `"snowflake"`; built `-tags minicore_disabled`).
- `internal/sources/databricks` — `github.com/databricks/databricks-sql-go`
  **v1.13.0** (pure-Go `database/sql`, name `"databricks"`).
- Each driver: `init()` registration into the phase-08 factory; the full adapter
  interface; the credential-shape descriptor for phase-08 custody; per-engine
  read-only + timeout + cursor-cap mechanisms honouring `QueryOpts`; the pooling
  shape for its kind.
- **Conformance-suite extension** (phase 08's suite), parameterized per driver and
  split by engine class: **dockerized** (mysql, sqlserver — real engines) vs
  **hermetic-fixtures + live-gate** (cloud trio). Recorded-fixture dialect tests
  per engine (incl. mysql + T-SQL) feed the phase-09 validator + `TypeCategory`.
- **The self-hostable dockerized harness (D-032 deliverable):** docker-compose
  services for mysql + sqlserver, and an **idempotent, licensing-clean dataset
  seeding script** loading public (Kaggle-class) datasets. Specced here; the
  compose services, the `make` targets, and `scripts/seed/…` land in the
  implementing PR (this plan + the smoke skeleton are the two files authored now).
- A CI **`CGO_ENABLED=0` static-build proof** covering the whole binary with all
  five drivers linked (snowflake via `-tags minicore_disabled`).

## Non-goals

- **Materialization / write support.** `sources.Materializer` is a *separate*
  interface (P1c, RFC §7.6); all five are **read-only query drivers only** here.
- **The exec caps/timeouts themselves.** Ceilings/defaults live in `exec` (phase
  10, RFC §14); this phase implements only the engine-side enforcement of the
  `QueryOpts` values.
- **The validator / dialect parser.** Owned by phase 09; this phase supplies
  recorded wire-format fixtures per dialect and consumes `ValidatedSQL`.
- **Credential custody / the connections registry.** Owned by phase 08; this phase
  declares each driver's credential-shape descriptor and consumes decrypted
  credentials at construction.
- **Bundling dataset bytes in git.** The seeding script downloads from pinned URLs
  (or generates synthetically) and records licenses; no large data files are
  committed (licensing-clean + repo-hygiene).
- New engines beyond the six (a seventh kind is a future driver package +
  conformance run, RFC §6.1).

## Design

Each driver is an `internal/sources` adapter (interface + factory + `init()`
registration, CLAUDE.md §4.4). `Query` takes only `ValidatedSQL` — a raw string is
unconstructible, so no driver executes unvalidated SQL. The validator is the
**primary** read-only gate; each driver adds an **independent** engine-native
posture (defense-in-depth, D-021). Credentials arrive decrypted from phase-08
custody only at construction, held in memory, never logged/echoed (RFC §6.3,
CLAUDE.md §7). Adapters are immutable after construction and safe under concurrent
reuse (CLAUDE.md §5).

### Version pins (verified against release assets — convention 8)

| Kind | Module | Pinned | Class | Shape | CGo verdict |
|---|---|---|---|---|---|
| `mysql` | `github.com/go-sql-driver/mysql` | **v1.10.0** | self-hostable | pure-Go `database/sql` (`"mysql"`) | **CGo-free** — no `import "C"` files in the tree. |
| `sqlserver` | `github.com/microsoft/go-mssqldb` | **v1.10.0** | self-hostable | pure-Go `database/sql` (`"sqlserver"`) | **CGo-free** — no `import "C"` files in the tree. |
| `bigquery` | `cloud.google.com/go/bigquery` | **v1.77.0** | cloud | pure-Go client (Jobs API, gRPC/HTTP); not `database/sql` | **CGo-free** — gRPC/HTTP transport, no `import "C"`. |
| `snowflake` | `github.com/snowflakedb/gosnowflake` | **v1.19.1** | cloud | pure-Go `database/sql` (`"snowflake"`) | **CGo-free only with `-tags minicore_disabled`.** The *default* build compiles `minicore_posix.go` (`//go:build !windows && !minicore_disabled`) carrying `import "C"` + `#cgo LDFLAGS: -ldl` — a `dlopen`-based native probe. Excluded by the tag; officially supported (`internal/compilation/minicore_disabled.go`). |
| `databricks` | `github.com/databricks/databricks-sql-go` | **v1.13.0** | cloud | pure-Go `database/sql` (`"databricks"`); Thrift + pure-Go Arrow | **CGo-free** — no `import "C"` files; `apache/thrift` + `apache/arrow/go/v12` pure-Go by default (Arrow's `cdata` CGo path is behind an unused tag). |

Verification: module tag + `go.mod` from `proxy.golang.org`/GitHub at the pinned
SHA; source-tree grep for `import "C"` / `#cgo`; `gosnowflake`'s minicore CGo file
confirmed present and tag-gated. The **binding** proof is criterion 3's CI static
build.

### `mysql` — `go-sql-driver/mysql` v1.10.0 (self-hostable / dockerized)

- **Query path.** `database/sql` → `QueryContext(ctx, sql.String())`.
- **Read-only posture (defense-in-depth).** MySQL **supports read-only
  transactions**: the adapter runs each query in `START TRANSACTION READ ONLY`
  (rolled back after fetch), so a write smuggled past a broken validator errors at
  the engine; belt-and-suspenders a read-scoped user (SELECT-only grant). The
  dockerized probe (criterion 8) asserts a write is rejected.
- **Timeout.** Server-side `MAX_EXECUTION_TIME` (the `SET SESSION
  max_execution_time` / `/*+ MAX_EXECUTION_TIME(n) */` optimizer hint, SELECT-only)
  from `QueryOpts` **plus** context cancellation (the driver sends `KILL QUERY` on
  ctx cancel, stopping server-side work).
- **Cursor caps.** `rows.Next()` stop at the ceiling. Never LIMIT-wrap.
- **Pooling.** Standard `*sql.DB` pool; conservative open/idle/lifetime.
- **Credential-shape descriptor.** Secret = password → `secret_ciphertext`.
  Non-secret `config_json` = `host`, `port`, `database`, `user`, `tls`. DSN
  assembled internally.
- **Dialect.** MySQL. `Capabilities`: CTE yes (8.0+), LIMIT yes.

### `sqlserver` — `microsoft/go-mssqldb` v1.10.0 (self-hostable / dockerized)

- **Query path.** `database/sql` (`"sqlserver"`) over TDS → `QueryContext`.
- **Read-only posture (defense-in-depth).** T-SQL has no `BEGIN READ ONLY`;
  enforced by (1) the validator (primary), (2) a **read-scoped login** (`db_datareader`
  only) + `ApplicationIntent=ReadOnly` in the connection, (3) no write-statement
  path in the adapter. Dockerized probe (criterion 8) asserts a smuggled write is
  rejected by the login's role.
- **Timeout.** Context deadline — go-mssqldb sends a TDS **Attention** (cancel) on
  ctx cancel, stopping the query server-side — plus the connection query-timeout
  param. (SQL Server's server-side governor is a server config, not per-session; the
  Attention-cancel path is the reliable per-query stop.)
- **Cursor caps (fork keeper).** `rows.Next()` cursor-level cap at the ceiling —
  the fork's shipped behavior — never LIMIT-wrapping, so `ORDER BY` survives.
- **Config (fork keeper — DSN-vs-discrete-fields).** The adapter accepts **discrete
  fields** (`host`, `port`/`instance`, `database`, `user`, `encrypt`,
  `app_intent`) and assembles the go-mssqldb DSN internally; a full-DSN override is
  supported.
- **Credential-shape descriptor.** Secret = password → `secret_ciphertext`;
  non-secret `config_json` = the discrete fields above.
- **Dialect.** T-SQL. `Capabilities`: CTE yes, LIMIT via `TOP`/`OFFSET…FETCH`
  (reported through `Supports`, not assumed).
- **Local image note.** `mcr.microsoft.com/mssql/server` is amd64-only; on
  darwin/arm64 dev the compose service uses `mcr.microsoft.com/azure-sql-edge`
  (ARM64, T-SQL-compatible), the full image in amd64 CI. Recorded in the compose
  spec + seeding script.

### `bigquery` — `cloud.google.com/go/bigquery` v1.77.0 (cloud)

- **Query path.** Wraps `*bigquery.Client`; `Query()` constructs a **read query
  job**, `Read(ctx)` → `*RowIterator`. Only query jobs are ever issued — no
  table/dataset mutation calls — so the adapter exposes no DDL/DML surface.
- **Read-only posture.** No `BEGIN READ ONLY`; enforced by the validator (primary),
  the query-only job path, and a read-scoped service account (`jobUser` +
  `dataViewer`, **no** `dataEditor`). The engine with the weakest native session
  guarantee — documented explicitly; live probe asserts a write job is rejected by
  the SA.
- **Timeout.** Context deadline + `Query.JobTimeoutMs` (server-side cancel) +
  optional `maximumBytesBilled` cost ceiling (a resource guard the SQL engines
  lack). Ctx cancel cancels the job.
- **Cursor caps.** Iterate `*RowIterator`, stop at the ceiling; `PageInfo().MaxSize`
  for page fetch. Never LIMIT-wrap.
- **Pooling.** No `sql.DB`; the client manages its own gRPC/HTTP pool. One client
  per source, reused.
- **Credential-shape descriptor.** Secret = service-account JSON key →
  `secret_ciphertext` (`option.WithCredentialsJSON`). Non-secret `config_json` =
  `project_id`, `location`, optional `dataset`.
- **Dialect.** GoogleSQL.

### `snowflake` — `gosnowflake` v1.19.1 (cloud)

- **Query path.** `database/sql` → `QueryContext`; Arrow default result format.
- **Read-only posture.** No transaction read-only mode; enforced by the validator
  (primary) + a **read-scoped role + warehouse** in the DSN (SELECT only) + no
  write path. Live probe asserts a smuggled write is rejected by the role.
- **Timeout.** `STATEMENT_TIMEOUT_IN_SECONDS` (server-side, from `QueryOpts`) +
  ctx cancel (server-side abort).
- **Cursor caps.** `rows.Next()` stop; Arrow-batch backed. Never LIMIT-wrap.
- **Pooling.** `*sql.DB` pool; conservative (Snowflake sessions are heavy).
- **CGo.** Built `-tags minicore_disabled`; the binary also sets
  `SF_DISABLE_MINICORE=true` at start (belt-and-suspenders, runtime no-op once the
  tag excludes the CGo file). The load-bearing D-005 constraint.
- **Credential-shape descriptor.** Secret = password | key-pair PEM | OAuth token →
  `secret_ciphertext` (key-pair preferred, rotation-friendly). Non-secret
  `config_json` = `account`, `warehouse`, `database`, `schema`, `role`, `region`.
- **Dialect.** Snowflake SQL.

### `databricks` — `databricks-sql-go` v1.13.0 (cloud)

- **Query path.** `database/sql` (`"databricks"`) over HTTP-Thrift to a SQL
  warehouse; `QueryContext`; pure-Go Arrow fetch.
- **Read-only posture.** No read-only txn mode; enforced by the validator (primary)
  + a Unity Catalog service principal scoped to SELECT + no write path. Live probe
  asserts a smuggled write is rejected by the principal.
- **Timeout.** Ctx deadline (`QueryContext` cancel → Thrift `CancelOperation`) +
  connector `timeout`.
- **Cursor caps.** `rows.Next()` stop; Arrow fetch-size configured. Never LIMIT-wrap.
- **Pooling (the brief-02 difference).** Small `MaxOpenConns`, long
  `ConnMaxLifetime`, first-use health check + single retry for warehouse
  cold-start/auto-suspend; no aggressive fan-out.
- **Credential-shape descriptor.** Secret = PAT | OAuth (M2M) client secret →
  `secret_ciphertext`. Non-secret `config_json` = `host`, `http_path`, `catalog`,
  `schema`.
- **Dialect.** Databricks SQL (Spark SQL family).

### The self-hostable dockerized harness (D-032 deliverable)

**docker-compose services** (added to `docker-compose.yml` alongside the phase-08
Postgres-on-5434, in the implementing PR):

- `mysql` — `mysql:8.x` pinned, port `3307:3306`, a dedicated read-scoped user +
  the seeded database.
- `sqlserver` — `mcr.microsoft.com/azure-sql-edge` (ARM64-friendly for
  darwin/arm64 dev; the full `mssql/server` image in amd64 CI), port `1433`, a
  read-scoped login.

**`make` targets** (implementing PR): `warehouses-up` / `warehouses-down` (start/
stop mysql + sqlserver + run the seeder), sibling to `pg-up`/`pg-down`.

**Dataset seeding script** `scripts/seed/warehouses.sh` (+ `scripts/seed/datasets/`
manifest), **idempotent** and **licensing-clean**:

- *Idempotent* — guards on a marker table (`_chartworks_seed(version)`); if the
  expected version rows are present it exits `OK` without reloading. Safe to re-run;
  a version bump re-seeds.
- *Licensing-clean* — only public-domain / CC0 or permissive (MIT/BSD) datasets
  (e.g. Chinook-class sample data, TPC-H `dbgen` synthetic data, or a CC0 Kaggle
  set); each dataset carries a license file in the manifest, and the script
  **asserts the license file is present + on an allowlist** before loading (a
  missing/unknown license fails the seed loudly — no silently-loaded data).
- *No committed bytes* — data is downloaded from a pinned URL or generated; the repo
  carries only the manifest + loader SQL, not the datasets.
- Datasets are shaped to exercise every `TypeCategory` (numeric/temporal/boolean/
  text/structured/binary) so discovery + typing conformance is meaningful.

**Conformance split (which criteria run where):**

| Conformance check | mysql / sqlserver (self-hostable) | bigquery / snowflake / databricks (cloud) |
|---|---|---|
| registration + factory | hermetic | hermetic |
| `ValidatedSQL`-only signature | hermetic | hermetic |
| dialect + `TypeCategory` fixtures | hermetic | hermetic |
| credential-shape secret-free | hermetic | hermetic |
| CGO_ENABLED=0 build | hermetic (CI) | hermetic (CI) |
| TestConnection / Discover / Sample | **dockerized** (seeded engine) | **live-gated** (D-010) |
| read-only enforcement probe | **dockerized** | **live-gated** |
| server-side timeout probe | **dockerized** | **live-gated** |
| cursor row-cap clamp | **dockerized** | **live-gated** |
| Databricks cold-start / pool | — | **live-gated** |

Dockerized tests SKIP when the engine env isn't set (like the store conformance's
Docker-Postgres gating); live tests SKIP unless the per-kind `.env` is set — both
guarded so CI never runs an unguarded external test (criterion 12).

## Config keys added

Per-kind connection/auth defaults + pool sizing under `sources` (RFC §14). Exec
ceilings (row cap, statement timeout) flow in via `QueryOpts`, not re-declared.
Each key is added to code + the example config + a smoke check **in the
implementing PR** (this plan declares them; the two files authored now are the plan
and the smoke skeleton).

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `sources.mysql.query_timeout` | duration | `60s` | no | `MAX_EXECUTION_TIME`; capped by exec. |
| `sources.mysql.max_open_conns` | int | `8` | no | `*sql.DB` pool ceiling. |
| `sources.mysql.tls` | string | `preferred` | no | TLS mode. |
| `sources.sqlserver.query_timeout` | duration | `60s` | no | Applied via ctx deadline / connection timeout. |
| `sources.sqlserver.max_open_conns` | int | `8` | no | Pool ceiling. |
| `sources.sqlserver.app_intent` | string | `ReadOnly` | no | Connection `ApplicationIntent`. |
| `sources.bigquery.job_timeout` | duration | `60s` | no | `JobTimeoutMs`; capped by exec. |
| `sources.bigquery.max_bytes_billed` | int64 | `0` (unset) | no | Optional cost ceiling. |
| `sources.bigquery.location` | string | `""` | no | Default job/dataset location. |
| `sources.snowflake.statement_timeout` | duration | `60s` | no | `STATEMENT_TIMEOUT_IN_SECONDS`. |
| `sources.snowflake.max_open_conns` | int | `4` | no | Sessions are heavy. |
| `sources.snowflake.conn_max_lifetime` | duration | `30m` | no | Recycle idle sessions. |
| `sources.databricks.query_timeout` | duration | `60s` | no | Connector timeout; capped by exec. |
| `sources.databricks.max_open_conns` | int | `2` | no | Small pool — expensive warehouse sessions (brief 02). |
| `sources.databricks.conn_max_lifetime` | duration | `60m` | no | Tolerate cold-start / auto-suspend. |

## Acceptance criteria

Numbered and mechanically checkable. **Hermetic** (CI): 1–5, 12. **Dockerized**
(self-hostable trio, seeded real engines): 6–9. **Live-gated** (cloud trio, D-010):
10–11. This set covers the master plan's key criteria (conformance per driver —
dockerized for the self-hostable class, live-tagged for the cloud class;
`CGO_ENABLED=0` build proof in CI; per-engine read-only documented + tested, split
by class).

1. **Registration + factory (hermetic).** Each of the five drivers registers via
   `init()` blank-import and is constructible through the phase-08 factory by
   `kind`; an unknown/misconfigured kind fails loud (typed error), never a silent
   nil adapter.
2. **`ValidatedSQL`-only signature (hermetic).** For every driver, `Query` accepts
   only `ValidatedSQL`; a raw-string execute cannot be expressed (compile-time /
   structural — the phase-08 proof extended to all five).
3. **`CGO_ENABLED=0` static-build proof (hermetic, CI).** The full binary builds
   with `CGO_ENABLED=0` and all five drivers linked, snowflake compiled `-tags
   minicore_disabled`, and the binary is statically linked (no dynamic libc). A CI
   job asserts the build succeeds and the binary is CGo-free.
4. **Recorded-fixture dialect + typing (hermetic).** Per-engine captured schema/
   type/result wire fixtures (incl. mysql + T-SQL) parse through the phase-09
   validator and classify to the correct `TypeCategory`; an unknown-dialect
   construct becomes a typed rejection via the `ansi` sentinel, never a silent pass.
5. **Credential-shape secret-free on read (hermetic).** Each driver's read/list
   config shape carries no secret field (type-level test); credentials inject only
   at construction and never appear in logs, errors, or results.
6. **Dockerized harness exists + seeds (dockerized).** docker-compose brings up
   mysql + sqlserver; `scripts/seed/warehouses.sh` is **idempotent** (re-run is a
   no-op via the marker table) and **licensing-clean** (asserts an allowlisted
   license file per dataset; a missing/unknown license fails the seed loudly).
7. **Dockerized conformance — connect/discover/sample (dockerized).** mysql +
   sqlserver pass `TestConnection` + `DiscoverSchema` + `SampleValues` against the
   seeded public datasets, with correct `TypeCategory` classification.
8. **Per-engine read-only — documented + tested (dockerized half).** The mysql +
   sqlserver read-only posture is documented (§Design) and a write smuggled past a
   hypothetically broken validator is rejected by the engine (read-only txn / role)
   — dockerized probe.
9. **Per-engine timeout + cursor cap (dockerized half).** For mysql + sqlserver: a
   query past the configured timeout stops server-side; row output clamps to the
   `QueryOpts` ceiling regardless of caller input with `ORDER BY` preserved (no
   LIMIT-wrapping) — dockerized.
10. **Cloud read-only + timeout + cap (live-gated).** For bigquery + snowflake +
    databricks: the read-only probe, server-side timeout, and cursor-cap clamp each
    hold against a real warehouse under the live gate; the read-only posture is
    documented (§Design).
11. **Cloud connect/discover incl. Databricks cold-start (live-gated).**
    `TestConnection` + `DiscoverSchema` succeed against a real warehouse per cloud
    engine, including Databricks cold-start / pool behavior.
12. **External halves are guarded (hermetic meta-check).** Every dockerized test
    SKIPs when its engine env is unset and every live test SKIPs when its `.env` is
    unset — no unguarded external test runs in CI; live suites run `-count=1` under
    the gate.

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven per driver — DSN/`config_json` assembly (incl. the
  sqlserver discrete-fields-vs-DSN path), capability reporting, `TypeCategory`
  mapping from recorded fixtures, timeout/cap option wiring (asserted against a
  call log, no network), credential-shape round-trip. `-race`.
- **Integration:** this phase consumes phase-08's seam + phase-10's execution layer
  and **extends** phase-08's conformance suite (§17 trigger). Self-hostable engines
  use **real drivers against dockerized instances** (the sanctioned real-driver path,
  as store conformance uses Docker Postgres); cloud engines use recorded fixtures in
  CI + the **live gate** for real round-trips (CI has no cloud warehouse). All
  `-race`.
- **Adversarial:** the read-only defense-in-depth probe (criteria 8 + 10) is a
  standing SQL-safety obligation (write/DDL smuggled past a broken validator →
  engine rejects) — dockerized for the self-hostable trio, live-gated for the cloud
  trio; the hermetic half asserts the `ValidatedSQL`-only signature. No new
  auth/ACL path is introduced (access is enforced upstream), so the cross-tenant /
  forged-header set rides phases 04/08.
- **Fuzz:** n/a — no new parse/decode surface (SQL parsing is phase 09). The
  recorded-fixture dialect corpus feeds phase 09's `FuzzValidate`.
- **Bench:** optional `BenchmarkTypeCategoryClassify` on the discovery path if hot;
  not a gate.

## Coverage targets

Default for a conformance-tested subsystem is **85%** (convention 4). The
**self-hostable** drivers reach full behavioral coverage via the dockerized engines
(like `postgres`), so 85% applies straight. The **cloud** drivers' real-warehouse
execution paths are only reachable under the D-010 live gate — hermetically
unreachable — so per CLAUDE.md §11 that class gets a **documented override + a
new decision entry** (the coverage-override proposal below, next free `D-NNN`):
the 85% band applies to the hermetically
reachable half (registration, factory, capability gating, DSN/credential
construction, `TypeCategory` fixture mapping, config, option wiring); the live-only
paths are covered under the live gate and excluded via a documented class+reason —
never a silent lowering.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/sources/mysql` | 85% | Conformance-tested via dockerized engine — full behavioral coverage. |
| `internal/sources/sqlserver` | 85% | As above. |
| `internal/sources/bigquery` | 85% (hermetic half) | Live-only execution paths excluded with a documented override (the coverage-override proposal in Decisions filed). |
| `internal/sources/snowflake` | 85% (hermetic half) | As above. |
| `internal/sources/databricks` | 85% (hermetic half) | As above. |

`scripts/coverage-bands.conf` entries (with the override class for the cloud trio)
land in the implementing PR that creates each package.

## Smoke checks

`scripts/smoke/phase-14.sh` maps each criterion to an assertion. Hermetic criteria
run the corresponding `go test` (via `run_group`, SKIPping cleanly until the tests
exist); dockerized criteria SKIP unless the per-engine docker env is set; live
criteria SKIP unless the per-kind `.env` is set (they belong to the wave-end live
gate, not the smoke loop).

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestRegistersAndConstructs` (all five) present + PASS (else SKIP). |
| 2 | `TestQueryRequiresValidatedSQL` present + PASS. |
| 3 | `CGO_ENABLED=0 go build -tags minicore_disabled ./cmd/chartworks` succeeds + binary reports no cgo (else SKIP if module set not present yet). |
| 4 | `TestDialectFixtures` / `TestTypeCategoryFixtures` (incl. mysql, tsql) present + PASS. |
| 5 | `TestConfigShapeHasNoSecret` present + PASS. |
| 6 | Seeder present + idempotent + license-allowlist assertion; SKIP unless `CHARTWORKS_MYSQL_DSN`/`CHARTWORKS_SQLSERVER_DSN` set (docker up). |
| 7 | `TestConnectDiscoverSample_{Mysql,Sqlserver}` — SKIP unless docker env set. |
| 8 | `TestReadOnlyProbe_{Mysql,Sqlserver}` — dockerized SKIP-gate. |
| 9 | `TestTimeoutAndRowCap_{Mysql,Sqlserver}` — dockerized SKIP-gate. |
| 10 | `TestCloudReadOnlyTimeoutCap_{Bigquery,Snowflake,Databricks}` — SKIP unless `CHARTWORKS_LIVE_<KIND>_DSN` set. |
| 11 | `TestConnectAndDiscover_{Bigquery,Snowflake,Databricks}` — live SKIP-gate. |
| 12 | Meta: `go test -run <External test names>` with no docker/live env reports SKIP (never PASS/FAIL) — guards hold. |

## Glossary additions

Pre-written for `docs/glossary.md` (landed in the implementing PR, CLAUDE.md §14).
Only genuinely new terms — `data source`, `data-source adapter`, `dialect`,
`ValidatedSQL` already exist.

- **Warehouse driver** *(Internals & seams)* — a data-source adapter for a specific
  customer engine (`mysql`/`sqlserver`/`bigquery`/`snowflake`/`databricks`),
  pure-Go, read-only on the query path, behind the RFC §6.1 seam. Adding one is a
  driver package + a conformance run, never core surgery.
- **Self-hostable vs cloud engine class** *(Internals & seams)* — the D-032 split:
  self-hostable engines (postgres/mysql/sqlserver) validate against dockerized
  instances seeded with public datasets; cloud engines (bigquery/snowflake/
  databricks) validate via recorded fixtures + the D-010 live gate.
- **Dataset seeding script** *(Internals & seams)* — the idempotent,
  licensing-clean loader that populates the dockerized self-hostable engines with
  public (Kaggle-class) datasets for conformance; asserts an allowlisted license per
  dataset and commits no data bytes.
- **`minicore` build exclusion** *(Internals & seams)* — the `gosnowflake` default
  build links a native `dlopen`-based "minicore" probe via CGo; Chartworks builds
  the snowflake driver `-tags minicore_disabled` (and sets `SF_DISABLE_MINICORE=true`)
  to preserve the `CGO_ENABLED=0` static binary (D-005). Plumbing/internal — never
  wire/UI-facing.

## Decisions filed

This plan **edits no file other than the two it authors**. **D-032** (the
five-driver set + dockerized-engine validation) already landed and this plan
implements it. The entries below are **proposed** for the orchestrator to accept and
append in the implementing PR:

- **Ratified as D-035** during the planning review (driver pins incl. the mandatory
  gosnowflake `minicore_disabled` build tag). Original proposal: *(orchestrator to assign
  the next free `D-NNN`)*. `mysql`
  `go-sql-driver/mysql` **v1.10.0**; `sqlserver` `microsoft/go-mssqldb` **v1.10.0**;
  `bigquery` `cloud.google.com/go/bigquery` **v1.77.0**; `snowflake`
  `gosnowflake` **v1.19.1** built **`-tags minicore_disabled`** (its default build
  links a CGo `minicore` probe — `#cgo LDFLAGS: -ldl` in `minicore_posix.go`);
  `databricks` `databricks-sql-go` **v1.13.0**. All verified pure-Go /
  CGo-free-buildable against release assets (convention 8); the binding proof is the
  CI `CGO_ENABLED=0` static build (criterion 3).
- **Proposed decision — coverage-band override for the cloud-trio drivers**
  *(orchestrator to assign the next free `D-NNN`)*. The
  bigquery/snowflake/databricks real-warehouse execution paths are hermetically
  unreachable (no CI cloud credentials, D-010); the 85% conformance band applies to
  the hermetically reachable half, with live-only paths covered under the live gate
  and excluded via a documented class+reason (CLAUDE.md §11) — never a silent
  lowering. (mysql/sqlserver reach full 85% via the dockerized engines and need no
  override.)
- **Relied upon (existing):** **D-032**, D-004, **D-005** (load-bearing), D-021,
  D-023.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). Record any pin bump (e.g. a
     gosnowflake release dropping the minicore CGo default, relaxing the
     `-tags minicore_disabled` requirement), any per-engine posture the dockerized/
     live probes contradict, any seeding-dataset license swap, and confirmation this
     file + the example config + docker-compose were updated in the same PR. -->

- none yet.
