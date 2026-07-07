# Phase 02 — `store-migrations`

> **Status:** draft
> **Owner:** orchestrator
> **Depends on:** phase-01-binary-config-telemetry

Authored per CLAUDE.md §16. Wave 1. Difficulty: medium.

---

## RFC / request sections

- **RFC-001 §12** — the budgeted 25-table Store & schema inventory (binding: a table or
  column beyond it needs an RFC amendment first; `tenant_id TEXT NOT NULL CHECK
  (tenant_id <> '')` on every tenant row; forward-only migrations).
- **RFC-001 §3.2** — `internal/store` owns "Chartworks' own state — seam + `postgres`
  driver (pgx/v5), migrations, conformance suite."
- **RFC-001 §3.3** — boot is fail-loud: store reachability + migrations check gate
  readiness; the one generic leased `jobs` queue lives in this schema (its table).
- **RFC-001 §5.1–5.2** — the `grants` relation shape and the "every store read/write
  method takes non-optional scope parameters; a scope-less query method is rejected in
  review" rule this phase makes structural.
- **RFC-001 §14** — the `store` config domain (Postgres DSN via `env:`, pool sizing,
  migration policy).
- **Decisions:** D-004 (postgres-only own store, SQLite out of scope, store-vs-warehouse
  boundary), D-020 (grants: one relation, three grains, scope parameters), D-025 (one
  generic leased `jobs` table shape), D-029 (`facet_vectors` table belongs to the schema
  but the vector seam/driver is phase 07's `vindex`), D-016/D-017 (source-credential and
  write-posture columns the schema reserves).

## Depends on

- **phase-01-binary-config-telemetry** — this phase adds a `store` config block to the
  typed config surface phase 01 owns, registers store metrics on phase 01's Prometheus
  registry, and logs through phase 01's slog handler. It needs phase 01's fail-loud
  config validator harness and `env:` indirection to exist.

No other phase is required: this is the foundational persistence phase every Wave-2+
phase builds on. It consumes no other subsystem's surface.

## Informing briefs

Per `docs/research/INDEX.md` (`internal/store` → primary **brief 02**, secondary
**brief 05**, 01):

- **brief 02** (`docs/research/02-predecessor-data-and-execution.md`) — the predecessors'
  own ~50-table metadata store, the generic leased `jobs` table shape, the
  store-vs-warehouse split, and the sprawl scar.
- **brief 05** (`docs/research/05-predecessor-diff.md`) — the client-vs-fork access-model
  diff, the `tenant_id NOT NULL` hardening, and the **fork's nullable-tenant regression**
  (the named counterexample this phase must not inherit).

## Brief findings incorporated

- **Inescapable `tenant_id NOT NULL` predicate everywhere** (brief 05 §"Divergence — Access
  model", carry-table row "Tenant isolation"; brief 05 lines 421–434, 455). The client
  predecessor's hardened schema carried `tenant_id NOT NULL` on topics/queries/schedules
  with tenant-scoped indexes; the fork **relaxed it to nullable in places** — an explicit
  P3 regression. This phase restores `tenant_id TEXT NOT NULL CHECK (tenant_id <> '')` on
  **every** tenant-owning table and makes a blank tenant a constraint violation, not a
  code-path convention (criteria 5).
- **Budget the schema; store each identifier once** (brief 02 §"Own data model",
  §"Scars"; lines 51–58, 350–355). The ~50-table sprawl with query/SQL/topic ids
  duplicated across `nlq_queries`/`sql_generations`/`sql_generation_examples`/
  `nlq_inference_cache` is the direct referent for CLAUDE.md §6's "Store schema is
  budgeted." This phase ships **exactly** RFC §12's 25 tables — no more — and asserts the
  inventory mechanically (criterion 4).
- **One generic leased `jobs` table, not per-feature job tables** (brief 02 §"Background
  jobs", §"Keepers"; lines 67–72, 261–273, 323–326). The predecessors' single `jobs`
  queue (`FOR UPDATE SKIP LOCKED`, `lease_owner`/`lease_expires_at`, heartbeat, reclaim)
  is a keeper; the 13 bespoke worker classes are the P7 counterexample. The schema ships
  one `jobs` table with the lease columns (D-025); the queue *logic* is phase 06.
- **Store-vs-warehouse boundary is a schema fact** (brief 02 §Summary, §"Keepers"; D-004).
  This schema holds only Chartworks' own state. Customer-warehouse credentials live here
  as an opaque `secret_ciphertext` column on `data_sources` (custody logic is phase 08);
  uploaded rows and workspace tables never enter this store (§7.4/D-024).
- **`WarehouseConnectionRecord` has no secret field** (brief 02 §"Source connectivity",
  lines 98–113). The schema keeps the secret in a single `secret_ciphertext` column so the
  read/list *shape* phase 08 builds over it can structurally omit the secret. Phase 02
  only lands the column; it never returns or logs it.

## Findings I'm departing from

- **The predecessors' dev-time `sqlite` driver** (brief 02 §"Own data model", lines 28–31:
  "a `sqlite`-for-dev / `postgres`-for-production driver split"). Not carried — D-004 puts
  SQLite explicitly out of scope. V1 ships exactly one driver (`postgres`, pgx/v5); the
  seam exists for a *future* backend, and local dev/CI use Docker Postgres (`make pg-up`).
  No in-memory shortcut backs the conformance suite (CLAUDE.md §9; criterion 9's fresh-DB
  harness).
- **The fork's nullable-tenant relaxation** (brief 05 lines 432–434). Deliberately not
  inherited — it is the named P3 counterexample; see criterion 5.
- **The predecessors' per-stage TTL cache tables** (brief 02 §"Result shaping", lines
  243–255). Not schema-modeled here beyond the RFC §12 `idempotency_cache`; caching
  strategy is a later-phase / RFC-open concern, not Store schema this phase budgets.

