# 02 — Predecessor data model, source connectivity, and query-execution path

> Status: draft · 2026-07-06 · sources: both predecessors

## Summary

Both predecessors keep two separate stores: their **own** metadata store (`sqlite`/
`postgres`, ~50 tables — topics, jobs, sessions, caches, feedback, schedules) and the
**customer warehouse(s)** they query, via a `WarehouseAdapter` seam. The client
predecessor ships one adapter (Databricks); the fork adds native-async Postgres and SQL
Server, a `NullWarehouseAdapter`, and a tenant-scoped, encrypted, per-connection
**Connections registry** — a real step beyond "one warehouse per deployment." SQL
execution is a two-phase `/plan` → `/run` split with a bounded, LLM-driven
**self-curation loop**: execute, and on a runtime or validation error, ask an LLM to patch
the SQL and retry (capped at 2 attempts), optionally triggering one re-plan on a zero-row
result. Read-only enforcement is a **single upstream gate** — an AST (`sqlglot`) validator
whitelisting `SELECT`/`WITH`/set-ops and blocking DDL/DML anywhere in the tree — not a
property of the adapter or connection; nothing stops write SQL reaching the warehouse if
the validator is bypassed or wrong. Row limits and timeouts are both application-level
(`asyncio.wait_for` + a `limit` param), never connection/session-level. A "dataset mode"
lets a tenant upload CSV/XLSX, profiled and queried locally via DuckDB instead of a
warehouse. A rich per-stage TTL cache system exists, but nothing caches warehouse **query
results**. Background maintenance runs on one generic, leased `jobs` queue; user-facing
schedules run on a separate interval/cron dispatcher.

## Own data model

