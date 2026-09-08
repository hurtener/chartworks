# Phase 15 topic publication bounded evidence

Status: bounded publication slice verified at `a3708ab196bb1db0c36e52720be41da097c157e0` on 2026-09-08. This evidence covers the implemented review, publication, retained-read, current-contract, rollback, archive and managed-facet boundaries. It does not claim the cumulative `TestPhase15` acceptance parent, the remaining semantic lifecycle work, or release completion.

## Implemented boundary

The publication slice adds an explicit review receipt for one private draft
revision, immutable publication versions, staged per-context facet generations,
atomic semantic/vector activation, retained current and exact-version reads,
current source-contract checks, rollback, archive and managed facet search. The
public surface is the registered HTTP/SDK consumer described by the
[topic-publication contract](../contracts/topic-publication-v1.md). Draft
profiles, actor/session provenance, credentials and source rows remain outside
public responses.

The implementation keeps the phase 07 persisted embedding-space key format and
checks the complete descriptor. Contract and rollback require their primary
topic action plus the existing `sources.read` action for live discovery; archive
does not perform discovery and remains available when a source is unhealthy.

## Verification evidence

The pre-fix root focused acceptance run against `1821373979d71e448c0215184d113cbe4191bbfe` passed the two publication acceptance tests in 16.274 seconds. That was a focused baseline and did not qualify the corrected head. Author-focused checks after the correction covered the changed gateway, vector-index and topic-publication boundaries; a Darwin `internal/semantics/topics` link attempt was not counted as a pass because the native Bruin Rust library was unavailable in that environment.

Root then ran the exact committed `a3708ab196bb1db0c36e52720be41da097c157e0`
source through the Linux Go 1.26.4 race/coverage gate with the real PostgreSQL
17/pgvector fixture and pinned native dependencies. The acceptance portion took
259.417 seconds. Every configured coverage band passed:

| Band | Statements | Result |
| --- | ---: | --- |
| `internal/store/postgres` | 2427/2863 | 84.77% |
| `internal/semantics/topics` | 217/266 | 81.58% |
| all other configured bands | — | passed |

The complete profile is `/tmp/chartworks-phase13-gates-2cdf7ef/coverage-publication-a3708ab.out`. The existing `internal/store/postgres` 84.5% exception remains unchanged and applies only to that package. The result is exact-head local verification; it is not a hosted-CI, whole-phase-acceptance or live-cloud qualification claim.

## Review rounds and corrections

The original dual review of `1821373979d71e448c0215184d113cbe4191bbfe` found two reachable P1 issues:

1. The phase 15 gateway key format had changed from the phase 07 persisted
   `vindex.Space` JSON digest. Ready legacy generations would therefore be
   rejected by the managed search key check after an otherwise compatible
   deployment.
2. The current source contract and rollback paths perform live discovery but
   their active documentation did not state the required secondary
   `sources.read` action. The missing conjunction was not sufficiently covered
   by ordinary negative tests.

Commit `a3708ab` corrected both findings. `EmbeddingSpace.Key` now hashes the
same complete JSON descriptor and field order used by the persisted phase 07
space, with an independent golden and a real acceptance check that reads the
stored key and searches the ready generation. The provider-registration,
publication-contract and phase plan text now state the primary/secondary action
conjunction, and ordinary acceptance tests deny contract/rollback without
`sources.read` while preserving archive without discovery.

Two independent narrow post-fix reviews of `a3708ab` inspected only those
closures and the new ordinary tests. Both found no actionable P0, P1 or P2
issue. No further review round was required.

## Remaining boundary

The six cumulative `TestPhase15/AC01`–`AC06` criteria remain unclaimed. Canonical
registry entities, onboarding/entity APIs, source-reference rewrite workflows,
generation and full lifecycle portability remain pending, as do phase 16 rules
and phase 21's broader HTTP/API work. Phases 15, 16 and 21 remain `in_progress`.
This document records a bounded publication consumer and its exact-head evidence;
it does not change the phase registry or imply shipped status.
