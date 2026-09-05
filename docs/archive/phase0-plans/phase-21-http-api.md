# Phase 21 — `http-api`

> **Status:** draft
> **Owner:** orchestrator
> **Depends on:** phase-04-access-grants, phase-08-sources-core, phase-11-uploads-workspace,
> phase-13-engineering-pipelines, phase-15-topics-lifecycle, phase-16-rules-clarification,
> phase-17-nlq-routing-context, phase-18-nlq-generation-execution, phase-19-byo-mode,
> phase-20-charts-spec (and, transitively, 01–03 foundations)

Wave 6. Difficulty: high. Owns `internal/api`: the full RFC §11.2 HTTP surface —
routing, request/response validation, typed error mapping (§9.5 vocabulary → HTTP),
transport hardening, multipart uploads, the middleware stack, mechanically-derived
audit + cross-tenant coverage from one route-descriptor table, and the operator-scoped
`/metrics` mount. Handlers are **thin**: they decode, call one core service, and encode
(P7). No business logic lives here.

---

## RFC / request sections

- **RFC-001 §11.2** — the HTTP surface (this phase): the group→endpoint table, `/v1`
  prefix, both audiences validated identically with the HTTP `aud` distinct from the MCP
  `aud`, transport hardening set explicitly, bearer-only (no cookie fallback).
- **RFC-001 §9.5** — the typed validation error vocabulary (`statement.blocked`,
  `table.not_granted`, `table.not_in_topic`, `column.unknown`, `join.unreachable`, …),
  **normative for both surfaces** → this phase pins its HTTP-status mapping.
- **RFC-001 §5.2–5.4** — the resolver + `EffectiveAccess` on the frozen envelope, capability
  scopes (§5.3), per-decision observability, and scope-debug (`GET /v1/admin/scope-debug`).
- **RFC-001 §5.5** — the registry-driven cross-tenant probe suite (mechanical, route-table
  derived).
- **RFC-001 §9.7** — the normalized answer envelope `run_query` returns (routing evidence,
  assumptions, SQL + provenance + validation report, result preview, chart spec, warnings).
- **RFC-001 §11.1 / D-019** — the MCP tool set the Ask/BYO parity skeleton mirrors;
  management-plane operations are **HTTP-only** in V1.
- **RFC-001 §15** — RED-per-route metrics, content-free store-backed audit with **mechanical**
  route-table coverage, health composition (`/healthz`, `/readyz`), operator-scoped `/metrics`.
- **RFC-001 §14** — the `server` config domain (addresses, timeouts, body limits, CORS).
- **Decisions:** D-006 (dual-audience bearer JWT), D-012 (three consumer classes — Console
  HTTP, standalone API customers), D-017 (NLQ read-only / DE write-path split, honoured by
  routing pipeline vs query endpoints), D-019 (MCP parity contract), D-020 (grants + scopes),
  D-021 (validation error vocabulary), D-024 (uploads first-class), D-030 (no local user
  management — admin/tenants + API keys only, no login/signup/header-mint routes).

## Depends on

- **phase-04 (`access-grants`)** — the one resolver → `EffectiveAccess` on the envelope, the
  capability-scope set (§5.3), the per-decision counters, scope-debug, and the
  **registry-driven adversarial harness** this phase feeds its route table into. The scope
  gate and the resource-grant checks consume phase-04; they are never re-implemented here.
- **phase-08 (`sources-core`)** — the sources/uploads/datasets read + management endpoints
  call the connections registry, custody, and discovery services.
- **phase-11 (`uploads-workspace`)** — `POST /v1/uploads` (multipart) streams to the upload
  workspace provisioning + dataset-registration service.
- **phase-13 (`engineering-pipelines`)** — pipelines CRUD/run/draft, runs, schedules call the
  pipeline + materializer + scheduler services (the governed write path, P1c/D-017).
- **phase-15/16 (`topics-lifecycle`, `rules-clarification`)** — the topics/versions/lifecycle-
  transition/export-import/generate and rules endpoints call the semantics services;
  `:recheck-source`/`:revalidate` are the domain-clean lifecycle ops (never "repair").
- **phase-17/18/19 (`nlq-*`, `byo-mode`)** — `POST /v1/query:*` (preflight/plan/run/refine,
  context/submit-sql) call the identical NLQ + exec + BYO core; the HTTP surface adds no
  validation/execution logic of its own (D-021, D-022).
- **phase-20 (`charts-spec`)** — the `run_query` response embeds the chart spec envelope.
- **phases 01–03** — the binary/config framework, the telemetry registry + request-id
  primitives + audit-emitter, JWT validation, and the frozen `identity.Envelope`.

This phase **closes the HTTP half** of the surface seam the core services opened; it opens
the public HTTP contract phase 23 (SDK + parity) builds on.

## Informing briefs

Per `docs/research/INDEX.md` (`internal/api` → primary **01**; secondary **04**, **06**):

- `docs/research/01-predecessor-architecture.md`
- `docs/research/04-predecessor-security-tenancy.md`
- `docs/research/06-predecessor-frontend-charts.md`

## Brief findings incorporated

- **Brief 01 — the full HTTP API inventory (shape, not the scars).** The predecessors'
  router groups (catalog/topics/rules/nlq/sessions/feedback/schedules/datasets/admin) map
  onto RFC §11.2's groups; this plan carries the *grouping and lifecycle-verb shape* while
  dropping the predecessor-only groups (auth login, templates, GEPA, maintenance, dashboard,
  relationships — see departures).
