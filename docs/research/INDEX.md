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

## Review currency

Briefs 01–13 describe the earlier investigation and dependency pins. Brief 14 records a
2026-09-04 selected-source audit, especially reporting, and distinguishes observed code,
documentation, test definitions, inventory, stubs and new proposals. Earlier source-diff
claims are historical, not a fresh guarantee that current source trees are identical.

The original brief 10 mine-ideas-only verdict was superseded by the later D-036/D-037
planning decision. Brief 13's runtime/library pins likewise require an actual host
compatibility check before adopting the new visual-delivery requirements. See the
[reporting development entry point](../reporting/README.md) and proposed
[RFC-002](../../RFC-002-Governed-Reporting.md); research alone does not change design authority.

## Briefs

| # | File | Subject |
|---|------|---------|
| 01 | `01-predecessor-architecture.md` | Both predecessors: process shape (FastAPI + 13 co-deployed poll-workers), router-first pipeline, full API inventory, config system, observability (incl. the dead-metrics scar), deploy divergence |
| 02 | `02-predecessor-data-and-execution.md` | Both predecessors: own ~50-table data model (sprawl evidence), warehouse adapters & the fork's encrypted Connections registry, plan/run execution path, SQL validator's single-gate read-only scar, result shaping, job/scheduler split |
| 03 | `03-predecessor-nlq-pipeline.md` | Both predecessors: routing, **the lean context-engineering layer** (card caps + complexity-tier token budgets), template precedence, three-stage validation, feedback/learn-positive loops, eval design, BYO-agent handoff analysis |
| 04 | `04-predecessor-security-tenancy.md` | Both predecessors: HS256/header-trust authn scars, topic-grain-only authz, tenancy enforcement + adversarial test registry, credential handling, SQL-safety posture, audit; direct recommendations for D-015/D-016/D-017 |
| 05 | `05-predecessor-diff.md` | *(mandatory historical baseline)* Client predecessor vs. generalistic fork: topic lifecycle, state machine and binding rules, carry-fix/keeper tables and access-model synthesis; recheck current behavior against brief 14 and the migration ledger |
| 06 | `06-predecessor-frontend-charts.md` | Both predecessors: rules-first chart selection, declarative chart-spec contract, frontend flows and vocabulary issues; visual delivery is now separately proposed in RFC-002 |
| 07 | `07-wrenai-ideas.md` | WrenAI (Apache-2.0): MDL semantic layer, Cube structured-aggregation object, dry-plan→dry-run→query ladder, phase-aware error taxonomy, selective column exposure |
| 08 | `08-datus-agent-ideas.md` | Datus-agent (Apache-2.0): SqlPolicyEnforcer seam, layered SQL-safety gates, metrics-first routing, KB taxonomy, reflect/fix split, BIRD/Spider eval harness |
| 09 | `09-agents-repo-ideas.md` | astronomer/agents (Apache-2.0): fail-closed MCP tool-annotation allowlist test, freshness scale, table-profile shape, data-layer heuristics; unguarded-SQL anti-pattern |
| 10 | `10-bruin-engine-evaluation.md` | Historical Bruin candidate-engine evaluation; initial mine-only verdict superseded by D-036/D-037. Revalidate pins/licensing at implementation. |
| 11 | `11-ssr-de-pipeline-draft.md` | Internal DE-pipeline draft: demand-driven medallion scoping, blind planner, canonical registry, whitelist-only composition; not evidence of implemented chart SSR |
| 12 | `12-genbi-landscape.md` | Historical web landscape: capability bar, semantic-layer primitives, evaluation reality and profiling-check families; public positioning is not proof of internal architecture |
| 13 | `13-dockyard-mcp-surface.md` | Historical MCP runtime/library decision and middleware seam; ordinary MCP support is not proof of Apps interoperability |
| 14 | `14-reporting-parity-audit.md` | Current selected-source reporting audit: blocks, revisions, outputs, certification, reports/dashboards, hybrid widgets, schedules/artifacts, source debt, NLQ continuity, security integration and closure ledger |
| 15 | `15-bruin-execution-reuse.md` | Evidence comparison of native adapters, stock Bruin CLI, leaf APIs and a narrow worker extension for governed multi-warehouse reads; exploration only |

## Subsystem → briefs

| Subsystem / phase area | Primary briefs | Secondary |
|---|---|---|
| `internal/api` (HTTP surface) | 01, 14 | 04, 06 |
| `internal/mcpserver` (MCP tools and Apps) | 13, 14 | 08, 09, 01 |
| `internal/auth` / `internal/identity` | 04, 14 | 01, 05 |
| access model (P1/D-015) | 04, 05, 14 | 01, 07 |
| `internal/config` | 01 | 04, 14 |
| `internal/store` (own state) | 02, 14 | 05, 01 |
| `internal/sources` (data-source adapters, credentials) | 02, 05 | 04, 10, 14 |
| `internal/engineering` (DE stage) | 11 | 09, 10, 12, 02, 14 |
| `internal/semantics` (topic packs, lifecycle) | 05, 03, 14 | 07, 10, 08, 12 |
| `internal/nlq` (routing, context, generation) | 03, 14 | 05, 07, 08, 12 |
| `internal/exec` (validation + read-only execution) | 02, 04, 14 | 03, 07, 08, 09 |
| `internal/charts` (chart-spec contract) | 06, 14 | 07 |
| reporting blocks / reports / dashboards / artifacts | 14 | 02, 05, 06 |
| rendering / iframe / MCP Apps | 14 | 06, 13 |
| `internal/gateway` (P5 seam) | 03 | 01, 08, 14 |
| `internal/telemetry` / audit | 01, 04 | 03, 14 |
| BYO-agent mode (D-014) | 03 | 08, 12, 13, 14 |
| `eval/` | 03, 12, 14 | 08, 07 |
| background jobs / scheduling | 01, 02, 14 | 05 |
