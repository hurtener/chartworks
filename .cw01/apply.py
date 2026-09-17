"""Update only CW-01 dispositions, owning contracts, and actual review evidence."""
from pathlib import Path
import subprocess

expected = {
    "docs/gap-analysis.md": "a1e6c17aead99063c54bc24e01ec150fff973a65",
    "docs/contracts/conditional-clarification-v1.md": "0ba6ae760c662002335b6db75687abb2ba4eb29c",
    "docs/contracts/rule-lifecycle-v1.md": "106aa5b647267fccdfae44fdcd8f9dd5e31d78d8",
    "docs/contracts/semantic-foundation-v1.md": "9beaa4ff9a30623aea3922c6d7559e8ba8901f29",
    "docs/plans/phase-16-rules-clarification.md": "4e24ebcd0305349efa6ed3ddfa954a003d54ec21",
    "docs/plans/phase-17-nlq-routing-context.md": "ad43c5751a4440976e9041392cfdbc3fe16c810b",
    "docs/plans/phase-18-nlq-generation-execution.md": "3cded77cc91e8015d2e39b758ec75d96c38642ac",
    "docs/reviews/cw-01-delivery-review.md": "0cc0c6bd0f753a16d2fc1c6f25cad9d04eee3c34",
}
pending = {}
for path, sha in expected.items():
    if subprocess.check_output(["git", "hash-object", path], text=True).strip() != sha:
        raise SystemExit(f"{path}: inspected document changed; reconcile before writing")
    pending[path] = Path(path).read_text()

def replace(path, before, after):
    if pending[path].count(before) != 1:
        raise SystemExit(f"{path}: reviewed anchor changed")
    pending[path] = pending[path].replace(before, after, 1)

path = "docs/gap-analysis.md"
replace(path,
        '| CLR-01 | Clarification | Required clarification slots lack question-specific activation | confirmed gap | 16/17 |',
        '| CLR-01 | Clarification | Required clarification slots lack question-specific activation | conditional reviewed policy/runtime implemented; explicit legacy migration | 16/17 |')
replace(path,
        '| CLR-02 | Clarification | Typed clarification answers can have no planning effect | confirmed gap | 16/17/18 |',
        '| CLR-02 | Clarification | Typed clarification answers can have no planning effect | typed binding and session replay implemented; bounded native SQL subset | 16/17/18 |')
updates = {
    "CLR-01": (
        "conditional reviewed applicability implemented by CW-01; literal/reference matching is deterministic, not a calibrated general-language interpreter.",
        "Reviewed topic/ruleset/policy pins, literal token phrases and exact semantic references select applicable policies. Not-applicable, satisfied, missing, invalid and conflicting outcomes are distinct. Blockers use stable specificity/priority/ID ordering and dependency-aware question groups; optional defaults are visible. Legacy patterns remain explicit reference-only selections until reviewed migration, so old patterns gain no accidental new blocking.",
        "[conditional evaluator](../internal/semantics/clarification_evaluate.go), [route consumer](../internal/nlqroute/clarification.go), [authoring/import](../internal/semantics/rulesets/clarification_authoring.go), and [AC01/06/07/10 acceptance](../test/acceptance/cw01_test.go)"),
    "CLR-02": (
        "typed resolution, mandatory-context and native binding consumers implemented by CW-01; unsupported SQL/type combinations fail explicitly.",
        "Reference options, exact numbers/ranges, explicit calendar/time windows, booleans, governed entities and bounded text resolve before provider work. Canonical evidence is pinned to the current source/topic/policy and actor/session, budgeted as a whole mandatory group, and protected before persistence. Service-owned predicates and parameters pass the existing validator and read executor; correction/removal supersedes the prior same-session value. Saved-query preparation/replay retains the protected binding evidence and checks current authority without retaining bearer tokens.",
        "[value resolver](../internal/semantics/clarification_values.go), [binding](../internal/exec/business_sql.go), [query/refinement evidence](../internal/nlqexec/clarification.go), [AC02–05/08/09 acceptance](../test/acceptance/cw01_test.go), and [saved consumers](../test/acceptance/cw01_saved_test.go)"),
}
for finding, (disposition, delivered, evidence) in updates.items():
    start = pending[path].index("### " + finding + " —")
    end = pending[path].find("\n### ", start + 1)
    if end == -1:
        raise SystemExit("missing owned finding boundary")
    section = pending[path][start:end]
    if section.count("- **Disposition:** confirmed gap.") != 1:
        raise SystemExit("owned disposition changed")
    section = section.replace("- **Disposition:** confirmed gap.", "- **Disposition:** " + disposition, 1)
    section = section.replace("- **Current boundary:**", "- **Sept 15 baseline boundary:**", 1)
    section = section.replace("- **Current repository evidence:**", "- **Historical implementation pointers:**", 1)
    section += "\n- **CW-01 delivery:** " + delivered + "\n- **Current evidence:** " + evidence + ". See the [versioned contract](contracts/conditional-clarification-v1.md) and [executed evidence/review](reviews/cw-01-delivery-review.md).\n"
    pending[path] = pending[path][:start] + section + pending[path][end:]
