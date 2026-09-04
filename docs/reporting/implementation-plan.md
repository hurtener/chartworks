# Reporting implementation, migration and acceptance plan

Status: proposed, 2026-09-04. This plan supplements the existing phase plans; it does not mark any implementation phase complete.

## 1. Reconcile the design before coding

The accessible main baseline is commit `8de9641ddba33bd86d4aeb2a080a8a4fedddf01d`. Its first planning PR is already merged. The inspected pass-2-named branch had the same commit as main; no distinct open reporting proposal was visible during this review. A separate planning branch contains post-merge provider/rerank work. Preserve all of it rather than overwriting or assuming that it has been merged.

Before the first implementation PR is approved:

- Accept or amend RFC-002. Update RFC-001's product/non-goals, auth profile, grant/scopes, domain persistence, rendering/delivery and release criteria to reference the exact accepted amendments.
- Append decisions to the established decision log; do not rewrite historical decisions. Allocate identifiers from the actual current tip, not a guessed D-number. Reconcile D-043's provider-role work explicitly.
- Update the master plan, relevant phase plans, research index, glossary and both mirrored contributor-rule files where the authority chain or scope changes. Confirm the mirrored rule files remain identical. This package intentionally leaves the historical RFC text intact while its amendment is proposed.
- Generate a feature ledger from brief 14. Each row needs a contract owner, target operation, implementation test, migration rule and status. Newly found source behavior extends the ledger rather than being dismissed as outside a prewritten phase.
- Pin the exact Harbor/Pengui/dependency versions for integration tests and approved source-engine matrix. Verify current licensing and deployment costs for pinned dependencies; historical research pins are not an indefinite supply-chain approval.

These are acceptance checks, not an excuse to build a new planning framework. The existing repository planning conventions remain the mechanism.

## 2. Workstreams and phase crosswalk

| Existing area | Required addition |
|---|---|
| Phase 01 configuration / telemetry | Reporting limits, renderer enablement, retention, issuer profile, deployment capability manifest and content-free reporting metrics. |
| Phase 02 persistence | Revision/attestation/run/artifact state, tenant-composite integrity, transactional pointers and idempotency/fencing. Reuse existing queue/state seams where appropriate. |
| Phases 03–04 auth and access | Pengui issuer-profile conformance; report/block/dashboard grants; preview and SQL-read boundaries; artifact data-policy partition and audience checks. |
| Phase 05 gateway | Bounded narrative role, prompt/model provenance, reservation/enforcement of budgets; reconcile per-role provider and rerank changes rather than fork the gateway. |
| Phase 06 durable operations / scheduling | Typed report/block/saved-query targets, persisted occurrence windows, service grant rechecks, retries and separate delivery effects. |
| Phases 07–10 retrieval / sources / validation / execution | Frozen path and same-validator enforcement; native-dialect adversarial tests; precision-safe results; query cancellation/reconciliation. |
| Phases 11–15 workspace / engineering / source adapters / semantics | Report eligibility for uploaded datasets and managed datasets; dependency impact and review; resumable onboarding and exact semantic references. |
| Phases 16–19 rules / routing / NLQ / external-agent mode | Capture-to-block authoring; preserve clarification, templates, follow-up and current multi-topic behaviors; no LLM re-entry on frozen refresh. |
| Phase 20 chart specifications | Stable multi-output contracts, saved mappings, KPI/table/narrative behavior and renderer conformance. A specification is not itself a viewer. |
| HTTP / MCP / client / evaluation / deployment plans | Typed reporting API, Apps resources and scoped bridge, iframe grants, optional static renderer, parity fixtures and rollout evidence. |
| Phase 26 engineering autonomy | Reporting dependencies and staged apply/compensation. Neither L3 autonomy nor new internal investigations blocks the first governed reporting slice. |

Four implementation tracks can proceed after contracts stabilize: core/access; block/report execution; thin renderer/host integration using synthetic artifacts; semantic/NLQ/source parity. Avoid independent teams inventing incompatible result contracts. Each seam lands with its first concrete consumer and tests.

## 3. Vertical delivery slices

### Slice A — trusted frozen block

Build against one real reference warehouse adapter, with the intended production adapter joining the same contract suite before migration claims. Create source/topic fixtures, verify JWT/grants, create a block draft with chart/KPI/table outputs, validate using the real adapter, publish/certify explicitly, execute with typed parameters, and retrieve the retained result.

Exit evidence: zero model calls on the frozen path; one logical query fan-out; correct exact numeric values; SQL visibility denied to a data-only reader; private draft invisible; cross-tenant IDs fail; revoked permissions stop execution and reads. This is not merely a mock domain object or a new interface.

### Slice B — reports, dashboards and useful visual delivery

