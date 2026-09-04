# RFC-001 — Chartworks execution baseline

> Status: implementation design, revised 2026-09-04 following owner feedback after PR #2.
> This file and RFC-002 replace the superseded planning instructions preserved in `docs/archive/`.
> Authority: RFC-001 for shared architecture/security; RFC-002 for reporting details; active phase plans for implementation; master plan; contributor rules; research. Decision history is append-only across `docs/decisions.md` and `docs/decisions/*.md`.
> Design acceptance is not implementation completion. All 34 phases are initially planned.

## 1. Product and ownership

Chartworks is Pengui's Go-native structured analytics and governed publishing capability: connect/upload -> profile/model -> explore -> approve reusable blocks -> compose reports/dashboards -> execute/schedule -> retain and render results. Direct-source querying is first-class; a medallion pipeline is not a prerequisite for a useful question or report.

Pengui owns authentication, the issuer, users, service identities, sharing, entitlements, authority issuance/renewal and revocation decisions. Chartworks verifies Pengui JWTs and applies their signed scopes and restrictions. It does not recreate those decisions. Independently deployed Chartworks still requires Pengui-issued authority; there is no alternative local-login mode.

Harbor and Pengui already support MCP Apps end to end, confirmed by the owner. No compatibility research, host qualification project, framework-adoption checkpoint or protocol upgrade is required. Chartworks builds its own tools/resources/viewer against that established capability and tests its own behavior.

Chartworks owns source and semantic metadata, safe data execution, business validation/publication/certification records, reporting definitions, operations, artifacts and its scheduler. Pengui owns product UI, identity and access decisions; Harbor owns agent orchestration/sessions. Other services are consumed through public seams, never through their internal databases. Soundings/Stowage can assist authoring but are not prerequisites for frozen refresh.

## 2. Vocabulary and invariants

A topic is a versioned semantic contract. A block is a reusable approved query plus parameters/dependencies/outputs. A report composes frozen, explicitly dynamic and text widgets. A dashboard orders exact report-revision pages. A run is an accepted logical operation with a resolved manifest and attempts. An artifact is its retained result, not a URL that reruns it.

P1: signed authority is enforced before data access; SQL is untrusted; reads and managed writes are structurally separate. P2: Pengui alone issues identity/authority; no body/header identity override. P3: tenant restrictions are mandatory in store and source access. P4: typed errors and observable bounded fallback, never silent authority expansion. P5: one model gateway. P6: clear domain vocabulary; protocol-standard field names and technical evidence fields remain legitimate. P7: one core service per capability, thin surfaces.

Publication, certification, current dependency health and data authorization are separate. Successful execution is not certification. Private previews do not become public when a later revision is published. Business correctness checks, SQL safety, retention and budgets remain Chartworks responsibilities; they are not a second identity/access policy system.

## 3. Architecture

One Go application with HTTP/MCP handlers, a PostgreSQL leased queue, domain services and bounded workers. Optional subprocesses are the existing managed-pipeline runner and static chart renderer. No broker/message bus/workflow engine is required.

Packages: `config`, `telemetry`, `identity`, `auth` (verification only), `access` (signed-scope enforcement only), `store`, `gateway`, `vindex`, `sources`, `exec`, `engineering`, `semantics`, `nlq`, `charts`, `reporting`, `rendering`, `jobs`, `api`, `mcpserver`; public SDK under `sdk/chartworks`.

Keep the dependency graph acyclic. `exec` owns the validated-plan type and read-adapter interface; concrete source drivers implement it. `exec` does not import their registry. The composition root supplies drivers/catalog dependencies. An opaque plan's zero value is rejected; neither clients nor unrelated packages can construct an executable plan. Domain services consume the frozen identity envelope on every entry point, including in-process SDK calls.

HTTP/MCP/SDK foundations land early (phases 21–23); each subsequent domain phase registers its concrete operations with them in the same change. Do not postpone all user-visible work until engineering/autonomy is complete. Do not advertise unimplemented operations.

## 4. Pengui-issued identity only

