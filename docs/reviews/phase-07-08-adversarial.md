# Phases 07/08 and phase-09 prerequisite — adversarial self-review

Reviewed on top of the merged phases 01–06 baseline
`9ab8af135ac062d62731212da012ca0e589aa127`. This extends the existing implementation;
it is not a rewrite or an independent external security audit.

## Findings and executable regressions

| Finding | Correction and evidence |
|---|---|
| Base-table positional aliases expose hidden physical columns | Reject the base-table column alias list while retaining normal/derived/CTE aliases; `TestSQLAliasCannotExposeHiddenInputs` uses a real PostgreSQL canary oracle and asserts zero native-planning credential lookups on rejection. |
| GROUP BY input precedence can be confused with output aliases | Input-only resolution for this subset; real database grouping oracle and validator rejection. |
| Nested ORDER BY aliases can smuggle excluded inputs | Bare output alias only; functions/operators/casts resolve against input reach. Real database ordering oracle and positive bare-alias regression. |
| Unknown nested AST fields, window references and VALUES shapes can evade partial walking | Positive whole-node vocabulary and mutation/negative corpus in `TestSQLRejectsUnknownASTFields` and `TestSQLResolverNestedFailureBoundaries`. |
| Future PostgreSQL majors can invalidate the catalog proof | Probe actual major before interpreting catalogs; reject unqualified majors. `TestQualifiedPostgresMajor` plus real PostgreSQL 17 source acceptance. |
| Failed audit commit could leave a partial source/vector transition | Actual database fault-trigger rollback regressions in `TestSourceStoreRejectsUnscopedOrPartialRecords` and `TestVindexRepositoryBoundsAndAtomicFailure`. |
| An oversized batch could return partially authorized evidence | Atomic bounded response failure; `TestVindexBatchResponseCapIsAtomic` exceeds the real 2 MiB response limit. |
| Caller mutation, incomplete generations, rotation and narrowed authority could reuse stale state | Detached bindings/slices, sealed manifests, immutable context revisions, signed selections and concurrent publish/search/rotation negatives in phase acceptance. |

## Executed development evidence and final gates

The reviewed runtime snapshot `8fe346673500657ebda47dea67e4123e1e0cda84` has retained
source, acceptance logs and real coverage instrumentation in Actions run
`33994776854`. That development run passed the 18 phase-07/08/09 criteria and the
whole uncached race-enabled coverage suite. It is not substituted for verification
of later finalization changes. Final-head check/commit links belong in the PR
verification comment after the read-only permanent workflow completes.

The final suite retains all 40 preceding named criteria, for **58 across phases
01–09**, with no implemented-phase skips. Package gates remain 85% storage/security,
exec/vindex; 80% other internal/SDK packages; 70% CLI. No coverage band was lowered.
Planning, drift, mirror, dependency/format checks, build, vet, lint, lifecycle smoke,
SQL/authority fuzzing and Linux amd64/macOS arm64 CGo-free builds remain final gates.

The real query-plan fixture records PostgreSQL/pgvector versions, dataset and batch
shape. Its small synthetic measurement is not a production warehouse/model latency
claim. Recorded provider responses and deterministic vectors do not measure live
model quality or paid-service availability.

## Honest boundaries

Only the qualified PostgreSQL source subset is executable. Unknown engines, majors,
exposure and parser capabilities fail explicitly. There is no public raw-SQL bypass,
local IAM, retained human JWT, alternative model backend or universal SQL-safety
claim. The full execution product remains phase 10, other engine qualification phase
14 and the Bifrost semantic-to-generation consumer phase 15. Twenty-five future
workstreams remain planned. No paid model call, production deployment or merge is
part of this delivery. Already-issued authority retains its documented offline
revocation window; current supplied JWTs remain mandatory.
