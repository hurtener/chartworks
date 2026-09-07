# Phase 15/16 bounded foundation evidence

Code reviewed: `2c149674b88a7c4d029e853f5f548a946afd4d9c`.
Phases 15/16 are `in_progress`; phase 21 remains planned. This is unfinished
implementation, not full-phase acceptance or a shipped lifecycle capability.

The bounded slice contains the typed topic-pack compiler, version-pinned rule and
clarification authoring compiler, neutral logical-slot export, untrusted draft
candidate import, and deterministic version/entity diff. The
[foundation contract](../contracts/semantic-foundation-v1.md) records the implemented
boundaries and remaining service work.

Two independent Astra reviewers inspected `fd65e891052bb71b1e9e2e8faf025b76f1aeee96`
and found no P0/P1 issues. Reviewer B identified one reachable P2: reference sorting
built allocating keys before rejecting oversized coordinates. Commit `2c14967`
moved reference-shape checks for KPI inputs, rule scope targets, and pattern targets
into the existing pre-copy/pre-sort validation stages. Its regression checks early
rejection and stable `invalid_reference` classification without allocation-count
thresholds. Reviewer B's narrow diff-only re-review passed at `2c14967`; no findings
remain for this bounded slice. No additional whole-branch review was performed.

| Check | Bounded evidence |
|---|---|
| Semantics unit/concurrency suite | `GOMAXPROCS=2 go test -race -count=1 -p=2 -coverprofile=/tmp/chartworks-semantics-reference-bounds.cover ./internal/semantics` passed; 93.9% statement coverage against the 80% package band |
| Exact committed-head regression | `TestSortedReferencesRejectOversizedCoordinatesDuringShapeValidation` passed with race detection at `2c14967` |
| Static checks | Focused `go vet`, planning, contributor-rule mirror, and diff checks passed |
| Full phase acceptance | Unavailable: no `TestPhase15/AC01`–`AC06` or `TestPhase16/AC01`–`AC06` completion is claimed |
| Cumulative preflight / cloud CI | No passing result claimed for this unfinished branch |
| Browser / live provider / source service | Not exercised by this pure foundation slice |

Remaining work includes phase 21 shared registration/OpenAPI; phase 15 persisted
draft/review/publication/rollback/archive, atomic topic/facet activation, live
source/profile health and authority revalidation, canonical-registry collision
checks, generation, onboarding, and service-backed portability; and phase 16
reviewed lifecycle, required-slot evaluation, real-token advisory injection,
execution-constraint consumers, replay/shadow, and cache invalidation. Actual
HTTP/SDK operations, PostgreSQL acceptance, and all named phase criteria remain
required. Marking the phases in progress ensures their missing implementation
gates cannot be bypassed through a planned-phase skip. No acceptance stubs were added.
