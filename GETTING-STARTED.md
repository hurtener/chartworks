# Running the current Chartworks build

This build includes shipped phases 01–12: configuration, identity enforcement, remote Bifrost inference, durable work, PostgreSQL sources, validated reads, managed uploads and profiling. It does **not** implement NLQ, reporting, the full MCP server or rendering yet. Pengui remains the sole authority issuer; Chartworks creates no credentials or local identity policy.

## Requirements

Use the pinned Go 1.26.4 toolchain, Python 3.10+ for repository checks, Docker Compose for disposable PostgreSQL, and PostgreSQL 17 client tools (`pg_dump`, `pg_restore`, `psql`) for backup/restore tests. The test account must have permission to create and drop disposable databases. Never point `CHARTWORKS_TEST_STORE_URL` at a customer warehouse.

```bash
make pg-up
export CHARTWORKS_TEST_STORE_URL='postgres://chartworks:chartworks@localhost:5434/chartworks?sslmode=disable'
export CHARTWORKS_STORE_URL="$CHARTWORKS_TEST_STORE_URL"
make build
./bin/chartworks version
```

The password shown above is the documented Docker-only fixture, not a production credential. Production connections require an operator-supplied DSN, transport encryption appropriate to the deployment, and a dedicated Chartworks metadata database. Customer source data never goes into this metadata schema.

## Configure and run

Use Pengui's configured issuer, public JWKS URL and intended capability audience. These are not signing secrets. Missing values are configuration errors; no first-user or developer fallback exists.

```bash
export CHARTWORKS_ISSUER='https://your-pengui-host.example'
export CHARTWORKS_JWKS_URL='https://your-pengui-host.example/.well-known/jwks.json'
export CHARTWORKS_AUDIENCE='your-chartworks-audience'
./bin/chartworks config-check --config examples/chartworks.foundation.json
./bin/chartworks serve --config examples/chartworks.foundation.json
```

Replace the example host and audience with the real platform configuration. `config-check` performs no network calls or inference. `serve` connects PostgreSQL and applies/checks migrations before listening. The foundation binds **only an explicit loopback IP**; broad listeners and external TLS deployment remain with the wider transport phase. JWT verification is active on every registered operational endpoint. Do not publish this foundation through an unauthenticated reverse proxy.

```bash
curl --fail http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl --fail http://127.0.0.1:8080/capabilities
```

Liveness is independent of dependency health. Readiness reports `starting`, `ready`, `unavailable` or `stale` for PostgreSQL and trusted verification-key material. The real JWT verifier and readiness use the same bounded trusted public-key cache; a readiness result does not authorize an individual request. Failed key refresh never extends key freshness. The default configuration does not enable inference or workers, and readiness never makes a paid model call.

Use Ctrl+C or SIGTERM to drain the listener, cancel and join dependency monitors, release idle HTTP connections and close the database pool. `chartworks mcp` currently exits 3 with an explicit unavailable message; it does not start a fake MCP server. Operational `/v1/*` and `/metrics` requests now require a valid Pengui bearer plus their separately registered action and addressed reach. Anonymous requests return 401; a valid caller receives a nondisclosing 404 for unregistered/inaccessible resources. Metrics also needs `ops.metrics` and returns 404 when disabled. Use the operation manifest below; there is no local login or default administrator token.

## Configuration contract

JSON is the implemented format. `config-check --defaults` prints the exact typed defaults, with secret references rather than resolved credentials. Precedence is defaults -> file -> explicitly named `env:NAME` references -> explicit `--listen` override. `--config` selects the file before `CHARTWORKS_CONFIG`; arbitrary environment variables do not silently override fields. See [the complete key reference](docs/configuration.md).

The `examples/chartworks.gateway.json` excerpt is now implemented. Combine it with your foundation configuration and set `features.gateway=true` to construct the remote SDK clients. Provider secrets are resolved at service construction, not by `config-check`; construction makes no inference calls. No local model download or second inference service is required.

## Tests and completion

```bash
make planning-check
make test
make coverage
make vet
make lint
python3 scripts/run_phase_acceptance.py --phase 01
python3 scripts/run_phase_acceptance.py --phase 02
python3 scripts/run_phase_acceptance.py --phase 03
python3 scripts/run_phase_acceptance.py --phase 04
make preflight-full
```

Real-store tests create unique `cw_test_*` databases on the explicit test server and remove them afterward. Missing PostgreSQL/client tools, a missing acceptance child, or a skipped runtime test is a failure, not a pass. Coverage instruments production packages across the full test suite, including integration callers; thresholds remain 85% for store, 80% for other internal code and 70% for CLI.

