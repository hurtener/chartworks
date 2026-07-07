# Phase 08 — `sources-core`

> **Status:** draft
> **Owner:** orchestrator
> **Depends on:** phase-02-store-migrations, phase-04-access-grants

Wave 3. Difficulty: high. Owns `internal/sources`: the data-source adapter seam,
the connections registry, credential custody, the `postgres` + `null` + `mock`
drivers, and schema discovery with dialect-agnostic type classification.

---

## RFC / request sections

- **RFC-001 §6** — data sources & credentials: §6.1 the adapter seam, §6.2 the
  connections registry, §6.3 credential custody (settles D-016).
- **RFC-001 §7.1** — registration & discovery (`TypeCategory` classification,
  selective registration, P1a "nothing queryable by default").
- **RFC-001 §3.2** — the `internal/sources` package boundary; **§12** — the
  `data_sources` and `datasets` table budget; **§17** — CGo-free pure-Go drivers.
- **RFC-001 §1.2 (P1c)** — the write split: `Query` carries no write operation;
  `Materializer` is a *separate* interface (declared here, implemented in phase 13).
- **RFC-001 §9.5–9.6 (P1b)** — `ValidatedSQL` is the only executable input to
  `Query`; the type itself lands in phase 09, execution wiring in phase 10.

## Depends on

- **phase-02 (`store-migrations`)** — `data_sources` and `datasets` rows are
  persisted through the `Store` seam; this phase reads/writes them with
  non-optional scope parameters. Closes the store↔registry seam phase 02 opened.
- **phase-04 (`access-grants`)** — every registry read/list and every discovery
  call takes an `access`-derived scope; `source`-grain grants gate visibility and
  `manage` gates credential mutation (RFC §5.1). This phase consumes the resolver;
  it never re-implements access.

`ValidatedSQL` (phase 09) and the read execution layer (phase 10) are **not** deps:
this phase declares the interface contract they satisfy. The seam ships with the
type referenced as an interface obligation; the concrete `exec.ValidatedSQL` and
its wiring arrive in 09/10 (master plan hard gate P1b, convention 9).

## Informing briefs

Per `docs/research/INDEX.md` (`internal/sources` → primary **02**, **05**;
secondary **04**, 10):

- `docs/research/02-predecessor-data-and-execution.md`
- `docs/research/05-predecessor-diff.md`
- `docs/research/04-predecessor-security-tenancy.md`

## Brief findings incorporated

- **Brief 02 — the fork's encrypted Connections registry.** The registry row
  carries `config_json` (host/port/database/user — never the secret) plus a
  *separately* encrypted `secret_ciphertext`; the **read/list shape has no secret
  field at all**, making leak-on-read structurally impossible (RFC §6.2). Adopted
  verbatim as a type-level obligation (criterion 1).
- **Brief 02 — `TypeCategory` computed once at discovery.** The predecessors'
  `money`-column bug came from re-deriving column category ad hoc at every read.
  Classification is computed **once** at `DiscoverSchema` into a normalized
  `TypeCategory` (numeric / temporal / boolean / text / structured / binary /
  unknown) and stored on the dataset (RFC §7.1, criterion 9).
- **Brief 02 — "nothing requires prior validation" scar.** The adapter's `Query`
  entry point takes an opaque `ValidatedSQL`, never a raw string, so an adapter
  *structurally cannot* execute an unvalidated statement (RFC §6.1, criterion 6).
- **Brief 02 — the `ansi` dialect sentinel + capability gating.** Feature support
  is queried through `Capabilities().Supports(x)`, never by type-switching on the
  concrete adapter, so an unknown construct degrades to a typed rejection rather
  than a silent pass (RFC §6.1, criterion 8).
- **Brief 05 — the `null` adapter keeper.** A fail-loud placeholder driver whose
  every operation returns a typed unavailable error (never nil/empty) — carried
  as criterion 5.
- **Brief 02 — MultiFernet rotation discipline, in Go stdlib crypto.** Encrypt
  always uses the primary (first) ring key; decrypt tries every ring key, so
  rotation is "prepend a new primary" with no bulk re-encryption (RFC §6.3,
  criteria 2–3).
- **Brief 04 — credentials never leak into logs/errors/results; content-free
  credential events.** Secrets decrypt only at adapter construction, live in
  memory only; `stored | rotated | test-failed` events audit ids + principals +
  decision, never secret or config bytes (RFC §6.3, §7 CLAUDE.md, criterion 10).
