# Phase 07 — `vindex` (pgvector facet index)

> **Status:** draft
> **Owner:** orchestrator
> **Depends on:** phase-02-store-migrations

Authored per CLAUDE.md §16. Copy origin:

```bash
cp docs/plans/_template.md docs/plans/phase-07-vindex.md
cp scripts/smoke/_template.sh scripts/smoke/phase-07.sh
```

---

## RFC / request sections

- **RFC-001 §3.2** — the package inventory: `internal/vindex` = "Vector search seam
  + `pgvector` driver (routing facets only; D-029)".
- **RFC-001 §12** — the `facet_vectors` table in the budgeted store inventory:
  `tenant_id, topic_id, version_id, facet_kind, facet_id, embedding vector(D) —
  HNSW, cosine (\`vindex\`)`.
- **RFC-001 §8.3** — facet decomposition at publish: packs decompose into typed
  facets (measure / dimension / derived-KPI / query-pattern / example) embedded into
  `vindex` under `(tenant, topic, version)` scoping; retrieval returns governed
  semantic units, never raw schema.
- **RFC-001 §9.1** — the routing consumer: facet retrieval from `vindex` (typed
  candidates, k-per-type, tenant-scoped cache). This phase ships the seam that
  serves that retrieval; the retrieval *policy* lands in phase 17.
- **RFC-001 §13 / §14** — embedding model + dims are **pinned per index** and
  validated at boot; the schema's `vector(D)` carries that pinned dimension.
- **D-029** — `vindex` seam confirmed: pgvector single driver, facet vectors only,
  scoped `(tenant, topic, version)`, dims pinned per index.

## Depends on

- **phase-02-store-migrations** — owns the §12 schema, including the `facet_vectors`
  table DDL, the `pgvector` extension enablement, and the HNSW/cosine index (phase 02
  migrates "the complete budgeted 25-table inventory"). This phase adds **no**
  migrations; it delivers the seam + driver over that table. If any of {extension,
  table, HNSW index} is absent when this phase lands, that is a **phase-02 gap fixed
  in phase 02's migration set** (forward-only discipline, §9 CLAUDE.md) — never
  re-migrated here.
- Runs in **Wave 2 alongside phase 05 (`gateway`)** but does **not** depend on it at
  build time. The pinned embedding dimension `D` is injected into the driver at
  construction by the composition root; the gateway's boot validation that the
  configured model's dims equal `D` is **phase 05's** obligation, not this one.

## Informing briefs

- **Brief 03** (`03-predecessor-nlq-pipeline.md`) — the routing / context-engineering
  layer: facet retrieval feeds routing; typed candidates with k-per-type retrieval;
  the tenant-embedding cleanup the fork carried.

## Brief findings incorporated

- **Typed facets, not raw schema (brief 03).** The seam stores and retrieves five
  typed facet kinds — `measure`, `dimension`, `derived_kpi`, `query_pattern`,
  `example` — matching RFC §8.3's decomposition. `Search` filters by kind so routing
  can pull *k-per-type* rather than a flat top-k, exactly as brief 03 documents the
  predecessors doing.
- **Tenant-embedding cleanup carried (brief 03 / fork keeper, RFC §8.2 rule 6).**
  Deletion is first-class and cascades at three grains from one selector: by version
  (publish/rollback invalidation), by topic (archive), and by tenant (erasure). The
  fork's "orphaned vectors on tenant delete" scar is closed by making
  `Delete(tenant-only)` a supported, tested operation.
- **Routing reads pre-built state (brief 03 / RFC §3.3).** The seam is a plain
  read/write index — no LLM call sits inside it. Embedding *production* is a gateway
  concern (phase 05) upstream; `vindex` only stores the resulting vectors and serves
  cosine-nearest retrieval. Expensive work never rides the query hot path.

## Findings I'm departing from

- **none.** Brief 03's vindex-relevant findings are adopted as above. The brief's
  richer routing machinery (evidence aggregation, calibrated confidence, caches) is
  **out of scope here** — it is phase 17's, which consumes this seam. That is a scope
  boundary, not a departure.

## Scope

Delivers `internal/vindex`:

- The **`Index` seam** (interface + factory + driver, §4.4 pattern): `Upsert`,
  `Delete`, `Search`, all batch-shaped, all taking a non-optional scope/selector with
  a mandatory non-empty `tenant_id`.
- The **`pgvector` driver** over the store's Postgres (pgx/v5, shared DSN/pool — the
  `facet_vectors` table lives in Chartworks' own store DB, §12). HNSW index, cosine
  distance (`<=>` / `vector_cosine_ops`).
