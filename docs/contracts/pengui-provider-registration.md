# Pengui provider registration and operational consumer

Implementation contract, 2026-09-05. This document describes the already available Pengui minter seam and the first actual Chartworks consumers. It is not a new login, authorization server, or deployment claim.

## Verified issuer contract

The inspected platform `internal/minter/local/local.go` implementation (blob `1bf2110b251ff2351220e5fd9818e3a4fceed53c`) exposes `MintCapabilityUserToken(ctx, principal, destination, scopes)`. Its `signScopes` path emits `iss`, `sub`, `aud`, `iat`, `nbf`, `exp`, `tenant`, `user`, `session`, `scopes` and attribution-only `runtime_id`. The provider scopes are opaque to the issuer; the minter validates and sorts them. No Chartworks-specific minting endpoint or platform signer change is required to carry this grammar.

The exact current provider-scope ceiling is 32 unique strings, 256 bytes each, 4096 total string bytes; printable ASCII with no whitespace. An empty JSON array is valid authentication but grants no operation. Chartworks uses these same hard ceilings; deployment settings can narrow them, never widen them. Scopes arrive only in `scopes: []string`, not a singular `scope`, string-delimited fallback, header, body, or query parameter.

The normal audience is a scalar. Pengui can intentionally issue up to two unique exact audiences from its destination configuration. Chartworks verifies the intended audience for the selected HTTP/MCP surface. Configuring distinct audiences prevents cross-surface replay; using the explicit single-audience shorthand intentionally permits the same intended resource on both. Audience aliases must be registered by the operator, never derived from caller input.

`iat` is required because the actual issuer always provides it and Chartworks enforces maximum issued lifetime. `exp` is required; integer NumericDates and positive lifetime are checked. `nbf` is optional, validated when present; the issuer currently backdates it by 60 seconds. `sub`, when present, must equal `user`. Service-prefixed identities are attributed as services, not granted extra permissions. The generic *user* mint path rejects the reserved service namespace; unattended service mint/binding integration remains platform-owned work in phase 06.

A signed MCP registration envelope with `tenant_id`, `user_id`, `session_id` and `audience` is not a bearer. Chartworks explicitly refuses that alternative shape on the data-plane verifier. Similarly, generic `admin` or `capability:connect` scope does not grant the new operational permissions.

## Operator handoff

1. Register the Chartworks issuer/JWKS and intended HTTP/MCP audience from trusted deployment configuration. Chartworks never reads private issuer keys.
2. Derive the tenant/user/session principal through Pengui's normal verified session path. Choose destination and provider scopes from operator-approved capability policy, not user or model arguments.
3. Pass the approved scope strings through `MintCapabilityUserToken` (or the corresponding already-approved platform delivery path). Send the resulting bearer only in `Authorization` to Chartworks.
4. Use the executable manifests for [operations](chartworks-operations.json), [sources](chartworks-source-operations.json), [validated reads](chartworks-read-operations.json), [durable work](chartworks-work-operations.json), [uploads/profiling](chartworks-engineering-operations.json), [managed pipelines](chartworks-pipeline-operations.json), and [NLQ routing](chartworks-nlq-operations.json) to select the minimum action and addressed reach needed by the consumer. The [read-only example](../../examples/pengui-chartworks-scopes.json) is illustrative, not an API request that authenticates a tenant.
5. Consume the real operation through `sdk/chartworks` or HTTP. The SDK's token provider supplies a current Pengui bearer for each request. It never mints, stores for unattended replay, or upgrades credentials.

There are no default operator-wide scopes. In particular, `ops.metrics` is deployment-level aggregate observability permission and `ops.inspect` exposes enforcement diagnostics. Pengui should authorize those for intended operators separately from normal tenant analytical access. Neither is inferred from a user's name, service prefix, creator status, or tenant read scope alone.

## Implemented operation requirements

| Method / path | Action | Addressed reach |
|---|---|---|
| GET `/v1/retention-policy` | `ops.read` | `cw.tenant.read:<signed-tenant>` |
| PUT `/v1/retention-policy` | `ops.write` | `cw.tenant.write:<signed-tenant>` |
| GET `/v1/audit-events` | `ops.audit` | `cw.tenant.read:<signed-tenant>` |
| POST `/v1/retention-sweeps` | `ops.maintain` | `cw.tenant.erase:<signed-tenant>` |
| GET `/v1/access/diagnostics` | `ops.inspect` | `cw.tenant.read:<signed-tenant>` |
| GET `/metrics` | `ops.metrics` | `cw.tenant.read:<signed-tenant>` |

For `tenant` kind, the addressed ID must equal the signed tenant; an explicit whole-ID wildcard still refers only to the signed tenant. GET audit supports one `limit` query value in 1..1000. PUT requires exactly `expected_revision`, `audit_days`, `operation_hours`; POST sweep requires JSON `{}` plus one canonical `Idempotency-Key`. Responses use lower_snake_case SDK wire types. The request never accepts a tenant or actor override.

Diagnostics are computed from the actual route registry and current envelope only. They do not query hypothetical groups/roles, load private object names, or return the token/identity/scope sets. Domain publication and data safety remain independently enforced in their owning phases.

### Upload and profile operations

The engineering manifest records action/effect classification. Resource checks remain server-side and depend on the addressed object and accepted operation manifest: upload reserve/stage/load use `sources.upload` with source `write` and tenant `write`; erase and expiry sweep use `sources.erase` with source or tenant `erase` as applicable. Profile construction and dependency registration use `engineering.profile` plus the exact source, dataset and execution-context reach recorded by the profile. Retained upload inspection uses `sources.read`; retained profile/evidence/history/health uses `engineering.read`. Engineering operation inspection/cancellation additionally uses `jobs.read`/`jobs.cancel` and revalidates the original domain reach.