- **Brief 01 — the `@audited(action, resource_type, target_param)` decorator + audit
  middleware.** Adopted as *shape*, hardened against its named gap ("only fires for routes
  explicitly decorated; coverage not verified complete"): audit metadata rides the **single
  route-descriptor table**, and a test iterates that table asserting every tenant-sensitive /
  mutating route carries an audit action (criterion 2). Content-free events (actor, target,
  decision, request-id; **no** body, no SQL text — that lives in the `queries` table).
- **Brief 01 — the dead-in-process-metrics scar.** `/metrics` is a real, operator-scoped
  Prometheus mount (§15), consuming phase-01's telemetry conformance test (every registered
  counter is exported); RED-per-route counters are registered at route mount, not ad hoc
  (criterion 12).
- **Brief 01 — the silent cookie/CSRF channel.** Dropped entirely: V1 is **bearer-only**, no
  cookie transport, so CSRF middleware is `n/a` by construction (criterion 9).
- **Brief 04 — 404-for-denied, not 403 (existence-hiding keeper).** A resource the caller has
  no grant on returns a 404 identical to a genuinely-absent resource, never a 403 that
  confirms existence (criterion 5). Capability-*scope* absence (an operation-*family* the
  token can't perform) is a 403 — the two are deliberately distinct (criteria 5 vs 6).
- **Brief 04 — the tenant-mismatch 400 guard.** When a path/param tenant disagrees with the
  token tenant, reject with a typed `tenant.mismatch` 400 rather than silently picking one by
  precedence (criterion 4). Chartworks has *no* tenant header at all (identity is the frozen
  envelope), so the only disagreement possible is path/param vs. token.
- **Brief 04 — the mechanical, registry-driven cross-tenant suite.** Adopted, but the registry
  is **derived from the route-descriptor table**, not hand-maintained — closing brief 04's own
  "registry is manually maintained and admits partial scope" gap (criterion 3). Accepts only
  400/401/403/404; a bare 2xx cross-tenant fails CI.
- **Brief 04 — the content-free structured audit event** paired with a mechanical "every
  tenant/topic-sensitive route is audited" check (criterion 2).
- **Brief 06 — the live P6 "repair" surface scar.** The predecessor shipped user-facing
  "Repair tables" / `POST /templates/{id}:repair` / a `TemplateAuditAction` literal `"repair"`.
  This surface exposes **no** such path: the lifecycle ops are `:recheck-source` and
  `:revalidate` (the glossary's domain-clean names); "repair"/"broken"/"wiring" never appear in
  a route path, field name, error message, or audit action (enforced by `make drift-audit`).

## Findings I'm departing from

- **Brief 01's auth routes** (`/v1/auth/*`: password login/signup, "mint JWT from a trusted
  upstream header", `whoami`, logout) — **not carried** (D-030: no local user management;
  self-issue = API keys, first-admin via the `chartworks admin bootstrap` CLI). The
  header-mint route is the exact header-trust scar brief 04 flags; removing the surface deletes
  the class.
- **Brief 01's "Spreadsheet Mode" datasets surface** (a CSV/XLSX path outside the topic/
  warehouse model) — **not carried**: uploads are first-class sources via the workspace
  (D-024), reached through the same identity/grants/execution path as any warehouse. No
  parallel weak-auth channel.
- **Brief 01's `templates`, `GEPA`, `maintenance`, `dashboard`, `relationships` groups** — out
  of V1 scope (§11.2 does not enumerate them; RFC §19 non-goals).
- **Brief 04's manually-maintained tenant-isolation registry** — departed in favour of a
  route-table-*derived* registry (the partial-coverage gap is the thing we are fixing).
- **A CSRF/`_csrf_protect` middleware** — `n/a` (bearer-only; no cookie transport exists to
  protect).

## Scope

Package **`internal/api`**, delivering:

1. **The router + one route-descriptor table.** A single `[]Route` registration slice — each
   `Route{Method, Path, Scope, Audited bool, AuditAction, TenantSensitive bool, Handler}` — is
   the *sole* source of truth. The router mounts from it; the audit-coverage test, the
   cross-tenant probe registry, and the parity registry all iterate it (no second list).
2. **Thin handlers** for every RFC §11.2 endpoint (route table below), each `decode → core
   service → encode`, returning the normalized envelopes (§9.7, §9.4, §10) or a typed error.
3. **The middleware stack** (fixed order below) with the resolver run **once** into the frozen
   envelope.
4. **The typed-error → HTTP-status mapper** (§9.5 vocabulary + access/tenant/transport codes),
   emitting a content-free error body.
5. **Transport hardening** — an explicitly-configured `http.Server` (timeouts, body limit via
   `MaxBytesReader`, Content-Type enforcement, Origin/CORS allowlist), bearer-only.
6. **Multipart upload** handling for `POST /v1/uploads`, streamed to the workspace service.
7. **Health + operator-scoped metrics** mounts (`/healthz`, `/readyz` unauthenticated;
   `/metrics` operator-scoped).
8. **The Ask/BYO parity skeleton** — a shared capability registry the MCP surface (phase 22)
   and SDK (phase 23) also consume; the HTTP half is asserted now, the MCP column is pending.

