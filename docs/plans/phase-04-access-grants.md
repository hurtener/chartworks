# Phase 04 — `access-grants`

> **Status:** draft
> **Owner:** orchestrator (Wave 2)
> **Depends on:** phase-02-store-migrations, phase-03-auth-identity

This is the **P1a phase**: it makes deny-by-default access a computed-in-the-query-path
property with one grants relation, one resolver, and a structurally unforgeable
"no scope ⇒ no query" guarantee. Everything downstream that reads a store row or a
customer warehouse consumes what this phase produces.

---

## RFC / request sections

- **RFC-001 §5** (the whole section — `access-grants` owns it): §5.1 grants (three grains,
  three permissions), §5.2 the one resolver → `EffectiveAccess`, §5.3 capability scopes
  (token-carried), §5.4 per-decision observability + scope-debug, §5.5 standing adversarial
  obligations.
- **RFC-001 §1.2 — P1a** (deny-by-default grants, computed in the query path) — the binding
  property this phase concretizes.
- **RFC-001 §4.2** (the frozen `identity.Envelope`) — the resolver attaches `EffectiveAccess`
  to the envelope phase 03 froze; identity/role/scopes are read once from the validated
  claim, never a header.
- **RFC-001 §12** — the `grants` table (`unique (tenant, principal, grain, resource)`) this
  phase reads; the `audit_events` and metric rows a decision emits.
- **RFC-001 §15** — `access_decisions_total{grain,decision,reason}` and the content-free
  decision event; scope-debug as the sanctioned admin diagnostic.
- **Decisions:** D-020 (the access primitive — this phase implements its mechanism), D-015
  (the principle D-020 concretizes), D-021 (allowlisting is sourced from *topic pack ∩
  caller grants* — this phase owns the grant half of that intersection), D-030 (principals
  are token-carried; no membership table to resolve roles from).

## Depends on

- **phase-02-store-migrations** — owns `internal/store`, the `grants` table, and the
  **non-optional scope parameter** on every store read/write method (`store.Scope`). This
  phase's resolver produces `store.Scope` values and reads grant rows through the store's
  grant-read methods. The store already enforces the inescapable `tenant_id` predicate at
  the storage layer (P3); this phase supplies the *resource* predicate that rides alongside
  it.
- **phase-03-auth-identity** — owns `internal/identity` and the frozen `identity.Envelope`
  (`tenant`, `principal`, `session`, `scopes`, `role`). The resolver runs against that
  envelope and hangs `EffectiveAccess` off it; it never re-reads a claim or a header.

## Informing briefs

- `docs/research/04-predecessor-security-tenancy.md` — the security/tenancy diff; direct
  recommendations for D-015 (per-principal deny-by-default ACL), the scope-debug diagnostic,
  and the registry-driven cross-tenant probe suite.
- `docs/research/05-predecessor-diff.md` — the access-model synthesis: the client
  predecessor's grant/visibility *data model* computed through the fork's single
  *access_resolver*, with the fork's nullable-`tenant_id` regression named as the
  counterexample.
- `docs/research/01-predecessor-architecture.md` — the unified deny-by-default resolver
  keeper (`no grant ⇒ absent`, `role nil ⇒ deny`, admin sentinel, tenant intersected
  *after* grant resolution so a grant can never leak cross-tenant).

## Brief findings incorporated

