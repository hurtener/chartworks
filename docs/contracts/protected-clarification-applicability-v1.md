# Protected clarification applicability

A reviewed clarification may be triggered by wording that is also a sensitive
answer. Redaction must not retain that wording merely to make future applicability
checks work. This extension retains bounded business evidence for the original
match, without treating it as source authority or a caller-supplied resumption
credential.

## Producer and immutable write

After current topic, rule, actor/session and source-context admission, routing
records only the matched topic/pattern/version coordinates, term ordinal and
specificity that disappear from the sanitized question. No original question or
matched term string is stored in the witness. The same bounded term matcher drives
fresh evaluation and term-coordinate validation. Public redaction markers cannot
supply term matches or join independent words into a matching phrase.

The witness binds the actor/session digest, exact publication/rules/source pins,
answer context, sanitized request and resolution digests, and the complete sanitized
route digest. A domain-separated digest commits to the exact original question,
actor/session, execution context and locale; the raw question is never retained.
This fingerprint is also issued when a public trigger survives redaction and no
hidden term coordinates are needed. Its producer seal is private Go state and is never encoded in JSON.
Before creating a query, the execution service binds that seal to the new query ID.
The PostgreSQL insert path rejects witness metadata without this exact seal. A
continued witness also names its actual immutable parent and parent-witness digest;
the insert transaction checks that ancestor under tenant, actor and session scope.
All insert paths use this guard, including pending-generation records.

## Authenticated read and replay

The trusted server constructor wires an authenticated repository reader into its
own routing service instance. The reader projects the stored route for one exact
query ID, restricted by verified tenant, actor, session, current operation and
signed execution-context reach. It reads neither SQL nor result rows, and it grants
no additional source permissions. There is no public request field for supplying
a reader, restoring a custody flag, or installing term coordinates.

The router verifies the fetched record, immutable route digest, actor/session and
query identity before replaying it through current topic/rules/source admission.
Only then may it install private request-local custody for one exact next request.
Ordinary JSON deserialization cannot create that custody. A plain serialized route
with private-only term evidence therefore cannot authorize its own replay or a new
query write.

## Replies, refinements and saved copies

A reply retains the form pin issued by the protected preflight and may submit its
canonical question or its byte-exact original question. The original is checked
only against the fingerprint from the authenticated immutable query record.
Different raw wording is rejected even if its private literals redact to the same
visible marker. A caller-supplied fingerprint cannot authorize a transition.
Fingerprint-only serialized routes may independently replay their public semantic
applicability, but cannot restore original-question custody or authorize a new
protected write. The private transition binds the actual query origin
and next request, including its current answer digest; mutating an answer after
that transition was admitted invalidates it. Refinement may replace typed answers
through the existing same-session path, but cannot transplant the seed to changed
question/policy/source context.

A saved copy first reloads and replays its exact protected parent, then receives a
new producer/query binding. Only the query-origin/ancestor coordinates change in
saved semantic equality; all other witness and route meaning remains identical.
The write and later derivation checks require the exact parent's witness digest.
Stored execution and terminal replay still require their ordinary current source
admission, native validation, owned predicate binding and operation receipts.

## Evidence and limits

`TestProtectedApplicabilityCustodyAndTransitions` covers producer/JSON separation,
exact-reader matching, private-question substitution, request mutation, source
rotation and saved-copy resealing. `TestClarificationTermContextDoesNotComeFromJSON`
checks that descriptive term coordinates do not enter the evaluator through JSON.

`TestProtectedApplicabilitySDKStoredReplayAcceptance` crosses the real SDK/HTTP,
PostgreSQL record, source execution, saved-copy and refined-query paths.
`TestProtectedApplicabilityNumericAndNullSDKAcceptance` adds exact numeric and NULL
oracles. These recorded-provider fixtures are not live-model evidence, and test
names alone do not qualify the implementation. Exact-source runtime results are
required before claiming these paths pass. Migration 082 updates both saved-copy
SQL guards to compare all route fields except the three resealed origin coordinates,
requiring the exact parent/child query identities. Legacy routes retain exact JSON
comparison. It preserves existing rows and all prior migration bytes. No permission
scope is introduced.

## Durable generation questions

Before a model clarification is persisted, the execution service sanitizes its
question, instructions, examples, and explicitly retained refinement input using
the same admitted masking surface. Transient masking inputs are detached and
private; JSON, public hashes and formatting do not carry them. New pending rows
carry an optional versioned fixed-input commitment bound to actor, session and
query ID. It retains only digests of the original submission and immutable input
projections. Canonical settings or the exact committed original may resume;
original private question text additionally requires the authenticated pending
row's original-question fingerprint. Different text sharing a redaction marker
cannot satisfy that commitment. Omitted commitments preserve legacy digest bytes.

Correction questions authenticate and reissue custody from their actual immutable
SQL parent before atomic terminal-parent/pending-child persistence. Repeated
pending rounds use the real pending origin for custody and durable parent pins;
the refinement request separately retains its SQL ancestor. Reviewed replacements
retain their Legacy parent and separately pinned Preflight origin. The guarded
INSERT accepts that distinct applicability origin only with a private service-issued
review seal, exact current preflight revision and lineage digest, matching context,
and the canonical parent witness hash. Fresh JSON review evidence cannot supply
that seal. Existing actor/session/source admission and all ordinary parent checks
remain mandatory.
