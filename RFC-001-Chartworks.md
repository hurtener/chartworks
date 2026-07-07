# RFC-001 — Chartworks

> **Status:** accepted · v1.0 · 2026-07-06
> **Supersedes:** the consumer request (`00_CHARTWORKS_CONSUMER-REQUEST.md`) and the
> bootstrap artifact (`00_KICKSTART-PROMPT.md`) as the design source of truth. The
> request doc remains the authoritative record of *what the consumers asked for*;
> departures are named here and logged in `docs/decisions.md`.
> **Inputs absorbed:** the kickoff interview · decisions D-001…D-018 · research
> briefs 01–13 (`docs/research/INDEX.md`) · the consumer request §§0–12.
> **Priority chain:** this RFC > phase plans (`docs/plans/`) > `CLAUDE.md`/`AGENTS.md`
> > the consumer request > research briefs > code comments.

---

## 1. Product definition

Chartworks is a Go-native **structured-data analytics** service — the family's
**Explorer seat**. It **connects** customer data sources (warehouses and structured
uploads), **engineers** them (profiling, quality checks, transformations,
materializations, lineage, refresh), **models** them semantically (versioned,
governed *topics*: measures, dimensions, KPIs, join graphs, business context),
answers **natural-language questions** through that model as **validated, strictly
read-only SQL** — generated either by Chartworks' own gateway-driven generator or by
the *caller's* agent via a published context contract (D-014) — and returns
normalized results with a declarative **chart spec**.

It is the sixth product in the family (Portico / Harbor / Dockyard / Stowage /
Soundings / Chartworks) and ships **one binary** — `chartworks` — presenting a
**dual surface**:

- **MCP tools** for agents (consumed through Harbor's MCP southbound driver;
  streamable-HTTP + bearer preferred, stdio and in-process supported).
- **HTTP API** for Pengui's Console and standalone API callers.

Same sources, same model, same auth, same access, same core logic — two thin entry
points (P7). It runs **standalone** (self-issued API-key tokens — a sellable
analytics product) and as an **ecosystem citizen** (validating Pengui-issued JWTs).
It is **swappable**: the Explorer contract is the MCP tool set (§11.1) + the HTTP
surface (§11.2) + JWT validation (§4) + the grant model (§5). Anything implementing
those is a drop-in.

### 1.1 Scope boundary

Chartworks owns: data-source connections and **credential custody** (encrypted at
rest, §6.3); structured uploads (CSV/XLSX/Parquet — through the same governed path
as warehouses, §7.4); profiling and quality checks; transformations and
**materializations** (the only write path, §7.6); dataset versioning, lineage,
freshness; the semantic model and its lifecycle (§8); NLQ routing, context
assembly, SQL generation (both modes), validation, read-only execution; chart-spec
production; its own job/schedule machinery; access *enforcement* on its own
resources.

