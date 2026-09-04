# Chartworks — actionable implementation plan

Revised 2026-09-04 after the owner merged PR #2 and clarified the security/Apps boundaries. The active RFCs and all phase plans are reconciled. The previous detailed plans are preserved under `docs/archive/phase0-plans/` as historical reference, not competing instructions.

## Fixed product decisions

Pengui is the sole issuer and authentication/access-policy owner. Chartworks verifies JWTs and applies signed scopes/resource restrictions; it does not maintain local roles, grants, service accounts, API keys or embed credentials. Harbor/Pengui MCP Apps compatibility is established. Build Chartworks's tools/resources/viewer, not a compatibility project. Preserve functional cron/interval/manual schedules and actual saved-query/block/report targets; discard the source event/condition/condition-check/custom-code stubs.

Business validation, semantic review, certification, SQL safety, source isolation, retention and execution budgets remain Chartworks responsibilities. These checks do not decide identity or sharing. See [the authority contract](../contracts/pengui-authority.md) and D-044–D-052 in the appended decision log.

## How to use this plan

Read RFC-001 for shared architecture/security, RFC-002 for reporting, then [COMMON.md](COMMON.md) and the owning phase below. `phase-registry.json` is the compact dependency/status/acceptance-count registry. `coverage.json` maps the 63 source-feature IDs and 40 review gates to named phase acceptance IDs. Neither file claims the tests have run.

All phases are initially **planned**. Each original phase has six criteria; each added phase has eight: **220 acceptance criteria** in total. One real `TestPhaseNN/ACxx` result must back each criterion. A planning SKIP is not a runtime pass. The completion workflow and strict runner are in COMMON.md.

Phase numbers are stable identifiers, not chronology. In particular, phases 21–23 are early transport/client shells, and phase 25 is the final release gate. Domain phases register concrete HTTP/SDK/MCP capabilities as they land. An unbuilt operation must be absent, not a success-returning placeholder.

## Dependency and ownership table

| Phase | Plan | Hard dependencies | Deliverable |
|---|---|---|---|
| 01 | [Binary/config/telemetry](phase-01-binary-config-telemetry.md) | none | Typed config, safe lifecycle/health, metrics/audit |
| 02 | [Store](phase-02-store-migrations.md) | 01 | Real PostgreSQL transactions, tenant/CAS/migration primitives; no IAM tables |
| 03 | [JWT/identity](phase-03-auth-identity.md) | 01 | Pengui verification-only envelope and provider scope handoff |
| 04 | [Scope enforcement](phase-04-access-grants.md) | 02,03 | Enforce signed action/resource reach; no local grant resolver |
| 05 | [Gateway](phase-05-gateway.md) | 01 | Per-role providers, structured outputs, budgets and optional roles |
| 06 | [Queue/occurrences](phase-06-jobs-scheduler.md) | 02,03,04 | Leases/fencing, idempotency, cron/interval and delegated-authority port |
| 07 | [Facets](phase-07-vindex.md) | 02,04 | Scoped pgvector generations and batched retrieval |
| 08 | [Source core](phase-08-sources-core.md) | 04,09 | PostgreSQL, registry/secret custody and actual execution contexts |
| 09 | [SQL validation](phase-09-sql-validate-core.md) | 03,04 | Read interface/opaque validated plan, parser/native safety |
| 10 | [Read execution](phase-10-exec-read.md) | 08,09 | Read-only query/caps/cancel, exact normalized data |
| 11 | [Uploads](phase-11-uploads-workspace.md) | 06,08 | CSV/XLSX/Parquet staged workspace -> normal governed dataset |
| 12 | [Profiling](phase-12-engineering-profiling.md) | 05,06,08,10 | Versioned profile/quality/freshness and source drift |
| 13 | [Pipelines](phase-13-engineering-pipelines.md) | 06,09,10,12 | SQL-only runner, managed writes, quality/lineage, staged effects |
| 14 | [Warehouse drivers](phase-14-warehouse-drivers.md) | 08,09,10 | Six-engine source contract and real support evidence |
| 15 | [Topics](phase-15-topics-lifecycle.md) | 04,05,07,12,21 | Entity lifecycle, ready-facet publication, health and portability |
| 16 | [Rules/clarification](phase-16-rules-clarification.md) | 05,15 | Rules, slots, hard constraints, replay/shadow |
| 17 | [Routing/context](phase-17-nlq-routing-context.md) | 05,07,15,16 | Lean budgets, pins, English/Spanish, confirmed multi-topic context |
| 18 | [NLQ](phase-18-nlq-generation-execution.md) | 09,10,17 | Preflight/plan/run/refine, templates, bounded correction and learning |
| 19 | [BYO](phase-19-byo-mode.md) | 02,10,17 | Versioned stored context reference and identical submit safety |
| 20 | [Output specs](phase-20-charts-spec.md) | 10,15 | Fourteen-kind catalog, exact formats and safe saved mappings |
| 21 | [HTTP shell](phase-21-http-api.md) | 01,02,03,04 | Early protected routing/schema/audit registration |
| 22 | [MCP shell](phase-22-mcp-server.md) | 21 | Early established-profile tools/resources, no host qualification |
| 23 | [SDK/CLI](phase-23-sdk-cli-parity.md) | 21,22 | Early typed clients, caller token provider and cumulative parity |
| 24 | [Evaluation](phase-24-eval.md) | 18,19,20,29 | Quality/adversarial/replay/optimization and evidence |
| 25 | [Final release](phase-25-e2e-release.md) | 24,26,34 | Cumulative all-feature/engine/operational closure |
| 26 | [L2 engineering](phase-26-engineering-autopilot.md) | 13,15,16,21,27 | Reviewed proposals and drift amendments; staged apply/compensation |
| 27 | [Governed blocks](phase-27-reporting-blocks.md) | 15,20,21 | Draft/revision/validation/certification/parameters/impact |
| 28 | [Frozen execution/artifacts](phase-28-reporting-execution-artifacts.md) | 05,06,10,20,27 | Multi-output runs, narratives, context-safe reuse and retention |
| 29 | [Reports/dashboards](phase-29-reports-dashboards.md) | 18,28 | Hybrid widgets, exact pages, filters, private review, partial outcomes |
| 30 | [Reporting schedules](phase-30-reporting-schedules.md) | 06,18,23,28,29 | Real targets, fresh Pengui broker authority, exact periods and delivery state |
| 31 | [MCP Apps viewer](phase-31-reporting-mcp-apps.md) | 22,23,28,29,30 | Reporting tools and shared read viewer over established Apps |
| 32 | [SSR/iframe/export](phase-32-reporting-rendering-embed.md) | 28,29,31 | BFF iframe, isolated SVG renderer and explicit safe exports |
| 33 | [Guided onboarding](phase-33-guided-onboarding.md) | 11,12,13,15,27 | Resumable connect/profile/semantic draft/review/example workflow |
| 34 | [Migration/cutover](phase-34-migration-parity-cutover.md) | 14,16,18,19,23,24,26–33 | Neutral imports, all-feature comparison, one schedule stream and rollback |

