# Phase 03/04 implementation decisions

Date: 2026-09-05. These extend the ownership decisions; no local authentication service or policy system is introduced.

### D-059 — Consume the existing Pengui provider bearer format exactly

The actual platform minter already carries operator-approved opaque provider scopes in `scopes: []string`, capped at 32 entries, 256 bytes each and 4096 total. Adopt those limits and the single `cw.<kind>.<permission>:<id>` addressed grammar. Require actual issuer fields tenant/user/session/iat/exp, consistent optional sub and validated optional nbf. Registration envelopes are not data-plane bearers. Use golang-jwt v5 for cryptography, with strict bounded duplicate-free decoding and one shared trusted public-key cache; no handwritten token-signing or local minting subsystem.

### D-060 — Deliver verified operational consumers without pulling later product phases forward

Phases 03/04 add protected retention-policy/audit/sweep/diagnostic/metric APIs and SDK methods over the existing real PostgreSQL consumer. The domain service also checks signed authority, independently of transport. Public health remains content-free; the listener retains the explicit loopback deployment boundary until the broader transport phase. Authentication is now an implemented capability, whereas reporting/NLQ and the full MCP server remain unimplemented. The shared verifier already supports exact HTTP/MCP audience profiles; this is not a host-compatibility project.

### D-061 — Scope freshness remains bounded by the signed snapshot

Readiness and verification share a single-flight, interval-bounded JWKS cache with hard last-success expiry. Unknown-key traffic cannot cause unbounded refreshes; short fail-closed rotation delay is explicit. Verified envelopes and derived query selections become unusable after token expiry plus configured skew; synchronous operations carry the same context deadline. Offline verification does not promise immediate permission revocation. Actual context *revision* IDs, rather than mutable context labels, bind future retained-result policy partitions. Phase 06 still owns fresh delegated authority for durable work; no bearer is persisted by this phase.
