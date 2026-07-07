# Brief 08 — Datus-agent: idea-mining for Chartworks

> Status: draft · 2026-07-06 · source: external_refs/Datus-agent-main (license: Apache-2.0)

**Source:** `github.com/datus-ai/datus-agent`, staged shallow copy under
`external_refs/Datus-agent-main` at the current tip. **License: Apache 2.0** — permissive,
no copyleft concern. This is a third-party OSS project, not one of the two gitignored
Python predecessors, so naming and reading it directly is fine — but the same discipline
applies: **no code is copied, ported, or vendored**; only ideas are inherited, cited by
file path for traceability. No predecessor file (`_ref/`) was read to produce this brief.

---

## Summary

Datus is a Python "data engineering agent" that turns NL into SQL via an **evolvable
context layer** (schema metadata, semantic models, reference SQL/templates, metrics,
platform docs) plus a **node-based workflow engine** with reflection/fix loops. It is
architecturally the closest OSS analog to Chartworks' whole pipeline — schema → semantic
model → NLQ-to-SQL → chart/dashboard — but is CLI-first and HITL-heavy, which Chartworks
(a stateless multi-tenant service) must not copy. The single strongest steal is its
**`SqlPolicyEnforcer` seam**: a pluggable, principal-aware component that rewrites/denies a
query's SQL *before* execution — close to the "deny-by-default, enforced inside the query"
shape Chartworks' P1 needs, already shaped as interface + no-op default + external
provider, mirroring Chartworks' own seam convention. Close behind: its layered,
independently-testable SQL-safety gates; its five-tier knowledge base as a topic-pack
content checklist; its metrics-first NLQ routing; its BIRD/Spider2 benchmark harness as an
`eval` template; and its dual MCP server/client posture as a precedent for a BYO-agent
context-handoff mode. Its CLI/HITL permission system, IM-channel "gateway," YAML
workflow DSL, and symmetric-JWT HTTP auth are flagged **skip** — wrong shape for a
stateless, multi-tenant service, or a direct P2 counter-example.

---

## 1. What it is

Datus ships one Python package, four entry points (`datus`/`datus-cli` REPL, `datus-api`
FastAPI server, `datus-mcp` MCP server, `datus-agent` benchmark/bootstrap CLI). It's a
Claude-Code-like interactive agent for data engineers: explore a source in the REPL →
accumulate context into a knowledge base → package a scoped "subagent" → serve via web,
HTTP, or MCP.

- **Node-based workflow engine** (`datus/agent/node/*`): a workflow is a YAML sequence of
  typed nodes (`schema_linking`, `gen_sql`, `execute_sql`, `reflect`, `fix`, `output`, plus
  control nodes `parallel`/`selection`/`subworkflow`) sharing a `Context`. Built-in
  templates: `fixed` (no adaptation), `reflection` (adds a retry loop), `metric_to_sql`
  (routes through metrics first).
- **Five-tier RAG knowledge base**: schema metadata, semantic models, reference SQL,
  reference templates, platform docs — one search interface, pluggable vector+relational
  storage (LanceDB+SQLite default; Postgres/pgvector as an alternate driver).
- **Subagents** (`docs/subagent/introduction.md`): named agentic-node configs (prompt,
  tool subset, scoped context, delegation policy), invoked via CLI, web URL, or as a tool
  (`task()`) — delegation capped at two levels deep.
- **Multi-engine execution** via a `ConnectorRegistry`: SQLite/DuckDB built in;
  Postgres/MySQL/Snowflake/StarRocks/ClickHouse etc. as installable adapters — plus a
  symmetric pattern for BI-platform adapters (Superset/Grafana) and semantic-layer
  adapters (MetricFlow/OSI).
- **Benchmark harness** running BIRD and Spider 2.0-Snow.
- **CLI/HITL-first throughout**: a `datus/tools/permission/` system (allow/deny/ask,
  named profiles) gates every write/DDL/bash/filesystem action behind a rule or an
  interactive prompt — the biggest structural mismatch with Chartworks (§7).

---

## 2. The agentic loop: plan, generate, self-correct, verify

