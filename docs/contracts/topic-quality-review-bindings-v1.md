# Request-bound whole-topic quality review

A whole-candidate advisory is bound to one immutable candidate, authoring context
and complete coverage list. Its request-local domain schema admits only the exact
three digest values and exact coverage identifiers. Every finding reference must
include the canonical kind prefix and, for columns, the encoded dataset/column
pair. Bare IDs, display names and dotted shorthand are not alternate identities.

The schema template is immutable and parsed into a fresh private document for
each request. The strict provider projection and full SDK-style envelope measure
that same bound schema. The complete list is never truncated or replaced with
unconstrained strings to fit provider enum, schema-byte or context limits. Those
limits fail closed before inference; no model budget or authority is increased.
The existing digest comparisons and `QualityReview.ValidFor` remain independent
consumer checks. Invalid provider advice cannot save the final draft page or
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

Unit regressions cover exact domain and projected-wire enums, all digest drifts,
bare and shorthand references, cross-request concurrent isolation, immutable
inputs, actual envelope accounting and provider/schema bounds. Real SDK and
PostgreSQL tests cover the serialized bound schema, durable rollback, retained
advice, explicit operator adjudication and the two result oracles. Hosted required
case lists include both new acceptance roots. Run evidence belongs to an exact
source checkpoint; these test descriptions do not themselves establish a pass.

Live generated-topic-to-SQL quality, broader engine coverage, charged-memory
renderer qualification and final release gates remain separately open.