### The full §11.2 route table (concrete)

All under `/v1` unless noted. `Scope` is the token-carried capability scope (§5.3); every
tenant-sensitive route *additionally* passes a resource-grant check in its core service (deny
⇒ typed not-found ⇒ 404). `A` = audited. `T` = tenant-sensitive (in the cross-tenant registry).

**Health (root, unauthenticated — the only unauthenticated routes, CLAUDE.md §7):**

| Method | Path | Scope | A | T |
|---|---|---|---|---|
| GET | `/healthz` | — (none) | | |
| GET | `/readyz` | — (none) | | |

**Sources:**

| Method | Path | Scope | A | T |
|---|---|---|---|---|
| GET | `/v1/sources` | `catalog.read` | | ✓ |
| POST | `/v1/sources` | `source.manage` | ✓ | ✓ |
| GET | `/v1/sources/{id}` | `catalog.read` | | ✓ |
| PATCH | `/v1/sources/{id}` | `source.manage` | ✓ | ✓ |
| DELETE | `/v1/sources/{id}` | `source.manage` | ✓ | ✓ |
| POST | `/v1/sources/{id}:test` | `source.manage` | ✓ | ✓ |
| PUT | `/v1/sources/{id}/credential` | `source.manage` | ✓ | ✓ |

**Uploads:**

| Method | Path | Scope | A | T |
|---|---|---|---|---|
| POST | `/v1/uploads` (multipart) | `source.manage` | ✓ | ✓ |
| GET | `/v1/uploads/{id}` (status) | `catalog.read` | | ✓ |

**Datasets:**

| Method | Path | Scope | A | T |
|---|---|---|---|---|
| GET | `/v1/datasets` | `dataset.read` | | ✓ |
| GET | `/v1/datasets/{id}` | `dataset.read` | | ✓ |
| GET | `/v1/datasets/{id}/profile` | `dataset.read` | | ✓ |
| POST | `/v1/datasets/{id}:refresh-profile` | `source.manage` | ✓ | ✓ |

**Pipelines:**

| Method | Path | Scope | A | T |
|---|---|---|---|---|
| GET | `/v1/pipelines` | `pipeline.manage` | | ✓ |
| POST | `/v1/pipelines` | `pipeline.manage` | ✓ | ✓ |
| GET | `/v1/pipelines/{id}` | `pipeline.manage` | | ✓ |
| PATCH | `/v1/pipelines/{id}` | `pipeline.manage` | ✓ | ✓ |
| DELETE | `/v1/pipelines/{id}` | `pipeline.manage` | ✓ | ✓ |
| POST | `/v1/pipelines/{id}:run` | `pipeline.run` | ✓ | ✓ |
| GET | `/v1/pipelines/{id}/runs` | `pipeline.manage` | | ✓ |
| GET | `/v1/pipelines/{id}/runs/{runId}` | `pipeline.manage` | | ✓ |
| POST | `/v1/pipelines:draft` (assisted, `pipeline_draft` role, draft-only) | `pipeline.manage` | ✓ | ✓ |

**Topics (CRUD + versions + lifecycle):**

| Method | Path | Scope | A | T |
|---|---|---|---|---|
| GET | `/v1/topics` | `topic.read` | | ✓ |
| POST | `/v1/topics` | `topic.write` | ✓ | ✓ |
| GET | `/v1/topics/{id}` | `topic.read` | | ✓ |
| PATCH | `/v1/topics/{id}` | `topic.write` | ✓ | ✓ |
| DELETE | `/v1/topics/{id}` | `topic.write` | ✓ | ✓ |
| GET | `/v1/topics/{id}/versions` | `topic.read` | | ✓ |
| GET | `/v1/topics/{id}/versions/{vid}` | `topic.read` | | ✓ |
| PATCH | `/v1/topics/{id}/versions/{vid}` (entity editing) | `topic.write` | ✓ | ✓ |
| POST | `/v1/topics/{id}:submit-review` | `topic.write` | ✓ | ✓ |
| POST | `/v1/topics/{id}:publish` | `topic.publish` | ✓ | ✓ |
| POST | `/v1/topics/{id}:rollback` | `topic.publish` | ✓ | ✓ |
| POST | `/v1/topics/{id}:archive` | `topic.publish` | ✓ | ✓ |
| POST | `/v1/topics/{id}:recheck-source` | `topic.write` | ✓ | ✓ |
| POST | `/v1/topics/{id}:revalidate` | `topic.write` | ✓ | ✓ |
| GET | `/v1/topics/{id}/grants` (sharing) | `topic.write` | | ✓ |
| POST | `/v1/topics/{id}:export` (P6-sanitized) | `topic.read` | ✓ | ✓ |
| POST | `/v1/topics:import` | `topic.write` | ✓ | ✓ |
| POST | `/v1/topics:generate` (`enhance` role, enqueues job) | `topic.write` | ✓ | ✓ |

**Rules:**

| Method | Path | Scope | A | T |
|---|---|---|---|---|
| GET | `/v1/topics/{id}/rules` | `topic.read` | | ✓ |
| POST | `/v1/topics/{id}/rules` | `topic.write` | ✓ | ✓ |
| PATCH | `/v1/topics/{id}/rules/{rid}` | `topic.write` | ✓ | ✓ |
| POST | `/v1/topics/{id}/rules/{rid}:activate` | `topic.write` | ✓ | ✓ |
| POST | `/v1/topics/{id}/rules/{rid}:retire` | `topic.write` | ✓ | ✓ |