## Execution sequence and incremental delivery

Start with 01, then 02/03/05 in parallel; 04 unlocks 06/07/09 and early 21->22->23. Read adapters follow 09->08->10, avoiding an import cycle. Profiling and source inspection unlock 15/20->27->28: the first approved block with real validation, selected outputs and a retained API artifact.

Run semantic/NLQ work 16->17->18 alongside block work, then add 19/29/30->31->32. This supplies hybrid reports, dashboards, functional scheduling, the Apps viewer and genuine SSR/BFF iframe rendering. The viewer can be developed early against synthetic sealed artifacts; its phase closes only with real domain consumers.

Uploads, full driver coverage, managed pipelines and L2 proposals can proceed on their graph branches; they do not hold the first reporting API demonstration hostage. Phase 33 composes the real guided setup flow. Phase 34 closes all required source behavior and cohort migration; phase 25 closes deployment/release with every transitive dependency satisfied.

The graph is acyclic at the **core implementation** boundary. Some cumulative checks explicitly exercise later consumers: phase 12's initial profile consumer is its inspection API, with semantic integration extended in 15; phase 11's reporting check is extended when reporting lands; 16's full query replay integration closes with 18/24; early 21–23 registration suites expand with every feature. Do not create reverse package dependencies to satisfy those tests. Distinguish delivered core interfaces from full cumulative feature closure in the evidence record.

## First useful product proof

A valid Pengui JWT selects an authorized source/topic. An API caller creates a block draft, validates real SQL, publishes/certifies explicitly, executes multiple selected outputs and reads the retained result without more SQL/model calls. Wrong resource reach fails. An expired token cannot refresh itself in Chartworks. A later report publication cannot reveal a private preview.

This is a demonstrable slice, not a full replacement claim. Full cutover also requires reports/dashboards, dynamics, schedules, Apps/static delivery, source adapters and all retained semantic/NLQ workflows.

## Scope boundaries and risks

Keep one Go core, one queue, one model gateway and one direct signed-scope enforcement path. Add migrations with actual consumers. Use a bounded optional chart renderer and the accepted pipeline runner rather than recreating them or introducing a workflow platform. No standalone authoring UI is required.

The most important risks are incomplete per-dialect relation/function/read-policy enforcement; confusing definition pins with data snapshots; reuse across actual execution partitions; missing fresh Pengui authority for durable work; private-preview leakage; imprecise decimals; and external effects being called atomic/exactly-once. The owning phases contain direct assertions for each.

L2 reviewed engineering remains included. L3 auto-apply, a new internal analyst, arbitrary federation, PDF/PNG document layout and event-driven scheduling extensions are not requirements hidden inside this migration. Existing source replay/multi-topic/dynamic-report behavior is not deferred under those labels.

## Completion and evidence

Run `make planning-check` for document/registry coherence, `make preflight-full` for cumulative implemented phase checks, and `make release-check` for the strict all-phase runtime gate. `CHARTWORKS_ALLOW_PLANNED_SKIP=1` only permits clearly labeled unimplemented-phase skips in documentation/development preflight; release mode ignores it. Every new code phase must leave planned state and supply its acceptance tests before claiming implementation.

The 63 feature IDs originate in brief 14. `coverage.json` retains every row: 62 required and Q11 deliberately discarded stubs. Forty review gate IDs are retained with corrected meaning; G27 tests Chartworks Apps behavior, G28 tests the BFF authority boundary, and G24 obeys the expiry-bounded JWT model. Definitions and real test evidence, not row counts, determine completeness.
