# Brief 04 — Predecessor security, tenancy, and credential handling

> Status: draft · 2026-07-06 · sources: both predecessors

## Summary

Both predecessors share one authz core (the fork adds a service-account scope layer and
a signed role-token for its SSO-integrated consuming application). The model the kickoff
described is exactly what the code shows: the **application** holds warehouse
credentials and privileges; a caller's only restriction is **topic-level** — read a
topic and you get whatever tables/columns/joins that topic pack exposes, with no
independent row/dataset/source grant underneath. Auth is a hand-rolled **HS256** JWT
(symmetric, single shared secret) plus **two header-trust exchange points** (an
upstream "trusted identity" header is exchanged for a signed token) — the header-trust
scar Soundings' brief already flagged, reappearing here at the token-*minting* boundary
rather than at every request. Tenant isolation is enforced consistently once a tenant
context is resolved (membership checks, mandatory `tenant_id` predicates, a mechanical
adversarial cross-tenant test suite), but resolution itself has one silent-degrade
scar: an unset JWT secret in platform auth mode is silently replaced with a random
per-process secret instead of failing loud. Customer warehouse credentials are single,
global, environment-configured (one Databricks workspace per deployment) — there is no
per-tenant credential store to inherit; D-016 is greenfield work, not a port.
SQL-safety is the strongest asset: an AST-based (sqlglot) validator blocking
DDL/DML/multi-statement, plus schema/column/join allowlisting against the routed topic
pack — genuinely worth inheriting — undermined by a regex-only injection-pattern check
that is trivially bypassable. Audit (a `@audited` decorator + middleware persisting
structured, content-free events) is broad and a real keeper, but opt-in per route and
never captures *which* table/column/topic decision fired, so a scope-debug diagnostic
is still new work.

## AuthN

Both predecessors authenticate one way at the API boundary: a Bearer token on
**`X-<Pkg>-Authorization`** — a non-standard header name, described in-code as
"the product's proxy-safe auth header" (`src/<pkg>/api/authz.py:27,65-67`), no
rationale given for why the standard `Authorization` header is avoided. The token is
one of two kinds:

1. **User JWT** — decoded by a **hand-rolled HS256 implementation**
   (`src/<pkg>/security/jwt.py`), no external JWT library, no RS/ES code path
   anywhere. `encode_jwt`/`decode_and_validate_jwt` check `iss`/`aud`/`exp` and use
   `hmac.compare_digest`; the mechanics are sound but the algorithm is symmetric only
   — an attacker who learns the shared secret forges tokens for any subject/role.
2. **Service-account token** — an opaque caller-supplied token hashed
   (`SHA-256(pepper:token)`, `src/<pkg>/stores/metadata_auth.py:169`) and looked
   up by hash, never decoded (`src/<pkg>/api/authz.py:103-141`). The pepper
   defaults to the same JWT secret if unset (`authz.py:56-63`) — a soft coupling
   between two supposedly-independent secrets.
3. A `wf_access` **cookie** is also accepted as a bearer-token fallback
   (`authz.py:73-75`) — a second silent transport for the same JWT.

**The identity-minting boundary is header-trust, not the request-time ACL boundary.**
`POST /auth/exchange` (`src/<pkg>/api/routes/auth.py:308-370`) **mints a signed
JWT from a plain HTTP header** with zero cryptographic verification of the header's
origin: platform mode trusts `x-forwarded-email` and auto-provisions the caller as
`ACTIVE` on first sight; standalone mode trusts `x-authenticated-user-id`, and the
**very first caller ever seen becomes `platform_admin`**. Everything downstream (JWT,
frozen principal, tenant/topic checks) is sound *given* the header was set by a
trustworthy reverse proxy — nothing in the code enforces that assumption.

**A second, independent header-trust identity channel exists for dataset-mode**
(uploaded spreadsheets): `resolve_dataset_principal`
(`src/<pkg>/api/identity.py`) reads `x-authenticated-user-id` directly, with **no
JWT involved at all**, plus an optional flag to also accept two legacy headers. Three
parallel identity channels (JWT, SA-token, dataset-header) is itself worth flagging as
a P7-shaped scar (see Scars).

