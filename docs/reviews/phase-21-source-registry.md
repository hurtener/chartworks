# Phase 21 bounded source registry evidence

Phase 21 is `in_progress`. This slice migrates the seven existing source
registration/catalog/validation routes to `internal/api`; it is not full phase
acceptance. `internal/sourceapi.SourceRegistry` now supplies the actual handler's
route/action selection, the existing checked operation manifest, and generated
OpenAPI from one concrete definition set. Disabled warehouse/native-validation
capabilities remain absent. No endpoint, authority model or migration was added.

Each definition requires request/response schemas, stable action/effect labels,
existing resource-loader and audit descriptions, error codes and body bounds.
Schemas derive from the actual Go wire DTOs using the already pinned JSON Schema
dependency. Write shapes are closed; response arrays admit the null representation
that Go emits for nil slices. The generated self-contained document uses
[OpenAPI 3.1.1](https://spec.openapis.org/oas/v3.1.1.html). Its description of existing
Pengui bearer authentication does not supply or expand authority.

The domain handler still performs Pengui verification, transport validation and
action checks; existing services retain resource resolution and signed-reach checks.
Resource-loader/audit labels are descriptions, not callbacks or proof of exhaustive
audit coverage. Request schemas describe wire shape; domain validity is still
enforced by the existing service. Error schemas describe the existing error mapping.
The registry API is immutable and tested for concurrent document generation and
detached metadata. Its initial supported DTO and route shapes are deliberately
bounded to the concrete source consumer.

## Focused verification

- `GOMAXPROCS=2 go test -race -count=1 -p=2 -coverprofile=/tmp/chartworks-phase21-api.cover ./internal/api`
  passed with **96.5%** statement coverage against its 80% package band.
- `go test -race -count=1 -p=2 ./internal/api ./internal/sourceapi ./test/acceptance -run 'Test(Schema|Registry|SourceRegistry|SourceAPIAndSDK)'`
  passed in the existing Go 1.26.4 Linux verification image against real PostgreSQL
  17, with the pinned Bruin `5f562c2` Go modules and unchanged `02638a8` native library.
  The existing HTTP/SDK fixture checks actual successful request/response bytes
  against registered schemas and actual error status/codes against registration,
  alongside missing-bearer/scope denial, body limits, stale source-context rejection,
  rotation, concurrent reads, and metadata reads without warehouse availability.
- Focused API vet, planning, contributor-rule mirror and diff checks passed.

These focused checks are not full sourceapi package coverage, cumulative preflight,
cloud CI, browser acceptance, or `TestPhase21/AC01`–`AC06`. No placeholder phase test
was introduced. Full phase acceptance remains unavailable.

## Remaining implementation

Execution/engineering/pipeline source routes, foundation health/capabilities,
security and work operations still need shared definition adapters. Public OpenAPI
delivery and cumulative generated SDK/isolation/audit coverage remain outstanding.
Existing local registries for those consumers are not represented as migrated.

Phase 15 still needs persisted draft/review/publication/rollback/archive, atomic
topic/facet activation, current source/profile health and Pengui revalidation,
canonical-registry collision handling, generation/onboarding and service-backed
portability. Phase 16 still needs lifecycle and constraint/slot runtime consumers,
advisory injection, replay/shadow and cache invalidation. This source adapter does
not complete their hard phase 21 prerequisite or their named acceptance criteria.

The prior bounded semantic foundation was rebased from `aca185a` onto exact PR10
source `62f0362d458e46e697b638ff916c47759020f55b`, producing `fd9b519` before this
slice. Its `internal/semantics` bytes are unchanged; the original reviewed history
is preserved at local branch `codex/phase-15-16-foundation-backup`.