## Scope

`internal/store` — the whole package, delivered as:

1. **The Store seam — narrow per-domain interfaces, no god-interface.** One small
   interface per domain group (≈17), each method carrying a non-optional identity/scope
   parameter. An aggregate `store.Store` composes them by *embedding* (a construction and
   transaction convenience), never as a flat all-methods interface.
2. **The `postgres` driver** (pgx/v5, `CGO_ENABLED=0`) implementing every interface,
   pool-configured, tenant-predicate-inside-the-query on every read/write.
3. **The forward-only migration runner** — embedded `.sql` migrations (`//go:embed`),
   ordered versions, a `schema_migrations` ledger, checksum integrity, an advisory-lock
   guard against concurrent double-apply, and a `migrations check` that boot uses to gate
   readiness (§3.3).
4. **Migrations for the complete budgeted 25-table inventory** (RFC §12), including the
   `vector` extension enablement and the `facet_vectors` table (its *seam/driver* is phase
   07's `vindex`, D-029 — this phase lands only the DDL).
5. **The store conformance suite** — table-driven, run against a fresh Docker Postgres,
   under `-race`, proving scope-parameterization, tenant isolation, and the constraint
   invariants for every per-domain interface.
6. **The `store` config block** (§14) with a fail-loud validator.
7. **The fresh-DB test harness** (convention 8) — every conformance/integration run
   provisions an empty database and migrates it to head, so no pre-migrated dev DB masks a
   fail-loud guard.

Per-domain interfaces (one per group; method depth is baseline CRUD each domain needs now,
and grows by later phases *adding a method to the seam + conformance coverage*, per
CLAUDE.md §9):

| Interface | Table(s) |
|---|---|
| `TenantStore` | `tenants` |
| `APIKeyStore` | `api_keys` |
| `SourceStore` | `data_sources` |
| `DatasetStore` | `datasets`, `dataset_profiles` |
| `PipelineStore` | `pipelines`, `pipeline_runs` |
| `TopicStore` | `topics`, `topic_versions`, `topic_audit` |
| `RuleStore` | `rules` |
| `GrantStore` | `grants` |
| `SessionStore` | `sessions` |
| `QueryStore` | `queries`, `saved_queries` |
| `ExampleStore` | `examples` |
| `FeedbackStore` | `feedback_events` |
| `JobStore` | `jobs` |
| `ScheduleStore` | `schedules`, `schedule_runs` |
| `IdempotencyStore` | `idempotency_cache` |
| `AuditStore` | `audit_events` |
| `GatewayCallStore` | `gateway_call_events` |

`facet_vectors` (table lands here) → **no** store interface; owned by `internal/vindex`
(phase 07). `schema_migrations` → internal to the runner, not a domain interface.

## Non-goals

- **No domain logic.** The access *resolver* (phase 04), the jobs *queue mechanics* (phase
  06), the vindex *vector search* (phase 07), credential *custody* (phase 08), topic
  *lifecycle* (phase 15) all build on this schema — none ships here. This phase ships the
  tables, the seam, and baseline scope-parameterized CRUD only.
- **No customer-warehouse access.** The store-vs-warehouse boundary (D-004) holds: no
  data-source adapter, no warehouse query, no upload-workspace provisioning.
- **No secret encryption/decryption.** `data_sources.secret_ciphertext` is stored and
  read as opaque bytes; the AES-256-GCM custody + key ring is phase 08.
- **No second driver.** SQLite/embedded is out of scope (D-004).
- **No down/rollback migrations.** Forward-only (§12); never edit a merged migration
  (CLAUDE.md §9).

## Design

**Seam shape.** Each per-domain interface is small and scoped; the first parameter of
every method that does I/O is `context.Context`, and every read/write carries an explicit
scope value derived from the frozen identity envelope (phase 03 supplies the type; phase 02
depends only on a minimal `store.Scope` it defines — `{TenantID string; Principal string}`
— which the envelope satisfies). Pseudocode (signatures, not implementations):

```go
// store/store.go — the aggregate is composition, never a flat god-interface.
type Store interface {
    Tenants() TenantStore
    Sources() SourceStore
    Datasets() DatasetStore
    // …one accessor per domain…
    // Tx runs fn in a single pgx transaction; the same per-domain
    // interfaces are reachable inside it.
    Tx(ctx context.Context, fn func(Store) error) error
    Close()
}

// Scope is the non-optional access boundary every query carries. A method
// without it cannot be expressed — there is no unscoped read/write API (D-020, P1/P3).
type Scope struct {
    TenantID  string // never "" — the CHECK backstops a caller that tries
    Principal string // "user:…" | "agent:…" | "svc:…" | "key:…"
}

type DatasetStore interface {
    Create(ctx context.Context, s Scope, d Dataset) (Dataset, error)
    Get(ctx context.Context, s Scope, id string) (Dataset, error)
    List(ctx context.Context, s Scope, f DatasetFilter) ([]Dataset, error)
    Update(ctx context.Context, s Scope, d Dataset) (Dataset, error)
    // dataset_profiles reached through the same interface + scope:
    PutProfile(ctx context.Context, s Scope, p DatasetProfile) error
}
```

**Tenant predicate is inside the query, never fetch-then-filter (P1/P3).** Every generated
SQL statement in the postgres driver includes `WHERE tenant_id = $scope.TenantID` (and, for
child rows, the same predicate carried on the child table — see the tenant-column decision
below). There is no code path that reads then filters in Go. A `List` with an
empty-but-valid scope still issues a tenant-predicated query; an *absent* scope is
impossible because `Scope` is a required parameter and `TenantID == ""` is rejected before
the query is built (typed `store.ErrNoTenant`, P4) and, as defense in depth, by the column
`CHECK`.

**Tenant column on child/derived tables (ambiguity resolved).** RFC §12's preamble mandates
`tenant_id NOT NULL CHECK` on **every tenant row** and "part of every query predicate," but
its representative key-column list does not spell out `tenant_id` for the five child tables
(`topic_versions`, `topic_audit`, `dataset_profiles`, `pipeline_runs`, `schedule_runs`).
Resolution: these tables **carry a denormalized `tenant_id TEXT NOT NULL CHECK` too**, so
every query has a *direct* tenant predicate rather than relying on a join a code path could
forget (P3: "an inescapable tenant predicate, not by a filter a code path could forget").
This is an application of §12's preamble, not a table/column beyond the budget, so it needs
no RFC amendment; flagged here for orchestrator ratification. The only table without
`tenant_id` is `schema_migrations` (pure infrastructure).

**Migration runner.**

```go
// store/migrate — forward-only, embedded, advisory-locked.
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Apply advances the DB to head. Idempotent: already-applied versions are
// skipped. Guarded by pg_advisory_lock so two concurrent boots never double-apply.
func (r *Runner) Apply(ctx context.Context) (applied []Version, err error)

// Check reports pending migrations without applying — boot uses this to gate
// readiness under policy=validate (§3.3). Also verifies each recorded version's
// checksum still matches its embedded file; a mismatch is ErrMigrationTampered
// (forward-only integrity, never a silent re-run).
func (r *Runner) Check(ctx context.Context) (pending []Version, err error)
```

`schema_migrations(version BIGINT PK, name TEXT, checksum TEXT, applied_at TIMESTAMPTZ)`.
Versions are zero-padded, lexicographically ordered filenames (`0001_init.sql`, …). No
`Down`. The runner never edits an applied row (CLAUDE.md §9).

**Conformance suite.** One table-driven suite parameterized over the per-domain interfaces,
exercising: create→get→list→update round-trips under a scope; a cross-tenant probe (write
under tenant A, read under tenant B → nothing); a blank-tenant write → typed error; and the
constraint/inventory introspection checks. It provisions a **fresh** database per run
(convention 8) via `make pg-up` + `Apply`, and runs under `-race`. It is the seam's
contract, so a future backend must pass it unchanged.

**How it upholds P1–P7:**
- **P1/P3** — deny-by-default & tenant isolation: scope is a required parameter, the
  predicate is inside the query, `tenant_id NOT NULL CHECK` is the column-level backstop,
  and cross-tenant reads return nothing (criteria 5–8).
- **P4** — fail loud: missing DSN refuses boot; a pending-but-unapplied migration under
  `validate` fails readiness; a tampered migration is a typed error; a blank tenant is a
  typed error, never a silent write.
- **P6** — no plumbing vocabulary escapes: this package is internal; interface/table names
  are internal identifiers, none reach a wire/UI/error string a user reads.
- **P7** — one persistence primitive: one seam, one driver, one migration ledger; the
  aggregate is composition, not a parallel path.

## Config keys added

Under the `store` domain (RFC §14). Documented here, in the phase-01 example config (added
in this phase's implementation PR per CLAUDE.md §4.2), and smoke-checked.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `store.dsn` | string (`env:`) | — | yes | Postgres DSN via `env:` indirection (e.g. `env: CHARTWORKS_STORE_DSN`); unset ⇒ refused boot (P4). Never logged. |
| `store.max_conns` | int | `10` | no | pgxpool max connections. |
| `store.min_conns` | int | `2` | no | pgxpool min connections (kept warm). |
| `store.connect_timeout` | duration | `10s` | no | Fail-loud dial timeout at boot. |
| `store.migrations.policy` | enum | `validate` | no | `validate` (boot fails if not at head — prod), `apply` (apply pending on boot — dev/CI), `off` (skip the check). Unknown value rejected by the validator. |

Unknown keys under `store` are rejected (phase 01's validator harness). Secrets ride only
`env:`; no plaintext credential in the config file.

## Acceptance criteria

Numbered, mechanically checkable; each maps to a smoke assertion below.

1. **Fresh-DB apply.** The runner applies every migration from an empty database to head;
   `schema_migrations` then lists each version exactly once, in ascending order.
2. **Idempotent re-apply.** Running `Apply` again against a DB already at head applies zero
   migrations and returns no error.
3. **Forward-only integrity.** `Check` returns a typed error (`ErrMigrationTampered`) if a
   recorded version's checksum no longer matches its embedded file; the public runner API
   exposes no down/rollback method (compile-time — the type has no `Down`/`Rollback`).
4. **Complete, exact inventory.** After migrating to head, exactly the 25 RFC §12 tables
   exist (plus `schema_migrations`) — none missing, none extra (introspection over
   `information_schema.tables`).
5. **Inescapable tenant predicate.** Every table except `schema_migrations` has
   `tenant_id` typed `text`, `NOT NULL`, with a `CHECK (tenant_id <> '')`; an insert with
   `tenant_id = ''` is rejected by the constraint (typed error surfaced, not a silent
   write).
6. **Scope-parameterized methods (no scope-less query).** An architecture test over every
   exported method of every per-domain interface asserts each carries a `store.Scope` (or
   equivalent tenant-bearing) parameter; a method without one fails the test.
7. **No god-interface.** A structural test asserts the aggregate `Store` is defined by
   embedding per-domain accessors and that no single interface flattens all domains'
   methods (each per-domain interface stays within its table group).
8. **Cross-tenant isolation at the store layer.** For every per-domain interface, a row
   written under tenant A is not returned by an identical read/list scoped to tenant B.
9. **Conformance green, fresh DB, `-race`.** The full conformance suite passes under
   `-race` against a freshly-provisioned, migrated Docker Postgres (no pre-migrated DB).
10. **Concurrent-migration safety.** Two runners applying concurrently against the same
    database do not double-apply (advisory-lock guarded): exactly one applies each pending
    version, the other no-ops, neither errors.
11. **Config fail-loud.** An unset `store.dsn` refuses boot with a typed config error, and
    an unknown key under `store` is rejected by the validator; the example config parses
    with a valid `store` block.
12. **Coverage.** `internal/store` (seam + postgres driver + migration runner) meets the
    85% band via `make coverage`.

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven per-domain CRUD; migration-version ordering/checksum logic; the
  `store.Scope` validation (blank tenant → typed error); the architecture tests for
  criteria 6–7 (reflection/AST over the interface set).
- **Integration:** **required** — this phase introduces the public interface every Wave-2+
  phase builds on and closes the store seam (§17). The conformance suite runs with **real
  drivers** against a fresh Docker Postgres (`make pg-up`), proves tenant-scope propagation
  end to end, covers ≥1 failure mode (blank tenant, tampered migration), and runs under
  `-race`. No boundary mock (the gateway `mock` is the only sanctioned one, not applicable
  here). Lives in-package — `internal/store` *is* the wiring boundary.
- **Adversarial:** **required** — this is a P1/P3 (tenant-isolation) path. The cross-tenant
  probe (criterion 8), the empty/blank-tenant probe (criterion 5), and a fetch-then-filter
  regression guard (assert the generated SQL carries the tenant predicate; no Go-side
  post-filter) are standing obligations here. Forged-header/JWT probes are phase 03/04
  (no header path exists in this package). The mechanically-derived route/tool registry
  probe is phase 04 — this phase's adversarial set is store-layer.
- **Fuzz:** n/a — this package parses no external/attacker-controlled payload (no JWT, no
  NLQ, no SQL-to-validate); inputs are typed Go values. Migration SQL is trusted, embedded,
  first-party. (Fuzz obligations land on phases 03/09.)
- **Bench:** **`BenchmarkStoreHotPath`** on the hottest reusable read (a scoped `Get`/`List`
  round-trip) — the driver is a shared, concurrently-reused artifact; a baseline benchmark
  is carried (`make bench`, not a CI gate). Plus the mandatory `-race` concurrent-reuse
  test on the shared `*postgres.Store` (convention 5).

## Coverage targets

Per CLAUDE.md §11 (85% for the `store` driver + conformance-tested subsystems). Entry added
to `scripts/coverage-bands.conf` in this phase's implementation PR (not this planning PR):

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/store` | 85% | Store driver + conformance-tested subsystem (CLAUDE.md §11 / convention 4). |

## Smoke checks

Each criterion maps to one assertion in `scripts/smoke/phase-02.sh` (backed by a named Go
test via `run_group`; the script SKIPs cleanly until `internal/store` exists).

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 Fresh-DB apply | `TestMigrateFreshToHead` PASS |
| 2 Idempotent re-apply | `TestMigrateIdempotent` PASS |
| 3 Forward-only integrity | `TestMigrateForwardOnlyGuard` PASS |
| 4 Complete, exact inventory | `TestSchemaInventoryMatchesRFC` PASS |
| 5 Inescapable tenant predicate | `TestTenantIdNotNullCheckEverywhere` + `TestBlankTenantRejected` PASS |
| 6 Scope-parameterized methods | `TestNoScopelessQueryMethod` PASS |
| 7 No god-interface | `TestPerDomainInterfacesNoGodInterface` PASS |
| 8 Cross-tenant isolation | `TestCrossTenantProbeReturnsNothing` PASS |
| 9 Conformance green, fresh DB, `-race` | `TestStoreConformance` PASS (run under `-race`) |
| 10 Concurrent-migration safety | `TestMigrateConcurrentAdvisoryLock` PASS |
| 11 Config fail-loud | `TestStoreConfigValidation` PASS |
| 12 Coverage (85%) | n/a in smoke — enforced by `make coverage` band gate |

## Glossary additions

The only genuinely-new terms are internal seam vocabulary (P6: they stay internal, never
reach a wire/UI/error string). Pre-written for `docs/glossary.md` → "Internals & seams",
landed in this phase's implementation PR:

- **Migration runner (forward-only)** — the embedded-`.sql`, advisory-locked applier that
  advances the `store` schema to head and records each version once in `schema_migrations`;
  never a down/rollback; a merged migration is never edited (CLAUDE.md §9).
- **Store conformance suite** — the table-driven suite, run against a fresh Docker Postgres
  under `-race`, that is the `store` seam's contract: scope-parameterization, tenant
  isolation, and the constraint/inventory invariants a future backend must pass unchanged.

(`store seam`, `data-source adapter`, `frozen per-request envelope` already exist in the
glossary — not re-added.)

## Decisions filed

No new `D-NNN` is filed in this planning PR. This phase relies on and implements existing
decisions: **D-004** (postgres-only own store, SQLite out of scope, store-vs-warehouse
boundary), **D-020** (grants relation + non-optional scope parameters), **D-025** (one
generic leased `jobs` table shape), **D-029** (`facet_vectors` table here, vector seam in
phase 07), **D-016/D-017** (the `secret_ciphertext` and `writable_destinations` columns the
schema reserves for phases 08/13). The **child-table `tenant_id` denormalization** (Design
§) is recorded as an application of RFC §12's preamble; if the orchestrator deems it a
standalone architectural decision, it should be filed as the next `D-NNN` in the
implementation PR rather than left implicit.

## Deviation log

_(Filled during implementation, not at authoring time — CLAUDE.md §4.3.)_
