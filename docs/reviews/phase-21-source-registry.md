# Phase 21 bounded source registry evidence

Phase 21 is `in_progress`. The shared registration now covers 33 existing source
operations: seven catalog/validation, five execution, fourteen engineering and
seven pipeline operations. It is not full phase acceptance. Each source-family
registry supplies its actual handler's route/action selection, existing checked
operation manifest, and generated OpenAPI from one concrete definition set. Disabled warehouse/native-validation
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

Two independent reviewers of `dae6bdb` found no P0/P1 and the same local P2:
caller-owned schema wrapper pointers could overwrite the registry's schemas.
`cloneDefinition` now copies both nonnil wrappers at input and return boundaries;
their private immutable backing remains shared. The focused regression reproduced
request/response validation and OpenAPI corruption through constructor input,
`Definitions`, and `Match` before the fix. It now passes, including concurrent
returned-wrapper mutation during document generation, under the API race suite
with **96.6%** coverage. An Astra medium narrow re-review closed the finding at
`81e0d6b53b67bdeb319aca82bfb02e6d39acc6d3` with no new findings. The owner's frozen
Go 1.26.4/Linux verification at that exact head passed the schema/registry and real
PostgreSQL source HTTP/SDK suite under race detection: API 1.349s, sourceapi 1.509s,
acceptance 17.956s. This establishes the first seven-route slice only.

## Execution, engineering and pipeline continuation

`ExecutionAPIRegistry`, `EngineeringAPIRegistry` and `PipelineAPIRegistry` replace
their former independent inventories with the same `api.Definition` and schema
compiler. Existing method/path/action/effect manifests are unchanged and checked
against generated OpenAPI for each configuration. The current engineering upload
limit is supplied by the actual service; the legacy metadata-only projection uses
the configured type's default because its four-field manifest contains no limits.
No domain behavior, action/resource scope, provider implementation or operation was
added by this adaptation.

The shared schema support now includes the concrete nullable response pointers,
string-to-integer profile count maps, and exact scalar JSON result cells. Read
cells permit string/number/boolean/null; objects and arrays remain excluded, since
the read core returns structured values as JSON text strings. Unknown costs remain
null. Private operation receipt fields are described from their existing public DTO;
they are not new request authority inputs.

Raw upload content is registered as `application/octet-stream` with the service's
bounded byte limit (at most 100 MiB), rather than a JSON/base64 field. Source/read
and engineering JSON retain their 65,536-byte bound; pipeline JSON retains 1 MiB.
Pipeline schemas explicitly model fields its existing closed JSON decoder permits
to be omitted or null before domain validation. This does not replace domain checks
or change decoder behavior. Other write schemas keep the stricter required-field
mode. The HTTP test observer records only bytes the handler actually consumes so
that it preserves denial-before-upload-read behavior.

Focused family fixtures validate successful wire bodies and actual error metadata
alongside existing authorization, retention, cancellation and SDK assertions.
The continuation checks passed on a frozen copy whose Go/module files were compared
byte-for-byte with the implementation worktree:

- API race unit tests and focused vet passed; API statement coverage is **96.0%**
  (`GOMAXPROCS=2 go test -race -count=1 -p=2 -coverprofile=/tmp/chartworks-phase21-families-api.cover ./internal/api`).
- The Go 1.26.4 Linux fixture used real PostgreSQL 17 and the existing pinned `5f`
  modules/runner with the unchanged native library. Race-enabled API/sourceapi
  schema, registry and manifest tests passed (1.417s and 3.523s).
- The focused acceptance set passed in 99.140s: `TestSourceAPIAndSDK`,
  `TestReadAPIAndSDK`, `TestEngineeringRegisteredSurfaces`,
  `TestEngineeringRejectsAmbiguousBodiesBeforeSourceAccess`,
  `TestEngineeringHTTPCheckpointCancellationAndResume`, `TestPipelineSDKLifecycle`,
  `TestPipelineRegisteredSurfacesDenyBeforeBodyOrSource`, and existing phase 11/12
  AC05/AC06 subtests. It used `go test -race -count=1 -p=2`, the existing binary
  fixture and actual supervised pipeline runner; model responses remained local
  fixtures. No live provider measurement is claimed.
- Planning (224 criteria, 63 features, 41 gates, 34 phases), mirror and diff checks
  passed. Full sourceapi coverage and cumulative phase acceptance are not claimed.

The first acceptance run exposed the observer's old 1 MiB response cap on the
existing larger read-result fixture. The observer now permits the established
16 MiB result plus 128 KiB receipt ceiling while individual SDK calls retain their
own caps; no production limit changed. These checks do not retroactively extend
the frozen `81e0d6b` evidence or mark phase 21 complete. The next review is scoped
to this continuation.

## Remaining implementation

Foundation health/capabilities, security and work operations still need shared
definition adapters. Public OpenAPI delivery and cumulative generated SDK/isolation/audit coverage remain outstanding.
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
