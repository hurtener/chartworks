# Pengui → Chartworks (the Explorer seat) — Interface & Requirements Request

> **Status:** derived · v1.0 · 2026-07-06
> **What this is:** the consumer-side contract request for Chartworks, derived at
> kickoff (interview answer 2, D-012) the way Soundings' Pengui request doc was —
> written from the perspective of the three consumers (Pengui's Console over HTTP,
> Harbor agents over MCP, standalone API customers) stating what they need Chartworks
> to be. It fills slot 5 of the CLAUDE.md §2 authority chain.
> **Relation to the RFC:** RFC-001-Chartworks absorbs and supersedes this document as
> the design source of truth; this remains the authoritative record of *what the
> consumers asked for*. Departures are named in the RFC and logged in
> `docs/decisions.md`.

---

## 0. One-paragraph frame

Chartworks is the family's **Explorer seat**: the structured-data analytics
capability. Where Soundings answers from *documents*, Chartworks answers from
*data* — warehouses the customer already runs (Postgres, BigQuery, Snowflake,
Databricks) and structured files they upload (CSV/XLSX/Parquet). It is more than an
NLQ-to-SQL box: an upfront **data-engineering stage** connects, profiles, shapes,
and materializes sources into governed datasets; a **semantic model** (topics:
measures, dimensions, KPIs, join graphs) grounds natural-language questions; questions
become **validated, read-only SQL**; results come back as normalized tables plus a
declarative **chart spec** a UI can render. One binary, dual surface, swappable
behind a contract — exactly the Soundings posture, for the structured half of the
world.

## 1. The non-negotiables (ecosystem invariants Chartworks must honor)

These are the family's standing invariants; Chartworks inherits them wholesale
(CLAUDE.md P1–P7):

1. **Deny-by-default access computed inside the query path** — never
   fetch-then-filter; an empty effective-access set short-circuits to "no results"
   without issuing a query.
2. **Identity lives only in the signed token** (asymmetric JWT; `HS*`/`none`
   rejected at the parser; `aud` mandatory and per-instance; frozen per-request
   envelope). No identity or access in `X-*` headers, ever.
3. **Multi-isolation on `(tenant, user, session)`** enforced at the storage layer.
4. **Fail loud** — typed errors + metrics, never silent degradation, never an
   unvalidated query executed as a fallback.
5. **One intelligence seam** — every model call through the gateway; structured
   outputs schema-constrained.
6. **Domain vocabulary only** on every wire/UI/error/log surface (§2.4 below).
7. **One logic core, thin surfaces** — MCP and HTTP expose the same capabilities
   through the same core; a capability ships on both surfaces with parity tests.

And two Chartworks-specific invariants the consumers explicitly require:

8. **Generated SQL is untrusted regardless of who generated it.** Chartworks' own
   generator and an external agent's submission pass the *same* validation and the
   *same* read-only execution gates. No caller identity or mode relaxes a check.
9. **The NLQ path never writes.** Writes into customer warehouses exist only in the
   data-engineering stage, through a declared, governed, audited materialization
   path that NLQ-originated SQL structurally cannot reach (D-017).

## 2. Naming & vocabulary (explicit request)

### 2.1 The primary nouns

- **data source** — a customer warehouse connection or an uploaded file source.
  Never "connection string", "adapter", "warehouse config" on the wire.
- **dataset** — a governed, queryable table-shaped artifact Chartworks knows about:
  a profiled source table, an uploaded file's table, or an engineered
  materialization. Datasets have freshness, lineage, and grants.
