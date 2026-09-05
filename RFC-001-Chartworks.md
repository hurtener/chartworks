# RFC-001 — Chartworks execution baseline

Status: implementation design, revised 2026-09-04 after owner feedback on reporting, Pengui authority, established MCP Apps and Bifrost-only remote inference. Design acceptance is not runtime completion; all 34 phases remain planned.

Authority: RFC-001 for shared architecture/security; RFC-002 for reporting; the contracts referenced here and active numbered phase plans for implementation; master plan; contributor rules; research. Append-only decisions are in `docs/decisions.md` and `docs/decisions/*.md`. Historical plans and proposals under `docs/archive/` are not competing instructions.

## 1. Product and ownership

Chartworks is Pengui's Go-native structured analytics and governed publishing capability: connect/upload -> profile/model -> explore -> approve reusable blocks -> compose reports/dashboards -> execute/schedule -> retain and render results. Direct-source querying is first-class; managed engineering is justified by need, not a prerequisite for the first answer.

Pengui owns authentication, issuer/signing, users/service identities, sharing, entitlements, authority decisions, renewal and revocation. Chartworks verifies Pengui JWTs and applies their signed scopes/restrictions; it does not recreate those decisions. Independently deployed Chartworks still consumes Pengui authority rather than a local-login mode.

Harbor/Pengui MCP Apps support is established end to end by the owner. No host compatibility research, qualification transcript, framework adoption checkpoint or mandated protocol migration is required. Chartworks builds and tests its own tools/resources/viewer.

Chartworks owns source and semantic metadata, safe data execution, business publication/certification records, reporting definitions, operations, artifacts and functional scheduling. Pengui owns product UI and identity/access policy; Harbor owns agent orchestration/sessions. Services use public interfaces, never each other's private database. Soundings/Stowage can assist authoring but are not prerequisites for frozen refresh.

## 2. Vocabulary and invariants

A topic is a versioned semantic contract. A block is an approved reusable query with typed parameters/dependencies/outputs. A report composes frozen, explicitly dynamic and text widgets. A dashboard orders exact report-revision pages. A run is an accepted logical operation with resolved manifest and attempts; its artifact is retained output, not a rerun URL.

P1: enforce signed authority before access, distrust SQL, structurally separate reads/managed writes. P2: identity comes from verified Pengui claims, never body/header hints. P3: mandatory tenant restrictions in store and source interfaces. P4: typed observable failure, no silent authority expansion. P5: one Bifrost SDK inference gateway. P6: clear domain vocabulary without distorting standard protocol fields. P7: one core per capability with thin surfaces.

Publication, certification, current health and signed data authority are separate. Successful execution is not business approval. Private previews stay private after later publication. SQL safety, reference integrity, lifecycle evidence, retention and budgets remain Chartworks responsibilities; these do not constitute a local identity policy system.

## 3. Architecture

One Go application with HTTP/MCP handlers, PostgreSQL metadata/leased queue, domain services and bounded workers. Optional supervised subprocesses are the managed-pipeline runner and static chart renderer. No extra message bus, general workflow engine or inference server is required.

Packages: config, telemetry, identity, auth (verification only), access (signed-scope enforcement only), store, gateway/bifrost, vindex, sources, exec, engineering, semantics, nlq, charts, reporting, rendering, jobs, api and mcpserver. Public SDK lives under `sdk/chartworks`.

Keep dependencies acyclic. `exec` owns validated plans/read-adapter interfaces; concrete source drivers implement them and the composition root injects them. A nonzero executable plan can only be produced by validation and binds source/dialect/context/semantics/parameters. Core services and in-process SDK operations receive verified envelopes, not ambient authority.

Phases21–23 land early transport/client shells; each domain phase adds concrete operations with its first consumer. Unbuilt operations are absent, not success-returning placeholders. Viewing/reporting does not wait for autonomy or scheduling completion.

## 4. Pengui-issued identity only

The sole request path is Authorization bearer -> configured asymmetric verifier/JWKS -> immutable verified envelope -> signed-scope enforcement -> domain service. Validate configured issuer, intended audience, algorithm/key binding, expiration/temporal rules and bounded required identity/scope fields. `sub` and `user` agree under the actual Pengui contract when both are present.

[The authority contract](docs/contracts/pengui-authority.md) defines the capability integration. Its new provider scope serialization and durable execution binding must be wired to actual Pengui interfaces during the owning phase; this document does not assert those extensions already exist in a deployment. Use one decoder, not alternate issuer profiles or guessed broker endpoints.