**Passwords** (`src/<pkg>/security/passwords.py`): PBKDF2-HMAC-SHA256, 210k
iterations, random 16-byte salt, constant-time verification — a sound, unused-for-now
baseline (most flows use token/header exchange).

## AuthZ (the topic-access model)

**The frozen envelope is already correctly sourced.** `RequestPrincipal`
(`src/<pkg>/api/authz.py:90-100`) — `principal_type`, `principal_id`, `tenant_id`,
`capabilities`, `role`, `platform_admin` — is built once per request from the
validated JWT/service-account lookup (not raw headers) and passed by value. The
identity half of P2 is essentially correct here; only the mint boundary (above) leaks
header trust in.

**Tenant resolution:** header (`x-<pkg>-tenant-id`, legacy `x-tenant-id`) >
explicit param > query param (`authz.py:274-286`). `require_tenant_context`
(`authz.py:289-329`) **rejects the request with 400 if these sources disagree**
(`tenant_mismatch`) and requires a stored `store.has_membership(tenant_id, user_id)`
row, unless the caller is `platform_admin` (unconditional bypass by design).

**The topic is the resource; there is exactly one topic-level predicate**,
`require_topic_access` (`authz.py:436-519`):

```
can_read := is_tenant_admin
         OR topic.visibility == TENANT_PUBLIC
         OR topic.owner_user_id == caller.id
         OR caller.id in {share.user_id for share in topic_shares}
```

A denied topic returns **404**, not 403 (a keeper — see below). `can_edit` /
`can_manage_sharing` are `True` only for tenant admins or the topic owner.

**This is the entire access surface below the topic.** Nothing here, in
`SQLValidator`, or in the warehouse adapters restricts which *rows* of an allowed
table are visible, or grants read on some columns but not others. The model is
genuinely two-tier: (1) can this principal see this topic at all, (2) does the
generated SQL only touch tables the topic pack declares — there is no tier (3), a
row/value-level grant *within* an allowed table. **This is precisely the scar D-015
exists to fix**: topic visibility is the entire grant; every principal who can read a
topic has structurally identical data access to every other principal who can.

**The fork narrows the grain to per-topic (not per-row), worth inheriting for shape,
not depth** (`src/<pkg>/api/routes/nlq.py:268-330`,
`docs/architecture/08-security/sa_authorization.md`). Two mechanisms stack:

- **Capability scopes** gate endpoint *families* (`datasets.*` vs. `warehouse.*` vs.
  `catalog.read`), coarse and all-or-nothing, not a data-shape restriction.
- **Per-SA topic grants** (`service_account.topic_ids`) restrict a non-owner-bound
  service account to an explicit topic allowlist. An in-code comment notes this
  replaced an **older, looser model** ("the old role-grant intersection and the
  `tenant_id == "default"` bypass are gone (strict deny-by-default)") — evidence this
  exact seam already underwent one deny-by-default hardening pass.
- **Owner-bound service accounts** instead delegate to the owner's own topic
  visibility, gated by a short-lived, distinctly-purposed signed role token
  (`src/<pkg>/security/canvas_tokens.py`) presented on its own header
  (`x-<pkg>-role-token`). The role token shares its signing key with the SSO-mint
  flow but carries a distinct `purpose` claim so neither token kind can be replayed
  as the other — a real, deliberate anti-replay pattern worth inheriting.

Neither predecessor has any dataset-level or data-source-level grant independent of
topic membership; dataset-mode restricts only by uploading-user identity, with no
explicit share/grant model analogous to topic sharing.

## Tenancy

Every store method in the topic/tenant path takes a mandatory `tenant_id` and applies
it as a hard predicate; `topic.tenant_id != tenant_id` is checked before any topic
content returns (`authz.py:476-480`, folded into 404). The client predecessor ships a
**mechanical, registry-driven adversarial test suite**
(`tests/isolation/test_cross_tenant_rejection.py`,
`src/<pkg>/api/tenant_isolation_registry.py`): every route in an explicit
allowlist is probed with tenant-A credentials requesting tenant-B's scope; only
401/403/404/422 are accepted, a bare 2xx hard-fails CI.

**The registry is manually maintained and admits partial scope.** Its own docs
(`docs/architecture/08-security/tenant-isolation.md`) state it covers "the
tenant-sensitive surfaces that are **highest risk**" — nlq, datasets, feedback,
catalog. A new tenant-sensitive route never added to the registry gets **zero**
adversarial coverage, silently.

**A historical planning document contradicts the current implementation.**
`docs/history/TENANT_SEPARATION_IMPLEMENTATION.md` opens "Current State:
Application-level tenant separation exists but **lacks security enforcement**" and
proposes JWT-based tenant auth as *future* work — the `authz.py`/`jwt.py`/isolation
registry already reviewed **is** that work, already built. Treat this document as a
**stale planning artifact predating the shipped implementation**, a live example of
the drift the phase-plan discipline exists to prevent, not a description of current
state.

**No DB-level Row-Level Security or schema-enforced tenant FK constraint exists in
either predecessor** — isolation is 100% application-code discipline, the same shape
Soundings' brief 04 found in its own predecessor (a cross-product pattern, not a
one-off).

