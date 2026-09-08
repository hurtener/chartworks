# Private topic drafts v1

Status: bounded phase 15 implementation, 2026-09-07. This implements private draft
revision storage and a real HTTP/SDK consumer; it does not complete phase 15 or 21.

## Admission and immutable history

`internal/semantics/drafts.Service` compiles the existing `TopicPack` and resolves
every dataset through `engineering.Service.Evidence` and live
`sources.Service.Discover`. The pinned profile must be complete, active in its
private actor/session profile head, and match source/context/dataset/revision/digest.
Declared physical column names, native types, categories and nullability must match
that profile and the actual discovered schema. No caller-supplied authoring field
is a verified binding proof. Admission calls neither a model nor query execution.

The service issues an unexported-content, ten-second `Prepared` admission value;
only that service can construct one. PostgreSQL rechecks the current source
revision, private complete profile digest and active profile head under transaction
locks. Rotation, erasure or profile replacement before commit fails admission.
There is no claim that an external warehouse cannot change after its catalog probe;
publication and later execution need their own source checks and fences.

Migration 012 stores tenant-composite heads, immutable versions and exact dependency
rows. Numeric revisions start at one; `expected_revision: 0` creates a topic and
positive values edit its exact current revision. CAS contention has one winner.
Each snapshot also has its unique authored `TopicPack.Version` and canonical digest;
reusing an authored version or a stale CAS returns conflict. No automatic retry or
idempotency replay is implied. The snapshot, dependency rows, head and
`topic.drafted` audit event commit atomically. Actor/session, revision, timestamp,
digest and bounded change note are retained. Database triggers reject snapshot or
dependency update/delete. There are no published pointers or facet writes.

## Signed authority and privacy

The ten draft operations in the [shared topic operation manifest](chartworks-topic-draft-operations.json) use the existing
Pengui verifier and signed action/resource model. The provider-registration contract
lists their exact action strings. Creation requires tenant write, topic write and
all source read, dataset query and execution-context use reaches. Editing requires
both the prior current draft's and proposed draft's complete dependencies. Live
admission also uses the existing source/profile actions (`sources.read` and
`engineering.read`). The caller supplies no tenant, actor, session or authority DTO.

Read/history/diff require `topics.read` and topic read; export separately requires
`topics.export` and topic export. Both require every persisted source read, dataset
query and execution-context use reach. Tenant, actor/session ownership and all
reference restrictions are SQL predicates applied before selecting manifests or
history metadata. A creator without current signed reach is denied. Revisions
remain private because their underlying phase 12 evidence is private; later public
publication must establish its own appropriate provenance/health boundary.

Retained reads never call a source or model and do not label a draft healthy or
executable. A replaced profile or rotated source does not rewrite history; current
signed reach to the retained context is still required. Erased profile evidence or
a deleted source makes affected revisions inaccessible, including history. This
access fence does not claim to implement topic erasure/retention or publication.

## Wire behavior and limits

Ten draft routes use `internal/api.Registry`, `SchemaFor`, the same generated
OpenAPI data and the Go SDK: save, import, onboarding, entity mutation, dataset
rebind, current draft, exact revision, history, diff and export. The runtime
composition root installs `topicapi.Handler`. Request
DTOs have lower-snake-case JSON fields; absent fields take their Go zero values and
then undergo domain validation. Unknown/duplicate fields, trailing JSON, wrong
scalar types, query strings, content encoding and bodies on GET are rejected.
Only `application/json` without media parameters is accepted. Bearers are verified
before decoding; responses are `no-store`. SDK calls obtain current supplied bearer
credentials and perform no automatic CAS retry.

Canonical topic/portable documents retain the compiler's 1 MiB limit; HTTP request
and ordinary SDK response caps are 2 MiB. A diff can expand two compact input packs
into per-entity hashes; only that concrete operation has a 16 MiB SDK response cap,
covering the compiler-bounded entity/identifier maxima. Responses exceeding their
operation cap still fail through the existing bounded SDK reader. The compiler
bounds all collections before sorting.
Source/profile admission and commit have a ten-second deadline further bounded
by bearer/request expiry; the initial exact-prior-revision read uses the existing
short metadata deadline.
History is descending, metadata-only, at most 32 entries with an exclusive numeric
`before` cursor; zero starts at the latest eligible revision. Exact read/export/diff
require positive revisions. Storage caps each topic at 128 revisions and each
tenant at 256 private topic heads, serialized at the tenant metadata lock. This
bounded stage offers no deletion to reclaim those limits.

Malformed definitions/mappings return `invalid_request`; missing/inaccessible
coordinates return `not_found` (history can return an empty list). Missing actions
return `forbidden`; source evidence mismatch returns `context_changed`; CAS/version
reuse returns `conflict`; limits return `limit_exceeded`. Canonical proposals with
a skipped revision, changed meaning at an existing revision, or a normalized term
reserved to another entity return `conflict`. Unknown dependency failures expose only
`unavailable`; cancellation/timeout has a typed response. Audit failure rolls back
all draft writes.

## Neutral portability and outstanding work

Export loads an authorized exact revision and invokes `ExportPortable` with explicit
logical slots. Its DTO structurally excludes installation coordinates, physical
names/native types, actor/session and credentials. Free authoring text remains text,
not automatically sanitized content. Import invokes `ImportDraftCandidate`, then
the same complete admission and persistence path as ordinary save; it creates only
a private draft. Synthetic round trips preserve semantic meaning and exact bindings.

Canonical entries in a private draft remain proposals. Admission checks them against
the tenant registry but neither creates a registry row nor treats the supplied revision
as approved. Revision one for a new ID, the exact next revision, and exact immutable
revision reuse are admissible proposals; reviewed publication owns approval. Physical
keys remain in the topic and mapped import may rewrite them without changing the global
meaning digest.

Profile onboarding derives one unresolved dataset scaffold from exact active private
profile evidence. It copies safe discovered columns into stable semantic column IDs
and proposes no measures, dimensions, KPIs, joins, rules or canonical meanings.
Entity mutation applies a bounded batch of puts/deletes to a detached prior model;
the compiler rejects duplicate IDs and any deletion or update that strands a
reference. Dataset rebind requires an explicit complete mapping from every stable
semantic column ID to a safe physical column in one active target profile. The
service derives source, context, dataset, source revision, profile digest, physical
type, category and nullability, rewrites all dataset-qualified references, and saves
the result through the same prior/new reach checks, discovery and commit fences.
Each successful operation creates one immutable CAS revision; failures leave the
head unchanged. These operations add no migration and reuse primary `topics.write`
plus the established secondary `engineering.read` and `sources.read` actions.

The separate [publication contract](topic-publication-v1.md) now owns review,
publication/facet activation, retained publication reads, rollback/archive and the
current published-topic source contract. Public source-health/recheck, bounded
resumable gateway generation, retention and full lifecycle bundle portability remain
pending. Onboarding evidence stays private; published health continues to use source
discovery and exposes no profile ID. Phase 16
execution/replay/shadow/provider quality remains pending. Phase 21 still needs
foundation/work/security adapters, public document delivery and cumulative
acceptance. No full `TestPhase15`, `TestPhase16` or `TestPhase21` pass is claimed.
