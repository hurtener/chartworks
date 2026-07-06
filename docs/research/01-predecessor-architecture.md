# Brief 01 — Predecessor architecture: services, API surfaces, config, observability

> Status: draft · 2026-07-06 · sources: both predecessors

**Hygiene note on citations.** Both predecessors lay out their code as
`src/<pkg>/...`, where `<pkg>` is the top-level package directory whose name is
the product name. Per repo hygiene, that name is never written out; every path
below elides it as `<pkg>` (e.g. `src/<pkg>/api/app.py`). All other path
segments are literal. Neither predecessor's internal orchestration-library
brand name is used either — it is called "the flow-orchestration library"
throughout; it is a pinned third-party dependency, not an ecosystem or client
identifier.

## Summary

Both predecessors are a single FastAPI process that is simultaneously the HTTP
API, ~13 co-deployed in-process asyncio poll-workers, and (client predecessor
only) a background OAuth-refresh loop — no separate worker process, no broker.
All state lives in one relational store (SQLite dev / Postgres prod) reached
through a hand-rolled `MetadataStore`, with FAISS or pgvector for embeddings.
The core shape is **router-first NLQ→SQL**: triage → semantic routing to a
topic (the semantic-model unit) → context packing → retrieval of
templates/rules/examples → DSPy-driven SQL generation → AST-based
(`sqlglot`) validation and repair → a chart-kind ranker/generator stage. Auth
is **symmetric HS256 JWT with a hand-rolled encoder/decoder** — a direct P2
violation Chartworks must not inherit. Access is a richer-than-P1 tenant role
+ per-resource ACL + capability-scope model that already resembles what P1/P3
gesture at. Metrics are an in-process counter with **no export path**; tracing
goes through OTel + an ML-experiment tracker. The generalistic predecessor
diverges materially on deploy target (Azure Container Apps vs. the client
predecessor's Databricks Apps) and on access model (a genuinely
deny-by-default, unified access resolver plus tenant-managed, encrypted
warehouse-connection registry) — both are strong RFC candidates.

## Architecture

**Process shape.** There is exactly one long-running process: a FastAPI ASGI
app (`src/<pkg>/api/app.py`, `create_app()`). Its `lifespan` context manager
does all of the following at boot, in-process, before serving:

1. Refresh a platform OAuth key when running under the client predecessor's
   deployment platform (`_ensure_databricks_api_key`), with a background
   45-minute refresh loop.
2. Warm a router/topic cache (`get_router_cache_refresher().refresh()`) —
   startup **hard-fails** if this raises.
3. Pre-load the embedding model and, if enabled, a cross-encoder reranker
   model, in a background thread — failures here are only logged, not fatal
   (an inconsistency: cache refresh is fail-fast, model warmup is not).
4. Construct and `.start()` **thirteen** worker objects (schema discovery,
   topic enhancement, topic regeneration, topic reindex, topic replay,
   "learn positive", relationship discovery, table addition, schema refresh,
   two GEPA prompt-optimization workers, dataset cleanup, dataset-schema
   enhancement) plus, if scheduling is enabled, a schedule dispatcher and a
   schedule execution worker. All run as `asyncio.create_task` loops inside
   the same process as the API server — there is no separate worker
   deployment, no message broker.
5. Mount ~19 routers plus (if built) a static SPA bundle.

Every worker follows one shape (e.g. `src/<pkg>/workers/schema_discovery.py`,
`SchemaDiscoveryWorker`): construct with a shared `ServiceRegistry`, spin up
`N` (`worker.concurrency`) asyncio tasks each polling `store.claim_job(...)`
on a fixed interval, process one job, sleep, repeat until a shutdown event
fires. This is a lease-claim, store-backed queue — the same "no external
broker, cooperative claim" shape as a sibling NLQ predecessor pattern, just
instantiated once per job *type* (13 times) rather than once for the whole
ingestion pipeline. `src/<pkg>/scheduling/dispatcher.py` and
`src/<pkg>/scheduling/executor.py` are a fourteenth and fifteenth instance of
the same polling shape, layered with cron/interval/event/condition trigger
evaluation (`src/<pkg>/scheduling/triggers/`) and pluggable execution targets
(saved query, report, condition-check, custom — `src/<pkg>/scheduling/targets/`).

