# Phase 03 — auth-identity

Status: shipped. Owner: internal/auth, internal/identity. Hard dependencies: 01.

## Authority and design

RFC-001 §4 and `docs/contracts/pengui-authority.md` are the security contract. [COMMON.md](COMMON.md) supplies the implementation/testing workflow. This phase is a token verifier and envelope decoder, not an authentication provider.

## Brief findings incorporated

Briefs 04, 14: asymmetric verification, signed identity, bounded key refresh, safe failures and immutable per-call context. Keep these outcomes without copying a sibling's local issuer.

## Findings I'm departing from

D-044 removes both issuer-driver modes, API-key exchange, signing secrets, local bootstrap and local service-account creation. Pengui alone issues authority. Apps compatibility is established and unrelated to this work.

## Scope and implementation tasks

1. Implement a verification-only Pengui JWT middleware and immutable envelope for HTTP/MCP/in-process clients; use the single documented identity/scope contract.
2. Wire trusted JWKS refresh and key rotation with bounded staleness, token/claim size limits and algorithm/key matching; verify temporal and issuer/audience claims.
3. Provide the provider-scope registration/consumer handoff to Pengui using its existing minting seam. No issuer profile framework or alternate auth mode.

## Non-goals

No local issuer, token renewal/signing, key exchange, login, grants, OAuth service or user/service-account store.

## Config and persistence

`auth.issuer`, `auth.jwks_url`, `auth.audiences.http/mcp`, `auth.algorithms`, `auth.max_token_lifetime`, `auth.clock_skew`, `auth.jwks_max_stale`, token/claim byte ceilings. Issuer/audiences are required; no signing configuration. Verification keys may be cached with freshness metadata, never private keys or user-policy records. Exact defaults are documented in the typed configuration reference.

## Acceptance criteria

1. **AC01** — Reject HS/none, bad signatures, algorithm/key confusion, token-provided key URLs, unknown keys and over-stale verification material.
2. **AC02** — Validate mandatory tenant/user/session/exp/intended aud and consistent subject; malformed nbf/iat or excessive lifetime is rejected.
3. **AC03** — Header/body/query actor or tenant values never replace verified claims; parsed envelope and slices cannot mutate shared state.
4. **AC04** — Scope decoding is bounded, unambiguous and pinned to the provider contract with a Pengui-shaped signed fixture, including service attribution.
5. **AC05** — No production signing/mint/exchange/bootstrap/API-key endpoints, issuer keys or fallback login can be constructed or registered.
6. **AC06** — Key rotation, concurrent reuse and token fuzzing preserve fail-closed behavior; expiry is tested without claiming instant offline revocation.

## Tests, coverage and smoke

Implement `TestPhase03/AC01` through `TestPhase03/AC06`, using test-only ephemeral signers and the real verifier/key loader. Test JOSE/claim malformed input and concurrent key rotation. COMMON.md requires 85% auth/identity coverage. `scripts/smoke/phase-03.sh` requires all six results; missing/skipped tests fail runtime acceptance.

## Glossary, decisions and deviations

D-044/D-045 define sole-issuer ownership and direct enforcement. Register the new provider scopes with their first Pengui consumer; do not claim they are already configured. No implementation completion is claimed.

## Implementation record — 2026-09-05

The six named acceptance criteria have real Go assertions and first consumers. See [provider handoff](../contracts/pengui-provider-registration.md) and [adversarial review](../reviews/phase-03-04-adversarial.md). No production platform credential or deployed session was used; issuer-shaped synthetic signing fixtures feed the actual verifier and PostgreSQL API/SDK consumer. No local policy database or issuer was added.

`internal/auth` now owns the key cache shared with readiness, bounded JWT decoder and golang-jwt cryptographic verification. HTTP and MCP intended audiences are exact; the full MCP transport stays phase22. Token scope limits match the actual provider minter: 32 / 256 bytes each / 4096 bytes total.
