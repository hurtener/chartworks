# Chartworks

**Governed analytics and publishing for Pengui.** A Go service for controlled data access, reviewed semantic topics, natural-language and external-agent SQL, and portable output specifications. Governed reporting and rendering build on these foundations; they are not all implemented yet.

## Current status

**Merged baseline: phases 01–19 and the phase-21 HTTP prerequisite are implemented** — 20 of 34 workstreams, with 124 named acceptance criteria. PRs #11 and #12 are merged. The latest baseline qualification is [CI 34284686569](https://github.com/hurtener/chartworks/actions/runs/34284686569), covering the phase-19 head whose tree is preserved by merge `99055d4fd5f7a613a9518f3b2b3e4e564301eb5c`.

**This change implements phase 20 and extends phase 21 for review.** It adds fourteen provider-neutral output kinds, exact-value labels, saved bindings, optional bounded authoring rank assistance, five protected HTTP/Go SDK operations, and executable registration guards. Phase 20 remains `in_progress` until review and required verification close; implementation is not a release claim. See the [current review and verification record](docs/reviews/phase-20-21-adversarial.md).

With phase 20 under review, **13 workstreams remain planned**: 22–34, including the phase-25 final release gate. There are 224 acceptance criteria across the full plan. Recorded cloud and model fixtures do not constitute live-provider or production-cutover qualification.

| Capability | Implemented boundary |
|---|---|
| Foundation, authority and operations | Strict configuration, health/readiness, PostgreSQL migrations, telemetry, Pengui JWT verification, signed action/resource enforcement, protected maintenance and audit reads. |
| Gateway and durable work | One embedded Bifrost SDK with remote-provider roles and bounded budgets; leased operations, fencing, cancellation and cron/interval/manual occurrences using fresh Pengui-issued execution authority. |
| Sources and safe execution | Versioned source contexts, positive SQL validation, opaque executable plans, exact typed results, read caps, cancellation and uncertainty reconciliation. PostgreSQL is qualified directly; the six-engine matrix records narrower per-driver evidence. |
| Uploads, profiling and engineering | CSV/XLSX/Parquet managed workspace ingestion, deterministic profiles and drift evidence, reviewed SQL-only managed pipelines and staged external effects. |
| Semantics and natural-language queries | Versioned private topic drafts and publication, canonical meaning, context-local facets, rules/clarification, compact authorized routing, plan/run/refine/templates, bounded correction and reviewed learning. |
| External-agent SQL | Opaque expiring context references, exact semantic/source pins, reauthorization, bounded idempotent submission steps and content-free receipts. Context access alone cannot execute SQL. |
| Output specifications — this change | All fourteen kinds, deterministic selection and explicit bindings, portable metadata/format hints, exact labels/totals, saved-schema validation and review-only rebinding. No warehouse requery or chart-state store. |
| HTTP — cumulative phase 21 | Implemented operations register schemas, action/resource and audit/effect metadata; OpenAPI comes from that registry. A runtime guard rejects unregistered paths before handler dispatch. |

## Ownership and security

**Pengui is the sole issuer, authentication and access-policy owner.** Chartworks verifies current Pengui JWTs and enforces their signed scopes. It does not create users, roles, API keys, login/OAuth flows, local token renewal or embed credentials. Warehouse secrets are a separate source concern. See the [authority contract](docs/contracts/pengui-authority.md) and [operator registration guide](docs/contracts/pengui-provider-registration.md).

Harbor/Pengui MCP Apps support is established by the owner. The later Chartworks MCP tools, retained-artifact viewer and exports are implementation work, not a reason to reopen host compatibility. Iframe credentials belong in the Pengui/client BFF.

**Production inference uses the embedded Bifrost Go SDK and remote providers only.** There is no local learned model, weight download or alternate direct model client. Optional inference is explicit and budgeted. Retained metadata, deterministic output building and other model-free operations remain useful with providers disabled. See the [gateway contract](docs/contracts/model-gateway.md).

## Start and verify

Use [GETTING-STARTED.md](GETTING-STARTED.md) for the reference build, PostgreSQL, native parser/managed runner, trusted issuer configuration and operation examples. The reference deployment includes its pinned native dependencies; an ordinary CGo-free build does not describe the complete current binary.

```bash
make planning-check  # Plans, dependency graph, links, configuration and checker tests.
make build           # Requires the documented pinned native build inputs.
make vet
make coverage        # Full race-enabled suite, real fixtures and package coverage bands.
make preflight-full  # Named implemented-phase acceptance; planned phases explicitly skip.
make release-check   # Final gate: every phase shipped, all criteria pass, no skips.
```

