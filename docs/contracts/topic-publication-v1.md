# Topic publication lifecycle v1

Status: bounded phase 15 implementation, 2026-09-07. This contract adds a real
publication consumer without claiming the remaining phase 15 capabilities or the
cumulative phase acceptance suite.

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
read, current contract, rollback and archive alongside the seven private draft
operations. Unknown or inaccessible coordinates remain nondisclosing; action and
resource failures happen before gateway or facet payload access. Gateway failure,
manifest failure, CAS contention and deferred consistency failure cannot expose a
partly active version.

Approved canonical registry entities, source-reference rewrite workflows,
entity/onboarding APIs and full lifecycle portability remain pending. Phase 16
activation/evaluation and the cumulative `TestPhase15` criteria are not claimed.