**Single-tenant-per-warehouse-connection is the real multi-tenancy shape** (see
Credential handling): tenancy isolates topics and generated SQL within one shared
warehouse connection — it never isolates *which warehouse* a tenant's queries run
against. A single scope-check bug is one hop from any tenant's data, because every
tenant's queries run through the same credential.

## Credential handling

**Exactly one warehouse credential per deployment, environment-sourced, never
per-tenant.** `WarehouseConfig` (`src/<pkg>/config/settings.py:298-360+`) holds
`databricks_host`/`databricks_http_path`/`databricks_token` (`SecretStr`) plus unused
placeholder fields (`postgres_dsn`, `bigquery_project`) for kinds **never wired to a
working adapter** — `WarehouseAdapterRegistry`
(`src/<pkg>/adapters/registry.py:41-45`) registers only Databricks. **Chartworks
inherits no per-tenant, per-data-source credential store to port** — D-016 is
genuinely new capability.

**No secret value leaked into a log/exception** was found in a scan across both
`src/` trees (`token`/`secret`/`password`/`credential`/`dsn`/`api_key`
interpolations); related log lines are presence/refresh events only
(`src/<pkg>/api/app.py:83`, `src/<pkg>/stores/lakebase.py:119`) — a correct
content-free credential-event pattern worth keeping. No hardcoded secret *value* was
found committed in either tree — treat as a negative result, not a guarantee; a
dedicated secret-scan pass is still warranted before RFC sign-off.

**A genuine rotation keeper, for Chartworks' *own* store credential (not a customer
warehouse):** `src/<pkg>/stores/lakebase.py` obtains a managed-Postgres
credential via the workspace API, caches it with an expiry margin, auto-refreshes
under a lock, and only ever assembles it into an in-memory DSN
(`_build_dsn`/`get_dsn`, lines 97-129) — never persisted, never logged. Informs
D-016's *rotation* half; the *storage* half is unaddressed here (this credential
sidesteps storage by fetching fresh every cycle).

**A partial-value redaction keeper:** an admin service-account listing redacts a
token to `f"{token[:8]}...{token[-4:]}"` (`src/<pkg>/api/routes/tenants.py:58`) —
a ready pattern for a Chartworks admin surface referencing a credential's identity
without exposing it.

## SQL-safety posture

The client predecessor's `SQLValidator`
(`src/<pkg>/services/sql_validator.py`, 798 lines) is the strongest asset in
either predecessor and the clearest candidate for P1's SQL-safety mechanism:

- **AST-first governance** (`_governance_checks_ast`, lines 728-795): parses with
  `sqlglot`, requires the top-level node be `Select`/`With`/`Union`/`Intersect`/
  `Except`; anything else (`Insert`/`Update`/`Delete`/`Merge`/`Create`/`Alter`/`Drop`/
  `Truncate`, plus DuckDB-specific `Command`/`Copy`/`Attach`/`Detach`/`Pragma`/`Call`)
  is rejected before further validation, walking the **entire tree**
  (`find_all(cls)`) so a DDL node smuggled inside a subquery/CTE is still caught.
