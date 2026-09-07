# Implemented configuration and authority

Source of truth: `internal/config.Values`, `Defaults()` and validation. JSON only; unknown, retired, duplicate, null and trailing documents are rejected. Document size is at most 1 MiB and nesting at most 32 levels. Errors identify a safe known field/container and fixed rule, never a rejected value or secret environment contents. Operational, source, validated-read, upload and profiling APIs require verified Pengui authority; NLQ, reporting and full MCP transports remain in their owning phases.

Use `chartworks config-check --defaults` for a machine-readable defaults snapshot. Required blanks deliberately do not form a runnable development-authority configuration. The example `examples/chartworks.foundation.json` supplies references for the required deployment-specific values.

## Transport and metadata storage

| Key | Type / units | Default | Bounds and behavior |
|---|---|---|---|
| `server.listen` | host:port string | `127.0.0.1:8080` | Explicit loopback IP only in this foundation; numeric port 0–65535. Port 0 is useful for isolated tests. |
| `server.read_header_timeout` | duration string | `5s` | Positive, at most 1 minute. |
| `server.read_timeout` | duration string | `15s` | Positive, at most 5 minutes. |
| `server.write_timeout` | duration string | `30s` | Positive, at most 5 minutes. |
| `server.idle_timeout` | duration string | `1m0s` | Positive, at most 10 minutes. |
| `server.shutdown_grace` | duration string | `10s` | Positive, at most 1 minute; timeout forces HTTP closure. |
| `server.max_body_bytes` | integer bytes | 10 MiB | 1 byte–100 MiB. Health endpoints accept no body; oversized known bodies receive 413. |
| `server.max_header_bytes` | integer bytes | 32 KiB | 1 KiB–1 MiB; enforced by Go HTTP server. |
| `store.dsn` | secret reference | `env:CHARTWORKS_STORE_URL` | Required nonblank environment value, never a literal credential in configuration. No implicit ambient database connection. |
| `store.max_conns` | integer | 10 | 1–100; minimum idle connections is zero. |
| `store.connect_timeout` | duration string | `5s` | Positive, at most 1 minute; initial connection/ping is bounded. |
| `store.transaction_timeout` | duration string | `5s` | At least 1 millisecond, at most 1 minute; context deadline plus PostgreSQL statement/lock limits. |
| `store.migration_policy` | enum | `apply` | `apply` atomically adds pending forward migrations; `check` rejects missing/mismatched history without migrating. |

The PostgreSQL schema is `chartworks`; queries are schema-qualified and pooled sessions use `search_path=pg_catalog`. The metadata API accepts neither arbitrary migration paths nor raw execution queries. Source validation is a separate protected operation returning a non-executable receipt. Warehouse connections use the dedicated source settings below, not these metadata settings.

## Verification configuration and dependency health

| Key | Type / units | Default | Bounds and behavior |
|---|---|---|---|
| `auth.issuer` | HTTPS URL or env reference | required | No userinfo, query or fragment; identity issuer belongs to Pengui. |
| `auth.jwks_url` | trusted HTTPS URL or env reference | required | No userinfo, query or fragment; redirects refused. HTTP fetch timeout and 1 MiB/32-key response limits. |
| `auth.audience` | string or env reference | required unless using pair | Explicit same-audience shorthand for both surfaces, at most 512 bytes. Cannot be combined with nonempty `auth.audiences`. |
| `auth.audiences.http` / `.mcp` | strings or env references | required together when shorthand absent | Exact per-surface intended audiences, at most 512 bytes each. Distinct values deny cross-surface replay. |
| `auth.algorithms` | string array | `RS256`, `ES256` | Nonempty unique allowlist from RS/ES 256/384/512. HS/none unsupported. |
| `auth.jwks_max_stale` | duration | `5m0s` | Positive, at most 1 hour. Failed refresh cannot reset last-success expiry. |
| `auth.refresh_interval` | duration | `1m0s` | Positive and strictly below maximum stale age. |
| `auth.request_timeout` | duration | `3s` | Positive, at most 1 minute. |
| `auth.clock_skew` | duration | `30s` | 0–1 minute; applies to registered token-time validation and envelope deadline, never to JWKS freshness. |
| `auth.max_token_lifetime` | duration | `15m0s` | 1 second–24 hours; integer `exp - iat` must be positive and within the limit. |
| `auth.max_token_bytes` | integer bytes | 32768 | 1024–65536; entire compact token bound before decoding. |
| `auth.max_claim_bytes` | integer bytes | 24576 | 512–token bound; decoded claims. JOSE header is separately capped at 2048 bytes. |
| `auth.max_scopes` | integer count | 32 | 1–32, matching the actual Pengui provider-minter maximum. |
| `auth.max_scope_bytes` | integer bytes | 4096 | 1–4096 summed scope-string bytes; each scope also has a fixed 256-byte ceiling. |

