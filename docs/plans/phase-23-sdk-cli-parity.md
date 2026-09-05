# Phase 23 — sdk-cli-parity

Status: planned. Owner: sdk/chartworks, cmd/chartworks. Hard dependencies: 21, 22.

## Authority and design

RFC-001 §11, D-044/D-050 and [COMMON.md](COMMON.md) apply. Clients consume the ordinary Pengui authority path; an in-process call must not bypass the verified-envelope requirement.

## Brief findings incorporated

Briefs 01, 14: external integration clients, typed errors/results, explicit authoring/consumption boundaries and safe retries.

## Findings I'm departing from

Remove bootstrap/key/grant management commands and any client-side issuer. Client credential renewal delegates to its supplied Pengui token provider, not a new local exchange service.

## Scope and implementation tasks

1. Provide HTTP and verified in-process clients plus CLI commands for configuration, inspection, queries/reporting, operations and authorized maintenance.
2. Use a caller-provided Pengui token supplier for renewal; never mint credentials in the SDK/CLI or persist tokens in command history/logs.
3. Keep domain types/errors/idempotency/cancellation/streaming-or-poll semantics aligned with registered public operations.

## Non-goals

No shadow-store of runtime/report entities, implicit credentials, CLI user management or generated unsupported endpoint methods.

## Config and persistence

Client base URL, token-provider configuration and request timeout; no signing/API-key generation settings. No additional business persistence. Token input must be secret-safe; example commands use a token-provider/environment/file descriptor pattern rather than encouraging bearer values in history.

## Acceptance criteria

1. **AC01** — Implemented Discover/Ask/BYO/Feedback calls share a generated operation matrix across HTTP/MCP/SDK with identical core outcomes; later domain additions extend the same matrix.
2. **AC02** — Reporting additions are exposed as their owner phases land; commands never pretend a missing endpoint is implemented.
3. **AC03** — In-process callers cannot bypass verified-envelope/resource checks; SDK token renewal delegates only to its supplied Pengui provider.
4. **AC04** — CLI has injected I/O, correct exit/status behavior and no bootstrap/key/grant/user management commands.
5. **AC05** — Retries preserve idempotency keys and cancellation intent; unauthorized/expired artifact responses are not silently rerun.
6. **AC06** — Read-only diagnostics and explicitly scoped erasure use real services and cross-tenant tests; examples contain only synthetic values.

## Tests, coverage and smoke

Implement `TestPhase23/AC01` through `TestPhase23/AC06`; exercise current concrete capabilities, client retry/expiry/error handling and in-process denial. Every later feature updates the generated client and parity matrix in the same change. COMMON.md sets 70% CLI and relevant SDK coverage; `scripts/smoke/phase-23.sh` requires all six results.

## Glossary, decisions and deviations

Token supplier is a client dependency, not an issuer. D-044/D-050 apply. No runtime completion is claimed.
