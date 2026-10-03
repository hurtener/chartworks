# Current eligible learning retrieval and negative observations

The generation-only PostgreSQL reader applies exact current topic, context,
source-binding, locale, rules and template provenance before selecting at most 64
examples. Rows must already be active, reviewed and sufficiently supported.
Candidate/stale rows cannot consume that budget. Current service-owned templates
are eligible only when the current request independently supplies their owned
constraints. Every returned row still passes the existing service applicability
and typed-template checks; retrieval creates no execution or publication authority.

The first bounded phase ranks matching simple-dictionary query terms. A second
phase fills any remaining room from eligible nonmatching examples by existing
weight/evidence ordering. Both phases are parameterized, tenant/topic scoped and
indexed. This is query-aware lexical retrieval, not a claim of embedding/paraphrase
quality. The subsequent Bayesian/Jaccard ranking, optional gateway rerank, final
context fitting and actual-use receipt remain visible. New metadata identifies
`current-eligible-fts-bayes-v2`; a too-large optional token expansion explicitly
uses `current-eligible-weight-fallback-v2`. Legacy repository adapters retain their
old policy marker. Public review listings remain a separate API.

A negative observation without a supplied correction remains recordable when
current source SQL validation fails, after ordinary authorization and current
source/topic admission. It is atomically idempotent and creates no example effect.
Positive feedback and supplied corrections still need fresh native SQL proof.
Cancellation, access denial and source-origin drift remain errors; they are not
silently converted into accepted evidence. Valid negative SQL feedback retains
its existing bounded example-evidence effect. This does not automatically rewrite
semantic meaning, activate an example or approve a topic.

Tests distinguish a durable negative receipt from executable/learned authority,
and reproduce pre-limit starvation with 65 candidate rows, 65 stale active rows and
65 current high-weight nonmatching rows before the relevant lower-weight example.
The acceptance gate requires that relevant example to reach the actual rendered
prompt and independently validated execution.

Publishing a new semantic revision does not rewrite retained example origins.
An example reviewed under an older topic remains visible in the review listing
with its old origin, but is ineligible for current generation. Explicit requalification now creates a separate freshly proved candidate under
[the versioned requalification contract](example-requalification-v1.md). It
preserves the earlier origin and still requires a separate activation review;
semantic publication alone cannot supply that proof.

The v2 generation selection receipt records the current eligibility origin digest
and whether current owned predicates admitted owned templates. Rows filtered out
before the bounded search have not been individually read: the receipt does not
invent per-example exclusions or counts for them. Legacy reader receipts retain
their distinct individual exclusion evidence.