No local signing keys, token mint/exchange, API-key registry, passwords, users/roles/groups/grants, service-account provisioning, OAuth server, bootstrap-admin or embed credential service. Trusted JWKS location is operator configuration; tokens cannot select arbitrary key URLs. Verification-key distribution is not issuance.

A valid JWT is a time-bounded authority snapshot. Offline validation does not imply immediate revocation after a Pengui policy change. Pengui controls renewal and lifetime; Chartworks rejects expired authority and never extends it. Each new protected request validates its supplied token. Long-lived work obtains fresh Pengui authority at required checkpoints rather than persisting/replaying user tokens or maintaining a local revocation database.

## 5. Authorization enforcement, not policy ownership

Pengui signs operation scopes and bounded resource restrictions. `access.Require` compares them with the addressed tenant/resource and source execution context. Metadata lookup establishes ownership/reference integrity, not permission. Creator names, service prefixes, audience labels or an `admin` string never create implicit access. Explicit tenant-wide authority is accepted only under the signed provider contract.

Executing reports checks their actual executable dependencies. Reading a retained result needs signed target/context read reach and its persisted privacy, not permission to generate a new query. Intersect signed reach, published semantic allowlists and the actual credential/RLS/secure-view context before source access. If a required restriction cannot be enforced, deny rather than fetch broadly and filter afterward.

Result reuse includes the actual context/data partition and exact definition/request semantics; a shared tenant/question is insufficient. A client-supplied context label cannot narrow broad data retroactively. Diagnostics explain enforcement of supplied authority, not hypothetical local roles or group membership.

## 6. Sources and secrets

Retain the source registry/adapter seam, discovery, normalized type categories, availability/health and structurally secret-free read shapes. Required real engines are PostgreSQL, MySQL, SQL Server, BigQuery, Snowflake and Databricks. Test mocks are not advertised as working production sources; unsupported capabilities fail explicitly.

Warehouse credential custody is separate from user auth. Use the established secret-provider/envelope-encryption seam with key rotation, pool invalidation and explicit connection testing. Prefer platform secret references where available; do not build a second Pengui integration-token vault. Source/model credentials never appear in read/list metadata, prompts, reports, tool output or ordinary logs. Read and managed-write credentials are separate.

Each engine proves discovery/type/normalization/cap/cancellation/SQL-context safety. Self-hostable engines use real container fixtures; cloud engines use recorded fixtures plus applicable live evidence before support/cutover claims. Pin actual adopted driver/parser versions and document build constraints; historical research or a PostgreSQL test does not prove all engines.

## 7. Engineering, uploads and onboarding

CSV/XLSX/Parquet uploads enter managed PostgreSQL workspace tables separate from metadata storage and become normal governed datasets. Bound bytes/rows/cells/decompression, sanitize executable spreadsheet content, reject unsafe paths and make staging/activation/cleanup crash-safe and tenant-scoped. No parallel weak-auth dataset mode.

Profiles record sampling method, type/null/distinct/range/value-family summaries, quality findings, freshness and observation time. Sampling caps do not guarantee that the engine scanned no pages; expose actual adapter cost controls. Profile generation is bounded/resumable; model-generated descriptions use the same remote Bifrost gateway.

Pipelines are versioned SQL-only steps with declared inputs/output/destination, strategy and blocking checks. Bruin remains behind `PipelineRunner`, not a recreated Go orchestration platform. Preserve applicable create/replace, append, merge/incremental, interval and scd2 strategies via tested per-engine support. Pin the runner and prove telemetry disabled, secret-safe argv/environment, bounded resources and machine-readable validate/lineage. No Python/R assets, ingestr or arbitrary shell execution.

Writes target only registered Chartworks-managed objects/schemas, checked independently at definition and execution rendering, backed by scoped write credentials. A name prefix is not ownership. Customer baseline data is inputs-only. Failed checks never silently publish partially valid outputs. Local metadata publication is transactional; external DDL requires durable steps, reconciliation and honest compensation/partial states.

Guided onboarding separates setup/connectivity, profiling, semantic proposal, human review/publication and optional materialization. Propose grain, joins/cardinality, measures, dimensions/KPIs, units/currency, temporal/null semantics with evidence and unresolved questions. Missing model output remains incomplete, not approved defaults. No automatic credential or cloud-infrastructure manufacture.

## 8. Semantic lifecycle and compact context

