# Reporting domain and API contracts

Status: proposed contracts supporting RFC-002, 2026-09-04. Paths and examples below are a design specification, not a claim that routes or an OpenAPI implementation exist. The first implementation slice must generate and test the complete wire schemas from these contracts.

## 1. Contract conventions

Use a versioned, closed schema for write requests and discriminated unions. Reject unknown authority-bearing fields rather than silently ignoring them. All identifiers are opaque, bounded server identifiers; all references are resolved within the verified tenant before loading content. Public request bodies cannot choose the effective tenant, user, service principal, source credentials, or policy partition.

Instants use RFC3339 timestamps with an offset; named report timezones use a configured IANA timezone database. Calendar dates remain dates. Durations, row counts, byte counts, token counts and money/usage fields have explicit units. Null, empty, absent, zero and unavailable are not interchangeable.

Exact decimal data uses a decimal string plus column scale/precision metadata; large integers use lossless encoding rather than relying on a browser number. Binary, non-finite numbers and unsupported result types fail explicitly unless a declared safe representation exists. Chart geometry may use a numerical approximation, but labels, totals, exports and evidence retain exact values.

All mutations that can be retried take an idempotency key. Draft changes and state transitions also require the expected revision/ETag. The idempotency key is scoped to tenant, actor/authorized delegation and operation family; reusing it with a different canonical request produces a conflict.

## 2. Domain objects

| Object | Required responsibilities |
|---|---|
| Block identity | Stable ID, tenant, owning topic, slug, creator, current draft and latest published pointers; localized discovery metadata and aliases. |
| Block revision | Parent block, revision number, lifecycle, exact topic/template references, execution specification, outputs, dependency manifest, authorship provenance and hashes. Published payload immutable. |
| Execution specification | Approved SQL text, dialect/source binding, typed parameter declarations, expected columns/types/nullability, safety policy reference and budgets. Raw SQL is a separately protected projection. |
| Validation evidence | Exact revision/execution/dependency hashes, validator/adapter versions, observed schema, quality findings, query attempt reference, actor and timestamp. Store protected diagnostic details separately from the public summary. |
| Certification attestation | Exact revision hash, evidence references, reviewer, policy version, timestamp and current valid/stale/withdrawn status. Certification cannot be inferred from a lifecycle label. |
| Report revision | Identity, locale/timezone policy, presentation schema, filters, widget bindings, execution/partial-failure policies, review/publication metadata. |
| Dashboard revision | Stable identity, bounded ordered pages pointing to exact report revisions; no duplicate execution model. |
| Reporting execution | Typed target, immutable resolved manifest, attempts, lease/fencing state, outcome, counts/cost evidence and artifact references. |
| Retained artifact | Sealed payload plus data-policy partition, privacy, source/revision evidence, created/expiry times and format/version metadata. Not a rerun URL. |
| Schedule | Typed target, service identity reference, trigger, temporal-window policy, revision policy, failure/overlap/missed-run policy and delivery policy. No stored end-user JWT. |
| Delivery receipt | Logical occurrence/artifact/destination/effect key, channel state and provider receipt or failure; separate from query success. |

Logical objects do not require one table each. Reuse existing operations, grants, secret references and artifact seams where their semantics match. Do not hide reporting state inside an unbounded generic JSON job payload solely to satisfy an old table-count target. Foreign keys/unique keys include tenant context; identity/revision ownership, one accepted transition, and idempotency uniqueness are database-enforced as well as service-validated.

## 3. State machines

### Blocks

Draft edits increment the draft version. Validation records evidence for that exact version. Publishing compares the expected draft version and current evidence atomically, creates/seals the published revision and updates the published pointer. Further editing creates a new draft. Certification is a separate authorized action against an exact published revision. Supersession does not erase older revisions. Archival removes execution eligibility according to policy but preserves authorized audit metadata.

An approval withdrawal does not rewrite the original attestation or fabricate a historical failure. Store the withdrawal event and current status. A renderer distinguishes approval at execution time from current approval. Current authorization is always rechecked; a withdrawn certification is not automatically the same event as revoked data access.

### Reports and dashboards

Reports: draft -> pending review -> published, with explicit return-to-draft/rejection, amendment and archive actions. Public reads choose the published pointer only. Private preview requires an explicit preview endpoint, an exact private revision and author/manager permission. A general run request cannot use a `prefer_draft` switch to bypass this distinction.

Dashboards use draft/published revisions and exact page references. A reader may receive a redacted page list, including an empty visible list, while a newly authored dashboard still requires valid content. Counts and error messages must not reveal unauthorized page names or values.

### Hashes