`uploads.enabled=false` removes the five upload mutations, and `profiling.enabled=false` removes the two profile mutations. The seven retained read/control operations stay registered. A task ID, profile ID or creator identity never substitutes for current signed resource/context reach. The SDK methods in `sdk/chartworks/engineering.go` call these same routes and obtain a current bearer from the caller's token provider.

### Managed pipeline operations

Phase 13 newly consumes four opaque action strings through the existing Pengui minter: `engineering.pipeline.write`, `engineering.pipeline.publish`, `engineering.pipeline.run` and `engineering.pipeline.read`. Operators must add them deliberately to the appropriate capability policy; their registration here is not a deployed grant. Proposal, draft, publication and run compose the existing `sources.query` action with exact external `source` and `execution_context` reaches. Draft, publication and run require declared `dataset` reaches, while proposal requires every governed dataset relation supplied to the model; no pipeline resource kind is introduced. Private derived stages use the accepted pipeline operation and checked manifest instead of separately minted output-source scopes. Publication remains distinct from draft write, and every external input is reauthorized before validation/execution. Retained run inspection/cancellation uses `jobs.read`/`jobs.cancel` plus the original pipeline and external dependency reach.

## Verification and boundaries

`TestPhase03/AC04` uses a synthetic signed fixture matching the inspected issuer serialization; `TestProviderRegistrationManifest` pins the published manifest/example to the actual registry and decoder. `TestPhase04` exercises the real PostgreSQL consumer through HTTP and the public SDK; `TestCompiledAuthorityLifecycle` builds and starts the actual binary with an ephemeral trusted TLS issuer, runs permitted operations, denies unsigned/foreign calls, and joins SIGTERM shutdown.

These tests establish consumer and serializer conformance. The phase 11/12 manifest parity check pins the published engineering inventory to `sourceapi.EngineeringRegistry(true, true)`; those phases are shipped. The phase 13/14 pipeline and source registrations are accepted at exact head `6883bc2103b2b870595222e623cd4d79bc01bc41` by [qualifying hosted CI run 34182486766](https://github.com/hurtener/chartworks/actions/runs/34182486766). These tests do not claim a deployed Pengui session, production signing key, customer connection or live model was used. No external platform deployment or new platform API was created. MCP transport/tools remain phase 22; both intended-audience verifier paths use the same verification core. Later reporting and source adapters must supply complete, server-resolved dependency/context metadata and apply selections before their own data access.

## Private topic draft consumer

The private draft operations in the [actual shared topic operation manifest](chartworks-topic-draft-operations.json) register `topics.write`, `topics.read` and `topics.export` through the existing opaque Pengui action seam. Operators must deliberately enable appropriate capability policies; this document creates no deployed grant. `topics.write` requires topic write and every source read, dataset query and execution-context use reach; creation also requires tenant write. Save, import, onboarding, entity mutation, dataset rebind and enhancement use existing secondary `sources.read` and `engineering.read` actions when they consult the actual source and private profile services. Enhancement uses the already configured `enhance` gateway role and introduces no action. Retained read/history/diff use `topics.read` plus topic read and the persisted dependency reaches. Export uses `topics.export` plus topic export and those same dependencies. Every draft revision remains private to its originating actor/session; the [draft service contract](topic-drafts-v1.md) defines its limits.

The bounded NLQ route in [its operation manifest](chartworks-nlq-operations.json) uses the existing `topics.read` action. Its service resolves every current topic dependency and execution context before Bifrost embedding or reranking; the route accepts no client-supplied DSN, source, or authority coordinate.

### Topic review and publication operations

The same manifest registers the new opaque actions `topics.review` and
`topics.publish`. Review requires topic publish reach plus every dependency reach
of the exact private draft revision. Publication requires topic publish, source
read, dataset query and execution-context use reach; live source discovery also
uses the existing `sources.read` action. Facet staging inside that operation uses
the publication action and cannot activate a generation independently. When reviewed
publication creates a new canonical entity revision, the same `topics.publish` action
also requires `cw.tenant.write:<signed-tenant>`; exact immutable revision reuse does
not. Draft/import preflight creates no registry state. Rollback
uses registered primary action `topics.publish` plus secondary action `sources.read`
for its live source discovery. Archive uses `topics.publish` and deliberately makes
no source discovery. Retained current/exact reads use `topics.read`; the current
source contract uses primary `topics.read` plus secondary `sources.read`. Public
publication DTOs exclude draft profile, actor and session provenance. The
[publication contract](topic-publication-v1.md) defines the atomic activation and
current-health boundaries. Retained health uses `topics.read` without discovery;
explicit recheck uses primary `topics.read` plus secondary `sources.read` and commits
only while the observed publication remains current. No issuer, local grant,
certification or execution
authority is introduced.

### Rule lifecycle operations

The same shared manifest registers rule draft, review, publication, current/exact
read, deterministic hard-constraint evaluation and retirement. These operations
reuse `topics.write`, `topics.review`, `topics.publish` and `topics.read`; no new
action is introduced. Each route requires its action-specific topic reach and all
source-read, dataset-query and execution-context-use reaches persisted by the exact
published topic before selecting or mutating rule payload. Rule publication
rechecks the current topic version in its transaction. Retirement validates the
retained pinned topic and active rule CAS so it remains available after topic
transition or archive. The
[rule lifecycle contract](rule-lifecycle-v1.md) defines the bounded evaluator and
remaining cumulative phase 16 work.
