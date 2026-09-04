# Reporting contracts and implementation ownership

Current design: RFC-002 and the active phase plans. Security is exclusively the [Pengui authority contract](../contracts/pengui-authority.md); no local roles/grants/token issuance or embed authority service exists.

## Objects and states

| Contract | Fields/invariants | Owner |
|---|---|---|
| Block/revision | Stable ID, localized question/aliases, exact topic/template, immutable published content, draft CAS, execution/outputs/dependencies/hashes | 27 |
| Validation/attestation | Real observed schema and exact content evidence; publication separate from certification/current health/withdrawal | 27 |
| Parameters | Nine preserved types, defaults/ranges/enums, bounded bindings, half-open resolved periods and provenance | 27, 28, 30 |
| Outputs | Chart/KPI/table/narrative, stable output IDs, safe bindings/formats, exact decimals and evidence | 20, 27, 28 |
| Run/attempt/artifact | Resolved references once, logical versus physical attempts, partition/privacy, usage, retained results and expiry | 06, 28 |
| Report/revision | Draft/review/published; grid/filter/widget definitions, explicit dynamic mode, partial policy and private preview | 29 |
| Dashboard/revision | Ordered exact report-revision page references and nondisclosing page redaction | 29 |
| Schedule/occurrence | Functional typed target, Pengui binding ref, exact due/window/revisions, failure/overlap/catch-up policy | 06, 30 |
| Rendition/export | Exact artifact, renderer/theme/version/viewport, static HTML/SVG or explicit data export; no execution side effect | 32 |
| External references | Neutral source identity/revision mapping, dry-run errors/quarantine, no imported credentials/authority | 34 |

Closed write schemas reject unknown fields, invalid IDs/references and impossible unions. Canonicalization is versioned. Hashes distinguish executable meaning, complete revision and rendition; server timestamps/provenance do not become executable authority. Store methods require tenant and verified operation context. Add tables with their first domain consumer, not one table for every conceptual noun.

## Scope families

Retain source/topic/dataset/pipeline/query/context/feedback/operational scopes. Reporting scopes are `reporting.read`, `reporting.sql.read`, `reporting.write`, `reporting.execute`, `reporting.preview`, `reporting.publish`, `reporting.certify`, `reporting.schedule.manage`, `reporting.export`. Each is combined with the Pengui-signed resource scope for the target/parent/dependencies. There is no `reporting.embed` mint permission because Chartworks issues no embed token.

Querying a dynamic widget also requires query permissions. Validation that actually queries requires execution authority. Reading an already-retained artifact does not require initiating-query permission. Creator labels, report audience text, widget origin, resource IDs and schedule binding references alone confer no access. A free-form `admin` claim is never an implicit bypass.

## Route inventory

Exact OpenAPI request/response structs and operation registrations are delivered with each owning phase; this table defines required actions, not presently implemented endpoints. All protected requests carry a Pengui bearer, use the same core and take expected-version/idempotency fields where applicable.

| Family | Required operations | Owner |
|---|---|---|
| `/v1/reporting-blocks` | Search/list/create; exact revision/outputs/SQL; draft patch; capture/duplicate assessment/period proposal; validate/preview/publish/reject/restore/archive; certify/withdraw; run | 27, 28 |
| `/v1/reports` | Catalog/create/get; revision/draft patch; submit/reject/return/publish/archive; exact private preview; published run/materialization resolution; health | 29 |
| `/v1/report-runs` | Metadata-only history/status; exact authorized result/output page; cancellation; retained artifact/rendition references | 28, 29 |
| `/v1/dashboards` | Create/get/list/draft/update/publish/archive and exact report-page references | 29 |
| `/v1/reporting-schedules` | Create/read/update/test/pause/resume/retire; occurrence and delivery-intent/history; attention/error status | 30 |
| `/v1/report-runs/{id}/render` | Authorized HTML/SVG render/read and versioned rendition metadata; no new query | 32 |
| `/v1/report-runs/{id}/export` | Explicit permitted JSON/CSV/HTML/SVG export, retention and limits | 32 |
| `/v1/onboarding` | Submit/status/answer unresolved slots/resume/cancel and references to ordinary reviewed artifacts | 33 |
| `/v1/imports` | Authorized neutral dry-run/validate/apply/status; unsupported record quarantine | 34 |

Action spelling follows one router convention chosen in phase 21 and generated into OpenAPI/SDK; no parallel colon/slash aliases are required. There are no auth/keys/grants/principals/bootstrap/embed-session routes. Schedule account creation and iframe bootstrap token examples from the earlier proposal are removed.

## Parameters and data semantics

Preserve required/optional distinction, explicit null versus omitted, allowed values, dimension references, locale/timezone and parameter source. Reject unknown/ambiguous assignments. Bind user values separately from approved query structure. The default precedence and import-equivalence obligation are in RFC-002 §5; mandatory access predicates never participate in override precedence.

An accepted occurrence uses stored logical time, not the retry clock. Preserve explicit range, previous periods, rolling periods, from-date and schedule-window forms. Exact decimals use declared lossless encoding; timestamps have offsets and dates remain dates. Null is not zero and boolean is not a number. Non-finite values, duplicate aliases and incompatible order/type/nullability fail explicitly.

One frozen query can produce several selected outputs. Required output failure follows explicit block/report policy; success with missing content is not complete success. Narratives may use only the sealed authorized result, and unsupported claims cannot be made correct by retrying SQL. Output data used for chart/table totals must advertise truncation.

## Operation acceptance and retries

Reserve `(tenant, authenticated actor/delegation, operation family, idempotency key)` with request hash. Resolve floating definitions/window/context once, seal manifest, then claim work with a fenced lease. Bound/reserve work and use fresh Pengui authority for durable execution. Store bounded intermediate rows when needed to resume formatting/narrative/render without repeating data execution.

On an indeterminate warehouse response, reconcile by driver query ID or record an explicitly new attempt. Retry cannot silently use a newer block or time period. Once a payload expires, same-key replay returns expiration/status rather than a new query. Result reuse is keyed on actual context partition/privacy and full resolved semantics, not report/tenant/question alone.

## Errors and privacy

Use stable typed codes for revision conflicts, validation/approval requirements, unavailable dependencies, invalid parameters/outputs, expired artifacts, insufficient signed scope and budget exhaustion. Return 401 for invalid auth, safe 403 for operation denial, nondisclosing 404 for inaccessible IDs, 409 for conflicts, 422 for malformed contracts; reveal 410 expiry only after authorization. Never expose raw SQL without its inspection scope, secrets, internal traces or hidden names.

Each subsequent API/MCP/render request validates its supplied JWT. Authority is only as current as the token's validity window; immediate offline revocation is not promised. Historical approval and current business health may be displayed separately without treating either as data permission.

## Testing and completion

`TestPhaseNN/ACxx` names in the active phase plans bind each contract to actual assertions. Source features map through `docs/plans/coverage.json`. JSON/OpenAPI shape checking and planning scripts do not establish runtime correctness; real PostgreSQL/source drivers, crash scenarios, UI/static output tests and owner-run cloud evidence close implementation gates.