Record a canonicalization version. Separate execution hash (approved query, source/dialect, parameter contract, relevant semantics and expected result schema), complete revision hash (also outputs and saved presentation/narrative definitions), and rendition hash (artifact plus renderer/theme/viewport). Authoring provenance and server audit timestamps are not executable authority. Derive dependency references during validation; a caller cannot omit a restricted relation from a supplied manifest and thereby authorize it.

## 4. Outputs and presentation

A block can expose several output definitions over one normalized result:

| Kind | Contract |
|---|---|
| Chart | Supported kind, column-ID bindings, stable ordering, label/axis/legend options, bounded series/categories and formatting tokens. No arbitrary JavaScript, URLs or executable formatter expressions. |
| KPI | Value binding, approved aggregation if any, unit/currency/percent metadata, comparison/target/trend/threshold policy and exact-value label. |
| Table | Allowed columns, titles/formats, stable sort, page size and explicitly defined totals. Totals over a truncated result must not be presented as full-source totals. |
| Narrative | Saved narrative type, allowed evidence fields, deterministic input reduction, row/character/token/call limits, prompt/model policy version, locale/tone, required caveats and claim evidence. |

Required outputs fail the block/report according to its explicit partial-failure policy. A successful query with an invalid chart mapping is not a fully successful chart. An empty result is a valid data state distinct from an execution failure. Every view shows truncation, missing widgets, stale approval and mixed source freshness where applicable.

The initial visualization conformance inventory includes area, bar, column, donut, grouped bar, heatmap, KPI card, line, pie, scatter, stacked bar, stacked column, table and treemap. Confirm the exact supported source contracts with fixtures before promising full visual parity.

A report presentation has a schema version, bounded grid, unique filter IDs and unique widget IDs. Widget union:

- `block`: block reference, revision/trust policy, output subset, parameters/bindings and narrow presentation overrides.
- `query`: explicit dynamic execution; either replayable question plus topic policy or a session-bound source-query reference. Never certified by virtue of report publication.
- `text`: escaped plain text or sanitized constrained Markdown, no data source.

A saved layout is not an HTML template. Validate grid bounds, references, ordering, unsupported overlap policy and responsive behavior. The canonical contract is not forced to a twelve-column limit solely because one input adapter used that layout.

## 5. Parameter and filter resolution

Parameter declarations include type, required/optional, default, allowed values/range, and whether callers may override them. Distinguish user-visible business filters from mandatory security predicates. A user-supplied country/team filter is never the authority enforcing row isolation.

Proposed canonical precedence, lowest to highest: block default -> report global parameter default -> widget literal default -> resolved declared filter binding -> explicitly permitted per-widget invocation override. Conflicting assignments at the same level fail. All assignments are typed before SQL binding; final bindings and their provenance are persisted. The importer must normalize old precedence into this contract and demonstrate equivalent results with fixtures, not assume the source used this exact order.

Only declared filter IDs and parameter mappings may be changed. Reject unknown fields, cross-widget binding references and any override of a security-reserved parameter. Identifier choices such as grain/sort/measure are validated enums mapped through approved query structure; they are not pasted into SQL from text.

Resolve relative periods using the report timezone and the accepted logical execution time. Windows are half-open `[start, end)`. Store both instants, timezone, period selector, resolution version and source of each value. Scheduled retries reuse the persisted occurrence window; caller-provided `schedule_context` is never scheduler authority. Ambiguous first-occurrence windows require an explicit start policy rather than an accidental zero-length period.

## 6. Authorization matrix

These are proposed operation scopes, additional to RFC-001. Token scopes gate operations; the current resolver gates referenced resources and data-policy partitions. No scope by itself is a data grant.

| Scope | Permitted family; remaining constraints |
|---|---|
| `reporting.read` | Discover/read authorized metadata, published definitions and eligible artifacts. Does not expose SQL or private previews. |
| `reporting.sql.read` | Read the approved SQL projection for otherwise authorized blocks. |
| `reporting.write` | Create/amend drafts and submit review requests within manage authority. Does not imply execution or publication. |
| `reporting.execute` | Execute eligible frozen blocks/reports; preview additionally needs exact private-revision authority. |
| `reporting.publish` | Publish/reject/return report or block revisions under expected-version checks and audience policy. |
| `reporting.certify` | Explicitly attest or withdraw approval. Reviewer separation follows configured policy; agents cannot synthesize this permission. |
| `reporting.schedule.manage` | Manage reporting schedules within delegated service/resource coverage; cannot select a stronger account to expand authority. |
| `reporting.export` | Export an authorized retained payload in a supported format; export policy may be narrower than on-screen display. |
| `reporting.embed` | Create a bounded viewer grant for an already authorized artifact/audience. Never an anonymous-publication shortcut. |