replace(path,
        "## Clarification and underspecification UX expansion\n",
        "## Clarification and underspecification UX expansion\n\n**CW-01 delivery update (2026-09-17):** CLR-01 and CLR-02 now have a native conditional/typed implementation and actual `TestCW01/AC01`–`AC10` consumer acceptance. The [current contract](contracts/conditional-clarification-v1.md) supersedes the proposal-only runtime boundaries below. [Review evidence](reviews/cw-01-delivery-review.md) distinguishes tested commits from later review fixes and final PR checks. CLAR-AC11 remains separate, unperformed representative-user research; no comprehension, live-model quality or full foreign-cutover claim is made. Other findings and the dated original inspection remain unchanged.\n")

pending["docs/contracts/conditional-clarification-v1.md"] += """

## Reviewed scalar boundary corrections

Timezone is an explicit reviewed IANA location or `UTC`; empty and `Local` runtime
defaults are rejected by authoring, value resolution and business binding. A host's
local timezone is never a substitute for a publication pin. Native timestamp
identity remains distinct: instant-bearing `TIMESTAMP` is not a wall-clock target
where the adapter defines it as an instant, and wall-clock bindings use the native
non-timezone cast rather than a session-dependent cast.

Exact numeric admission checks both fractional and integral digit capacity. The
BigQuery binding selects `NUMERIC` only when the declared scale is at most 9 and
integral capacity at most 29; otherwise a supported declaration uses `BIGNUMERIC`,
with at most 38 integral and 38 fractional digits. A declaration outside the fully
representable domain fails before provider work, even when one particular answer
is small. No rounding, floating-point conversion or partially representable extra
digit is used to make a declaration appear supported.

These corrections are covered by the native scalar-boundary regression tests in
`internal/exec/business_temporal_test.go`, `business_timezone_test.go`, and
`business_precision_test.go`, plus
`internal/semantics/clarification_timezone_test.go`. They do not expand the SQL
shape subset or qualify a live cloud deployment.
"""

for name in ("rule-lifecycle-v1.md", "semantic-foundation-v1.md"):
    pending["docs/contracts/" + name] += """

## CW-01 conditional clarification extension

The [conditional clarification v1 contract](conditional-clarification-v1.md)
supersedes the initial slot-only clarification description above. Reviewed policies
now select applicable questions; accepted answers become exact references or typed
business constraints. The existing draft/review/publication and current signed
source/context checks remain mandatory. Legacy definitions retain their digest and
explicit reference-only migration disposition; they do not acquire new required
blockers or inferred non-reference effects.

Typed resolution, dependency-aware questions, whole-group tokenizer admission,
protected persistence, native binding, correction/removal and current-authority
replay are implemented through the existing routing/query services. Draft preview,
replay/shadow and exact-topic import/export have API/SDK consumers. This extension
adds no issuer, grants, alternative validator or model gateway. Actual software
acceptance and bounded review are in the [CW-01 evidence](../reviews/cw-01-delivery-review.md);
representative-user comprehension remains a separate, unperformed study.
"""

for name in ("phase-16-rules-clarification.md", "phase-17-nlq-routing-context.md", "phase-18-nlq-generation-execution.md"):
    pending["docs/plans/" + name] += """

## CW-01 delivered clarification extension

The [conditional clarification contract](../contracts/conditional-clarification-v1.md)
extends this phase's existing core, without changing authority ownership, immutable
publication, source partitions or validated-read prerequisites. It supplies reviewed
conditional applicability and typed answer resolution, mandatory-group token/privacy
handling, source-bound parameter effects, session correction/removal and retained
consumer replay. Preview, replay/shadow and safe import dispositions use the existing
API/SDK surfaces rather than a new authoring application.

`TestCW01/AC01` through `TestCW01/AC10` add the clarification acceptance corpus;
the six existing `TestPhase16`, `TestPhase17` and `TestPhase18` criteria remain
unchanged and continue to run. [CW-01 delivery evidence](../reviews/cw-01-delivery-review.md)
records exact executed checks and review corrections. No CLAR-AC11 comprehension
study, live cloud/model quality measurement or general SQL-shape expansion is
claimed by these software tests.
"""

pending["docs/reviews/cw-01-delivery-review.md"] = """# CW-01 delivery and adversarial review

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
go test -race -count=1 -timeout=20m ./test/acceptance \\
  -run '^(TestPhase(16|17|18)|TestCW01)$' -v
# PASS: all 10 TestCW01 criteria and all 18 Phase16/17/18 criteria.
# The package completed successfully in 42.722s on this runner.

go test -race -count=1 -timeout=20m \\
  ./internal/semantics/... ./internal/exec ./internal/nlq... \\
  ./internal/api ./internal/topicapi ./internal/mcpserver \\
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
"""

for path, text in sorted(pending.items()):
    Path(path).write_text(text)
print("Recorded owned clarification dispositions, contract extension and executed review evidence")
