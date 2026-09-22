# CW-08 reviewed-learning adversarial record

## Scope reviewed

This review covers LRN-01 and LRN-02 at the implementation head: feedback and
example persistence, applicability, retrieval/reranking, precedence, immutable
selection evidence, review/retirement, portability, and HTTP/MCP/SDK/CLI
registration. It does not claim phase-24 live quality calibration or the full
phase-34 cohort migration.

## Findings and corrections

1. **Import replay could amplify evidence.** The initial import reused the feedback
   aggregation upsert, so a repeated bundle could add counts. Import now has its own
   atomic insert-or-read path. Exact replay returns the existing row without changing
   evidence; a matching digest with a different origin returns a conflict.
2. **Candidate truncation could hide the most relevant row.** Reading only eight
   score-ordered rows made lexical relevance ineffective beyond that prefix. The
   selector now evaluates a bounded 64-row pool, records exclusions, and still admits
   no more than seven examples.
3. **Review rationale was not durable.** Activation now stores the review note as
   protected metadata alongside reviewer, time and CAS version. It is excluded from
   ordinary projections, prompts, labels and logs.
4. **Feedback identity over-collapsed corrected outcomes.** The uniqueness boundary
   now ignores note-only changes while allowing distinct corrected SQL outcomes.
   Deterministic exact retries remain no-ops.
5. **Origin conflicts returned a driver error.** Feedback/upsert origin conflicts now
   map missing conflict-update rows to the typed store conflict boundary.
6. **Portability was missing an MCP consumer.** Protected export and revalidated
   candidate-only import now bind to the same registered HTTP operations as the SDK
   and generated CLI surface.
7. **Source rotation could assign a false current origin to stored SQL.** Feedback
   now rejects a changed retained binding digest and validates both stored and
   corrected SQL against the current binding before recording learning evidence.
8. **The populated migration could trip the old active-row guard.** Migration 042
   now installs a temporary exact normalization guard while its table lock is held,
   normalizes active and candidate legacy rows, then installs the final stricter
   lifecycle guard before commit. Migration 041 remains byte-for-byte historical.
9. **Activation eligibility had a read/update race.** The service forces the read
   version into the state transition, while PostgreSQL repeats positive/negative
   eligibility in the same update. Current topic/source/context/locale/rule/template
   applicability is rerouted and checked before that CAS.
10. **Malformed rerank output could omit or repeat candidates.** Selection now
    requires a complete duplicate-free permutation of known IDs and rejects any
    observed non-finite score.
11. **Activation reused the generation router's `topics.read` gateway action.**
    Review now uses a metadata-only current-origin path authorized by the registered
    `feedback.write` action and exact dependency reach. Production topic, rule and
    source readers expose bounded review reads; the path makes no model or vector
    call and never derives an unsigned action.

No P0/P1 finding remained after these corrections. Learned state never supplies
authority, changes a publication, bypasses the native validator, or modifies a
retained query.

## Verification

The focused CGO-free package tests and vet pass for `internal/nlqexec`,
`internal/nlqapi`, `internal/store/postgres`, `internal/mcpserver`, and
`sdk/chartworks`. The acceptance package compiles with `CGO_ENABLED=0`, including
the real-store concurrency cases. `git diff --check` passes.

`TestCW08PopulatedExampleUpgrade` passes against a disposable local PostgreSQL 17
server with pgvector. It installs the immutable prefix through migration 041 with
populated active and candidate examples, applies migration 042, verifies both rows,
checks the restored lifecycle guard, and exercises stale-version plus atomic
negative-evidence activation refusal. The wider phase-18 acceptance fixture still
requires the native CGO parser build; its CGO-free invocation stops at the existing
`exec: capability unsupported` boundary rather than supplying real-store evidence.
The existing Docker engine remains unusable because its image/content storage
returns I/O errors; no existing container was restarted or modified.
`TestVerifyOriginUsesFeedbackAuthorityWithoutGateway` exercises the real router
method with a reviewer that has no `topics.read` action, proves zero engine calls,
and rejects missing review authority and wrong-context reach. The production-wiring
PostgreSQL acceptance `TestCW08FeedbackOnlyReviewAuthority` is included for the
native-parser-capable acceptance environment.

`make planning-check` reaches the planning checks but its pre-existing Python
coverage-gate unit tests fail on macOS temporary-path aliasing (`/private/var` versus
`/var`). This change does not modify that gate. D-074 remains intact: no automatic
heavy workflow was added.