The one path is `Authorization: Bearer` -> configured asymmetric verifier/JWKS -> verified immutable envelope -> scope enforcement -> domain service. Validate allowed algorithms, key/algorithm binding, configured issuer, exact intended audience, mandatory expiration, valid temporal claims and bounded tenant/user/session/scope fields. `sub` and `user`, when both present, must agree under the Pengui contract. Do not manufacture internal identity prefixes from request fields.

The provider contract is `docs/contracts/pengui-authority.md`. Scope spelling is a Chartworks integration contract that Pengui must supply, not a claim that its present configuration already includes every new reporting scope. There is one decoder, not multiple issuer profiles or auth modes.

No signing keys, token mint/exchange endpoints, local API keys, passwords, OAuth server, local service-account provisioning, grant CRUD, role expansion or bootstrap-admin flow exists in Chartworks. JWKS loading is verification-key distribution, not identity issuance. Keys are obtained only from operator-configured trusted locations, never token-supplied URLs.

A verified JWT is a time-bounded authority snapshot. Offline validation does not promise immediate revocation of already-issued tokens. Pengui controls renewal/revocation and maximum token lifetime; Chartworks rejects expired tokens and never extends them. Each new data/artifact/rendition request needs a valid bearer. Long work must obtain fresh Pengui authority before a later privileged checkpoint when its accepted token is no longer valid. No local revocation database or policy-epoch protocol is invented.

## 5. Authorization enforcement, not policy ownership

Operation scopes and bounded resource restrictions are signed by Pengui. `access.Require` checks them against the addressed resource, tenant and registered execution context. No database query computes user memberships, sharing or grants; no `admin` string creates an implicit tenant-wide bypass. Explicit tenant-wide resource scopes can be accepted only as specified by the provider contract.

Resource lookup may load business metadata to verify ownership/reference integrity or resolve dependencies; it never grants authority. A report invocation also checks every executable dependency. A retained result requires signed read reach to the target and its recorded data partition. Pengui may authorize a reader without authoring/query-initiation privileges; artifact reading must not require a new query capability.

For queries, intersect signed source/dataset/topic reach with the published semantic allowlist and trusted source execution context before compiling/executing. Row restrictions use the existing warehouse credential/RLS/secure-view mechanism bound to that context; do not invent a general row-policy language. Where a required restriction cannot be enforced, deny instead of reading broadly and filtering in application code.

Result reuse includes actual execution context/data partition, privacy and exact request/definition semantics; it does not derive safety from a shared tenant or similar question. A context ID or audience label from a report body is not an authorization claim. Diagnostics explain checks against supplied verified authority, not hypothetical local users/roles.

## 6. Sources and secrets

Retain a source adapter seam and connection registry with source status, discovery, normalized type categories, health and non-secret read shapes. Drivers: PostgreSQL, MySQL, SQL Server, BigQuery, Snowflake, Databricks. Mock drivers are test-only; unavailable drivers return typed unsupported status and are not advertised as functional.

Warehouse credentials are distinct from user authentication. Retain the existing source-credential custody seam: prefer a reference to the platform's secret provider where available; operator-provided encrypted source credentials remain a connector concern under the existing design. Never duplicate Pengui integration tokens or implement a second integration vault. Secrets never appear in source list/read models, reports, prompts, tool results or ordinary logs. Read and engineering credentials are separate. Rotation, driver pool invalidation and connection testing are explicit.

Every engine passes discovery/type/normalization/cap/cancellation/safety conformance. PostgreSQL/MySQL/SQL Server use real container fixtures; cloud drivers require recorded fixtures plus owner-run live evidence before their release claims. Retain dependency/build constraints from the prior research as candidates, pin actual adopted versions in the implementation, and do not claim every driver is CGo-free without evidence.

## 7. Engineering, uploads and onboarding

CSV/XLSX/Parquet uploads enter a managed PostgreSQL workspace and become governed datasets through the same source/semantic/query path. The workspace is separate from metadata-store tables. Parse bounded bytes/rows/cells, classify types, neutralize executable spreadsheet content, and reject unsafe paths/decompression growth. Staging/activation/cleanup are crash-safe and tenant-scoped.

Profiles carry sampling method, normalized types, null/distinct/range summaries, value families, quality findings, freshness and observation time. Sampling limits are not a promise that a warehouse scanned no pages; expose actual adapter capabilities/cost controls. Profile and semantic generation are resumable bounded operations.