- **topic** — the semantic-model unit (measures, dimensions, KPIs, join graph,
  business context) that grounds NLQ. Topics are versioned and governed
  (draft → review → published → archived from the consumer's viewpoint).
- **question / query** — the NL question and the SQL derived from it. "Plan" =
  show me the SQL without running it; "run" = execute it.
- **pipeline** — a data-engineering definition that produces datasets
  (transformations + materializations + schedule).

### 2.2 Words to never ship

On any API field, UI string, error message, or human-read log line — the
predecessors leaked exactly these and grew a user-facing "repair" surface out of
the leak (the anti-pattern P6 names):

`repair` · `broken` · `enhancement`/`enhance` (as a user-facing verb) · `index` /
`reindex` · `embedding` · `vector` · `shard` · `namespace` · `sync` · `wiring` ·
`collection` · `cache` (as a user-facing noun) · `worker` · `job` (prefer the
domain operation's own name; a long-running operation is e.g. "profiling",
"publishing", "refreshing").

A source table that went missing is "unavailable — reconnect or re-check the
source", never "broken — repair". The RFC owns the authoritative list; drift-audit
enforces it mechanically.

### 2.3 Principal names

The prefixed-string convention the family already uses: `user:<id>`,
`agent:<id>`, `svc:<name>`, `key:<id>` (self-issue API keys). Chartworks must
support **per-agent principals** as first-class grant subjects — a Harbor agent's
access is its own, not its owner's (D-015).

## 3. Identity & auth — how the token is validated

### 3.1 Two modes + the trust root

Same dual-mode shape as Soundings, from one binary, combinable:

- **external_issuer** (ecosystem citizen): validate Pengui-issued JWTs against
  Pengui's JWKS. `iss` pinned, `aud` per-instance and **distinct per surface**
  (an HTTP/Console audience and an MCP/agent audience, so a UI token cannot be
  replayed as an agent token), `exp` + skew enforced, JWKS cached with
  `jwks_max_stale` failing **closed**.
- **self_issue** (standalone product): Chartworks mints its own tokens for API-key
  callers; keys hashed, compared constant-time, never logged.

### 3.2 The claims Chartworks reads

`tenant` (mandatory), `sub` (prefixed principal), surface `aud`, capability
scopes, and optionally an agent identity when the caller is an agent acting under
its own grants. Read once into a frozen envelope. Chartworks never recomputes
Pengui-side sharing; resource-level grants on Chartworks-owned resources (sources,
datasets, topics) are Chartworks' own domain state, resolved deny-by-default per
request.

### 3.3 What Chartworks must NOT do (drop the predecessors' trust scars)

- No header-derived identity anywhere — including at the **token-minting
  boundary**: the predecessors minted durable admin-capable sessions from a bare
  `x-forwarded-*` header (first caller ever seen became platform admin). Any
  identity-exchange endpoint must consume a *cryptographically verifiable*
  assertion, or not exist.
- No parallel identity channels per feature (the predecessors had three). One
  validation path for every surface and every mode.
- No silent secret fallback: a missing signing secret/key is a refused boot,
  never an ephemeral generated one.

### 3.4 Service-to-service

Rare; `svc:` principals with their own scoped grants, never a bespoke shared
secret.

## 4. The access model requested (D-015)

The predecessors' model — "the app holds the warehouse privileges; topic
visibility is the entire grant" — is the scar to fix. Requested shape:

1. **Grants are per-principal** (users, agents, service accounts, API keys) and
   exist at **three grains**: data-source, topic, dataset. Absence of a grant is
   denial. Tenant role (admin/member/viewer) sets defaults; explicit grants
   extend or restrict within the tenant boundary, which is always intersected.
2. **The DE stage makes dataset-grain mandatory**: an engineered materialization
   must be grantable independently of the topic that produced it.
3. **Access is resolved once per request** into the frozen envelope by one
   resolver (one representation, P7) and applied **inside** every store and
   data-source query. Empty set → typed "no access" without touching the
   warehouse.
4. **Per-decision observability**: every allow/deny increments a metric and emits
   a content-free structured event; an admin-only, read-only **scope-debug**
   diagnostic answers "why can't caller X see dataset Y / route to topic Z" —
   never a user-facing surface.

## 5. The dual surface — one capability, two faces

### 5.1 MCP tool surface (agent-facing; consumed via Harbor)

Requested tool families — final names/shapes are the RFC's to pin, tiered so a
minimal agent needs only the first two tiers:

| Tier | Tools (indicative) | Purpose |
|---|---|---|
| Discover | `list_topics`, `describe_topic`, `list_datasets`, `describe_dataset` | What can I ask, over what data, with what freshness/lineage |
| Ask | `preflight_question`, `plan_query`, `run_query`, `refine_query` | Route/clarify → SQL without execution → execute validated SQL → refine in-session |
| BYO-agent | `get_query_context`, `submit_sql` | The Teramot-class mode (D-014): hand me the governed context bundle, I write the SQL, you validate + execute it under the same gates |
| Feedback | `submit_feedback` | Verdicts close the learning loop |

Requirements: typed, normalized results (routing evidence, SQL, validation
report, result preview, chart spec) — never raised stack traces, never internal
ids/endpoints/credentials. Every tool annotated read-only vs write and
CI-enforced fail-closed (a new unannotated tool fails the build). Plan-vs-run
must map to distinct capability scopes so a plan-only agent can never execute.

### 5.2 HTTP surface (Console / API customers)

Everything the MCP surface does, plus the management plane: data-source CRUD +
connection testing; uploads; profiling and dataset inspection; pipeline authoring
and runs; topic lifecycle (create/generate, draft editing, review, publish,
rollback, archive, sharing/grants, export/import); schedules; sessions/history;
feedback review; admin (tenants, principals, grants, scope-debug, audit query).
`/healthz` + `/readyz` unauthenticated; everything else behind the token. The
HTTP `aud` is distinct from the MCP `aud`.

### 5.3 SDK

A Go SDK (`sdk/chartworks`) with HTTP and in-process modes, surface-parity
tested, as Soundings ships.

## 6. What Chartworks owns at the boundary

- **Owns:** data-source connections + credential custody (encrypted at rest,
  D-016); ingestion of structured uploads; profiling/quality; transformations +
  materializations (the governed write path); dataset versioning/lineage/
  freshness; the semantic model and its lifecycle; NLQ routing/generation/
  validation/execution; chart-spec production; its own jobs/schedules; access
  *enforcement* on its own resources.
- **Does not own:** identity/sharing *policy* (Pengui's); answer prose /agent
  orchestration (Harbor/Pengui — Chartworks returns data + evidence, not essays);
  memory (Stowage); unstructured documents (Soundings — a scanned PDF of a table
  is Soundings' problem; a CSV/XLSX/Parquet file or a warehouse table is
  Chartworks'); chart *rendering* (V1 emits the declarative spec only, D-013).
- **The document/data routing decision** is made upstream by Pengui; Chartworks
  does not sniff file semantics beyond structured-format validation.

## 7. Multi-tenancy & isolation

Multi-tenant day one. The tenant predicate is `NOT NULL` and inescapable at the
store layer (the fork relaxed this — do not inherit); every customer-warehouse
query is scoped by the caller's resolved grants; per-tenant data sources mean one
tenant's questions can never execute against another tenant's warehouse, and
credential custody is per-source, per-tenant. Uploads are tenant+principal
scoped. Cross-tenant probes are a standing adversarial test, not a review note.

## 8. Observability, audit, health

- Prometheus-style metrics: per-access-decision counters, DE throughput,
  routing/generation/validation/execution latency, gateway token/cost metering,
  cache hits.
- Audit events are **content-free** (ids + principals + decision) — source
  connect/test, credential rotation, topic transitions, grant changes, every
  plan/run/submit-SQL, every materialization. Never data rows, never SQL text in
  logs, never token bytes.
- `/healthz` (liveness) + `/readyz` (own store reachable + JWKS fresh);
  **data-source reachability is reported status, never a boot gate** — Pengui
  derives "connected/healthy" live from Chartworks and persists only the intent
  to connect.

## 9. Operational shape

One static binary (`chartworks serve|mcp|admin|eval`), CGo-free (D-005),
Postgres-only own store (D-004), Docker-compose dev environment (Postgres on
5434), config via typed YAML + `env:` indirection failing loud at boot, no
unauthenticated surfaces beyond health. Background work (profiling, enhancement,
publishing, refresh, pipeline runs) runs in-process on a **single generic leased
job queue** — not the predecessors' 13 bespoke worker classes.

## 10. Anti-patterns to avoid (the predecessors' scars — do not reopen)

1. Topic-grain-only access — visibility of a topic must not imply access to all
   its data forever (§4).
2. Header-trust identity, at request time *or* mint time (§3.3).
3. A parallel weak-auth "spreadsheet mode" — uploads enter the same governed
   source→dataset→topic path as warehouses, same access primitive, same
   execution route.
4. Read-only enforcement living in exactly one upstream validator with no
   execution-layer backstop; write and read paths distinguished only by a flag.
5. Fetch-then-filter anywhere; unscoped query methods existing at all.
6. Silent degradation: dead metrics counters that export nowhere; empty results
   standing in for denials; a missing secret auto-generated at boot.
7. Plumbing vocabulary on the wire ("repair"-class surfaces, §2.2).
8. Free-text JSON parsing of model output (the predecessors did it twice).
9. Two-of-anything: parallel identity channels, per-feature job tables, bespoke
   caches without one owner, a second SQL-execution entry point "just for this
   feature".
10. Unbounded LLM repair loops — every retry loop has a hard cap and a typed
    terminal failure.

## 11. What the consumers commit to

- **Pengui**: issues JWTs with the agreed claims; owns identity/sharing policy
  and the id map between Console entities and Chartworks resources (in one
  place); derives connection health live; routes document-vs-data upstream;
  invokes tenant-erasure primitives when offboarding.
- **Harbor**: consumes the MCP surface through its southbound driver with bearer
  tokens; treats tool annotations (read-only/write) as authoritative; propagates
  session identity so `(tenant, user/agent, session)` isolation holds.
- **API customers**: use self-issue keys under the same contract; no bespoke
  auth or result shapes on request.

## 12. Open questions to settle in the RFC

1. The exact access-claim/grant split: which access facts ride the token
   (capability scopes) vs live as Chartworks grant state (resource grants) —
   and the resolver's precedence rules.
2. The V1 data-source driver set (Postgres, BigQuery, Snowflake, Databricks,
   DuckDB-for-uploads?) and the credential-encryption mechanism/key source.
3. The published Context Bundle schema for BYO-agent mode and its versioning
   policy (it is a public contract the moment it ships).
4. The MCP library (mcp-go vs Dockyard, D-011) and the tool-name inventory.
5. Whether the governed business-rules/clarification layer (client predecessor
   only) is V1 scope, and at which wave.
6. The chart-spec contract's exact field set (brief 06's recommendation) and
   whether the deterministic chart-kind selector ships in V1.
7. Scheduling scope for V1 (cron/interval only vs condition/event triggers).
8. Row caps, statement timeouts, and result-size ceilings — the concrete
   numbers and where each is enforced (application + server-side).