The real verifier and readiness share one bounded public-key cache. It rejects duplicate IDs/fields, symmetric/private material, invalid coordinates/moduli and key/algorithm mismatch; fetch failure does not extend last-success freshness. Rotation is single-flight and interval-bounded, including unknown-kid traffic. JSON nulls, duplicate claims, alternate authority representations and token-selected URLs are denied. Only the actual `tenant/user/session/scopes` issuer representation is accepted. See [provider registration](contracts/pengui-provider-registration.md).

## Telemetry and explicit capability enablement

| Key | Type | Default | Behavior |
|---|---|---|---|
| `telemetry.log_format` | enum | `json` | `json` or `text`, using slog only. |
| `telemetry.metrics` | boolean | true | Enables the internal counter/gauge exporter. False disables updates/export while retaining bounded lifecycle logs. `/metrics` is protected by explicit Pengui-issued `ops.metrics` and tenant read reach; false returns 404 even to a permitted operator. |
| `telemetry.otel` | boolean | false | True is explicitly rejected: the optional export adapter is not implemented, not silently ignored. |
| `features.gateway` | boolean | false | Enables the real Bifrost SDK adapter; false leaves it unconstructed. |
| `features.mcp` | boolean | false | True rejected until the MCP surface phase is implemented. |
| `features.reporting` | boolean | false | True rejected until reporting phases are implemented. |
| `features.renderer` | boolean | false | True rejected until rendering is implemented. |

`/capabilities` reports verified authentication, signed-scope enforcement and operational APIs as implemented, with analytical business APIs still unavailable. It never returns tokens, source IDs or DSNs. Health is public; the [registered operational routes](contracts/chartworks-operations.json) require Pengui-issued authority. No login/bootstrap/token/grants/principals routes exist. The optional metrics switch cannot disable authentication.

## Bifrost configuration and inactive excerpts

`gateway.driver` must be `bifrost`; `gateway.max_attempts_per_call` defaults to 2 and accepts 1–4. `gateway.bifrost.providers[]` contains a unique remote `name`, an `api_key` in `env:NAME` form, and an optional HTTPS `base_url` without userinfo/query/fragment. No local/ollama/mock production provider is accepted. Provider credentials are resolved only when the adapter is enabled. Native provider types currently supported are openai/openrouter for chat/embedding and cohere for rerank; a route alias uses the optional `type` field.

`gateway.roles` is a closed map: `embedding`, `enhance`, `sqlgen`, `sqlfix`, `clarify`, `pipeline_draft`, `profile_summary`, `rerank`, `narrative`, `visual_rank`. Each configured role supplies its remote provider/model and a positive timeout of at most 5 minutes. `max_tokens` is 0–65536; phase 05 applies the role-specific execution budget. Optional roles require `enabled=true` to call inference. Only rerank accepts `on_failure` (`fail` or `preserve_candidates`). Structured roles require a positive `max_tokens` cap.

The embedding role also requires `dimensions` 1–16384, `max_batch_items` 1–1024 and `max_batch_bytes` 1–4 MiB. Rerank requires `max_candidates` 1–1024. These are configuration-shape bounds, not live provider capability proofs. The separate [gateway contract](contracts/model-gateway.md) owns execution/response validation in phase 05. The existing example remains the remote embedding/rerank starting point; no local model support is added.

## Sources and secret handling

File selection: explicit `--config` takes precedence over `CHARTWORKS_CONFIG`. Value precedence: typed defaults -> supplied file fields -> explicit named environment references -> explicit `--listen`. Environment names after `env:` use uppercase letters, digits after the first character, and underscores, with a 128-character ceiling. Missing or empty referenced values fail. There is no second flat environment alias system.

Configuration snapshots and their returned `Values()` copies are safe to share concurrently. Printing the configuration redacts resolved credentials; serialization includes references only. The raw DSN accessor is limited to connection construction and must never be logged. All driver failures are mapped to safe categories rather than returning SQL/connection error text.

## Implemented worker and gateway bounds