Pipelines are versioned SQL-only definitions with declared inputs, outputs, destination, quality checks and strategy. The accepted runner direction remains Bruin behind `PipelineRunner`, not a new Go orchestration engine. Preserve applicable create/replace, append, incremental/merge, interval and scd2 strategies through per-engine tested support. No Python/R assets, ingestr path, shell interpolation or unauthorized telemetry. A pinned runner, disabled telemetry, secret-safe invocation and machine-readable validate/lineage evidence are implementation deliverables.

Writes target only registered Chartworks-managed objects/schemas. A matching name prefix alone never proves ownership. Validate destinations independently at definition and rendered-execution boundaries; use scoped write credentials. Baseline customer data remains read-only. Failed checks never silently publish partial data.

Guided setup (phase 33) separates configuration/connectivity from semantic inference and optional materialization. It proposes grain, joins/cardinality, measures, dimensions, derived KPIs, units/currency, temporal/null semantics and unresolved questions with evidence. Missing model output is explicit incompleteness. Human semantic publication remains required. No cloud infrastructure provisioning platform is added.

## 8. Semantic lifecycle and compact context

Retain draft -> review -> published -> deprecated versions with an orthogonal active/archived topic state. Exactly one active published version, compare-and-swap publication, immutable published payloads, draft-only mutation, discard guards, diff/history and rollback are mandatory. Build routing facets before the active pointer changes or keep the prior version active until a version-fenced publication transaction completes. A background failure cannot pair new semantics with old facets.

Carry table rename/reference rewrites across measures/dimensions/KPIs/joins, source-health recheck, archive exclusion, entity CRUD/moves, onboarding profiles, canonical registry and neutral export/import. Tenant/project sharing is decided by Pengui; exports/imports preserve meaning and references, not old authentication machinery.

Topic generation uses bounded batches with stable entity IDs. Published rich packs project to compact capability cards; one `ContextAssembler` owns query-time pruning, uses one tokenizer-backed budget and per-request copies, preserves explicit pinned metrics and hard constraints, and reports insufficiency instead of silently dropping mandatory rules. Rules/examples have declared budgets and provenance; enforceable restrictions cannot disappear because a prompt is full.

## 9. NLQ, BYO and safety

Retain span hints, typed/batched semantic retrieval, published/healthy/authorized eligibility, optional reorder-only reranking, calibrated routing confidence, no-route/clarify outcomes, and session-scoped follow-ups. Preserve English/Spanish functional fixtures rather than deferring known language behavior. Confirmed same-source multi-topic relationships, join reachability/cardinality and all-source restrictions are migration scope; arbitrary cross-warehouse federation is not implied.

Generation uses the explicit precedence `edit_base > hints > examples > default`, native dialects and schema-constrained outputs. Bounded correction can occur only in exploration: at most one validation correction and one execution correction, all counted under the run's global budgets and revalidated. Zero rows are not permission to widen a filter/time range; self-curation preserves mandatory constraints and exposes suggested semantic changes for confirmation.

BYO context bundles restate constraints and provenance. Store the exact bundle under an opaque expiring reference bound to caller/tenant/session/context; references confer no authority and are reauthorized with a Pengui JWT. This replaces the old locally signed bundle handle. Submitted SQL runs through the same safety/execution core; context-only authority cannot submit or execute.

Validation: bounded decoding; whole-tree statement/relation/function validation where the dialect parser is proven; source-native dry planning under the read credential; positive authorization of actual dependencies and context; read-only execution. EXPLAIN, a SELECT prefix or successful parsing is not independently a safety proof. Unproven relation/function/RLS coverage fails closed. Never use EXPLAIN ANALYZE as a harmless validator. Preserve CTE/window/set-operation fixtures without simplistic keyword bans.

Execution uses server-side and context timeouts, cancel/reconciliation where supported, cursor-level row/byte caps, stable ordering and lossless data encoding. No string-wrapped LIMIT that changes semantics. Plan-only operations cannot execute. Record attempts/query IDs; an indeterminate remote attempt is not universal exactly-once execution.

