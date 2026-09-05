# Phase 03 — auth-identity

> **Status:** draft
> **Owner:** orchestrator
> **Depends on:** phase-01-binary-config-telemetry (hard); phase-02-store-migrations (soft, parallel — see "Depends on")

Authored per the CLAUDE.md §16 workflow. Implements the identity-and-auth half of
Wave 1: the single JWT validation path, the two issuer drivers, the API-key mint
boundary, and the frozen `identity.Envelope`. It closes brief 04's two
highest-severity scars — header-trust identity minting and silent secret
auto-generation — by construction.

---

## RFC / request sections

- **RFC-001-Chartworks.md §4** (Identity & auth) — the whole of this phase:
  §4.1 two issuers/one validation path, §4.2 claims + the frozen envelope,
  §4.3 the cryptographic mint boundary, §4.4 no local user management.
- **RFC §3.2** (packages) — `internal/auth` and `internal/identity` ownership.
- **RFC §3.3** (runtime shape) — boot is fail-loud; JWKS fetch (external mode) and
  a present-but-loadable signing key (self-issue mode) gate readiness.
- **RFC §5.3** (capability scopes) — the `scopes` claim is *read into* the envelope
  here; grant → `EffectiveAccess` resolution is phase 04, not this phase.
- **RFC §7 of CLAUDE.md** (security) — asymmetric-only, `aud` mandatory + per-surface,
  stale JWKS fails closed, identity never on `X-*` headers, API keys constant-time.
- **RFC §11.1/§11.2** — the distinct MCP vs HTTP audiences a token is validated against.
- **RFC §12** — the `api_keys` and `tenants` store rows this phase reads/writes
  (owned and migrated by phase 02).
- **RFC §14** — the `auth` config domain (keys enumerated below).
- **RFC §15** — `/readyz` = … + JWKS fresh (external mode); per-decision auth metrics.
- **Decisions:** D-006 (asymmetric dual-mode auth, dual per-surface audiences,
  frozen envelope), D-030 (no local user management; self-issue = API keys; admin
  bootstrap is a local CLI action). D-020 is referenced only for the `scopes` slot the
  envelope carries; its resolver is phase 04.

## Depends on

- **phase-01 (hard).** Provides the `chartworks` binary skeleton + `admin` subcommand
  dispatch (D-008 stdlib `flag`), the typed config loader with `env:` indirection and
  fail-loud validators (the `auth` domain plugs into it), and the telemetry
  foundations (slog, the Prometheus registry the auth-decision counters register on,
  the audit-emitter interface auth events use).
- **phase-02 (soft, parallel — 01 → 02∥03).** This phase needs the `api_keys` and
  `tenants` rows (RFC §12), which phase 02 owns and migrates. To keep 02 and 03
  parallel, **phase 03 declares the two narrow store ports it needs**
  (`auth.KeyStore` — lookup an API key by id, scoped; `auth.TenantStore` — create/read
  a tenant + its first admin) and codes against those interfaces; the `postgres` store
  driver satisfies them. Unit tests use an in-package fake of these ports; the
  **Postgres-backed** bootstrap + API-key-exchange integration test runs once phase
  02's `api_keys`/`tenants` migrations are on the `wave-1` branch (wave-1 checkpoint
  gate, §17). This mirrors the master-plan dependency (`03 ← 01`) exactly while making
  the seam to 02 explicit rather than implicit.

## Informing briefs

Per `docs/research/INDEX.md` (`internal/auth` / `internal/identity` → **04**, then
01, 05):

- **Brief 04 — predecessor security, tenancy, credential handling** *(load-bearing).*
  The two scars this phase provably closes (header-trust mint boundary; silent secret
  auto-generation), plus the HS256/hand-rolled-JWT scar P2 exists to rule out, the
  404-existence-hiding keeper, the tenant-mismatch-400 keeper, and the distinct-`purpose`
  anti-replay pattern.
- **Brief 01 — predecessor architecture** *(secondary).* The config scar (a
  `_FLAT_ENVIRONMENT_MAP` flat/nested dual regime — one naming convention only, RFC §14)
  and the dead-metrics scar (auth-decision counters must actually export).
- **Brief 05 — predecessor diff** *(secondary).* The fork's resolver discipline (tenant
  predicate always intersected last) — relevant to the envelope's tenant field even
  though the resolver itself is phase 04.

## Brief findings incorporated

