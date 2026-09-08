# Rule lifecycle v1

Status: bounded phase 16 runtime contract, 2026-09-08. Phase 16 remains `in_progress`.

## Implemented lifecycle

A private rule draft is bound to one retained active topic version and its immutable published digest. Draft revisions are immutable and use actor/session-scoped optimistic concurrency. Feedback or compilation can create a proposal, but neither can activate it. Review receipts are immutable, identify an exact draft revision and digest, and record an explicit approve or reject decision. Publication accepts only an approved receipt from the same actor/session, rechecks the active topic version and digest in the publication transaction, writes an immutable public rule version, and advances one per-topic CAS pointer. Current rule reads and evaluation fail after the pinned topic is replaced or archived, while exact retained rule reads remain available. Retirement clears the active rule pointer with CAS after validating its retained topic digest, including after a topic transition or archive; exact prior versions remain readable.

Public rule DTOs contain the rule definition, topic/rule pins, digest, state and timestamps. They contain no draft actor, session, private profile identifier, source credential, token or issuer data. Rules are compiled from the retained public topic projection. The projection keeps exact semantic references and can carry exact canonical revision references once topic publication supports them; this slice does not create a canonical registry.

The shared topic registry and SDK expose save, review, publish, current/exact read, deterministic evaluation and retirement. They reuse the existing `topics.write`, `topics.review`, `topics.publish` and `topics.read` actions. Each operation also requires the action-specific topic reach and every persisted source-read, dataset-query and execution-context-use reach of the pinned topic before rule payload selection or mutation. No action string, issuer or local grant is added.

## Deterministic hard-constraint evaluation

Evaluation accepts 1 through 256 unique, exact `semantics.Reference` values and rejects malformed or unknown references. It applies only the closed `require_reference` and `exclude_reference` constraints. Topic-scope constraints always apply; entity-scope constraints apply when an explicit input reference intersects the scope. The result reports sorted required and excluded references plus bounded missing-required or excluded-present violations. Advisory guidance and clarification patterns are not interpreted by this evaluator, and it makes no source or model call.

An allowed evaluation is evidence about this exact rule/topic version only. It is not a validator-issued plan, SQL safety proof, execution authority, row restriction or access decision. A later query consumer must retain the normal validated-read and current signed-authority requirements.

## Persistence and concurrency

Migration 014 adds actor/session-scoped rule draft heads and immutable versions, immutable review receipts, immutable published versions, a mutable CAS publication head, and immutable publish/retire events. Topic-version foreign keys prevent orphan pins. Draft, review and publication mutations lock and recheck the active topic publication before committing. Retirement instead locks the retained pinned topic plus the active rule head so cleanup remains possible after topic transition or archive. Published definitions and events are protected by the existing immutable-row trigger.

Two independent initial reviews identified the stale current-read/evaluation and
non-retirable post-transition state. Both narrow post-fix reviews of
`673fc0f..279b137` reported no remaining finding in that lifecycle correction,
schema inventory update or SDK alias. This closes the bounded review finding only;
cumulative Phase 16 acceptance and final integrated-head verification remain separate.

## Remaining phase 16 work

This slice does not implement question/token matching, required-slot gating, real-token advisory assembly, sensitive supplied-value validation, Bifrost clarification generation, replay/shadow comparisons, or query/block evidence and cache invalidation. `TestPhase16/AC01` through `AC06` remain cumulative phase gates; the focused lifecycle test is not a substitute for them. Phase 16 cannot be marked shipped until those consumers and tests pass.
