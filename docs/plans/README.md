# Chartworks — actionable implementation plan

Current implementation status: phases 01–12 are shipped. Phase 13 has implementation submitted for acceptance and remains in progress; twenty-one later workstreams remain planned. The registry records status; actual named tests and reviewed execution evidence establish acceptance, not this paragraph or a green documentation check. Historical superseded plans remain under `docs/archive/phase0-plans/`.

## Fixed decisions

Pengui alone owns issuer/authentication/access-policy decisions. Chartworks validates Pengui JWTs and enforces their signed scopes and resource restrictions. No local users, roles, grants, API keys, service identities, bootstrap admin, OAuth server or embed credential issuer. [The authority contract](../contracts/pengui-authority.md) and [implemented provider handoff](../contracts/pengui-provider-registration.md) pin the actual issuer shape, bounds and protected operations.

Harbor/Pengui MCP Apps support is established. Implement Chartworks tools/resources/viewer; no host qualification, framework selection or unrelated protocol migration. Use the existing Pengui/client BFF for iframe credentials.

All production completion, structured generation, embeddings and reranking use the embedded Bifrost Go SDK and remote providers. No local learned models, weights/downloads, cross-encoder service or parallel direct model client. [The gateway contract](../contracts/model-gateway.md) and [reference excerpt](../../examples/chartworks.gateway.json) are binding phase 05 inputs. Deterministic tokenization, SQL parsing, pgvector search and rendering remain normal application work. Constructing the gateway performs no inference; the explicitly authorized operator probe performs paid remote inference when invoked.

Keep functional cron/interval/manual scheduling and actual pipeline/saved-query/block/report targets. Discard source event/condition/condition-check/custom-code stubs. Business validation, immutable revisions, certification, SQL safety, source data partitions, retention and execution budgets remain Chartworks responsibilities, not a second IAM system.

## Working from the plans

Read RFC-001, RFC-002, [COMMON.md](COMMON.md), then the owning phase. Each phase names packages, dependencies, concrete tasks, configuration/persistence, non-goals and individually testable criteria. `phase-registry.json` supplies the dependency/status/count ledger; `coverage.json` maps all source features and review gates to criteria. Neither file is runtime evidence.

There are **34 workstreams and 224 acceptance criteria**: phase 05 has ten, the other original phases have six each, and phases27–34 have eight each. Phases01–12 are shipped, phase13 is in progress and the remaining twenty-one are planned. Each implemented criterion requires its real `TestPhaseNN/ACxx` result. Missing/skipped/empty tests cannot count as success.

Numbers identify workstreams, not chronology. Phases21–23 extend the early transport/client registration seams; domain phases add concrete operations as they land. The six operational routes and matching SDK methods introduced in phases03/04 are real first consumers, not a claim that the later full HTTP/MCP/client phases are finished. Phase25 is the final release gate.

## Dependency and ownership table

| Phase | Plan | Hard dependencies | Deliverable |
|---|---|---|---|
| 01 | [Binary/config/telemetry](phase-01-binary-config-telemetry.md) | none | Typed config, lifecycle/health, metrics/audit |
| 02 | [Store](phase-02-store-migrations.md) | 01 | PostgreSQL transaction/tenant/CAS/migration primitives, no IAM tables |
| 03 | [JWT/identity](phase-03-auth-identity.md) | 01 | Pengui verification-only envelope and shared key cache |
| 04 | [Scope enforcement](phase-04-access-grants.md) | 02,03 | Signed action/resource enforcement and actual operational consumers |
| 05 | [Bifrost gateway](phase-05-gateway.md) | 01 | Remote SDK inference, independent roles, strict response/budget contracts |
| 06 | [Queue/occurrences](phase-06-jobs-scheduler.md) | 02,03,04 | Leases/fencing, occurrences and actual fresh-authority adapter/first durable consumer |
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
| 21 | [HTTP shell](phase-21-http-api.md) | 01,02,03,04 | Protected registration/schema/audit across actual domain operations |
| 22 | [MCP shell](phase-22-mcp-server.md) | 21 | Established-profile tools/resources, no host qualification |
| 23 | [SDK/CLI](phase-23-sdk-cli-parity.md) | 21,22 | Typed clients, caller token provider, cumulative parity |
| 24 | [Evaluation](phase-24-eval.md) | 18,19,20,29 | Quality/adversarial/replay and remote prompt optimization |
| 25 | [Final release](phase-25-e2e-release.md) | 24,26,34 | Cumulative capability/engine/operational closure |
| 26 | [L2 engineering](phase-26-engineering-autopilot.md) | 13,15,16,21,27 | Reviewed proposals/drift amendments and honest compensation |
| 27 | [Governed blocks](phase-27-reporting-blocks.md) | 15,20,21 | Draft/revision/validation/certification/parameters/impact |
| 28 | [Frozen runs/artifacts](phase-28-reporting-execution-artifacts.md) | 05,06,10,20,27 | Selected outputs, narratives, context-safe reuse and retention |
| 29 | [Reports/dashboards](phase-29-reports-dashboards.md) | 18,28 | Hybrid widgets, pages, filters, private review and partial outcomes |
| 30 | [Reporting schedules](phase-30-reporting-schedules.md) | 06,18,23,28,29 | Real targets, reused Pengui authority adapter, exact periods and delivery state |
| 31 | [MCP Apps viewer](phase-31-reporting-mcp-apps.md) | 22,23,28,29 | Reporting tools/shared viewer independent of schedule delivery |
| 32 | [SSR/iframe/export](phase-32-reporting-rendering-embed.md) | 28,29,31 | BFF iframe, isolated SVG rendering, explicit safe exports |
| 33 | [Guided onboarding](phase-33-guided-onboarding.md) | 11,12,13,15,27 | Resumable setup/profile/semantic draft/review workflow |
| 34 | [Migration/cutover](phase-34-migration-parity-cutover.md) | 14,16,18,19,23,24,26–33 | Neutral import, parity, schedule handoff and rollback |

