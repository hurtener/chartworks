# Client parity implementation decision

### D-070 — Registered client parity, canonical mounts and explicit replay

Date: 2026-09-09. Status: implementation decision for phase-23 review.
Supersedes only the phase-21 temporary root-only mounting restriction and any
client inference that a required idempotency header proves safe replay. The
Pengui issuer, service/authority ownership, existing query validation and original
model/request bounds remain unchanged. D-044/D-050 and COMMON.md still apply.

## Decision

Preserve typed SDK methods and add a generic consumer of the actual immutable
HTTP registration. Generate a detached operation matrix from the live OpenAPI
document; join only matching installed MCP owner contracts. The CLI is a thin
consumer of those same methods. In-process calls traverse the actual authenticated
handler with a fresh caller-supplied token, no ambient envelope values and no
listener or parallel service implementation.

The configuration loader, OpenAPI generator and SDK share one canonical absolute
mount grammar. The router strips exactly that mount and rejects encoded path
aliases before authentication. Root remains the default; dot/empty segments and
credential-bearing URL fields are not accepted. This delivers the pieces that
were intentionally missing when phase 21 initially rejected non-root mounts.

Replay eligibility is explicit owner metadata (`read`, `never`, `keyed`). A
required Idempotency-Key header alone cannot enable mutation replay. Only existing
ledger-backed operations explicitly classified by their owner may opt into a
maximum-three-attempt SDK loop with unchanged body/path/key and fresh provider
credentials. Query/model operations remain non-replayable by default. Expired,
denied, missing and uncertain transport results cannot silently create new work.

The original model/request JSON decoder retains its 65,536-item collection cap.
Validated service response decoding has a separate 100,000-item cap, matching the
already supported executor row ceiling, while retaining byte, depth, duplicate-key
and finite numeric restrictions. This avoids generic/MCP/CLI transport failure
for a valid qualified read without relaxing untrusted input or warehouse limits.

## Evidence and tradeoffs

The recovery baseline's unit tests passed but its real phase-23 fixture failed on
`server.base_path`; a client-only prefix implementation was insufficient. Real
multi-surface and output-ceiling acceptance now exercises the missing integration.
A post-commit lost-response sweep proves replay uses the existing operation ledger,
not only a transport mock. Fuzz, cross-tenant, wrong-context, ambient-envelope,
short-write and cancellation tests retain failure before dependency access.

Generic calls re-read the installed contract and validate JSON schemas; they are
not a performance replacement for typed SDK calls. In-process calls intentionally
pay the same serialization/verification boundary rather than adding a privileged
shortcut. Streaming is not emulated. The synchronous adapter requires cooperative
handlers and providers; custom client transports are trusted caller dependencies.

No new endpoint, database table, broker, token exchange, authority grant, client
business shadow state or reporting placeholder is introduced. Details and exact
verification boundaries are in [clients v1](../contracts/clients-v1.md) and the
[adversarial record](../reviews/phase-23-adversarial.md).
