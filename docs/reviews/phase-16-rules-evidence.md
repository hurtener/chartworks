# Phase 16 rules evidence

Status: proposed shipped delivery for PR #11, 2026-09-08. This record covers the
combined phase 16 branch based on `c3ccd730ac6537c65f3d439aa9b9564c982f0289`;
the proposed status remains conditional on hosted checks and merge.

The branch preserves migrations 015 and 016 from the combined integration base and
adds the exact phase 18 runtime metadata migration `017_nlq_runtime.sql` from
checkpoint `9f44a1920114550315ccb58fa0065a8d7e30aa3e`, plus `018_rule_evidence.sql`.
The phase 16 slice adds immutable replay/shadow comparison evidence, publish/retire
invalidation fences, detached pattern reads, SDK methods, and the shared topic registry
handlers. The service performs rule-version pinning before exact topic/dependency and
payload reads. The phase 17 routing service remains the first required-slot and
real-token advisory consumer; phase 18 owns query/evidence consumption of the
invalidation ledger.

`test/acceptance/phase16_test.go` defines all six named criteria over real PostgreSQL
fixtures: signed lifecycle and self-approval rejection; conflict and mandatory
preservation; typed clarification before gateway use and supplied choice admission;
English/Spanish tokenizer tiers, omission bounds and typed combined-budget failure;
exact retained replay/shadow evidence and immutable comparison rows; and ordered
publish/retire invalidation cursors with retained historical reads.

Checks completed on the macOS host:

- `GOMAXPROCS=2 go vet ./internal/semantics/rulesets ./internal/topicapi ./internal/store/postgres ./sdk/chartworks ./test/acceptance`
- `CGO_ENABLED=0 GOMAXPROCS=2 go test ./internal/semantics/rulesets ./internal/topicapi ./internal/store/postgres ./sdk/chartworks ./test/acceptance -run '^$'`
- `make planning-check`, contributor-rule mirror check, and `git diff --check`

The normal macOS test link cannot resolve the pinned Bruin Rust library in this host's
module cache, so no local runtime pass is claimed here. Root must run the committed
head with the Linux/native image, real PostgreSQL, race detection and the dependent
phase 18 consumer before changing phase status or calling the work complete. No live
cloud provider or release gate is claimed.

## Current integrated verification

The earlier host limitation above remains historical. Root's committed-source strict
Phase 16 run at `cdbafc35e905597052da88518ee6717baa4d3396` passed all six children
with zero skips, including readiness and lifecycle checks; the narrow follow-up review
of the Phase 16 fixes is clear. The test-only PostgreSQL coverage addition
`192a8bb508197b06f0693bf5d2b8ba6590e2ee20`, integrated at the cross-phase head,
passed the native real-PG race check in 9.340 seconds. The later Phase 18 store
boundary tests `9422ee3fbdf07f3ae7da3939da2aa49f7c763338` are integrated as
`513a3be15eb07e48e3c5cb7bc31818848cd7275a`.

Root's exact 513 Linux/native race and coverage run passed every configured band;
`internal/store/postgres` measured 3039/3593 (84.58%), above the approved 84.5%
exception, and `internal/nlqexec` measured 579/711 (81.43%). The acceptance portion
completed in 161.492 seconds, with full lint, vet and build also passing. Phase 18
remains the real invalidation consumer. PR #11 records Phase 16 as shipped
conditionally; that status becomes effective after every required hosted check
passes and the PR merges. This record does not claim hosted CI or release
completion.
