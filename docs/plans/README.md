# Chartworks — actionable implementation plan

Revised 2026-09-04 after PR #2 and the owner's authentication, Apps and remote-inference directives. The active RFCs and numbered plans replace the historical plans preserved in `docs/archive/phase0-plans/`. This is an implementation baseline, not a claim that Go capabilities already exist.

## Fixed decisions

Pengui alone owns issuer/authentication/access-policy decisions. Chartworks validates Pengui JWTs and enforces signed scopes and resource restrictions. No local users, roles, grants, API keys, service identities, bootstrap admin, OAuth server or embed credential issuer. [The authority contract](../contracts/pengui-authority.md) separates the integration schema from already implemented platform behavior.

Harbor/Pengui MCP Apps support is established. Implement Chartworks's tools/resources/viewer; no host qualification, framework selection or unrelated protocol migration. Use the existing Pengui/client BFF for iframe credentials.

All production completion, structured generation, embeddings and reranking use the embedded Bifrost Go SDK and remote providers. No local learned models, weights/downloads, cross-encoder service or parallel direct model client. [The gateway contract](../contracts/model-gateway.md) and [reference excerpt](../../examples/chartworks.gateway.json) are binding inputs to phase 05. Local deterministic tokenization, SQL parsing, pgvector search and rendering remain normal application work.

Keep functional cron/interval/manual scheduling and actual pipeline/saved-query/block/report targets. Discard source event/condition/condition-check/custom-code stubs. Preserve business validation, immutable revisions, certification, SQL safety, actual source data partitions, retention and execution budgets: those remain Chartworks domain responsibilities, not a second IAM system.

## Working from the plans

Read RFC-001 for the shared architecture, RFC-002 for reporting, [COMMON.md](COMMON.md), then the owning phase. Each phase names its packages, hard dependencies, tasks, configuration/persistence, non-goals and individually testable acceptance criteria. `phase-registry.json` supplies the dependency/status/count ledger. `coverage.json` maps all 63 source-feature IDs and 41 review gates to phase criteria; neither file is test evidence.

There are **34 planned phases and 224 acceptance criteria**: phase 05 has ten, the other original phases have six each, and phases 27–34 have eight each. Each criterion requires a real `TestPhaseNN/ACxx` result. Missing/skipped/empty tests cannot count as implementation. All phase statuses start planned.

Phase numbers identify workstreams, not chronology. Phases 21–23 provide early transport/client shells; domain phases register concrete operations as they land. Phase 25 is the final release gate. An unbuilt operation is absent rather than a success-returning placeholder.

## Dependency and ownership table

| Phase | Plan | Hard dependencies | Deliverable |
|---|---|---|---|
| 01 | [Binary/config/telemetry](phase-01-binary-config-telemetry.md) | none | Typed config, lifecycle/health, metrics/audit |
| 02 | [Store](phase-02-store-migrations.md) | 01 | PostgreSQL transaction/tenant/CAS/migration primitives, no IAM tables |
| 03 | [JWT/identity](phase-03-auth-identity.md) | 01 | Pengui verification-only envelope |
| 04 | [Scope enforcement](phase-04-access-grants.md) | 02,03 | Signed action/resource enforcement, no local grant policy |
| 05 | [Bifrost gateway](phase-05-gateway.md) | 01 | Remote SDK inference, independent roles, strict response/budget contracts |
| 06 | [Queue/occurrences](phase-06-jobs-scheduler.md) | 02,03,04 | Leases/fencing, occurrence idempotency, cron/interval, authority-provider seam |
| 07 | [Facets](phase-07-vindex.md) | 02,04 | pgvector generations, embedding-space identity, batched retrieval |
| 08 | [Source core](phase-08-sources-core.md) | 04,09 | PostgreSQL reader, registry/custody and actual source execution contexts |
| 09 | [SQL validation](phase-09-sql-validate-core.md) | 03,04 | Read interface/opaque validated plan and positive SQL safety |
| 10 | [Read execution](phase-10-exec-read.md) | 08,09 | Caps/cancel/reconciliation, exact normalized data |
| 11 | [Uploads](phase-11-uploads-workspace.md) | 06,08 | CSV/XLSX/Parquet workspace to ordinary governed datasets |
| 12 | [Profiling](phase-12-engineering-profiling.md) | 05,06,08,10 | Profile/quality/freshness and drift |
| 13 | [Pipelines](phase-13-engineering-pipelines.md) | 06,09,10,12 | SQL-only managed writes, checks/lineage and staged effects |
| 14 | [Warehouse drivers](phase-14-warehouse-drivers.md) | 08,09,10 | Six-engine contracts and applicable source evidence |
| 15 | [Topics](phase-15-topics-lifecycle.md) | 04,05,07,12,21 | Versioned semantics, ready-facet publication, health/portability |
| 16 | [Rules/clarification](phase-16-rules-clarification.md) | 05,15 | Rules/slots, constraints, replay/shadow |
| 17 | [Routing/context](phase-17-nlq-routing-context.md) | 05,07,15,16 | Compact budgets, remote rerank, pins, languages and confirmed joins |
| 18 | [NLQ](phase-18-nlq-generation-execution.md) | 09,10,17 | Plan/run/refine/templates/correction/learning |
| 19 | [BYO](phase-19-byo-mode.md) | 02,10,17 | Stored context references and identical submit safety |
| 20 | [Output specifications](phase-20-charts-spec.md) | 10,15 | Fourteen-kind catalog and saved mappings/formats |
| 21 | [HTTP shell](phase-21-http-api.md) | 01,02,03,04 | Early protected registration/schema/audit |
| 22 | [MCP shell](phase-22-mcp-server.md) | 21 | Established-profile tools/resources, no host qualification |
| 23 | [SDK/CLI](phase-23-sdk-cli-parity.md) | 21,22 | Typed clients, caller token provider, cumulative parity |
| 24 | [Evaluation](phase-24-eval.md) | 18,19,20,29 | Quality/adversarial/replay and remote prompt optimization |
| 25 | [Final release](phase-25-e2e-release.md) | 24,26,34 | Cumulative capability/engine/operational closure |
| 26 | [L2 engineering](phase-26-engineering-autopilot.md) | 13,15,16,21,27 | Reviewed proposals/drift amendments and honest compensation |
| 27 | [Governed blocks](phase-27-reporting-blocks.md) | 15,20,21 | Draft/revision/validation/certification/parameters/impact |
| 28 | [Frozen runs/artifacts](phase-28-reporting-execution-artifacts.md) | 05,06,10,20,27 | Selected outputs, narratives, context-safe reuse and retention |
| 29 | [Reports/dashboards](phase-29-reports-dashboards.md) | 18,28 | Hybrid widgets, pages, filters, private review and partial outcomes |
| 30 | [Reporting schedules](phase-30-reporting-schedules.md) | 06,18,23,28,29 | Real targets, fresh Pengui authority, exact periods and delivery state |
| 31 | [MCP Apps viewer](phase-31-reporting-mcp-apps.md) | 22,23,28,29 | Reporting tools/shared viewer independent of schedule delivery |
| 32 | [SSR/iframe/export](phase-32-reporting-rendering-embed.md) | 28,29,31 | BFF iframe, isolated SVG rendering, explicit safe exports |
| 33 | [Guided onboarding](phase-33-guided-onboarding.md) | 11,12,13,15,27 | Resumable setup/profile/semantic draft/review workflow |
| 34 | [Migration/cutover](phase-34-migration-parity-cutover.md) | 14,16,18,19,23,24,26–33 | Neutral import, complete parity, schedule handoff and rollback |