**→ Chartworks landing: `internal/nlq`, `internal/exec`.**

- **`fixed` vs. `reflection` bracket the design space**: deterministic no-retry pipeline
  vs. one with a post-execution `reflect` node that scores the result and dispatches a
  strategy (regenerate, re-schema-link, doc search, or a dedicated `fix` node). The choice
  of loop shape is a per-request/per-topic knob, not a global pipeline — P4 ("fail loud")
  maps cleanly: a bounded loop is fine only if every retry is visible (metric/log per
  round) and an exhausted budget returns a typed error, never a silent best-effort answer.
- **`reflect` (decide whether/how to retry) is split from `fix` (targeted repair given an
  error message)** — a clean separation for Chartworks' `nlq`/`exec` boundary: a routing
  decision vs. a repair action, so a retry budget is enforced uniformly regardless of
  which repair strategy ran.
- **The reflection loop is capped by an explicit, observable round counter**
  (`workflow.reflection_round`, checked against config) — a minimal, reusable
  acceptance-criterion shape: Chartworks' NLQ retry loop needs the same explicit,
  telemetry-visible ceiling, not an implicit "keep trying."
- **`parallel` + `selection` nodes** fan out multiple SQL-generation strategies and pick a
  winner — a reusable pattern if `nlq` ever wants best-of-N generation without a bespoke
  ensemble mechanism.

---

## 3. Context/metadata management for text-to-SQL

**→ Chartworks landing: `internal/semantics`, `docs/glossary.md`.**

| Datus component | Stores | → Chartworks topic-pack analog |
|---|---|---|
| Schema Metadata | table/column defs + samples | base schema ingest, pre-semantic-model |
| Semantic Model | dims/measures/entity FKs, filter patterns mined from historical SQL | topic pack's measures/dims/join-graph core |
| Business Metrics | MetricFlow-backed KPIs, queryable directly (skips ad-hoc SQL gen) | topic pack's KPI layer + a direct metric-execution path |
| Reference SQL | historical queries + LLM summaries, searchable by intent | known-good query corpus per topic |
| Reference Template | parameterized (Jinja2) SQL, server-rendered | a third SQL-generation mode (§5) |
| Platform Docs | ingested dialect/platform docs | dialect-safety context for multi-warehouse support |

- **Semantic models are mined from historical question→SQL pairs**, not hand-authored; a
  separate `refresh-profile` mode cheaply re-profiles live data (bounded, read-only)
  without full regeneration. → Worth adopting "profile refresh is cheap and separate from
  authoring" as its own operation in `engineering`/`semantics`.
