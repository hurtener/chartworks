# Phase 23 — sdk-cli-parity

Status: in_progress. Owner: sdk/chartworks, cmd/chartworks. Hard dependencies: 21, 22.

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

Implemented `TestPhase23/AC01` through `TestPhase23/AC06`; exercise current concrete capabilities, client retry/expiry/error handling and in-process denial. Every later feature updates the generated client and parity matrix in the same change. COMMON.md sets 70% CLI and relevant SDK coverage; `scripts/smoke/phase-23.sh` requires all six results.

## Glossary, decisions and deviations

Token supplier is a client dependency, not an issuer. D-044/D-050 and
[D-070](../decisions/2026-09-09-client-parity.md) apply. Implementation and named
acceptance are separate from review/merge; this phase stays `in_progress` until
reviewed and merged. The later reporting capabilities are not claimed complete.

## Implemented surface and acceptance mapping, 2026-09-09

The [client v1 contract](../contracts/clients-v1.md) owns configuration and usage.
The existing typed HTTP SDK remains the preferred application interface. The
registered `Invoke` facade and CLI derive their operation matrix from the running
server's actual OpenAPI document, and join only compatible installed MCP tools.
An unbuilt operation is absent, not a generated success-returning stub.

| Criterion | Executable implementation and evidence |
| --- | --- |
| AC01 | Eleven Discover/Ask/BYO/Feedback contracts run through raw HTTP, typed and generic HTTP SDK, typed and generic in-process SDK, network/in-process MCP, CLI HTTP and CLI MCP. Real PostgreSQL, published topics, native validator/executor and recorded provider calls supply the outcomes. Exact ordered data and publication payloads agree; the configured 100,000-row ceiling also passes the generic, MCP and CLI projections. |
| AC02 | The matrix covers every current registered HTTP operation and its concrete SDK/CLI command. Real MCP owner IDs/actions/effects/audit metadata join it. Unbuilt reporting/artifact and issuer operations remain unavailable, with zero model/source work. |
| AC03 | `NewInProcessWithOptions` forwards a fresh token through the production authenticated handler, strips ambient context values, preserves deadlines/cancellation and never constructs an envelope. Real tests reject invalid, wrong-audience, same-tenant/wrong-context and cross-tenant calls even with an earlier verified envelope in context. Canonical mounts reject encoded aliases and path escape. |
| AC04 | The compiled command routes to injected stdin/stdout/stderr, environment, HTTP client and descriptor opener. Tests exercise config/inspection/queries, explicit effect acknowledgement, status exits, malformed input, short writes, cancellation, token-source errors and no issuer/admin commands. |
| AC05 | Owner-classified keyed retries freeze body/path/key, reacquire only the supplied provider's current token, and preserve cancellation. A real committed sweep with a lost reply is replayed without another effect; expired BYO references, denied authority and unsafe query replay are not rerun. |
| AC06 | Real operational diagnostics do not mutate the store or invoke model/source work. Explicitly scoped retention erasure is bounded, audited, idempotent and cross-tenant isolated. Fixtures/examples are synthetic. |

No additional endpoint or business table is necessary. Phase 21 is extended with
one canonical configured mount grammar and an explicit `x-chartworks-replay`
contract. Read response decoding has its own bounded 100,000-item mode; the
model/request decoder's existing 65,536-item bound is unchanged. No row/byte,
coverage, native validation or authority gate is lowered.

The [adversarial record](../reviews/phase-23-adversarial.md) identifies corrections,
verification commands and exact-source evidence boundaries. All six named tests
must pass; the registry does not excuse missing runtime assertions.
