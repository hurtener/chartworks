# Explicit semantic feedback proposals

A proposal is private authoring evidence. It does not approve SQL, change a
publication, change source reach, or activate learning examples.

## Concrete workflow

1. Submit ordinary governed query feedback. Use `listTopicFeedbackEvidence`
   with the owned query ID to discover exact negative/corrected feedback IDs.
2. `proposeTopicFeedback` accepts a proposal ID, topic, exact feedback ID,
   current private draft revision and explicit current `author_intent`, optionally
   with an explicit caller-supplied `vocabulary` catalog.
   The `enhance` gateway role sees the current admitted authoring packet and
   that explicit intent. Historical questions, SQL, notes, bound parameter
   values and result rows are excluded. The packet's existing profile and
   literal disclosure rules still apply.
3. `readTopicFeedback` returns exact proposed edits, digests and provenance.
4. `applyTopicFeedback` accepts an exact proposal ID and digest. It creates an
   ordinary immutable private draft and marks application in the same metadata
   transaction. It carries no inherited generation-quality approval.
5. Existing explicit topic review and publication activate the new definition
   and matching facet generation. Subsequent generation consumes that reviewed
   definition through the existing semantic context assembler.

HTTP routes are POST `/v1/topic-feedback/evidence`, `/proposals`, `/read`, and
`/apply`. MCP bindings and public Go SDK methods use the same domain service.
The caller needs existing signed authoring and feedback actions plus complete
current topic/source/dataset/context reach; this adds no new grant mechanism.

Report-origin proposals use a detached content-free evidence snapshot. Report
or dashboard deletion erases its owned SQL, feedback notes, parameters and
results normally, and writes a separate origin-erased tombstone in the same
transaction. The immutable proposal retains only independently supplied author
intent, current catalog-derived semantics and content-free provenance. Pending
tombstoned proposals stay inspectable but cannot apply or silently reanchor.
Already applied/published independent drafts retain their normal lifecycle.
Creation, application and document deletion share a metadata fence. The current
bounded proposal origin is a single-topic query.

## Closed edit surface

Existing measures may change descriptions, aliases, aggregation and selection
of their existing filter IDs, and add `new_filters` selecting only admitted
vocabulary IDs through text equality/inclusion with NULL exclusion. Each new
filter binds to the edited existing measure and cannot overwrite a filter ID.
Existing dimensions may change descriptions,
aliases, existing temporal policies and selection of their existing filter IDs.
Categorical dimensions may append admitted `vocabulary_ids` for their exact
field, preserving fixed `author_input` provenance; existing values cannot be
replaced or overwritten.
Existing KPIs may change descriptions, aliases, existing metric inputs,
closed two-input arithmetic expressions, and selection of existing filter IDs.
Expressions use exactly two existing input IDs joined by `plus`, `minus`,
`times`, or `divided by`. There is no SQL output field.

The explicit catalog is independently author-supplied, not mined from old
feedback or warehouse samples. Each text value binds to an exact current
source/context/dataset/profile revision and field; sensitive or unclassified fields, foreign
origins, ambiguous aliases, unknown IDs, types other than text, and unsupported
NULL policies fail closed before disclosure. Unknown field sensitivity requires
an explicit prior ordinary draft classification; a vocabulary value cannot
classify the whole field. Current live profile admission
runs before inference, after inference and before application. The protected
proposal snapshot and request/proposal digests bind the exact input catalog.

Literal values are not model output. Selecting an existing filter ID retains
its exact protected values server-side. Source bindings, physical columns,
entity creation/deletion, permissions and arbitrary expressions are outside
this proposal surface. New concept creation uses normal reviewed topic
onboarding/enhancement or explicit authoring. Selecting/removing a filter is
only a proposal until the ordinary publication review is completed.

## Persistence, concurrency and limitations

Migration 069 retains tenant/actor/session-owned detached proposals and content-free origin tombstones, exact current
publication and query revision, source binding/evidence digest, private draft
revision/digest, author-intent request digest, candidate digest and model usage.
A repeated completed proposal ID with the same request returns its retained
result without inference. A conflicting request fails. Concurrent first calls
may both reach inference; only one immutable proposal wins and neither can
produce an extra draft. Apply atomically fences the stored candidate,
publication, query revision, source/profile continuity and draft revision.
A successful apply retries to the same retained draft. Stale or foreign
origins fail rather than acquiring new reach.

Proposal creation is explicit assisted authoring, not autonomous extraction of
meaning from opaque correction SQL. Execution success is not correctness
approval. Historical examples remain bound to their original topic versions;
publication does not rewrite their origins. Requalification of old examples
is a separate freshly proved candidate/review operation described in
[example-requalification-v1](example-requalification-v1.md).

## Acceptance

`TestTopicFeedbackProposalLifecycle` uses a recorded provider with real
PostgreSQL metadata/native-source boundaries: protected feedback discovery,
private-value exclusion, idempotency, foreign/stale/digest negatives,
ordinary draft creation, explicit review/publication, and later generated
context plus numerical result. This is not a live-provider quality claim.
`TestTopicFeedbackVocabularyLifecycle` additionally creates an admitted new
measure population filter and verifies its later generated native result.
Unit tests check closed edit/schema behavior, vocabulary negatives and
physical-origin preservation.
