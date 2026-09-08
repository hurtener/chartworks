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

## Current delivery disposition

The bounded `a3708ab` publication evidence above remains exact historical evidence.
The current phase 15 candidate now composes this publication boundary with canonical
registry, onboarding/entity and source-rebind operations, durable health/recheck,
bounded resumable generation, portability and the cumulative
`TestPhase15/AC01`–`AC06` suite. Root's pinned Linux/native/PostgreSQL cumulative run at
`c3ccd730ac6537c65f3d439aa9b9564c982f0289` passed all packages and acceptance in
173.985 seconds. Author fix `8291e84cf5bcb04c16ecbbae6cbf3d8e561d9d43`
added the unresolved-rebind regression and passed all six Phase 15 children in 14.491
seconds before integration as `39bc68c753ff65e1d382102dcd6bcfdd0ea8387b`.

Exact full-chain testing at `39bc68c` found that migration 017 had replaced the audit
action constraint without preserving migration 016's `topic.health_rechecked` action.
The resulting audit rejection atomically rolled back a correct source-drift health
observation and returned HTTP 400. Forward migration 019 at
`d103ba95af8a4f951b0a4d2589292ec269905a67` restores the complete closed action union,
including the health action, while retaining unknown-action rejection. Its author
strict Phase 02 and Phase 15 runs passed all six children with zero skips, and the
repair integrated as `dd6f79e`. Root verified its SHA-256-checked committed-source
archive: strict Phase 02 and Phase 15 each passed all six children with zero skips,
including AC04 health and enhanced rebind, and native race `TestSafeErrors` passed.

The narrow follow-up review of fixes `8291e84` and `d103ba9` cleared. The later
unresolved-rebind correction
`6884f23126dbf01e45c2d799e8f10dfee03c2955` is integrated and root's semantic checks
passed. Exact 513 integrated-head coverage, strict phase checks and full lint/vet/build
checks pass. The proposed PR #11 delivery records Phase 15 as shipped conditionally;
that status becomes effective after every required hosted check passes and the PR
merges. The 84.5% coverage exception applies only to `internal/store/postgres`;
recorded Bifrost fixtures do not measure live semantic quality. This bounded record
does not claim hosted CI or a merge.