- **The mint boundary is cryptographic (brief 04 §Scars, headline).** Self-issue mints a
  token **only** from an API-key exchange (key → constant-time hash compare → short-lived
  JWT). There is **no** header-exchange endpoint and **no** first-caller-becomes-admin
  path — the exact `POST /auth/exchange` scar. Because the surface does not exist,
  criterion 8 is a *structural* test that no header-sourced mint path can be constructed,
  not a runtime probe of an endpoint.
- **Fail loud on a missing signing key (brief 04 §Scars, "silent secret auto-generation").**
  A missing/unloadable self-issue signing key, or a missing JWKS URL in external mode, is
  a **refused boot with a typed error** — never a per-process ephemeral key (the
  multi-replica non-determinism bug). Criterion 6.
- **Asymmetric-only, rejected at the parser (brief 04 §Scars, "HS256-only, hand-rolled").**
  RS/ES `256|384|512` only; `HS*` and `none` are rejected **before** any claim is read,
  by pinning accepted algorithms on the parser itself (never trusting the token header's
  `alg`). Criterion 1.
- **Identity never from headers (brief 04 §AuthZ).** The predecessor's `RequestPrincipal`
  was already correctly *sourced* from the validated token; only the mint leaked header
  trust. We keep the frozen-envelope shape and forbid any `X-*` read for identity/tenant/
  access anywhere downstream. Criteria 4b/11.
- **Distinct-`purpose` anti-replay → satisfied by per-surface `aud` (brief 04 keeper 7).**
  The fork used a distinct `purpose` claim so two token kinds off one key can't be
  replayed as each other. We achieve the same separation with D-006's **per-surface
  audiences** (`<instance>.http` vs `<instance>.mcp`), validated distinctly — a Console
  token cannot be replayed as an agent token. Criterion 4.
- **404 existence-hiding + tenant-mismatch-400 (brief 04 keepers 2, 3).** Recorded as
  standing disciplines the surface phases (21/22) inherit; the tenant-mismatch guard is
  moot at the *token* layer (tenant comes only from the validated claim, never a header),
  which is a strictly stronger position than the predecessor's header/param reconciliation.
- **One config naming convention (brief 01 §Configuration).** The `auth` keys are nested
  dotted keys under one regime; no flat `SCREAMING_SNAKE` alias map. Secrets ride `env:`
  indirection only.
- **Auth-decision counters actually export (brief 01 §Metrics scar).** Every auth-decision
  counter this phase registers is asserted present on `/metrics` by phase 01's telemetry
  conformance test (this phase adds its metric names to that assertion).

## Findings I'm departing from

- **A separate `purpose` claim (brief 04 keeper 7).** Not carried as its own field —
  per-surface `aud` (D-006) already provides the anti-replay separation, and a second,
  overlapping mechanism would be a P7 "two of anything" scar. Deliberate.
- **PBKDF2 password hashing / any password baseline (brief 04 §AuthN).** Not ported —
  D-030 deletes password/login/signup/invite surfaces entirely. API keys are the only
  self-issue credential.
- **The `x-<pkg>-authorization` non-standard header and the `wf_access` cookie fallback
  (brief 04 §AuthN).** Not carried — standard `Authorization: Bearer` only, no cookie
  transport (V1 is bearer-only; RFC §11.2). A second silent transport is exactly the P7
  scar.
- **SA-token / dataset-mode parallel identity channels (brief 04 §AuthN, three channels).**
  Not carried — one validation path for both surfaces and both modes. `svc:` principals
  (if used) ride the same validated-JWT contract, not a bespoke lookup.
- **Regex injection check.** n/a to this phase (SQL-safety is phase 09); flagged only so
  it is on record it is not smuggled in here.

## Scope

Delivers `internal/auth` and `internal/identity`:

- **`internal/identity`** — the frozen `Envelope` value type carrying
  `(tenant, principal, session, scopes)` plus an `EffectiveAccess` slot the phase-04
  resolver populates. Constructed once via a constructor that consumes only validated
  claims; immutable after construction (no exported setters; unexported fields; accessor
  methods only). The `Principal` parse/format helpers for the `user:` / `agent:` /
  `svc:` / `key:` prefixes (glossary; D-020).
- **`internal/auth`** — the single `Validator` (asymmetric-only parse, `iss`/`exp`/skew,
  per-surface `aud`), behind an **issuer seam** with two drivers registered by `init()`
  blank-import (§4.4 pattern): `self_issue` (loads/holds the keypair, mints from API-key
  exchange, JWKS-serves its own public key) and `external_issuer` (fetches + caches a
  remote JWKS, fails closed past `jwks_max_stale`). The API-key exchange
  (constant-time hash compare against `KeyStore`, short-lived JWT with the key's
  tenant/principal/scopes). The `chartworks admin bootstrap` command (local-only,
  provisions the first tenant + admin via `TenantStore`). Boot-time key validation wired
  into phase 01's fail-loud validator set and `/readyz`.