Compose multiple blocks plus safe text. Implement bounded filters, defaults and explicit partial-failure policy. Persist a fully resolved run and render it through an API client, a small iframe viewer and the actual target MCP host. Add versioned dashboard pages without a separate query engine. Provide useful fallback output to clients without Apps support.

Go-rendered tables/KPIs/text may be the first static capability. Full chart SSR is enabled only after its real SVG and security gates pass. No builder UI is needed.

Exit evidence: the same run displays the same values, units and trust state across surfaces; opening it repeatedly creates no warehouse/model work; exact-page/report references remain stable; draft previews do not become public after publication.

### Slice C — unattended delivery that can survive failure

Add exact block and report schedule targets, reviewed saved queries, and explicitly dynamic saved questions with their extra authority requirements. Persist occurrence times/windows and test retry recovery, fencing, cancellation, service revocation and limits. Catalog delivery is complete before an optional outbound adapter.

Exit evidence: two worker instances cannot commit competing results; retries preserve the original period; a stronger service identity cannot be selected to expand the creator's reach; expired artifacts do not silently rerun; email is never reported delivered without a real receipt. A physically repeated remote query after an indeterminate attempt is visible in evidence.

### Slice D — authoring quality and explicit hybrid reports

Add approved-query capture, question aliases/duplicate assessment, assisted parameterization, bounded narratives, review/publish workflows, dependency impact and new-draft amendment proposals. Preserve explicit dynamic widgets and distinguish replayable questions from session-bound references. Use source fixtures privately and synthetic public regressions.

Exit evidence: generated narrative claims resolve to authorized evidence; a changed semantic definition cannot retain an inappropriate current approval; dynamic SQL is never labeled certified; disabled session-bound execution fails clearly rather than impersonating an old user.

### Slice E — onboarding and parity completion

Ship resumable source connection/configuration checks, profiling, proposed semantic models, human review/promotion, example questions and draft block suggestions. Keep direct-source operation available; create managed data transformations only when needed. Preserve the established NLQ/context/template/rule/refinement/feedback/dataset workflows and deployed dialects identified by the evidence ledger.

Exit evidence: all required source rows have an approved implementation/equivalence disposition; neutral imports and shadow comparisons pass; source grants, schedules, output mappings and private/public states survive migration. Only here can a full replacement claim be considered.

New event-driven scheduling, arbitrary integrations, L3 autonomy and broad internal investigation orchestration require their own accepted scope. The inspected condition-check target is also explicitly a stub returning unsupported, separate from the event/condition trigger stubs. Do not advertise any of them as carried implementation.

## 4. Acceptance matrix

Use real database/warehouse drivers at the boundary where the gate concerns driver behavior. Unit tests can isolate pure state/formatting logic, but a mock SELECT executor cannot prove read-only execution, cancellation, tenant isolation or dialect coverage. A skipped integration test does not count as a passing release gate.