Executing a dynamic widget additionally requires the relevant `query.plan`/`query.execute` authority and authorized context. Validation that performs a real query also needs execution authority. Publication cannot grant access to source data, confer certification, or turn an origin hint into a trusted provenance claim.

For Pengui integration, the issuer-profile test fixture must prove the actual signed claim spelling, subject mapping, session policy, scope representation, exact audience handling and authorized delegation. Do not blindly copy either sibling: the inspected Stowage JWT path verifies without minting, while Soundings has self/external issuer modes and additional ACL/project authority sets. Chartworks' existing grant resolver remains authoritative for Chartworks resources.

Browser embed capabilities have a separate audience and read-only endpoint family. Service schedules store a durable service identity/grant reference and obtain or validate fresh execution authority as appropriate to deployment; they never replay a user's expired bearer. Revoked service identities or removed topic grants prevent further occurrences before values are produced or delivered.

## 7. Proposed HTTP surface

Use `/v1` and existing authentication/error conventions. The names below are the target operation inventory; generate a machine-readable route/scope registry in the first slice and drive isolation tests from actual registration.

| Route family | Operations |
|---|---|
| `/v1/reporting-blocks` | Authorized search/list and create draft. |
| `/v1/reporting-blocks/{id}` | Metadata and revision discovery; archive under explicit lifecycle policy. |
| `/v1/reporting-blocks/{id}/revisions/{revision}` | Exact definition; dedicated `/sql` protected projection. |
| `/v1/reporting-blocks/{id}/draft` | Compare-and-swap amendment; assisted parameter/duplicate-question proposal as separate bounded authoring operations. |
| `/v1/reporting-blocks/{id}/validate` | Evidence-producing validation, exact draft version and explicit execution authority. |
| `/v1/reporting-blocks/{id}/publish` | Exact-version publication. |
| `/v1/reporting-blocks/{id}/certifications` | Create/list explicit attestations; withdrawal addresses an exact attestation. |
| `/v1/reporting-blocks/{id}/runs` | Create governed execution; selected outputs/parameters and idempotency key. |
| `/v1/reports` and `/v1/reports/{id}` | Authorized catalog and report identity/definition management. |
| `/v1/reports/{id}/draft`, `/submit-review`, `/publish`, `/reject` | Explicit lifecycle actions with version checks. |
| `/v1/reports/{id}/previews` | Exact private revision preview, retained privately. |
| `/v1/reports/{id}/runs` | Published run creation or policy-eligible retained/in-progress resolution. |
| `/v1/report-runs/{id}` | Metadata/manifest and authorized artifact references; no new execution. |
| `/v1/report-runs/{id}/outputs/{output}` | Bounded retained result pages and rendition/export operations under current authority. |
| `/v1/dashboards` and `/v1/dashboards/{id}` | Versioned page collection and explicit publication; no duplicate report engine. |
| `/v1/reporting-schedules` | Create/list; exact target and service identity reference. |
| `/v1/reporting-schedules/{id}` | Read/update under expected version; explicit `/pause`, `/resume`, `/retire`, `/test`, `/runs`. |
| `/v1/reporting-embeds` | Create/revoke a restricted artifact-view grant; redemption/read family is separate from normal API authority. |

Long operations return an operation/run reference promptly and use the existing durable queue. Clients can poll or consume the already supported event transport; adding a second work system is unnecessary. Request cancellation before acceptance is different from canceling an accepted durable operation; expose an explicit cancel action and check current authority.

Machine-readable errors include a stable code, safe message, request ID and bounded details. Expected categories include `reporting.revision_conflict`, `reporting.validation_required`, `reporting.approval_required`, `reporting.unavailable`, `reporting.parameter_invalid`, `reporting.output_invalid`, `reporting.artifact_expired`, `reporting.policy_changed`, and `reporting.budget_exceeded`. Use 401 for invalid authentication, 403 for a missing operation scope, 404 for nondisclosing inaccessible identifiers, 409 for concurrency/policy state conflicts, 422 for invalid contracts, and 410 for an expired payload only after authorization. Do not return raw SQL, credentials, stack traces or private object names in ordinary errors.

## 8. Execution algorithm and idempotency

