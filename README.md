# Chartworks

**Governed analytics and publishing for Pengui.** A Go service for controlled data access, reviewed semantic topics, natural-language and external-agent SQL, and portable output specifications. Governed reporting and rendering build on these foundations; they are not all implemented yet.

## Current status

**The merged baseline through phase 29 implements 172 named acceptance criteria.**
Phases **30 and 31 add sixteen criteria for reporting schedules and the MCP Apps
read viewer**, bringing the implemented/in-progress inventory to 188 across 29 of
34 workstreams. The phase registry retains `in_progress` for review submissions;
actual named tests and exact-source CI, not that status label, establish readiness.
The [delivery contract](docs/contracts/reporting-delivery-v1.md) and
[adversarial evidence](docs/reviews/phase-30-31-adversarial.md) describe this change.

**Five workstreams remain planned: 24–25 and 32–34**, including evaluation,
static rendering/BFF exports, onboarding, migration and the final release gate.
There are 224 criteria in the complete plan. Catalog pull delivery is implemented;
no outbound notification service, static-export implementation, live-provider
qualification or production-cutover claim is implied.

| Capability | Implemented boundary |
|---|---|
| Foundation, authority and operations | Strict configuration, health/readiness, PostgreSQL migrations, telemetry, Pengui JWT verification, signed action/resource enforcement, protected maintenance and audit reads. |
| Gateway and durable work | One embedded Bifrost SDK with remote-provider roles and bounded budgets; leased operations, fencing, cancellation and cron/interval/manual occurrences using fresh Pengui-issued execution authority. |
| Sources and safe execution | Versioned source contexts, positive SQL validation, opaque executable plans, exact typed results, read caps, cancellation and uncertainty reconciliation. PostgreSQL is qualified directly; the six-engine matrix records narrower per-driver evidence. |
| Uploads, profiling and engineering | CSV/XLSX/Parquet managed workspace ingestion, deterministic profiles and drift evidence, reviewed SQL-only managed pipelines and staged external effects. |
| Semantics and natural-language queries | Versioned private topic drafts and publication, canonical meaning, context-local facets, rules/clarification, compact authorized routing, plan/run/refine/templates, bounded correction and reviewed learning. |
| External-agent SQL | Opaque expiring context references, exact semantic/source pins, reauthorization, bounded idempotent submission steps and content-free receipts. Context access alone cannot execute SQL. |
| Output specifications | All fourteen kinds, deterministic selection and explicit bindings, portable metadata/format hints, exact labels/totals, saved-schema validation and review-only rebinding. No warehouse requery or chart-state store. |
| HTTP — cumulative phase 21 | Implemented operations register schemas, action/resource and audit/effect metadata; OpenAPI comes from that registry. A runtime guard rejects unregistered paths before handler dispatch and selects the declared HTTP/MCP intended audience. |
| SDK/CLI — phase 23 | Existing typed clients plus registry-driven HTTP calls, authenticated in-process transport, explicit caller token providers, bounded safe replay and injected CLI I/O. No local issuer or future reporting stubs. |
| Governed blocks — phase 27 | SQL-private metadata, scoped SQL inspection, CAS revisions, exact read-backed validation, immutable publications/attestations, nine parameter types, private multi-output previews, localized question discovery and review-only dependency/period proposals. No scheduled refresh or narrative execution. |
| Reviewed engineering — phase 26 | Versioned change proposals, validation and impact review, exact approved promotion and schedule-change receipts; not arbitrary code execution. |
| Frozen reporting runs — phase 28 | Bounded selected-output execution, optional grounded narrative, durable results and read receipts, explicit recovery, artifact privacy and retention. |
| Reports/dashboards — phase 29 | Independent revision/review/publication pointers, exact report pages, typed business filters, shared-block output fan-out, opt-in dynamic query durability, partial outcomes and metadata-only artifact summaries. |
| Reporting schedules — phase 30 | Four real target types on the shared queue; fresh Pengui authority, immutable due/window/revision pins, CAS lifecycle/history, fenced budgets and independent artifact/catalog delivery. No local issuer or fabricated notification. |
| MCP Apps — phases 22/31 | Eighteen existing tools plus five reporting tools when installed; a versioned read viewer, fourteen visual kinds, typed filter reruns, exact retained paging, theme/locale and safe structured/text fallback. No browser credentials or separate scheduler. |

## Ownership and security

