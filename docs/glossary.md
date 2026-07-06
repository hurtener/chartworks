# Chartworks — Glossary

> Chartworks vocabulary. A new term introduced by a phase lands here in the same PR
> (CLAUDE.md §14). Domain terms only reach the wire and UI (P6); plumbing words stay
> internal.
>
> **Bootstrap note.** The Domain section is a **stub** — the RFC populates it (and the
> authoritative P6 forbidden-word list) once the pipeline and semantic model are settled.
> Only obviously-safe, product-defining terms are seeded now.

## Ecosystem

- **Chartworks** — this product: the ecosystem's Explorer seat, a Go-native structured-data
  analytics service (engineer → model → NLQ-to-SQL → charts, with access enforcement and
  read-only guarded SQL).
- **Soundings** — the sibling KnowledgeProvider: a Go-native full RAG over *unstructured*
  documents. Chartworks is its structured-data counterpart; the two split the
  document-vs-data boundary of the ecosystem.
- **Portico / Harbor / Dockyard / Stowage** — the other sibling products: the MCP gateway,
  the agent framework, the MCP Apps framework, and memory infrastructure, respectively.
- **Pengui** — the white-label multi-runtime coordinator that consumes the ecosystem's
  capabilities; owns identity, sharing policy, external id maps, and the Console UI.
  Whether Chartworks has a Pengui-side consumer request is settled at kickoff.
- **The client predecessor** — the prior client-tailored NLQ-to-SQL
  platform (`_ref/original_wayfinder/`), carrying production fixes worth mining (especially
  the topic lifecycle). Never named directly in this repo (D-001).
- **The generalistic predecessor** — the generalistic "Explorer" fork of the same codebase
  (`_ref/forked_wayfinder_explorer/`). Never named directly in this repo (D-001).

## Domain (populated by RFC-001 §2)

- **Data source** — a customer warehouse connection or an upload workspace: the
  structured-data backend Chartworks reads (and, only via materializations, writes).
  Reached through a data-source adapter; **distinct** from Chartworks' own `Store`
  (D-004). Status lifecycle: `unverified → connected → unavailable`.
- **Dataset** — a governed, queryable, table-shaped artifact: a registered source table,
  an uploaded file's table, or a materialization. Carries schema, profile, freshness,
  lineage, version, and grants; the finest access grain (P1a, D-020).
- **Topic** — the semantic-model unit grounding NLQ: measures, dimensions, derived KPIs,
  join graph, business context, governed rules. Versioned and governed (RFC §8.2:
  draft → review → published → deprecated; topic-level active ⇄ archived).
- **Topic pack** — a topic version's full payload (the authoring view).
- **Capability contract** — the compact, prompt-safe projection of a published topic
  pack that routing and SQL generation actually consume (the lean context layer,
  RFC §8.3).
- **Context bundle** — the *published, versioned* projection handed to a BYO agent in
  `get_query_context`: routing result, contract slice, restated governance constraints,
  dialect + SQL requirements, clarification slots (RFC §9.4, D-022).
- **Question / plan / run** — the NL question; *plan* = route + generate + validate
  without executing; *run* = plan then execute (distinct capability scopes).
- **Preflight** — the routability check: which topic(s), what confidence, which
  clarification slots — no SQL, no execution.
- **Pipeline** — a declarative, versioned data-engineering definition: SQL steps +
  quality checks + a declared destination + an optional schedule (RFC §7.3).
- **Materialization** — a pipeline write into a declared destination; the only write
  Chartworks performs against customer infrastructure (P1c, D-017/D-021).
- **Upload workspace** — the managed Postgres database where a tenant's uploaded
  CSV/XLSX/Parquet files become queryable tables, reached through the standard
  `postgres` adapter like any warehouse (RFC §7.4, D-024).
- **Grant** — an explicit per-principal permission `(grain ∈ source|topic|dataset,
  permission ∈ read|query|manage)`; absence means denial (D-020).
- **Principal** — `user:<id>` · `agent:<id>` · `svc:<name>` · `key:<id>`. Agents hold
  their own grants (D-020).
- **Session** — a conversation scope for query refinement; part of the isolation triple.
- **Freshness** — dataset recency status: `fresh` / `stale` / `very_stale` / `unknown`.
- **Lineage** — a dataset's declared upstream datasets + producing pipeline/step.
- **Governed rule** — a tenant/topic-scoped, structurally validated business constraint
  injected into generation context under its own token budget; lifecycle
  `proposed → active → retired` (RFC §8.4, D-027).
- **Clarification slot** — a named ambiguity ("which region", "which time grain")
  detected by topic-scoped patterns *before* generation (RFC §8.4).
- **Example (learned)** — a question→SQL pair with a routing weight, learned from
  feedback; lifecycle `candidate → active → retired` (RFC §9.8).
- **Re-check source / revalidate** — the domain-clean names for the operations the
  predecessors called "repair": refresh schema after source drift; re-verify a topic or
  example against its sources (P6).
- **Chart spec** — the declarative, provider-agnostic presentation contract:
  `ColumnMetadata[]` + `ChartRecipe` (kind, bindings, formatting, score, rationale) +
  provenance envelope; V1 never pre-inflates a charting library's options (RFC §10,
  D-026).
- **Scope-debug** — the admin-only, read-only diagnostic reporting *which predicate*
  denied access or routing (RFC §5.4); never user-facing.
- **NLQ (Natural Language Query)** — a natural-language question routed through the
  semantic model to validated read-only SQL.

## Internals & seams

- **`gateway` seam** — the one intelligence seam; all embedding/LLM/rerank/SQL-generation
  calls flow through it. V1 drivers: `bifrost`, `mock` (D-003, P5).
- **Schema-constrained generation** — the only way the gateway produces structured output
  (routing decisions, generated SQL, chart specs): a mandatory JSON schema constrains the
  call and re-validates the result; a violation is a typed error, never a free-text/partial
  parse (P5).
- **Recorded-fixture test** — replays a once-captured, secret-scrubbed real provider
  wire-format response so the `bifrost` mapping is validated without a live paid API call
  in CI.
- **`store` seam** — Chartworks' own durable state; V1 driver `postgres` (pgx/v5). SQLite is
  out of scope (D-004). Distinct from the customer data sources it queries.
- **Data-source adapter** — the read-only, access-scoped connector to a customer data
  warehouse behind its own seam; the V1 driver set is RFC-owned (D-004).
- **Frozen per-request envelope** — the caller's identity triple `(tenant, user, session)`
  plus resolved access claim, read once from the validated token and never mutated
  mid-request (P2).
- **Self-issue / external-issuer** — the two auth modes: Chartworks mints its own tokens
  (standalone) or validates Pengui-issued tokens (ecosystem). Both from one binary, with
  per-instance dual audiences (MCP / HTTP) (D-006).
- **SQL-safety property** — the Chartworks-specific binding property the RFC must define:
  read-only execution, schema allowlisting, and injection guardrails over generated SQL
  (P1, CLAUDE.md §6). Until settled, generated SQL is not executed against real data.
- **Scope-debug diagnostic** — the admin-only, read-only diagnostic answering "why didn't
  the caller see data X / route to topic Y" — never a user-facing "repair" surface (the
  anti-pattern the predecessors grew).
- **Live-verification gate** — the real-provider, `.env`-driven, never-CI wave-end check
  that runs the pipeline against real models (D-010).
