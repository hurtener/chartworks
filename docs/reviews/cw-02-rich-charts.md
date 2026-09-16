# CW-02: rich charts and deterministic selection review

## Scope and baseline

PR #22 targets main from `35027d93d68c9c2ad3afc4ef26b9deca2b28aa73` on
`feat/cw-02-rich-chart-shapes`. Backend checkpoint
`7abace5b09b670b21de9739698720a6fdddff4b0` was preserved, then actual viewer and
saved-lifecycle coverage landed at `bdcd661bedf0f6599d58b24cbb214af62e45f820`.
The requirements-map update is scoped to VIS-02/a/b, VIS-04/a and their chart
matrix. Historical material and unrelated owner findings are retained unchanged.

The review is a bounded author adversarial review, not an independent approval,
a live-provider study or a performance certification. The coordinator owns merge.
Two focused passes traced actual boundaries and then the fixes; CI findings were
handled as targeted follow-ups rather than repeated unbounded redesign.

## Pass 1: saved meaning, transformation and actual consumers

| Finding | Fix / regression |
|---|---|
| Rich structs alone did not make the scalar viewer understand repeated measures, bubble size or deep hierarchy. | The actual bundled component consumes v2 series identities, per-unit scale panels, numeric area-size geometry and nested hierarchy nodes. Shared synthetic fixtures build from deserialized mappings; actual browser assertions verify every measure and the new geometry. |
| An early undrawable-state return could erase exact retained evidence. | Rich wide rows, original row indices, nulls, exact series observations and hierarchy aggregates remain accessible even with no geometry. All-missing, all-zero-size and all-null-path cases assert retained table presence and absent drawings. |
| Frozen reuse still identified the old scalar builder version. | Reuse keys use `charts.BuildVersion` independently from wire version. The real saved-lifecycle test recomputes the retained manifest key and rejects the old builder identity. Published v1 definitions are not mutated. |
| Duplicate tuples hidden by null values could otherwise permit ambiguous rendering. | Binding validation rejects canonical category/series tuples, heatmap cells and complete leaf paths before omission. Exact numeric/time identity is distinct from literal labels. Regressions cover duplicate/null/negative/invalid and expansion limits. |

The save/read/build/view chain crosses the closed HTTP API and public SDK, real
PostgreSQL persistence, validated source execution, immutable publication, frozen
fanout and the current authorized Apps provider. Thirteen outputs share one query;
retained read/rebuild/view counters assert no source/model work. V1 table coexistence,
rich source-revision drift, type/unit/grain pins and detached rebinding are covered
by lifecycle and core regressions. No new local identity or approval mechanism was
introduced.

## Pass 2: drawing fidelity and evidence quality

| Finding | Fix / regression |
|---|---|
| A minimum bar thickness could overlap densely packed independent series. Default outlines could still exceed otherwise-correct slots. | Rectangle thickness stays within its slot and rich/comparison bar outlines have zero width. A 100-category, five-breakdown, two-measure browser fixture checks 1,000 rectangles, geometric separation and computed stroke width. |
| Distinct-unit series on one numerical scale could imply a false comparison. | Independently labeled panels group only identical unit/currency/percentage representations. Series order, names, identity and exact values survive; browser tests check panel counts and all per-series glyphs. |
| Reusing an already-visible generic error could let a later malformed-output test pass without processing its input. | Every malformed rich version/series/tuple/tree/size case first restores a uniquely acknowledged valid generation. Tests then require cleared chart/table state, not merely an old error message. |
| A ranker fixture with repeated categories legitimately left only scatter suitable, so its expected model call was wrong. | The first case now asserts `not_applicable`, zero calls and retained bubble binding. A second case uses distinct synthetic labels so honest alternatives exist, then asserts one gateway call and metadata-only canaries. Suitability and ranking gates are not weakened to satisfy the test. |
| CI lint reported chained intent branches and English-spelling false positives for intentional Spanish cues. | Equivalent switches preserve scores; two local, explained spelling suppressions preserve unaccented Spanish input. Dedicated tests assert composition selection and relationship selection without inventing bubble size. No global lint exclusion was added. |

Selection review traced intent classification, typed roles, retained cardinality,
semantic additive signals, candidate binding, suitability/floor/limit sealing,
stable ties and optional permutation. No ranker can recover a discarded candidate.
The actual API/gateway fixture excludes the raw question, labels, exact numeric
values and source/topic pins from ranking metadata. No alternate model client,
SQL regeneration, authority widening or mutable publication was found in this
scoped change.

## Actual validation evidence and final gate

The first continuation run was Reporting contracts
[35060782847](https://github.com/hurtener/chartworks/actions/runs/35060782847),
which tested merge candidate `3cdb8f9e4bd5860f1ee721578a25e9edb95e5f5a`
for head `bdcd661bedf0f6599d58b24cbb214af62e45f820`.
It passed all eight strict phase 30 criteria, all eight phase 31 criteria including
the actual Chromium component, broad reporting/API/store/gateway/SDK regressions,
planning, build and vet. Its new closed-HTTP ranking expectation failed as described
above; the saved-lifecycle subtest passed. This failed run is not represented as a
green final acceptance result. The CI lint findings were also fixed after their
actual report.

Fresh local Go 1.26.4 `go test -race -count=1 -cover ./internal/charts
./web/report-viewer` passed after the fixes: chart-core statement coverage 91.5%;
embedded-viewer Go package coverage 100%. The latter is not browser JavaScript
coverage. Both JavaScript syntax checks, Go formatting and `git diff --check`
passed locally. The local browser fixture could not initialize in the container;
real browser/SQL/native acceptance was executed in the repository's hosted CI,
not replaced with a static DOM test or a skipped criterion.

Final readiness requires a fresh green run of the committed head/merge candidate,
including `TestCW02RichCharts`, strict phase 31 browser checks, lint and applicable
repository checks. The PR's head SHA, checks and final delivery receipt are the
authoritative final evidence; a preceding run or the counts in this document do not
substitute for that gate. The checked-in workflows remain read-only against
committed source: no preparation script repairs code, no test-generated commit,
and no relaxed coverage or missing-test allowance is part of this PR.

## Explicit limitations and unchanged ownership

The catalog is twelve plots plus KPI/table, not unlimited variants. Hierarchy has
a hard eight-level ceiling; rich expansion, series/cardinality and byte limits
reject unsuitable sizes. Line/area align observed ordered positions, not elapsed
time or inferred calendar buckets. Approximate or subpixel drawing never replaces
exact accessible values. Hierarchy prefix sums cover returned complete paths;
truncated totals never claim whole-result/source scope.

Intent is a bounded English/Spanish rule classifier, not a live-model quality claim
or an NLQ clarification policy. JSON compatibility is implemented; standalone
import/export, static rendering, broader locale/display formatting, richer reporting
KPI/table/narrative definition policies and whole-dashboard layout remain separately
owned work. Existing signed identity/issuer/context isolation, immutable publication,
execution validation, budgets and v1 saved mapping interfaces remain in force.