`gateway.limits` defaults to concurrency8, tenant_concurrency4, max_input_bytes262144,
max_output_bytes1048576, cache_entries1024, cache_bytes16777216, cache_ttl10m.
Concurrency must be 1–64; per-tenant concurrency cannot exceed it. Input/output caps
are 1–4 MiB, cache entries0–8192, cache bytes0–128 MiB and TTL1s–1h. Zero cache
entries/bytes disable storage. Output bounds reject oversized SDK observations;
they do not claim a streaming transport-level memory sandbox around the SDK.

`jobs` defaults: enabled=false, workers4, global_concurrency16, tenant_concurrency2,
max_pending10000, max_pending_per_tenant1000, max_attempts3, batch100, lease15s,
heartbeat5s, poll500ms, attempt_timeout10s, backoff1s. Worker count1–32, global
concurrency1–128 (at least workers), tenant concurrency1–global, pending1–100000,
per-tenant pending1–global pending, attempts1–8, batch1–1000. Lease1s–1m,
heartbeat10ms–less-than-half-lease, poll10ms–5s, timeout100ms–1m, backoff10ms–30s;
retry backoff is capped at1m. The queued operation lifetime snapshots the current
retention policy's `operation_hours`, not a hardcoded 24h. Pending limits count
only pending, retry and running requests. Terminal receipts remain available for
idempotent replay without consuming live capacity; explicitly resuming a cancelled
request must reacquire capacity under the same admission lock.

`jobs.broker_url` is the trusted HTTPS Pengui `/exchange/execution-authority`
endpoint. `jobs.credentials[]` contains up to128 unique tenant partitions plus
`env:` client ID/secret references. No credential is stored in job rows or receipts.
`auth.audiences.jobs` is distinct, execution-only and optional while workers are off.
Execution JWT validation uses zero clock skew and at most60s lifetime; the matching
Pengui producer issues30s and refuses to issue beyond binding expiry.

Schedule definitions require an explicit timezone, `missed` skip/catch_up and
`overlap` skip/queue. Catch-up max1–32; interval60s–31days with a fixed anchor;
cron is a five-field expression. A tick advances at most32 definitions and32
occurrences per definition; older skipped windows are recorded as a range, not
an invented count. Skipping old work uses a5s late-tick grace. Equal local times at
DST fall-back have distinct UTC occurrence keys; nonexistent spring-forward
local times do not execute. Manual overlap skip is a conflict for a new request;
a replay returns its original accepted receipt.

## Source and validation configuration

The typed `sources` block defaults to disabled. Bounds: max_conns 1–16 (default 4), max_rows 1–1000 (256), max_bytes 1 KiB–4 MiB (1 MiB), connect_timeout and query_timeout 1 ms–4 s (1 s and 2 s). Connection aliases are tenant-bound and carry version, declared relations/columns and independent env: read/write references; the reader never resolves write credentials.

Cloud source contexts bind a non-secret digest of the exact resolved credential material. Changing that material requires `Source.Rotate`, including when the warehouse coordinates are unchanged. Governed BigQuery accepts explicit inline credential material in the resolved JSON or a credential file captured once up to 1 MiB; Application Default Credentials and opaque credential providers are unsupported because their effective principal cannot be pinned across restart. Pre-release cloud bindings whose fingerprints predate credential binding also require rotation.

The typed `exec` block bounds SQL bytes 128–65536 (32768), parameters 1–64 (64), AST depth 4–64 (64), AST nodes 32–16384 (8192) and concurrent validations 1–8 (2). Unknown/retired keys fail. These limits are not skip-validation settings. See `../examples/chartworks.sources.json` and `contracts/vector-sources-validation.md` for enforced fixed vector bounds, credential custody, PostgreSQL qualification and operational behavior.

## Upload and profiling configuration

Both blocks default to `enabled=false`. Disabling new work removes upload/profile mutation routes while retained upload status, profile evidence/history/health and engineering operation read/cancel routes remain registered and authorized. The upload workspace uses a configured `sources.connections[]` alias with separate `read_dsn` and `write_dsn` references; the metadata-store database name is rejected as the workspace target.