**Query (Ask + BYO — the MCP-parity tier):**

| Method | Path | Scope | A | T |
|---|---|---|---|---|
| POST | `/v1/query:preflight` | `query.preflight` | ✓ | ✓ |
| POST | `/v1/query:plan` | `query.plan` | ✓ | ✓ |
| POST | `/v1/query:run` (accepts `Idempotency-Key`) | `query.execute` | ✓ | ✓ |
| POST | `/v1/query:refine` (session-scoped) | `query.execute` | ✓ | ✓ |
| POST | `/v1/query:context` (BYO bundle) | `query.context` | ✓ | ✓ |
| POST | `/v1/query:submit-sql` (accepts `Idempotency-Key`) | `query.submit` | ✓ | ✓ |
| GET | `/v1/queries` (history) | `catalog.read` | | ✓ |
| GET | `/v1/queries/{id}` | `catalog.read` | | ✓ |
| GET/POST/PATCH/DELETE | `/v1/saved-queries[/{id}]` | `catalog.read` (read) / `query.plan` (write) | ✓ (writes) | ✓ |

**Sessions:**

| Method | Path | Scope | A | T |
|---|---|---|---|---|
| GET | `/v1/sessions` | `catalog.read` | | ✓ |
| GET | `/v1/sessions/{id}` | `catalog.read` | | ✓ |
| GET | `/v1/sessions/{id}/queries` | `catalog.read` | | ✓ |

**Feedback:**

| Method | Path | Scope | A | T |
|---|---|---|---|---|
| POST | `/v1/feedback` | `feedback.write` | ✓ | ✓ |
| GET | `/v1/feedback` | `catalog.read` | | ✓ |

**Schedules:**

| Method | Path | Scope | A | T |
|---|---|---|---|---|
| GET | `/v1/schedules` | `pipeline.manage` | | ✓ |
| POST | `/v1/schedules` | `pipeline.manage` | ✓ | ✓ |
| GET/PATCH/DELETE | `/v1/schedules/{id}` | `pipeline.manage` | ✓ (writes) | ✓ |
| POST | `/v1/schedules/{id}:pause` / `:resume` | `pipeline.manage` | ✓ | ✓ |
| GET | `/v1/schedules/{id}/runs` | `pipeline.manage` | | ✓ |

**Grants (three grains):**

| Method | Path | Scope | A | T |
|---|---|---|---|---|
| GET | `/v1/grants` | `admin` | | ✓ |
| POST | `/v1/grants` | `admin` | ✓ | ✓ |
| DELETE | `/v1/grants/{id}` | `admin` | ✓ | ✓ |
| GET | `/v1/principals` | `admin` | | ✓ |

**Admin:**

| Method | Path | Scope | A | T |
|---|---|---|---|---|
| GET/POST/DELETE | `/v1/admin/tenants[/{id}]` (self-issue mode only) | `admin` | ✓ (writes) | (cross-tenant by design) |
| GET/POST/DELETE | `/v1/admin/api-keys[/{id}]` | `admin` | ✓ (writes) | ✓ |
| GET | `/v1/admin/scope-debug` (§5.4) | `admin` | ✓ | ✓ |
| GET | `/v1/admin/audit` (audit query) | `admin` | | ✓ |
| GET | `/metrics` (root, **operator-scoped**) | `admin` | | |

`/v1/admin/tenants` is mounted **only in self-issue mode** (D-030); in external-issuer mode
the route is not registered (a request 404s at the router, not a handler-level branch).

### Typed error mapping (§9.5 codes → HTTP status)

The mapper is a single table; a golden test pins it (criterion 7). Every error body is
content-free — `{ "error": { "code": "<typed>", "message": "<safe>" } }` — and **never** leaks
an internal id, endpoint URL, warehouse credential, model detail, SQL text, or a stack trace
(CLAUDE.md §6, §7).

| Typed condition | Code (example) | HTTP status |
|---|---|---|
| Missing/invalid/expired token, bad `aud`, stale JWKS (fail-closed) | `auth.unauthenticated` | 401 |
| Valid token lacking the route's capability scope | `scope.denied` | 403 |
| No grant on a specific resource (existence-hiding) / resource absent | `resource.not_found` | 404 |
| Path/param tenant disagrees with token tenant | `tenant.mismatch` | 400 |
| Empty effective-access set on a single-resource read | `access.none` → not_found | 404 |
| Empty effective-access set on a list read | `access.none` (short-circuit, no query) | 200 `[]` |
| Malformed request body / bad field | `request.invalid` | 400 |
| SQL validation failure (submit/plan) — `statement.blocked`, `table.not_in_topic`, `table.not_granted`, `column.unknown`, `join.unreachable`, multi-statement, parse | the §9.5 code, verbatim | 422 |
| Clarification required (ambiguous question) | `query.clarification_required` | 422 |
| No route / not routable | `query.no_route` | 422 |
| Body exceeds the configured limit | `request.too_large` | 413 |
| Missing/unsupported `Content-Type` on a write | `request.unsupported_media_type` | 415 |
| Disallowed `Origin` (CORS) | `request.forbidden_origin` | 403 |
| Warehouse statement timeout / context deadline | `exec.timeout` | 504 |
| Row/resource-cap exhaustion at execution | `exec.resource_exhausted` | 422 |
| Data source unavailable / gateway unreachable | `dependency.unavailable` | 503 |
| Idempotency-key conflict (in-flight/replayed) | `request.conflict` | 409 |