Retain draft -> review -> published -> deprecated versions and active/archived topic state. Exactly one active publication, CAS, immutable published payloads, draft-only edits, discard guards, diff/history and rollback are mandatory. Build a complete matching facet generation before atomically switching the active pointer; a background failure cannot pair new semantics with old/incomplete vectors.

Preserve entity CRUD/moves, table rename/reference rewrites across measures/dimensions/KPIs/joins, source-health recheck, archive exclusion, onboarding profiles, canonical registry and neutral export/import. Pengui decides sharing; imported role/header/token material never becomes authority.

Topic generation uses remote Bifrost structured calls in bounded batches with stable IDs. Rich authoring packs project to compact published capability cards. One ContextAssembler owns runtime pruning, uses one tokenizer-backed budget, per-request copies and explicit provenance, preserves pinned metrics/hard constraints and reports insufficiency rather than silently dropping mandatory rules. Rules/examples have separate declared budgets and confidence/prior meaning.

## 9. NLQ, BYO and read safety

Preserve deterministic span hints, remote query embeddings, typed/batched authorized retrieval, published/healthy eligibility, optional Bifrost reranking, calibrated confidence and explicit no-route/clarify outcomes. Reranking sees only already authorized candidates. Retain English/Spanish fixtures, prior context/SQL and session-scoped follow-up deltas. Confirmed same-source multi-topic joins/cardinality and all-resource restrictions are migration scope; arbitrary federation is not implied.

Generation uses `edit_base > hints > examples > default`, native dialect and validated structured output. Exploration may perform at most one validation correction and one execution correction within the shared operation attempt/token/time budget. Every correction is revalidated. Zero rows do not authorize broader filters or time ranges. Semantic corrections remain reviewed proposals.

BYO bundles restate constraints and provenance, stored behind opaque expiring references bound to caller/tenant/session/context. References confer no authority and are reauthorized using Pengui JWTs, replacing local signed handles. External SQL receives identical validation/execution gates; context-only permission cannot submit or execute.

SQL validation combines bounded decoding, positive whole-tree statement/relation/function controls where the dialect parser is proven, source-native dry planning under restricted credentials and verified actual dependency/context reach. SELECT prefixes, parsing and EXPLAIN are not independent safety proofs; never use EXPLAIN ANALYZE as harmless validation. Unknown dependency/function/policy coverage denies execution. Preserve legitimate CTE/window/set operations without simplistic keyword filtering.

Read execution combines read-only credentials/session restrictions, server-side and context deadlines, cancellation/reconciliation where supported, cursor row/byte caps, stable ordering and lossless normalized results. Do not string-wrap LIMIT or rebind changed columns silently. Record source query IDs/attempts; physical exactly-once behavior is not assumed.

Feedback, corrected examples, deduplication, DB-first Wilson/recency/evidence weights, rule proposals, shadow comparison, historical replay, evaluation-case seeding and bounded prompt-pack optimization belong to phases16/18/24. Model-assisted optimization also uses Bifrost remote inference; no local training runtime. Promotion is reviewed rather than feedback auto-publishing business semantics.

## 10. Outputs and rendering

Provider-neutral metadata, recipes, bindings, format hints, alternatives and selection provenance remain canonical. Rules-first deterministic selection is pure; optional model ranking belongs only to exploration/authoring through Bifrost. The catalog includes area, bar, column, donut, grouped bar, heatmap, KPI, line, pie, scatter, stacked bar, stacked column, table and treemap.

Frozen blocks reuse approved queries/saved outputs, with no interpretation, retrieval, routing, SQL generation/correction or chart selection. Chart/KPI/table outputs are deterministic over the result. Explicit bounded narratives are post-query remote calls with no query/write tools and retained exact output. RFC-002 governs reporting lifecycle and artifacts.

A shared read viewer serves MCP Apps and BFF-backed iframes. Go renders tables/KPIs/text; an isolated optional ECharts SVG worker supplies genuine chart SSR. Neither is a builder application or new execution service. Opening/rerendering an artifact makes zero warehouse/model calls, including when providers are unavailable.

## 11. Surfaces

Business HTTP operations use `/v1`, one registered action/resource loader, typed error and audit/effect contract, and one verified envelope. Health exposes only sanitized liveness/readiness; metrics/audit/destructive maintenance require signed operational scopes.