| Key | Default | Accepted bounds / behavior |
|---|---:|---|
| `uploads.formats` | `csv,xlsx,parquet` | Unique nonempty subset of those three qualified formats. |
| `uploads.max_bytes` / `max_rows` / `max_columns` / `max_cells` | 100 MiB / 1,000,000 / 256 / 4,000,000 | Positive ceilings; these defaults are also their maximums. |
| `uploads.max_cell_bytes` / `max_expanded_bytes` | 64 KiB / 256 MiB | Expanded bytes must be at least `max_bytes`; limits apply before activation. |
| `uploads.max_archive_entries` / `max_sheets` / `max_expansion_ratio` | 1,024 / 32 / 100 | Positive, with the displayed defaults as maximums. |
| `uploads.max_page_bytes` / `max_row_group_bytes` | 8 MiB / 64 MiB | Page 1 KiB–8 MiB; row group at least page size and at most 64 MiB. |
| `uploads.max_per_tenant` / `max_tenant_bytes` | 32 / 1 GiB | Count 1–1,000; bytes at least `max_bytes` and at most 10 TiB. |
| `uploads.concurrency` / `timeout` / `staging_ttl` | 2 / 1m / 24h | Concurrency 1–8; timeout 1 ms–1m; staging TTL 1m–7d. |
| `profiling.sample_rows` / `sample_bytes` | 1,000 / 1 MiB | Rows 1–100,000; returned evidence 1 KiB–16 MiB. These do not prove a physical scan-byte ceiling. |
| `profiling.planner_cost_ceiling` / `timeout` | 10,000,000 / 30s | Positive cost at most 1e12; timeout 1 ms–1m. |
| `profiling.fresh_for` / `stale_after` | 24h / 7d | Fresh-for 0–365d; stale-after must be greater and at most 10 years. |
| `profiling.summaries` / `max_versions` | false / 32 | Summaries use the existing optional `profile_summary` gateway role; versions 2–1,000. |
| `profiling.policies` | empty | At most 128 tenant/source policies and 256 unique declared range columns each. Policies minimize retained values; they grant no authority. |

CSV uses the standard-library decoder; XLSX uses the pinned Excelize dependency and requires an explicit sheet; Parquet uses the pinned parquet-go dependency and the bounded supported encodings. Every activated upload is a normal PostgreSQL 17 managed source consumed through the same validated-read path. Other warehouse engines remain phase 14 work and are not implied by upload format support.

## Managed pipeline configuration

The `pipelines` block defaults to disabled. New pipeline draft/publication/run routes are registered only when enabled; immutable definition reads and retained run controls remain available. The runner is an operator-pinned executable, never selected by a request or definition.

| Key | Default | Bounds and behavior |
|---|---:|---|
| `pipelines.enabled` | false | Requires enabled sources/validator plus all runner coordinates below. |
| `pipelines.runner_path` | empty | Absolute path when enabled; startup verifies the executable digest. |
| `pipelines.runner_version` | `v0.11.749` | Exact supported Bruin runner version. |
| `pipelines.runner_sha256` | empty | Exact lowercase SHA-256 of the executable when enabled. |
| `pipelines.temp_dir` | empty | Absolute operator-owned private ephemeral directory when enabled. |
| `pipelines.timeout` | 45s | 1 second–1 minute. |
| `pipelines.concurrency` | 1 | 1–8. |
| `pipelines.max_steps` | 8 | 1–32. |
| `pipelines.max_sql_bytes` | 65536 | 256 bytes–1 MiB per step. |
| `pipelines.max_output_bytes` | 1 MiB | 1 KiB–4 MiB runner output. |

These settings bound the supervised process and definition decoder. They do not grant source reach, prove a managed destination, make external writes transactional, or turn process termination into warehouse cancellation evidence.

The reference CI build checks out the exact D-067 fork commit, uses Go 1.26.4 and locked Rust 1.98.1, builds the parser static library, then builds the runner with `CGO_ENABLED=1` and the `bruin_no_duckdb` tag. The phase-14 Chartworks service imports the forked native parser and leaf packages: its shipping build now requires CGo under D-067 and must receive the immutable parser-library directory through the build environment; generated libraries are build artifacts and are not written into the module cache or repository. Linux acceptance supplies 2 GiB of private executable tmpfs for runner workers. SQL Server qualification uses the Linux-amd64 fixture; cloud connectors retain recorded-only status until their separately stated live cutover evidence exists.

Phase-14 per-engine support and native build instructions are in [warehouse drivers](contracts/warehouse-drivers.md).

Each `sources.connections` entry may set `dialect` to `postgres`, `mysql`,
`sqlserver`, `bigquery`, `snowflake` or `databricks`; omission preserves PostgreSQL.
`allow_insecure_local` defaults to false and applies only to explicitly configured
loopback MySQL/SQL Server development fixtures. Other endpoints require verified
TLS. It does not change signed source/context authorization or grant write access.
Cloud connection material remains an operator-only `read_dsn` environment reference,
with engine-specific fields validated by the leaf adapter; public source models
never include these bytes. Managed-write aliases remain PostgreSQL-only in phase13.
