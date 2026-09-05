# RFC-002 — Governed reporting implementation contract

Status: implementation design, revised 2026-09-04. RFC-001 controls shared architecture/security and the Bifrost-only inference contract; this file controls reporting semantics. Active phases27–34 and the revised original phases implement it. Historical proposals are archived; a design is not a passing runtime test.

## 1. Product boundary

Explore a question, turn a useful result into a reviewed reusable definition, execute it predictably, retain the evidence/result and consume it through API, MCP Apps, iframe or static rendering. No standalone builder is required. Harbor/Pengui Apps support is established; no host qualification work is introduced.

Pengui owns identity/access decisions and signs authority. Chartworks verifies JWTs, applies signed scopes/restrictions and enforces business/data-safety invariants. No local issuer, role/grant/service-account registry, bootstrap or embed credential service. Read [the authority contract](docs/contracts/pengui-authority.md).

All learned-model operations, including narrative, authoring assistance, embeddings and rerank, use [the embedded Bifrost SDK with remote providers](docs/contracts/model-gateway.md). No local models, weight downloads or alternate direct-compatible client. Frozen/no-narrative execution and artifact viewing remain independent of provider health.

## 2. Domain and custody

Block identity carries localized metadata, canonical question/aliases, authorship provenance and draft/published pointers. Revisions contain exact topic/template references, approved SQL/execution definition, typed parameters, ordered expected schema, dependency manifest and stable saved output IDs. Validation evidence binds exact content/dependencies and observed schema to a real checked execution; certification is a separate attestation.

A report revision contains ordered/grid widgets, filters/bindings, safe presentation settings, locale/timezone and execution/partial policies. A dashboard revision orders exact report-revision pages; it adds no query engine.

A run seals resolved definitions, parameters/window, selected outputs, actor/service attribution, actual source context/data partition, observation times, attempts/usage and output/model/renderer versions. The artifact retains values and evidence. Chartworks is authoritative; Pengui/Harbor keep references or rebuildable discovery, not divergent sole copies.

Immutable definitions do not freeze source data. Re-executing a revision can produce different values; a retained artifact records one execution. Cross-source runs are not one transactional snapshot without actual engine support. Expose observation/freshness evidence.

## 3. Lifecycles and business trust

Preserve draft/published/superseded/archived block semantics. Published content is immutable; edits create drafts. Validation/publication require expected-version checks and exact fresh content-bound evidence. Publication grants neither certification nor data/audience authority.

Certification references exact revision/evidence. Current valid/stale/withdrawn/unavailable status stays separate from historical approval. Preserve published/certified-only/explicit-stale/private-preview execution policy; stale business approval never permits expired JWTs or wider data.

Reports retain private draft -> pending review -> published plus explicit reject/return/amend/archive. Preview needs exact private revision and signed preview authority. Public run cannot accept a prefer-draft bypass. Persist artifact privacy independently of later report publication.

Version canonicalization and execution/revision/rendition hashes separately. Validation derives actual dependencies; authors cannot omit restricted relations from a manifest to authorize them. Classify cosmetic, exact rename, review-required and unavailable changes. Even a rename proposes a new draft and revalidation; approved SQL never changes during refresh.

## 4. Explicit execution lanes

Exploration retains context/routing/generation/clarification/refinement/templates/validation/bounded correction/feedback. Capturing a result is draft authoring, not automatic certification.

Frozen execution resolves an eligible published revision, verifies signed authority and dependency health, binds typed parameters, validates the approved query against the actual source context, executes and fans selected saved outputs out from one logical normalized result. Interpretation/retrieval/routing/SQL generation or correction/chart selection are forbidden. Default model calls are zero; only an explicitly enabled saved bounded narrative is an allowed post-query exception through Bifrost.

One logical query serving several outputs does not imply physically once-only remote execution after a crash/network failure. Record and reconcile attempts/query IDs. Opening or rerendering an existing artifact always makes zero source/model calls.

Dynamic widgets require explicit opt-in and ordinary query safety/scope/budgets. Replayable questions differ from session-bound query references; unavailable originating authority/context fails clearly. Publishing the report does not certify dynamic SQL. The service derives provenance/trust; it does not accept caller badges. Session-only scheduling requires explicit permitted and available context.

## 5. Parameters, outputs and presentation

Preserve date/datetime/relative period/dimension value/number/integer/boolean/grain/top-N types, required/default/range/enum rules, locale/timezone and value provenance. Bind values rather than SQL fragments; identifier-like choices map to closed approved structures.

Canonical precedence: block default -> report global default -> widget literal -> declared filter binding -> explicitly permitted invocation override. Same-level conflicts fail. Imports normalize old precedence with equivalent result fixtures rather than assuming every source path used this order. Security restrictions are not overridable business filters.

Resolve relative periods from named timezone and accepted logical time into half-open intervals. Preserve explicit/from-date/previous/rolling/schedule windows, first-occurrence and leap/DST policy. Retries retain their period. Assisted parameterization produces bounded draft changes and original-question/template/provenance lineage, never unrelated silent SQL changes.

Chart/KPI/table outputs preserve mappings, ordering, labels/legends, units/currency/percent formatting, comparisons/thresholds, safe options and expected schema. Decimal/large-integer values remain exact in transport/labels/evidence/exports. Truncated subtotals cannot appear as full-source totals. Required fourteen-kind visual coverage is in phase20; fallback must be labeled and is not full chart parity.