## Non-goals

- **The MCP tool surface** — phase 22 (this phase ships the shared capability registry the
  parity skeleton reads; the MCP column is filled there).
- **The SDK + admin CLI + full three-surface parity suite** — phase 23.
- **Any core service logic** — sources/engineering/semantics/nlq/exec/charts/access/feedback
  services are owned by their phases; `internal/api` only calls them (P7 thin surface).
- **Rendering charts / composing prose** — the response carries the declarative chart spec
  (§10); prose is the caller's job (§1.1).
- **A cookie/session transport, CSRF, login/signup, header-mint** — dropped by design (D-030,
  bearer-only).
- **Result pagination / cross-request result cache** — RFC §19 non-goals (idempotency keys
  cover retries).

## Design

### Middleware stack (fixed order)

```
request-id ─▶ auth ─▶ envelope ─▶ access-resolve ─▶ scope-gate ─▶ audit ─▶ handler
```

- **request-id** — reuse an inbound `X-Request-Id`/`X-Correlation-Id` or generate one
  (phase-01 primitive); it rides `ctx` and every log/audit/metric line. Outermost so even an
  auth rejection is correlated.
- **auth** — validate the bearer JWT exactly as the MCP surface does: asymmetric-only,
  `iss`/`aud` (the **HTTP** audience, distinct from MCP), `exp`/skew, JWKS fail-closed past
  `jwks_max_stale`. `HS*`/`none` rejected at the parser (phase 03). Identity/access are **never**
  read from an `X-*` header (P2). Failure ⇒ 401 before any handler.
- **envelope** — build the frozen `identity.Envelope` (tenant, user/principal, session) once
  from the validated claim; immutable thereafter (P2). The tenant-mismatch guard runs here:
  if a path/param tenant is present and disagrees with the envelope tenant ⇒ typed 400.
- **access-resolve** — call the phase-04 resolver **once** → `EffectiveAccess` onto the frozen
  envelope. Not re-resolved per handler; handlers read it from `ctx`.
- **scope-gate** — check the route's declared capability scope against the token's scopes
  (§5.3). Missing scope ⇒ 403 (`scope.denied`) before the handler. This is the operation-family
  gate; resource-grant checks happen inside the core service and surface as 404.