Both predecessors persist their own state through a metadata store abstraction
(`stores/metadata.py` + per-driver mixins in `stores/sql/metadata/{sqlite,postgres}.py`),
selected by a `sqlite`-for-dev / `postgres`-for-production driver split (the same
seam-with-one-V1-driver shape Chartworks' `store` package already commits to). The schema —
defined once as inline SQL DDL, shared verbatim between the two predecessors modulo the
divergences in the table below — spans roughly these groups:

| Group | Representative tables |
|---|---|
| Identity/tenancy | `users`, `tenants`, `tenant_memberships`, `invite_codes`, `service_accounts` + `service_account_credentials`/`capabilities`/`topic_grants`, `principal_audit_log` |
| Semantic model lifecycle | `topics`, `topic_versions`, `topic_version_audit_log`, `join_edges`, `cross_topic_relationships`, `topic_pair_usage_stats`, `topic_access`, `topic_shares` (client) / `topic_grants` (fork) |
| NLQ/SQL pipeline | `nlq_queries`, `nlq_inference_cache`, `sql_generations`, `sql_generation_examples`, `sql_templates` + `template_audit_log`, `idempotency_cache` |
| Datasets (upload mode) | `datasets`, `dataset_schemas`, `dataset_schema_audit` |
| Feedback/learning | `feedback_events`, `clarification_feedback`, `semantic_samples`, `prompt_packs`, `prompt_pack_metrics` |
| Scheduling | `saved_queries`, `schedules`, `schedule_runs` |
| Presentation | `chart_recipes` |
| Background jobs | `jobs`, `job_batch_checkpoints` |
| Observability | `audit_events`, `llm_call_events`, `eval_run_events` |
| Governance | `business_rules`, `business_rule_library_subscriptions`, `business_rule_firings` |
| Sessions | `sessions`, `session_events` |

Source: `_ref/original_wayfinder/src/<pkg>/stores/sql/metadata/sqlite.py` (CREATE TABLE
statements, ~50 tables total); mirrored in `_ref/forked_wayfinder_explorer/src/<pkg>/stores/sql/metadata/sqlite.py`.

**This table count is itself a cautionary data point.** CLAUDE.md §6 already names "id in
seven columns across six tables" sprawl as the anti-pattern the Store-schema budget rule
exists to prevent — this predecessor schema, at ~50 tables with heavy field duplication
across `nlq_queries`/`sql_generations`/`sql_generation_examples`/`nlq_inference_cache` (each
carrying its own copy of query text, SQL text, and topic/session identifiers), is a live
example of exactly that sprawl. It grew organically table-by-table as each pipeline stage
needed its own persistence, never with a single owning schema review.

**Fork-only additions** (generalistic predecessor `stores/sql/metadata/sqlite.py`):
`external_identities`, `redeemed_sso_jtis` (SSO login support), `tenant_business_domain_templates`,
`topic_grants` (replaces `topic_shares` — a sharing-model rename/redesign; brief 05 owns
the full diff), and **`warehouse_connections`** — the tenant-scoped Connections registry
described below. Per-item detail on the topic-lifecycle-adjacent tables (`topic_shares` →
`topic_grants`) belongs to brief 05, not here.

**Jobs table** (`jobs` + `job_batch_checkpoints`) is a single generic queue shared by every
background maintenance concern (see "Background jobs" below): `queue_name`, `job_type`,
`status`, `priority`, `attempts`, `lease_owner`/`lease_expires_at`, `progress_percent`/
`progress_stage`, `payload_json`/`result_json`/`error_json`. One shape, many job types —
notably *not* fragmented into per-feature job tables, in contrast to the schema sprawl
noted above.

## Source connectivity & credentials

Both predecessors define a `WarehouseAdapter` protocol (`domain/protocols.py`) with
`execute`, `discover_schema`, `sample_values`, `sample_rows`, `test_connection`, `supports`,
plus a `name`/`dialect`/`capabilities` triplet used for prompt-time SQL-dialect steering and
feature gating (`capabilities` is a `frozenset` like `{"LIMIT", "CTE",
"SESSION_TEMP_VIEWS", "SAFE_SAMPLE"}`; callers gate on `adapter.supports("CTE")` rather
than branching on adapter type).

- **Client predecessor** ships exactly one real adapter — **Databricks**
  (`adapters/databricks.py`, via `databricks.sdk.WorkspaceClient`), plus `mock` (in-memory
  SQLite) and `duckdb` (local file, for "Spreadsheet Mode" — see below). One
  process-global warehouse is configured at boot (`WarehouseConfig` from env/settings);
  there is no per-tenant connection concept.
- **Generalistic fork** adds **`postgres`** (native-async, `psycopg3` + `AsyncConnectionPool`,
  `min_size=1, max_size=10`, lazy-opened) and **`sqlserver`**/**`sqlserver_dsn`** (discrete
  fields vs. a raw connection string, same adapter class) — registered in
  `adapters/registry.py` alongside a `NullWarehouseAdapter`
  (`adapters/null.py`) that raises `NoWarehouseConnectionError` on any data operation. The
  fork's comment on `null.py` explains why it exists: "After the move to per-connection
  resolution there is no process-global warehouse; every real query/job resolves its own
  adapter from a stored connection" — flow orchestrators still need *an* adapter reference
  at construction time, and the null adapter fills that slot, **failing loud** instead of
  silently querying the wrong database (directly analogous to Chartworks' P4).
- **The fork's Connections registry** (`domain/warehouse_connection.py` +
  `stores/metadata_warehouse.py`) is the significant connectivity evolution: a
  tenant-scoped `warehouse_connections` table storing `kind`, non-secret `config_json`
  (host/port/database/username — never the secret), a separately-encrypted
  `secret_ciphertext` column, and a `status` lifecycle (`unverified → connected → error`)
  with `last_tested_at`/`last_error`. Secrets are decrypted only at the point a
  `WarehouseConfig` is rebuilt for adapter construction (`to_warehouse_config`), keyed by a
  credential-shape descriptor per kind (`{"dsn": ...}` for Postgres, `{"token": ...}` for
  Databricks, `{"password": ...}` / `{"connection_string": ...}` for SQL Server) —
  `WarehouseConnectionRecord`, the read/list shape returned to callers, has **no secret
  field at all**, structurally impossible to leak through a list/get response.
  Encryption is `cryptography.fernet.MultiFernet` (`config/warehouse_secrets.py`): the
  first configured key encrypts new writes, all configured keys are tried on decrypt, so
  keys can be rotated without a bulk re-encryption migration. `WAREHOUSE_SECRET_KEYS`
  unset raises `WarehouseSecretConfigError` — fail closed, no accidental plaintext
  storage.
- **File upload (CSV/XLSX) path** exists only as "dataset mode" / "Spreadsheet Mode" and
  bypasses the warehouse-connection concept entirely: uploads land on local disk under a
  tenant/owner/dataset-scoped directory (`build_dataset_storage_dir`), get parsed with
  `pandas` (`.csv`, `.xlsx`, `.xlsm`, `.xls`; each format independently
  enable/disable-able via `DATASETS_ALLOW_CSV`/`DATASETS_ALLOW_XLSX`), and are queried
  through the **`DuckDBAdapter`** (`adapters/duckdb.py`) against a local DuckDB file — a
  third, independent execution path alongside "topic + configured warehouse" and "topic +
  Connections-registry warehouse." Notable production hardening baked into the ingest
  service (`services/dataset_ingest.py`): a byte-level **xlsx repair** step
  (`_repair_xlsx_if_needed`) that patches non-standard sheet-visibility XML before
  `pandas`/`openpyxl` touch the file (a real Excel-in-the-wild bug fix); a header-row
  **auto-detection** heuristic (`_detect_header_row`, scans up to 20 rows); an anomaly-row
  detector; and a **cell-value sanitizer** (`sanitize_cell`, default `max_len=200`) that
  truncates oversized cell content before it reaches a chart/LLM context.
  Source: `_ref/original_wayfinder/src/<pkg>/services/dataset_ingest.py`.
- **Connection pooling**: Databricks keeps a single lazily-created `WorkspaceClient`
  (no pool — one HTTP-based SQL Warehouses API client, reused across calls); Postgres uses
  a real `AsyncConnectionPool` (`open=False`, `autocommit=True` — "this is a read-only
  query path" per the adapter's own comment) with an explicit `aclose()` for graceful
  shutdown; SQL Server has an equivalent pool. Databricks and Postgres/SQL Server therefore
  have materially different resource-lifecycle shapes — the Databricks case has no pool
  to size or exhaust, the SQL adapters do.

## Execution path

- **`/plan` vs `/run` split** (`api/routes/nlq.py`): `/plan` runs routing → SQL generation →
  validation and returns generated SQL **without executing it**; `/run` calls `/plan`'s
  logic internally then hands off to a **self-curation orchestrator**
  (`flows/sql_self_curate.py`) that actually executes against the warehouse. This maps
  cleanly onto two distinct capability scopes (`CapabilityScope.QUERY_PLAN` vs
  `QUERY_EXECUTE`) already enforced at the route layer — a plan-only caller never reaches
  a code path that touches the warehouse.
- **Idempotency**: `/run` accepts a client-supplied `idempotency_key`, scoped to
  `(operation, tenant_id, actor_id, client_key)`, checked against an `idempotency_cache`
  table before doing any work and populated after — a retried identical request short-
  circuits to the cached `NLQResponse` rather than re-executing against the warehouse.
- **The self-curation flow** (`flows/sql_self_curate.py`, a small `penguiflow`-based node
  graph: `prepare → fix_validation → execute → fix_execution → finalize`):
  - If the SQL failed **validation**, `fix_validation` calls an LLM-backed `SQLFixer`
    (`inference/sql_fixer.py`) with the validation error codes + business/structured
    context, then proceeds to execute the fixed SQL.
  - `execute` runs `warehouse.execute(sql, limit=state.max_rows)` wrapped in
    `asyncio.wait_for(..., timeout=state.timeout_seconds)` when a timeout is configured
    (application-level cancellation; nothing prevents the underlying driver call from
    continuing server-side after the Python task is cancelled — this is a *client-side*
    timeout, not a session-level one).
  - On a runtime `TimeoutError` or any other exception, `fix_execution` sends the error
    message + SQL back to the same `SQLFixer` for one corrective retry, gated by
    `fix_attempts >= 2` (validation-fix attempt + execution-fix attempt share the same
    counter, so at most ~2 LLM-assisted repair round-trips happen before giving up).
  - `finalize` flags `self_curation_replan_triggered` when execution succeeded but
    returned **zero rows** — the `/run` route (`_execute_run_with_self_curate` in
    `api/routes/nlq.py`) uses that signal to trigger exactly **one** full re-plan
    (`_execute_plan` called again) before giving up, guarded by a `replan_attempts < 1`
    check to prevent runaway recursion.
- **Read-only enforcement is a single upstream gate, not defense in depth.** The
  `SQLValidator` (`services/sql_validator.py`) parses SQL with `sqlglot` and:
  - allowlists only `Select`/`With`/`Union`/`Intersect`/`Except` as the top-level
    statement (`governance.disallowed_statement` otherwise);
  - blocks `Insert`/`Update`/`Delete`/`Merge`/`Create`/`Alter`/`Drop`/`Truncate` nodes
    **anywhere** in the parsed tree (not just top-level — a nested subquery containing DDL
    is also caught);
  - for the `duckdb` dialect specifically, additionally blocks `Command`/`Copy`/`Attach`/
    `Detach`/`Pragma`/`Call` (DuckDB-specific escape hatches that a generic SQL blocklist
    would miss);
  - rejects multiple statements via a naive semicolon-position check
    (`governance.multiple_statements`);
  - runs a **regex** injection-pattern heuristic (`'\s*OR\s*'1'='1'`, case-insensitive) as
    a pre-parse belt-and-braces check — acknowledged in the code as a heuristic, not a
    complete defense, and easily bypassed by any equivalent tautology the regex doesn't
    enumerate;
  - schema-allowlists every referenced table/column against the **topic pack** metadata
    routed for that query (not a static database-wide allowlist — table/column
    permissibility is scoped per NLQ *by the semantic-routing result*), including a
    join-path reachability check (`_tables_joinable`) that rejects a query joining two
    tables from the same topic if no `join_paths` edge connects them.
  - **Nothing below this gate re-checks read-only-ness.** The `WarehouseAdapter.execute`
    implementations (Databricks, Postgres, SQL Server, DuckDB) execute whatever SQL string
    they are handed; Postgres opens its pool with `autocommit=True` specifically because
    "this is a read-only query path" (adapter comment) — an assumption, not an enforced
    property (no read-only transaction, no restricted DB role visible in the adapter
    code, no `SET TRANSACTION READ ONLY`). If the validator is ever bypassed, mis-configured,
    or wrong about a new dialect's DDL surface, the adapter has no independent read-only
    guarantee.
- **Row limiting is inconsistent across adapters, deliberately.** Databricks and (in the
  client predecessor's DuckDB adapter) wrap the statement in
  `SELECT * FROM (...) AS subquery LIMIT {n}` — a **string-rewrite** approach. The fork's
  Postgres/SQL Server adapters explicitly **reject** this pattern in a code comment
  ("wrapping the statement in a derived table is dialect-sensitive and legally discards
  the inner `ORDER BY`") and instead enforce the limit at the **cursor** level
  (`cursor.fetchmany(limit)` for Postgres; SQL Server uses `SELECT TOP (n)` textually
  where the query has no CTE/set-op complicating it). This divergence is a real, evidenced
  lesson: LIMIT-by-wrapping silently breaks ordering guarantees for any query with a
  top-level `ORDER BY`.
- **Dialect handling** is adapter-scoped (`dialect` class attr: `databricks`, `postgres`,
  `tsql`, `duckdb`, `sqlite` for mock) and threaded through to the SQL generator/validator
  as a string so prompts and `sqlglot` parsing target the right grammar; **column type
  classification** was unified in the fork (`domain/column_types.py`) into one
  `TypeCategory` enum (`NUMERIC`/`TEMPORAL`/`BOOLEAN`/`TEXT`/`STRUCTURED`/`BINARY`/
  `UNKNOWN`) with a single `classify_type(raw_type, dialect)` function — the module's own
  docstring cites the motivating bug ("a SQL Server `money` column silently failed every
  one of [the ad hoc per-module type checks]") that a single dialect-agnostic vocabulary,
  computed once at schema-discovery time and stored on `ColumnInfo`, fixes.
- **No query-execution timeout exists at the connection/session level** in any adapter —
  Databricks has a **polling** timeout (300s) for waiting on an already-submitted
  statement to finish, not a query-cancellation timeout; Postgres/SQL Server pools set no
  `statement_timeout`; the only cancellation lever anywhere is the self-curation flow's
  `asyncio.wait_for`, which abandons the Python-side await but does not confirm the
  warehouse-side statement actually stopped running.

## Result shaping

- `QueryResult` (`domain/protocols.py`) is the adapter-agnostic execution output:
  `rows` (sequence of column-keyed mappings), `columns`, `row_count`, `execution_time_ms`.
  Every adapter returns this same shape regardless of dialect.
- `_preview_from_query_result` (`flows/sql_self_curate.py`) converts `QueryResult.rows`
  (mapping-shaped) into an ordered `ResultPreview` (`columns` + row-of-lists) for the API
  response — a deliberate flattening so a client doesn't have to know per-row dict key
  order matches column order.
- No pagination exists beyond the `max_rows`/`limit` ceiling set at plan/run time — a
  result set is fetched once, capped, and returned whole; there is no cursor/offset
  contract for "get the next page of this same query."
- A `presentation` envelope (`services/presentation.py`, gated by
  `settings.presentation.v1_enabled`) attaches chart/answer-shaping metadata to a
  successful curate result via `_attach_presentation_to_curate_result` — the chart-spec
  generation step Chartworks' own `charts/` package will need an analogue of. A capped
  **sample window** (200 rows, per a comment in `api/routes/nlq.py`: "rows beyond that are
  never inspected") limits how much of a result the presentation ranker itself looks at,
  independent of the `max_rows` cap already applied to the SQL execution itself.
- **No query-result cache.** The predecessors run a rich per-pipeline-stage TTL cache
  system (`services/cache_base.py`'s `AsyncTTLCache`, coordinated by
  `services/cache_coordinator.py`) — named caches exist for embeddings, retrieval, span
  extraction, context packing, template selection, and **SQL validation results**
  (`services/validation_cache.py`, keyed on a hash of `(tenant, sql, dialect, topic-version)`)
  — but there is no equivalent cache for the actual **rows returned by warehouse
  execution**. Every `/run` call (short of an exact idempotency-key replay) re-executes
  against the customer warehouse even for a semantically identical question asked twice.
  Each named cache carries independent `enabled`/`ttl_seconds`/`max_entries` config
  (`config/settings.py`, `CACHING_*` env keys) and is tenant-scoped via a `CacheScope`
  dataclass, with a `GLOBAL_CACHE_TENANT` fallback bucket used (and metrics-flagged) only
  when no tenant is resolvable — worth naming explicitly since an unscoped-cache bug is
  exactly the kind of cross-tenant leak P3 exists to prevent.

## Background jobs

Two **independent** scheduling/execution systems exist, easy to conflate:

1. **The generic `jobs` queue** (`stores/sql/metadata/{postgres,sqlite}.py`, `jobs` +
   `job_batch_checkpoints` tables) backs all semantic-model **maintenance** work: schema
   discovery, topic enhancement/regeneration/reindex/replay, table addition, schema
   refresh, dataset cleanup/schema enhancement, relationship discovery, "learn positive"
   (feedback-driven learning), and GEPA prompt optimization/autopilot
   (`workers/*.py`, one dedicated worker class per job type, e.g.
   `SchemaDiscoveryWorker` in `workers/schema_discovery.py`). Postgres claims a row with
   `FOR UPDATE SKIP LOCKED`; each claimed job carries a `lease_owner`/`lease_expires_at`
   pair refreshed by worker heartbeats (`registry.settings.worker.heartbeat_interval_seconds`)
   and a configurable `lease_timeout_seconds`, so a crashed worker's job becomes reclaimable
   once its lease lapses — the same lease-reaping shape as other job systems in the
   ecosystem, evidenced here specifically for Chartworks' own predecessor rather than
   inferred from a sibling.
2. **The scheduling subsystem** (`scheduling/dispatcher.py` + `scheduling/executor.py` +
   `scheduling/store.py`) is entirely separate and **user-facing**: a `ScheduleDispatcher`
   polls every `poll_interval_seconds` (default 15s) for due `schedules` rows (cron or
   interval triggers — `scheduling/triggers/{cron,interval}.py` — plus stubbed
   `condition`/`event` trigger kinds), batches up to `max_batch_size` (default 100) due
   schedules per tick, and enqueues one `schedule_execute` job per due schedule into the
   *same* generic `jobs` queue from group 1 — the two systems share the execution
   substrate but not the triggering logic. Schedule **targets** (`scheduling/targets/`)
   are pluggable: `saved_query.py` (re-run a saved NLQ and, per
   `warehouse.execute(sql_text, params=params, limit=saved_query.max_rows)`, apply the
   same `max_rows` capping as an interactive run), `report.py`, `condition_check.py`
   (alerting-style condition evaluation), `custom.py`.

## Keepers

- **Two-phase plan/run split with distinct capability scopes** — lets an agent/consumer
  request "show me the SQL you'd run" without ever touching the warehouse, and makes the
  execute-vs-plan authorization boundary a route-level fact, not a convention. Directly
  useful for Chartworks' `internal/nlq` → `internal/exec` boundary.
- **Idempotency-key-scoped `/run` caching** — prevents duplicate warehouse execution on
  client retries without requiring a semantic result cache; cheap, tenant/actor-scoped,
  worth carrying as-is.
- **Tenant-scoped, encrypted, per-connection Connections registry with a `NullWarehouseAdapter`
  fail-loud fallback** (fork only) — this is the clean answer to "Chartworks queries
  customer data sources it doesn't own": secrets encrypted at rest with key rotation
  (`MultiFernet`), never returned on read, and a construction-time placeholder adapter
  that raises instead of silently defaulting to *some* connection. This is close to a
  direct template for Chartworks' own data-source credential handling (§7 security rules).
- **AST-based (not regex-based) SQL governance, schema-scoped per routed topic, with a
  join-path reachability check** — the strongest existing prior art for Chartworks' P1
  SQL-safety property (read-only + schema allowlist + injection guardrails). The dialect-
  aware blocked-node-list pattern (base DDL/DML list + dialect-specific additions like
  DuckDB's `Attach`/`Copy`/`Pragma`) generalizes cleanly to more warehouse dialects.
  `governance.disallowed_statement`/`table.out_of_scope`/`join.unreachable_tables` are
  reusable *shapes* for Chartworks' own typed validation-error vocabulary (not the codes
  themselves — those stay behind the predecessor boundary as prior art, not code).
- **Cursor-level row limiting over string-rewrite LIMIT wrapping** — the fork's own code
  comment records *why* (`ORDER BY` gets silently discarded by a naive subquery wrap);
  worth encoding as a standing rule for Chartworks' exec package rather than relearning it.
- **A single dialect-agnostic column-type classification (`TypeCategory`/`classify_type`)
  computed once at discovery time** — avoids the "N ad hoc per-module type checks, one of
  which quietly misses a dialect's oddball type name" bug class the fork's own docstring
  names. Directly reusable pattern for a Chartworks semantic model that must span
  Postgres/Databricks/etc.
- **A bounded, LLM-assisted execution-error repair loop with a hard attempt cap** — turns
  "the generated SQL almost works" into a usable answer without unbounded retry cost; the
  `fix_attempts >= 2` ceiling and the "zero rows → one re-plan, never more" rule are both
  concrete, evidenced bounds worth carrying forward (not the specific counter mechanics,
  but the "cap total LLM-repair rounds, fail loud past the cap" principle — P4-aligned).
- **One generic leased job queue for all background maintenance, kept separate from the
  user-facing cron/interval scheduler** — avoids a proliferation of bespoke job tables while
  still letting user-visible "run this saved query every Monday" scheduling have its own,
  simpler trigger model. A clean seam boundary to inherit.

## Scars

- **Read-only enforcement lives entirely in one upstream validator; no adapter or
  connection-level backstop exists.** Every adapter's `execute()` runs the SQL string it is
  given; the Postgres adapter's `autocommit=True` is justified in a comment by an assumption
  ("this is a read-only query path"), not enforced by a restricted database role, a
  read-only transaction, or a session-level guard. A validator bug, a new dialect's
  unenumerated write-capable statement, or a direct call to `adapter.execute()` bypassing
  the validator (both `saved_query.py` and `sql_self_curate.py` call `warehouse.execute`
  directly — nothing in the adapter *requires* prior validation to have occurred) all
  reach the customer warehouse unguarded. This is precisely the gap CLAUDE.md's P1
  SQL-safety property (§1, §6) exists to close for Chartworks — evidence that "validate,
  then trust the caller forever" is not sufficient; defense-in-depth (e.g., a read-only DB
  role/connection option, or a policy that the exec layer *itself* independently re-checks
  before dispatch) should be evaluated for the RFC even though it's implementation
  overhead the predecessor never paid.
- **A weak regex injection heuristic sits next to a strong AST-based one, in the same
  function, with no indication the regex adds anything the AST check doesn't already
  cover.** `'\s*OR\s*'1'='1'` (case-insensitive) only catches one classic tautology
  pattern; any equivalent (`'x'='x'`, numeric tautologies, UNION-based tricks not already
  blocked by the AST allowlist) sails through. Dead-weight code that looks like security
  but mostly is not — worth not carrying forward as a substitute for AST-level guarantees.
- **The ~50-table own-schema sprawl** (see "Own data model") is itself a scar: heavy
  duplication of query/SQL/topic identifiers across `nlq_queries`/`sql_generations`/
  `sql_generation_examples`/`nlq_inference_cache`, each grown independently as a pipeline
  stage needed its own persistence, with no evident single schema-ownership review. This
  is the direct, concrete referent for CLAUDE.md §6's "Store schema is budgeted" rule and
  the "id in seven columns across six tables" warning — not a hypothetical.
- **Client-side-only execution timeouts.** `asyncio.wait_for` around `warehouse.execute`
  abandons the *awaiting* coroutine on timeout but does not confirm cancellation reached
  the warehouse — a timed-out query may keep consuming warehouse compute after Chartworks
  has already told the caller it failed. No adapter sets a session/statement-level timeout
  (Postgres's `statement_timeout`, SQL Server's query timeout, a Databricks warehouse-side
  cap) as a second, server-enforced backstop.
- **Row-limiting is dialect-inconsistent by necessity, not by design intent stated
  upfront** — the client predecessor's original LIMIT-by-string-wrapping approach silently
  discarded `ORDER BY` on wrapped queries (a real, fixed bug per the fork's own comment);
  it was fixed per-adapter as new adapters were added rather than as a single documented
  execution-layer contract decided once. Chartworks should decide and document this
  up front rather than rediscover it per adapter.
- **No warehouse query-result cache** — every distinct `/run` call re-executes against the
  customer's warehouse, which is a real cost/latency lever the predecessors never built
  despite building six other cache layers upstream of it. Not necessarily wrong (result
  caching against a live, changing warehouse has its own correctness hazards — staleness,
  per-access-scope invalidation), but conspicuously unaddressed; worth an explicit RFC
  decision rather than another unstated gap.
- **Three execution paths (configured single warehouse / per-connection registry
  warehouse / local DuckDB dataset mode) coexist without a single unifying "how do I run
  SQL against source X" contract** — each has its own adapter resolution, its own
  quota/size limits, and its own row-shaping code path. A consolidation worth planning for
  explicitly rather than accreting a fourth path later.

## Open questions for the RFC

1. Does Chartworks' P1 SQL-safety property mandate a **second, independent read-only
   enforcement layer** below the AST validator (a restricted database role, a read-only
   transaction/session flag, or a data-source-adapter-level assertion), or is a single
   well-tested AST gate + adversarial test suite (per CLAUDE.md §11) considered
   sufficient? The predecessor evidence argues for defense-in-depth given how easy it is
   to accidentally call `execute()` without validating first.
2. Should generated-SQL execution get a **server-enforced statement timeout** (Postgres
   `statement_timeout`, SQL Server query timeout, warehouse-side query caps) in addition
   to (or instead of) an application-level `asyncio`-style cancellation, so a cancelled
   query actually stops consuming the customer's warehouse compute?
3. Does Chartworks want a **query-result cache** at all, and if so, at what layer
   (semantic/question-level vs. exact-SQL-level) and with what invalidation story given
   P1's deny-by-default access model — a cached result computed under caller A's access
   grant must never be served to caller B under a different grant?
4. Is the **Connections registry** (tenant-scoped, encrypted, multi-kind warehouse
   credentials with a status lifecycle) the right shape for how Chartworks' "sources"
   package (§3, `TBD-by-RFC`) manages customer data-source credentials, or does the
   ecosystem's own secrets/credentials story (if one exists elsewhere, e.g. via Pengui)
   supersede it?
5. Should row-limiting be a single documented **cursor/fetch-level** contract mandated for
   every adapter from V1 (per the fork's own lesson about string-rewrap breaking
   `ORDER BY`), rather than left to each adapter's discretion?
6. CSV/XLSX upload IS Chartworks V1 scope — the bootstrap boundary quote carves it *out
   of Soundings and into Chartworks* (Chartworks is the Explorer seat), and the kickoff
   (D-013) confirms uploaded CSV/XLSX/Parquet connectors. The real question the
   predecessor evidence poses: does the upload path go through the **same** data-source
   adapter seam and governed engineering/access path as warehouse sources (P7 — one
   primitive family), rather than re-growing the predecessors' parallel "dataset mode"
   with its own weaker identity path and third execution route?
7. Should the **per-pipeline-stage cache taxonomy** (one named `AsyncTTLCache` per stage,
   centrally coordinated) be adopted as Chartworks' standard caching pattern across
   `nlq`/`semantics`/`gateway`, or does Chartworks want fewer, coarser-grained caches from
   the start to avoid the same "six caches, no result cache" gap?