- **Metrics-first routing is its own workflow** (`metric_to_sql`): search existing metrics
  before generating ad-hoc SQL. Direct precedent for an NLQ routing rule: check the topic
  pack's KPI layer first — cheaper, more consistent, and narrows the SQL-safety surface
  (a known metric's SQL shape is pre-vetted; ad-hoc generation is not).
- **Table-scope enforcement is a separate, independently-testable check** from the
  read-only check (`_check_sql_table_scope`) — i.e. "read-only" and "schema-allowlisted"
  are two distinct gates, exactly the shape P1 wants (read-only + allowlist + injection
  guardrails as separable checks, not one monolithic validator).
- **Subject-tree taxonomy** (`domain/layer1/layer2`) has a "predefined" mode (fixed
  categories) and a "learning" mode (LLM invents/reuses categories organically). Useful
  vocabulary precedent — but the learning mode is the same unsupervised-persisted-mutation
  risk flagged for OpenKB's wiki-compilation in the sibling Soundings brief; same caution
  applies if Chartworks lets a topic taxonomy grow unsupervised.

---

## 4. Multi-engine support and execution safety

**→ Chartworks landing: `internal/sources`, `internal/exec`.**

- **Connector/BI/semantic-adapter registries confirm Chartworks' own §4.4 seam
  convention** (interface + entry-point-discovered drivers) — cite as external precedent
  when the `sources` adapter set is finalized; nothing new to adopt.
- **The strongest steal: `SqlPolicyEnforcer`** (`datus/tools/sql_policy.py`). A `Protocol`
  with one method — `enforce_read(sql, *, datasource, dialect, principal) ->
  EnforcementResult` (`allowed`, an optional *rewritten* `sql`, a `reason` on denial,
  `applied_policies` for audit) — called **inside** the read path before the connector
  ever sees the SQL, with a `NoopSqlPolicyEnforcer` default and a real implementation
  loaded via `importlib` only when configured. This is a near-literal sketch for
  Chartworks' deferred P1 SQL-safety mechanism: a query-path-internal, principal-aware
  filter as a seam with a deny-by-default no-op, letting the RFC pin the *concrete* access
  model later while the *seam* ships early.
- **Read-only vs. write/DDL is enforced at several independent layers**: a `read_only`
  flag checked even when the interactive permission layer is bypassed; a single-statement
  check (rejects multi-statement scripts); a statement-type classifier routing
  SELECT/SHOW/EXPLAIN one way and everything else another; plus the table-scope check
  (§3). Each layer fails closed on its own and is independently unit-testable — directly
  actionable for P1: build several small gates (statement-count, statement-type,
  allowlist, policy-rewrite), not one "validate SQL" function.
- **The read-only `explore` subagent bakes a `LIMIT` requirement into the tool contract
  itself** ("only SELECT, always with LIMIT") rather than trusting the caller — a cheap
  habit worth carrying into `exec`'s result-preview contract.

---

## 5. Benchmarks/evals and accuracy technique

**→ Chartworks landing: `eval/`.**

- **BIRD and Spider 2.0-Snow are used as-is**, not a custom set, with a documented
  download/bootstrap/benchmark/eval command chain. Chartworks' `eval/` package should plan
  to consume these (or a licensing-cleared subset) for external comparability rather than
  inventing a parallel harness.
- **The failure taxonomy is a ready-made report vocabulary**: Passed / No-SQL-or-Empty /
  Failed, with Failed split into Table Mismatch vs. Table-Matched-Result-Mismatch (further
  split into Row-Count vs. Column-Value Mismatch) — mirrors P4's "typed failure classes,
  never undifferentiated fail."
- **Per-task eval detail includes tool-call and node-type traces**, not just pass/fail —
  worth adopting so a regression ("took three extra turns for the same answer") is visible
  even when the final SQL is still correct.
- **Stated accuracy philosophy**: metrics-first routing and reference-corpus retrieval are
  the primary accuracy levers, ahead of prompt engineering — external validation that a
  strong topic pack + reference corpus likely matters more than NLQ-node sophistication.

---

## 6. BYO-agent mode

**→ Chartworks landing: the RFC's BYO-agent context-handoff design.**

- **Datus already runs both directions of MCP**: as a server exposing schema
  discovery/semantic search/safe read execution to an external agent (Claude
  Desktop/Code, Cursor), and as a client consuming external tools via `.mcp`. Close
  structural precedent: a BYO-agent handoff needs the same tool surface — topic
  discovery, semantic search, safe read execution — that Chartworks' own `nlq` stage
  would use internally.
- **Its "Dynamic" multi-datasource mode selects scope via a URL path segment** — flagged
  as an anti-pattern, not a steal: a multi-tenant Chartworks MCP tool must derive scope
  from the validated token/envelope (P2/P3), never a caller-suppliable path/header, even
  though "one server process, many datasources" is a fine idea.
- **The tool set exposed to an external agent is explicitly curated** (list/describe/
  search/read-only query, nothing mutating) — a workable starting checklist for a
  BYO-agent-mode tool surface.
- **Agent-as-tool delegation is capped at two levels** — a concrete, hardcoded bound worth
  copying wherever Chartworks allows agent delegation.

---

## 7. What to skip, and why

- **CLI/HITL tri-state permission system** (allow/deny/ask, interactive prompts). Built
  for a human at a REPL; Chartworks has no human in the request path (P4 wants a typed
  error, not "pause and ask"). A Chartworks-equivalent gate must resolve to a binary
  allow/deny synchronously.
- **The IM/notification "gateway" (Feishu/Slack bots)**. Unrelated to Chartworks'
  `internal/gateway` (the P5 LLM seam) — same word, different concept; flagged so no one
  conflates them when citing this brief.
- **The workflow YAML DSL as a user-facing configuration surface.** The internal
  node/context architecture (§2) is worth keeping; exposing pipeline authoring as
  end-user YAML is a different product shape than Chartworks' RFC-pending pipeline.
- **Auto Memory** (a 2000-byte flat per-agent file, prompt-injected every turn). No clean
  analog in a stateless multi-tenant service — it would just become ordinary
  `(tenant,user,session)`-scoped `Store` state, not a novel mechanism.
- **Unsupervised "learning-mode" taxonomy growth** (§3) — same P5/P7 and ACL-ambiguity
  risk as OpenKB's cross-document wiki compilation (see the sibling Soundings brief §6).
- **`datus-api`'s OAuth2-client-credentials + HS256 JWT auth.** A direct, named
  counter-example to P2 (asymmetric-only, `HS*`/`none` rejected at the parser) — useful
  precisely as ammunition for why P2 is stated as strictly as it is, not as something to
  adopt.

---

## 8. Steal / Adapt / Skip

| Idea | Verdict | Landing |
|---|---|---|
| `SqlPolicyEnforcer` seam (principal-aware `enforce_read`, no-op default, pluggable provider) | **Steal** (architecture, not code) | `internal/exec` |
| Layered, independently-testable SQL-safety gates | **Steal** | `internal/exec` |
| Metrics-first NLQ routing before ad-hoc generation | **Steal** | `internal/nlq` |
| Five-tier KB taxonomy as a topic-pack content checklist | **Adapt** (skip unsupervised "learning mode") | `internal/semantics` |
| Semantic model mined from history + cheap separate "profile refresh" | **Adapt** | `internal/semantics`/`internal/engineering` |
| `reflect`/`fix` split, bounded by an explicit round counter | **Steal** | `internal/nlq`, `internal/exec` |
| Reference-template mode (pre-approved parameterized SQL) | **Adapt** (concept yes, Jinja2 specifics no) | `internal/nlq` |
| BIRD + Spider 2.0-Snow eval corpus + typed failure taxonomy | **Steal** | `eval/` |
| Per-run tool-call/node-type trace in eval output | **Steal** | `eval/` |
| Curated read-only MCP tool subset for BYO/external agents | **Steal** (as a checklist) | `internal/mcpserver` |
| Delegation depth capped at 2 | **Steal** | wherever delegation exists |
| Connector/BI/semantic-adapter registry pattern | **Confirm, don't newly adopt** | `internal/sources`, RFC §4.4 |
| CLI/HITL tri-state permission system | **Skip** | wrong shape for a service |
| IM/notification "gateway" (Feishu/Slack) | **Skip** | name collision with P5 gateway only |
| User-facing YAML workflow DSL | **Skip** (internal node/context idea is fine) | n/a |
| Flat-file "Auto Memory" | **Skip** | becomes ordinary scoped `Store` state |
| HS256 shared-secret JWT auth | **Skip** | cited as a negative example for P2 |

---

## Open questions

1. **Where does metrics-first routing live** relative to the pipeline stages — does
   `semantics` expose a "known metric" lookup `nlq` consults first, or does `nlq` own that
   logic directly? Mirrors the Store-vs-data-source boundary discipline behind D-004.
2. **Does `charts` need two output shapes** — push into an existing BI target vs. a
   self-contained rendered artifact — as Datus's `gen_dashboard` (BI push) vs.
   `gen_visual_dashboard`/`gen_visual_report` (self-contained HTML) split suggests?
3. **Should the SQL-safety seam be one `PolicyEnforcer` interface covering read-only +
   allowlist + injection guardrails together, or three separate seams** matching P1's
   three named properties? Datus conflates them into one `enforce_read` call; P1 names
   them distinctly. An RFC decision, not one this brief should prejudge.
4. **Does a BYO-agent mode need its own distinctly-scoped MCP tool surface**, separate
   from what `nlq` uses internally, or the same tools plus an extra caller-identity check?
   Datus has no multi-tenant isolation requirement, so there's no precedent to lean on.