Narrative definitions pin type/instructions, allowed/redacted evidence fields, deterministic reduction, row/byte/character/call/token/time limits, prompt/model/schema versions, locale/tone and evidence/caveat requirements. They have no query/write tools, use the remote SDK gateway, validate claims and retain exact output. Later rendering never regenerates text. Failure/omission is reflected in report policy.

Safe text/Markdown is not arbitrary HTML. Grid/filter/widget/output references are unique and bounded. Presentation overrides cannot add scripts, external resources, unauthorized columns or security predicates. Dashboard redaction omits hidden names/data; authoring still requires valid pages.

## 6. Runs, idempotency and retained values

Reserve request key/canonical hash before resolving floating references. New work resolves once and seals the manifest. Replay uses it even after new publication; a changed request under the same key conflicts. Queue attempts are fenced. Retain bounded intermediate normalized results where needed to resume outputs without another source call. Reconcile indeterminate remote work or record another budgeted attempt rather than manufacture exactly-once evidence.

Equivalent-result reuse differs from request replay. Include exact definitions/output selection/parameters/window/locale/timezone, actual partition/privacy/freshness and relevant narrative/render versions in its key. Private previews stay actor/reach constrained. No tenant-only or raw-token-keyed shared results. Without reliable watermarks, state observation time/maximum age, not live freshness.

Artifact list/read/page/rendition needs a valid supplied Pengui JWT with target and actual partition reach; it does not query local IAM or require query-execute permission. Offline authority freshness is bounded by token expiry. Retention removes values and derived renditions while permitting minimal safe tombstones. Expired identifiers do not rerun old work silently.

## 7. Scheduling and delivery

One queue/occurrence engine supplies real cron/interval/manual tests. Reporting targets are reviewed saved SQL, explicit dynamic saved questions, pinned certified block/output selection and published report. Direct blocks create no hidden report. Default revisions are exact; explicitly latest-published resolves once per occurrence.

Phase06 delivers the actual thin Pengui authority adapter with its first durable consumer; phase30 reuses it for reporting targets (D-055). Admission validates signed target/dependency and binding-use authority. Store the opaque binding, not a bearer or local account. Dispatch/retry obtains fresh Pengui authority, verifies it normally and checks actual target/context plus business eligibility. The concrete platform binding/renewal API must be read/reused or extended in Pengui; this plan does not assert a newly described API already exists. Missing/denied authority blocks work without local signing, impersonation or ambient bypass.

Persist due instant/timezone/window/resolved revisions/overlap/missed-run decisions/attempts/idempotency. Tomorrow's retry does not change today's period. Specify first/missing/duplicate DST behavior and bounded catch-up. Use fences for commits. Missing approval/health/output leads to attention/blocked state, never automatic replacement of the pin.

Query success, retention, catalog publication and notification intent/receipt are separate. Catalog pull delivery is the baseline; recipients are neither authority nor email-sent evidence. Optional outbound notifications use existing Pengui integrations and durable effect/receipt state. Unsupported event/condition/condition-check/custom-code stubs are absent from registration and rejected on input; bounded maintenance stays internal.

## 8. API, Apps and rendering

All surfaces use the same domain services. Default reporting tools are search/describe/run/run-history/view. API/SDK provides full authoring/publication/certification/schedule management; new MCP mutation tools need a concrete consumer and the same explicit authority/confirmation rules.

The shared read viewer consumes versioned specs and artifacts through the established Apps bridge. Public assets contain no tenant values/platform/provider credentials. Test Chartworks metadata, loading/paging/filter/exact-value/error/private/trust/theme behavior and content safety—not host compatibility. Viewer31 does not depend on scheduling30; later scheduled results use its existing catalog.

Iframe uses an authenticated Pengui/client BFF which forwards a scoped JWT server-side. Chartworks returns authorized HTML/SVG/data. No local embed grants/bootstrap codes/cookies/token minting or bearer URLs. The serving BFF controls approved ancestors and user session behavior.

Go renders tables/KPIs/text; a pinned isolated ECharts worker renders SVG from sealed typed data, with no network/arbitrary URLs/scripts/source/model credentials. Bound resources and sanitize output. SSR must show promised chart content with client chart JavaScript disabled. Authorized JSON/CSV/HTML/SVG export is explicit; PDF/PNG/paginated document generation is later scope, not implied.

## 9. Delivery and closure

27/28 own blocks/frozen runs/artifacts;29 owns reports/hybrid/dashboard;30 owns reporting targets/delivery;31 owns Apps/viewer;32 owns SSR/BFF/export;33 owns guided onboarding;34 owns migration. Shared original phases own foundations/source/semantics/NLQ/evaluation/L2 engineering, with25 final release regardless of number.

The registry maps63 source rows and41 review gates to224 named criteria over34 phases. Q11 is explicitly discarded stubs; other required capabilities cannot disappear behind a demo or disabled flag. G41 adds Bifrost-only remote inference. Planning tests prove coherence, not live source/model behavior or completed migration. Runtime closure requires real named tests and applicable evidence.

## Phase05/06 implementation addendum (2026-09-05)

D-062/D-063 deliver the remote-only Bifrost gateway, fixed-input operator probes,
one PostgreSQL queue and bounded maintenance occurrences with the actual
[Pengui execution-authority v1 companion](docs/contracts/execution-authority-v1.md).
Metadata cancellation/read/pause do not require live models or an enabled worker.
Broader analytic/reporting targets and external-effect reconciliation remain owned
by their later phases; this is not a reporting release or live provider acceptance.
