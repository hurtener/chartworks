# Research briefs — reverse index

> Subsystem → informing briefs. A phase plan cites the briefs it inherits from
> ("Brief findings incorporated" section); a plan citing no brief is a drift signal
> (CLAUDE.md §16). Briefs are authoritative for *context*, never for design — the RFC
> and phase plans decide.

Both predecessors are referred to only as **the client predecessor**
(`_ref/original_wayfinder`) and **the generalistic predecessor**
(`_ref/forked_wayfinder_explorer`) (CLAUDE.md "Predecessor hygiene"); no predecessor
code or file is ever pasted into a brief. Citation convention: `_ref/` root directory
names are sanctioned; inner package names are elided as `<pkg>`.

## Briefs

| # | File | Subject |
|---|------|---------|
| 01 | `01-predecessor-architecture.md` | Both predecessors: process shape (FastAPI + 13 co-deployed poll-workers), router-first pipeline, full API inventory, config system, observability (incl. the dead-metrics scar), deploy divergence |
| 02 | `02-predecessor-data-and-execution.md` | Both predecessors: own ~50-table data model (sprawl evidence), warehouse adapters & the fork's encrypted Connections registry, plan/run execution path, SQL validator's single-gate read-only scar, result shaping, job/scheduler split |
| 03 | `03-predecessor-nlq-pipeline.md` | Both predecessors: routing, **the lean context-engineering layer** (card caps + complexity-tier token budgets), template precedence, three-stage validation, feedback/learn-positive loops, eval design, BYO-agent handoff analysis |
| 04 | `04-predecessor-security-tenancy.md` | Both predecessors: HS256/header-trust authn scars, topic-grain-only authz, tenancy enforcement + adversarial test registry, credential handling, SQL-safety posture, audit; direct recommendations for D-015/D-016/D-017 |
| 05 | `05-predecessor-diff.md` | *(mandatory)* Client predecessor vs. generalistic fork: **topic lifecycle exhaustively** (unified state machine + 7 binding rules), carry-fix table (client) + keeper table (fork), access-model synthesis |
| 06 | `06-predecessor-frontend-charts.md` | Both predecessors: rules-first chart selection, the declarative chart-spec contract (the D-013 reserved slot), frontend flows, the live P6 "repair" violation, recommended V1 chart-spec contract |
| 07 | `07-wrenai-ideas.md` | WrenAI (Apache-2.0): MDL semantic layer, Cube structured-aggregation object, dry-plan→dry-run→query ladder, phase-aware error taxonomy, selective column exposure |
| 08 | `08-datus-agent-ideas.md` | Datus-agent (Apache-2.0): `SqlPolicyEnforcer` seam, layered SQL-safety gates, metrics-first routing, KB taxonomy, reflect/fix split, BIRD/Spider eval harness |
| 09 | `09-agents-repo-ideas.md` | astronomer/agents (Apache-2.0): fail-closed MCP tool-annotation allowlist test, freshness scale, table-profile shape, data-layer heuristics; the unguarded-SQL anti-pattern (named, not adopted) |
| 10 | `10-bruin-engine-evaluation.md` | Bruin (Apache-2.0, pinned v0.11.666) as candidate engine (D-018): verdict **mine-ideas-only** (CGo+Rust/Python collide with D-005); `semantic-engine` submodule carved out as a separate narrow adopt-spike candidate |
| 11 | `11-ssr-de-pipeline-draft.md` | Internal ssr DE-pipeline draft: take demand-driven medallion scoping, blind planner, canonical registry, whitelist-only composition; ditch refresh/drift design and toolchain assumptions |
| 12 | `12-genbi-landscape.md` | Web landscape: Teramot capability bar (BYO-SQL split unconfirmed publicly), semantic-layer primitives (Cube/MetricFlow/Malloy), BIRD/Spider 2.0 eval reality, profiling-check families |
| 13 | `13-dockyard-mcp-surface.md` | D-011 evidence: Dockyard runtime (pinned v1.8.0) vs mark3labs/mcp-go (v0.55.1) — decision table, the missing global tool-middleware seam, the "mcp-go now / Dockyard later" hybrid |

## Subsystem → briefs

| Subsystem / phase area | Primary briefs | Secondary |
|---|---|---|
| `internal/api` (HTTP surface) | 01 | 04, 06 |
| `internal/mcpserver` (MCP tools) | 13 | 08, 09, 01 |
| `internal/auth` / `internal/identity` | 04 | 01, 05 |
| access model (P1/D-015) | 04, 05 | 01, 07 |
| `internal/config` | 01 | 04 |
| `internal/store` (own state) | 02 | 05, 01 |
| `internal/sources` (data-source adapters, credentials) | 02, 05 | 04, 10 |
| `internal/engineering` (DE stage) | 11 | 09, 10, 12, 02 |
| `internal/semantics` (topic packs, lifecycle) | 05, 03 | 07, 10, 08, 12 |
| `internal/nlq` (routing, context, generation) | 03 | 05, 07, 08, 12 |
| `internal/exec` (validation + read-only execution) | 02, 04 | 03, 07, 08, 09 |
| `internal/charts` (chart-spec slot) | 06 | 07 |
| `internal/gateway` (P5 seam) | 03 | 01, 08 |
| `internal/telemetry` / audit | 01, 04 | 03 |
| BYO-agent mode (D-014) | 03 | 08, 12, 13 |
| `eval/` | 03, 12 | 08, 07 |
| background jobs / scheduling | 01, 02 | 05 |