**Router-first NLQ pipeline** (`src/<pkg>/flows/*.py`, `src/<pkg>/retrieval/*.py`,
`src/<pkg>/inference/*.py`), each stage a node in the flow-orchestration
library's graph:

1. **Triage** (`retrieval/triage.py`) — cheap candidate scoring, optional
   cross-encoder rerank, MMR diversification.
2. **Semantic routing** (`flows/semantic_routing.py`) — routes the question to
   a *topic* (a semantic-model unit: measures/dimensions/tables/relationships)
   using the pre-warmed router cache.
3. **Context packing** (`flows/context_packing.py`) then **semantic retrieval**
   (`flows/semantic_retrieval.py`, `retrieval/semantic.py`) — pulls the
   matched topic's templates, business rules, and worked examples.
4. **SQL generation** (`flows/sql_generation.py`, `inference/sql_generator.py`)
   — DSPy-structured generation against the topic's schema and retrieved
   context.
5. **Validation + self-curation** (`services/sql_validator.py`,
   `domain/sql_validation.py`, `flows/sql_self_curate.py`,
   `inference/sql_fixer.py`) — a `sqlglot`-AST-based validator (not
   regex/string matching) checks syntax, semantic joinability, and a
   topic-scoped table/column allowlist; a **SELECT-only** gate
   (`services/sql_validator.py` ~line 734) rejects anything else; on failure,
   an LLM repair loop attempts a bounded number of fixes before failing loud.
6. **Response assembly** (`flows/response_assembler.py`) — turns the raw
   inference result into a normalized, signal-rich response shape (routing
   evidence, confidence, generated SQL, warnings).
7. **Presentation / charts** (`src/<pkg>/presentation/`) — a chart-kind
   *catalog* (`presentation/catalog/`) of ~13 kinds (bar, column, line, area,
   pie, donut, scatter, heatmap, treemap, grouped/stacked bar/column, KPI
   card, table), each with a pure `generate_options(rows, slot_mapping, ...)`
   function producing a charting-library option spec
   (`presentation/generators/*.py`), a rule-based per-kind *suitability*
   scorer (`presentation/suitability/*.py`, does this data fit this chart
   kind), an LLM *ranker* (`presentation/llm_ranker.py`) to pick among
   suitable kinds, and a slot-mapping *validator* (`presentation/
   mapping_validator.py`). This directly prefigures Chartworks' "charts"
   pipeline stage.

**Defensive layering exists but access is computed once, not
fetch-then-filter**, per the generalistic predecessor's
`services/access_resolver.py` docstring: a single "unified topic access
resolution" function collapses what had been two divergent code paths (a
user path and a service-account path) into one, with an explicit
deny-by-default policy: no grant ⇒ not visible, `role is None` ⇒ deny, tenant
admin bypasses, non-admin combines role-threshold grants ∪ per-user ACL
grants, service accounts get self-sufficient grants only — and the tenant
boundary is *always* intersected afterward so a grant can never leak
cross-tenant. This is materially closer to Chartworks' P1/P3 than the
inherited-principle stub in `CLAUDE.md` §1 currently spells out, and is
evidence the RFC can lean on directly (see Open questions).

## API surfaces

Both predecessors are **HTTP-only** — grepping either tree for an MCP
server/tool surface returns nothing; the dual HTTP+MCP surface Chartworks
plans is new ground for this lineage too, exactly as brief 01 in the sibling
Soundings repo found for its own predecessor.

Router inventory (client predecessor, `src/<pkg>/api/app.py`
`include_router` calls; the generalistic predecessor adds two more — see
below), grouped by purpose, all under `/v1`:

