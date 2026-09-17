# CW-01 delivery and adversarial review

Scope: CLR-01, CLR-02 and CLAR-AC01 through CLAR-AC10 software delivery in
[PR #23](https://github.com/hurtener/chartworks/pull/23), branch
`feat/cw-01-conditional-typed-clarification`. Original main baseline:
`35027d93d68c9c2ad3afc4ef26b9deca2b28aa73`; integrated main:
`2219fa29093253e0c51b94c4b9de3a4e52f19ee1`. No merge is performed by this assignment.

The [versioned contract](../contracts/conditional-clarification-v1.md) defines the
implemented scope and unsupported cases. This record separates actual executed
checks from later fixes. Final readiness belongs to the exact-head CI and review
record attached to PR #23, not to a test inventory or a historical green run.

## Executed integrated runtime evidence

[Runtime run 35276259796, job 105387528210](https://github.com/hurtener/chartworks/actions/runs/35276259796/job/105387528210)
executed branch head `e48ad9a2a2c5eba709a97f22a040aa889e8005b1` with main above at
GitHub test merge `036bfbb6384f8df68484e7c0e4789282ae4428fb` on 2026-09-17.
It used Go 1.26.4, race instrumentation, a real PostgreSQL 17/pgvector service,
the pinned native read parser, and recorded model responses. The source and module
lock were verified; no preparation or repair script ran during validation.

Executed commands and observed results:

```sh
go test -race -count=1 -timeout=20m ./test/acceptance \
  -run '^(TestPhase(16|17|18)|TestCW01)$' -v
# PASS: all 10 TestCW01 criteria and all 18 Phase16/17/18 criteria.
# The package completed successfully in 42.722s on this runner.

go test -race -count=1 -timeout=20m \
  ./internal/semantics/... ./internal/exec ./internal/nlq... \
  ./internal/api ./internal/topicapi ./internal/mcpserver \
  ./internal/store/postgres ./sdk/...
# PASS: every package with tests; drafts has no package-local test files.

git diff --exit-code
test -z "$(git ls-files --others --exclude-standard)"
# PASS: validation left the checked-out source unchanged.
```

Named passing nested cases include bilingual named periods; invalid date, grain,
number, precision, boolean and foreign/reference-only options; exact result changes;
disjunction, reviewed aliases and existing filters; unsupported shape with no read;
source-revision invalidation; mandatory-group budget; migration; authoring; missing
export action/resource; actual HTTP/MCP/SDK consumers; saved-query selection identity;
and typed saved-query/reference-choice replay.

The separate [lint job 105387528610](https://github.com/hurtener/chartworks/actions/runs/35276259774/job/105387528610)
passed after the formatting correction at the same branch head. Native shipping
build jobs for Linux amd64 and macOS arm64 also passed in that run. The entire CI
workflow is not inferred green from these individual jobs. Later scalar corrections
below require their own exact-source rerun; this historical run is not reattributed.

## Bounded integration review and corrections

The reviewed path was answer admission -> versioned canonical resolution -> sealed
mandatory context -> service-owned parameter binding -> ordinary validator/read
receipt -> persisted query -> same-session and saved-query replay. Review included
current publication/source/reach changes, duplicate/foreign answers, immutable
parent evidence, sensitive values, defaults/conflicts and legacy import behavior.
This is source-level adversarial review with regression checks, not an invented
independent reviewer or representative-user study.

Earlier concrete integration corrections retained by this branch:

- The question lifecycle now exposes typed answers and a retained preflight origin;
  admission no longer overwrites the router's redacted canonical request with raw
  submitted strings. Answer edits do not reuse previously filtered SQL.
- The retained saved-query projection now includes protected clarification evidence
  required by its shared scanner. Preparation/replay preserves exact source and
  parameter identity instead of discarding the typed binding record.
- Clarification export now uses the existing signed export reach. Missing action
  and hidden-resource denials preserve their distinct nondisclosing contracts;
  neither direct service nor HTTP response returns the protected definition.
- The real profile fixture uses coherent publication provenance; the mandatory
  constraint regression checks the deterministic order actually owned by the
  assembler. Constraints were not dropped or relaxed to satisfy an assertion.
- Duplicate logging methods, obsolete unused helpers and lint findings were fixed
  without lowering lint or coverage gates. The final saved-query formatting defect
  was corrected in `e48ad9a2a2c5eba709a97f22a040aa889e8005b1` and the full lint job passed.

## Scalar-boundary adversarial review

Focused follow-up over the integrated binder and parser identified three semantic
correctness findings. They do not grant authority, but could violate a reviewed
business constraint or postpone a predictable error until source work.

| Finding | Concrete correction | Regression |
|---|---|---|
| Native instant type also accepted as wall-clock; wrong wall-clock cast | Commit `f6eb70e7e2db65daa90e3e67dd656e981dd795a0` separates native timestamp identities and uses the native non-timezone cast. | `TestBusinessTemporalSourceTypeIdentity`, `TestBusinessTemporalNativeMismatchFailsBeforeBinding` |
| Empty or `Local` timezone could use runtime defaults | Commit `44ecb15f00888adf2dee29644c24b1d29e942862` rejects implicit timezone pins at authoring, parsing and binding; explicit UTC remains valid. | `TestClarificationRejectsImplicitTimezones`, `TestBusinessBindingRejectsImplicitTimezones` |
| Decimal selection ignored native integral-digit capacity | Commit `9163c1fb9e0b1d760eabf50ac0e8f1ab31f89dc1` checks the full declared native domain before provider work and selects the correct exact decimal cast. | `TestBusinessDecimalNativeIntegralCapacity` |

Primary contract checks: [Go LoadLocation](https://pkg.go.dev/time#LoadLocation),
[native timestamp](https://docs.databricks.com/aws/en/sql/language-manual/data-types/timestamp-type),
[native wall-clock timestamp](https://docs.databricks.com/aws/en/sql/language-manual/data-types/timestamp-ntz-type),
and [decimal precision/scale](https://docs.cloud.google.com/bigquery/docs/reference/standard-sql/data-types#decimal_types).
The new native scalar tests are synthetic adapter contracts, not live cloud tests.

## Delivery boundaries

The owned map dispositions describe native conditional clarification and typed
propagation, not complete behavioral parity or foreign-system cutover. Existing
policies without a reviewed condition retain explicit reference-only migration;
non-reference legacy values require review instead of dismissing a blocker.

Scalar binding intentionally supports the documented qualified SELECT/join/filter/
grouping subset. Unsupported CTE, nested/set, ambiguous target or native type/grain
combinations fail explicitly; a business answer never substitutes for validator-
issued read proof. Existing queries without scalar clarification keep their prior
SQL support. Date inputs use explicit Gregorian/IANA policy and aligned supported
grains; arbitrary natural-language interpretation is not claimed.

CLAR-AC11 representative-user comprehension research was not performed. No live
model quality, production latency, cloud qualification or deployment is claimed.
The final branch must exclude temporary editing/offline-toolchain workflows and
must pass the unchanged read-only CI gates. Final checks are recorded against the
actual cleaned PR head and its test-merge SHA in the PR review evidence.