Feedback, corrected examples, deduplication, DB-first Wilson/recency/evidence weighting, rule proposals, shadow comparisons, historical replay, evaluation-case seeding and bounded prompt-pack optimization have explicit owners in phases 16/18/24. Promotion is reviewed; feedback never auto-publishes business semantics.

## 10. Outputs and rendering

Keep provider-neutral column metadata, chart recipes, bindings, format hints, selection provenance and alternatives. Deterministic chart selection is pure and rules-first; optional model ranking is confined to exploration/authoring. The required catalog covers area, bar, column, donut, grouped bar, heatmap, KPI, line, pie, scatter, stacked bar, stacked column, table and treemap.

Frozen blocks reuse saved definitions; they do not reroute, regenerate SQL or select charts. Chart/KPI/table outputs are deterministic over normalized data. Optional saved narratives are evidence-bounded post-query calls through the gateway, with zero query/write tools and exact retained output. RFC-002 defines reports, artifacts and publication.

A shared read-oriented viewer serves MCP Apps and BFF-backed iframe delivery. Go renders tables/KPIs/safe text; a bounded optional ECharts SVG worker supplies genuine static chart rendering. This is not a builder application or a second execution service. Opening an artifact causes zero model/warehouse calls.

## 11. Surfaces

All business HTTP operations use `/v1` and the same verified envelope. `/healthz` and `/readyz` expose only sanitized liveness/readiness. Metrics, audits and destructive maintenance need signed operational scopes. Route/tool registration binds operation scope, resource resolver, audit classification and error contract once.

Retain the established eleven discovery/question/BYO/feedback tools, adding the narrow reporting tools in phase 31. Resources use the existing MCP Apps mechanism. Tool annotations describe real effects: query/read-only warehouse behavior does not make persisted runs or paid calls side-effect-free. No host compatibility qualification gate or mandated MCP protocol upgrade is added.

Source/dataset/pipeline/topic/rule/query/session/feedback/schedule/reporting operations are implemented by their domain phase and exposed through the early transport shells. There are no grants/principals/users/keys/auth/bootstrap or embed-token-issuance routes. SDK and CLI acquire Pengui tokens from their caller/configuration; neither mints authority. In-process SDK calls still require a verified envelope.

## 12. Persistence and asynchronous authority

Use PostgreSQL, pgx and forward-only migrations. Tenant IDs are mandatory and participate in keys/foreign keys/query predicates. No V1 SQLite driver. Create domain tables with their first real consumer, rather than allocating every speculative table at boot.

Retain domain relations for sources/datasets/profiles, pipelines/runs, topics/versions/audit, rules, query sessions/queries/saved queries, examples/feedback, facets, operations/schedules/occurrences, proposals/decision records, audit/model usage and migration state. Add typed block/revision/attestation, report/revision/dashboard, reporting execution/artifact/output/retention and external-reference relations in their owning phases. Reuse operations, idempotency and attempts instead of creating a second queue. Stored topic visibility is presentation/lifecycle metadata, never an access grant.

Remove planned local IAM relations: API keys, grants, roles, memberships, identity-service accounts and issuer-key state. An optional tenant operational-settings row is not a tenant identity authority. No local autonomy-access policy or embed-grant store is required.

Each accepted operation stores target/manifest/attribution and an opaque Pengui execution binding when it must outlive the initiating request. Tokens remain transient secrets. At dispatch/retry, obtain fresh authority through Pengui's existing broker integration, validate it normally and verify exact target coverage. No local re-signing or replay of expired tokens. An unresolved broker integration blocks unattended execution rather than enabling an ambient service-account bypass. See phase 30 and the authority contract.

## 13. Gateway, budgets and evaluation

One gateway with per-role provider/model/endpoint/secret-reference/time/token settings and adapters for the established Bifrost path and deterministic test fixtures. Roles cover embedding, semantic generation, SQL generation/correction, clarification, pipeline drafting, profile summaries, optional reranking, narrative and optional visualization ranking. Required roles fail loudly; optional roles report explicit skipped/unavailable outcomes. No local NLP-model downloads are introduced.