| Gate | Scenario and required assertion |
|---|---|
| G01 — issuer profile | Accept the exact configured Pengui fixture; reject wrong tenant mapping, conflicting subject representations, malformed required claims, inappropriate service prefix and unapproved agent delegation. |
| G02 — JWT | Reject HS/none, key/algorithm confusion, invalid signature, wrong issuer/audience, expired or invalid temporal claims, unknown key and over-stale JWKS. Test configured valid audience-array behavior without cross-resource acceptance. |
| G03 — resource/data scope | Probe every registered route/tool with foreign tenant/resource IDs. Empty grant sets issue no warehouse call. Same-tenant users with different row/column policy partitions cannot share a cached artifact. |
| G04 — no scope amplification | Body/header/user-selected service ID cannot enlarge authority. Read-only embed token cannot call run, SQL-read, author, certify or normal MCP/API endpoints. |
| G05 — private states | Draft and pending-review artifacts stay private before and after publication of any report revision. Unknown/inaccessible references are nondisclosing. |
| G06 — optimistic concurrency | Competing draft edits/publications have one winner; stale expected versions conflict. An edit invalidates evidence as appropriate to its execution/revision hash. |
| G07 — immutability | Published revision content cannot change. Amendments create a new draft; pinned schedules do not follow a later publication. |
| G08 — certification | Publishing is not certification. A revoked/stale attestation cannot produce a current certified badge; policy-limited historical reads retain honest as-of evidence. |
| G09 — frozen execution | Instrument or fail on every forbidden interpret/retrieve/route/generate/rewrite/select-chart stage. Zero model calls without narrative. Multiple outputs share the same logical normalized result. |
| G10 — SQL security | Test CTEs, nested/subqueries, qualified-table escape, alias ambiguity, multiple statements, comments, writes/DDL, side-effecting functions, external-access functions and view/RLS/column-policy behavior per adapter. Unknown safety is a denial. |
| G11 — parameter safety | Malformed/range-invalid values, unknown filters, security-reserved fields, identifier injection and conflicting bindings fail. Allowed business filters do not override mandatory source policy. |
| G12 — temporal correctness | Previous month, leap day, explicit range, rolling period, first schedule occurrence, missed occurrence and DST transitions resolve into the intended half-open instants. Retries keep them unchanged. |
| G13 — exact data | Decimal cents and `9007199254740993` survive API, labels and export. Boolean is not accepted as numeric; null is not zero; non-finite/unsupported values fail safely. |
| G14 — result contract | Removed/renamed/reordered columns, incompatible type/nullability and duplicate aliases fail or follow an explicitly approved compatibility rule; no silent remapping. |
| G15 — output fidelity | Every supported presentation kind has golden bindings, labels, order, units, legends/stacking and empty/negative/missing-value cases. Table fallback is labeled, not counted as chart parity. |
| G16 — narrative grounding | Disallowed fields never enter the prompt; limits apply before calls; numerical claims reference retained evidence; unsupported claims fail the narrative policy. Generated text is retained with version provenance. |
| G17 — hybrid trust | A report mixes frozen, dynamic and text widgets with explicit provenance. Publication never certifies changing SQL. Missing originating session authority fails a session-bound widget. |
| G18 — partial failure | Strict report fails on required missing output. Explicit partial policy preserves successful content but names omissions and never emits a complete-success claim. |
| G19 — reference resolution | Resolve floating report/block/topic references once per run. Publication during a long run cannot mix revisions. Dashboard pages remain exact references. |
| G20 — request idempotency | Same key/request returns the accepted operation; different request conflicts. Retry after a new publication still uses the original manifest. Expired payload returns a truthful expired state, not a fresh query. |
| G21 — reusable result | Same question/report with different policy partition, output subset, period, locale or freshness policy cannot incorrectly reuse a result. Joined in-flight work has correct ownership and visibility. |
| G22 — lease recovery | Kill the worker after claim, after warehouse acceptance, after normalized-result persistence, during narrative/render and before final artifact commit. Fence stale writes; report indeterminate/repeated attempts honestly. |
| G23 — schedules | Cron/interval, pause/resume/retire, overlap/missed-run bounds, timeouts, retries, test-run provenance and exact block/report target behavior pass with a deterministic clock. |
| G24 — service revocation | Revoke the schedule owner or topic grants after schedule creation and before execution/delivery. No fresh values are produced or delivered under stale authority. |
| G25 — budget admission | Race concurrent requests at tenant/global limits. Reserve and enforce model calls/tokens, query attempts, bytes/rows and time; failed/retried work still consumes the intended accounting. |
| G26 — delivery state | Query success, retained artifact and catalog/outbound delivery have distinct states. Duplicate effect keys cannot send twice where the provider supports idempotency; uncertain provider outcomes stay uncertain. |
| G27 — actual MCP host | Verify UI metadata/resource MIME/CSP, bounded results, bridge calls, scope renewal, errors, paging, resizing, theme and fallback in the pinned Harbor/Pengui chain; record a transcript. |
| G28 — iframe boundary | No bearer in URL/storage; wrong parent/source/nonce, expired/replayed code, arbitrary redirect and overbroad origin policy fail. Revocation takes effect on subsequent reads. |
| G29 — SSR truth | With client chart JavaScript disabled, promised chart SVG/table/KPI content still exists. An empty client-rendered shell fails this gate. |
| G30 — renderer isolation | Attempt script/event/URL/CSS injection through labels, Markdown, theme and SVG. Verify no network/credentials, bounded resources, timeout and safe output rejection. |
| G31 — retention/deletion | Expire/delete values and renditions consistently; cursors/artifact references fail safely; audit tombstones do not retain sensitive payloads. Preview artifacts never inherit later public visibility. |
| G32 — observability | Trace attribution is correct; default logs contain no tokens, credentials, rows, SQL or prompts. Metrics use bounded labels; protected diagnostics have separate access/retention. |
| G33 — semantic impact | Cosmetic changes preserve compatible meaning; exact rename yields a proposed draft; semantic change requires review; removed dependency stops eligible refresh. No published query is auto-rewritten. |
| G34 — NLQ parity | Captured question/clarification/follow-up/template/rule/context fixtures preserve meaning, policy and expected outputs across the Go path. Measure quality separately from query execution success. |
| G35 — managed writes | Reject writes to every baseline object even under an apparently managed name. Registry ownership, credentials, proposal approval, staged effects and compensation behavior are independently tested. |
| G36 — onboarding | Resume after each failed stage without duplicate resources; missing semantics remain unresolved; credentials never enter model context; human promotion remains explicit. |
| G37 — import | Dry-run mapping reports every omitted/changed/invalid field. Stable external IDs/revisions deduplicate. Private previews, exact pins, grant mappings, period semantics and output bindings remain intact. |
| G38 — performance | Record environment and datasets for startup/idle, discovery, frozen overhead, normalization, artifact view, render and schedule recovery benchmarks. No inherited POC speedup is reported as a measurement. |
| G39 — deployment | Boot with production-like config, health/readiness, JWKS rotation/staleness, backup/restore, graceful shutdown and optional renderer disabled/enabled. Missing required configuration fails loudly. |
| G40 — release claim | Every promised source feature and engine has runtime evidence or an explicitly approved equivalent/disposition. Planning scripts reporting SKIP never close this gate. |