- **audit** — reads the route descriptor's `AuditAction`; on the way out it classifies the
  response (success/denied/error) from the status and emits a content-free `AuditEvent`
  (actor, resource type/id, decision, request-id). Because the action rides the route table,
  coverage is mechanical (criterion 2), not an opt-in decorator (brief 01's gap).
- **handler** — decode → one core-service call → encode. No business logic (criterion 13).

### Mechanical coverage derived from the route table

The route-descriptor slice is the single registry. Three consumers iterate it:

1. **Router mount** — every `Route` is mounted; a mounted path absent from the table (or a
   table entry never mounted) fails a structural test (criterion 1).
2. **Audit coverage** — a test asserts every `Audited || TenantSensitive` mutating route
   carries a non-empty `AuditAction` and that a request to it produces an audit event
   (criterion 2). Closes brief 01's opt-in gap and brief 04's "audit not verified complete".
3. **Cross-tenant adversarial registry** — every `TenantSensitive` route is enumerated and
   probed with a tenant-A token against a tenant-B resource; only 400/401/403/404 pass, a bare
   2xx fails CI (criterion 3, RFC §5.5). Derived, never hand-maintained (brief 04's fix).

### Transport hardening (explicit, never SDK defaults — §11.2, CLAUDE.md §7)

The `http.Server` is constructed field-by-field: `ReadHeaderTimeout`, `ReadTimeout`,
`WriteTimeout` (> the exec statement timeout so a legitimate `:run` completes),
`IdleTimeout`, `MaxHeaderBytes`. Every request body is wrapped in `http.MaxBytesReader` at the
configured limit (`server.http.max_body_bytes`, default 10 MiB); the multipart upload route
overrides this with the workspace ceiling. Write routes enforce `Content-Type:
application/json` (multipart for `/v1/uploads`); a mismatch ⇒ 415. `Origin` is checked against
`server.http.cors.allowed_origins` (default empty = same-origin only). Graceful shutdown
drains in-flight requests within `server.http.shutdown_grace`. A test asserts none of these are
left at the Go zero-value / SDK default (criterion 8).

### Bearer-only (no cookie transport)

No route reads or sets a cookie; auth is the `Authorization: Bearer` header only. A
cookie-only request is unauthenticated ⇒ 401. CSRF protection is `n/a` (there is no
cookie-authenticated mutating request to protect — brief 01's silent cookie channel is
dropped). A test asserts the handler set constructs no `http.Cookie` (criterion 9).

### Multipart upload

`POST /v1/uploads` parses a multipart body, **streams** the file part to the phase-11 workspace
provisioning + dataset-registration service (never buffering the whole file in memory), bounded
by the workspace upload ceiling (default 100 MiB / 1M rows, §14). An oversized part ⇒ 413; a
non-multipart body ⇒ 415; a malformed/typed parse error from the core service ⇒ 422. The
registered dataset is queryable through the identical grants + adapter + execution path as any
warehouse table (D-024) — no parallel channel (criterion 10).

### Operator-scoped `/metrics` + health

`/healthz` (liveness) and `/readyz` (store reachable + migrations current + JWKS fresh in
external mode + embedding pin validated, §15) are unauthenticated. `/metrics` is mounted at
root but **operator-scoped**: it requires a bearer token carrying `admin` — an unauthenticated
scrape ⇒ 401 (it is *not* the bare phase-01 handler exposed publicly). Data-source reachability
is reported as source *status*, never a `/readyz` gate (§15). This phase consumes phase-01's
telemetry conformance test (every registered counter appears on `/metrics`); RED-per-route
counters register at mount (criterion 12).

### Thin-surface architecture test (P7)

`internal/api` may import the core service interfaces, `internal/identity`,
`internal/access`, `internal/telemetry`, and stdlib. A test asserts it does **not** import
`internal/store`, `internal/gateway`, `internal/exec` internals, or any provider SDK; that it
assembles no SQL string; and that each handler body is the `decode → core-service → encode`
shape (no branching business logic). A capability's side effects (validation, audit, events)
live in the core, so the surface cannot omit them (criterion 13).

### Ask/BYO parity skeleton vs MCP

A shared **capability registry** enumerates the Ask tier (`preflight`, `plan`, `run`, `refine`)
and BYO tier (`context`, `submit`) capabilities, each bound to one core-service method. The
HTTP route table and (phase 22) the MCP tool set both register against it. A parity test
asserts every Ask/BYO capability has an HTTP route calling the same core method the MCP tool
will; the MCP column is a **pending** row phase 22 fills, and phase 23 closes the full
three-surface suite (criterion 14). This makes a future surface gap a mechanical failure (P7).

### Property alignment

- **P1a** — resolver run once; every core call takes the scope from `EffectiveAccess`; empty
  set short-circuits (list ⇒ 200 `[]`, single ⇒ 404); denied resource ⇒ 404 existence-hiding.
- **P1b/P1c** — the HTTP surface adds no SQL assembly and no execution logic; `:run`/`:submit`
  go through the identical validate→execute core (D-021/D-022); writes reach only the pipeline
  materializer path (D-017), never a query endpoint.
- **P2** — identity/access from the validated claim into the frozen envelope; no `X-*` trust;
  HTTP `aud` distinct from MCP `aud`.
- **P3** — tenant predicate rides every core call; tenant-mismatch is a typed 400; the
  cross-tenant registry is mechanical.
- **P4** — every denial/failure is a typed error + a metric + a content-free audit/log; no
  silent empty result reads as "nothing found" (`access.none` is typed).
- **P5** — no provider SDK in `internal/api`.
- **P6** — route paths + field names + error codes + audit actions are domain terms only;
  `:recheck-source`/`:revalidate` replace "repair"; enforced by `make drift-audit`.
- **P7** — one route table, one capability registry, thin handlers over one core each.

## Config keys added

Domain `server` (RFC §14). Documented here, in the example config, and smoke-checked. The
`server` domain schema is introduced by this phase (the HTTP server is constructed here);
`auth.audiences.http`, `exec.*`, and `workspace.upload_*` are consumed from their owning phases,
not redefined.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `server.http.addr` | string | `:8080` | no | HTTP listen address. |
| `server.http.read_header_timeout` | duration | `5s` | no | Explicit; never the SDK default. |
| `server.http.read_timeout` | duration | `15s` | no | Full request read deadline. |
| `server.http.write_timeout` | duration | `90s` | no | Must exceed `exec.statement_timeout` (60s) so a legitimate `:run` completes. |
| `server.http.idle_timeout` | duration | `60s` | no | Keep-alive idle deadline. |
| `server.http.max_header_bytes` | int | `1048576` (1 MiB) | no | Explicit header cap. |
| `server.http.max_body_bytes` | int | `10485760` (10 MiB) | no | `MaxBytesReader` cap for JSON routes; `/v1/uploads` uses the workspace ceiling. |
| `server.http.cors.allowed_origins` | `[]string` | `[]` (same-origin only) | no | Explicit Origin allowlist; empty = no cross-origin. |
| `server.http.shutdown_grace` | duration | `20s` | no | Graceful-drain window. |
| `server.http.metrics_scope` | string | `admin` | no | Capability scope gating `/metrics` (operator-scoped, §15). |

## Acceptance criteria

1. **Route-registry completeness (mechanical).** Every route mounted on the router is present
   in the single `[]Route` descriptor table and vice versa; a mounted-but-untabled (or
   tabled-but-unmounted) route fails a structural test. The table is the sole source the audit,
   cross-tenant, and parity consumers iterate.
2. **Mechanical audit coverage from the route table.** A test iterates the descriptor table and
   asserts every audited/tenant-sensitive mutating route carries a non-empty `AuditAction` and
   that a request to it emits a content-free `AuditEvent` (actor, resource, decision,
   request-id) — no opt-in gap, no body/SQL bytes.
3. **Mechanical cross-tenant adversarial suite.** The registry derived from the descriptor
   table probes every tenant-sensitive route with a tenant-A token against a tenant-B resource;
   only 400/401/403/404 are accepted, a bare 2xx fails.
4. **Tenant-mismatch typed 400.** A request whose path/param tenant disagrees with the token
   tenant returns a typed `tenant.mismatch` 400 (never silent precedence selection).
5. **Denied-resource-as-404 (existence-hiding).** A read/mutation on a topic/dataset/source the
   caller has no grant on returns a 404 with a typed not-found body identical to a genuinely
   absent resource — never a 403 that reveals existence.
6. **Missing capability scope → 403.** A valid token lacking the route's declared scope is
   rejected `scope.denied` 403 by the scope gate before the handler runs (distinct from 5).
7. **Typed error mapping golden (§9.5 → HTTP).** A golden table maps each validation code
   (`statement.blocked`, `table.not_in_topic`, `table.not_granted`, `column.unknown`,
   `join.unreachable`, multi-statement, parse) → 422, and `access.none`/`tenant.mismatch`/
   transport codes to their statuses; every error body is content-free (no internal id, URL,
   SQL, credential, model detail, or stack trace).
8. **Transport hardening explicit.** The `http.Server` is built with explicit
   read/read-header/write/idle timeouts, `MaxHeaderBytes`, and a `MaxBytesReader` body cap; an
   over-limit body ⇒ 413, a wrong `Content-Type` on a write ⇒ 415, a disallowed `Origin` ⇒ 403;
   a test asserts no field is left at the Go/SDK default.
9. **Bearer-only, no cookie transport.** No route reads or sets a cookie; a cookie-only auth
   attempt ⇒ 401; a test asserts the handler set constructs no `http.Cookie`; CSRF middleware is
   absent by design.
10. **Multipart upload.** `POST /v1/uploads` accepts a multipart file within the workspace
    ceiling and streams it to the workspace service (no full-file buffering); an oversized part
    ⇒ 413, a non-multipart body ⇒ 415; the resulting dataset is queryable through the standard
    grants + adapter path (integration).
11. **Middleware order + resolve-once.** The chain is request-id → auth → envelope →
    access-resolve → scope-gate → audit → handler; a test asserts the order and that
    `EffectiveAccess` is resolved exactly once into the frozen envelope (not per handler).
12. **`/metrics` operator-scoped; health unauthenticated.** `GET /metrics` requires the
    operator scope (unauthenticated scrape ⇒ 401); `/healthz`/`/readyz` stay unauthenticated;
    every registered metric appears (phase-01 telemetry conformance consumed).
13. **Thin-surface architecture test.** `internal/api` imports no provider SDK, does not import
    `internal/store`/`internal/gateway`/`exec` internals, assembles no SQL, and each handler is
    decode→core-service→encode (no business logic).
14. **Ask/BYO parity skeleton vs MCP.** A shared capability registry enumerates Ask
    (preflight/plan/run/refine) + BYO (context/submit); a parity test asserts each has an HTTP
    route bound to the same core-service method the MCP tool will call, with the MCP column
    pending phase 22.
15. **One endpoint per group (integration smoke).** With a real token against a fresh Docker
    Postgres, one representative endpoint per §11.2 group returns its typed normalized shape;
    SKIPs cleanly when the test DSN is unset.

## Test obligations

Per CLAUDE.md §11:

- **Unit:** route-registry completeness (1); audit-coverage table walk (2); tenant-mismatch
  400 (4); scope-gate 403 (6); error-mapping golden (7); transport-hardening field assertions +
  413/415/403 (8); no-cookie assertion (9); middleware-order + resolve-once (11); metrics
  operator-scope + health-open (12); thin-surface import/no-SQL architecture test (13); parity
  registry enumeration (14). Table-driven where it fits; the error mapping is golden.
- **Integration:** **required** — Deps name many shipped subsystems and this phase closes the
  HTTP half of their surface seam (§17). Against a **real Docker Postgres** with a **real token**
  (no boundary mocks except the gateway `mock` for NLQ paths, paired with a recorded fixture):
  one endpoint per group (15), the multipart upload → queryable dataset round-trip (10), and
  identity/scope propagation through a `:run` end to end. Under `-race`; SKIP on unset DSN.
- **Adversarial:** **required** (this is an ACL/auth surface). The registry-driven cross-tenant
  probe over every tenant-sensitive route (3); empty-access-set short-circuit (list ⇒ 200 `[]`,
  single ⇒ 404, no query issued — store call-count assertion); forged-`X-*`-header attempt has
  no effect (identity is the envelope only); fetch-then-filter regression guard on list
  endpoints; denied-resource-404 (5). Once the query endpoints wire exec, the injection /
  schema-escape / write-smuggling corpus rides the eval red-team suite (phase 24) but is
  exercised here through `:submit-sql` returning 422.
- **Fuzz:** `FuzzDecodeRequest` over the JSON/multipart decode surface — invariant: never
  panics, never mutates the frozen envelope, always a typed error on malformed input (never a
  partial decode). (JWT parsing fuzz lives in phase 03.)
- **Bench:** `BenchmarkMiddlewareChain` — the per-request chain (request-id → … → scope-gate)
  is on every request's hot path; baseline, not a CI gate. Concurrent-reuse: the router +
  middleware stack are shared reusable artifacts ⇒ a `-race` concurrent-request test.

## Coverage targets

`internal/api` is a new `internal/` package ⇒ **80%** (convention 4 default). Although it
carries auth/access-adjacent gating, the *access logic* lives in phase-04 (`internal/access`,
85%); this package is the thin surface over it, so the 80% new-package band applies. The
implementing PR adds the entry to `scripts/coverage-bands.conf`.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/api` | 80% | New `internal/` package; the access logic it gates is covered at 85% in `internal/access` (phase 04) — this is the thin surface (CLAUDE.md §11 default). |

## Smoke checks

Each criterion maps to a named Go test invoked by `scripts/smoke/phase-21.sh` (via
`run_group`); the script SKIPs entirely until `internal/api` exists, and the integration
criteria (10, 15) SKIP when the Docker-Postgres DSN is unset.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestRouteRegistryComplete` passes (mounted ⇔ tabled). |
| 2 | `TestAuditCoverageFromRouteTable` passes (every audited route iterated; content-free event). |
| 3 | `TestCrossTenantRegistryProbe` passes (only 400/401/403/404; 2xx fails). |
| 4 | `TestTenantMismatch400` passes (typed `tenant.mismatch`). |
| 5 | `TestDeniedResourceReads404` passes (existence-hiding; identical to absent). |
| 6 | `TestMissingScope403` passes (scope gate before handler). |
| 7 | `TestErrorMappingGolden` passes (§9.5 → HTTP; content-free body). |
| 8 | `TestTransportHardening` passes (explicit fields; 413/415/403). |
| 9 | `TestNoCookieTransport` passes (no cookie read/set; cookie-only ⇒ 401). |
| 10 | `TestUploadMultipart` passes under `-race` on Docker Postgres (SKIP if DSN unset). |
| 11 | `TestMiddlewareOrder` passes (order + resolve-once into frozen envelope). |
| 12 | `TestMetricsOperatorScoped` passes (scrape ⇒ 401; health open; metrics exported). |
| 13 | `TestApiThinSurface` passes (import boundary + no SQL + decode/call/encode). |
| 14 | `TestAskTierParitySkeleton` passes (HTTP ⇔ shared capability registry; MCP pending). |
| 15 | `TestHTTPEndpointPerGroup` passes under `-race` on Docker Postgres (SKIP if DSN unset). |

## Glossary additions

New terms, pre-written for `docs/glossary.md` (same PR, CLAUDE.md §14). Terms already defined
(scope-debug, grant, principal, capability scope, frozen per-request envelope, chart spec,
context bundle, self-issue/external-issuer, re-check source/revalidate) are **not** duplicated.

- **HTTP surface** — the `/v1` REST surface Chartworks presents from the one binary, for the
  Console and standalone API customers (D-012); bearer-JWT authenticated with the HTTP `aud`
  distinct from the MCP `aud` (RFC §11.2). Management-plane operations are HTTP-only in V1.
- **Route registry** — the single `[]Route` descriptor table (method, path, scope, audit
  action, tenant-sensitivity) that is the sole source of truth for router mounting and for the
  *mechanically derived* audit-coverage, cross-tenant-probe, and parity checks (RFC §5.5, §15).
- **Existence-hiding** — the discipline of returning `404` (not `403`) for a resource the
  caller has no grant on, identical to a genuinely-absent resource, so the surface never
  confirms a resource exists to a caller who cannot see it (brief 04 keeper).
- **Operator-scoped metrics** — the `GET /metrics` Prometheus mount gated by the operator
  (`admin`) scope — never unauthenticated, distinct from the open `/healthz`/`/readyz` (§15).
- **Typed error envelope (HTTP)** — the content-free `{ "error": { "code", "message" } }`
  body every failure returns; the code is the §9.5 vocabulary verbatim for validation
  failures, and the body never carries an internal id, URL, credential, SQL, or stack trace.

## Decisions filed

No new `D-NNN` entries. This phase implements existing decisions:

- **D-006** — dual-audience asymmetric bearer JWT; the HTTP `aud` is validated distinct from
  the MCP `aud`; no `X-*` identity trust.
- **D-012** — the HTTP surface serves the Console + standalone API-customer consumer classes.
- **D-017 / D-021** — the read/write posture split and the validation error vocabulary the
  error mapper renders to HTTP (query endpoints read-only; writes only via pipelines).
- **D-019** — the MCP-parity contract the Ask/BYO parity skeleton anchors.
- **D-020** — the grants + capability-scope model the scope gate and resource checks consume.
- **D-024** — uploads are first-class via the workspace, reached through the standard path.
- **D-030** — no local user management: no login/signup/whoami/header-mint routes; admin =
  tenants (self-issue mode) + API keys + scope-debug + audit.

### Ambiguities resolved (recorded for the wave-end audit)

- **Health/metrics path placement.** RFC §11.2 heads "all under `/v1`" but lists `/healthz`,
  `/readyz` bare and `/metrics` under the Admin group. Resolved: `/healthz` + `/readyz` at
  **root, unauthenticated** (CLAUDE.md §7's only exceptions); `/metrics` at **root,
  operator-scoped**; everything else under `/v1`. This is the minimal reading consistent with
  "no unauthenticated endpoints except healthz/readyz."
- **Dataset/refresh-profile + upload scope.** No `dataset.manage` scope exists in §5.3; DE-stage
  dataset mutations (`:refresh-profile`, `/uploads`) gate on `source.manage` scope + the
  dataset/source-grain `manage` grant.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3): each reasonable deviation, why, and
     confirmation this file was updated in the same PR. -->

- none yet.
