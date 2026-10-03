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

The separate [publication contract](topic-publication-v1.md) owns review,
publication/facet activation, retained publication reads, rollback/archive and the
current published-topic source contract. Public health is a retained snapshot;
explicit recheck uses source discovery and exposes no profile ID. The bounded
enhancement operation classifies at most 32 stable column coordinates per request,
persists its cursor and gateway receipt with the resulting immutable draft revision,
and retains unresolved outcomes through publication projection and neutral
portability. Phase 16 execution/replay/shadow/provider quality remains separately
owned. Live provider quality is not inferred from recorded gateway fixtures.

## Evidence-bound authoring and whole-candidate advisory

Enhancement checks the complete topic dependency reach, active profile digests,
and current discovered source revision/schema before any gateway request. Its
mandatory context includes the exact business definition, existing semantic
entities/aliases/temporal policies/grouped-population policy, confirmed and proposed
relationships (including composite keys), and profile origins, policy digests,
observation time and sampling limits. Only aggregate observed/null/distinct counts
are supplied; reviewed non-sensitive columns may also carry closed type-family
counts. Raw rows, ranges, latest values and optional profile-summary prose are not
supplied. Governed values and filter literals are withheld unless they match an explicitly
admitted non-sensitive vocabulary; filter structure remains visible. These omissions require human adjudication and do not
silently remove filters or governed values from the retained candidate.

Context is canonical, digest-bound and limited to 1,024 columns and 128 KiB per
model input. Exceeding a mandatory bound fails before that call rather than
truncating the candidate. Relationship proposals may cross the 32-column page
boundary through the complete authorized column catalog; one endpoint must be in
the current page. New relationship provenance is assigned by the service to the
exact authoring-context digest. Proposals do not become executable joins, and a
sample never proves key uniqueness or grants authority.

The final page performs a bounded `topic_review` gateway call against the entire
new candidate. A closed advisory binds the exact candidate/context/coverage digests
and lists every reviewed entity. Findings cover ambiguity, aliases, grain,
temporal coherence, unresolved relationships/columns and incomplete evidence.
Deterministic collision, missing-definition, unresolved and redaction checks are
retained alongside model findings. The model cannot return edits or publication
approval; even `no_findings` is not a correctness proof. Any provider, output,
coverage, source-drift or budget failure leaves the prior draft/checkpoint intact.
The optional, independently routed `topic_review` role must be explicitly enabled;
enhancement preflights it before any model call and fails closed when disabled.
The reference configuration leaves it disabled. Each generation/review call has a one-call, 64K-token, 30-second cap; the final page
therefore permits at most two calls, 128K reserved tokens and 60 seconds of model
work, still subject to request and bearer expiry.

Migration 067 retains the input digest and advisory atomically with the immutable
generation checkpoint. New generated completions require a matching advisory for
explicit human approval; incomplete generated checkpoints cannot be approved.
Manual and legacy drafts retain their explicit human review policy without
fabricated model evidence. A later ordinary edit is a new manual draft; a prior
report never becomes evidence for its changed digest. Human review and publication
remain separate signed operations. SQL-example feedback does not supply topic
meaning or authorize semantic changes.

`TestAuthoringContext*`, `TestAuthoringRelationshipCatalogCrossesPagesWithoutWidening`,
`TestEnhancementPreservesWithheldProtectedMeaning`, `TestWholeTopicReview*`, and
`TestPhase15/AC05` exercise the bounded transport and lifecycle with synthetic
profiles and recorded gateway responses. These checks do not establish live model
quality or end-to-end numerical SQL correctness.

Generated advisories also require every pinned profile to remain the active head
with its exact digest, even when source revision and schema are unchanged. Approval
and pre-model publication enforce that condition; publication repeats it under
profile-head share locks through commit. Historical report reads remain immutable.
After profile replacement, an explicit rebind and fresh complete generation review
produce new evidence; old review receipts cannot publish the stale generated draft.

## Explicit authoring vocabulary and population proposals

`EnhanceRequest.vocabulary` is optional on the existing HTTP/MCP/SDK enhancement
operation. The Go SDK exposes `TopicAuthoringValue`. Each entry contains `id`, an
exact column `field`, the complete matching `origin` source/profile reference,
`kind: text`, `value`, bounded aliases, `sensitivity: non_sensitive`, and
`nulls: exclude`. At most 32 entries and 32 KiB are admitted. Values are at most
256 UTF-8 bytes; identifiers, aliases, duplicate or ambiguous spellings, source
origin and text-column types are checked. The referenced column must already be explicitly classified non-sensitive in the
draft; unknown and sensitive classifications are rejected. The existing CAS draft
save supplies a separate, explicit author privacy annotation. A value-only assertion
never silently reclassifies the entire field. Source profiles currently have no separate sensitivity
classification; profile range permission is never reused as permission to disclose
sampled values. Inputs are explicit author assertions, not observed membership.

