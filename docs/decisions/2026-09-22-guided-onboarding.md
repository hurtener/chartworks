### D-084 — guided onboarding composes domain services behind a private durable ledger

Status: accepted, 2026-09-22. Owners: phases 33, 11, 12, 15, 19, 27 and 28.

Guided onboarding owns only an actor/session-private, CAS-fenced progress ledger.
Source registration/upload, discovery, profiling, topic drafts, independent review,
publication, query planning, block authoring and report authoring remain owned by
their existing services and authority checks. A run contains stable references,
bounded evidence, explicit unresolved choices and the next required action. It
contains no bearer, credential, SQL, result row or provider prompt.

Each resume advances at most one bounded stage using a deterministic operation key.
A committed domain effect is reconciled by its immutable reference after a crash;
the ledger never creates an alternate copy of that domain object. Semantic meaning
requires an exact independent review before publication. Query, block and report
suggestions remain private and uncertified until their ordinary lifecycle APIs are
used. A report suggestion copies no filter values; later report authoring resolves
selectable options through D-082's exact revision-bound option service. Managed
transformation requests stop for review and continue only through
the existing reviewed engineering path.

Source drift creates a new private, affected-only amendment reference and appends
it to the run. Active topic versions and approved analytical definitions remain
immutable. The caller must retain current signed reach to the run, source and
execution context; run IDs, creator identity and stored evidence grant no access.
Run reads and cancellation resolve the source's current context server-side before
returning any persisted progress. Drift closes over stable source, dataset, column
and semantic/proposal dependency coordinates; conservative private-intent impacts
are marked as such.

English and Spanish status/question text is bounded. Budgets cover stages, model
calls, tokens, entities and wall time. Cancellation stops future stages and retains
the progress receipt; it does not claim rollback of already committed effects.
Publication reserves the complete remaining call/token allowance before dispatch,
including multi-batch embeddings and uncertain crash receipts.
