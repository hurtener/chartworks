# Reviewed learning lifecycle v1

This contract closes LRN-01 and LRN-02 without creating a second authority or
model path. Learned examples are protected domain metadata. They may influence
generation only after current Pengui authority, current topic/source/context
continuity, exact semantic and rule pins, evidence thresholds, and explicit
review all succeed.

## Origin and lifecycle

Each candidate has schema version 1 and pins locale, topic version, execution
context, resolved source-binding digest, ordered rule versions, and reviewed
template selections. The question/SQL digest deduplicates equal candidates only
inside the same tenant and topic. A conflicting origin never merges merely
because its text matches.

States are `candidate`, `active`, and `retired`. Feedback creates or updates a
candidate. Activation is a separate `feedback.write` review with a nonempty
note, optional version CAS, at least one positive observation, posterior score
of at least 0.60, and strictly more positive than negative evidence. Retirement
removes the example from new selection. Retained query evidence is immutable,
so activation, retirement, or later feedback cannot change a frozen plan.

## Evidence aggregation and anti-gaming

Positive and negative outcomes are immutable rows. The per-candidate score is
the bounded beta posterior mean `(positive + 1) / (positive + negative + 2)`;
uncertainty is `1 / sqrt(total + 2)`. This replaces fixed increments and makes
contradictory outcomes visible. One actor/session/query can contribute each
verdict/correction outcome once; changing a free-form note cannot add weight.
The feedback identity is deterministic, and feedback plus its
aggregate update commit in one transaction. Exact retries are successful
no-ops. Concurrent distinct reviews use the database conflict/update boundary;
counts cannot be lost or multiplied by replay.

Notes do not enter scores, generation prompts, metrics labels, or normal logs.
Feedback never publishes a rule, semantic topic, or example by itself.

## Retrieval and precedence

Planning reads at most 64 inspectable candidates and admits no more than the
existing seven-example context cap. Non-active, low-confidence, contradicted,
unsupported-origin, wrong-locale, stale topic, stale context, changed source,
changed rule, and changed template candidates receive typed exclusion reasons.
The deterministic baseline combines bounded token-set similarity with the
posterior score and stable ID ties. When the request enables reranking, only
already authorized candidate questions enter the one Bifrost gateway; a complete
permutation is required. SQL, raw result values, credentials, and private notes
never enter reranking.

Generation keeps the existing lane order:
`edit_base > hints > selected examples > default`. Selection cannot override
mandatory constraints, metric closures, signed reach, or validator-issued plan
requirements. The exact deterministic baseline order, selected example
ID/version/position, score, uncertainty, decision,
exclusion reason, optional observed rank score, policy version, and gateway
receipt are stored immutably with the query. Run/replay uses that frozen query;
it does not consult mutable learning state or call the model again. When reranking
is enabled, the retained baseline is the bounded shadow comparator for the observed
reranked order; it performs no second SQL generation or execution.

## Portability and surfaces

Ordinary example reads redact SQL unless the caller has
`reporting.sql.read`. Protected export requires `feedback.write`, SQL inspection,
and current topic/dependency reach. The neutral schema carries version, exact
origin, question, protected SQL, digest, and positive/negative counts.

Import accepts one bounded row at a time. It routes an explicit anchor through
the current semantic/rule environment, requires exact origin equality, checks
the digest, validates SQL with the native validator and current binding, and
stores a candidate. Imported evidence never arrives active and therefore needs
local review. Unsupported versions, stale pins, lossy mappings, or counts above
1000 reject. No token, credential, raw result value, or local identity policy is
portable.

HTTP and the Go SDK expose feedback, list, review, protected export, and
revalidated import. MCP exposes feedback, bounded inspection, and review through
the same registered operations. The generic registered-operation CLI can inspect
and invoke the HTTP contracts with its existing explicit execution acknowledgement.

## Failure and evaluation boundary

Provider/budget failure during optional reranking is typed and follows the
configured gateway policy; it cannot widen candidates. Frozen operations remain
usable without the learning/rerank role. Repository tests establish deterministic
selection, bilingual token behavior, stale-pin exclusion, duplicate feedback,
positive/negative aggregation, concurrency, rollback by retirement, and protected
projection. These fixtures do not establish live-model quality, calibrated
business correctness, optimal thresholds, or long-horizon decay. Phase 24 owns
held-out evaluation and any reviewed policy-version replacement; phase 34 owns
external cohort mapping and full migration rehearsal.