1. Authenticate and authorize the operation/target. Canonicalize the caller's request and reserve `(tenant, authorized actor/delegation, operation family, idempotency key)` transactionally. A different request under the same key conflicts.
2. For a new operation, resolve all floating references and temporal parameters once, calculate the effective policy partition, reserve budgets and seal the manifest. For an existing operation, reuse its manifest rather than resolving `latest` again. Recheck current authorization before exposing its state or payload.
3. Claim work with a lease and fencing token. Recheck eligibility and service grants. Execute only the approved source/dialect plan with current timeout, row/byte limits and mandatory source policy. Record adapter query IDs when supported.
4. Validate and normalize the result. Persist the bounded intermediate result when needed to resume output/render work without repeating the warehouse call. Do not retain unbounded raw rows by default.
5. Evaluate deterministic outputs. Run only explicitly declared bounded narratives through the existing model gateway, with reserved budgets and evidence checks. Persist their exact output and model/prompt versions.
6. Seal the artifact, counts, partial issues and timestamps. Publish the permitted discovery record. Enqueue a delivery effect only when the target policy permits it.

A transport timeout after a warehouse accepted work can leave the attempt indeterminate. Reconcile by adapter query ID where possible; otherwise a retry is a new recorded attempt and consumes budget. Fencing prevents an old lease owner from committing a newer owner's result. It cannot retroactively unexecute a remote query. Delivery likewise promises deduplicated logical effects where supported, not universal exactly-once email.

Equivalent-run reuse is a separate optimization from idempotent request replay. Its fingerprint includes exact resolved definitions, selected outputs, parameters/window, locale/timezone, policy partition, source freshness policy, and narrative/render versions when relevant. A shared result is never keyed only by report ID, tenant, or question text. Without a reliable source watermark, reuse advertises its observed timestamp and maximum age; it does not promise fresh underlying data.

## 9. Illustrative manifest

The example is intentionally synthetic and omits payload rows. `complete` means execution completeness, not a globally synchronized warehouse snapshot.

```json
{
  "schema_version": "chartworks.report-run/v1",
  "run_id": "run_demo_01",
  "report_id": "report_demo",
  "report_revision_id": "report_revision_3",
  "status": "succeeded",
  "complete": true,
  "visibility": "published",
  "source": "scheduled",
  "logical_time": "2026-09-01T03:00:00Z",
  "timezone": "America/Argentina/Buenos_Aires",
  "window": {
    "start": "2026-08-01T03:00:00Z",
    "end_exclusive": "2026-09-01T03:00:00Z"
  },
  "resolved_blocks": [
    {
      "block_id": "block_demo",
      "revision_id": "block_revision_7",
      "topic_revision_id": "topic_revision_4",
      "output_ids": ["value", "trend", "detail"],
      "query_origin": "approved",
      "approval_at_execution": "certified"
    }
  ],
  "warehouse_attempts": 1,
  "model_calls": 0,
  "issues": []
}
```

The public projection omits the internal policy-partition details, raw SQL and secret references. Current approval and current access are evaluated separately when the artifact is read.

## 10. Scheduling specifics

Supported initial triggers are cron and interval, plus explicit test-run operations. Each logical occurrence has a unique `(schedule, occurrence instant)` key. The definition declares timezone, start/end bounds, overlap policy, missed-run policy and maximum backlog. Defaults should be conservative: no overlapping execution and no unbounded catch-up. Fall-back duplicated wall-clock times and spring-forward missing times have explicit tested behavior. Retries preserve the selected occurrence; rescheduling future work does not mutate already accepted occurrences.

Typed targets: reviewed saved query; dynamic saved question; exact block plus output IDs; report with pinned/default or explicitly latest-published revision policy. Session-bound dynamic targets are denied unless current policy and available authorized context make them reproducible. A revoked certificate or missing pinned revision requires attention, not automatic substitution.

Separate run states, artifact-retention states and delivery states. Catalog-only delivery remains useful and supported. A destination address does not itself authorize its recipient to see the result. For outbound delivery, validate the intended data audience before producing/dispatching an attachment and use durable effect keys and receipts. Retention cleanup is a bounded system task; a custom-job field is not an arbitrary code execution API.

## 11. Retention, observability, and performance

Default logs and metrics contain identifiers, stages, durations, counts and stable reasons—not tokens, SQL, rows, prompts or raw results. Avoid per-user/report IDs as unbounded metric labels; use trace/audit records for those dimensions. Protected diagnostic detail has independent access and retention.

Benchmark cold startup, idle footprint, authorized discovery, frozen execution overhead excluding warehouse latency, concurrent isolation, result normalization, renderer throughput and schedule recovery separately. Do not translate a recorded Python retrieval POC into a promised Go speedup. Test limits at admission and during work: per-tenant and global concurrency, bytes, rows, model calls/tokens, warehouse attempts, runtime and artifact retention.

Deleting/expiring an artifact removes its values from all retained renditions and derived indexes according to policy. A minimal non-sensitive audit record may remain. Online authorization/revocation still gates a previously issued view grant. An authorized person who already downloaded a file cannot be made to forget it; export policy must acknowledge that boundary.
