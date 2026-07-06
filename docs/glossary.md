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

## Domain

> *Stub — the RFC populates this section. Seeded only with product-defining terms unlikely
> to change.*

- **Data source** — a customer structured-data backend Chartworks reads to answer a query
  (e.g. a data warehouse). Reached read-only and access-scoped through a data-source
  adapter; **distinct** from Chartworks' own `Store` (D-004).
- **Dataset** — a scoped, addressable body of structured data within a data source that a
  caller may be granted access to; the unit deny-by-default access is computed over (P1).
- **NLQ (Natural Language Query)** — a natural-language question Chartworks routes, through
  the semantic model, to validated read-only SQL. The core inference the product migrates
  from the predecessors.
- **Data-engineering stage** — the upfront preparation step that shapes structured sources
  before modeling and NLQ; the extension the predecessors did not have (CLAUDE.md §1).
  *(Exact scope RFC-owned.)*
- **Semantic model** — the predecessors' "topic pack" concept: the curated business
  metadata (measures, dimensions, KPIs, join graphs) that grounds NLQ routing. *(Final
  shape, vocabulary, and lifecycle RFC-owned; the topic-lifecycle diff brief informs it.)*
- **Chart spec** — the normalized, provider-agnostic visualization output a query result
  renders as. *(Shape RFC-owned.)*

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
