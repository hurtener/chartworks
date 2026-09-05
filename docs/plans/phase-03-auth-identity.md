# Phase 03 — auth-identity

Status: shipped. Owner: internal/auth, internal/identity. Hard dependencies: 01.

## Authority and design

RFC-001 §4, D-044/D-045 and D-059–D-061, and [the Pengui authority contract](../contracts/pengui-authority.md) control security. [COMMON.md](COMMON.md) supplies the implementation/testing workflow. This is a token verifier and envelope decoder, not an authentication provider.

## Brief findings incorporated

Briefs 04, 14: asymmetric verification, signed identity, bounded key refresh, safe failures and immutable per-call context. These outcomes remain without copying a sibling's local issuer.

## Findings I'm departing from

D-044 removes both issuer-driver modes, API-key exchange, signing secrets, local bootstrap and local service-account creation. Pengui alone issues authority. Apps compatibility is established and unrelated to this work. The actual provider minter supplies at most 32 scopes / 256 bytes each / 4096 bytes total, not a guessed larger claim contract.

## Scope and implementation tasks

1. Verification-only Pengui JWT middleware and immutable envelope for HTTP/MCP/in-process callers, using the single documented identity/scope contract.
2. Shared trusted JWKS refresh, rotation, bounded staleness, token/claim size limits and algorithm/key matching; issuer/audience/time verification through golang-jwt v5.
3. Concrete [provider registration handoff](../contracts/pengui-provider-registration.md) against Pengui's existing opaque provider mint seam, with a checked manifest and real operational consumer. No alternate issuer profile or local mode.

## Non-goals

No local issuer, token renewal/signing, key exchange, login, grants, OAuth service or user/service-account store. Full MCP transport is phase22; durable delegated authority is phase06. This milestone does not claim a deployed Pengui session was exercised.

## Config and persistence

The typed configuration supports issuer/JWKS, exact HTTP/MCP audiences or an explicit same-audience shorthand, asymmetric algorithm allowlist, maximum token lifetime/skew, hard key freshness, refresh/request timeouts, token/claim/scope limits. The [configuration reference](../configuration.md) documents implemented defaults and bounds. There is no signing setting or local policy persistence.

## Acceptance criteria

1. **AC01** — Reject HS/none, bad signatures, algorithm/key confusion, token-provided key URLs, unknown keys and over-stale verification material.
2. **AC02** — Validate mandatory tenant/user/session/exp/intended aud and consistent subject; malformed nbf/iat or excessive lifetime is rejected.
3. **AC03** — Header/body/query actor or tenant values never replace verified claims; parsed envelope and slices cannot mutate shared state.
4. **AC04** — Scope decoding is bounded, unambiguous and pinned to the provider contract with a Pengui-shaped signed fixture, including service attribution.
5. **AC05** — No production signing/mint/exchange/bootstrap/API-key endpoints, issuer keys or fallback login can be constructed or registered.
6. **AC06** — Key rotation, concurrent reuse and token fuzzing preserve fail-closed behavior; expiry is tested without claiming instant offline revocation.

## Tests, coverage and smoke

`TestPhase03/AC01` through `AC06` exercise the real verifier/key loader with ephemeral test-only asymmetric signers. Additional tests cover all six supported RS/ES algorithms, correctly signed duplicate claims, strict JSON, issuer limits, concurrent cache use, rotation/removal, immutable slices, and compiled TLS-JWKS-to-PostgreSQL operations. Fuzz targets cover the actual verifier and JSON decoder. Coverage retains the 85% auth/identity threshold. `scripts/smoke/phase-03.sh` requires all six named results; missing/skipped tests cannot close acceptance.

## Glossary, decisions and deviations

The [adversarial review](../reviews/phase-03-04-adversarial.md) and [verification record](../reviews/phase-03-04-verification.md) describe evidence and boundaries. Signature/key freshness is an authorization snapshot, not instantaneous permission revocation. Only the trusted verifier constructs production envelopes; storage coordinates do not authenticate users. Request verification and health use one cache. The operator must configure its approved scope set in Pengui; no production registration is claimed from the synthetic fixture.
