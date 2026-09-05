# Phase 07 — vindex

Status: planned. Owner: internal/vindex. Hard dependencies: 02, 04.

## Authority and design

RFC-001 §8, D-049/D-053 and [COMMON.md](COMMON.md) apply. Facets are authorized semantic evidence, not access policy. The vector store receives vectors computed through [the Bifrost SDK contract](../contracts/model-gateway.md); it contains no local inference engine.

## Brief findings incorporated

Briefs 03 and 05: typed facets, bounded batched retrieval, source attribution, tenant cleanup and compact context. Preserve independent model-space identity and input mapping from the reviewed gateway references.

## Findings I'm departing from

No old/new embedding-space mixing, tenant-only cache key or source POC timing represented as a Go benchmark. Identical vector dimensions do not make different models compatible.

## Scope and implementation tasks

1. Implement pgvector batch upsert/delete/search with topic/version/tenant/signed-authority restrictions and bounded query limits.
2. Store the complete embedding-space descriptor per generation: provider route/model revision, dimensions, preprocessing/input-type/normalization options. Query vectors must match it. The initial reference dimension is 1024, not an unconditional hard-coded dimension.
3. Preserve batched source association and deterministic ties. Namespace caches by tenant/context plus full space and source-generation identity, never raw bearer or model name alone.
4. Carry publication/archive/tenant cleanup through version-fenced transitions. Semantic publication waits for a complete new generation; provider failure cannot expose half-indexed semantics or silently use another model.

## Non-goals

No local IAM, second vector backend, distributed index platform, local embedding model or direct provider API. Pure PostgreSQL/pgvector search remains local computation, not prohibited local inference.

## Config and persistence

Vindex batch/search bounds and generation descriptors; index tuning follows measured data. Store exact tenant/topic/version/facet provenance and complete embedding-space identity. The generated index dimension agrees with the configured space. Gateway settings are owned by phase 05 rather than duplicated here. A test can supply deterministic vectors to this pure storage boundary; production generation must use Bifrost.

## Acceptance criteria

1. **AC01** — Foreign, unpublished or disallowed facet generations cannot enter results, including batched queries.
2. **AC02** — Upsert/search rejects dimension or full-space mismatch, including same-dimension model/preprocessing changes; generation activation never mixes old/new vectors.
3. **AC03** — Batch origin, tie ordering and per-kind limits match scalar-query semantics; repeated input text in different context partitions cannot leak cached evidence.
4. **AC04** — Delete-by-topic/version/tenant and archive invalidation remove targeted facets without affecting other tenants or generations.
5. **AC05** — Concurrent publication/search or interrupted remote embedding cannot expose new semantics with old/incomplete facets; read-through state is immutable per request.
6. **AC06** — Real pgvector conformance and recorded query-plan/batch benchmarks run; source POC timings are not target measurements, and this package contains no inference/provider implementation.

## Tests, coverage and smoke

Implement `TestPhase07/AC01` through `TestPhase07/AC06` against PostgreSQL/pgvector. Include two 1024-dimensional but incompatible embedding spaces, cross-context caches, interrupted indexing and concurrent publish/delete/search. COMMON.md requires 85% conformance coverage. The gateway-to-generation integration is exercised by the semantic consumer in phase 15; this storage phase stays acyclic and does not invent a local model for its fixtures. `scripts/smoke/phase-07.sh` requires all six results.

## Glossary, decisions and deviations

Embedding-space identity, facet generation and model output dimension are separate terms. D-049/D-053 apply. No runtime completion is claimed.
