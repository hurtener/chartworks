# Phase 20 and cumulative phase 21 — adversarial review

Status: implementation and same-author adversarial review in progress. This record is not independent human approval or a phase-25 release claim. Base: merged `99055d4fd5f7a613a9518f3b2b3e4e564301eb5c`. Preserve its phase-19 implementation and earlier qualified source paths.

## Scope inspected

The pure fourteen-kind specification core, typed read-result adapter, protected service, five registered HTTP/SDK methods, optional Bifrost rank boundary, schema generator, composed route guard, configuration, capability reporting and evidence fixtures. No reporting store, renderer, additional issuer or source executor was added.

## Findings corrected

| Probe | Failure risk | Correction and regression evidence |
|---|---|---|
| Reorder saved column pins without changing binding IDs | Table values could acquire the wrong column metadata. | Validate exact pin order against bound IDs; reordered-table and saved-mapping tests in `internal/charts` and phase 20 AC04/AC05. |
| Supply the same instant with different timezone offsets | Time-series uniqueness could accept an overlapping coordinate. | Compare normalized instants; duplicate-offset negative in the chart core. |
| Put JSON null in required strings, booleans or version numbers | Go zero values could weaken a nominally closed request contract. | New nullable-collections schema mode permits only nullable collections, not required scalar nulls; schema unit tests and HTTP AC06 negatives. |
| Send concurrent large bodies before service admission | A service semaphore alone did not bound transport allocations. | Separate non-queued decoding admission before reading bodies; blocked-body AC06 test receives 429 and verifies release after failure. |
| Supply enormous explicit binding lists | Binding construction could allocate before validating its cap. | Check binding/order counts before constructing exact column pins; explicit-bound negatives. |
| Cancel or expire authority after a paid rank attempt | Returning only an error could erase usage, or expose data after authority expired. | Typed content-free failure receipt, no candidate/data result; cancellation and expiry tests through service, HTTP and SDK. |
| Mutate rank score/usage pointers after return | Shared result aliases could change earlier observations or race. | Copy ranked items and usage token/cost pointers; detached receipt and concurrent-reuse tests. |
| Add an HTTP handler without registering its security contract | Metadata-only coverage did not prevent an unregistered route from executing. | Composed `api.Guard` rejects unregistered paths and enforces registered bearer/action admission; per-route deny/dispatch tests and existing real-resource tests. |
| Omit authentication errors from a protected registration | OpenAPI/security claims could diverge. | Registration rejects missing 401/403 mappings; method mismatch remains the shared explicit empty 405 contract. |
| Put executable formatter/URL data into options | An eventual renderer could interpret unsafe configuration. | Closed typed options, rejection of unknown fields, scripts/URL forms, fixed byte/depth limits; AC06 plus core fuzzing. Ordinary data labels remain literal text, not executable configuration. |
| Pass large numeric JSON values through generic decoding | IEEE-754 conversion could corrupt exact labels before rendering. | Qualified read cells stay strings or `json.Number` until exact typed projection; large integer/decimal HTTP/SDK tests and real PostgreSQL read-to-spec regression. |
| Claim arbitrary binary/structured strings are typed read values | Malformed encoded values could cross the specification boundary. | Validate hexadecimal binary and JSON structured cells before use; typed projection and malformed-cell unit negatives. |

## Acceptance and fixture coverage

`TestPhase20/AC01`–`AC06` cover rules-first selection, metadata/provenance, all fourteen kinds, exact values/total scope, saved-mapping proposals and bounded secure options/ranking. Eighty-four checked-in JSON goldens cover binding, ordering, empty, negative, null and long-label cases for each kind. Explicit requests never silently substitute a table. These are specification goldens, not rendered pixels.

`TestChartsFromQualifiedReadExecution` exercises real PostgreSQL data through the existing positive validator/read executor, source HTTP SDK and typed chart adapter into explicit chart/table outputs. It asserts exact decimal/large-integer labels and no additional source/credential access when drawing or rebuilding. It requires the real database fixture and never skips in its absence.

`TestPhase21CumulativeRegistryGuard` enumerates actual composed protected operations, including NLQ/BYO and charts, and verifies missing/invalid bearer and missing-action denial before dispatch. Its positive probe proves admission only, not resource access. Phase 21's existing real HTTP/SDK/PostgreSQL/resource tests remain required. Phase 21 AC03–AC06 cover limits/CORS, registration/OpenAPI, implemented-only surfaces and concurrent public requests.

The optional rank test uses the actual Bifrost SDK against a recorded TLS provider response. Additional adversarial engines exercise incomplete/foreign/duplicate/nonfinite ranking, budget exhaustion, disabled providers, interruption, and result/receipt isolation. No recorded test is described as live provider quality or cost measurement.

## Verification at preparation time

Local Linux amd64 Go 1.26.4 race runs passed all six named phase-20 criteria, the non-database phase-21 guard and AC03–AC06, and the new unit tests. A focused combined instrumentation run measured `internal/charts` 85.2%, `internal/chartservice` 89.5% and `internal/chartapi` 89.3%; that particular command also matched the separate real-database integration and correctly failed because this container has no PostgreSQL fixture. Its measurements are diagnostics, not a passing full-suite result. The later complete committed-head CI must satisfy all configured coverage bands without lowering them.

Required before ready status: exact-source full race coverage and database/native fixtures, strict phase 20/21 and cumulative named acceptance, vet/lint, native/reference-container builds, fuzzing, planning/drift/mirror checks and unchanged source. Hosted results will be recorded here or in the PR with the exact tested head. Until then, no full-suite success is claimed.

## Remaining boundaries

Phase 20 remains in review. Phase 21 is an already shipped prerequisite extended cumulatively, not the implementation of future render/export/private-preview routes. Those domain phases must register real operations and retain isolation tests when they land. Rendering, retained artifacts, publication of output definitions and the remaining 13 planned workstreams are not implemented by this PR. Caller-supplied metadata cannot certify a source/topic or widen signed reach. Exact totals cover provided rows only, including rows omitted from plotting; they are not full-source totals or inferred aggregates.