- The **facet-kind vocabulary** as a closed Go enum: `measure`, `dimension`,
  `derived_kpi`, `query_pattern`, `example`.
- **Dimension enforcement**: the pinned `D` is injected at driver construction;
  every upserted vector whose length ≠ `D` is rejected with a typed error
  (defense-in-depth beneath phase 05's boot validation).
- A **conformance suite** against Docker Postgres + pgvector (`make pg-up`), and the
  standing adversarial obligations (cross-tenant probe, scope-escape guard) for this
  seam.
- Registers `internal/vindex` (+ the `pgvector` sub-package) in
  `scripts/coverage-bands.conf` at the **85%** `vindex` band and wires
  `scripts/smoke/phase-07.sh` — both **in this phase's PR**.

## Non-goals

- **No migrations.** The `facet_vectors` DDL, `pgvector` extension, and HNSW index
  are phase 02's (§12). See *Depends on*.
- **No embedding production.** Vectors arrive pre-computed; the `embedding` gateway
  role and its metering are phase 05's (§13).
- **No retrieval policy.** k-per-type selection, evidence aggregation, tenant-scoped
  caches, calibrated confidence, span hints — all phase 17 (§9.1). This seam returns
  raw cosine matches; the router decides what to do with them.
- **No facet decomposition.** Turning a published topic pack into facets, and the
  publish/rollback/archive invalidation that *calls* `Upsert`/`Delete`, is phase 15
  (§8.2/§8.3). This phase proves the primitives those transitions will invoke.
- **No second driver.** pgvector only in V1 (D-029). The seam is the contract; a
  future backend is a new driver, never a rewrite.

## Design

### Data flow

```
publish/rollback (phase 15) ─Upsert(scope, []Facet)─▶ ┌───────────────┐
archive / erase   (phase 15) ─Delete(selector)──────▶ │ vindex.Index  │
                                                       │  (pgvector)   │
routing           (phase 17) ─Search(scope, query)──▶ └──────┬────────┘
                              ◀──[]Match (cosine)────────────┘
                                             facet_vectors (store DB, §12)
```

### The seam (indicative Go)

```go
package vindex

type FacetKind string

const (
    FacetMeasure      FacetKind = "measure"
    FacetDimension    FacetKind = "dimension"
    FacetDerivedKPI   FacetKind = "derived_kpi"
    FacetQueryPattern FacetKind = "query_pattern"
    FacetExample      FacetKind = "example"
)

// Scope is the inescapable (tenant, topic, version) predicate. TenantID and
// TopicID and VersionID are all mandatory on Upsert/Search — a zero value is a
// typed error, never a wildcard.
type Scope struct {
    TenantID  string
    TopicID   string
    VersionID string
}

// Selector narrows a Delete. TenantID is mandatory (non-empty); TopicID and
// VersionID are optional narrowing predicates. This one shape yields the three
// cascade grains (P7 — one representation):
//   {tenant}                     → delete-by-tenant   (erasure)
//   {tenant, topic}              → delete-by-topic     (archive)
//   {tenant, topic, version}     → delete-by-version   (publish/rollback swap)
type Selector struct {
    TenantID  string // required, non-empty
    TopicID   string // "" = all topics in tenant
    VersionID string // "" = all versions in topic
}

type Facet struct {
    FacetID   string
    Kind      FacetKind
    Embedding []float32 // len must == pinned D
}

type Query struct {
    Vector []float32   // len must == pinned D
    Kinds  []FacetKind // empty = all kinds; else restrict (supports k-per-type)
    K      int         // top-K by cosine similarity, clamped to a ceiling
}

type Match struct {
    FacetID string
    Kind    FacetKind
    Score   float32 // cosine similarity (1 - distance)
}

type Index interface {
    // Upsert replaces the scope's facets (batch). Idempotent on (scope, facet_id).
    Upsert(ctx context.Context, scope Scope, facets []Facet) error
    // Delete removes every row matching the selector; returns the row count.
    // A no-match delete is not an error (idempotent invalidation).
    Delete(ctx context.Context, sel Selector) (int64, error)
    // Search returns the top-K cosine matches within scope, optionally by kind.
    Search(ctx context.Context, scope Scope, q Query) ([]Match, error)
}
```

### How it upholds P1–P7

- **P1a / P3 (deny-by-default, tenant isolation — inescapable predicate).** No method
  exists without a scope/selector carrying a **mandatory non-empty `tenant_id`**; a
  scope-less query cannot be expressed (the type has no such constructor — §6 CLAUDE.md
  "a query method without a scope parameter is rejected"). Every generated SQL
  statement pins `WHERE tenant_id = $1 AND topic_id = $2 AND version_id = $3` (Search)
  before the `ORDER BY embedding <=> $q`, so a cross-tenant or cross-version vector is
  structurally unreachable — it is filtered *inside* the query, never fetch-then-filter.
  An empty scope (blank tenant) is a typed error, not a wildcard scan.
- **P4 (fail loud).** A vector whose length ≠ pinned `D` ⇒ typed error, never a
  silent truncate/pad. A blank `tenant_id` on any call ⇒ typed error. A driver
  construction with `D ≤ 0` ⇒ refused construction. No empty-catch, no silent
  degrade to a full-table scan.
- **P5 (one intelligence seam).** `vindex` imports **no** provider SDK and makes **no**
  model call — it stores vectors produced upstream by the `gateway` `embedding` role.
  The dimension `D` is injected, sourced from gateway config; this seam never reaches
  a provider.
- **P6 (domain vocabulary).** `vindex`, `embedding`, `vector`, `facet` are **internal**
  package/type names only. They never appear on a wire, UI, error message, or
  human-read log surface: typed errors use domain-clean codes (e.g.
  `facet.dimension_mismatch`, `facet.scope_missing`) and content-free logs. The §2
  forbidden list governs external surfaces; this seam has none.
- **P7 (one primitive family).** One `Index` interface, one pgvector driver, one
  `Selector` shape yielding all three delete grains — no parallel per-grain methods,
  no second vector path.

### Embedding-dims pinning interaction with the gateway

The `facet_vectors.embedding` column is typed `vector(D)` in phase 02's migration,
where `D` is the pinned dimension (a schema constant / config-migration value; pgvector
requires a fixed dimension to build an HNSW index). At runtime:

1. **phase 05 (gateway)** validates at boot that the configured `embedding` model's
   output dimension equals the pinned `D` — a mismatch is a **refused boot** (§13).
2. **phase 07 (this seam)** is constructed with that same `D` and enforces it on every
   write: `len(facet.Embedding) != D` ⇒ typed `facet.dimension_mismatch`. This is
   **defense-in-depth** — the boot check guards config drift; the per-write check guards
   a caller passing a wrong-width vector, and the DB's `vector(D)` type is the third
   backstop (an insert of the wrong width fails at the driver). All three must hold
   independently.

`D` is therefore **not** a `vindex` config key — it is injected at construction from
the gateway/embedding config the composition root already owns.

### Persistence

Reuses the **store's** Postgres (pgx/v5) DSN + pool — `facet_vectors` is one of the §12
store tables (Chartworks' own state), not customer-data territory. `vindex` opens no
new connection config; it takes a pgx pool/handle at construction. HNSW cosine index
(`USING hnsw (embedding vector_cosine_ops)`) is created by phase 02; Search issues
`ORDER BY embedding <=> $query LIMIT k` under the scope predicate.

## Config keys added

**none.** The seam reuses the `store` Postgres connection (§14 `store`) and the pinned
embedding dimension from `gateway.embedding` (§14 `gateway`, injected at construction).
No `vindex`-specific config key exists — deliberately, to avoid a dims value that could
drift from the gateway's pinned value (single source of truth, P5/§13).

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| _(none)_ | | | | Reuses `store` DSN + `gateway.embedding` dims (injected) |

## Acceptance criteria

1. **Cross-tenant search returns nothing.** Facets upserted under tenant A are never
   returned by a `Search` scoped to tenant B, even with an identical query vector and
   identical `(topic, version)` ids (adversarial — §5.5).
2. **Search is inescapably scoped to `(tenant, topic, version)`.** A facet under
   `(topic X, version 1)` is not returned by a `Search` scoped to `(topic X, version 2)`
   or `(topic Y, version 1)`; the predicate is applied inside the query.
3. **Delete-by-tenant cascades.** `Delete({tenant})` removes every facet across all
   topics/versions of that tenant and only that tenant; returns the removed count.
4. **Delete-by-topic cascades.** `Delete({tenant, topic})` removes every version's
   facets for that topic, leaving other topics of the same tenant intact.
5. **Delete-by-version cascades.** `Delete({tenant, topic, version})` removes exactly
   that version's facets, leaving the topic's other versions intact. A no-match delete
   returns count 0 and no error (idempotent invalidation).
6. **pgvector conformance on Docker Postgres.** Against `make pg-up` (Postgres +
   pgvector): a batch `Upsert` followed by `Search` returns the nearest facets in
   descending cosine-similarity order, honoring `K` and the `Kinds` filter (k-per-type).
7. **Dimension mismatch is a typed error.** `Upsert` (or `Search`) with a vector whose
   length ≠ the pinned `D` returns a typed `facet.dimension_mismatch` error and writes
   nothing — never a silent truncate/pad/degrade (P4).
8. **Batch upsert round-trips and is idempotent.** Upserting N facets in one call
   persists all N; re-upserting the same `(scope, facet_id)` with a new vector replaces
   it (no duplicate rows).
9. **Concurrent reuse is race-free.** Concurrent `Search`/`Upsert`/`Delete` against one
   shared driver instance pass under `-race` (the driver is immutable after
   construction; per-request state rides ctx/params, §5 CLAUDE.md).

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven facet-kind enum validation, `Selector` → predicate mapping
  (the three grains), dimension-check, `Query.K` clamping. Pure-Go, no DB.
- **Integration / conformance:** the vindex conformance suite runs against **real**
  Docker Postgres + pgvector (`make pg-up`) under `-race` — this phase introduces the
  public `Index` interface phases 15 and 17 build on, and closes the seam over phase
  02's `facet_vectors` table (§17 trigger). Criteria 1–8 are conformance tests; they
  `t.Skip` cleanly when the store URL is unset (surfaced as SKIP by the smoke runner).
  No boundary mock — the seam *is* pgvector.
- **Adversarial:** criterion 1 (cross-tenant probe) and criterion 2 (scope-escape
  guard) are standing obligations for this tenant-scoped seam (§5.5). The cross-tenant
  probe seeds two tenants and asserts a bare match count of 0 across the boundary.
- **Fuzz:** n/a — `vindex` parses no external/attacker-controlled encoded input
  (vectors are `[]float32` from the gateway; ids are validated store keys). The
  parse/decode fuzz obligation lives on JWT (phase 03), NLQ payloads and SQL validation
  (phases 09/17/18).
- **Bench:** `BenchmarkSearch` on the HNSW path (a hot reusable artifact serving the
  routing hot path). Baseline only, not a CI gate (§11).

## Coverage targets

`vindex` is a **conformance-tested seam** → the 85% band (convention 4). Entries added
to `scripts/coverage-bands.conf` in this phase's PR:

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/vindex` | 85% | Conformance-tested seam / tenant-scoped access path (convention 4) |
| `internal/vindex/pgvector` | 85% | The V1 driver — same band as `store` drivers |

## Smoke checks

`scripts/smoke/phase-07.sh` SKIPs the whole script until `internal/vindex` exists, then
drives each acceptance criterion as one `go test` name via `run_group` (Docker-Postgres
tests `t.Skip` → SKIP when no store URL). Criterion 9 runs under `-race`.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestCrossTenantSearchReturnsNothing` PASS |
| 2 | `TestSearchScopedToTopicVersion` PASS |
| 3 | `TestDeleteByTenantCascades` PASS |
| 4 | `TestDeleteByTopicCascades` PASS |
| 5 | `TestDeleteByVersionCascades` PASS |
| 6 | `TestPgvectorConformance` PASS |
| 7 | `TestDimensionMismatchRejected` PASS |
| 8 | `TestBatchUpsertRoundTrip` PASS |
| 9 | `TestConcurrentReuse` PASS (run under `-race`) |

## Glossary additions

New terms this phase introduces (landed in `docs/glossary.md` "Internals & seams" in
the same PR, per §14). Only genuinely-new terms; existing entries are untouched.

- **`vindex` seam** — the vector-search seam over topic facets; V1 driver `pgvector`,
  facet vectors only, scoped `(tenant, topic, version)`, embedding dimension pinned per
  index and validated at boot (D-029, RFC §3.2/§8.3). An internal seam — its names
  (`embedding`, `vector`, `facet`) never reach a wire/UI/error surface (P6).
- **Facet** — a typed, embedded semantic unit decomposed from a published topic pack
  for routing retrieval; kinds: `measure` / `dimension` / `derived_kpi` /
  `query_pattern` / `example` (RFC §8.3). Retrieval returns governed facets, never raw
  schema.

## Decisions filed

No new decision. This phase implements existing decisions:

- **D-029** — `vindex` seam: pgvector single driver, facet vectors only, scoped
  `(tenant, topic, version)`, dims pinned per index.
- **D-004** — the `facet_vectors` table is Chartworks' own store state, distinct from
  customer data sources; `vindex` reuses the store Postgres.
- Relies on the RFC §12 schema (phase 02) and RFC §13 embedding-dims pin (phase 05).

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3): any reasonable deviation from this
     plan discovered while building it — what changed, why, and confirmation this file
     was updated in the same PR. Empty at authoring time. -->

_none yet._
