# Complete reviewed intent replacement for retained queries

Retained analytical versions 1–6 can replay their original plan under the
original measured contract. A legacy model-owned filter, ordering or limit does
not acquire current authority merely because it was retained. Normal refinement
continues to reject such an upgrade with localized review guidance before model
or row execution.

For an explicit replacement, first create a current preflight containing the
complete intended question, reviewed dimensions, metric selections and grouping.
Submit the preflight's typed clarification answers with the legacy query ID:

- `query_id`: the retained legacy query
- `intent_review.query_id`: the current pending preflight
- `intent_review.answer_context`: that preflight's exact answer context
- `intent_review.answers`: explicit current typed answers

This replaces the complete intent. It never approves historical SQL, copies its
parameters or treats historical model choices as current reviewed values. Other
question fields and parameter/reference/metric edits cannot be mixed with this
request. A new reviewed preflight is required to change a submitted replacement.
Versions 1–6 use this answer-based path. Version 7 has only the specific grouped-domain transition below; no generic newer-version bypass is available.

The current actor, tenant, signed session, context, source binding and exact topic
and rule pins must match. The service rechecks the protected pending form and
reviewed answer domains before generation. Both protected origins are retained
with revisions and digests and checked atomically when saving the child. Replay
rechecks both origins and current authority. Stale origins fail closed.

Repeated identical submissions, including concurrent submissions and submissions
after process restart or execution, return the same initial child without another
model call. Changed answers on the same pair of origins conflict. Generation
clarification retains the existing bounded continuation contract. Normal native,
analytical, correction and execution validation apply to the new plan. The old
record stays unchanged and independently replayable.

English guidance: Review the complete current intent in a new preflight, then
submit its query ID, answer context and typed answers as `intent_review`.

Spanish guidance: Revisa la intención actual completa en una nueva consulta
previa; envía su identificador, contexto y respuestas tipadas en `intent_review`.

## Verification

`TestSQLRecoveryLegacyIntentReviewVersionMatrixAcceptance` checks native retained
proof/replay and the new replacement for each version 1–6.
`TestSQLRecoveryLegacyIntentReviewAcceptance` covers English/Spanish exact results,
private provider-wire boundaries, foreign actor/session/tenant and pending forms,
stale/changed answers, concurrent idempotence, native correction, process restart,
non-executable generation decisions and bounded resumption, immutable storage and
stale-origin replay denial. The focused suites run with real PostgreSQL and native
validation under the race detector. Model responses are recorded local fixtures;
these checks are not live-provider quality or cost measurements.

## Retained version 7 group-domain review

A retained grouped query with the dedicated `analytical_group_domain_review_required`
condition can be replaced from an independently reviewed current publication.
The old native-safe statement and exact historical receipt remain retained
metadata; this does not certify or re-enable the ambiguous old grouping.

Publish explicit per-fact `GroupDomains`, then create a ready, SQL-empty preflight
for the complete current intent. Submit `intent_review.query_id` and
`intent_review.selection_digest` equal to that preflight's `Route.Selection.Digest`.
Do not combine this variant with `answer_context`, answers or other edits.
The selection digest confirms an exact server-owned current catalog selection;
it is not permission to submit arbitrary domains or approve historical SQL.

Both the historical publication and current publication are independently pinned
and authorized. Their versions may differ only through this dedicated transition;
topic IDs, actor/session, execution context and full source binding still match.
Every current lane must have an explicitly reviewed domain. Stale origins fail
closed. The ordinary versions 1–6 answer-based flow retains its existing equal-pin
checks. `TestSQLRecoveryLegacyGroupedDomainReviewAcceptance` covers this transition
with real native-safe historical SQL and independently reviewed publications.