Real database, native-runner and source fixtures are required for their tests. Missing dependencies are failures, not evidence of passing integration. CI tests committed source without repair scripts. Planning checks and the status registry are bookkeeping, not runtime proof.

## Portable output specifications

The catalog is **area, bar, column, donut, grouped bar, heatmap, KPI card, line, pie, scatter, stacked bar, stacked column, table and treemap**. Every kind has its own slot requirements and empty/null/negative/order behavior; a table fallback does not count as another chart kind.

Five operations use the same service from HTTP and the public Go SDK:

| HTTP | Go client | Signed action |
|---|---|---|
| `GET /v1/charts/catalog` | `ChartCatalog` | `charts.read` |
| `POST /v1/charts/select` | `SelectChart` | `charts.select` |
| `POST /v1/charts/specify` | `SpecifyChart` | `charts.bind` |
| `POST /v1/charts/build` | `BuildChart` | `charts.bind` |
| `POST /v1/charts/rebind` | `RebindChart` | `charts.bind` |

Each also requires current signed `cw.tenant.read:<tenant>` reach. The SDK receives a caller-supplied token provider; Chartworks issues no credentials. [The operation manifest](docs/contracts/chartworks-chart-operations.json) and [version-one contract](docs/contracts/chart-specifications-v1.md) describe schemas, errors and authority precisely.

`ChartDataFromReadResult` converts an already returned, qualified read result without another source call. Integers and decimals remain exact strings; geometry may explicitly approximate them. Truncated totals are labeled as totals of returned rows, never full-source totals. Units, currency, percent basis, grain, aggregation and versioned provenance travel separately from display geometry.

Saved mappings pin compatible column metadata and chosen outputs. Building one does not select a different chart or call a model. Schema drift fails; an unambiguous semantic rebind returns a `review_required` proposal rather than editing an approved definition. Optional rank assistance is author-requested, disabled by default and restricted to the already suitable candidates. It cannot invent SQL or bindings. [Configuration](examples/chartworks.charts.json) bounds input size, categories, series, alternatives, concurrency, options and gateway work.

These endpoints transform **caller-supplied data**. They do not certify its provenance or confer access to any source, topic or retained artifact. The output is sealed typed rendering input, **not a rendered image**. Static rendering, artifact privacy/retention and block-revision persistence remain with their later owning phases.

## Semantic queries and external agents

Reviewed semantic publications, rules and authorized compact context feed the existing validator/read executor. SQL parsing alone is not a safety proof. Executable plans remain bound to actual source/context restrictions, dependencies and parameters; neither NLQ nor external SQL gets a weaker execution path.

Phase 19 supports caller-driven multi-step analysis through opaque context references and explicit SQL submission. Lookup and submission recheck current authority and exact pins. Retrying an accepted step returns its content-free receipt without rerunning SQL or pretending that result values were retained. It is not a second agent loop or result cache. See [BYO SQL](docs/contracts/byo-sql.md), [configuration](examples/chartworks.byo.json) and [phase-19 evidence](docs/reviews/phase-19-byo-mode.md).

Durable operations obtain fresh Pengui authority through the [execution-authority companion contract](docs/contracts/execution-authority-v1.md). Enable dispatch only with that real Pengui integration deployed. Chartworks never persists a user's token to replay later or signs a replacement locally.

## Roadmap and evidence

The [actionable master plan](docs/plans/README.md) owns sequencing, dependencies and status. [RFC-001](RFC-001-Chartworks.md), [RFC-002](RFC-002-Governed-Reporting.md), [COMMON.md](docs/plans/COMMON.md) and [AGENTS.md](AGENTS.md) define implementation obligations. Historical plans under `docs/archive/` are not current instructions.

Still planned: the full MCP/CLI surface, evaluation and release gates, reviewed L2 engineering, governed blocks, frozen reporting runs and retained artifacts, reports/dashboards, reporting schedules, Apps viewer, static rendering/BFF embeds/exports, guided onboarding and migration/cutover. The completed queue is not a claim that reporting schedules already exist; output specs are not a claim that reports or renderers are shipped.

Useful evidence includes the [warehouse-driver matrix](docs/contracts/warehouse-drivers.md), [managed-pipeline contract](docs/contracts/managed-pipelines.md), [read-execution contract](docs/contracts/read-execution.md), [semantic/NLQ delivery record](docs/reviews/phase-15-18-current-evidence.md) and [phase-20/21 adversarial review](docs/reviews/phase-20-21-adversarial.md). No merge, deployment or full replacement qualification is implied by this branch.
