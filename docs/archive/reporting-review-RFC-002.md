# RFC-002 — Governed reporting and portable delivery

Status: PROPOSED, 2026-09-04. Documentation only; no runtime capability is claimed.

This is a targeted amendment to RFC-001, not a replacement of its source, semantics, NLQ, or engineering architecture. Acceptance of this RFC supersedes only the conflicts listed below. Until acceptance and the documentation reconciliation gate in `docs/reporting/implementation-plan.md`, implementation must not treat the old rendering exclusion as compatible with this proposal. Other RFC-001 requirements remain in force.

## 1. Product decision

Chartworks is a governed analytics execution and publishing service. Its valuable reusable asset is not an attractive chart: it is an approved business question, executable definition, typed parameter contract, and traceable output that can be reused through an API, an agent, or an embedded report without asking a model to reinterpret the question on every refresh.

The end-to-end product path is:

```
Connect / upload -> inspect -> propose semantic definitions -> review / publish
        -> explore a question -> save a governed block -> validate / publish / certify
        -> compose a report -> run / schedule -> retain an artifact
        -> API data | MCP App | authenticated embedded viewer | server-rendered output
```

Not every customer needs materializations. Direct querying of a usable source is a first-class path. Managed data engineering is introduced when evidence justifies it, not as an obligatory medallion project before a first answer.

## 2. Explicit amendments

| Existing decision / passage | Amendment |
|---|---|
| D-013 / D-026 and RFC-001 rendering non-goal | Chart specification remains the stable core contract, but read-oriented rendering is in scope. No standalone authoring application is required. MCP App and iframe delivery are required product surfaces; genuine chart SSR is a separately testable capability. |
| RFC-001 source/topic/dataset grant grains | Extend the same resolver to blocks, reports, and dashboards; artifact access is derived from the retained artifact's privacy and data-policy partition. Do not introduce a second IAM system. |
| RFC-001 token scopes | Add the reporting operation families in `docs/reporting/contracts.md`. SQL inspection, execution, authoring, publication, certification, scheduling, and artifact reading remain distinct. |
| RFC-001 external claims use prefixed `sub`, optional `session` | Add an explicit issuer-profile adapter. Pengui's verified tenant/user/session convention must interoperate without pretending that its raw subject already has Chartworks' internal principal prefix. Standalone API-key subjects retain their own documented profile. |
| Chart-only planning / HTTP / MCP phase coverage | Add block governance, report composition, dashboards, durable artifacts, scheduling targets, and delivery conformance. A chart-spec phase alone cannot close reporting parity. |
| D-038 layered SQL validation | Retain native-dialect generation and independent read credentials. Clarify that EXPLAIN, successful parsing, or a SELECT keyword alone is not a read-only/security proof; adapter-specific function, relation, and policy controls are still required. |
| D-039–D-041 atomic proposal / revert terminology | A metadata publication can be transactional. Multi-engine DDL and external pipeline effects are not assumed to be one distributed transaction. Require staged application, durable steps, compensation where possible, and honest partial-failure states. |
| D-042 investigations sequencing | New internal investigation orchestration may remain later. Existing source capabilities must not be relabeled as new investigations and silently deferred; multi-topic and follow-up parity need explicit disposition. |
| Existing table and phase budgets | Re-budget required reporting state explicitly. Neither a historical table cap nor 25/26 planning smoke scripts is evidence of reporting completeness. Reuse the queue and access model rather than multiplying workers or stores. |

The post-merge D-043 provider-role amendment is relevant and should be reconciled into the accepted baseline: configurable providers by role, optional reorder-only reranking with observable fallback, and reuse of established gateway behavior. This RFC does not represent unmerged branch work as already present on main.

## 3. Domain boundaries

Chartworks owns source definitions and references to secrets, semantic versions, query validation/execution, blocks, reports, dashboards, executions, retained artifacts, and reporting schedules. Pengui owns product entitlements, organization/project identity projection, white-label configuration, and its integration credential broker. Harbor owns agent sessions and orchestration. Product integrations use public APIs/protocols; neither Pengui nor Chartworks reads another service's internal database.

A report artifact is authoritative in Chartworks. Harbor may retain its reference in a conversation; Pengui may keep a rebuildable discovery index. Neither becomes the sole copy of the artifact. Any durable transfer to a different artifact store must declare custody, retention, and deletion responsibilities instead of silently creating two authorities.

Stowage and Soundings remain separate capabilities. Retrieving business documentation or conversational memory may assist authoring, but neither is a mandatory dependency of a frozen report refresh.

## 4. Two execution lanes

### 4.1 Exploration

