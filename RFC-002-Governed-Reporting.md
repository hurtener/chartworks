# RFC-002 — Governed reporting implementation contract

> Status: implementation design, revised 2026-09-04 after the owner merged the reporting analysis.
> RFC-001 controls common security/architecture; this file controls reporting semantics. Active phases 27–34 implement this contract together with the revised original phases.
> The prior proposal is preserved under `docs/archive/`. Design acceptance never means runtime tests have passed.

## 1. Product boundary

Explore a new question through NLQ; turn a useful result into a reviewed reusable definition; execute it predictably; retain a trustworthy result; consume that result through API, MCP Apps, iframe or static rendering. A report builder application is not required. Pengui/Harbor Apps support is established and is not a research or qualification task.

Pengui owns all identity and access decisions. Chartworks validates Pengui JWTs, applies their signed scopes/resource restrictions and enforces its own business/data-safety invariants. No local token issuer, role/grant system, service-account registry or embed credential service is added. See `docs/contracts/pengui-authority.md`.

## 2. Domain and custody

A block identity has localized metadata, canonical question/aliases, creator provenance and draft/published pointers. Its revision contains exact topic/template references, SQL/execution definition, typed parameters, expected ordered result schema, dependency manifest and saved output definitions. Validation evidence binds exact content/dependencies/schema to a real checked execution. Certification is a separate attestation.

A report revision contains ordered/grid widgets, filters, bindings, safe presentation settings, locale/timezone and execution/partial policies. A dashboard revision is an ordered collection of exact report-revision page references. It is not another query engine.

A run seals resolved definitions, parameters/window, selected outputs, actor/service attribution, actual execution context/data partition, source observation times, attempts, usage and output/renderer/model versions. The retained artifact contains the run's values and evidence. Chartworks is authoritative for it; Pengui/Harbor store references or rebuildable discovery metadata, not a divergent authoritative copy.

Definitions can be immutable while the source data changes. A retained artifact records one execution; rerunning the same revision can produce new values. Multiple sources are not claimed to share one transaction snapshot without explicit engine support. Show observation/freshness evidence rather than implying synchronized data.

## 3. Lifecycles and business trust

Blocks retain draft, published, superseded and archived lifecycle semantics. Editing publication creates a new draft. Validation and publication are separate actions with expected-version checks. Publication requires fresh evidence for the exact relevant content; it does not confer certification, source access or a broad audience.

Certification is bound to the exact revision and evidence. Current valid/stale/withdrawn/unavailable status is separate from historical approval. Preserve published/certified-only/explicit-stale/private-preview execution policies. Allowing stale business approval never allows expired JWTs or broader data access.

Reports retain private draft -> pending review -> published, explicit reject/return/amendment and archival. Previews require an exact private revision and Pengui-issued preview authority. A public run endpoint cannot accept a prefer-draft switch. The artifact's private status is persisted and cannot become public because the report later publishes.

Use canonicalization versions and separate execution, complete revision and rendition hashes. Derive dependency manifests during validation; an author cannot omit a restricted dependency to authorize execution. Cosmetic changes, exact rename candidates, meaning changes and unavailable dependencies have explicit impact states. Even an exact rename creates a new draft and revalidation; no approved SQL changes during refresh.

## 4. Two explicit execution lanes

Exploration retains routing/context/generation/clarification/refinement, templates, validation, bounded correction and feedback. Capturing its result into a block is an authoring proposal, never automatic certification.

Frozen execution resolves an eligible published revision, checks signed authority/dependency health, resolves typed parameters, validates the approved query for the actual context, executes it and fans out selected saved outputs from one normalized logical result. Interpretation, retrieval, routing, SQL generation/correction and chart selection are forbidden here. Zero model calls is the default; an explicitly enabled saved narrative is the only bounded post-query exception.

One logical query serving several outputs does not promise physical exactly-once execution after a network failure. Record attempts and reconcile query IDs where possible. A new viewer opening an existing artifact always produces zero SQL/model calls.

Dynamic widgets remain supported by explicit opt-in. A replayable question has a different durability contract from a query tied to an authorized originating session. Normal query safety, scope and budgets apply. Publication of the enclosing report does not certify changing SQL. Provenance/trust is produced by the service, not accepted from a widget's origin label. Session-only scheduling needs explicit authorization and available context; otherwise it fails clearly.

## 5. Parameters, outputs and presentation

Preserve date, datetime, relative period, dimension value, number, integer, boolean, time grain and top-N types; required/default/range/enum rules; locale/timezone and resolution provenance. Bind values, not SQL fragments. Identifier-like choices map to a closed approved enum/structure.

Canonical precedence is block default -> report global default -> widget literal -> declared filter binding -> permitted invocation override. Conflicting assignments at one level fail. The migration adapter must prove equivalence for older precedence by normalizing its inputs, not assume this new ordering matched every source path. Mandatory data restrictions are outside overridable business parameters.

Relative ranges use named timezone plus logical execution time and half-open `[start,end)`. Preserve explicit/from-date/previous-period/rolling/schedule-window behavior, leap/DST handling and first-occurrence policy. Retries retain the original accepted window. Assisted period authoring proposes a limited draft change and preserves original question/template/provenance; unrelated query structure cannot change silently.

Chart, KPI and table outputs preserve bindings, labels, legends, ordering, units/currency/percent formats, comparisons/thresholds, safe options and schema constraints. Decimal and large-integer values stay lossless in API data, labels, evidence and exports. Do not label a truncated table subtotal as a complete source total. Required chart kinds are enumerated in phase 20; an explicit fallback does not count as full visual parity.