**Pengui is the sole issuer, authentication and access-policy owner.** Chartworks verifies current Pengui JWTs and enforces their signed scopes. It does not create users, roles, API keys, login/OAuth flows, local token renewal or embed credentials. Warehouse secrets are a separate source concern. See the [authority contract](docs/contracts/pengui-authority.md) and [operator registration guide](docs/contracts/pengui-provider-registration.md).

Harbor/Pengui MCP Apps support is established by the owner. The Chartworks retained-artifact viewer uses that ecosystem; static rendering and exports remain phase 32. Iframe credentials belong in the Pengui/client BFF, not the public Apps resource.

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

Real database, native-runner and source fixtures are required for their tests. Missing dependencies are failures, not evidence of passing integration. CI tests committed source without repair scripts. Planning checks and the status registry are bookkeeping, not runtime proof. The [CI verification lanes](docs/ci-verification.md) distinguish fast pull-request feedback from the manually dispatched full final-gap/release qualification; a green fast lane does not imply the full suite passed.

## MCP tools and metadata resources

Enable `features.mcp` to mount **POST `/v1/mcp`** on the existing server. The
[configuration excerpt](examples/chartworks.mcp.json) and [MCP contract](docs/contracts/mcp-v1.md)
define exact hosts/origins, request/response/concurrency limits and operator setup.
Each request requires a fresh Pengui bearer for the MCP intended audience and
`mcp.use`; each operation then enforces its ordinary domain action and exact reach.
The transport is stateless and returns JSON, with no bearer persistence or separate
credential channel. HTTP and in-process clients use the same guarded dispatch.

The eleven core tools list/describe topics and datasets; preflight/plan/run/refine
questions; obtain context/submit SQL; and submit feedback. Source listing, retained
context lookup and five chart-specification tools bring the installed inventory to
eighteen. The reporting group adds `reporting_search`, `reporting_describe`, `reporting_run`, `reporting_runs` and `reporting_view`. Paid or persisted calls are annotated accordingly: preflight is **not** a
pure metadata read. Disabled or unbuilt services are absent, never success stubs.

The chart catalog, published topic and registered dataset metadata can also be read
as three resource bindings through the same services, without source/model calls.
These three resources remain JSON metadata. The reporting group additionally supplies the versioned credential-free Apps HTML resource and authorized retained-artifact tools.
Phase 21 and the typed Go SDK additionally expose `POST /v1/topics/list`,
`POST /v1/datasets/list` and `POST /v1/datasets/describe`. Their database queries apply
all signed dependency/context restrictions before pagination, not after broad reads.

## SDK and operator CLI

Applications supply a current Pengui token provider to `sdk/chartworks.New` and
use the existing typed domain methods. `NewInProcessWithOptions` uses the same
production authenticated handler without opening a listener. `Operations`,
`OperationMatrix` and `Invoke` expose only installed public contracts.

```bash
# The application/Pengui integration already supplies CHARTWORKS_TOKEN.
# Never place a literal bearer in shell arguments or history.
chartworks client config --url https://analytics.example/warehouse
chartworks client operations --url https://analytics.example/warehouse
chartworks client diagnostics --url https://analytics.example/warehouse
```

Generic calls require `--execute` and take JSON or upload bytes from stdin, not
SQL/token command arguments. They do not silently retry denied, expired or missing
objects. See the [complete client and CLI examples](docs/contracts/clients-v1.md)
for response/status handling, stable operation keys and separate HTTP/MCP tokens.

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

Still planned: evaluation and release gates, static rendering/BFF embeds/exports,
guided onboarding and migration/cutover. Schedule catalog publication does not
mean an email was sent; retained artifact viewing does not execute a new query.
See the [runtime delivery guide](docs/contracts/reporting-delivery-v1.md) for
operator setup, target/lifecycle semantics, API/SDK methods and tested boundaries.

Useful evidence includes the [warehouse-driver matrix](docs/contracts/warehouse-drivers.md), [managed-pipeline contract](docs/contracts/managed-pipelines.md), [read-execution contract](docs/contracts/read-execution.md), [semantic/NLQ delivery record](docs/reviews/phase-15-18-current-evidence.md) and [phase-20/21 adversarial review](docs/reviews/phase-20-21-adversarial.md). Phase 22 adds its own [MCP contract](docs/contracts/mcp-v1.md) and [review record](docs/reviews/phase-22-adversarial.md); no deployment or full replacement qualification is implied.