- **Config:** the `auth` domain (below) added to phase 01's typed config + the example
  config.
- **Telemetry:** auth-decision counters (`auth_validations_total{surface,mode,decision,reason}`)
  registered on the phase-01 registry and added to the telemetry conformance assertion;
  content-free auth audit events (validation-denied, key-exchange, bootstrap) via the
  phase-01 audit emitter.

## Non-goals

- **Grant resolution / `EffectiveAccess` computation** — phase 04 (`internal/access`).
  This phase defines the envelope *slot* and leaves it empty; it does not read `grants`.
- **Wiring auth into HTTP/MCP request middleware** — phases 21/22. This phase ships the
  `Validate(ctx, raw, expectedAudience)` function and proves both audiences; the surfaces
  call it.
- **The `scope-debug` diagnostic** — phase 04 (resolver-backed).
- **Capability-scope *enforcement*** (gating operation families) — phase 04/surfaces.
  Scopes are only *carried* into the envelope here.
- **API-key CRUD / rotation admin endpoints** — HTTP admin surface, phase 21 (this phase
  ships only lookup + the bootstrap CLI, plus the `keys` CLI stub deferred to phase 23).
- **`svc:`-to-`svc:` call machinery** beyond the principal prefix.

## Design

**Validation flow (one path, both surfaces, both modes):**

```
raw bearer token
  └─▶ parse header, PIN accepted algs = {RS,ES}×{256,384,512}   ← HS*/none rejected HERE,
        (never read alg from the token)                            before any claim is read
  └─▶ select issuer driver by `iss` (self_issue | external_issuer)
  └─▶ verify signature against the driver's key material
        self_issue: local public key (loaded at boot)
        external_issuer: cached JWKS; if stale > jwks_max_stale ⇒ typed 401 + not-ready
  └─▶ enforce claims: iss pinned · exp + leeway(skew) · aud == expectedSurfaceAudience
        (caller passes http-aud or mcp-aud; the other is a cross-rejection)
  └─▶ read tenant (mandatory), sub→Principal, session?, scopes[]  ← ONCE
  └─▶ identity.NewEnvelope(...)  → frozen, immutable
```

