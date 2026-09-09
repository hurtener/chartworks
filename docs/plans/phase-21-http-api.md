# Phase 21 — http-api

Status: shipped. Owner: internal/api. Hard dependencies: 01, 02, 03, 04. Current cumulative evidence: [phases 15–18 and 21](../reviews/phase-15-18-current-evidence.md).

The prerequisite shipped with merged PR #11. Cumulative endpoint coverage is
extended in this change for the merged NLQ/BYO consumers and phase-20 outputs.

## Authority and design

RFC-001 §11, D-044–D-050 and [COMMON.md](COMMON.md) apply. This shell lands early despite its phase number. Domain owners register their concrete endpoints as they implement them; do not wait until every domain exists to serve the first usable slice.

## Brief findings incorporated

Briefs 01, 04, 06, 14: thin HTTP surface, shared authority/enforcement, explicit transport limits, generated route/audit coverage and typed outcomes.

## Findings I'm departing from

No all-at-once late API phase and no local IAM/admin-issuance or embed-auth routes. Authenticated lifecycle/maintenance is not an invitation to recreate users/grants.

## Scope and implementation tasks

1. Land the authenticated HTTP shell early with health/capabilities, registration metadata, typed error mapping and hardened request handling.
2. Require each domain phase to register concrete endpoints, scopes, dependency loaders, audit semantics and public schemas in the same feature change.
3. Generate the OpenAPI contract and isolation/audit coverage from real registration; do not implement a parallel business service in handlers.

## Bounded implementation, 2026-09-07

`internal/api` now owns immutable registration metadata, DTO-derived closed wire
schemas, route matching and OpenAPI 3.1.1 generation. Its real consumers are all 33
existing source catalog, validation, execution, engineering and pipeline routes in
`internal/sourceapi`. The router, legacy manifest and generated document use the same definitions;
existing Pengui verification, resource loading, service enforcement, body limits,
error mapping and audit behavior remain in their existing handlers/services.
Resource-loader and audit labels describe those existing paths; they do not replace
execution callbacks or prove exhaustive isolation/audit coverage.

The [bounded evidence note](../reviews/phase-21-source-registry.md) records focused
HTTP/SDK/PostgreSQL and schema checks. Foundation, security and work adapters are
covered by the HTTP prerequisite continuation below; later domain phases continue
to extend the same registry. The continuation adds the actual acceptance parent
below; the earlier source slice did not use a placeholder parent.

## HTTP prerequisite continuation, 2026-09-08

The shared registry now composes the foundation health/readiness/capabilities and
OpenAPI routes with the existing security, gateway/work, source, engineering,
execution, pipeline and topic registries. Public operations are explicitly marked
unauthenticated; every protected definition carries its existing Pengui action,
resource-loader description, audit classification, closed request/response schema,
error mapping and body/query/header bounds. `GET` and `HEAD /openapi.json` are
served by the foundation handler from this immutable composition. The typed server
configuration defaults to `/` with an empty CORS allowlist; non-root prefixes are
rejected until route, OpenAPI and SDK joining can be delivered together.

`TestPhase21/AC01` through `AC06` exercise the actual composed registration, real
source HTTP/SDK/PostgreSQL behavior, security denial ordering, transport limits and
CORS, generated OpenAPI parity, absence of identity/reporting placeholders, and
concurrent public HTTP requests. The pinned Linux/native-parser race run and phase
acceptance runner passed all six children. This closes the HTTP prerequisite slice
while later domain phases continue to add their concrete registered operations.

## Non-goals

No business logic in handlers, fictitious endpoints, local key/bootstrap/grant/user services or token-in-URL authentication.

## Config and persistence

Server HTTP/CORS/body/deadline settings and public base path; no HTTP authentication provisioning endpoints. Choose one action spelling convention and generate its SDK/OpenAPI, not parallel colon/slash aliases. No new business store is owned here.

## Acceptance criteria

