# Phases 15–18 and 21 — current delivery evidence

Status snapshot: 2026-09-08, proposed PR #11 delivery at documentation head
`17969f15c2dc09c19863aca85be3c5df1c2fbbbb`, with runtime evidence from exact head
`513a3be15eb07e48e3c5cb7bc31818848cd7275a`. This is the current cross-phase
evidence index for the proposed delivery. It records reviewed completion slices and
the remaining hosted gate; it does not claim that hosted CI passed or that the PR
merged. Historical bounded evidence keeps its original date and exact head. Required
hosted evidence is tracked at the [PR #11 checks](https://github.com/hurtener/chartworks/pull/11/checks).

The proposed delivery registry records phases 15, 16, 17, 18 and 21 as `shipped`.
Those statuses become effective only if every required hosted check passes and PR
#11 merges. Fifteen later workstreams remain planned, including the unimplemented
phase 25 full-release gate.

## Current dispositions

### Phase 15 — topics lifecycle

The original review of `8291e84cf5bcb04c16ecbbae6cbf3d8e561d9d43` and the
forward migration repair `d103ba95af8a4f951b0a4d2589292ec269905a67` are cleared
by their narrow follow-up review. The later unresolved-rebind correction
`6884f23126dbf01e45c2d799e8f10dfee03c2955` is integrated, and root's exact
semantic checks passed. Root's committed-source Phase 02 and Phase 15 strict
runs passed all six children with zero skips, including health and enhanced
rebind. The bounded evidence remains in [the topic draft record](phase-15-topic-drafts.md)
and [the publication record](phase-15-topic-publication.md).

Phase 15's exact 513 integrated coverage, strict acceptance and full lint/vet/build
checks pass. The proposed PR #11 delivery records Phase 15 as shipped conditionally;
that status becomes effective after every required hosted check passes and the PR
merges. Recorded Bifrost fixtures do not measure live semantic quality. Its approved
84.5% exception remains limited to `internal/store/postgres`.

### Phase 16 — rules and clarification

The reviewed lifecycle and evidence slice is integrated through `cdbafc35`, with
the final local lint cleanup at `72a27bd`. Root's strict Phase 16 run passed all
six children with zero skips, the readiness/lifecycle checks passed, and the
narrow follow-up review of the Phase 16 fixes is clear. The real-PG rule-evidence
coverage addition `192a8bb508197b06f0693bf5d2b8ba6590e2ee20` and the Phase 18
PostgreSQL boundary tests `9422ee3fbdf07f3ae7da3939da2aa49f7c763338` are both
integrated at the current head as `513a3be`.

Root's committed-source strict checks for the integrated head pass the named
Phase 16 criteria with zero skips, and no open Phase 16 P0/P1 finding remains.
The exact 513 Linux/native race and coverage run passed every configured band:
`internal/store/postgres` measured **3039/3593 (84.58%)**, and
`internal/nlqexec` measured **579/711 (81.43%)**. The profile is
`/tmp/chartworks-phase13-gates-2cdf7ef/coverage-integration-513a3be.out`, and the
acceptance portion completed in 161.492 seconds. Full lint, vet and build also
passed at 513. Phase 18 remains the real consumer of invalidation effects. The
proposed PR #11 delivery records Phase 16 as shipped conditionally; that status
becomes effective after every required hosted check passes and the PR merges. See
[phase 16 evidence](phase-16-rules-evidence.md).

### Phase 17 — routing and context

The first routing review's two P1 fixes are recorded at
`ba65fa7b731a69182a8d93be86d5c5b86130b93f`. The follow-up review is clear, and
root's strict Phase 17 acceptance at `492c9fbf6f7fc7f6fe2f5659d2499afdbb3409b3`
passed all six children with zero skips. The actual route, HTTP/SDK and runtime
OpenAPI checks are included in that disposition. The [routing evidence](phase-17-routing.md)
retains the exact relationship, metric ambiguity and sealed-context assertions.

The proposed PR #11 delivery records Phase 17 as shipped conditionally; that status
becomes effective after every required hosted check passes and the PR merges.
Recorded Bifrost responses are deterministic fixtures, not live semantic-quality
measurements.

### Phase 18 — generation and execution

The public HTTP/SDK execution surface landed at `6801a28acbd728578ab1f3329a06bff4fc830f76`;
root's strict Phase 18 acceptance passed all six children. The real invalidation
consumer landed at `c88dbcc`, and the author acceptance plus root's full `37f713c`
suite passed its six criteria. The public transport fixes at `459b91d6990fca2d0676bae6d5e0a76bdc81c3be`
were integrated as `ecd08fa`.

The first complete Phase 18 review was recorded at
`37f713c2ee492536ed15086ecd5e7afa53a364e1`; the second complete review at
`4efbb5a4b4fe61eda20ddf0efe488385660916bd` cleared the bounded review round.
The unsafe-correction fix is integrated as `7493899338f646382a219d336dd272258619a42f`,
the detached route-request fix as `7bd27f16cf3885ff792ecc76d9053a51fc185efc`, and
the local confidence-pointer P2 fix `1607e58b003d94ea52047cca1350664a218b6428` as
`bef6cae`. Root's narrow review of the corrected head found no P0/P1; the Phase 18
PostgreSQL boundary tests `9422ee3` are integrated in `513a3be`.

The correction contract is bounded and preserves the existing Bruin validator and
executor path: every dialect retains the exact validated SQL, bound parameters,
plan coordinates and source/context revision; PostgreSQL may additionally accept
the same AST while ignoring only locations and formatting. Any other SQL change
fails closed with `ErrUnsafeCorrection` and does not execute a second candidate.
Receipts are recorded for every attempt. The proposed PR #11 delivery records Phase
18 as shipped conditionally; that status becomes effective after every required
hosted check passes and the PR merges. The [runtime evidence](phase-18-nlq-runtime.md)
records the retained-pin and stale-evidence behavior.

### Phase 21 — HTTP prerequisite

The Phase 21 narrow fix at `2395f8cd15a2bf6f23c78ee512c81810c8a1226d` is clear in
follow-up review. Root's strict verification of the integrated source at
`513a3be15eb07e48e3c5cb7bc31818848cd7275a` passed all six Phase 21 criteria with
zero skips, including composed registry/OpenAPI and actual HTTP/SDK boundaries.
This closes the bounded prerequisite slice. The proposed PR #11 delivery records
Phase 21 as shipped conditionally; that status becomes effective after every
required hosted check passes and the PR merges. Later domain consumers remain
outside this prerequisite. See [Phase 21 evidence](phase-21-source-registry.md).

## Cumulative gate boundary

The proposed delivery contains the reviewed Phase 15–18 and 21 slices. Root's exact
513 archive run passed every configured coverage band and full lint, vet and build;
the one-line foundation-smoke contract correction at
`d6f19db5397b2aab6d2772c3de02aec52c4bafdb` then passed the compiled binary smoke,
planning checks and mirror check without runtime changes. The [PR #11 checks](https://github.com/hurtener/chartworks/pull/11/checks)
remain the final hosted gate. No threshold is rounded or lowered. This document
does not claim hosted CI passed, a merge, live-cloud qualification or live
semantic quality. After a green hosted run and merge, the registry count will be
19 shipped phases and 15 planned workstreams; the phase 25 full-release gate
remains planned.