Chartworks does **not** own: identity/sharing *policy* (Pengui's); answer prose or
agent orchestration (Harbor/Pengui — Chartworks returns data + evidence, never
essays); memory (Stowage's); unstructured documents (Soundings' — a scanned PDF of
a table is Soundings' problem; a CSV/XLSX/Parquet file or warehouse table is
Chartworks'); chart *rendering* (V1 emits the declarative spec only, D-013/D-026);
the document-vs-data routing decision (upstream, Pengui's).

### 1.2 Binding properties

P2–P7 are binding as stated in `CLAUDE.md` §1. This RFC concretizes **P1** into
three named sub-properties:

- **P1a — Deny-by-default grants, computed in the query path.** Access to a data
  source, dataset, or topic requires an explicit grant (§5). The caller's resolved
  effective-access set is intersected **inside** every store query and every
  customer-warehouse query. Empty set ⇒ typed denial, no query issued.
- **P1b — SQL-safety.** Generated or submitted SQL is untrusted regardless of
  origin (internal generator or BYO agent — request §1.8). It executes only after
  the **layered validation** of §9.5 (tokenizer screens; client-side AST
  validation via the parser seam where a driver covers the dialect; engine-side
  dry-run/EXPLAIN as the dialect-true validator with referenced-table
  allowlisting against *topic pack ∩ caller grants* on every engine) and only
  through the read-only execution layer of §9.6, whose **primary read-only
  guarantee is the SELECT-only credential/session** the engine itself enforces
  (D-038), plus server-side timeouts and cursor-level row caps. Validation
  failure is a typed error — never a skip-to-execute.
- **P1c — The write split, and the write boundary.** The NLQ path is
  structurally read-only: the `exec` seam exposes no write operation. Warehouse
  writes exist only in the engineering stage's materialization path (§7.6) — a
  **distinct interface** with declared destinations, its own scoped grants, and
  full audit. No flag selects between read and write on a shared entry point
  (D-017, brief 04). And the write path itself is bounded (D-040): it writes
  **only into Chartworks-managed schemas** — client baseline tables/views are
  read-only forever, inputs only; the medallion is Chartworks tables/views
  exclusively. No autonomy level (§7.7) can relax this.

---

## 2. Vocabulary & domain model

The wire/UI nouns (request §2, binding under P6):

| Term | Meaning |
|---|---|
| **data source** | A customer warehouse connection or an upload workspace (§7.4). Tenant-scoped, credentialed, testable. |
| **dataset** | A governed, queryable, table-shaped artifact: a registered source table, an uploaded file's table, or a materialization. Carries schema, profile, freshness, lineage, version, grants. |
| **topic** | The semantic-model unit grounding NLQ: measures, dimensions, derived KPIs, join graph, business context, governed rules. Versioned; lifecycle §8.2. |
| **topic pack** | A topic version's full payload (authoring view). |
| **capability contract** | The compact, prompt-safe projection of a published topic pack — what routing and generation actually see (§8.3). |
| **context bundle** | The *published, versioned* projection handed to a BYO agent (§9.4). |
| **question / plan / run** | The NL question; *plan* = route+generate+validate without executing; *run* = plan then execute. |
| **pipeline** | A data-engineering definition (steps + checks + destination + schedule) producing datasets. |
| **materialization** | A pipeline write into a declared destination — the only write Chartworks ever performs against customer infrastructure. |
| **grant** | An explicit per-principal permission at one grain: source, topic, or dataset (§5). |
| **managed schema** | A Chartworks-created namespace inside a customer warehouse (default prefix `chartworks_`) — the only place materializations may write (D-040). |
| **baseline table** | Any table/view Chartworks did not create — read-only forever, inputs only (D-040). |
| **proposal** | The atomic, reviewable, revertible changeset an L2/L3 agent produces: pipelines + datasets + topic deltas + schedules (§7.7, D-039). |
| **decision record** | The stored reasoning trail attached to every agent-proposed object: goal, matched-vs-built, alternatives, evidence, provenance (D-039). |
| **autonomy policy** | A tenant's declaration of what may auto-publish at L3; outside it, degrade to review — loudly (D-039). |
| **principal** | `user:<id>` · `agent:<id>` · `svc:<name>` · `key:<id>`. |
| **session** | A conversation scope for refinement; part of the isolation triple. |
| **freshness** | Dataset recency status: `fresh` / `stale` / `very_stale` / `unknown` (age-bucketed; brief 09). |

**Forbidden on any wire/UI/error/log surface** (the P6 list, enforced by
drift-audit): `repair` · `broken` · `enhance`/`enhancement` (as user-facing verbs)
· `index`/`reindex` · `embedding` · `vector` · `shard` · `namespace` · `sync` ·
`wiring` · `collection` · `cache` · `worker` · `job` (prefer the operation's
domain name: "profiling", "publishing", "refreshing"). The predecessors' template
`repair` action is renamed: the operation is **`revalidate`** (check a topic/
example against its sources) and **`re-check source`** (refresh schema after
source drift). A failed source is "unavailable", never "broken".

---

## 3. Architecture

### 3.1 The pipeline

```
 sources ──▶ engineering ──▶ semantics ──▶ nlq ──▶ exec ──▶ charts
 (connect,   (profile,        (topics,      (route,  (validate  (spec
  uploads,    quality,         lifecycle,    context, + read-    only)
  creds)      transforms,      context       generate only
              materialize,     engineering)  | BYO)   execute)
              lineage)
```

One shared core stack, constructed once in `cmd/chartworks`; every surface (HTTP,
MCP, SDK in-process, CLI) is a thin caller (P7 — enforced structurally by an
architecture test, not convention).

### 3.2 Packages (settles §3 of CLAUDE.md; supersedes its `TBD-by-RFC` marks)

| Package | Owns |
|---|---|
| `internal/config` | Typed YAML config, `env:` indirection, fail-loud validation |
| `internal/telemetry` | slog, Prometheus registry, per-decision counters, audit emitter, OTel adapter (off by default) |
| `internal/auth` | JWT validation (asymmetric-only), self-issue + external issuer, JWKS fail-closed, API keys |
| `internal/identity` | The frozen envelope: `(tenant, principal, session, scopes, effective grants)` |
| `internal/access` | The grant model + the one access resolver (§5) + scope-debug |
| `internal/store` | Chartworks' own state — seam + `postgres` driver (pgx/v5), migrations, conformance suite |
| `internal/vindex` | Vector search seam + `pgvector` driver (routing facets only; D-029) |
| `internal/gateway` | The intelligence seam — `bifrost` + `mock` drivers (P5) |
| `internal/sources` | Data-source adapter seam + drivers (§6), connections registry, credential custody |
| `internal/engineering` | Profiling, quality checks, schema inference, pipelines, materializations, lineage, freshness |
| `internal/semantics` | Topic packs, lifecycle, facet building, capability contracts, governed rules |
| `internal/nlq` | Span hints, retrieval, routing, context assembly (both modes), generation, clarification, feedback/learning |
| `internal/exec` | SQL validation (three-stage) + read-only execution + result shaping |
| `internal/charts` | Deterministic chart-kind selection + the declarative chart-spec contract |
| `internal/jobs` | The one generic leased job queue + typed handlers + schedule dispatcher (D-025) |
| `internal/api` | HTTP surface |
| `internal/mcpserver` | MCP surface (mcp-go, D-019) |
| `sdk/chartworks` | Public Go client (HTTP + in-process) |
| `eval/` | Golden suites, red-team suite, accuracy harness (`chartworks eval`) |

### 3.3 Runtime shape

One process: HTTP server + MCP server + the job runner's worker pool (configurable
concurrency) + the schedule dispatcher. **One generic leased job queue** (Postgres
`FOR UPDATE SKIP LOCKED`, lease + heartbeat + reclaim) with typed handlers —
profiling, topic generation, publishing reindex, pipeline runs, refresh — replaces
the predecessors' 13 bespoke worker classes (brief 01 scar; D-025). Schedules
(cron/interval, §7.8) dispatch into the same queue. Expensive LLM work never sits
on a query's hot path (brief 03): enhancement/profiling are async jobs; routing
reads pre-built state.

Boot is fail-loud and consistent: config validation, store reachability,
migrations check, JWKS fetch (external mode), and pinned embedding-model/dims
validation (§13) all gate readiness. Data-source reachability is *reported*, never
a boot gate (§15).

---

## 4. Identity & auth

### 4.1 Two issuers, one validation path

`self_issue` and `external_issuer`, selectable and combinable from one binary
(D-006). Validation is identical for both surfaces and both modes: asymmetric only
(RS/ES 256|384|512) — `HS*`/`none` rejected **at the parser** before claims are
read (the predecessors were HS256-only with a hand-rolled decoder; brief 04);
`iss` pinned; `exp` + skew enforced; `aud` mandatory, per-instance, and
**per-surface** — `aud: <instance>.http` and `aud: <instance>.mcp` are distinct so
a Console token can never be replayed as an agent token (§7 of CLAUDE.md). JWKS is
cached and fails **closed** past `jwks_max_stale` (typed 401 + not-ready).

### 4.2 Claims and the envelope

Chartworks reads: `tenant` (mandatory), `sub` (prefixed principal), `aud`,
`scopes` (capability scopes, §5.3), optional `session`. These are read **once**
into the frozen `identity.Envelope`; the access resolver (§5.2) then attaches the
caller's effective grants. The envelope is immutable after construction
(API-enforced) and is the only identity object any downstream code sees. No `X-*`
header is ever consulted for identity, access, or tenant (the predecessors' triple
header-trust scar, brief 04).

### 4.3 The mint boundary is cryptographic

Self-issue mints tokens **only** from an API-key exchange: key presented →
constant-time hash compare → short-lived JWT with the key's tenant/principal/
scopes. There is **no** header-exchange endpoint, no first-caller-becomes-admin
bootstrap (the predecessors' highest-severity scar, brief 04): initial admin
provisioning is an operator CLI action (`chartworks admin bootstrap`), local-only,
against the store. A missing signing key/secret is a refused boot — never a
generated ephemeral one (P4; the predecessors silently generated one per process).

### 4.4 No local user management (D-030)

Chartworks has no password login, signup, or invite machinery. Ecosystem users
arrive as Pengui-issued tokens; standalone callers are API keys. This deletes the
predecessors' entire users/passwords/invites surface and its attack area.

---

## 5. The access model (P1a concrete; settles D-015)

### 5.1 Grants: one representation, three grains

One `grants` relation (P7 — one access representation):

```
grant := (tenant_id, principal, grain ∈ {source, topic, dataset},
          resource_id, permission ∈ {read, query, manage}, granted_by, ts)
```

- **read** — see the resource's metadata (catalog listing, description, schema).
- **query** — have NLQ answered from it / execute against it.
- **manage** — mutate it (edit topics, run pipelines, rotate credentials —
  per-grain semantics pinned in the phase plan).

Tenant **roles** set defaults within the tenant boundary: `admin` (implicit
`manage` on all tenant resources — an explicit resolver sentinel, never a skipped
check), `member` (implicit `read` on catalog; everything else by explicit grant),
`viewer` (explicit grants only). **Agents are first-class principals**
(`agent:<id>`): a Harbor agent's grants are its own, never inherited from its
owner unless explicitly granted (request §2.3). The tenant predicate is always
intersected after grant resolution — a grant can never leak cross-tenant (fork's
resolver discipline, brief 01/05).

**Dataset-grain is what makes the DE stage governable**: a materialization is
grantable independently of the topic or pipeline that produced it (brief 04's
D-015 requirement 1).

### 5.2 The resolver

`internal/access` exposes **one** resolver: `Resolve(envelope) → EffectiveAccess`
— the caller's visible/queryable resource sets per grain, computed once per
request from role + explicit grants, deny-by-default (`no grant ⇒ absent`,
`role nil ⇒ deny`, admin bypass as an explicit sentinel). `EffectiveAccess` rides
the frozen envelope. Every store read/write method and every warehouse-query
construction takes non-optional scope parameters derived from it; a query method
without scope parameters is rejected in review (CLAUDE.md §6). An empty effective
set short-circuits: typed `access.none` result, metric incremented, no query
issued.

### 5.3 Capability scopes (token-carried)

Scopes gate *operation families*; grants gate *resources*. Both must pass. The
V1 scope set: `catalog.read` · `topic.read` · `topic.write` · `topic.publish` ·
`dataset.read` · `source.manage` · `pipeline.manage` · `pipeline.run` ·
`query.preflight` · `query.plan` · `query.execute` · `query.context` (BYO bundle)
· `query.submit` (BYO SQL) · `feedback.write` · `autonomy.propose` ·
`autonomy.apply` (D-041) · `admin`. Plan-vs-execute are distinct scopes
(predecessor keeper, brief 02); BYO context-vs-submit are distinct so a
context-reading agent can be denied execution; propose-vs-apply are distinct so
a goal-submitting caller can never self-approve.

### 5.4 Per-decision observability + scope-debug

Every allow/deny increments `access_decisions_total{grain,decision,reason}` and
emits a content-free structured event. The admin-only, read-only **scope-debug**
diagnostic (`GET /v1/admin/scope-debug?...`, CLI `chartworks admin scope-debug`)
replays a hypothetical `(principal, resource | question)` through the resolver —
and through routing eligibility for topics — reporting *which predicate* failed
(tenant mismatch / no grant at grain / scope missing / topic not published /
source unavailable). This is the sanctioned answer to "why can't X see Y"; it is
never a user-facing or self-service surface (CLAUDE.md §8).

### 5.5 Standing adversarial obligations

A **registry-driven cross-tenant probe suite** (client predecessor keeper, brief
04) is mechanical from Wave 1: every route/tool is enumerated from the router/
server registration tables themselves (not a hand-maintained list — closing the
predecessors' "manually maintained allowlist" gap), and each tenant-sensitive one
is probed cross-tenant; a bare 2xx fails CI. Plus: empty-access-set probe,
forged-header attempt, fetch-then-filter regression guard, and (once exec ships)
write/DDL-injection and schema-escape probes (CLAUDE.md §11).

---

## 6. Data sources & credentials

### 6.1 The adapter seam

`internal/sources` defines the data-source adapter interface (interface + factory
+ driver, §4.4): `Kind()`, `Dialect()`, `Capabilities()` (feature set:
CTE/LIMIT/temp-views/…, gated by `Supports(x)` — never type-switching on adapter),
`TestConnection(ctx)`, `DiscoverSchema(ctx, scope)`, `SampleValues(ctx, scope, …)`,
and the **read-only query entry point** `Query(ctx, ValidatedSQL, QueryOpts) →
QueryResult`. `ValidatedSQL` is an opaque type constructible **only** by the
validator (§9.5) — an adapter structurally cannot execute a raw string (closing
brief 02's "nothing requires prior validation" scar). Materialization writes live
on a **separate** interface implemented only by destination-capable drivers
(§7.6), never on the query interface (P1c).

**V1 driver set (D-032):** `postgres` (pgx — also serves upload workspaces,
§7.4), `mysql` (go-sql-driver/mysql), `sqlserver` (microsoft/go-mssqldb — the
fork shipped this driver; brief 05), `bigquery`, `snowflake`, `databricks` (each
on its official Go SQL driver/connector), plus `mock` (tests) and `null`
(fail-loud placeholder, fork keeper). All six real drivers are pure-Go — the
CGo-free posture holds (D-005). A generic `ansi` dialect sentinel covers
unknown-dialect handling (fork keeper, brief 05). Adding a driver is a driver
package + conformance run, never core surgery.

**Validation strategy per engine class (D-032):** the self-hostable engines —
Postgres, MySQL, SQL Server — run the full adapter conformance suite against
**dockerized instances loaded with public datasets** (Kaggle-class), locally and
at wave ends, so their read-only posture, capping, timeouts, and dialect fixtures
are validated without cloud credentials; the cloud warehouses (BigQuery,
Snowflake, Databricks) validate hermetically via recorded fixtures and fully via
the live gate (D-010).

### 6.2 The connections registry

Tenant-scoped `data_sources` rows: `kind`, display name, **non-secret**
`config_json` (host/port/database/user — never the secret), separately encrypted
`secret_ciphertext`, `status` lifecycle (`unverified → connected → unavailable`),
`last_tested_at`, `last_error` (content-free). The read/list shape has **no
secret field at all** — structurally impossible to leak on read (fork keeper,
brief 02). Connection tests run the adapter's `TestConnection` and update status;
Pengui reads status live (§15).

### 6.3 Credential custody (settles D-016)

Secrets are envelope-encrypted at rest in the store: AES-256-GCM data keys
wrapped by a **master key ring** supplied via config (`env:` indirection —
`CHARTWORKS_SOURCE_KEYS`, ordered list). Encrypt always uses the primary (first)
key; decrypt tries all ring keys, so rotation = prepend a new primary, no bulk
re-encryption (the fork's MultiFernet discipline, brief 02, in Go stdlib crypto).
Missing keys at boot with any stored secret present = refused boot. Credentials
are decrypted only at adapter construction, held in memory only, never logged,
never echoed into errors or results (§7 CLAUDE.md); credential *events*
(stored/rotated/test-failed) are audited content-free. External secret-manager
backends are a future driver behind this same custody seam, not V1.

---

## 7. The data-engineering stage (settles D-013's mechanism)

The stage that makes Chartworks more than its predecessors. Design synthesizes
brief 11's keepers (demand-driven scoping, canonical identity, whitelist
composition, contract-before-materialize), brief 09's profiling shapes, and brief
12's quality-check families — with **no external pipeline engine**: Bruin is
mined for ideas only (D-018 resolved → D-023; its SQL-safety and ingestion cores
require CGo/Rust/Python, colliding with D-005).

### 7.1 Registration & discovery

Connecting a source triggers (async, jobs queue) **schema discovery** via the
adapter: schemas/tables/columns land as candidate **datasets** with normalized,
dialect-agnostic column type classification (`TypeCategory`: numeric / temporal /
boolean / text / structured / binary / unknown — computed **once** at discovery,
fork keeper, brief 02). Registration is selective: an admin chooses which
schemas/tables become datasets (the allowlist starts here — nothing is queryable
by default, P1a).

### 7.2 Profiling & quality

A `profile` job produces the normalized **dataset profile**: per-column null% /
distinct-count bucket / min-max / sample values (sanitized, length-capped),
row count, date range, and a quality assessment over the six standard dimensions
(completeness, uniqueness, validity, consistency, integrity, timeliness — brief
12), plus **value families** for low-cardinality text dimensions (brief 09) and
a `freshness` classification (age-bucketed). Profiles are versioned artifacts in
the store; they feed topic generation (§8.1), the chart-spec's column metadata
(§10), and the Console's dataset inspector. Large-table safeguards: sampling
ceilings and a "large table — date-bound queries preferred" hint recorded as
dataset metadata (brief 09).

### 7.3 Transformations & pipelines

A **pipeline** is a declarative, versioned definition: ordered **SQL steps**
(dialect of the destination source), each with declared inputs (upstream
datasets) and one declared output, optional **quality checks** per step
(row-count bounds, non-null, uniqueness, referential integrity, freshness —
check families from §7.2), and a destination (§7.6). Steps are authored by hand
(Console/HTTP) or **LLM-assisted** through the gateway (schema-constrained
generation grounded in the profiles + canonical registry) — assisted authoring
produces a *draft* pipeline; nothing materializes without an explicit,
`pipeline.manage`-scoped publication (brief 11's contract-before-materialize
gate). Step SQL passes the same AST validation machinery as NLQ SQL, with a
write-shaped allowlist: exactly one statement, writing only to the declared
output, reading only from declared inputs (the whitelist-composition discipline,
brief 11).

A **canonical identity registry** (per tenant) maps business entities/terms to
their governing dataset + key columns — "resolve then compare, never compare
names" (brief 11) — shared by pipeline assistance and topic generation, stored as
part of the semantic layer (§8), not a second vocabulary (P7).

### 7.4 Uploads (settles brief 01/02's open question; D-024)

CSV/XLSX/Parquet uploads are **first-class data sources, not a parallel mode**.
Each tenant gets a managed **upload workspace**: a Chartworks-provisioned
Postgres database/schema (same cluster as the store or a designated one — but a
*separate database*, reached through the standard `postgres` **adapter** like any
customer warehouse; never through the `store` seam, preserving the D-004
boundary). An upload is parsed (header detection, type inference, cell
sanitization — the client predecessor's hardening, brief 02), loaded into a
workspace table, and registered as a dataset with origin `upload` — then
profiled, modeled, queried, and granted exactly like everything else. Same
identity path, same access primitive, same execution route (killing the
predecessors' weak-auth "Spreadsheet Mode" scar). No DuckDB — its Go driver
requires CGo (D-005 holds). Upload limits (bytes, rows, formats) are config.

### 7.5 Versioning, lineage, freshness

Every dataset carries: a monotonic version (bumped on schema change or
re-materialization), `lineage` (upstream dataset ids + producing pipeline id +
step), and `freshness` (§2). Lineage is recorded at materialization time from the
pipeline definition (declared, not inferred — V1 does not parse arbitrary SQL for
lineage). Schema drift detection: a re-discovery diff marks affected datasets and
flags dependent topics' source health (§8.2, stage 7).

### 7.6 Materializations — the governed write path (P1c concrete; D-036)

The **only** write Chartworks performs against customer infrastructure, executed
by **Bruin** (pinned v0.11.666, Apache-2.0) as a CLI subprocess behind the
**`PipelineRunner` seam** (interface + factory + driver; `bruin` is the V1
driver):

- Destinations are **declared** per pipeline and are always **Chartworks-managed
  schemas** (D-040): a (source, managed schema) pair — namespaces Chartworks
  creates (default prefix `chartworks_`, per-source configurable) — marked
  writable by the tenant admin + a `manage` grant on that source. An output
  resolving to any non-managed schema is rejected at definition validation AND
  at the render gate; where the engine supports it, the write credential is
  itself scoped to the managed schemas. Client baseline tables appear only as
  inputs. No declared destination ⇒ no write, ever. Chartworks renders its
  declarative pipeline definition to Bruin's format at run time; connections are
  injected via env-var interpolation or a custody-rendered tmpfs config —
  plaintext secrets never persist to disk.
- **The NLQ adapters have no write capability at all** — the strongest P1c
  shape: reads live in `internal/sources` adapters; writes live behind
  `PipelineRunner` in a separate executor process. No shared entry point exists
  to flag-switch.
- Strategies (inherited from Bruin, per-engine support recorded per driver):
  create+replace, delete+insert, truncate+insert, append, **merge/incremental**,
  time_interval, scd2. Quality checks (unique/not_null/accepted_values/
  pattern/min/max/custom) run blocking by default.
- **V1 hard rules (D-036):** SQL-only assets — never Python/R assets, never
  `ingestr` ingestion (FSL license + Python runtime); Bruin telemetry disabled
  (confirming the mechanism is a phase-13 implementation blocker); `bruin
  validate -o json` gates rendering, `bruin lineage -o json` feeds lineage.
- Every materialization run is audited by Chartworks (content-free: pipeline,
  step, destination, row count, duration, outcome) and stamped into dataset
  lineage + freshness. Failed quality checks **fail the run loudly** (typed
  error, metric, run status) — never a silent partial publish (P4). Bruin exit
  codes and per-asset results map to typed run outcomes at the seam.

### 7.7 The autonomy ladder (D-039)

The DE stage's agentic posture is an explicit ladder; every level shares the same
gates, the same D-040 write boundary, and the same audit spine:

- **L0 manual / L1 assisted** — human-authored or agent-drafted pipelines,
  human-published (the §7.3 flow).
- **L2 goal-driven proposal (the V1 target).** A business goal enters (HTTP,
  Console) ⇒ the demand-driven engine plans **blind** (no catalog access at plan
  time, so gaps are detectable rather than silently trimmed — brief 11), then
  matches top-down against existing datasets/topics via the canonical registry
  (retrieve → verify → confirm; resolve-then-compare, never name comparison) and
  builds bottom-up only what's missing ⇒ one atomic, reviewable **proposal**:
  pipelines + datasets + topic deltas + schedules, each choice carrying a
  **decision record** (goal, matched-vs-built, alternatives rejected, evidence,
  model/agent provenance, token cost). Approval applies the changeset through
  the ordinary publication gates; rejection and edit-then-approve are
  first-class; every applied proposal is revertible as a unit.
- **L3 policy-scoped auto-apply (designed in V1, per-tenant opt-in).** A tenant
  `autonomy_policy` declares what may publish without a human gate: permitted
  risk classes (non-destructive strategies), destination scopes (managed schemas
  only — non-negotiable), required quality gates, cost/row ceilings, schedule
  bounds. Outside policy ⇒ degrade to L2 review, loudly (P4). Auto-applied
  changes remain decision-recorded, post-hoc reviewable, and revertible.
  **Autonomous topic publication is barred at every level** (D-041): topic
  deltas stop at draft/review — meaning changes always get a human. Revert
  semantics per D-041 (managed-schema artifacts drop by default; atomic per
  proposal).
- **The evolution loop.** Schema drift, failed freshness, or failed quality
  checks generate *proposed amendments* through the same proposal machinery —
  the medallion evolves under audit instead of decaying behind flags. Goal
  history + decision records make "why does this table exist" a query, not
  archaeology.

Proposal/decision-record/policy surfaces are management-plane: HTTP + SDK only
(agents do not administer tenants, §11.1). Owned by phase 26.

### 7.8 Refresh & scheduling

Schedules (cron or interval — condition/event triggers are post-V1, D-013 scope
note) attach to a pipeline or a saved query and dispatch runs into the jobs
queue. Missed-window and overlap policy: skip-if-running, log + metric. The
dispatcher is the fork/client shared shape (brief 02) on the single queue.

---

## 8. The semantic model (`internal/semantics`)

### 8.1 Topics and packs

A **topic** binds a set of datasets into an NLQ-answerable unit: enhanced tables
(columns, keys, measures, dimensions), derived KPIs, a join graph, semantic
context (synonyms, business definitions, example questions, canonical entities
from §7.3's registry), query patterns, and governed rules (§8.4). Topic
*generation* is an async job: schema + profiles → gateway-driven enhancement
(schema-constrained: measure/dimension/KPI synthesis, definitions, example
questions, join inference) → a DRAFT version for human review. Manual authoring
edits every entity through the same versioned mutation path.

### 8.2 The lifecycle (brief 05's unified state machine — binding)

```
create ─▶ DRAFT ─▶ REVIEW ─▶ PUBLISHED ─▶ DEPRECATED
           │ update-in-place    ▲   │ rollback (prior published re-activates)
           │ (diff + audit)     │   └ supersede-on-next-publish
           └ discard (delete)   └ publish = atomic active-version swap
topic-level: ACTIVE ⇄ ARCHIVED (orthogonal)
```

Binding rules (brief 05 §"recommended", carried verbatim):

1. Exactly one PUBLISHED-active version per topic; publish is an atomic
   `active_version_id` swap capturing the prior id, and **carries its reindex**.
2. The active version is never mutated in place and never discarded; edits
   branch a DRAFT.
3. Every transition is a typed audit row (actor + change counts). No silent
   stage change (P4).
4. **Source health is a first-class lifecycle input**: datasets carry
   `is_available`/`health_status`; unavailable tables are excluded from routing
   and generation (never broken SQL), and a **re-check source** operation
   refreshes schema and clears resolved issues (the top client-predecessor carry,
   brief 05). Renaming/replacing a source table rewrites every pack reference
   (measures/dims/KPIs/joins) — the client's `update_table`, carried.
5. Transition authority = the one access resolver: DRAFT edits need
   `topic.write`+topic `manage`; REVIEW→PUBLISHED needs `topic.publish`;
   archive needs `manage`. The transition-authority matrix is part of this RFC
   (closing brief 05 Q8).
6. Invalidation is part of the transition: publish/rollback enqueue facet
   reindex; archive clears routing caches and removes facets; tenant delete
   cascades facet cleanup (fork keeper). The invalidation matrix in brief 05
   §stage-10 is binding.
7. Export/import with id sanitization ships (client carry) — the sanitizer is
   the P6 reference implementation for externally visible artifacts.

### 8.3 Context engineering (the crown jewel — kept and enhanced)

The mechanism brief 03 documents, carried with its scars fixed:

- **Facet decomposition at publish time**: packs decompose into typed facets —
  measure / dimension / derived-KPI / query-pattern / example — embedded into
  `vindex` under `(tenant, topic, version)` scoping. Retrieval returns governed
  semantic units, never raw schema.
- **Two-layer compression, one owner**: the capability contract applies hard
  structural caps (top-N measures/dimensions/joins/patterns; truncated
  definitions — owned by `semantics`, computed at publish), then a
  complexity-tier token budget (low/medium/high, bands set by routing
  confidence) prunes further at query time. **All runtime pruning lives in one
  component** (`nlq.ContextAssembler`, per the §3.2 placement) — not split
  across packer and generator (the predecessors' duplication trap, brief 03
  §2.2/Q3).
- **One budget currency**: a real tokenizer-backed estimator for *every* context
  lane (evidence, rules, examples) — no char/4 second currency (brief 03 Q5).
- **Never mutate source evidence**: pruning operates on per-call copies; the
  unpruned contract stays in result metadata; every call logs
  original/pruned/reduction (brief 03 keeper).
- **Governed-rules lane**: rules get their own independent token budget; dropped
  rules and contradictions are named output fields, never silent (client
  keeper).
- **Provenance on everything**: filters and priors carry `source`+`confidence`;
  uncertain provenance is stated in the context itself.
- **One confidence primitive**: routing confidence is *the* calibrated quantity;
  example weight is a separate, documented prior that feeds it — their
  relationship is pinned in the nlq phase plan, not left as two unreconciled
  scores (brief 03 Q2).

### 8.4 Governed rules & clarification (V1-scoped; D-027)

The client predecessor's business-rules and underspecification layers are the
largest capability gap vs the fork (brief 03/05) and are **in V1 scope, scoped
down**: rule authoring (tenant/topic/measure-scoped, categorized, prioritized,
structurally validated), lifecycle `proposed → active → retired`, and budgeted
injection into the context lane. Shadow evaluation and historical replay are a
later wave (the machinery is additive). **Proactive clarification**: topic-scoped
ambiguity patterns (generated at enhancement, manually editable) run before
generation and produce named clarification slots (brief 03 §7) — `preflight` and
`plan` surface them; sensitive-literal marking is enforced (unmarked ⇒ rejected,
fail-loud).

---

## 9. NLQ-to-SQL (`internal/nlq` + `internal/exec`)

### 9.1 Routing

Router-first (ADR-001 keeper): lightweight span hints (lexicon/heuristic Go —
**no in-process NLP library**, no bilingual model downloads; D-028) → facet
retrieval from `vindex` (typed candidates, k-per-type, tenant-scoped cache) →
evidence aggregation → `routing_decision ∈ {single_topic, multi_topic,
clarify, no_route}` + calibrated confidence. Only **published, health-eligible,
grant-visible** topic versions are routable (fork's governance filter ∩ §5
grants). The router is a shared singleton, immutable after construction;
per-request state rides the context (CLAUDE.md §5).

### 9.2 Context assembly

`ContextAssembler` (§8.3) produces the **query context**: business context
(topic, join strategy, confidence, time window, dialect + complexity directive),
structured evidence (the pruned capability contract), governed rules lane,
example lane (§9.3), and clarification slots if any. One stable envelope with a
`strategy` discriminator (brief 03 §2.4 keeper).

### 9.3 Internal generation (mode a)

Gateway-driven, schema-constrained (P5 — never free-text JSON). Example
precedence is **one explicit, unit-tested resolution function**:
`edit_base > hints > examples > default` (brief 03 §2.5) — learned examples
(§9.8) enter as edit-base/hints only above weight+similarity thresholds,
otherwise as ranked few-shot. Dialect targeted per adapter. Bounded repair: on
validation failure, ≤1 gateway fix attempt, revalidate, then typed failure
(P4; the predecessors' cap-2 discipline tightened).

### 9.4 BYO-agent mode (mode b — D-014/D-022)

The Teramot-class flow, built on §8.3's machinery:

- **`get_query_context`** (scope `query.context`) returns the **context bundle**:
  a *published, versioned* (`bundle_version`) schema — routing result, the
  capability contract slice, governed rules **restated as explicit constraints**
  (an external agent can't be assumed to know them; brief 03 Q10c), dialect +
  SQL requirements (single statement, SELECT-family only, allowlisted
  tables/columns enumerated), clarification slots, and optional
  provenance-labeled prior SQL (known-good examples as *guidance*, not
  machinery; Q10d). Every field exists because it helps an *arbitrary* agent
  produce compliant SQL — the stricter bar of Q10a.
- **`submit_sql`** (scope `query.submit`) accepts `(bundle_ref, sql)` and runs
  the *identical* §9.5 validation + §9.6 execution as mode (a) — the validator
  assumes an adversarial submitter (Q10b). Provenance (`generator: internal |
  byo`) is recorded on the query row and in the result envelope.
- One validation/execution core serves both modes (P7); mode (b) can never
  bypass a check mode (a) runs — enforced by construction (`ValidatedSQL` is the
  only executable type) and by a standing parity test.

### 9.5 Validation (P1b concrete; layered — D-038)

Validation is layered; no layer's absence silently widens access (P4):

1. **Tokenizer screens (every dialect)** — byte/length caps, encoding sanity,
   dialect-aware single-statement enforcement, comment/quote hygiene. **A
   pre-parse rule must not duplicate parser-level judgment** (the CTE-blocking
   regression, brief 03 §4 — a standing review rule plus a golden CTE fixture
   guard it).
2. **Client-side AST validation via the parser seam** (interface + factory +
   driver): driver `crdb` = `cockroachdb-parser` v0.25.2 (postgres-family —
   postgres sources and all upload workspaces); driver `sqlglotgo` =
   `jonathan-fulton/sqlglot-go` v0.4.0 (all six dialects; **per-dialect
   adoption gated** on independently reproducing its conformance against the
   phase-09 fixture corpus — D-038). Where a driver covers the dialect: the
   allowlist walk — top-level node ∈ SELECT-family only; blocked node classes
   (`INSERT`/`UPDATE`/`DELETE`/`MERGE`/`CREATE`/`ALTER`/`DROP`/`TRUNCATE`/
   `CALL`/`COPY`/dialect escapes) rejected **anywhere in the tree**; every
   table/**column** reference ∈ (topic pack schema ∩ caller's dataset grants) —
   intersected, not conflated (brief 04's D-017 requirement 2); join
   reachability against the declared join graph; CTE-local names resolved
   scope-aware (warning-level on shadowing). A dialect without a proven driver
   skips this layer — it never fakes it.
3. **Engine-side dry-run/EXPLAIN (every engine, pre-execution)** — the candidate
   SQL is dry-run (BigQuery) or EXPLAIN'd (Postgres/MySQL/Snowflake/Databricks;
   SQL Server showplan) under the read-only credential: the engine's own parser
   is the dialect-truth syntax check, and the **referenced-table set is checked
   against topic ∩ grants** before the real run — table-grain allowlisting
   guaranteed on every engine regardless of layer 2 coverage.

Typed error codes (`statement.blocked`, `table.not_granted`,
`table.not_in_topic`, `column.unknown`, `join.unreachable`,
`parse.unsupported`, …) — the vocabulary is normative for both surfaces. No
regex "injection heuristics" presented as controls (brief 02/04): the
allowlist + statement blocking *are* the injection guardrail; identifiers are
never string-assembled from model output. **Generation targets the source's
native dialect** — no ANSI-subset constraint (D-038).

### 9.6 Execution (read-only, defense-in-depth)

`exec.Query(ctx, ValidatedSQL, opts)` — the only execution entry point on the
read path:

- **Read-only credentials/sessions are the PRIMARY read-only guarantee
  (D-038)**: each connection is provisioned SELECT-only where the engine
  supports it, reinforced by read-only transaction/session mode (Postgres
  `BEGIN READ ONLY`; engine-equivalent elsewhere; documented per driver); the
  connection test asserts the posture by attempting a write and expecting
  engine denial. The validator adds depth — it is never the sole guarantee
  (brief 02's headline scar).
- **Server-side statement timeout** per adapter (e.g. Postgres
  `statement_timeout`) *plus* the context deadline — a cancelled query stops
  consuming warehouse compute (brief 02 Q2). Defaults in §14.
- **Cursor-level row caps** — never LIMIT-by-string-wrapping (it silently drops
  `ORDER BY`; the fork's documented lesson is a standing exec rule). Caps clamp
  to the configured ceiling regardless of caller input.
- Result shaping: adapter-agnostic `QueryResult` → ordered `ResultPreview`
  (columns + rows, capped) + column metadata for §10. No result pagination in
  V1; no cross-request result cache (an explicit non-goal — §19; idempotency
  keys cover retries).
- **Execution-time self-repair is V1 but bounded and gated**: on a warehouse
  execution error, ≤1 gateway repair → revalidate → re-execute → typed terminal
  failure. It exists only behind `query.execute` and only after P1b shipped
  (CLAUDE.md's hard gate: no phase executes generated SQL before P1b lands).
- `/run` supports caller idempotency keys scoped `(operation, tenant, principal,
  key)` (predecessor keeper).

### 9.7 Answering the question

`run_query` returns the normalized envelope: routing evidence (topics,
confidence, decision), assumptions + ambiguity assessment, the SQL (with
provenance + validation report), the result preview, the chart spec (§10), and
typed warnings. Ids are resolved to display labels before returning — never an
internal id as a user-facing label. Prose composition is the caller's job (§1.1).

### 9.8 Feedback & learning

`submit_feedback` records a verdict (optionally a corrected SQL). A
learn-positive job (DB-first — durable weights in the store, cache read-through;
ADR-005 keeper) re-embeds the example, dedupes paraphrases, and recomputes its
routing weight (Wilson-score + recency + evidence-growth blend). Example
lifecycle: `candidate → active → retired`, audited. Positive corrections may
*propose* governed rules (into §8.4's lifecycle) — never auto-activate.
Golden-eval cases can be seeded directly from positive feedback (§16).

---

## 10. Charts (`internal/charts`; D-026)

V1 is headless: emit the **declarative** contract, never render, never
pre-inflate a charting library's options (brief 06's recommendation, verbatim):

- `ColumnMetadata[]` — name, display name, source role, data type, semantic
  type, temporal grain, aggregation, format hint, query role, bucketed
  cardinality, and a `source_ref` back to the semantic model (what makes future
  rebind-on-drift possible).
- `ChartRecipe` — `kind` (the predecessors' proven 14-kind enum), `title`,
  `column_bindings` (slot → columns), `formatting`, `score`, `rationale`.
- `ResultPresentation` — column metadata + primary + alternatives +
  `generator_version` + `picker` provenance (`rules | rules_fallback` in V1).

Selection is the deterministic rules engine (slot binding + weighted suitability
+ adaptive alternatives — pure Go, no model call, no I/O; ported structurally
from brief 06 §1). The LLM ranker is post-V1 and, when added, goes through the
gateway schema-constrained (the predecessors' free-text JSON parse is the named
anti-pattern). Zero-viable-candidates degrades to a `table` recipe — a rendering
fallback, never an access/validation fallback (P4 distinction, brief 09 §8).

---

## 11. Surfaces

### 11.1 MCP tool surface (settles D-011 → D-019)

**Library: `mark3labs/mcp-go`, pinned at current upstream (v0.55.x), for V1** —
the Soundings-proven path. Deciding factor (brief 13): its global
`WithToolHandlerMiddleware` seam makes the scope/grant gate **structural** —
every tool passes it, a missed wrap is impossible — which is load-bearing for
P1a; Dockyard applies per-tool wrapping by convention. Chartworks' tool contracts
are Dockyard-portable by discipline (Go structs as source of truth, one
registration list, one `internal/mcpserver` package); **Dockyard is re-evaluated
at the Wave-5 boundary** (by then its runtime may have a global middleware seam
or a second production adopter). Panic recovery + typed error results are
middleware-installed as Soundings does. This supersedes the question D-011
re-opened.

**The V1 tool set (11 tools):**

| Tier | Tool | Scope | Effect |
|---|---|---|---|
| Discover | `list_topics` | `catalog.read` | read-only |
| | `describe_topic` | `topic.read` | read-only |
| | `list_datasets` | `catalog.read` | read-only |
| | `describe_dataset` | `dataset.read` | read-only (schema, profile, freshness, lineage) |
| Ask | `preflight_question` | `query.preflight` | read-only (routability + clarification slots; no SQL) |
| | `plan_query` | `query.plan` | read-only (SQL + validation report; **no execution**) |
| | `run_query` | `query.execute` | read-only execution |
| | `refine_query` | `query.execute` | read-only execution (session-scoped) |
| BYO | `get_query_context` | `query.context` | read-only (the context bundle) |
| | `submit_sql` | `query.submit` | read-only execution of externally-generated SQL |
| Feedback | `submit_feedback` | `feedback.write` | writes feedback state only |

Every tool carries an explicit read-only/write annotation and a **fail-closed
allowlist test**: a registered tool absent from the annotation table fails CI
(brief 09's pattern). Tool results are typed (§9.7's envelope; §9.5's error
codes); a raised stack trace across the boundary is a build-failing bug.
Management-plane operations (sources, pipelines, grants, lifecycle transitions)
are **HTTP-only in V1** — agents ask questions; they don't administer tenants.

### 11.2 HTTP surface

All under `/v1`, both audiences validated identically, HTTP `aud` distinct from
MCP `aud`:

| Group | Endpoints (indicative) |
|---|---|
| Health | `GET /healthz`, `GET /readyz` (unauthenticated; §15) |
| Sources | CRUD `/sources`, `POST /sources/{id}:test`, credential update/rotate |
| Uploads | `POST /uploads` (multipart), status |
| Datasets | list/get `/datasets`, `GET /datasets/{id}/profile`, `POST /datasets/{id}:refresh-profile` |
| Pipelines | CRUD `/pipelines`, `POST /pipelines/{id}:run`, runs list/status; assisted-draft `POST /pipelines:draft` |
| Topics | CRUD + versions, entity editing, `:submit-review`, `:publish`, `:rollback`, `:archive`, `:recheck-source`, `:revalidate`, sharing/grants, `:export`, `POST /topics:import`, `POST /topics:generate` |
| Rules | CRUD `/topics/{id}/rules`, activate/retire |
| Query | `POST /query:preflight`, `:plan`, `:run`, `:refine`; `POST /query:context`, `POST /query:submit-sql`; `GET /queries` (history), saved queries CRUD |
| Sessions | list/get sessions + their queries |
| Feedback | `POST /feedback`, list |
| Schedules | CRUD, pause/resume, runs |
| Grants | CRUD `/grants` (three grains), principals listing |
| Admin | tenants (self-issue mode), API keys, scope-debug, audit query, `GET /metrics` (operator-scoped) |

Transport hardening set explicitly (timeouts, body limits, Origin/Content-Type
checks — never SDK defaults). CSRF protections apply only if a cookie transport
is ever added (V1 is bearer-only; no cookie fallback — the predecessors' silent
cookie channel is dropped).

### 11.3 SDK, CLI, parity

`sdk/chartworks`: HTTP + in-process modes, same typed results. CLI
(stdlib `flag`, D-008): `chartworks serve | mcp | admin <bootstrap|scope-debug|
keys|erase> | eval | version`. **Parity tests are standing**: every Ask/BYO/
Discover capability is exercised through HTTP, MCP, and the SDK in one test
suite; a capability shipping on one surface only fails the wave gate (P7).

---

## 12. Store & schema inventory (budgeted)

The complete V1 inventory — a table/column beyond it requires an RFC amendment
first. `tenant_id TEXT NOT NULL CHECK (tenant_id <> '')` on **every** tenant
row (the fork's nullable regression is the named counterexample) and part of
every query predicate; forward-only migrations.

| Table | Purpose (key columns) |
|---|---|
| `tenants` | tenant_id, name, status, created_at |
| `api_keys` | tenant_id, key_id, key_hash, principal, scopes, created_at, revoked_at |
| `data_sources` | tenant_id, source_id, kind, name, config_json, secret_ciphertext, status, last_tested_at, last_error, writable_destinations JSONB, timestamps |
| `datasets` | tenant_id, dataset_id, source_id, origin (source_table\|upload\|materialized), locator, schema_json, version, availability/health, freshness, lineage_json, timestamps |
| `dataset_profiles` | dataset_id, version, profile_json, quality_json, profiled_at |
| `pipelines` | tenant_id, pipeline_id, name, version, definition_json, destination (source_id, schema), status, created_by, timestamps |
| `pipeline_runs` | run_id, pipeline_id, status, stats_json, error_json, started/finished_at |
| `topics` | tenant_id, topic_id, name, status (active\|archived), visibility, active_version_id, created_by, timestamps |
| `topic_versions` | version_id, topic_id, version_number, stage (draft\|review\|published\|deprecated), pack_json, created_by, timestamps |
| `topic_audit` | topic_id, version ids, action (typed), actor, change_counts_json, ts |
| `rules` | tenant_id, rule_id, topic_id, scope_json, category, definition_json, status, priority, timestamps |
| `grants` | tenant_id, principal, grain (source\|topic\|dataset), resource_id, permission, granted_by, ts — unique (tenant, principal, grain, resource) |
| `sessions` | tenant_id, session_id, principal, created_at, last_activity_at |
| `queries` | tenant_id, query_id, session_id?, principal, question, routing_json, mode (internal\|byo), bundle_version?, sql_text, validation_json, status, exec_stats_json, created_at |
| `saved_queries` | tenant_id, id, name, question, sql_text, topic_id, max_rows, created_by, timestamps |
| `examples` | tenant_id, example_id, topic_id, question, sql_text, weight, status (candidate\|active\|retired), provenance, timestamps |
| `feedback_events` | tenant_id, id, query_id, verdict, correction_sql?, created_by, ts |
| `facet_vectors` | tenant_id, topic_id, version_id, facet_kind, facet_id, embedding vector(D) — HNSW, cosine (`vindex`) |
| `jobs` | job_id, tenant_id, kind, status, priority, attempts, lease_owner, lease_expires_at, payload/result/error JSONB, timestamps |
| `schedules` | tenant_id, schedule_id, target (pipeline\|saved_query), target_id, trigger_json, status, timestamps |
| `schedule_runs` | schedule_id, run_id, outcome, ts |
| `proposals` | tenant_id, proposal_id, goal_text, status (draft\|proposed\|approved\|rejected\|applied\|reverted), changeset_json, created_by (principal/agent), reviewed_by, timestamps |
| `decision_records` | tenant_id, record_id, proposal_id, subject (pipeline\|dataset\|topic\|schedule), decision_json (matched-vs-built, alternatives, evidence refs, provenance, cost), ts |
| `autonomy_policies` | tenant_id, policy_id, source_id?, rules_json (risk classes, quality gates, ceilings, schedule bounds), status, updated_by, timestamps |
| `idempotency_cache` | tenant_id, operation, principal, client_key, response_hash, payload, expires_at |
| `audit_events` | ts, tenant_id, principal, action, resource_type/id, decision, request_id — content-free |
| `gateway_call_events` | ts, tenant_id, stage, model, tokens_in/out, cost, latency_ms |
| `schema_migrations` | forward-only |

28 tables — well under the predecessors' ~50-table sprawl (brief 02's ~50-table scar),
with query/SQL/topic identifiers stored once each (no cross-table duplication).
Uploaded file bytes live on disk/object storage under a tenant-scoped path
(config), not in the store; workspace tables live in the upload-workspace
database (§7.4), which is customer-data territory, not Store territory (D-004).

---

## 13. Gateway seam (P5; D-003)

Drivers: `bifrost` (wrapping `github.com/maximhq/bifrost/core`) + `mock`.
Model **roles**, each independently configurable (model id, temperature, token
budget): `embedding` (facet/routing vectors — model + dims **pinned per index**,
validated at boot; a change is an explicit re-embed operation), `enhance` (topic
generation), `sqlgen` (mode-a generation), `sqlfix` (bounded repair),
`clarify` (ambiguity-pattern generation), `pipeline_draft` (assisted DE
authoring), `profile_summary` (dataset descriptions). All structured outputs are
JSON-schema-constrained; free-text JSON parsing of model output is forbidden and
lint-checked (the predecessors did it twice — brief 06). Every call is metered
(tokens, cost, latency → `gateway_call_events` + Prometheus). The `mock` driver
backs every test; each role carries ≥1 recorded-fixture test against the real
wire format (§10 CLAUDE.md).

---

## 14. Config surface

Typed YAML + `env:` indirection for every secret; unknown keys rejected; per-
subsystem fail-loud validators; an example config ships and is smoke-checked.
One naming convention from day one (no flat/nested dual regime — brief 01 scar).
Key domains and the defaults that are contractual until re-tuned:

- `server`: HTTP/MCP addresses, timeouts, body limits (default 10 MiB), CORS.
- `auth`: mode(s), issuer, JWKS URL + `jwks_max_stale` (default 15m), audiences
  (`http`, `mcp`), self-issue keypair path, token TTL (default 1h).
- `store`: Postgres DSN (`env:`), pool sizing, migration policy.
- `sources`: credential key ring (`env: CHARTWORKS_SOURCE_KEYS`), per-kind
  defaults, connection-test interval.
- `workspace`: upload workspace DSN, upload limits (default 100 MiB / 1M rows;
  csv+xlsx+parquet on).
- `exec`: default row cap 10 000 (hard ceiling 100 000), statement timeout 60s
  (server-side + context), preview rows 200, self-repair on/off (default on).
- `nlq`: complexity-tier token budgets (defaults ≈ 1500/3000/6500), confidence
  bands (0.70/0.85), rules-lane budget (default 300 tokens, tokenizer-backed),
  example caps (max 7).
- `gateway`: driver, per-role model configs, embedding model + dims (pinned).
- `jobs`: worker concurrency (default 4), lease timeout, heartbeat interval.
- `telemetry`: log format, metrics on, OTel endpoint (off by default).

---

## 15. Telemetry, audit, health

- **Metrics** (Prometheus): RED per surface/route/tool;
  `access_decisions_total{grain,decision,reason}`; DE throughput (profiles,
  pipeline runs, rows materialized); NLQ funnel (routed/clarified/planned/
  executed, per mode); validation failures by code; execution latency +
  timeouts; gateway tokens/cost per role; jobs queue depth/lease reclaims;
  cache hit rates. **Every counter is exported** — the predecessors' dead
  in-process registry (brief 01) is the named counterexample; a telemetry
  conformance test asserts every registered metric appears on `/metrics`.
- **Audit** (content-free, store-backed, queryable via admin API): source
  connect/test/credential events, topic transitions, grant changes, every
  plan/run/context/submit (ids + decision + provenance — SQL text lives in the
  `queries` domain table, never in logs), every materialization, erasures.
  Coverage is **mechanical**: audited-route registration is asserted against the
  route/tool tables in CI (closing the predecessors' opt-in decorator gap,
  brief 04).
- **Health**: `/healthz` liveness; `/readyz` = store reachable + migrations
  current + JWKS fresh (external mode) + embedding pin validated. Data-source
  reachability is **status, not readiness** — reported per source; Pengui
  derives "connected/healthy" live (request §8).
- **Erasure**: `chartworks admin erase --tenant` hard-deletes a tenant's store
  rows, facet vectors, workspace database contents, and uploaded files —
  audited, loud on partial failure.

---

## 16. Evaluation (`chartworks eval`, `eval/`; D-031)

- **Golden suites** (CI-gated): (a) routing — question → expected topic/decision;
  (b) SQL generation — question + pinned pack → expected SQL (normalized
  compare, acceptable-alternatives, result-hash override — "right answer"
  outranks "same text"; brief 03 §6); (c) validation — the typed-error corpus
  incl. the standing golden CTE fixture; (d) chart selection — result shape →
  expected recipe; (e) context assembly — pack → budgeted context (token-count
  regression). Gates: pass-rate threshold (initial 0.85) + zero criticals; runs
  on the `mock`/fixture path in CI.
- **Red-team suite** (CI-gated): injection (statement smuggling, tautologies,
  UNION exfiltration, dialect escapes), schema-escape probes, cross-tenant
  probes through the full pipeline, resource exhaustion (row/timeout cap
  verification), BYO-mode adversarial submissions. Category taxonomy from brief
  03 §6 + brief 08.
- **Accuracy benchmarking** (manual loop, not CI): BIRD/Spider-2.0-informed
  categories against the sample warehouse — scored as *grounded* generation
  (with the semantic layer), tracked over time; never marketed as a
  BIRD-comparable number (brief 12's caveat).
- **Live gate** (D-010): real provider models + the sample warehouse via `.env`,
  `-count=1`, wave-end blocking, never CI-required. Golden cases seed from
  positive feedback (§9.8).

---

## 17. Operational shape

**The container is the deployment unit (D-037).** The reference image (glibc
base — debian-slim class, never bare musl) carries: the `chartworks` binary
(CGo-free today, CGo permissible per-dependency by decision entry) + the pinned
`bruin` binary (v0.11.666, glibc-dynamic; telemetry disabled — D-036). Bare-
metal/standalone remains supported: the binary runs everywhere; pipeline
execution requires `bruin` on PATH and otherwise degrades to a typed "pipeline
execution unavailable" (P4). `docker-compose` dev: Postgres 16 + pgvector on
**5434** (store) — the upload workspace uses a second database in the same
instance; dockerized MySQL/SQL Server with public datasets join for driver
conformance (D-032). Reference `Dockerfile` ships at the release wave.
Deployment is platform-agnostic; no platform-coupled bootstrap code outside a
driver (brief 01's Databricks-Apps coupling is the counterexample). Graceful
shutdown: servers drain, job leases release, in-flight Bruin runs are awaited
or lease-reclaimed with status checkpointed.

---

## 18. Decisions settled by this RFC

Logged as D-019…D-041 in `docs/decisions.md`:

| D | Decision |
|---|---|
| D-019 | MCP surface on `mcp-go` (pinned v0.55.x) for V1; Dockyard-portable contract discipline; Dockyard re-evaluated at the Wave-5 boundary (resolves D-011) |
| D-020 | Access = one grants relation, three grains (source/topic/dataset), per-principal incl. `agent:`, one resolver, deny-by-default, scopes token-carried (resolves D-015's mechanism) |
| D-021 | SQL-safety = three-stage AST validation ∩ grants + defense-in-depth read-only execution + split read/write interfaces + server-side timeouts + cursor-level caps (P1b/P1c concrete) |
| D-022 | BYO-agent mode ships as a published, versioned context bundle + `submit_sql` through the identical validation/exec core (mechanism for D-014) |
| D-023 | Bruin: mine-ideas-only; no engine dependency (CGo/Rust/Python collisions with D-005); `semantic-engine` submodule optionally re-spiked post-V1 (resolves D-018) |
| D-024 | Uploads enter as first-class sources via a managed Postgres upload workspace queried through the standard adapter; no DuckDB, no parallel mode |
| D-025 | One generic leased job queue + typed handlers + schedule dispatcher; no per-concern worker classes |
| D-026 | Charts V1 = declarative spec + deterministic rules selector; no renderer, no LLM ranker (post-V1, gateway-gated) |
| D-027 | Governed rules + proactive clarification are V1 scope (authoring, lifecycle, budgeted injection); shadow/replay machinery deferred to a late wave |
| D-028 | No in-process NLP library / language-model downloads; span hints are lexicon-light Go; language handling rides the gateway |
| D-029 | `vindex` seam confirmed: pgvector single driver, facet vectors only |
| D-030 | No local user/password/invite management; self-issue = API keys; admin bootstrap is a local CLI operation |
| D-031 | Eval strategy: golden + red-team CI gates (0.85 threshold), grounded-accuracy manual loop, live gate per D-010 |
| D-032 | V1 warehouse drivers = postgres, mysql, sqlserver, bigquery, snowflake, databricks; self-hostable engines validated against dockerized instances with public datasets, cloud engines via fixtures + the live gate |
| D-033 | Parser pin (cockroachdb-parser v0.25.2); its generation-subset obligation later superseded by D-038 |
| D-034 | BYO `bundle_ref`: stateless signed TTL handle pinning context, not capability |
| D-035 | Convention-8-verified driver/parser pins (incl. gosnowflake `minicore_disabled`) |
| D-036 | Bruin (pinned v0.11.666) as the DE write executor: CLI subprocess behind the `PipelineRunner` seam, SQL-only, custody-rendered connections; narrows-and-supersedes D-023 |
| D-037 | D-005 reversed per its clause: the container is the deployment unit; CGo permissible per-dependency; single-static-binary is a preference, not an invariant |
| D-038 | Layered read-side validation: read-only credentials primary, engine dry-run/EXPLAIN dialect-truth + table-grain allowlist everywhere, parser seam (crdb + gated sqlglot-go) for client-side depth; native-dialect generation |
| D-039 | The DE autonomy ladder: L2 goal-driven proposals w/ decision records (V1 target); L3 policy-scoped auto-apply (per-tenant opt-in); the drift-driven evolution loop; phase 26 |
| D-040 | The write boundary: managed `chartworks_*` schemas only; client baseline data read-only forever; enforced at definition validation + render gate + write-credential scope |
| D-041 | Autopilot governance: `autonomy.propose`/`autonomy.apply` scopes; no autonomous topic publication at any level; revert = drop managed artifacts, atomic per proposal |

Consumer-request §12 questions: Q1 §5.3/§5.1 · Q2 §6.1/§6.3 · Q3 §9.4 ·
Q4 §11.1 · Q5 §8.4 (in V1, scoped) · Q6 §10 (yes, deterministic selector) ·
Q7 §7.8 (cron/interval only) · Q8 §14 (`exec` defaults).

---

## 19. Non-goals (V1)

Chart rendering / frontend (D-013) · LLM chart ranker (D-026) · Python/R
pipeline assets + `ingestr` ingestion through Bruin (D-036 — SQL-only in V1) ·
condition/event schedule triggers (§7.8) ·
shadow-evaluation + historical replay for rules (D-027) · query-result caching
(correctness-hazardous under per-grant access; revisit with evidence) · result
pagination · cross-topic relationship discovery jobs (post-V1) · GEPA-style
prompt-pack optimization (post-V1; prompt packs themselves are config) ·
SQLite/embedded store (D-004) · external secret-manager driver (§6.3 seam
exists) · Dockyard adoption (re-evaluated Wave 5, D-019) · MCP Apps UI ·
row/column-level *warehouse* security pushdown (grants gate datasets; row-level
predicates are post-V1) · multi-language NL tuning (D-028) · Spreadsheet-mode
parallel path (never — §7.4).

---

*Amendments follow CLAUDE.md §15: a superseding decision entry plus an RFC PR —
never a silent edit.*