The other 22 phases remain planned. Development preflight reports planned phases as explicit skips; `make release-check` correctly refuses an all-product release until every phase is shipped. See the [phase 11/12 evidence ledger](docs/reviews/phase-11-12-current-evidence.md).

## Backup, restore and rollback

Run the archive tool as an operator against the **dedicated metadata database**, never as a tenant-facing HTTP action. The DSN is expanded into libpq environment fields, never command-line arguments or logs. Backups are created privately with mode 0600 and atomically published without replacing existing files. Treat the backup itself as sensitive data.

```bash
python3 scripts/store_archive.py backup /secure/directory/chartworks.dump
# Provision a NEW empty database and change CHARTWORKS_STORE_URL to that database.
python3 scripts/store_archive.py restore /secure/directory/chartworks.dump --confirm-empty
```

Restore only trusted archives into an empty database under exclusive operator control: PostgreSQL archives can contain executable SQL. Restore is transactional and rejects nonempty targets; it never uses `--clean`. The `--confirm-empty` check is not a lock against another administrator concurrently modifying the target. URI connection options are explicitly supported/validated by the tool; unrecognized options fail rather than disappear.

The driver verifies ordered migration versions and checksums on startup. A changed, missing or future version is a refusal, not an automatic repair. Recovery means restoring a compatible backup into a new database and deliberately switching configuration; never edit an applied migration or run an invented down migration.

The retention service is the first internal consumer of immutable revisions, compare-and-swap pointers, content-free audit, operation keys and fencing. The bounded synchronous sweep is now reachable only through the registered Pengui-JWT-protected operational service; unattended scheduling is not implemented here. Its scope type is a storage isolation coordinate, not authentication. Expired operation keys retain tombstones so retries cannot silently create fresh work. Old revision references and audit dependencies are preserved; a policy change blocks previously accepted destructive work until a new operation is explicitly accepted.

## Verified operational access (phases 03/04)

The production `serve` command now protects retention policy, audit, synchronous retention sweep, diagnostics and metrics with Pengui JWTs and signed addressed scopes. The old health-only foundation boundary is superseded for these implemented operations, not for the later analytical/MCP features. The listener remains explicit-loopback; a trusted backend supplies credentials. See [operator registration](docs/contracts/pengui-provider-registration.md), [operation manifest](docs/contracts/chartworks-operations.json) and [authority contract](docs/contracts/pengui-authority.md). The public Go client is `sdk/chartworks`; its caller supplies a current Pengui token provider. Chartworks issues no credentials.

## Enable remote models

Keep the existing foundation JSON and merge its top-level `gateway` member with `examples/chartworks.gateway.json`; set `features.gateway` to `true`. Get the OpenRouter and Cohere keys from your approved provider accounts and set the environment variables named in the excerpt. The **Cohere key is separate**: the pinned SDK does not support reranking through OpenRouter. Never paste keys into the JSON or commit them.

Review every role's model, timeout and output limits. Choose an operator-owned `embedding.model_revision` and change it whenever the remote embedding generation changes. Dimension equality does not make two spaces interchangeable. Rerank, narrative and visual ranking run only when explicitly enabled. `config-check` validates the choices without spending money.

After startup, an operator with `ops.model` and `cw.tenant.use:<tenant>` can call `POST /v1/gateway/probes` with `{"role":"embedding"}` (or another configured role). This is a **paid remote request with fixed synthetic input**. Use the Go SDK's `ProbeGateway`; it obtains the bearer from your existing Pengui token provider. Do not put a bearer in a URL, command history or a repository file. Probe failures contain sanitized receipts, not provider error bodies. Run one separately authorized live smoke per chosen provider operation before declaring deployment support.

## Enable durable maintenance

First merge and deploy the companion Pengui execution-authority change and configure its operator-approved binding file, described in [execution authority v1](docs/contracts/execution-authority-v1.md). Use a registered, enabled runtime and capability; obtain its existing tenant-bound broker client through Pengui's normal vault lifecycle. Do not create a local Chartworks signer or borrow an end-user JWT for the worker.

Merge `examples/chartworks.jobs.json` into the foundation configuration. The new `auth.audiences.jobs` must be an execution-only audience ending in `:execution`, distinct from both ordinary audiences. When using `auth.audiences`, remove the shorthand `auth.audience`. Replace the example issuer URL and tenant with real approved values, and set the environment variables referenced by the broker credentials. The schema contains **references only**. Enable `jobs.enabled` only after the companion endpoint is available.