Retain eleven discovery/question/BYO/feedback tools and add narrow reporting search/describe/run/history/view tools. Resource metadata uses the established Apps mechanism. Paid calls and persisted runs have actual side-effect annotations even if source SQL is read-only. Test new code without host qualification or forced protocol changes.

Source/dataset/pipeline/topic/rule/query/session/feedback/schedule/reporting operations are implemented by owning domain phases through early thin shells. No grants/principals/users/keys/auth/bootstrap/embed-token issuance routes. SDK/CLI consume caller-supplied Pengui authority; in-process calls do not bypass it. [COMMON.md](docs/plans/COMMON.md) defines required surface parity.

## 12. Persistence and durable authority

PostgreSQL/pgx and forward-only migrations with tenant-composite identity/reference constraints are the initial metadata store. No SQLite driver. Add domain tables with consumers: sources/datasets/profiles; pipelines/runs; topics/versions/audit/rules; queries/sessions/saved questions; examples/feedback/facets; operations/schedules/occurrences; reviewed proposals/decision records; reporting identities/revisions/attestations/manifests/artifacts/output retention; neutral external-reference mappings; audit/model usage. Reuse common queue/idempotency/attempt state rather than a second work platform.

No local identity, API-key, role/grant/membership, service-account or signing-key tables. Tenant operational settings and business lifecycle metadata cannot grant access.

Accepted durable work records immutable target/manifest/attribution and an opaque Pengui-authorized execution binding, not token bytes. Before dispatch/retry or required later checkpoints, a thin injected provider obtains fresh Pengui authority and the ordinary verifier checks it. The concrete platform binding/renewal API is a required Pengui integration deliverable, not asserted to exist merely because another capability has a minter. Reuse the actual broker where it fits, extend Pengui where required, and never implement local signing or guess an endpoint. Missing fresh authority blocks unattended execution without an ambient bypass.

## 13. Bifrost SDK, budgets and evaluation

[The model gateway contract](docs/contracts/model-gateway.md) and [policy](docs/contracts/model-gateway-policy.json) are normative. Production has one driver: embedded `github.com/maximhq/bifrost/core`, initial reference pin v1.6.2 observed in Soundings. Every completion, structured generation, embedding and rerank call uses that SDK with a remote configured provider. No local learned models, weight downloads, model-serving processes, ONNX/cross-encoder fallback or alternate direct-compatible client. A separate Bifrost proxy is not required. Fixtures are explicit test-only dependencies.

Ten role keys independently resolve provider/model/endpoint/credential/timeout/limits: embedding, enhance, sqlgen, sqlfix, clarify, pipeline_draft, profile_summary, rerank, narrative, visual_rank. The non-secret [example excerpt](examples/chartworks.gateway.json) reuses the observed remote embedding/rerank settings from sibling configuration. Do not copy `.env`/credentials, sibling auth or local storage. SDK/provider version support is proven by actual adapter tests, not assumed across differing sibling pins.

Embedding responses require complete input-count/index coverage, valid dimensions and finite values. Full embedding-space identity includes route/model revision/preprocessing/input/normalization options; same dimensions alone are insufficient. Reindex before generation switch, never auto-fallback to a different model. Cache/batches preserve input and tenant/context identity.

Reranking only orders authorized candidates; require complete unique valid indices and finite scores rather than inventing omitted scores. Disabled roles make zero calls. Enabled failure behavior is explicit fail or visible preservation of original authorized order. Schema outputs are independently validated; provider errors are sanitized. Client lifecycle/request cancellation and bounded concurrency are tested.

Reserve/enforce operation and tenant concurrency, call/token/warehouse-attempt/row/byte/time/retention budgets before and during work. Coordinate SDK/domain retry budgets, preserve actual provider attribution and unknown costs, and avoid adding aggregate costs to their component costs twice. Billing entitlements remain Pengui-owned.

Frozen/no-narrative execution and artifact reads work with inference unavailable. Pure tokenization, rules, SQL parsing, pgvector search and rendering are normal local computation, not learned inference. Evaluate routing/context/semantic correctness/SQL/output/reporting and engine coverage separately. Zero critical violations are required; live model accuracy is not recorded-fixture determinism or an inherited POC speedup.

## 14. Configuration

Typed configuration rejects unknown/retired keys. Pengui issuer/JWKS/intended audiences/algorithm/temporal/size rules are required; no auth mode or signing secret. The phase05 decoder consumes the nested Bifrost provider/role excerpt and fails loudly on invalid required config. Provider network health is reported per capability rather than preventing unrelated reads.

