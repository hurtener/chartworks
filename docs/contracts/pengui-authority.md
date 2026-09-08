# Pengui authority consumed by Chartworks

Implementation contract, revised 2026-09-05. Pengui alone authenticates, decides permissions and issues tokens. Phases 03/04 consume its existing provider-scope mint format. The separate durable execution-binding adapter remains a phase 06 deliverable; this document does not claim that proposed binding API is already deployed.

## Request verification

Pengui-issued Authorization bearer -> configured asymmetric/JWKS verifier -> immutable envelope -> signed action/resource enforcement -> domain service. `internal/auth.Verifier` uses the maintained golang-jwt library for signature and registered-claim verification after bounded, duplicate-free JOSE/claim decoding. It accepts the actual `tenant`, `user`, `session`, `scopes` representation, requires `iat` and `exp`, validates optional `nbf` and `sub`, and checks the intended surface audience. Exact fields, limits, issuer-source evidence, examples and the real operation manifest are in [provider registration](pengui-provider-registration.md).

Require exactly one Authorization bearer. Cookies, URL parameters, body values, forwarded identity headers, embedded JWKs, token-selected key URLs and critical header extensions cannot authenticate a request. Only trusted configuration chooses the issuer, key address, algorithm allowlist and audience. Subject must agree with the user when present; service attribution grants nothing by itself.

No local issuer/signing key, login/OAuth, users/groups/roles/grants, service accounts, passwords/API keys, bootstrap admin or embed credential system. Metadata establishes reference ownership and business validity, not access. A bare admin string, actor prefix or creator label grants nothing. There are no new identity or permission-policy database tables.

## One bounded verification-key cache

Readiness and token verification share the same public-key cache. A successful bounded JWKS retrieval atomically replaces the set and records last-success freshness. Invalid documents, redirects, private/symmetric keys, bad key/algorithm binding or failed requests never extend it. Fresh prior keys can remain usable only until the configured hard stale deadline. Removed keys stop working after a successful replacement.

Refreshes are single-flight and globally bounded by the configured interval, including unknown-kid requests; there is no unbounded negative-key cache or goroutine per unknown key. Cancellation-aware waiters leave without extending freshness. Consequently, a just-rotated unknown kid may be denied until the next allowed refresh. That is a deliberate fail-closed availability tradeoff, not permission to use an unverified key.

## Signed operation and addressed-resource scopes

Use Pengui's existing opaque `scopes: []string` provider mint seam. Actions such as `reporting.execute` and `reporting.publish` are separate from addressed reach. The sole representation is `cw.<kind>.<permission>:<id>`; no parallel local grants resolver or alternate scope encoding is introduced.

Kinds: source, dataset, topic, block, report, dashboard, run, execution_context, execution_binding, tenant. Permissions: read, query, write, execute, preview, publish, certify, export, use, erase. IDs are 1–128 canonical ASCII alphanumeric/underscore/hyphen/dot/colon characters. Split only the first colon after the permission. Reject whitespace, encoded/path IDs and ambiguous delimiters. Only explicit whole-ID `*` permits all eligible resources **inside the signed tenant**; it is not prefix matching or a cross-tenant bypass. For tenant-kind reach, the actual addressed target is the signed tenant itself.

Scopes are bounded by the actual issuer limit: at most 32 unique printable ASCII strings, 256 bytes each and 4096 total bytes. Malformed/duplicate/excessive authority fails, never truncates. Unknown ordinary action strings remain non-authorizing for unregistered operations; malformed strings in the reserved `cw.` namespace fail validation. Empty scopes authenticate but grant no operation.

`access.Require` checks an exact action and all required target/dependency reaches. `access.Constrain` creates a detached, canonical, tenant-bound selection before a query can occur; an empty/expired selection is not unrestricted access. The selection retains the verified snapshot and becomes unusable after its expiry. Actual store/source adapters must apply tenant plus exact IDs or explicit all-in-tenant in the query, not fetch broad data and security-filter it afterward.

## Real first consumers and future domain enforcement

The operational retention, audit, maintenance, diagnostic and metric routes use the central [operation registry](chartworks-operations.json). Their domain service independently enforces the same signed action/reach, so direct in-process calls cannot bypass HTTP middleware. Storage coordinates are derived only after that check. The existing raw `store.Scope` remains a storage coordinate, not a token verifier or locally created authorization credential.