| Group | Prefix | Purpose (illustrative) |
|---|---|---|
| Health/config | `/v1/healthz`, `/v1/config` | liveness + public UI feature flags |
| Auth | `/v1/auth/*` | password login/signup, "mint JWT from a trusted upstream header," `whoami`, logout, public auth config |
| Tenants | `/v1/tenants/*` | tenant CRUD, membership, invites, service-account CRUD/rotation (admin/tenant-admin gated) |
| Admin/security | `/v1/admin/*` | platform-admin user/tenant/service-account listing, membership grants, LLM-provider opt-out per tenant |
| Catalog | `/v1/catalog/*` | discovery summary, scoped topic listing/inspection, "preflight a vague ask" |
| Topics | `/v1/topics/*` | topic CRUD, versioned generation jobs, export/import bundles, sharing/visibility, **the topic lifecycle surface** — draft → review → promoted → deprecated stages with an audit trail (`domain/metadata.py: TopicVersionStage`, `TopicVersionAuditAction`) |
| Topic rules | `/v1/topics/{id}/rules` | business-rule CRUD scoped to a topic, activate/retire/replay |
| Underspecification | `/v1/topics/{id}/underspec-patterns` | clarification-pattern CRUD + regenerate + replay for ambiguous questions |
| NLQ | `/v1/nlq/*`, `/v1/sessions/*` | plan/run/agent-query/agent-query:execute/agent-query:refine/preflight/debug/resolve-coefficient — the actual question-answering surface |
| Datasets | `/v1/datasets/*`, `/v1/dataset-schemas/*` | ad hoc spreadsheet upload (CSV/XLSX), sheet selection, schema save — a "Spreadsheet Mode" that sits *outside* the topic/warehouse model |
| Templates | `/v1/templates/*` | seeded SQL templates: promote/deprecate/retire, validate/auto-repair against a topic version, paraphrase management, confidence weight tuning |
| Relationships | `/v1/relationships/*` | cross-topic relationship discovery, confirm/deprecate |
| Rules | `/v1/rules/*` | tenant business rules + a curated "global library" tenants can opt into, propose→shadow→promote→retire workflow, per-rule usage stats |
| Schedules | `/v1/tenants/{tenant_id}/schedules/*` | recurring job CRUD, pause/resume/retire, run history |
| GEPA | `/v1/admin/gepa/*` | prompt-pack CRUD/activate/hot-reload, SQL-generation telemetry capture, first-try success ratio, optimization job enqueue/status, autopilot status/force-run |
| Maintenance | `/v1/maintenance/*` | operator maintenance actions (not enumerated in this pass) |
| Feedback | `/v1/feedback*` | runtime + clarification feedback, calibration training data, per-topic accuracy |
| Dashboard | `/v1/dashboard/*` | stats + recent query activity |
| Jobs | `/v1/jobs/*` | generic job status/creation |

Generalistic-predecessor-only additions: `api/routes/warehouse_connections.py`
(tenant-managed warehouse connection CRUD) and
`api/routes/tenant_business_domain.py`. The client predecessor has no
equivalent — its warehouse target is fixed via env config, not a per-tenant
CRUD surface (see Keepers).

**Auth on every route** flows through `api/authz.py`
`RequestPrincipal` resolution: a proxy-safe custom header (a
`x-<product>-authorization`-shaped name in the client predecessor, elided per
hygiene rules) carries a Bearer JWT
or a service-account token; `require_tenant_context_with_role` /
`require_membership_role` then gate by `TenantRole` (admin/member/viewer) and
`CapabilityScope` (fine-grained: `catalog.read`, `topic.read`, `topic.write`,
`query.preflight`, `query.plan`, `query.execute`, `sessions.read`).
Platform-admin and per-tenant-admin routes are separately gated. A CSRF check
(`_csrf_protect` middleware) guards cookie-authenticated mutating requests
specifically to exempt header-bearer clients. A **platform auto-auth**
middleware exists for the client predecessor only: it auto-provisions a user
from a configured "trusted platform identity" header and silently mints and
injects a Bearer JWT into the request — a header-trust-to-token conversion
worth scrutinizing against P2 (see Scars).

## Configuration

Both predecessors use `pydantic-settings` (`BaseSettings`), loaded from
environment variables with `.env` support, real env > `.env` file > code
default. `src/<pkg>/config/settings.py` (~1,700 lines) is dominated by a
`_FLAT_ENVIRONMENT_MAP` — a single dict mapping ~120 flat `SCREAMING_SNAKE`
env-var names onto nested dotted-tuple settings paths (e.g. `DBX_HOST` →
`(warehouse, databricks_host)`), so operators can set either the flat legacy
name or a `SECTION__FIELD` double-underscore nested name. Config domains,
by volume:

- **Warehouse** (customer data target): `kind` enum
  (databricks/postgres/bigquery/snowflake) + per-kind connection fields, most
  as `SecretStr`. The client predecessor fixes this per-deployment via env;
  the generalistic predecessor additionally supports a tenant-managed,
  encrypted, rotatable connection registry (see Keepers).