- **Brief 05 synthesis (the crux):** take the **client's grant data model** (explicit
  per-resource grants + `tenant_id NOT NULL` everywhere) and compute it through the **fork's
  single resolver** (one access representation, admin-bypass as an explicit sentinel,
  tenant intersected after). This phase implements exactly that: one `grants` relation
  (phase 02's table), one `access.Resolve`, no second access path (P7).
- **Brief 01 resolver keeper (carried verbatim as resolver invariants):** `no grant ⇒
  absent`; `role nil ⇒ deny` (here: an unrecognized/absent role floors to `viewer` —
  explicit-grants-only — never widens); **tenant boundary always intersected *after* grant
  resolution** so a resolved grant can never cross tenants; a service/agent principal gets
  self-sufficient grants only (never its owner's). The admin bypass is an explicit resolver
  branch that still passes through the tenant intersection — never a skipped check.
- **Brief 04 → D-015 requirements:**
  - Req 1 — a grain **finer than topic membership**: this phase carries all three grains
    (`source`/`topic`/`dataset`); dataset-grain is what makes an engineered materialization
    grantable independently of its topic (the DE stage's governability, brief 04 §"D-015").
  - Req 2 — a **per-agent principal composing with per-resource grants** across two
    orthogonal dimensions: capability **scopes** gate operation families (token-carried),
    **grants** gate resources (store-carried); *both must pass*. Agents are first-class
    (`agent:<id>` holds its own grants).
  - Req 3 — the primitive is **computed once into the frozen envelope from the validated
    claim**, never re-derived from a header past the mint boundary.
  - Req 4 — a **scope-debug diagnostic** (admin-only, read-only, never in a tool/user
    result) that records *which predicate* failed — the exact gap neither predecessor's
    audit middleware could produce.
- **Brief 04 registry-driven adversarial suite (the client keeper):** the cross-tenant
  probe suite enumerates tenant-sensitive targets **from a registration table, not a
  hand-maintained list** — closing the predecessors' "manually maintained allowlist admits
  partial scope, silently" gap (brief 04 §"registry is manually maintained"). At phase 04
  the registration table is the resolver's `(grain, permission)` matrix; the route/tool
  enumeration layers onto the same probe harness at phases 21/22.
- **Brief 04 read-as-404 (deferred, seam noted):** denied-reads-as-not-found
  (existence-hiding) is a *surface* behavior; this phase produces the typed `access.none` /
  `access.denied` decision the HTTP surface (phase 21) maps to 404. The decision vocabulary
  is fixed here so the surface has a stable contract.

## Findings I'm departing from

- **Brief 04's `x-<pkg>-authorization` proxy-safe header** — explicitly *not* carried
  (flagged by the brief itself as an open question, not a pattern). Identity, tenant, role,
  and scopes come only from the validated token via the frozen envelope (P2); no `X-*`
  header is consulted. The forged-header adversarial test proves it has no effect.
- **Brief 04's caller-supplied row/result caps** — out of scope here; caps are an `exec`
  concern (phases 09/10, RFC §9.6). This phase gates *which resources* are reachable, not
  *how many rows* come back.
- **Regex "injection heuristics" as a control** — not this phase's concern and, per D-021,
  never a named control anywhere. The access layer gates resources; the AST allowlist
  (phase 09) is the injection guardrail.

## Scope

Delivers `internal/access`:

- **The grant model types** — `Grain ∈ {source, topic, dataset}`, `Permission ∈ {read,
  query, manage}`, `Role ∈ {admin, member, viewer}`, `Grant`, and the token-carried
  capability-scope set (RFC §5.3, a closed enum). These pure types are the one access
  representation (P7); `internal/store` imports them for its grant-read signatures — the
  dependency runs one way (`store → access`), so there is no cycle.
- **The one resolver** — `Resolve(ctx, envelope) → (EffectiveAccess, error)`: role defaults
  ∪ explicit grants, the admin sentinel, tenant intersected after. Constructed once,
  immutable, concurrency-safe; per-request state rides `ctx`/params (CLAUDE.md §5).
- **`EffectiveAccess`** — the per-grain visible/queryable/manageable resource sets (or the
  admin sentinel), hung on the frozen envelope. Its `ScopeFor(grain, permission) →
  store.Scope` is the **only** sanctioned way to obtain a query scope; the query-issuing
  helper `Guard(...)` refuses an empty non-admin scope with a typed `ErrAccessNone`
  **before any store call**.
- **Per-decision telemetry** — `access_decisions_total{grain,decision,reason}` +
  content-free `audit_events` decision rows, emitted through the phase-01 telemetry seam on
  every allow/deny.
- **Scope-debug** — the read-only resolver-replay that names the failed predicate
  (`tenant_mismatch` / `no_grant_at_grain` / `scope_missing`); exposed as an admin-only
  library entry point that the admin API (phase 21) and admin CLI (phase 23) wrap. The
  routing-eligibility predicates (`topic_not_published`, `source_unavailable`) are reserved
  in the vocabulary and wired by phases 15/17.
- **The registry-driven adversarial harness** — a mechanically enumerated cross-tenant
  probe over the resolver's `(grain, permission)` matrix, plus the standing empty-access,
  forged-header, and fetch-then-filter guards. The harness is the seam phases 21/22 extend
  by enumerating their route/tool tables.

## Non-goals

- **Grant CRUD surfaces** (HTTP `/v1/grants`, principal listing) — phase 21. This phase
  reads grants and resolves them; authoring them is a management-plane surface.
- **Roles as a persisted membership model** — there is no `tenant_memberships` table
  (D-030); role is a token claim. This phase consumes it, does not manage it.
- **Routing eligibility** (`published ∩ healthy ∩ granted`) — phase 17 composes the grant
  set this phase produces with topic health/publication; only the grant predicate and the
  reserved predicate names live here.
- **The scope∩grant *allowlist* inside SQL validation** — phase 09/18 intersect the topic
  pack with the grant set this phase resolves; this phase owns the grant half only.
- **Store method signatures / the `grants` migration** — owned by phase 02.

## Design

### The grants relation (RFC §5.1)

One relation, read from phase 02's `grants` table:

```
Grant := (tenant_id, principal, grain ∈ {source, topic, dataset},
          resource_id, permission ∈ {read, query, manage}, granted_by, ts)
          unique (tenant_id, principal, grain, resource_id)
```

- **read** — see the resource's metadata (catalog listing, description, schema).
- **query** — have NLQ answered from it / execute against it.
- **manage** — mutate it (edit topics, run pipelines, rotate credentials).

Permissions are ordered `read < query < manage`; a higher grant implies the lower for
visibility (a `query` grant implies `read`), pinned in the resolver and unit-tested.

### The one resolver (RFC §5.2, brief 01/05)

```
Resolve(ctx, envelope) → EffectiveAccess
```

`internal/access` defines a `GrantReader` interface (`GrantsFor(ctx, tenant, principal) →
[]Grant`); the postgres store satisfies it structurally, so `internal/access` does **not**
import `internal/store` for grant reads — the resolver is testable against a spy reader.
Resolution, in order:

1. **Role defaults.** From `envelope.Role()`:
   - `admin` → the **admin sentinel** (`manage` on all tenant resources, all grains). This
     is an explicit resolver branch (`EffectiveAccess.adminAll = true`), never a skipped
     check — every downstream predicate still runs, it just resolves to "all within this
     tenant."
   - `member` → implicit `read` on the catalog grain(s); everything else by explicit grant.
   - `viewer` (**and any absent/unrecognized role — the `role nil ⇒ deny` floor**) →
     explicit grants only, no implicit visibility.
2. **Explicit grants.** Union the principal's `Grant` rows into per-grain, per-permission
   sets. Agents resolve `agent:<id>`'s **own** rows only — never the owner `user:<id>`'s
   (brief 01/05; the join key is the principal string, and the owner is never substituted).
3. **Tenant intersection — always last.** Every resolved entry is intersected with
   `envelope.Tenant()`; a grant row whose `tenant_id` differs is dropped. A grant can never
   leak cross-tenant regardless of any other axis (P3), and the admin sentinel is
   tenant-bounded (a tenant admin is not a platform admin).

The resolver is a shared singleton, immutable after construction; `Resolve` reads only its
arguments, so it is safe under concurrent reuse (`-race`).

### `EffectiveAccess` on the envelope, and why a scope-less query is unexpressible (P1a)

`EffectiveAccess` rides the frozen envelope and is the sole source of query scope:

```
func (ea EffectiveAccess) ScopeFor(g Grain, p Permission) store.Scope
```

- `store.Scope` (defined in phase 02) is the **non-optional parameter type on every store
  read/write method** and on every warehouse-`Query` construction. There is no store query
  method that omits it — a method without it does not compile and is rejected in review
  (RFC §5.2); an architecture test asserts the invariant over `internal/store`'s exported
  query methods.
- `store.Scope`'s **zero value is deny** (empty permitted set, `adminAll=false`): a
  forgotten or default-constructed scope denies rather than widens. Deny-by-default is the
  *type's* default, not a branch a code path could forget.
- The admin-all scope is producible **only** through `ScopeFor` off a resolved admin
  `EffectiveAccess`; it is not a bare boolean any caller can set to bypass the resource
  predicate.
- **The short-circuit is before I/O.** The access-issuing helper `Guard(ea, g, p, run)`
  computes `ScopeFor`; if the scope is empty and non-admin it returns `ErrAccessNone`,
  increments the deny metric, emits the content-free event, and **never invokes `run`** (the
  store closure). The store is not called — proven by a call-count assertion on a spy store
  (criterion 1). Non-empty ⇒ the scope becomes an intersected SQL predicate
  (`tenant_id = $1 AND resource_id = ANY($2)`; admin ⇒ `tenant_id = $1`), applied *inside*
  the query — never fetch-then-filter.

### Scope-debug flow (RFC §5.4)

`ScopeDebug(ctx, principal, target) → Decision` replays a hypothetical
`(principal, resource)` (or, later, `question`) through the resolver and reports the first
failing predicate from a fixed vocabulary:

- `tenant_mismatch` — the target's tenant ≠ the principal's tenant.
- `no_grant_at_grain` — no grant of the required permission at the resource's grain.
- `scope_missing` — the required capability scope is absent from the token.
- *(reserved, wired by 15/17)* `topic_not_published`, `source_unavailable`.

It is read-only, admin-gated, and content-free; it is **never** a user-facing or
self-service surface (CLAUDE.md §8) — the exact anti-pattern (a user-facing "repair"
surface) the predecessors grew. The admin API and CLI are thin wrappers over this one
function (P7).

### Both-must-pass: scopes gate operations, grants gate resources (RFC §5.3)

Capability **scopes** (token-carried, the closed V1 enum) gate operation families; **grants**
gate resources. A decision requires *both*: a caller holding `dataset.read` scope but no
`dataset` grant on resource X is denied `no_grant_at_grain`; a caller holding the grant but
missing the scope is denied `scope_missing`. Neither substitutes for the other.

### Per-decision telemetry (RFC §5.4/§15)

Every allow and every deny increments
`access_decisions_total{grain,decision∈{allow,deny},reason}` and emits a content-free
`audit_events` row (ids + principal + decision + reason + request-id — never a resource
value, never a token byte) through the phase-01 telemetry/audit seam. The empty-set
short-circuit is the `reason="empty_set"` deny (criterion 2).

## Config keys added

**None.** Access is grant-data-driven (the `grants` table) and token-carried (scopes +
role in the validated claim); there is nothing tunable, and RFC §14 defines no `access`
config domain. Scope-debug is always-available and admin-gated (no enabling flag — a flag
would be a way to silently disable a diagnostic). Recorded here as a deliberate "none"
per the template.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| — | — | — | — | none — access is grant-data-driven + token-carried (RFC §14 has no `access` domain) |

## Acceptance criteria

1. **Deny-by-default issues no query.** A non-admin principal with no grant, resolving any
   grain, yields typed `access.None`; `Guard` returns `ErrAccessNone` and the store is
   **never called** — asserted by a spy `store`/`GrantReader` whose query-call counter reads
   `0` (the store-call-count proof).
2. **Empty-set short-circuit is metered.** That same path increments
   `access_decisions_total{grain,decision="deny",reason="empty_set"}` exactly once and emits
   exactly one content-free decision event.
3. **Agent principals resolve their own grants.** For `agent:A` owned by `user:U` where
   `user:U` holds grants `agent:A` lacks, `Resolve` on the `agent:A` envelope produces an
   `EffectiveAccess` that excludes `user:U`'s grants (table-driven; the owner is never
   substituted).
4. **Admin sentinel, tenant-bounded.** A principal with `role=admin` resolves to the
   admin-all sentinel (`manage` on all grains) via the explicit resolver branch; a
   cross-tenant target still fails with `tenant_mismatch` (an admin of tenant T1 cannot
   reach tenant T2's resources).
5. **Tenant intersected after resolution.** A grant row whose `tenant_id` ≠ the envelope
   tenant is never present in `EffectiveAccess` (cross-tenant grant-leak guard,
   table-driven).
6. **Role defaults.** `member` ⇒ implicit catalog `read` only; `viewer` and any
   absent/unrecognized role ⇒ explicit grants only (no implicit visibility) — table-driven.
7. **Scope-debug names the failed predicate.** For each denial cause, `ScopeDebug` returns
   the matching predicate name (`tenant_mismatch` / `no_grant_at_grain` / `scope_missing`) —
   a table mapping each cause to its predicate, one row per predicate.
8. **Both-must-pass.** Grant present + scope absent ⇒ `scope_missing`; scope present + grant
   absent ⇒ `no_grant_at_grain` (table-driven, both directions).
9. **Registry-driven adversarial suite green.** The cross-tenant probe enumerates
   tenant-sensitive targets from the resolver's `(grain, permission)` registration matrix
   (not a hand list); each cross-tenant probe denies (a bare allow fails). Empty-access,
   forged-header (no effect — identity from the envelope only), and fetch-then-filter
   regression guards all pass.
10. **Resolver is concurrency-safe.** `Resolve` under `-race` with concurrent distinct
    envelopes on the shared resolver singleton produces per-envelope-correct results with no
    shared mutation (concurrent-reuse test).

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven over the resolver (criteria 3–8): role × grain × permission ×
  grant-set matrices; the permission-ordering (`read<query<manage`) implication; `ScopeFor`
  zero-value-denies; the admin sentinel branch. Golden test on the decision-event shape
  (content-free: ids/principal/decision/reason/request-id, no resource value).
- **Integration:** required — this phase closes a seam phase 02 opened (the `store.Scope`
  parameter + grant reads) and introduces a public interface (`access.Resolve`,
  `EffectiveAccess`, `Guard`) every later phase builds on (§17). Runs the resolver against a
  **real Docker Postgres** (`make pg-up`) with real `grants` rows and a real
  phase-03-issued token → frozen envelope, proving identity/scope propagation and the
  intersected-predicate query end to end, under `-race`, covering ≥1 failure mode
  (empty-set short-circuit issues no query — the call-count proof at the real seam).
- **Adversarial:** required (ACL path) — the registry-driven cross-tenant probe (criterion
  9), empty-access-set probe, forged-header attempt, and the fetch-then-filter regression
  guard. Mechanically enumerated from the resolver's `(grain, permission)` matrix; the
  route/tool enumeration is the seam phases 21/22 extend.
- **Fuzz:** n/a — this phase parses no wire/decode surface (JWT parsing fuzz lives in phase
  03; SQL-validation fuzz in phase 09). The resolver's inputs are typed envelope/grant
  values, not bytes.
- **Bench:** `BenchmarkResolve` — the resolver is on every request's hot path; a baseline
  (not a CI gate) to catch a resolution-cost regression as grant counts grow.

## Coverage targets

`internal/access` is an access package — the **85% band** applies (CLAUDE.md §11 /
master-plan convention 4). Added to `scripts/coverage-bands.conf` in this PR.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/access` | 85% | access/ACL package — the security-sensitive band, not the 80% default |

## Smoke checks

Each criterion maps to a Go test exercised by `scripts/smoke/phase-04.sh` (via
`run_group`), which SKIPs cleanly until `internal/access` is built.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestDenyByDefaultIssuesNoQuery` — spy store query-call counter == 0 on empty access |
| 2 | `TestEmptySetShortCircuitMetric` — `deny/empty_set` counter +1 and one decision event |
| 3 | `TestAgentResolvesOwnGrants` — agent EffectiveAccess excludes owner's grants |
| 4 | `TestAdminSentinelTenantBounded` — admin-all resolves; cross-tenant target ⇒ `tenant_mismatch` |
| 5 | `TestTenantIntersectedAfterResolution` — foreign-tenant grant absent from EffectiveAccess |
| 6 | `TestRoleDefaults` — member catalog-read-only; viewer/unknown explicit-only |
| 7 | `TestScopeDebugNamesPredicate` — each denial cause ⇒ its predicate name |
| 8 | `TestBothMustPass` — grant∖scope ⇒ `scope_missing`; scope∖grant ⇒ `no_grant_at_grain` |
| 9 | `TestAdversarialRegistryCrossTenant` — enumerated matrix probed cross-tenant, all deny; empty/forged-header/fetch-then-filter guards pass |
| 10 | `TestResolveConcurrentReuse` — `-race` concurrent `Resolve`, per-envelope-correct |

## Glossary additions

Only terms this phase introduces that are not already in `docs/glossary.md` (which already
carries **grant**, **principal**, **scope-debug**). To land in the same PR:

- **EffectiveAccess** — the caller's resolved, per-grain visible/queryable/manageable
  resource sets (or the admin sentinel), computed once per request by the one resolver and
  hung on the frozen identity envelope; its `ScopeFor` is the only sanctioned source of a
  query scope (RFC §5.2, D-020).
- **Capability scope** — a token-carried permission over an *operation family* (e.g.
  `query.plan`, `topic.publish`); distinct from a grant, which is over a *resource*. Both
  must pass (RFC §5.3).
- **Tenant role** — the token-carried default tier `admin` / `member` / `viewer`: admin ⇒
  the manage-all sentinel, member ⇒ implicit catalog read, viewer (and any absent role) ⇒
  explicit grants only (RFC §5.2, D-030).
- **Admin sentinel** — the explicit resolver branch granting a tenant admin `manage` on all
  tenant resources; an explicit "all within this tenant" value, never a skipped check, and
  always tenant-bounded (brief 01/05).
- **Access decision** — a resolver allow/deny carrying `{grain, decision, reason}`; every
  one increments `access_decisions_total` and emits a content-free audit event (RFC §5.4).
- **Registry-driven cross-tenant probe** — the adversarial suite that enumerates
  tenant-sensitive targets from a registration table (never a hand-maintained list) and
  probes each cross-tenant; a bare allow fails CI (RFC §5.5, brief 04).

## Decisions filed

**None new.** This phase implements existing decisions:

- **D-020** — the access primitive (one grants relation, three grains, one resolver,
  deny-by-default, agents first-class, scopes token-carried): this phase *is* its mechanism.
- **D-015** — the per-principal deny-by-default ACL principle D-020 concretizes.
- **D-021** — the *topic pack ∩ caller grants* allowlist intersection: this phase owns the
  grant half of that intersection (phases 09/18 own the pack half).
- **D-030** — principals/roles are token-carried; no membership table to resolve from.

A future decision is filed only if the reserved routing-eligibility predicates
(`topic_not_published`, `source_unavailable`) need a shape change when phases 15/17 wire
them — expected to be additive, not a change to this phase's contract.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). Empty at authoring time. -->

- (none yet)
