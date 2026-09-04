# Phase 07 — vindex

Status: planned. Owner: internal/vindex. Hard dependencies: 02, 04.

## Authority and design

RFC-001 §8, D-049 and [COMMON.md](COMMON.md) apply. Facets are authorized semantic evidence, not an alternative source of access policy.

## Brief findings incorporated

Briefs 03, 05: typed facets, bounded batched retrieval, source attribution, tenant cleanup and compact context.

## Findings I'm departing from

No old/new model mixing, tenant-only cache assumption or source POC timing represented as a Go benchmark.

## Scope and implementation tasks

1. Implement the pgvector seam with batch upsert/delete/search and topic/version/tenant/authority restrictions.
2. Pin embedding dimensions/model per facet generation; support scoped batched retrieval with source attribution and deterministic tie ordering.
3. Carry publication/archive/tenant-erasure cleanup via version-fenced transitions; use real PostgreSQL/pgvector fixtures.

## Non-goals

No local IAM, second vector backend, speculative distributed index or local embedding-model server.

## Config and persistence

Vindex search/batch limits and embedding-generation identifiers; index tuning documented against measured data. Store exact tenant/topic/version/facet provenance and embedding model/dimensions. Published-generation pointers are fenced by the semantic lifecycle consumer.

## Acceptance criteria

1. **AC01** — Foreign, unpublished or disallowed facet generations cannot enter results, even in batched search.
2. **AC02** — Upsert/search rejects dimension/model mismatch; a migration never mixes old/new embedding generations.
3. **AC03** — Batch result source tracking, tie ordering and per-kind limits match golden fixtures and scalar-query semantics.
4. **AC04** — Delete-by-topic/version/tenant and archive invalidation remove all targeted facets without affecting other tenants.
5. **AC05** — Concurrent publication/search cannot expose new semantics with old facets; read-through state is immutable per request.
6. **AC06** — Real-driver conformance, query-plan observations and batch benchmarks run; POC timings are not reported as target measurements.

## Tests, coverage and smoke

Implement `TestPhase07/AC01` through `TestPhase07/AC06` against PostgreSQL/pgvector with concurrent read/publish/delete fixtures. COMMON.md requires 85% conformance coverage. `scripts/smoke/phase-07.sh` requires all six results; semantic lifecycle integration extends the same contract in phase 15.

## Glossary, decisions and deviations

Published facet generation and source attribution use the shared glossary. D-049 applies. No runtime completion is claimed.