Embedding identity/dimensions are pinned to the relevant published facet generation. Frozen-only operation must not fail because an unrelated model provider is down. Reserve and enforce global/tenant operation concurrency, model calls/tokens, warehouse attempts, rows/bytes, elapsed time and retained payload limits before and during work. Retries count. Usage attribution is durable but not a second billing/entitlements service.

Evaluation spans routing, context budgets, semantic correctness, validation, visualization, reporting invariants and all supported engines. Zero critical security violations are mandatory; quality thresholds are documented per suite and compared against the accepted baseline. Live model accuracy is distinct from recorded-fixture determinism; no benchmark is claimed from unexecuted tests.

## 14. Configuration

Typed configuration rejects unknown/retired keys. Required auth configuration: Pengui issuer/JWKS/intended audiences/algorithm allowlist/temporal and size ceilings. There is no auth mode or signing-secret configuration. Feature limits live with their owning phase and in a generated config reference/example checked against the actual config schema.

Reference limits: metadata body 10 MiB; upload 100 MiB/1M rows subject to format expansion caps; query rows default 10,000 and ceiling 100,000; preview 200 rows; statement timeout 60s; context tiers 1500/3000/6500 tokens with separately budgeted mandatory constraints; examples at most seven; worker concurrency four. These are limits/defaults, not measured performance promises. Reporting retention/render/schedule limits are specified in their phase plans.

## 15. Observability and operations

Content-free audit events, bounded-cardinality metrics and protected diagnostics. Preserve stage timing, token/cost and cache attribution without logging tokens, warehouse credentials, rows or raw prompts/SQL. Protected query-domain evidence has explicit retention/access. Every registered metric must export; every mutating or paid operation has attribution and failure evidence.

Readiness checks configuration, store/migrations and trusted verification-key availability; feature-specific dependencies are reported separately. Data-source/provider availability is not a reason to take healthy artifact-reading endpoints down. Graceful shutdown fences/releases leases, cancels or reconciles remote work and never invents success. Erasure removes tenant values, facets/workspace data and retained renditions under explicit signed maintenance authority; partial erasure is visible.

## 16. Verification and phase completion

The active `docs/plans/README.md` and machine-readable phase/coverage registry define dependencies and scope. Each phase has concrete tasks, acceptance IDs, named tests, configuration/migration ownership and a smoke script. Planning consistency is checked separately from runtime acceptance. A missing or skipped runtime test is never a passing release result.

Archive documents are reference material only. Changes to old non-goals or ownership are recorded in the appended decision entries, not left to an implementation agent to infer.

## 17. Deployment

The container remains the reference unit under the accepted CGo exception policy. Go is the application core; pinned pipeline/render subprocesses are bounded and supervised. PostgreSQL/pgvector backs metadata; customer-source databases remain separate concerns. Do not impose Redis/Kafka/Temporal, a browser automation runtime for every chart, or a standalone dashboard UI.

Build/setup documentation must include Pengui registration/scope configuration, verification keys, source connections, enabled renderer/runner, storage/backup/retention and shutdown/recovery. Missing optional capabilities are advertised honestly. A migration cohort cannot be declared complete when one of its required engines/features is disabled.

## 18. Decisions and implementation ownership

Owner feedback is recorded in D-044–D-052 in `docs/decisions/2026-09-04-execution-baseline.md`. These explicitly supersede the conflicting portions of the earlier dual-issuer, local-grant, rendering exclusion, signed-bundle, host-checkpoint and atomic-cross-system language. Unchanged safety, semantic, connector and gateway design goals remain.

## 19. Non-goals and explicit later work

No Chartworks authentication/authorization policy service, compatibility investigation for the established hosts, standalone builder UI, unrestricted custom schedule jobs, schedule event/condition stubs, arbitrary script widgets, general multi-warehouse federation, or full BI document-layout engine. Cron/interval/manual runs and real targets are required. Outbound notifications are delegated to existing Pengui integrations, not silently advertised as sent by a catalog write.

L2 reviewed engineering and drift-generated amendments remain planned functionality. L3 autonomous apply and a new internal multi-step analyst orchestrator remain explicit post-cutover extensions; they are not prerequisites for reporting parity. Existing cross-topic, replay, learning and dynamic-report behaviors are not swept into that deferral.