Configure a retention policy for each tenant before admitting work. A submission uses `{"kind":"retention.sweep","binding_id":"maintenance"}` and one explicit `Idempotency-Key`. The bearer must allow `scheduling.write`, `ops.maintain`, tenant `write`/`erase` and the exact execution-binding `use` reach. Reading and cancelling use `scheduling.read`/`scheduling.cancel` plus addressed `run.read`/`run.write` reach. Schedule creation uses the same admission authority; read/state/manual-run operations additionally use the corresponding signed `schedule` reach. The SDK exposes these typed operations without retaining credentials.

A successful submission reports `pending`, not completed erasure. The worker records its actual terminal outcome. `blocked` means authority or accepted-definition validation failed; after correcting the cause, submit a **new logical key** rather than silently rewriting the accepted job. Cron is five-field/IANA-zone, intervals are anchored, and manual runs share overlap rules. Pause/resume preserves the durable cursor and applies the configured bounded missed-run policy when resumed; pausing does not cancel already accepted jobs.

The same metadata database is the queue; no Redis, message bus or extra scheduler is needed. Multi-replica bounds are pinned on first use. To change the shared concurrency/pending fingerprint, stop all workers, verify there is no active work, and explicitly remove the single `chartworks.queue_limits` configuration row before restarting all replicas with the same new settings. This row contains no authority or secrets.

Turning off `jobs.enabled` stops new admission and dispatch while preserving authorized reads, cancellation and schedule pause. Neither inference nor the broker is required for these retained metadata operations. Back up the metadata database before applying migration003; checksum checks preserve migrations001/002 unchanged. Use `make coverage`, phase 05/06 smoke scripts and `make preflight-full` for real fixture-based validation; later planned phases remain explicit skips in development, not shipped analytics features.

## Vector generations and qualified PostgreSQL sources

The reference metadata image is `pgvector/pgvector:0.8.2-pg17`; install the extension before migrations when using a restricted migration role. Merge the source excerpt from `examples/chartworks.sources.json` into the existing configuration, replace the synthetic tenant/relation coordinates, and provide the read DSN through the referenced environment variable. Never put a resolved credential or human JWT in metadata. The optional write reference is not used by the reader.

The source adapter is qualified for PostgreSQL 17 and the documented ordinary-heap subset. Use explicit source test/discovery and revision-checked rotation; registered metadata is not a health promise. Obtain Pengui-issued action/resource/context reach for the actual operations. See `docs/contracts/vector-sources-validation.md` and the executable source operation inventory. No additional warehouse engine, public raw-SQL route or local model is enabled by this excerpt.

## Validated read execution (phases 09/10)

Merge the non-secret [execution excerpt](examples/chartworks.execution.json) with
your source/verifier configuration; warehouse aliases remain opt-in. Install/check
forward migration 006 after unchanged 001–005. The metadata pool must have at least
`exec.execution_concurrency + 2` connections. Default HTTP write/client timeouts are
75 seconds; custom proxy/client budgets must leave validation and cleanup room.

Using a current Pengui bearer with source/context and all dataset query scopes,
POST `/v1/sources/sales/execute` with a synthetic registered `sales:v1` context:

```json
{"context":"sales:v1","sql":"SELECT id, amount FROM analytics.sales ORDER BY id","parameters":[],"execution":{"operation":"read-example-001","attempt":1,"preview":false,"rows":0,"bytes":0}}
```

HTTP 200 returns an accepted attempt receipt; check its status before using result
values. `empty` retains schema; `truncated` marks incomplete rows/bytes. Exact
integers/decimals are strings, booleans and null retain JSON types, and JSON columns
contain exact JSON text strings. The same invocation never automatically retries.

After a lost response, GET `/v1/read-operations/read-example-001` to recover its
attempt ID, then GET `/v1/read-executions/{id}`. POST `{}` to that path's `/cancel`
or `/reconcile` suffix. A cancellation request is not termination proof; unknown
remote state remains uncertain. No result values are retained or rerun by these
metadata calls. After a proven interrupted/failed attempt, an explicitly supplied
next attempt number may retry the same immutable operation; changed input conflicts.

Read the complete [read contract](docs/contracts/read-execution.md) before enabling
execution. Actual scan bytes remain unknown; row/response-byte caps are not scan
budgets. The 24-hour content-free receipt window is not phase-28 result retention.

## Managed uploads and profiling (phases 11/12)

Configure one tenant-bound `sources.connections[]` alias for the managed workspace with independent `env:` read/write DSN references. It must be a PostgreSQL 17 database distinct from the metadata-store database. Set `uploads.enabled=true` only after that alias and the shared durable worker are ready; set `profiling.enabled=true` only with the validated-read source/context declarations required for its sample. Optional `profiling.summaries=true` also requires the configured Bifrost `profile_summary` role.