The exploration lane preserves language understanding, authorized topic selection, bounded context packing, semantic retrieval, templates, clarification/refinement, dialect-aware generation and validation, bounded corrective attempts, feedback, evaluation, and agent-facing signals. An external agent may use the context/submit split instead of Chartworks' internal generator. Every submitted query still receives the same validation and authorization.

A result can be proposed as a block. This is a new authoring operation, not automatic certification of a successful answer.

### 4.2 Governed execution

A published block revision binds one approved query definition to typed parameters, an expected result schema, a dependency manifest, and one or more saved outputs. Execution resolves the revision once, checks current authority and health, binds values, executes the approved query, validates the result, and evaluates selected outputs from that same result.

Frozen execution must not invoke question interpretation, topic routing, SQL generation, query rewriting, or automatic chart selection. Model use is zero unless a saved bounded narrative output explicitly permits it. Narrative generation cannot modify or regenerate the query.

“One query, several outputs” is a logical execution contract, not an exactly-once guarantee across a warehouse/network crash. Retries and indeterminate attempts are recorded honestly. Viewing an existing artifact must never issue a warehouse query or model call.

### 4.3 Explicit dynamic widgets

Preserve dynamic query widgets as a separate, opt-in lane. Distinguish a replayable question from a session-bound reference. Session-bound execution needs the originating authorized context and an explicit scheduling policy; it must not be silently reconstructed under a service account.

A published report may contain dynamic widgets, but publication does not certify their changing SQL or results. Origin and trust are calculated by the service, never accepted from a caller-supplied badge. Disabling dynamic execution returns an explicit per-widget issue, not an unnoticed replacement by a frozen block.

## 5. Revision, publication, certification, and health

These are independent concepts:

- A draft is editable under optimistic concurrency. Published execution definitions are immutable; an amendment creates a new draft.
- Publication makes an exact revision eligible for its configured readers and execution policy. It does not grant source access or prove business correctness.
- Certification is a review attestation bound to the exact content/dependency hash and validation evidence. It can become stale or be withdrawn without rewriting historical evidence.
- Health reflects current source availability, permissions, schema/semantic compatibility, and dependency state.

Reports retain draft, review, and published states. Previews remain private even after a later report revision is published. Dashboards are lightweight versioned collections of exact report revision references, not a second visualization or execution engine.

Dependency changes are classified as cosmetic, amendment-eligible, review-required, or unavailable. Even a mechanically detectable rename produces a new draft plus validation; it never rewrites an approved query during refresh. Semantic definition changes always require human publication approval. Historical artifacts retain their original evidence and show current approval/availability status separately.

## 6. Report composition and artifacts

A report revision contains a bounded grid, frozen/dynamic/text widgets, declared filters and parameter bindings, presentation overrides, locale/timezone policy, and partial-failure policy. Support the source's ordinary chart, KPI, table, and bounded narrative outputs. A text widget is safe text or constrained Markdown, not arbitrary HTML or script.

A run resolves all floating references once into a manifest before execution. The manifest records report/block/topic revisions, selected outputs, resolved parameters and temporal window, actor/service attribution, data-policy partition, source observation times or watermarks, execution evidence, and renderer/model/prompt versions when used. A dashboard's page order and exact report references are likewise versioned.

An immutable artifact is the retained result of a particular execution, not merely a saved URL to rerun a report. Pinning definitions does not pin warehouse data; a later execution can legitimately produce different values. Cross-source execution is not described as one transactional data snapshot unless the adapters can prove that property. Expose per-source observation times and warn when freshness is mixed.

Artifact immutability lasts until retention/deletion policy removes sensitive payloads. A minimal audit tombstone can survive; an expired payload must not be silently recomputed under its old identifier. Rendering a retained artifact may be repeated without data/model execution, using the recorded renderer version or an explicitly labeled compatible rendition.

## 7. Authorization and data isolation

All surfaces call the same domain services and access resolver. Authorization applies before discovery, execution, result retrieval, rendering, export, and schedule delivery—not only when a report is created.

The hosted profile validates asymmetric signatures, configured issuer, intended per-surface audience, expiration, applicable not-before/issued-at rules, mandatory identity fields, and bounded claims. Unknown key IDs and stale verification material fail closed. Never trust body/header tenant overrides, browser-supplied principal prefixes, or an unsigned agent identifier. Do not store a user's short-lived token as a schedule credential.

The Pengui issuer adapter maps the verified tenant/user/session triple into the internal identity envelope and checks any simultaneous subject representation for consistency. Agent identity is either a separately authorized service principal or an explicitly verified delegation constraint; merely naming an agent must not inherit its owner's authority. Scope syntax and token claims are pinned by issuer-profile contract tests, not guessed from sibling services.