**Issuer seam (§4.4 interface + factory + driver).** `auth.Issuer` interface:
`Verify(ctx, token) (claims, error)` + `Keys()`/readiness hook; drivers `self_issue`
and `external_issuer` register via `init()`. A single V1 does not collapse the seam —
both drivers ship, and the two are *combinable* (one binary can validate its own tokens
and Pengui's). The `Validator` is immutable after construction and safe under concurrent
use (JWKS cache guarded; per-request state is arguments only — §5 concurrency rule).

**API-key exchange (the only self-issue mint).** `POST`-shaped exchange operation
(exposed on the HTTP admin surface in phase 21; the core function lives here):
presented key → parse `key:<id>` → `KeyStore.Lookup(tenant?, keyID)` →
`subtle.ConstantTimeCompare` of the stored hash → on match, mint a short-lived JWT
(`token_ttl`, default 1h) carrying the key's tenant/principal/scopes and the self-issue
`iss` + the requested surface `aud`. Unknown/revoked key ⇒ typed error, constant-time
path preserved, never logged. **No other mint path exists** — there is no function that
produces a token from a header, a param, or a "first caller."

**The frozen envelope (P2).** `identity.Envelope` is a struct with unexported fields and
no setters; `NewEnvelope(claims)` is the only constructor and it takes validated claims.
Downstream code holds it by value / via an interface of accessors; mutation is not
expressible (criterion 7). `EffectiveAccess` is an embedded zero value phase 04's
resolver fills via a single `WithAccess` that returns a *new* envelope (copy-on-attach),
never an in-place mutation, so immutability holds across the 03→04 seam.

**Boot / readiness (fail-loud, P4).** The `auth` config validator (phase 01 seam) checks:
at least one mode enabled; self-issue ⇒ a signing keypair path that **loads** (missing/
unreadable/malformed ⇒ refused boot, typed error — never generate one); external ⇒ a
JWKS URL present and a first fetch attempted; per-surface audiences both set and distinct.
`/readyz` reports not-ready while JWKS is stale (external mode).

**P-property upholding:** P2 (this phase *is* P2's implementation), P4 (every failure is a
typed error + metric, no silent degrade), P6 (no plumbing nouns on the wire — errors are
domain-framed: "token invalid", "not authorized", never "jwks shard"), P7 (one validation
path, one envelope, one mint boundary).

## Config keys added

Under the `auth` domain (RFC §14; nested dotted keys, one convention; secrets via `env:`):

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `auth.modes` | `[]string` (`self_issue`\|`external_issuer`) | `[]` | yes (≥1) | Combinable; empty ⇒ refused boot |
| `auth.issuer` | string | — | self_issue | The `iss` this instance mints/pins |
| `auth.audiences.http` | string | — | yes | Per-instance HTTP `aud` |
| `auth.audiences.mcp` | string | — | yes | Per-instance MCP `aud`; must differ from `.http` |
| `auth.token_ttl` | duration | `1h` | no | Self-issued JWT lifetime |
| `auth.clock_skew` | duration | `60s` | no | `exp`/`nbf` leeway |
| `auth.self_issue.keypair_path` | string (`env:` allowed) | — | self_issue | Signing keypair; missing/unloadable ⇒ refused boot |
| `auth.external_issuer.jwks_url` | string | — | external_issuer | Remote JWKS endpoint |
| `auth.external_issuer.jwks_max_stale` | duration | `15m` | no | Past this, JWKS fails **closed** (not-ready + typed 401) |

All documented here, in the example config (added to phase 01's `config.example.yaml`),
and smoke-checked (below).

## Acceptance criteria

1. **Parser-level `HS*`/`none` rejection.** A token signed `HS256` (or `alg: none`) is
   rejected **before any claim is read**; the accepted-algorithm set is pinned on the
   parser and the token-header `alg` is never trusted to select it. Typed error, metric
   incremented.
2. **`iss`/`exp`/skew enforced.** A wrong-`iss`, an expired token (beyond `clock_skew`),
   and a not-yet-valid token each yield distinct typed errors; a token within skew passes.
3. **JWKS fails closed on staleness.** In external mode, once the cached JWKS is older than
   `jwks_max_stale` and a refresh cannot complete, validation returns a typed 401 and
   `/readyz` reports not-ready — never a silent accept.
4. **Per-surface `aud` cross-rejection.** A token minted with the HTTP audience is rejected
   when validated against the MCP surface, and vice versa; each surface passes only its own
   audience. (4b: no `X-*` header can supply or override `aud`, `tenant`, or identity.)
5. **API-key exchange is constant-time and scoped.** A valid `key:<id>` mints a short-lived
   JWT carrying the key's tenant/principal/scopes; an unknown/revoked key is rejected via a
   constant-time compare and never logged; the minted token's TTL == `auth.token_ttl`.
6. **Missing signing key ⇒ refused boot.** With self-issue enabled and the keypair
   missing/unreadable/malformed, boot fails with a typed error and **no** ephemeral key is
   generated (brief 04's silent-secret scar). Symmetrically, external mode with no JWKS URL
   refuses boot.
7. **Envelope immutability is API-enforced.** `identity.Envelope` exposes no mutator; a
   compile-time/structural test proves per-request identity cannot be changed after
   `NewEnvelope`, and `WithAccess` returns a new value rather than mutating.
8. **No header-exchange mint path exists.** A structural test enumerates every self-issue
   mint entrypoint and asserts the only one is API-key exchange — there is no
   token-from-header, token-from-param, or first-caller-admin path (D-030; the endpoint is
   absent by construction, so this is a proof of absence, not a probe).
9. **`chartworks admin bootstrap` is local-only.** The command provisions the first tenant
   + admin against the store (via `TenantStore`), is reachable only through the local CLI
   (no HTTP/MCP route registers it), and is idempotent-guarded (a second bootstrap on a
   populated tenant is a typed refusal, not a silent second admin).
10. **`FuzzParseToken` with a seed corpus.** A `Fuzz` target over the raw-token parse
    surface, seeded (valid RS/ES tokens, an `HS256` token, `alg:none`, truncated/garbage,
    oversized) asserting the invariant *never panics, and never returns a valid envelope
    for a non-asymmetric or unverifiable token*. Runs as an ordinary CI test.
11. **Forged-header attempt has no effect (adversarial).** A request carrying
    `X-Tenant-Id`/`X-Authenticated-User-Id`/`X-…-Authorization` style headers alongside a
    valid token resolves identity/tenant strictly from the token; the headers are ignored
    (proven by asserting the envelope equals the token-only case).

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven algorithm-rejection matrix (criterion 1), claims-enforcement
  matrix (2), audience matrix (4), API-key exchange happy/unknown/revoked (5), boot-config
  validation matrix (6), envelope-immutability structural test (7), mint-path enumeration
  (8). Concurrent-reuse `-race` test on the `Validator` (JWKS cache) — a hot reusable
  artifact.
- **Integration:** required — this phase closes the `KeyStore`/`TenantStore` seam that
  phase 02 satisfies. A **real Docker Postgres** test (`make pg-up`) exercising
  `admin bootstrap` (9) and API-key exchange (5) against the real store driver, run once
  phase 02's `api_keys`/`tenants` migrations are on `wave-1` (§17; gated `t.Skip` until the
  store URL + tables exist, surfacing as a smoke SKIP). External-issuer validation uses a
  local JWKS stub (a generated keypair serving a JWKS document) — no boundary mock of the
  parser itself.
- **Adversarial:** forged-header attempt (11), per-surface audience cross-rejection (4),
  `HS*`/`none` at the parser (1). Cross-tenant and empty-access probes are phase 04's
  (this phase has no grant resolution); the tenant field's source-from-claim-only is the
  relevant guard here.
- **Fuzz:** `FuzzParseToken` (10) — the prime parse/decode surface, seeded, invariant
  asserted.
- **Bench:** `BenchmarkValidate` on the hot validation path (asymmetric verify + claims);
  a baseline, not a CI gate.

## Coverage targets

Per CLAUDE.md §11 / master-plan convention 4, `auth` and `identity` are in the **85%**
band (auth/access packages). Entries added to `scripts/coverage-bands.conf` in this PR.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/auth` | 85% | Auth package band (convention 4) |
| `internal/identity` | 85% | Identity/access band (convention 4) |

## Smoke checks

`scripts/smoke/phase-03.sh` SKIPs entirely until `internal/auth` exists (no `go.mod` /
package yet at authoring time). Each criterion maps to one `go test -run` assertion via the
shared `run_group` helper (`scripts/smoke/lib.bash`); a not-yet-built test surfaces as a
SKIP, never a FAIL.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestParserRejectsHSAndNone` (`internal/auth`) PASS |
| 2 | `TestIssExpSkewEnforced` (`internal/auth`) PASS |
| 3 | `TestJWKSStaleFailsClosed` (`internal/auth`) PASS |
| 4 | `TestPerSurfaceAudienceCrossRejected` (`internal/auth`) PASS |
| 5 | `TestAPIKeyExchangeConstantTimeAndScoped` (`internal/auth`) PASS |
| 6 | `TestMissingSigningKeyRefusesBoot` (`internal/auth`) PASS |
| 7 | `TestEnvelopeImmutable` (`internal/identity`) PASS |
| 8 | `TestNoHeaderExchangeMintPath` (`internal/auth`) PASS |
| 9 | `TestAdminBootstrapLocalOnly` (`internal/auth`) PASS (Postgres half `t.Skip`s → SKIP until phase 02) |
| 10 | `FuzzParseToken` (`internal/auth`) seed-corpus PASS |
| 11 | `TestForgedHeaderNoEffect` (`internal/auth`) PASS |

## Glossary additions

New terms this phase introduces (landed in `docs/glossary.md` in this PR). Existing
entries — *Frozen per-request envelope*, *Self-issue / external-issuer*, *Principal*,
*Session* — are unchanged.

- **Mint boundary** — the cryptographic line at which self-issue produces a JWT: **only**
  an API-key exchange, never a header or a "first caller." Binding it cryptographically
  closes brief 04's highest-severity scar (RFC §4.3).
- **API-key exchange** — the sole self-issue mint path: a presented `key:<id>` →
  constant-time hash compare against the store → short-lived JWT carrying the key's
  tenant/principal/scopes (RFC §4.3).
- **Admin bootstrap** — the local-only operator CLI action (`chartworks admin bootstrap`)
  that provisions a tenant's first admin against the store; there is no
  header-exchange / first-caller-admin endpoint (D-030, RFC §4.3).

## Decisions filed

No new decision. This phase implements existing decisions:

- **D-006** — asymmetric dual-mode auth, per-surface dual audiences, JWKS fail-closed,
  frozen envelope.
- **D-030** — no local user management; self-issue = API keys; admin bootstrap via local
  CLI.
- **D-020** (referenced) — the `scopes` claim slot the envelope carries; grant resolution
  is phase 04, not here.

A reviewer note (not a decision): the 03↔02 store-port seam (`KeyStore`/`TenantStore`) and
the 03↔04 envelope `EffectiveAccess` slot are the two cross-phase seams this plan opens; if
either shape changes during phase 02/04 it is a plan deviation logged below, not a silent
edit.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). Empty at authoring time. -->

none yet.