Reference bounds: metadata body10MiB; upload100MiB/1M rows plus expansion limits; query default10,000/ceiling100,000 rows, preview200 and statement timeout60s; context tiers1500/3000/6500 tokens with mandatory constraint handling; seven examples; four workers. Reporting/renderer/scheduler limits live in their phase. These are configurable limits, not performance promises.

## 15. Observability and operations

Content-free audit and bounded-cardinality metrics preserve actor/resource/operation IDs, stage timing, token/cost/attempt/cache attribution and actual failures. Default logs contain no tokens, warehouse/provider credentials, rows or raw SQL/prompts. Protected diagnostic/domain evidence has separate access and retention. Every registered metric exports and every mutating/paid action carries attribution.

Readiness checks config, store/migrations and verification-key availability. Source/inference/render health is capability-specific; its failure does not take healthy artifact reads down. Drain/cancel/fence leases and reconcile remote work on shutdown. Erasure covers values, facets, workspace and rendered outputs under signed maintenance authority, with explicit partial failures.

## 16. Verification and phase completion

The master, phase registry and coverage map assign **34 phases, 224 acceptance criteria, 63 source-feature rows and 41 review gates**. Every criterion requires an actual named test. The Python planning checker validates graph/metadata/links/counts/mapping/mirror/config coherence only. Missing/skipped/empty tests cannot pass runtime acceptance, and release mode requires every phase shipped with actual successful test events.

Historical material is evidence at its stated depth/date. Scope/ownership changes are reflected in active plans instead of left for an implementation agent to reconcile. Source parity requires real behavior and migration evidence, not a file inventory.

## 17. Deployment

The container remains the reference unit. Go is the core; accepted per-dependency CGo exceptions remain explicit. Shipping core builds may prefer CGo-free while race tests use a race-capable toolchain. Pin and supervise pipeline/render subprocesses. Model weights/inference services are absent from image and startup. Metadata PostgreSQL/pgvector and customer sources remain separate.

Document Pengui scope registration/verification, source/model secrets, enabled runner/renderer, backup/retention and recovery. No Redis/Kafka/Temporal or headless browser is required for every chart. A cohort cannot be called migrated while a required engine or feature is disabled.

## 18. Decisions and ownership

D-044–D-052 in `docs/decisions/2026-09-04-execution-baseline.md` replace old dual-issuer/local-grants/no-rendering/signed-handle/host-checkpoint/distributed-atomicity instructions. D-053/D-054 in `docs/decisions/2026-09-04-bifrost-only.md` make remote Bifrost SDK exclusive and complete the actionable plan/checks. D-043 remains reserved for the separate historical provider-role follow-up. Earlier unchanged safety and semantic/source goals remain in force.

## 19. Non-goals and later work

No Chartworks identity policy service, host compatibility investigation, builder application, local inference, alternate production model client, unrestricted custom schedule jobs, event/condition stubs, script widgets, general multi-warehouse federation or full BI document-layout engine. Required scheduling is functional cron/interval/manual with real targets; optional notifications use Pengui integration receipts.

L2 reviewed engineering/drift proposals remain planned. L3 auto-apply and a new internal analyst are explicit post-cutover extensions, not reporting blockers. Existing cross-topic/replay/learning/hybrid-report behavior is not swept into those deferrals. PDF/PNG/paginated documents are not implied by required HTML/SVG rendering.

## Phase 03/04 authority implementation

D-059–D-061 implement the existing Pengui provider-scope seam with one JWT verifier/cache and immutable signed envelope. [The operator handoff](docs/contracts/pengui-provider-registration.md) and [actual operation manifest](docs/contracts/chartworks-operations.json) describe the implemented consumer. Scope limits are 32 entries, 256 bytes each, 4096 total; exact HTTP/MCP audiences may be configured separately. Synchronous operational routes and SDK clients are present, but no local issuer, grants database, reporting API or full MCP transport is added by this milestone.

## Phase05/06 implementation addendum (2026-09-05)

D-062/D-063 deliver the remote-only Bifrost gateway, fixed-input operator probes,
one PostgreSQL queue and bounded maintenance occurrences with the actual
[Pengui execution-authority v1 companion](docs/contracts/execution-authority-v1.md).
Metadata cancellation/read/pause do not require live models or an enabled worker.
Broader analytic/reporting targets and external-effect reconciliation remain owned
by their later phases; this is not a reporting release or live provider acceptance.