- **Single-statement enforcement**: a semicolon anywhere but the trailing position is
  rejected (line 700).
- **Schema/table/column allowlisting against the routed topic pack**
  (`_validate_tables`/`_validate_columns`, lines 372-571) — the concrete
  implementation of "generated SQL may only touch tables/columns the semantic model
  exposes," already built.
- **Join-path validation** (`_validate_joinability`/`_tables_joinable`, lines
  573-657): rejects joining two same-topic tables with no declared join path.
- **Row caps are caller-supplied, not a hard system ceiling in this file.** `max_rows`
  flows from the request payload through to `warehouse.execute(..., limit=...)`
  (`src/<pkg>/scheduling/targets/saved_query.py:220,422`); a `plan_row_cap`
  config key exists (`config/settings.py:548-554`) but this validator does not itself
  enforce it. Confirm at RFC stage whether every execution call site actually clamps
  to a configured ceiling — this file gives no such guarantee.

**The one weak link:** the injection check is a single regex,
`'\s*OR\s*'1'='1'` (line 710) — trivially bypassable by any different literal,
whitespace variant, comment-based, or UNION-based payload. The real protection is the
AST/allowlist layer above it; this line reads as a labeled security control
(`governance.injection_pattern`) but does negligible work and should not be carried
forward under that framing.

**Identifiers built by manual string escaping (not parameterization) in the
Databricks adapter's own schema-discovery queries** (`_quote`,
`src/<pkg>/adapters/databricks.py:279-281`) — low practical risk (internal
inputs, already topic-pack-validated), but a manual-escaping pattern that must not be
copied by analogy to the adapter's correct value-parameterization
(`_build_parameters`, lines 200-211) when a future data-source adapter is written.

**No execution-time enforcement of read-only beyond "the validator ran first."**
Once SQL passes validation, the adapter sends it to the warehouse as-is — no
warehouse-side read-only role, no `SET TRANSACTION READ ONLY`, no dry-run/EXPLAIN
guard. The validator is the *entire* read-only enforcement, a single point of
failure for D-017 if it is ever bypassed or a new execution path is added around it.

## Audit