Execution checks require the service-resolved target, every executable dependency and each actual context revision. Retained-result checks require `reporting.read`, an exact run or eligible published-parent read reach, and the actual artifact context partitions; no query-execute permission is needed merely to read retained results. Private previews additionally require `reporting.preview` and target preview reach, even when exact run read is present. Later publication never erases an artifact's private status.

The execution/artifact functions are implemented enforcement primitives, not reporting/storage endpoints advertised ahead of their phases. Their callers must derive complete dependency/context manifests from trusted domain metadata. A helper cannot infer a deliberately omitted dependency; later validator/domain fixtures prove completeness at the first concrete data consumer.

Creation checks parent write reach: blocks under topics, datasets under sources, sources/topic drafts/reports/dashboards under the tenant. Publication, certification, SQL inspection, export and schedule management remain distinct actions. Immutable revisions, evidence, health, references and SQL safety are separate domain-validity checks, not invented identity policy.

## Actual source context

A registered context fixes credentials/warehouse role/secure-view or RLS behavior and semantic binding. Selecting it requires signed use authority. **Its immutable revision ID is the addressed context ID** (synthetic example `context1:v1`), and defines the result partition. A changed role or exposure creates a different revision; a caller-supplied label cannot narrow broad data or make old artifacts safe for a newly restricted context.

Reading/reusing retained values needs Pengui entitlement to the target and actual partition plus persisted preview privacy. No tenant-only shared result cache, browser security filter or local team-sharing logic is permitted. A context wildcard is broader authority only when Pengui explicitly signs it.

## Durable work: phase 06 owns the adapter

Admission validates target/dependency and execution-binding use reach. Store the immutable accepted manifest, opaque Pengui-authorized binding and attribution, not bearer bytes. The binding cannot select a stronger identity than the signed request permits.

`ExecutionAuthorityProvider` is a thin client that obtains fresh Pengui JWTs at dispatch/retry/checkpoints and verifies them through the same core. Only server-accepted binding/operation/target/audience go to the existing platform seam. Phase 06 reads the actual broker contract, reuses an available operation or implements a Pengui-owned extension, records exact schemas/fixtures and proves its first durable consumer. Phase 30 reuses it. No guessed endpoint, ambient credentials, parallel broker/auth service or local signing fallback is allowed.

The synchronous bounded operational consumer implemented now checks signed authority before I/O and caps its context by token expiry plus configured skew. It does not expose unattended work or persist a user's bearer. Missing/refused future renewal records blocked work and does not change accepted revisions, parameters, occurrence windows or attribution. Retained reads do not wait for a broker, warehouse or inference provider.

## Freshness and browser delivery

Offline signature validation cannot observe permissions changed after issuance. Pengui controls lifetime/renewal/revocation. Chartworks validates every supplied token and enforces expiry plus bounded configured skew. Context cancellation bounds accepted in-process/HTTP work; it is not a claim of instantaneous distributed revocation or rollback of already committed effects.

Apps use the established host bridge; resources/tool arguments contain no shared bearer. Iframe uses a Pengui/client BFF forwarding scoped Pengui credentials server-side. Chartworks provides authorized data/HTML/SVG and issues no browser session/bootstrap code/signed capability URL. Bifrost provider secrets remain unrelated remote-inference credentials and are never accepted as Chartworks authority.

## Verification scope

Named phase 03/04 acceptance covers cryptography, key rotation/staleness, claim bounds, immutable scopes, pre-I/O denial, tenant isolation, distinct permissions and current-context partition rules. The compiled-binary test exercises ephemeral trusted TLS verification -> real PostgreSQL -> SDK -> shutdown. These are Chartworks implementation tests with synthetic issuer-shaped fixtures, not a claim that customer credentials or a deployed Pengui session were exercised. Full MCP transport, later reporting data paths and durable renewal stay assigned to their owning phases.

Private topic draft admission additionally requires the exact topic write target and every source read, dataset query and execution-context use dependency. Retained draft operations enforce their own topic read/export action, private actor/session provenance and the complete persisted dependency set before fetching content; creator identity alone grants no access. See [topic draft service](topic-drafts-v1.md).