- **Storage** (the service's own metadata): a Postgres DSN (prod) or a SQLite
  path (dev/test) + an object-store URI — the same system-of-record /
  blob-store split Chartworks already plans, and the same instinct behind
  D-004's Postgres-only V1 store.
- **Vector**: backend enum (FAISS / pgvector) + embedding model id + pinned
  dimensions.
- **Worker**: concurrency, lease timeout, heartbeat interval, max retries —
  shared across all 13+ poll-workers.
- **Scheduling**: enabled flag, dispatcher poll interval, max due-batch size.
- **Limits**: plan row cap, run timeout, router confidence threshold,
  inference/idempotency cache TTLs.
- **Models**: default LLM + temperature/max-tokens, a pricing-table path
  (`config/model_pricing.yaml`), per-stage token/batch budgets for four
  distinct enhancement sub-tasks (measure/dimension/semantic/derived-KPI),
  each independently batch-sized and token-budgeted.
- **Retrieval / caching**: five independently configurable cache domains
  (embedding, retrieval-results, span-extraction, context-pack,
  template-selection, sql-validation-result), each with its own
  enabled/ttl/max-entries triple — a "cache everything, independently" default
  posture.
- **SQL validation**: a `cte_scope_mode` enum (progressive/strict/legacy) and
  a dedupe-enabled flag — i.e. the validator's strictness is itself a runtime
  knob, not fixed.
- **Auth/identity**: auth mode (standalone/platform), JWT secret/issuer/
  audience/TTL, a "trusted subject header" name, password-login and
  self-signup toggles, invite-code requirement, a *separate* "trusted user
  header" + legacy-header-allowlist pair specifically for dataset-mode
  identity resolution.
- **Datasets** ("Spreadsheet Mode"): enabled flag, upload dir, TTL, cleanup
  interval, max file bytes/rows, CSV/XLSX allow flags — a config surface that
  exists specifically because this mode explicitly steps *outside* the
  topic/warehouse boundary Chartworks' bootstrap doc draws.

No large `model_validator(mode="after")` fail-loud block comparable to the
sibling Soundings predecessor was found in a single obvious place in this
pass; validation is instead scattered as `field_validator`s per nested config
model. This is worth a closer look in a dedicated config-hygiene pass — it
was not exhaustively verified here.

## Observability

**Logging**: standard `logging.getLogger(__name__)` per module; no structured
logger wrapper or request-scoped context var was found comparable to the
sibling predecessor's JSON formatter + trace-id propagation. Errors are
generally full-`logging.exception` (stack trace to log), which is fine for
operators but means log *shape* is not normalized (P4's "structured,
content-free log" is not the default posture here).

**Metrics — a real scar**: `src/<pkg>/observability/metrics.py` is a
thread-safe in-process `Counter` registry (`increment_counter`,
`snapshot_counters`, `reset_counters`). It is called from many places
(dataset auth failures, cache hits, etc.) but **`snapshot_counters` is
imported nowhere outside the observability package itself** — no `/metrics`
endpoint, no Prometheus/OTel exporter reads it. These counters exist purely
for test assertions; in a running deployment they accumulate and are never
seen by an operator. This is a dead-end telemetry sink, distinct from —
and easy to mistake for — the separate OTel/tracing path below.

**Tracing / experiment tracking**: `src/<pkg>/observability/telemetry.py`
wires an OTel tracer + counter (optional dependency, degrades to no-op if not
importable) and business-flow-node telemetry — the flow-orchestration
library's own event stream (node start/success/error/cancelled) is bridged
into both OTel spans and an ML-experiment-tracking backend
(`observability/mlflow_tracing.py`, optional dependency, degrades to plain
logging if unavailable). This is the same "best-effort degrade to logging if
the observability backend isn't configured" shape as the sibling Soundings
predecessor, and shares its double-sink caveat: audit-adjacent events
(security blocks, ACL denials) and plain business telemetry are not
obviously separated into a guaranteed content-free channel — worth checking
in a dedicated audit-hygiene pass before assuming route-level `@audited`
annotations (see Keepers) are sufficient on their own.

**Audit**: a lightweight, explicit opt-in model —
`src/<pkg>/api/audit.py` defines an `@audited(action=..., resource_type=...,
target_param=...)` decorator; `src/<pkg>/api/middleware/audit.py` reads that
marker off the matched route, resolves a request id (from
`x-request-id`/`x-correlation-id` or a generated UUID), classifies the
result as success/denied/error from the status code, and emits a
structured `AuditEvent` (actor, target, result — no request body captured by
default). This is a good, minimal, per-route-opt-in shape, though it only
fires for routes explicitly decorated — coverage was not verified to be
complete across all mutating routes in this pass.

**Health**: a single `/v1/healthz` returns `{status, environment}` — no
dependency-reachability breakdown (DB/warehouse/vector-store) comparable to
Chartworks' planned `/healthz` + `/readyz` split was found in this pass.

## Runtime & dependencies

Single Python package, no separate worker binary. Dependency surface
(`pyproject.toml`) confirms the architecture above: `fastapi` + `uvicorn`
(API), `psycopg[binary,pool]` + `pgvector` (Postgres + vector prod backend),
`aiosqlite` (dev/test store), `sqlglot` (the SQL AST validator/parser),
`dspy` + `litellm` (structured LLM programs, multi-provider routing),
`mlflow` (experiment tracking / tracing sink), the flow-orchestration
library (node-graph execution), `sentence-transformers` + `torch`
(local cross-encoder reranking), `spacy` with **English and Spanish** model
downloads (NLP preprocessing — a bilingual posture worth flagging for the
RFC's scope), `duckdb` + `pandas` + `openpyxl` (the CSV/XLSX "Spreadsheet
Mode" — an in-memory query engine over uploaded files; note the bootstrap
boundary quote carves CSV/XLSX *out of Soundings and into Chartworks*, and
the kickoff (D-013) confirms uploads are V1 scope — the design question is
*how* they enter, not whether). A `pg0-embedded` dev-only dependency provides an
embedded local Postgres for `./start.sh`, without weakening the prod-Postgres
requirement — i.e. no SQLite-in-prod shortcut, consistent with D-004's
Postgres-only V1 posture.

**Deploy targets diverge between predecessors** — the clearest top-level
architectural fork found in this pass: the client predecessor ships an
`app.py` Databricks-Apps entry point (`uvicorn.run("...api.main:app", ...)`,
reading `PORT` from the platform) plus `deploy/deploy.py` and
`deploy/*.app.yaml`; the generalistic predecessor instead ships a multi-stage
`Dockerfile` (Node frontend build → Python builder via `uv sync` → slim
runtime image) plus Azure Container Apps Bicep templates
(`deploy/aca/*.bicep`, `deploy/aca/*.bicepparam`) and an Azure Pipelines file.
Both still run the identical in-process-workers-plus-API shape once started;
only the container/platform wrapper differs. This is strong evidence the
runtime shape (one binary, co-deployed workers) is a real architectural
choice worth evaluating on its own merits — not an artifact of either
platform's constraints — while the platform-specific bootstrap code
(`_ensure_databricks_api_key`, the OAuth refresh loop) is exactly the kind of
platform coupling Chartworks' Go rewrite should not carry forward at all.

## Keepers

- **Router-first pipeline shape**: triage → route-to-topic → retrieve
  context → generate SQL → validate/repair → assemble a normalized response
  → rank/generate a chart. Maps directly onto Chartworks' provisional
  engineer → model → NLQ-to-SQL → charts pipeline (`CLAUDE.md` §1) and gives
  it concrete stage boundaries to design interfaces around.
- **AST-based SQL validation, not string matching**: `sqlglot`-parsed
  syntax + semantic joinability + a topic-scoped table/column allowlist +
  a hard SELECT-only gate (`services/sql_validator.py`). This is close to a
  reference implementation of the P1 SQL-safety property the RFC still owes
  Chartworks — read-only execution, schema allowlisting, and (via AST
  parsing rather than string concatenation) a structurally sound answer to
  the injection-guardrail requirement.
- **Unified, deny-by-default access resolver** (generalistic predecessor,
  `services/access_resolver.py`): one function collapsing a user path and a
  service-account path that had drifted apart, with an explicit "no grant ⇒
  no access, `role is None` ⇒ deny, tenant boundary always intersected after"
  policy and a documented distinction between an "admin bypass" sentinel and
  an "empty/deny" result. This is close to a working model for P1/P3's
  concrete mechanism.
- **Tenant-managed, encrypted, rotatable warehouse-connection registry**
  (generalistic predecessor, `domain/warehouse_connection.py`,
  `config/warehouse_secrets.py`): connections are tenant-scoped rows with a
  non-secret `config` and a `MultiFernet`-encrypted secret that is **never
  returned on read**; the cipher tries multiple keys on decrypt so a key can
  be rotated by prepending a new primary while old data stays readable. A
  strong candidate shape for however the RFC lets Chartworks tenants attach
  their own data sources.
- **Fine-grained capability scopes** (`domain/auth.py: CapabilityScope`):
  action-level (`query.execute` vs `query.plan` vs `query.preflight`), not
  just a coarse role — worth a look for Chartworks' own access primitive.
- **Versioned topic lifecycle with an audit trail**
  (`domain/metadata.py: TopicVersionStage` — draft → review → promoted →
  deprecated, `TopicVersionAuditAction`), plus visibility
  (private/tenant_public) and an explicit export/import bundle format
  (`/v1/topics/{id}:export`, `:import`). The exact divergence between the two
  predecessors on this lifecycle is brief 05's job; noted here only as an
  architectural pattern worth inheriting the *shape* of.
- **Chart-kind catalog + pure generator + rule-based suitability + LLM
  ranker** (`presentation/`): each chart kind is a small pure function with a
  typed slot contract, scored for fit independently of the LLM, then ranked —
  a clean separation Chartworks' "charts" stage can copy structurally.
- **Store-backed, lease-claim poll-workers, no broker**: proven at 13+
  instances without an external queue; a legitimate lightweight pattern for
  Chartworks' own background jobs (schema/topic maintenance) if Postgres-only
  is the storage constraint anyway (D-004).
- **Route-level `@audited` opt-in decorator**: cheap, explicit, greppable —
  a workable starting shape for Chartworks' audit requirement (§7).

## Scars

- **Symmetric JWT (HS256), hand-rolled**: `security/jwt.py` implements
  encode/decode by hand with `hmac.new(..., hashlib.sha256)` — this is
  exactly the `HS*` algorithm P2 requires Chartworks to reject at the parser.
  Do not adapt this module's *algorithm choice*; the encode/decode plumbing
  pattern (explicit `iss`/`aud`/`exp` checks, `parse_bearer_token` helper) is
  fine, the crypto primitive is not.
- **Header-to-token auto-provisioning** (`api/app.py`
  `_platform_auto_auth` middleware): trusts a configured identity header,
  silently mints a JWT from it, and injects the minted Bearer token back into
  the request's own header list for downstream code to "discover." This
  blurs the P2 line between "identity lives in the signed token" and "identity
  arrives via an `X-*` header" — the header *is* the identity source of
  truth here, the JWT is just a wrapper minted per-request. A dataset-mode
  identity path (`api/identity.py: resolve_dataset_principal`) does this even
  more directly: it reads a "trusted header" (with a legacy-header fallback
  list) and treats the string value as the principal, no token at all.
- **Metrics that go nowhere**: `observability/metrics.py`'s counters are
  never exported (no `/metrics`, no OTel bridge) — an entire observability
  axis that looks real (call sites everywhere) but is operationally inert.
  Anyone auditing "does this have metrics" from call-site greps alone would
  be misled.
- **13-to-1 worker-per-job-type proliferation, all co-deployed with the API
  process**: every new background concern got its own `Worker` class, its
  own poll loop, its own concurrency/lease config, all sharing the API
  process's resource envelope with no separate scaling or restart boundary.
  This is the "two-of-everything, bespoke call path" failure mode P7 exists
  to prevent, just expressed as "N workers" instead of "N auth models."
- **Startup fail-fast is inconsistent**: router-cache refresh failure at
  boot re-raises (fatal); embedding-model and cross-encoder warmup failures
  are only logged (non-fatal) — so a broken embedding model silently degrades
  first-request latency/quality rather than failing the health check, a P4
  tension worth resolving explicitly rather than inheriting by accident.
  Similarly, a schedule's dispatcher/executor "cancel" only sets a DB status
  in at least one predecessor code path noted elsewhere in this pass and does
  not reliably interrupt in-flight work — a documented-caveat pattern (not a
  TODO) that Chartworks should treat as a genuine gap, not a precedent.
  (See a closer parallel to this exact caveat already called out in the
  sibling Soundings predecessor's reembed-job-cancel behavior.)
- **A "Spreadsheet Mode" that grew as a parallel, weaker path**: CSV/XLSX
  upload + DuckDB in-memory querying exists as a first-class mode in *both*
  predecessors, config-flagged and API-surfaced (`/v1/datasets/*`) — but it
  sits **outside** the topic/warehouse model, with its own execution route
  and the weakest identity path in the codebase (trusted-header, no JWT at
  all, `api/identity.py`). Uploads ARE Chartworks V1 scope (D-013 — the
  bootstrap quote carves CSV/XLSX into Chartworks, not out of it); the scar
  is the *shape*: an ad hoc ingestion mode accretes a parallel, weaker
  security posture unless it is forced through the same governed
  source/access path as everything else (P7).
- **Environment-coupled bootstrap code left in the main app factory**:
  `_ensure_databricks_api_key` / `_refresh_databricks_api_key` /
  `_databricks_api_key_refresh_loop` live directly in
  `src/<pkg>/api/app.py`, gated by a `lakebase.enabled` settings flag rather
  than living behind a deploy-target seam. Confirms the extensibility-seam
  discipline in `CLAUDE.md` §4.4 (interface + factory + driver, no
  platform-specific code outside its own driver) is solving a real, observed
  problem, not a hypothetical one.
- **Two independent config-naming regimes for the same value**
  (`_FLAT_ENVIRONMENT_MAP`'s ~120-entry legacy-flat-name ↔ nested-name
  mapping): keeps old deployments working but means every new setting is
  either "flat legacy name" or "nested name" depending on when it was added,
  with no single documented convention going forward — worth deciding once,
  explicitly, for Chartworks' own config rather than growing the same map by
  accretion.

## Open questions for the RFC

1. **P1's concrete access primitive**: the generalistic predecessor's
   `access_resolver.py` (role-threshold ∪ per-user ACL grants, tenant-owned-
   pack intersection, service-account self-sufficiency, explicit deny
   sentinel vs. admin-bypass sentinel) is a working, already-debugged
   implementation of almost exactly what P1/P3 describe as a principle. Does
   the RFC adopt this shape directly (data-source/dataset grants + role
   threshold), or something narrower?
2. **How does the upload path enter the pipeline?** CSV/XLSX/Parquet upload
   is confirmed V1 scope (D-013). Both predecessors kept re-deriving a
   parallel weak-auth "Spreadsheet Mode" — the RFC should absorb uploads
   on-model (through the same engineering → semantic-model → NLQ path and the
   same access primitive) rather than as a separate mode with its own
   identity and execution route.
3. **Topic lifecycle stages** (draft/review/promoted/deprecated + audit
   trail + export/import + sharing/visibility) — brief 05 owns the full
   client-vs-fork diff, but at the architecture level: does Chartworks adopt
   a versioned-with-audit-trail lifecycle for its semantic model as a
   binding property, or leave it to a phase plan?
4. **Worker cardinality**: does Chartworks want N maintenance workers (one per
   background concern, as both predecessors converged on), or a single
   generic job-runner with typed job handlers registered into it? The
   predecessors' 13-worker proliferation argues for the latter under P7.
5. **Metrics export**: neither predecessor actually exports its counters
   anywhere. Chartworks' `internal/telemetry` (per `CLAUDE.md` §8) should be
   designed so "the counters exist" and "the counters are observable" cannot
   silently diverge the way they did here.
6. **Bilingual NLP preprocessing** (English + Spanish spaCy models) appeared
   in the client predecessor's dependency set — is multi-language NL input
   in scope for Chartworks, or was that client-specific and out of scope for
   the generalistic product frame?
7. **Chart catalog scope**: the predecessors' ~13-kind chart catalog
   (bar/column/line/area/pie/donut/scatter/heatmap/treemap/grouped-and-
   stacked variants/KPI card/table) is a concrete starting inventory for
   Chartworks' own "charts" stage — should the RFC adopt this list as the V1
   catalog, or trim it?