Narratives have saved instructions/type, approved input columns, redaction, deterministic reduction, row/character/call/token/time ceilings, prompt/model version, locale/tone and evidence requirements. They have no query or write tools. Validate claims against retained evidence; retain exact output and mark failure/omission according to policy. Rendering the narrative later never regenerates it.

Safe text/Markdown is not arbitrary HTML. Grid/filter/widget/output references are unique and bounded. Presentation overrides cannot introduce external URLs, executable code, unauthorized columns or security predicates. Dashboard pages can be redacted without exposing hidden names/data; the authored dashboard still needs valid content.

## 6. Runs, idempotency and retained values

Reserve the idempotency key with a canonical request hash before resolving floating references. A new accepted run resolves everything once and seals its manifest. A replay reuses that manifest even after newer publication; a changed request under the same key conflicts.

Use the existing queue/attempt/fencing primitives. Retain bounded normalized intermediate results when necessary to resume output work without repeating the warehouse call. When the remote outcome is indeterminate, reconcile or record a new attempt; never fabricate a once-only guarantee. All retries consume the relevant budgets.

Equivalent-result reuse is distinct from request replay. Its key includes definitions, selected outputs, parameters/window, locale/timezone, actual context partition, privacy, freshness policy and narrative/render versions. Private preview reuse is also actor/reach constrained. No cross-context tenant-only cache. If a reliable watermark is unavailable, describe maximum age and observation time, not live freshness.

Artifact list/read/page/rendition operations require current supplied Pengui JWTs and matching target/context reach. They do not independently query an IAM store or require query-execute permission. JWT expiry bounds authority freshness; offline signature validation does not imply immediate revocation. Retention removes sensitive values/renditions and may leave a minimal audit tombstone. An expired identifier does not recompute its old result.

## 7. Scheduling and delivery

Use one scheduler/queue with real cron/interval/manual-test support. Targets are reviewed saved SQL, explicit dynamic saved question, pinned certified block plus output selection, and published report. Direct block scheduling creates no hidden report. Default pins are exact; explicit latest-published is resolved once per occurrence.

Schedule creation requires the caller's signed target/dependency permissions and use of the Pengui execution binding. Store that opaque binding, not a bearer token or local service account. At occurrence/retry, obtain a fresh Pengui token through its existing broker integration, verify it normally, then enforce target/context reach and business eligibility. No local mint, impersonation or stronger-account selection fallback exists.

Persist due time, timezone, window, revision resolution, overlap/missed-run decision, attempts and idempotency. Retrying tomorrow does not change today's report period. Specify first/duplicate/missing DST occurrence behavior and bounded catch-up; use fencing to protect commits. Health/certification loss marks needs-attention/blocked rather than silently following a different revision.

Run success, artifact retention and delivery intent/receipt are separate. The authenticated run catalog is the baseline delivery channel. Recipient metadata is neither a data grant nor proof of email. Optional outbound notification uses an existing Pengui integration and a durable effect/receipt; no new mail system belongs here. API returns unsupported for discarded event/condition/condition-check/custom-code stubs; bounded maintenance remains internal.

## 8. API, Apps and embedded rendering

The same domain services serve all surfaces. Read/search/describe/run/run-history/view are the narrow default reporting MCP tools. Authoring/publish/certify/schedule management is fully available through API/SDK; additional MCP mutations require a concrete consumer and the same explicit scope/confirmation rules.

The shared viewer consumes versioned specs and retained artifacts through the established Apps bridge. Resource assets contain no tenant values or platform credentials. Test Chartworks tool/resource metadata, paging, filters, value fidelity, error/private/trust states and UI behavior—not whether Harbor/Pengui support Apps.

Iframe integration uses Pengui's/client's authenticated BFF route, which forwards a scoped Pengui JWT server-side. Chartworks supplies authorized HTML/SVG/data. There are no local embed grants, bootstrap codes, cookies, token minting or bearer-bearing iframe URLs. The serving BFF applies allowed frame ancestors and its ordinary user/session controls.

Go can render tables/KPIs/text directly; an isolated pinned ECharts SVG worker renders charts from sealed typed data. No arbitrary URLs/scripts, network or source/model credentials enter that worker. Enforce time/memory/concurrency/output limits and sanitize output. Genuine chart SSR must remain visible with client chart JavaScript disabled. JSON/CSV/HTML/SVG exports are explicit authorized outputs; PDF/PNG/paginated documents are not implied.

## 9. Actionable delivery and closure

Phases 27/28 deliver governed block lifecycle and frozen execution/artifacts. Phase 29 adds report/hybrid/dashboard composition. Phase 30 adds functional reporting schedules and delegated authority consumption. Phase 31 supplies Apps/tools/viewer. Phase 32 supplies SSR/BFF iframe/exports. Phase 33 supplies guided onboarding and phase 34 migration parity/cutover. Revised phases 01–26 own shared infrastructure, sources/semantics/NLQ, API/SDK foundations, evaluation and L2 engineering. Phase 25 is the final release gate, despite its number.

The registry and coverage map under `docs/plans/` assign all prior 63 capability rows and 40 review gates to named acceptance tests or an explicit discarded-stub disposition. Required features cannot vanish behind a preview milestone, feature flag or a source file inventory. Every runtime criterion needs real evidence; planning checks alone close none of them.