**A working, structured, content-free audit pattern exists — stronger than what
Soundings' brief found in its own predecessor.** The client predecessor's
`@audited(action=..., resource_type=..., target_param=...)` decorator
(`src/<pkg>/api/audit.py`) + `register_audit_middleware`
(`src/<pkg>/api/middleware/audit.py`) captures per decorated route: tenant id,
actor type/id, action, target resource type/id, a `success|denied|error` result,
request id, and metadata limited to method/path/status/**query-parameter *keys*
only**, persisted via `store.append_audit_event`. Coverage is broad: every NLQ
plan/run/agent-query/execute/refine/preflight route, dataset/schema/rule/topic
mutations, admin tenant/service-account management.

**Gaps:** (1) **opt-in per route** — an undecorated tenant/topic-sensitive route
emits nothing, with no mechanical check (unlike the isolation registry's route-match
CI gate) that it should have been decorated; (2) **no decision-level detail** — the
event says `nlq.run` succeeded/was denied, never *which* topic was routed to or
*which* table/column check failed, confirming Soundings' brief 04 §6 finding
transfers here: a scope-debug diagnostic is new work, not a port; (3) **no
per-decision metric**, only the durable event — no counter/histogram for
denial-rate-style observability.

## Keepers

1. **AST-first SQL governance + topic-pack schema/column/join allowlisting** —
   carry the *shape* (reject non-SELECT-family top nodes → walk the whole tree for
   blocked node types → allowlist tables/columns against the semantic model →
   validate declared join paths), not the regex injection check.
2. **404-for-denied-topic, not 403** — a consistently-applied existence-hiding
   discipline, directly reusable for topic/dataset not-found-vs-forbidden.
3. **The tenant-mismatch 400 guard** — reject outright when header/param/query
   tenant disagree, rather than silently picking one by precedence.
4. **The mechanical, registry-driven cross-tenant adversarial test suite** — a
   working instance of CLAUDE.md §11's standing cross-tenant-probe obligation;
   adapt near-as-is (explicit route registry + a probe accepting only
   401/403/404/422, hard-failing any 2xx).
5. **`@audited` decorator + content-free structured event** — adapt the shape; pair
   it with a mechanical "every tenant/topic-sensitive route must be audited" check to
   close the opt-in gap.
6. **Lakebase's fetch-fresh, never-persisted, auto-rotating credential** — a working
   "rotate a short-lived credential instead of storing a long-lived one" model,
   informing D-016's rotation mechanism.
7. **The signed role-token's distinct `purpose` claim** (fork) — a minimal
   anti-replay pattern for any place Chartworks mints two token kinds off one key
   (e.g. an HTTP UI token vs. an MCP agent token, alongside the distinct-`aud`
   requirement in CLAUDE.md §7).
8. **Partial-value credential redaction in an admin listing** — ready pattern for a
   Chartworks admin surface.

## Scars

- **Header-trust identity minting (highest-severity finding in this brief).**
  `POST /auth/exchange` mints a durable, `platform_admin`-capable signed session from
  an unauthenticated HTTP header, with the first caller ever seen auto-provisioned as
  admin. Same failure family as Soundings' header-trust scar, relocated to a single
  higher-leverage choke point: compromise this one endpoint (an exposed dev/staging
  deployment, a misconfigured ingress) and the attacker mints a durable identity, not
  just one request's access. **Chartworks' P2 must bind the *mint* boundary
  cryptographically** (a signed IdP assertion, not a bare header), not only guarantee
  the token it produces afterward is well-formed.
- **A second, parallel, independent header-trust identity channel for dataset-mode**
  — no JWT at all. Two systems independently deciding "who is calling" is the P7
  violation CLAUDE.md names directly ("two of anything is the failure mode we are
  correcting") — concrete evidence, not a hypothetical.
- **Silent secret auto-generation instead of fail-loud.**
  `<Pkg>Settings._validate_relationships`
  (`src/<pkg>/config/settings.py:1577-1583`): if platform-mode auth is enabled
  and `jwt_secret` is unset, the app **silently generates a random per-process
  secret** rather than refusing to boot (standalone mode *does* fail loud on the same
  condition). Multiple replicas would each mint a different ephemeral secret, so a
  token minted by one replica randomly fails validation on another — a latent,
  non-deterministic bug and a textbook P4 violation: a missing required secret
  degrades into "works most of the time," not a boot failure.
- **HS256-only, hand-rolled JWT, no asymmetric option anywhere** — the direct,
  concrete evidence for why CLAUDE.md P2 mandates asymmetric-only JWT with
  `HS*`/`none` rejected at the parser; the predecessors are exactly the deployment
  shape P2 rules out architecturally.
- **Topic-grain-only access, no data-source/dataset/row grant underneath (the
  headline scar this brief was scoped to document).** Every principal who can read a
  topic has identical data access to every other principal who can read that same
  topic. D-015 is a direct response to this exact code, not a hypothetical
  improvement.
- **One warehouse credential per deployment; no data-source multi-tenancy to
  inherit.** Any multi-tenant deployment of either predecessor necessarily shares one
  warehouse connection across every tenant it serves, with no independent
  credential-level blast-radius limit.
- **Regex-only injection check presented alongside real AST governance** — risk is
  not that the predecessor is unsafe (the AST/allowlist layer carries the weight)
  but that a maintainer could mistake the regex for meaningful defence, or drop the
  AST layer in a refactor while leaving the regex, believing coverage unchanged.
- **No DB-level tenant constraint anywhere** — isolation is 100% application-code
  discipline in both predecessor families independently, not a one-off.
- **Two manually-maintained allowlists with no mechanical "did you forget" check** —
  the tenant-isolation registry and the `@audited` decorator are the same shape of
  gap (a hand-maintained list a reviewer must remember to update) in two different
  subsystems of the same codebase.
- **`x-<pkg>-authorization` instead of standard `Authorization`** — described
  only as "proxy-safe," no rationale in reviewed source for what specifically about
  the standard header was unsafe. Flag as an open question, not an inherited
  pattern.
- **Row/result caps are caller-supplied, not independently enforced by the
  validator** — confirm before the RFC assumes any request-shape cap is a hard
  ceiling.

## What D-015/D-016/D-017 must cover

**D-015 (per-principal deny-by-default ACL).**

1. Define the access primitive with **at least one grain finer than "topic
   membership"** — a dataset, a data source, or a row/column predicate independent
   of topic visibility. Topic-grain-only access is workable for a
   single-tenant-per-warehouse deployment but breaks exactly where the
   data-engineering stage lives: engineered datasets/materializations need their own
   grants, not "whoever can see the topic can see the materialization."
2. Specify how a **per-agent principal** composes with a **per-topic/per-dataset
   grant** — the fork's two-mechanism stack (capability scopes gate endpoint
   families; `topic_ids` gate topic visibility) is a workable template for two
   orthogonal dimensions, but Chartworks needs a third (dataset/source-level) neither
   predecessor has.
3. Require the primitive be **computed once into a frozen envelope from a validated
   token claim**, never re-derived from a header past the identity-mint boundary —
   and require the **mint boundary itself** be cryptographically bound, closing the
   scar this brief found (an unbound header at `/auth/exchange`), not only the
   request-time header-trust scar Soundings' brief already closed.
4. Mandate a **scope-debug diagnostic** (admin-only, read-only, never surfaced in a
   tool/user-facing result) recording which predicate failed — tenant mismatch,
   topic-grant miss, dataset-grant miss, row-predicate miss — since neither
   predecessor has this and the existing audit middleware structurally cannot
   produce it.

**D-016 (encrypted-at-rest data-source credentials).**

1. No predecessor has a per-tenant/per-data-source credential store to port; pin
   envelope-encryption mechanism and key source explicitly rather than defaulting to
   "encrypted somehow."
2. Adopt Lakebase's **rotation** discipline as a design target (even though its
   *storage* pattern — never persist, fetch-fresh — doesn't apply to long-lived
   customer credentials): define a rotation cadence and a fail-loud behavior for a
   credential that fails to rotate, rather than silently continuing on a stale one.
3. Require a **content-free credential-event log** (present → absent → refreshed →
   rotation-failed) explicitly, citing the predecessors' clean log-the-event-not-the-
   value pattern as the bar, applied to **every** adapter kind Chartworks ships — the
   predecessors' unused Postgres/BigQuery config fields show how easily a second
   adapter kind lags the first one's credential hygiene.

**D-017 (NLQ read-only vs. governed DE write path).**

1. Inherit the **AST-first, whole-tree governance check** structurally as the
   binding mechanism for "no DDL/DML ever" on the NLQ path, but pair it with an
   **independent execution-time enforcement** (a read-only warehouse role/session
   setting, or an explicit dry-run/plan check) rather than relying on validation
   alone — closing the "validator is the entire read-only guarantee" gap this brief
   found.
2. Require the **schema/column allowlist be sourced from the same access primitive
   D-015 defines**, not only topic-pack membership as today — a query validated
   against "this topic's tables" is not automatically validated against "this
   caller's grants"; the predecessors conflate the two only because, for them, they
   are the same thing.
3. For the DE stage's governed write path (no predecessor analog), extend the same
   AST-first allow/deny shape with an explicit **write-destination allowlist**, and
   require the write path and the NLQ read path be **structurally incapable of
   sharing an execution entrypoint** (distinct adapter methods/interfaces, not one
   `execute()` distinguished only by an `is_read_only` flag a caller could omit) —
   flag-gated behavior is exactly the shape a future refactor could silently invert.
4. Explicitly **do not carry forward** the regex-based injection-pattern check as a
   named security control; if kept at all, label it a defence-in-depth smoke check,
   not the injection guardrail P1 requires — the actual guardrail is the
   AST/allowlist layer, and the RFC should say so plainly.