## Delivery sequence

Phases01–09 now supply the foundation, gateway, queue, vector generations and qualified source/validation core. Phase04 unlocks07/09 and the expanded21->22->23 surfaces. Source interfaces/adapters follow09->08->10 to avoid import cycles. Profiling/semantics unlock15/20->27->28: a real approved block, selected outputs and retained API artifact.

Run semantic/NLQ work16->17->18 alongside block work, then add19/29. Reports unlock30 and31 in parallel; scheduling is not a viewer prerequisite. Static delivery follows31->32. Viewer development may use synthetic sealed artifacts earlier, but closure requires the actual consumer.

Uploads, full driver coverage, pipelines and L2 proposals proceed on their own dependencies. They do not delay the first reporting demonstration unnecessarily. Phase33 composes onboarding,34 closes every required capability/cohort, and25 closes release. L3 and a new internal analyst do not gate this migration.

Core dependencies are acyclic. Cumulative tests extend earlier services without reverse imports. The phase 05 build-time dependency of28 supports optional narrative interfaces; frozen runtime operations without narrative still make zero SDK/provider calls. Retained artifact reads do not require a live warehouse or model provider.

## First useful product proof

A Pengui JWT selects an authorized source/topic. Create a block draft, validate real SQL, publish/certify explicitly, run selected outputs and read the retained result without another SQL/model call. Wrong reach fails, expired authority cannot renew itself locally, and later publication cannot reveal a private preview. This is a useful slice, not a complete replacement claim.

Complete cutover also requires semantic/NLQ/learning behavior, hybrid reports, dashboards, schedules, Apps/static delivery and required adapters. All 63 feature rows remain: 62 required and Q11 deliberately discarded stubs. G27 tests Chartworks Apps code, G28 the BFF boundary, G24 expiry-bounded authority, and G41 Bifrost-only inference.

## Verification and remaining integration work

`make planning-check` checks graph/criteria/links/mappings/mirrors/configuration. `make preflight-full` runs actual implemented acceptance and reports later planned phases as unimplemented. `make release-check` requires every phase shipped and every criterion passed without skips.

The existing Pengui provider bearer serialization is now consumed by phases03/04. [Its operation manifest](../contracts/chartworks-operations.json) and [registration guide](../contracts/pengui-provider-registration.md) let the operator configure approved scope sets; no production registration is fabricated. Phase06 implements the [Pengui-owned execution authority v1 extension](../contracts/execution-authority-v1.md) with its real maintenance consumer. Merge/deploy the companion issuer before enabling dispatch; Phase30 reuses the same provider. No local issuer or guessed broker endpoint is permitted.

Standing risks remain dialect safety, complete dependency manifests, unsafe artifact-context reuse, retry reference/window drift, private previews, exact numeric values and overstated external atomicity. Their later real-adapter tests remain required. D-059–D-061 record the implemented authority decisions without weakening those obligations.

Read execution now extends the merged phase-09 validator on the existing source/store seams. See [D-065](../contracts/read-execution.md) for exact typed results, bounded attempts and cancellation/reconciliation. Final named acceptance and read-only CI establish readiness, not this status paragraph.