The first generation step seals a canonical catalog in migration 070's immutable
checkpoint. Omitted catalogs on later pages inherit it; a supplied different
catalog conflicts before any model call. Source/profile drift still fails admission
and generated review/publication. Existing requests without a catalog remain valid,
but no literal population proposal can be resolved without an authorized mapping.

The model may return `filter_proposals` selecting vocabulary IDs for a current-step
measure and `value_proposals` selecting IDs for a current categorical dimension.
Text `eq` or finite `in` with NULL exclusion is supported. Same-dataset is the default; the bounded relationship exception below requires an independently proved v9 lane consumer. The
server resolves strings and seals `author_input` provenance; model-supplied SQL,
new strings, unknown IDs, cross-field mixtures and changed NULL semantics are not
admitted. Existing protected filters and values cannot be silently overwritten.
Explicitly admitted values can reach whole-topic review; other literals remain
withheld and generate an incomplete-evidence finding.

An optional `group_domain` proposal uses only `metric-group-domain-v1` with
`raw_source_groups` or `qualifying_population`. Existing policy cannot be changed
by enhancement. Choosing the domain needs business meaning; the model's proposal
is still private material requiring exact whole-topic review and explicit human
publication approval. Neither a vocabulary entry nor a successful SQL run approves
topic meaning. Schema/profile-only and vocabulary-assisted generated evaluations
are separate corpus conditions; expected numeric results are not authoring input.

### Supplemental counts and reviewed KPI periods

Enhancement may return at most eight `count_proposals` for current-page fields.
Each supplies a name, description, aliases and unit, while the server derives the
stable field-bound measure ID and fixes its aggregation to `count`. This is
`COUNT(column)`: NULL values do not count, zero values do, and repeated identifiers
are not deduplicated. A nonnullable row-identity count can therefore differ from a
nullable amount count. The proposal preserves the column's original SUM or
dimension and cannot select a new source, field, ID, DISTINCT operation or SQL.
The admitted metric catalog labels the supplemental ID with its exact field and
aggregation. Existing same-dataset vocabulary filters can target that ID. Normal
whole-topic advisory, explicit human review and publication remain required.

A KPI may optionally carry `periods` with policy `metric-period-bindings-v1` and
one to four exact `{measure, dimension}` bindings. Bindings cover every transitive
leaf measure exactly once, point to reviewed temporal dimensions, and agree on
one axis for all leaves of the same fact. A parent KPI cannot override an
already-reviewed nested KPI's period. Cross-dataset axes need a confirmed
nonmultiplying relationship and later physical proof. Cohort/order time and
refund-activity time are separate business meanings. The metadata contains no
SQL, year, literal bounds, permissions or inferred business definition. It is
canonicalized, digest-bound, cloned, carried through portable drafts, and included
as dependency evidence without becoming an output, grouping request or selected
metric root. Missing metadata preserves legacy definitions but grants no default
time-basis interpretation to a new lane consumer.

### Exact relationship-bound vocabulary filters

An optional `filter_proposals[].join_id` may select one already-confirmed direct
INNER relationship from the measure fact (left) to the vocabulary field's dataset
(right), with many-to-one or one-to-one cardinality and retained review evidence.
Unknown, candidate, rejected, inverted, indirect, multiplying and LEFT paths are
not admitted. The value remains caller-supplied, non-sensitive, origin-bound
authoring input; a relationship does not authorize disclosure or invent membership.

The exact join ID persists as `SemanticFilter.relationship`, survives cloning,
portable drafts and immutable digests, and enters dependency evidence. It cannot
be silently dropped or changed during enhancement. Generic/retained consumers
must fail closed; only the versioned scalar lane consumer may admit it after
checking exact placement and physical nonmultiplication. New vocabulary admission
does not itself grant execution, topic publication or broader source access.

### Known-amount completeness obligation

A numeric SUM may declare `completeness` with policy
`known-amount-with-unknown-count-v1` and an exact `unknown_count` KPI reference.
The companion must be exactly `COUNT(nonnullable fact identity) - COUNT(the
same amount field)`, with COUNT rather than DISTINCT and the same canonical
mandatory filter population, including exact relationship identity. Cosmetic
filter IDs do not change that population. The companion has no extra KPI filters
or independent period mapping: it inherits the owning measure's current query
population and grouping. Independent cohort/activity companions retain their own
period definitions and cannot be silently reused or overridden by this link.

The dependency graph is acyclic: SUM points to the companion KPI, which points to
two separate COUNT measures and their columns. COUNT measures cannot themselves
carry completeness links. The metadata and reference survive cloning, portable
transport, immutable hashing and dependency expansion; expansion alone does not
select another output. A later execution consumer must select the exact required
companion and bind metric identities to result columns using proof-issued output
evidence. Alias/ordinal guessing, NULL/missing companion values, and truncated
results cannot establish amount completeness. This is distinct from chart/result
truncation completeness and does not claim full-source coverage.