The implemented support boundary is:

| Input/source | Current behavior | Boundary |
|---|---|---|
| CSV upload | bounded parse, staged load, activation and erasure | declared columns/types; 100 MiB and one million row maxima |
| XLSX upload | bounded workbook parse and explicit sheet selection | no formulas/macros/executable content; qualified Excelize subset |
| Parquet upload | bounded page/row-group parse with exact supported values | qualified parquet-go encoding/type subset |
| Activated upload | ordinary PostgreSQL managed source | same source binding, validator and executor; no parallel query path |
| PostgreSQL 17 source | deterministic profile plus optional sanitized summary | returned row/byte and planner-cost bounds do not prove scan bytes |
| Other warehouse engines | phase 14 source matrix | Managed uploads remain PostgreSQL-only; native MySQL/SQL Server use real fixtures, while cloud rows remain recorded-only pending separately approved live qualification |

The sequence is reserve `POST /v1/uploads`, send the exact binary body to `PUT /v1/uploads/{id}/content` with `application/octet-stream`, then start or explicitly resume `POST /v1/uploads/{id}/load`. Build a profile with `POST /v1/profiles`; inspect retained state/evidence/history and dependency health without a source/model call. Every work request uses an explicit operation key, and a lost response is recovered through `/v1/engineering-operations/{id}` rather than silently creating another attempt. Use the [manifest](docs/contracts/chartworks-engineering-operations.json) and [provider registration contract](docs/contracts/pengui-provider-registration.md) for the exact actions.

## Output specifications without a renderer

The protected chart catalog and four transformation endpoints are available after
normal server startup, even with inference and warehouse features disabled. Add
only the required `charts.read`, `charts.select` or `charts.bind` action and signed
tenant read reach through Pengui's existing capability policy. The
[v1 contract and SDK example](docs/contracts/chart-specifications-v1.md) show how to
adapt an authorized `ReadResult`, choose an output and rebuild its exact mapping
without querying again. The [configuration excerpt](examples/chartworks.charts.json)
keeps optional ranking disabled; enabling it also requires the existing Bifrost
`visual_rank` role and explicit author opt-in on the request.

This returns provider-neutral drawing input, not PNG/SVG/PDF, a stored chart,
published block or report. Those product surfaces remain in later owning phases.

## MCP tools on the existing server

Merge [examples/chartworks.mcp.json](examples/chartworks.mcp.json) into your existing
configuration, retaining the trusted Pengui issuer/JWKS, intended HTTP/MCP audiences,
metadata connection and installed domain services. Use exact `mcp.allowed_hosts`
for your deployment; allow browser origins deliberately through
`server.cors_allowlist`. The default 75-second server write timeout exceeds the
65-second MCP call limit; existing deployments with shorter timeouts must adjust
it or lower `mcp.timeout`. No provider is contacted merely by enabling MCP.

```bash
./bin/chartworks config-check --config /path/to/chartworks.json
./bin/chartworks mcp --config /path/to/chartworks.json
# Equivalently: chartworks serve --config ... with features.mcp=true.
```

The explicit `mcp` command fails when MCP is disabled. Both commands use the same
listener, health/capability services and authenticated route registry. There is no
stdio token store or separate public authentication endpoint. The host/client
supplies a current Pengui bearer carrying `mcp.use` plus the needed domain actions
and signed resource/session/context restrictions. Use the MCP intended audience,
not an HTTP-only bearer. Never put that bearer in an MCP URL or resource URI.

A client initializes using a JSON-RPC object such as the following, sent to
`POST /v1/mcp` with `Content-Type: application/json`,
`Accept: application/json, text/event-stream`, and its current Authorization bearer:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"analytics-client","version":"1"}}}
```

Send `notifications/initialized` next (202, no body), then use the negotiated
`Mcp-Protocol-Version` on subsequent requests. `tools/list` returns the permitted
installed bindings. A model-free chart catalog call is:

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"chart_catalog","arguments":{}}}
```

This additionally requires `charts.read` and `cw.tenant.read:<signed-tenant>`.
There is no GET/SSE stream or transport-session cookie. A successful tool result
contains a typed `result`; inspect `isError`, error outcome and domain receipts
before retrying anything that may have persisted or spent budget.
`sdk/chartworks.Client.MCP` obtains the bearer from its caller-supplied token
provider on every message. Its typed `ListTopics`, `ListDatasets` and
`DescribeDataset` methods instead use their ordinary HTTP endpoints/audience.
See the [MCP contract](docs/contracts/mcp-v1.md) for all eighteen tools, metadata
resource URIs and exact restrictions. The reporting Apps viewer remains phase 31.