## Delivery sequence

Start 01, then 02/03/05 in parallel. Phase 04 unlocks 06/07/09 and early 21->22->23. Source interfaces/adapters follow 09->08->10 to avoid the old import-cycle ambiguity. Profiling/semantics unlock 15/20->27->28: a real approved block, selected outputs and a retained API artifact.

Run semantic/NLQ work 16->17->18 alongside block work, then add 19/29. Reports unlock **30 and 31 in parallel**: scheduling is not a prerequisite for the viewer. Static delivery follows 31->32. The viewer can be developed against synthetic sealed artifacts earlier; closure requires its actual domain consumer.

Uploads, complete driver coverage, pipelines and L2 proposals proceed on their own branches. They do not hold the first reporting demonstration hostage. Phase 33 composes guided onboarding; phase 34 closes every required source capability and cohort migration; phase 25 closes release after all transitive dependencies. L3 and a new internal analyst do not gate this migration.

Core dependencies are acyclic. Cumulative tests extend earlier services as later consumers arrive without reverse imports: profiling initially serves its inspection API, semantic use lands in15; rule replay's full query integration closes in18/24; early transport parity expands per feature. Phase05 is a build-time dependency of28 because narrative support uses its interface; a frozen runtime operation without narrative must still make zero SDK/provider calls.

## First useful product proof

A Pengui JWT selects an authorized source/topic. Create a block draft, validate real SQL, publish/certify explicitly, run multiple selected outputs and read the retained result without another SQL/model call. Wrong resource reach fails; expired tokens cannot renew themselves in Chartworks; a later publication cannot reveal a private preview. This is a usable slice, not a complete replacement claim.

Complete cutover additionally requires the mapped semantics/NLQ/learning behavior, hybrid reports, dashboards, schedules, Apps/static delivery and all required source adapters. The 63 feature rows remain: 62 required, Q11 deliberately discarded stubs. G27 tests Chartworks Apps code, G28 tests the BFF boundary, G24 respects expiry-bounded JWT freshness, and G41 enforces Bifrost-only remote inference.

## Verification and remaining integration work

`make planning-check` validates this graph, criteria, links, mappings, mirrored rules and gateway configuration excerpt. `make preflight-full` adds actual acceptance tests for implemented phases and reports planned phases as unimplemented SKIPs. `make release-check` requires every phase shipped and every expected test passed with no SKIPs; statuses alone are not proof.

The Pengui resource-scope serialization and fresh scheduled-authority adapter must be wired to the platform's actual contract during phases03/04/30. This is a Pengui-owned integration change where needed, not permission to implement a local issuer or guess a platform endpoint. Current source/host/model availability is not claimed from this documentation review.

The standing risks are per-dialect safety, unsafe cross-context artifact reuse, reference/window drift on retries, private-preview leakage, numeric precision and overstated external atomicity. Their owning phase tests remain required. D-044–D-054 record ownership and scope; D-043 remains reserved for the earlier separate provider-role history.