- **Brief 04 — fail-closed at boot.** Missing key ring with any stored secret
  present is a refused boot, never a silently generated key (P4; the auth-side
  ephemeral-key scar's data-source analogue, criterion 4).

## Findings I'm departing from

- **Brief 02's ~50-table registry sprawl** — not carried; the registry is the
  single budgeted `data_sources` table (RFC §12), source/dataset identifiers
  stored once each.
- **Brief 10 (Bruin) connector layer** — not adopted as an execution engine
  (D-023: CGo/Rust/Python collide with D-005). The V1 `postgres` driver is pgx/v5,
  pure-Go; the seam shape is mined, the engine is not.
- **A regex "injection heuristic" pre-filter on the adapter** — deliberately
  absent (brief 02/04): the AST allowlist (phase 09) is the injection guardrail;
  the adapter's contribution to P1b is the `ValidatedSQL`-only signature plus
  defense-in-depth read-only session mode (phase 10), not string scanning.

## Scope

Package **`internal/sources`**, delivering:

1. **The adapter seam** (interface + factory + driver, CLAUDE.md §4.4). The
   `Adapter` interface: `Kind()`, `Dialect()`, `Capabilities()`,
   `TestConnection(ctx)`, `DiscoverSchema(ctx, scope)`, `SampleValues(ctx, scope,
   …)`, `Query(ctx, ValidatedSQL, QueryOpts) → QueryResult`. Drivers register via
   `init()` blank-import through a factory keyed by `Kind`.
2. **The `Materializer` interface declared but not implemented here** — a separate
   write interface (P1c) that destination-capable drivers implement in phase 13.
   Declaring it now keeps `Query` write-free by construction.
3. **The connections registry** — `data_sources` rows (RFC §12) with the
   status lifecycle `unverified → connected → unavailable`, a **secret-free**
   read/list DTO, and `last_tested_at` / content-free `last_error`.
4. **Credential custody** — AES-256-GCM envelope encryption, a config-supplied
   master key ring (`CHARTWORKS_SOURCE_KEYS`), primary-encrypt / try-all-decrypt
   rotation, fail-closed boot, content-free credential events.
5. **Drivers** — `postgres` (pgx/v5, also serves upload workspaces per D-024),
   `null` (fail-loud placeholder), `mock` (test double).
6. **Schema discovery** — `DiscoverSchema` produces candidate datasets with the
   normalized `TypeCategory` computed once; **selective** registration (nothing
   queryable by default, P1a).
7. **The adapter conformance suite** — a driver-agnostic test kit (`TestConnection`,
   discovery, typing, scoping) other drivers (phase 14) run.

## Non-goals

- **Read execution** (read-only session mode, timeouts, row caps, result shaping)
  — phase 10. This phase provides the `Query` *signature* and the driver hook.
- **`ValidatedSQL`'s concrete type + AST validation** — phase 09.
- **Materialization writes** — phase 13 (the interface is only *declared* here).
- **Warehouse drivers** (`bigquery`/`snowflake`/`databricks`) — phase 14, behind
  this same seam and conformance suite.
- **Upload parsing + workspace provisioning** — phase 11 (it reuses the
  `postgres` adapter this phase ships).
- **Background connection re-testing / scheduling** — dispatch is phase 06's queue
  (this phase exposes `TestConnection` and the interval config only).
- **External secret-manager backend** — a future driver behind the custody seam
  (RFC §6.3), not V1.

## Design

### Data flow

```
admin registers source ──▶ data_sources row (config_json + secret_ciphertext, status=unverified)
                             │
   TestConnection ───────────┤ adapter built (secret decrypted in-memory only) → status swap
                             │                                        + last_tested_at + last_error
   DiscoverSchema ──────────▶ candidate datasets (schema_json + TypeCategory once)
                             │
   selective registration ─▶ datasets rows (origin=source_table) — queryable, grant-gated
                             │
   Query(ValidatedSQL) ─────▶ [phase 10 wires the read path over this hook]
```

### Key types (indicative)

```go
// Adapter — the read-only, access-scoped connector to one customer data source.
type Adapter interface {
    Kind() Kind
    Dialect() Dialect
    Capabilities() Capabilities
    TestConnection(ctx context.Context) error
    DiscoverSchema(ctx context.Context, scope access.SourceScope) ([]DatasetCandidate, error)
    SampleValues(ctx context.Context, scope access.SourceScope, ref ColumnRef, limit int) ([]string, error)
    Query(ctx context.Context, sql ValidatedSQL, opts QueryOpts) (QueryResult, error)
}

// ValidatedSQL — an opaque, unforgeable token constructible ONLY by internal/exec
// (phase 09). Declared here as the sole executable input; an adapter cannot run a
// raw string (P1b, D-021). Until phase 09, this is the seam's interface obligation.
type ValidatedSQL interface{ validatedSQL() } // sealed marker; no exported ctor

// Capabilities — feature set, queried via Supports, never by type-switching.
type Capabilities interface{ Supports(Feature) bool }

// Materializer — the SEPARATE write interface (P1c). Declared, not implemented
// here; only destination-capable drivers implement it (phase 13).
type Materializer interface {
    Materialize(ctx context.Context, scope access.DestScope, spec MaterializeSpec) (MaterializeResult, error)
}
```

- **Factory registry.** `Register(kind Kind, ctor Factory)` called from each
  driver's `init()`; `New(kind, cfg, custody)` returns a typed `ErrUnknownKind`
  for an unregistered kind (never a nil adapter). Duplicate registration is a
  boot-time programmer error (panic in `init`, before serving).
- **Secret-free read shape (P6/§6.2).** The persisted row has
  `secret_ciphertext`; the `SourceView` returned by list/get has **no** secret
  field — enforced at the type level (criterion 1). Only the custody component
  ever touches ciphertext, and only to build an adapter.
- **Credential custody (§6.3, D-016).** `Custody` wraps AES-256-GCM: `Encrypt`
  uses ring key 0 (primary) with a fresh random nonce; `Decrypt` iterates the ring
  until a key authenticates the GCM tag. Rotation = prepend a new primary; no bulk
  re-encryption. Ring keys arrive via `env: CHARTWORKS_SOURCE_KEYS` (ordered,
  32-byte keys). Constructing custody with an empty ring while any
  `secret_ciphertext` exists is a refused boot (criterion 4). Plaintext secrets
  never enter a log, an error, or a `QueryResult`; credential *events*
  (`stored|rotated|test-failed`) are content-free audit rows (criterion 10).
- **Status lifecycle (§6.2).** `unverified → connected → unavailable`;
  `TestConnection` runs the adapter probe and swaps status, stamping
  `last_tested_at` and a content-free `last_error`. Pengui reads status live
  (RFC §15 — status, not readiness). Illegal transitions are typed errors.
- **`TypeCategory` (§7.1).** `DiscoverSchema` maps each dialect column type to a
  normalized `TypeCategory` **once**, stored on the dataset's `schema_json`. The
  `money`/`numeric`/`decimal` family maps to `numeric` (brief 02's named bug).
  The `ansi` sentinel dialect covers unknown-dialect columns → `unknown`, a typed
  category, never a silent guess.

### Seam & property alignment

- **P1a** — registry reads and discovery take non-optional `access` scope; empty
  effective set short-circuits (no query issued). Nothing is queryable until an
  admin selectively registers a dataset.
- **P1b** — `Query`'s only executable input is `ValidatedSQL`; the adapter is
  structurally incapable of running a raw string (criterion 6).
- **P1c** — `Query` exposes no write; writes live only on the separate
  `Materializer` (declared here, implemented phase 13).
- **P3** — every `data_sources` / `datasets` access carries the tenant predicate
  through the phase-02 store methods; no unscoped query API exists.
- **P4** — `null` adapter, missing-ring boot, unknown kind, illegal status
  transition all fail loud with typed errors + metrics, never a silent degrade.
- **P5/P7** — no provider SDK here; one registry, one custody component, one
  resolver consumed (not re-implemented). Drivers are pure-Go (D-005).

## Config keys added

Domain `sources` (RFC §14). Documented here, in the example config, and smoke-checked.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `sources.credential_key_ring` | `[]string` via `env: CHARTWORKS_SOURCE_KEYS` | — | yes, if any stored secret exists | Ordered master key ring; **primary = first**. 32-byte keys (base64). Empty ring + stored secret ⇒ refused boot. |
| `sources.connection_test_interval` | duration | `15m` | no | Cadence hint for background re-testing (dispatch is phase 06); `0` disables. |
| `sources.discovery_sample_limit` | int | `100` | no | Ceiling for `SampleValues` rows (large-table safeguard, brief 09/02). |
| `sources.postgres.connect_timeout` | duration | `10s` | no | Per-kind default; adapter connect/handshake timeout. |
| `sources.postgres.discovery_timeout` | duration | `30s` | no | Per-kind default; `DiscoverSchema` deadline. |

## Acceptance criteria

1. **Secret-free read shape (type-level).** The registry list/get return type
   (`SourceView`) has no secret field and no `secret_ciphertext`; a
   compile-time/reflection test asserts absence, so a secret cannot leak on read.
2. **Envelope encryption round-trip.** AES-256-GCM `Encrypt`→`Decrypt` returns the
   original plaintext; ciphertext ≠ plaintext; two encrypts of the same plaintext
   differ (fresh nonce).
3. **Ring rotation decrypt.** A secret encrypted under the old primary still
   decrypts after a new primary is prepended (decrypt tries all ring keys);
   `Encrypt` always uses the current primary. No bulk re-encryption needed.
4. **Refused boot on missing ring with stored secrets.** Constructing custody with
   an empty key ring while ≥1 `secret_ciphertext` is present returns a typed error
   (refused boot) — never a silently generated key.
5. **`null` adapter fails loud.** Every `null`-adapter operation
   (`TestConnection`/`DiscoverSchema`/`SampleValues`/`Query`) returns a typed
   unavailable error, never `nil`/empty.
6. **Raw-string execute unexpressible.** `Query`'s executable input is the opaque
   `ValidatedSQL`, unconstructible outside its owning package; a compile-time proof
   test confirms a raw string cannot be passed, and no adapter method takes a raw
   SQL string for execution.
7. **Factory registry — unknown kind is typed.** `New` with an unregistered kind
   returns `ErrUnknownKind` (never a nil adapter); a driver registers via `init()`.
8. **Capability gating, not type-switching.** Feature support is read via
   `Capabilities().Supports(f)`; an architecture test asserts no `switch a.(type)`
   over concrete adapters outside the factory.
9. **`TypeCategory` computed once, golden.** A dialect→category golden fixture maps
   column types to `{numeric,temporal,boolean,text,structured,binary,unknown}`;
   `money`/`decimal` ⇒ `numeric` (brief 02 bug); the category is stored on the
   dataset and not recomputed on read.
10. **Content-free credential events.** `stored|rotated|test-failed` events contain
    ids + principal + decision only; a test scans emitted payloads and asserts no
    plaintext-secret and no `config_json`-secret bytes appear.
11. **Status lifecycle.** `unverified → connected → unavailable` transitions are
    accepted, an illegal transition is a typed error, and `TestConnection` updates
    status + `last_tested_at` + content-free `last_error`.
12. **Discovery conformance on Docker Postgres.** The `postgres` driver passes the
    adapter conformance suite (`TestConnection`, `DiscoverSchema`, `SampleValues`,
    tenant-scoped) against a fresh Docker Postgres, under `-race`; SKIPs cleanly
    when the test DSN is unset.

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven `TypeCategory` classification golden (criterion 9);
  custody round-trip + rotation + fail-closed boot (2–4); factory unknown-kind (7);
  `null` fail-loud (5); status-lifecycle transitions (11); secret-free-shape
  reflection test (1); capability-gating architecture test (8); `Query`-signature
  compile-proof (6).
- **Integration:** **required** — Deps name phase-02 (store) and phase-04 (access).
  The `postgres` adapter conformance suite runs against a **real Docker Postgres**
  (`make pg-up`, fresh DB, convention 8) proving discovery, typing, and tenant-scope
  propagation end to end (criterion 12), under `-race`. Registry rows persist
  through the real `store` driver.
- **Adversarial:** this phase touches an access-scoped read path. Cross-tenant
  probe (a source/dataset of tenant A is invisible to tenant B's discovery and
  list) and empty-access-set short-circuit (no query issued) ride here; the
  registry-driven cross-tenant suite (RFC §5.5) enumerates this phase's read
  methods. Fetch-then-filter regression guard on registry list. Forged-header:
  n/a at this layer (identity is the frozen envelope from phase 03), but the
  scope parameter is non-optional so a header cannot substitute.
- **Fuzz:** `FuzzDecryptEnvelope` over arbitrary ciphertext bytes — invariant:
  never panics, and only an authentic GCM tag under a ring key yields plaintext
  (a corrupted/forged ciphertext is a typed error, never a partial read).
- **Bench:** `BenchmarkEncrypt` / `BenchmarkDecryptRingScan` (custody is on the
  adapter-construction path; ring-scan cost scales with rotation depth) — baseline,
  not a CI gate.

## Coverage targets

`internal/sources` carries credential custody (a security path) and is a
conformance-tested subsystem ⇒ **85%** (above the 80% new-package default;
convention 4). The implementing PR adds the entry to `scripts/coverage-bands.conf`.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/sources` | 85% | Credential custody security path + conformance-tested adapter seam (CLAUDE.md §11, convention 4). |

## Smoke checks

Each criterion maps to a named Go test invoked by `scripts/smoke/phase-08.sh`
(via `run_group`); the script SKIPs entirely until `internal/sources` exists, and
criterion 12 SKIPs when the Docker-Postgres DSN is unset.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestSourceViewHasNoSecretField` passes (reflection: no secret field/tag). |
| 2 | `TestEnvelopeEncryptRoundTrip` passes (round-trip + nonce uniqueness). |
| 3 | `TestKeyRingRotationDecrypt` passes (old-key ciphertext decrypts after rotate). |
| 4 | `TestMissingRingWithStoredSecretsRefusesBoot` passes (typed refused boot). |
| 5 | `TestNullAdapterFailsLoud` passes (every op returns a typed error). |
| 6 | `TestQueryRequiresValidatedSQL` passes (compile-proof: no raw-string execute). |
| 7 | `TestFactoryUnknownKind` passes (`ErrUnknownKind`, non-nil adapter never returned). |
| 8 | `TestNoAdapterTypeSwitch` passes (architecture scan: gate via `Supports`). |
| 9 | `TestTypeCategoryClassificationGolden` passes (`money`⇒`numeric`, computed once). |
| 10 | `TestCredentialEventsContentFree` passes (no secret/config bytes in payloads). |
| 11 | `TestConnectionStatusLifecycle` passes (legal swaps + typed illegal-transition). |
| 12 | `TestPostgresAdapterConformance` passes under `-race` on Docker Postgres (SKIP if DSN unset). |

## Glossary additions

New terms, pre-written for `docs/glossary.md` (same PR, CLAUDE.md §14). Terms
already defined (data source, data-source adapter, dataset, upload workspace) are
**not** duplicated.

- **Connections registry** — the tenant-scoped `data_sources` catalog: non-secret
  `config_json`, separately encrypted credential, and the status lifecycle
  `unverified → connected → unavailable`. Its read/list shape carries no secret
  field (RFC §6.2).
- **Credential custody** — the envelope-encryption component: AES-256-GCM data
  secrets protected by a config-supplied master **key ring**; encrypt uses the
  primary key, decrypt tries every key, rotation prepends a new primary (RFC §6.3,
  D-016). Missing ring with stored secrets ⇒ refused boot.
- **Key ring** — the ordered list of master keys (`env: CHARTWORKS_SOURCE_KEYS`);
  first = primary. Enables no-bulk-re-encrypt rotation.
- **Adapter capabilities** — a data-source adapter's declared feature set
  (CTE/LIMIT/temp-views/…), queried via `Capabilities().Supports(x)` and never by
  type-switching on the concrete adapter (RFC §6.1).
- **Schema discovery** — the adapter operation that enumerates a source's
  schemas/tables/columns into candidate datasets, classifying each column's
  `TypeCategory` **once** at discovery (RFC §7.1).
- **`TypeCategory`** — the dialect-agnostic column classification
  (`numeric / temporal / boolean / text / structured / binary / unknown`) computed
  once at discovery; the fix for the predecessors' re-derived-`money` bug.
- **`null` adapter** — the fail-loud placeholder driver whose every operation
  returns a typed unavailable error; the fork keeper that prevents a
  silently-degrading no-op source (brief 05).
- **`ansi` dialect sentinel** — the generic dialect used for unknown-dialect
  handling, degrading unknown constructs/types to typed rejections rather than
  silent passes (RFC §6.1, brief 05).

## Decisions filed

No new `D-NNN` entries. This phase implements existing decisions:

- **D-016** — customer data-source credentials encrypted at rest in the store
  (custody mechanism, §6.3).
- **D-021** — SQL-safety: `ValidatedSQL` is the only executable input; split
  read/write interfaces (`Query` vs `Materializer`, P1b/P1c).
- **D-020** — the grants/scope model consumed on every registry read + discovery.
- **D-024** — the `postgres` adapter also serves the upload workspace (phase 11).
- **D-004** — the customer data source is reached through the adapter seam, never
  the `store` seam (boundary preserved here).
- **D-005** — pure-Go `postgres` driver (pgx/v5); CGo stays disabled.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3): each reasonable deviation,
     why, and confirmation this file was updated in the same PR. -->

- none yet.
