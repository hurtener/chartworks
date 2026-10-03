# Request-bound whole-topic quality review

A whole-candidate advisory is bound to one immutable candidate, authoring context
and complete coverage list. Its request-local domain schema admits only the exact
three digest values and exact coverage identifiers. Every finding reference must
include the canonical kind prefix and, for columns, the encoded dataset/column
pair. Bare IDs, display names and dotted shorthand are not alternate identities.

The schema template is immutable and parsed into a fresh private document for
each request. The canonical domain schema continues to bind those exact identities.
A subsequent live provider restriction rejected embedded quotes in strict enum
literals, including the quotes in canonical JSON column coordinates. The quality
role therefore exposes a complete `entity_references` array of `{handle, entity}`
pairs. Provider choices are exact deterministic request-local handles such as
`e000000`, not new semantic identities. The input names the encoding version and
the canonical domain-schema digest; both the mapping and digest are included in
the complete measured request.

The gateway validates and measures the strict handle schema. The quality boundary
then validates the handle-only response, restores each canonical reference using
only this request's map, and independently validates the original canonical
schema before the existing digest comparisons and `QualityReview.ValidFor`.
Unknown handles, raw canonical references on the handle wire, stale bindings and
references absent from the original domain schema fail closed. Findings, status
and detail are otherwise unchanged. Handles are never persisted as entity IDs.
A handle reused in another request does not survive its exact digest bindings.

The complete list is never truncated or replaced with unconstrained strings to
fit provider enum, schema-byte or context limits. These limits fail closed before
inference; no model budget or authority is increased. Other gateway roles are not
rewritten: inspected dynamic concept IDs are hashes and visual candidate IDs are
restricted identifiers; the enhancement/SQL/engineering/feedback/narrative enums
are closed quote-free vocabularies. This is a bounded quality-role compatibility
correction, not a claim about every provider's full JSON Schema implementation. Invalid provider advice cannot save the final draft page or
replace the preceding durable checkpoint. Validation errors preserve the gateway
error class and disclose only a bounded stage, not identities or model content.

## Evidence and judgment

Candidate descriptions express declared business meaning. They are evidence of
what currency, population, grain or calendar policy is intended, not proof that a
physical key is unique or a relationship has an asserted cardinality. The review
prompt explicitly separates these claims, reads the actual unresolved list and
asks for specific contradictions or missing evidence. A physical column and its
own semantic projection sharing an alias is not, alone, competing business meaning.
Withheld literals, absent required physical guarantees and genuinely unresolved
meaning still require review. This prompt change does not prove model fidelity.

A valid `needs_review` advisory remains a retained advisory, with every finding
intact. An authorized operator can adjudicate it through the existing exact-digest
review and publication operations. Neither `needs_review` nor `no_findings`
grants approval or signed access. The synthetic acceptance case exercises this
existing operator path only after its independent business oracle accepts the
unchanged candidate, and then checks gross and calendar-month results against
independent native PostgreSQL oracles. It does not auto-approve arbitrary topics.

## Reproduction and qualification

A captured synthetic live review supplied the correct candidate, context and
coverage digests but used 13 unqualified entity references in five findings. The
former static schema accepted those strings; the domain consumer rejected them.
An offline counterfactual changing only those references made the same five
findings and `needs_review` status valid. The generated candidate independently
passed the synthetic business contract. This isolates the identity contract
failure from the separate quality of advisory wording.

Unit regressions cover exact canonical-domain and provider-handle enums, quote
and Unicode preservation, unknown handles, all digest drifts, bare and shorthand
references, cross-request concurrent isolation, immutable inputs, original-domain
validation after mapping, actual envelope accounting and provider/schema bounds.
A recorded SDK fixture reproduces the observed quoted-enum HTTP400 before checking
the successful handle-encoded role call; strictness is never disabled. Real SDK and
PostgreSQL tests cover the serialized bound schema, durable rollback, retained
advice, explicit operator adjudication and the two result oracles. Hosted required
case lists include both new acceptance roots. Run evidence belongs to an exact
source checkpoint; these test descriptions do not themselves establish a pass.

The [fixed live baseline](../reviews/generated-topic-live-qualification.md) now
passes generated-topic publication and two SQL result oracles while preserving
one operator-adjudicated advisory. Broader live quality, engine coverage,
charged-memory renderer qualification and final release gates remain open.