A snapshot's data-policy partition includes the tenant, effective source/row/column policy, credential execution context, and approved audience policy. A caller may receive it only if the current resolver proves access to that partition. A service account producing a wide result does not make all its rows safe for every tenant member. There is no fetch-wide-then-filter-in-JavaScript path. V1 denies cross-partition artifact reuse unless an explicit governed audience policy proves it safe.

Artifact viewing does not require permission to author SQL or initiate another query, but it does require current data entitlement. Preview privacy and SQL visibility remain separate checks. Embedding never creates anonymous sharing implicitly. Existing grants and an approved delegated audience policy—not a new ad hoc ACL subsystem—carry any future report-only sharing.

## 8. Scheduling and delivery

Reuse the existing durable leased queue and scheduler. First-class targets are a reviewed saved query, an explicitly dynamic saved question, a pinned block/output selection, and a report. A block schedule does not create a hidden report. Cron and interval triggers are required; event/condition triggers found as source stubs are documented extensions, not evidence of implemented parity.

New schedules default to exact published revisions. A latest-published policy is explicit and resolves once per occurrence. Scheduled execution uses a current authorized service identity with only the necessary resource coverage. Creating a schedule cannot enlarge the caller's authority by selecting a stronger service identity.

The logical occurrence stores its due time, timezone, resolved half-open window, target revisions, and idempotency key. Retries do not change the period to the current clock. Define overlap, missed-run, daylight-saving, retry, timeout, cancellation, and fencing behavior. Apply budgets before and during work, including model/warehouse retries—not only after an expensive run completes.

Execution success, artifact retention, and external delivery are separate states. The authenticated run catalog is a supported delivery channel. A list of recipients is not proof that email was sent. Outbound delivery is an optional adapter or Pengui integration using a durable effect record, retry policy, and provider receipt; never an implied feature of catalog publication.

## 9. Rendering without a builder application

Ship a small read-oriented renderer with three adapters: MCP host bridge, authenticated embedded viewer, and server rendering. They consume the same versioned report/artifact contract. Theme configuration is constrained data; no per-customer code fork or arbitrary user JavaScript.

API-first does not mean zero browser code: visual MCP Apps and interactive iframes require a viewer. Serving an HTML shell that later draws charts in the browser is not chart SSR. The baseline may render tables, KPIs, and text in Go; full static chart SSR can use an isolated, pinned ECharts SVG renderer. This does not move query execution or governance out of Go. No headless browser is required for initial SVG support; pagination/PDF fidelity is a later separately tested capability.

MCP Apps support, iframe authentication, CSP, and exact host compatibility are specified in `docs/reporting/delivery.md`. Do not equate ordinary MCP tools with working MCP Apps. Unsupported hosts still receive useful structured/text results.

## 10. Onboarding and managed engineering

Offer a resumable connect -> inspect -> propose -> validate -> review -> publish flow. Separate operational setup (configuration, connectivity, permissions, secret references) from semantic inference and optional materialization. The model cannot provision arbitrary infrastructure or manufacture credentials.

Profiles and inferred joins/measures/dimensions/KPIs carry evidence, confidence, and unresolved questions. Capture grain, cardinality, units/currency, aggregation behavior, temporal meaning, null policy, and sensitive fields. Missing semantic output is explicit incompleteness, not silently approved default semantics. Use bounded batches and stable identifiers; regenerate affected definitions rather than an entire workspace unnecessarily.

Retain the established managed-write boundary: customer baseline objects remain inputs only. A name beginning with a managed-schema prefix is insufficient; the managed-object registry and database privileges must prove ownership. Separate engineering credentials and adapters from read execution. Staged proposals, quality checks, lineage, safe publication switches, and dependency-aware compensation precede policy-limited autonomy. Semantic publication never becomes autonomous.

## 11. Deliberate limits

Use one Go domain service and existing PostgreSQL queue/access model. Add narrowly typed reporting persistence, not a generic workflow platform. Reuse the gateway for optional narrative calls. Add an object-store driver when artifact size/retention needs it; do not require a distributed data plane for a first deployment.

Do not build a drag-and-drop application, arbitrary code widgets, custom JavaScript formatting, a new OAuth authorization server, an email campaign system, unrestricted custom schedule jobs, or a general-purpose orchestration engine. Do not postpone source parity behind L3 autonomy or a new internal analyst agent.

## 12. Acceptance

The evidence ledger in `docs/research/14-reporting-parity-audit.md` and acceptance matrix in `docs/reporting/implementation-plan.md` define closure. An observed source model or test is not proof that Chartworks implements it. Every required capability needs a target contract, executable test, and migration disposition. Planning-only skips cannot satisfy release gates.

The first demonstrable slice is: verified identity -> approved topic -> block draft -> validation -> publication/certification -> report -> durable run -> API artifact -> MCP/iframe view -> scheduled occurrence -> revocation test. This is a bounded product proof, not the complete migration release.