1. **AC01** — Bearer verification/resource enforcement wraps every registered protected endpoint, including render/export and private preview.
2. **AC02** — Wrong/missing identity, scope, tenant/path consistency and inaccessible resources produce safe stable HTTP outcomes.
3. **AC03** — Body/content/origin/time/size limits and cancellation behavior are explicit; no cookie/token-in-URL identity fallback exists.
4. **AC04** — Actual registration drives OpenAPI, auth/isolation/audit tests; a new route cannot omit its requirements.
5. **AC05** — Source/topic/query/reporting/operation owners add only implemented endpoints; no auth/grants/keys/bootstrap/embed-issuance route is registered.
6. **AC06** — Health/capabilities are real first consumers; typed HTTP services and concurrent request handling share the same core as SDK/MCP.

## Tests, coverage and smoke

Implement `TestPhase21/AC01` through `TestPhase21/AC06`. Registration tests enumerate currently implemented operations, and every later feature PR reruns them after registering its real endpoints. No placeholder route counts as completed capability. COMMON.md sets coverage; `scripts/smoke/phase-21.sh` requires all six results.

## Glossary, decisions and deviations

D-050 changes execution order, not phase IDs. The HTTP prerequisite runtime is
verified and PR #11 is merged. Phase 21 remains shipped; each new domain adds and
verifies its real consumers rather than claiming unimplemented endpoints.
Later domain consumers and release gates remain outside this prerequisite.

The phase 15 [private draft consumer](../contracts/topic-drafts-v1.md) and
[publication lifecycle](../contracts/topic-publication-v1.md) add fourteen concrete
operations through the same shared registry and generated schemas, with actual
HTTP/SDK/PostgreSQL/pgvector fixtures. These domain consumers extend the completed
HTTP prerequisite and do not by themselves close the remaining phase/release
criteria.

## Cumulative enforcement continuation, 2026-09-08

The acceptance inventory now includes enabled engineering/pipeline mutations,
NLQ routing/execution, BYO context/submission and all five chart operations.
`api.Guard` is installed at the production composition boundary: unregistered
paths cannot reach a handler; registered protected paths require current bearer
verification and their exact declared action before domain dispatch. Every
protected definition must declare 401/403 outcomes. Domain services still enforce
actual resource/dependency reach and mutation audit transactions; descriptive
resource-loader/audit strings are not executable security proofs.

Registry-driven denial tests enumerate the full implemented set, prove no domain
dispatch for missing/invalid bearer or missing action, and retain actual
HTTP/SDK/PostgreSQL resource checks. Generated OpenAPI uses closed request/response
DTOs, with required non-null chart scalars, nullable Go collections and optional
attempt receipts on interrupted ranking. Empty 405 responses match the existing
shared contract. The [review](../reviews/phase-20-21-adversarial.md) distinguishes
these cumulative improvements from still-unimplemented reporting/render/export
and MCP consumers; no placeholder is introduced for them.

## Phase-22 cumulative consumers, 2026-09-09

Three retained-metadata operations (`listTopics`, `listDatasets`, `describeDataset`)
now share the ordinary protected registry, DTO schemas and Go SDK with their MCP
bindings. Signed topic and every dependency restriction are applied before SQL
pagination; dataset access names its exact source/context. These operations neither
probe a warehouse nor expose private drafts. The checked-in source/topic manifests
and exhaustive registry tests include them.

`POST /v1/mcp` is a registered protected transport, not a guard exception.
`Definition.Surface` selects the current MCP intended audience and `mcp.use` at
the composition boundary; domain dispatch still requires its own action/resources.
OpenAPI describes that audience, bounded JSON-RPC request/response and empty 202
notification success. The bodyless 405 contract remains unchanged. NLQ preflight's
existing effect is corrected to paid routing plus persisted session evidence.

`TestPhase21` reruns cumulative denial/registration coverage, while `TestPhase22`
exercises the real HTTP/MCP/SDK services and per-request authority. The
[phase-22 review](../reviews/phase-22-adversarial.md) records evidence. No reporting,
render/export, Apps-viewer or credential-issuance placeholder is added.
