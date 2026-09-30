# Durable generation questions v1

A blocked generation decision is retained as a protected, non-executable query.
The existing error includes redacted questions, query ID, answer context, expiry,
a `resume` discriminator (`plan` or `refine`), and a bounded catalog of exact
reviewed measure, KPI and dimension IDs. The model supplies only questions; the
service derives choices from current admitted publications. Ambiguous cross-topic
IDs are omitted; an oversized catalog is withheld rather than silently truncated.
These IDs grant no authority.

## Answer origin and existing typed consumers

For `plan`, submit the original Plan request and the same fixed question/settings.
Provide `generation_query`, `generation_context`, and operation `resume:<query_id>`.
The complete reviewed reference selection, existing typed rule answers/choices,
reviewed grouping or interpretation edits may resolve the question. Existing form
pins, source/context, locale, free text and model instructions cannot be substituted.
The router revalidates exact rule/choice/value origin and all current semantic
constraints before generation; model-authored prose creates no choice authority.

For `refine`, resend the original Refine request, replacing `query_id` with the
pending ID and supplying both generation-origin fields. Existing typed parameter,
reference, metric, governed-answer, grouping and interpretation edits can change.
The service reloads the protected original Refine request and exact SQL-parent
lineage, reauthorizes it, and reruns the ordinary Refine pipeline. Private model
slots are restored through their existing native role guard, including requested
typed replacements; canonical business values stay under the existing binder.
Neither private values nor model parameters are exposed by the question catalog.
The client cannot supply new free-text instructions through the answer path.

A ready child pins the immutable pending parent revision/digest and an immutable
hash of the exact typed submission. Reusing the operation with another answer
conflicts before inference. Run the child with `resume:<pending_id>` as its
operation; Run cannot replace that identity and erase resumption deduplication.
A separate question starts an independent ordinary generation lifecycle.

## Correction and persistence boundaries

When a confirmed physical rejection's bounded correction asks a question, the
PostgreSQL repository atomically commits the terminal failed query and its new
pending question. The original attempt is never automatically restarted. The
pending parent digest is bound to the exact post-update database representation
within that transaction. A duplicate question insert rolls the parent update back.
A restarted terminal Run returns that same pending origin with no model/read work.
The user explicitly resumes through Refine, then explicitly runs the new child.
Cancellation, uncertainty and other non-repairable failures cannot open this path.

Pending evidence and resolution hashes are immutable database columns. Tenant/actor
predicates and session checks protect every read. Original parent revisions/digests
are rechecked; a subsequent mutation cannot silently change retained private intent.
Pending SQL/results are absent, and Run rejects non-executable records. No bearer
or provider credentials are retained.

## Bounds and surfaces

Pending origins expire after fifteen minutes. Every request requires current signed
authority for the actor/session, topics, source, datasets and execution context.
Current publications, exact source binding and reviewed scope must still match.
A repeated operation returns the same pending decision without generation; resumed
requests use the existing cross-process operation lock. At most three question
rounds are admitted in a pending lineage, with model usage retained. Expired, stale,
foreign and mismatched origins fail before model work.

HTTP, MCP and SDK use the same reflected requests and generation problem. The SDK
bounds and validates both redacted text and the reviewed catalog. No new operation,
identity policy, executor or model role is introduced. Deterministic reviewed typed
answers are not a claim of arbitrary free-text answer understanding or live
ambiguity calibration.

## Regression evidence

Unit families: `TestGenerationPending*`. Native/PostgreSQL cases:
- `TestSQLRecoveryDurableGenerationQuestionsAcceptance`: restart, cold concurrent
  resume, immutable origin, tenant/actor isolation, changed-answer rejection,
  execution and zero-model replay
- `TestSQLRecoveryDurablePrivateRefinementAcceptance`: English/Spanish private slot
  custody, typed replacement, exact source records and replay
- `TestSQLRecoveryDurableCorrectionQuestionsAcceptance`: confirmed physical error,
  atomic pending/finalization, rollback, restarted question replay and explicit child
- `TestSQLRecoveryDurableGovernedAnswerAcceptance`: private rule-authored values
- `TestSQLRecoveryDurableCalendarGroupingAcceptance`: reviewed calendar grain

Recorded-provider tests establish these lifecycle boundaries, not live model quality.
Exact-source integrated qualification remains owned by the completion tracker.
