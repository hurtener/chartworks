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
4. Use [the actual operation manifest](chartworks-operations.json) to select the minimum action and addressed reach needed by this consumer. The [read-only example](../../examples/pengui-chartworks-scopes.json) is an illustrative scope set for a synthetic verified principal, not an API request that authenticates a tenant.
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

## Verification and boundaries

`TestPhase03/AC04` uses a synthetic signed fixture matching the inspected issuer serialization; `TestProviderRegistrationManifest` pins the published manifest/example to the actual registry and decoder. `TestPhase04` exercises the real PostgreSQL consumer through HTTP and the public SDK; `TestCompiledAuthorityLifecycle` builds and starts the actual binary with an ephemeral trusted TLS issuer, runs permitted operations, denies unsigned/foreign calls, and joins SIGTERM shutdown.

These tests establish consumer and serializer conformance. They do not claim a deployed Pengui session, production signing key, or customer connection was used. No external platform deployment or new platform API was created. MCP transport/tools remain phase 22; both intended-audience verifier paths already run against the same verification core and their current tests. The future reporting and source adapters must supply complete, server-resolved dependency/context metadata and apply selections before their own data access.
