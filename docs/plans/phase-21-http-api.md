# Phase 21 — http-api

Status: in_progress. Owner: internal/api. Hard dependencies: 01, 02, 03, 04.

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
HTTP/SDK/PostgreSQL and schema checks. Foundation, security and work adapters still
need migration, followed by cumulative generated coverage and public document
delivery. None of AC01–AC06 is claimed complete;
`TestPhase21/AC01`–`AC06` remain required. No placeholder acceptance parent was added.

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

D-050 changes execution order, not phase IDs. No runtime completion is claimed.

The phase 15 [private draft consumer](../contracts/topic-drafts-v1.md) adds seven concrete operations through the same shared registry and generated schemas, with actual HTTP/SDK/PostgreSQL fixtures. This domain consumer does not complete foundation/work/security adapter migration, public document delivery or the cumulative phase 21 acceptance criteria.
