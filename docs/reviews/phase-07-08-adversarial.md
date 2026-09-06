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

## Recovery additions

`TestVindexRejectsUnstableCosine` reproduces the pinned pgvector cosine NaN with
finite extreme inputs and rejects both overflow and underflow magnitudes at the
service and database boundaries. The wide documented L2-norm interval is checked
without silently changing normalization. A deliberately invalid distance from the
backend is rejected atomically, with no partial results.

Phase 08 AC04 now rejects all JSON reconstruction of a plan, including attempts
against an existing valid plan: failed reconstruction clears its prior authority
and performs zero warehouse work. The source reference excerpt is exercised
through the actual closed configuration decoder by `TestSourceReferenceExcerpt`.
No lint/coverage exception was added; code and synthetic credentials were corrected
instead of suppressing the reported checks.

## Final fuzz-gate recovery

The stronger iteration-bounded SQL fuzz campaign exposed a setup failure at head
`9d1d4f2a334925aec43ddd6824b3fbc1ff9c7fe4`: [PR CI run 33998461486](https://github.com/hurtener/chartworks/actions/runs/33998461486/job/101392981407)
passed race coverage and all 58 named criteria, then failed during the unchanged
`FuzzSQLTree/seed#1` baseline after approximately 11 seconds. That run is a failure,
not a complete passing release or preflight result.

The pinned parser compiles its WASM runtime on the first `ParseToJSON` call.
Go 1.26.4 separately imposes a ten-second watchdog on each fuzz target invocation;
raising the overall `-timeout` does not change that watchdog. The ordinary test
suite initializes the parser before reaching the fuzz corpus, while a fresh fuzz
worker previously compiled it inside the first timed input.

`FuzzSQLTree` now initializes and validates the real parser/resolver in process
setup before `F.Fuzz`, in both coordinator and fresh workers. The same four seeds,
real pinned parser, resolver, input bounds and race instrumentation remain. The
per-input watchdog and the `-fuzztime=64x -timeout=3m -parallel=2` campaign are not
relaxed. Setup errors fail the test; there is no skipped seed or mock parser.
Final readiness requires the new run to complete baseline coverage and actual
mutations as well as every cumulative gate; exact results belong in the PR comment.

Primary implementation references: [Go 1.26.4 worker watchdog](https://github.com/golang/go/blob/go1.26.4/src/internal/fuzz/worker.go)
and [pinned parser runtime construction](https://github.com/wasilibs/go-pgquery/blob/b511bb3bfd6e3dc19af37ab4c7d44f711995fc7c/parser/parser_wazero.go).

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
