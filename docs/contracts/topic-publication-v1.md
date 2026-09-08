# Topic publication lifecycle v1

Status: phase 15 implementation candidate, 2026-09-08. This contract includes the
real publication, retained-health and explicit recheck consumers. Exact-head review,
cumulative coverage and release integration remain separate gates.

The exact-head review and root verification for this bounded slice are recorded
in the [phase 15 publication evidence](../reviews/phase-15-topic-publication.md).

## Review and publication

An explicit review records an immutable approve/reject receipt for one exact private
draft revision and compiler digest. `topics.review` and the complete persisted draft
dependency reach are checked before the receipt is written. Reviewer identity and
session remain protected audit/storage evidence and are absent from public response
types. A rejected receipt cannot publish.

Publication requires a current `topics.publish` bearer with topic publish, source
read, dataset query and execution-context use reach. Live source discovery verifies
the exact source revision, context, dataset and safe column shape before gateway
dispatch. The service compiles the reviewed draft and obtains one immutable,
non-secret provider/route/endpoint/model/revision/dimension/preprocessing/input/
normalization descriptor from the actual Bifrost engine. Facet inputs are bounded
before dispatch and partitioned by their actual execution context and source.

Every per-context generation is staged invisibly. One PostgreSQL transaction locks
the topic head and the sorted union of old and new context heads, verifies every
manifest origin and body hash, marks complete generations ready, retires removed
contexts, and switches all vector pointers with the semantic version. Any failure
leaves the previous publication usable. Deferred database constraints reject a
standalone vector publish, archive or cleanup that would desynchronize a managed
topic head.

Canonical entity meaning is the tenant-wide stable ID, exact revision, name and
normalized aliases; physical key references remain part of the reviewed topic. Drafts
may propose exact reuse, a new ID at revision one, or the next revision. Publication
checks collisions before gateway work and again under a tenant registry lock. Creating
a revision requires the existing tenant-write resource under `topics.publish`; exact
revision reuse does not. The new immutable revision, append-only term reservations,
topic-local reference rows, published definition and facet/head transition commit or
roll back together. A same revision with different meaning, a revision gap, or a term
reserved to another ID is a conflict.

Canonical facets are split by actual source and execution context and contain only
that origin's key references. Global vocabulary may repeat in those local facets; no
facet carries another context's physical keys.

## Read, health, rollback and archive

Retained current and exact-version reads use only stored public definition and facet
metadata. They make no source or model request and do not expose private profile,
actor or session provenance. They still require current `topics.read` plus the topic
and every persisted source/dataset/context reach.

The separate contract operation requires both its registered primary `topics.read`
action and the existing secondary `sources.read` action. It performs current source
discovery, checks exact revision/context/dataset/column evidence, then confirms the
same publication revision inside PostgreSQL. It returns either the retained
publication with an observation time or a typed context change/conflict; published
state alone is not a health claim.
Managed facet search similarly threads the verified envelope into the repository and
checks all dependencies and current source revisions in the same repeatable-read
snapshot before selecting facet bodies. Legacy unmanaged vector fixtures retain their
scope-only behavior; raw coordinates cannot read a managed topic.

Rollback requires both its registered primary `topics.publish` action and the
existing secondary `sources.read` action. It reactivates one exact retained semantic
version and its complete original context/generation set without a gateway call,
rechecks current source evidence and authority, retires contexts absent from the
target, and commits one new lifecycle revision. Archive requires `topics.publish`
and the persisted resource reaches but performs no source discovery; it atomically
marks the topic and all active facet heads unavailable and remains possible when a
source is unhealthy. No-op rollback/archive transitions are conflicts rather than
new evidence.

## Registered surface and remaining scope

The shared topic registry and Go SDK expose review, publish, retained current/exact
read, retained health, explicit current-source recheck, current contract, rollback
and archive alongside the private draft
operations. Unknown or inaccessible coordinates remain nondisclosing; action and
resource failures happen before gateway or facet payload access. Gateway failure,
manifest failure, CAS contention and deferred consistency failure cannot expose a
partly active version.

Draft source-reference rewrite, entity/onboarding APIs and neutral lifecycle
portability are concrete consumers. Health reads use only the retained public
snapshot; recheck performs source discovery and commits a complete replacement for
the same publication revision. Private profile IDs and evidence never enter the
public health DTO. Phase 16 activation/evaluation remains separately owned.