## 5. Migration runbook

Inventory first: source schemas, active topics/templates/rules, blocks and their revisions/attestations, report layouts and private states, dashboards, schedules/service accounts, grant mappings, retained artifacts and actual warehouse engines. Include orphan references and disabled features; do not silently drop them.

Create a private source-to-canonical mapping workbook/ledger outside this repository. The repository contains only neutral adapters, stable external-reference fields, synthetic examples and schema tests. Map identities/grants through the current signed-authority model; do not import bearer tokens, trusted headers, passwords or stored source secrets into report documents.

Dry-run every import. Require explicit lifecycle mapping; omission becomes private draft rather than accidental publication. Verify parameter precedence, exact topic/revision references, old section-to-layout projection, aliases, disabled outputs, unknown chart kinds, locale/timezone, recipient metadata and expiry. Quarantine unsupported records with a reason and a remediation plan.

Retain historical origin/trust honestly. Importing a source certification is not automatically a new Chartworks attestation: retain historical evidence and require the accepted revalidation/certification policy for current approval. Preserve stable references where safe, using a mapping relation rather than forcing source database IDs into new authority fields.

Run shadow comparisons on fixed synthetic and authorized customer-controlled datasets. Compare result schemas, exact values/aggregates, parameter windows, trust/publication visibility, artifacts and behavior under denied access. For generated SQL, compare semantic correctness and policy compliance, not merely text equality. Non-deterministic narrative text is judged by evidence/claim correctness and explicit budgets rather than exact wording.

Cut over per tenant/source/report cohort. Pause old schedules before enabling equivalent new occurrences; record the last accepted occurrence and deduplicate the handoff window. Do not run two uncontrolled schedulers sending the same report. Keep old read access available for an agreed rollback window; do not destructively delete source artifacts during validation.

Rollback disables new schedule creation/dispatch, restores the previous accepted publication/pointer where applicable, and redirects readers deliberately. It does not pretend external SQL effects or sent messages were rolled back. Record compensations, irreversible effects and remaining dependencies.

## 6. Decisions still requiring implementation evidence

The proposal chooses the product contracts, but these items cannot honestly be closed from repository reading alone: exact deployed host/SDK profile, real source-engine coverage, per-tenant row-policy/credential execution partitions, approved audience delegation rules, all current multi-topic/query-learning behaviors, measured startup/latency/cost, renderer feature/fidelity coverage, and production migration volumes/retention requirements.

Safe defaults exist: deny unknown authority/partitions; require exact pins; keep private material private; disable unsupported dynamic/schedule/renderer features explicitly; use catalog-only delivery; retain direct-source operation. A disabled required feature is a preview limitation, not full parity.

## 7. Coding-agent handoff prompt

> Implement Chartworks' governed-reporting foundation from the accepted RFC-001 plus RFC-002 and this reporting package. Begin by reconciling the documentation authority chain and generating the operation/scope/feature ledger. Preserve the primary-source product behavior using neutral synthetic tests; do not copy source identifiers, code, prompts, schemas or credentials into the repository.
>
> Deliver Slice A end to end with the actual store, identity resolver and real warehouse adapter, then extend through reports, artifacts, thin visual delivery and scheduling. One Go core owns execution/governance; existing gateway, grants and leased queue are reused. Do not create a parallel workflow system, standalone builder UI or unbounded plugin platform. A renderer consumes sealed data and cannot query or generate SQL.
>
> Make immutable revisions, separate certification/current health, typed periods/parameters, private previews, lossless numeric values, current authorization on retained artifacts, and policy-partitioned reuse structural invariants. Frozen execution has zero model calls unless an explicitly bounded narrative is enabled. Never rewrite a published query or silently repin a schedule. Never put a control-plane JWT in iframe URLs or app code. A SELECT prefix, EXPLAIN success, UI visibility hint or current user's browser fields are not authorization proofs.
>
> Ship each primitive with its first consumer, real boundary tests and negative cases from G01–G40. Record every skipped/unverified gate and do not report it as passed. Keep the ledger and getting-started/deployment documentation current. Return the changed contracts, migration changes, test commands/results, measured environment, operational limitations, and exact remaining parity rows. Do not claim a complete replacement after the first reference-adapter demo.
