# Grounded calendar v2: offline repair qualification

Observed 2026-10-03. Local recorded-provider / real PostgreSQL evidence only.
The explicit `grounded-calendar-v2` producer repairs the bounded calendar/grouping
phrases; no native analytical, population, group-domain, entailment, or source
proof was weakened. Legacy/default and continuation-v1 requests retain their
old grammar. Publication remains held; hosted CI has not run for this candidate.

## Exact tested source and recovery

- Tested source: `6376e0abf4e2fa7580a998d77e07f4e6449f92f4`
- Tested tree: `2316fea74dc0327371defd7a0ce3ba4999e6c12a`
- Policy/integration patch base: `0fe01c682228ca5fcbbb305cb0f246e2110f508c`
- Base tree: `50bc7eba80dd0f46fea846e7b82476fcaf39acd1`, identical to
  paired-baseline source `d07b364cefe8736b6a868448264fe6b3ac78e888`
- Full policy/integration patch SHA-256:
  `be78e05787794ab8244270f8c6b6e3032b1f9c49a07d769dd3879aded6b71ebb`
- Verified recovery bundle SHA-256:
  `67731649ef1f97a9f39dd6c8aab586f347a01b830247ce46d6ec292c65aff0f8`
- Bundle prerequisite: `8562a36023aa9892199c9c283759fab107c2f08a`,
  tree `9c909ddbdf6fc23f33c7029715d1ebcdfa9e3601`. The bundle includes both
  paired-baseline commits and the repair; the policy patch excludes that baseline.
- Native race binary SHA-256:
  `d4ebc1cac9dcdf5f194b7334148402b8da994be3ce60b2722ba9ff781cdeaf06`
- Qualification receipt SHA-256:
  `81f0b64e34733fd01cf28244a8656626dfa7121209f92ee5320adb6d53b3080d`

The [machine-readable receipt](grounded-calendar-v2-offline-receipt.json) contains
full log/report hashes and verified roots. Its source names the runtime/test/CI
candidate, not this later documentation-only evidence commit. The run checked
tracked content and source/tree identity before and after, with a clean worktree.
Go 1.27.1, `-race`, the pinned native parser/runner and disposable PostgreSQL 17
were used. Build output and database state stayed in owned temporary storage.
No paid traffic or production credentials were used.

## Observed results

- D matched **28/28** unchanged contracts: all 16 original paired controls and
  all 12 fresh paraphrases. It uses B's exact publication, frozen source and
  question/oracle files, opts into only the new producer, and runs before training
  without examples. This is a producer/policy repair result, not a learning gain.
- A/B/C each remained **20/28**, with the same eight failed expected-answer
  cases and unchanged original questions, expected outcomes and actual outcomes.
  All three shared lifecycle/authority controls passed. The actual source-row
  snapshot equals the original `c15e64b` red baseline.
- Training remains **3/12**, producing three independently reviewed examples.
  C consumes them in six cells, including cohort-net; the activity-net eligible
  example remains omitted. Recorded SQL responses and pinned metric IDs do not
  establish live language quality or automatic metric-selection quality.
- The original quality gate remains **false** and calibration **unknown**.
  `TestPairedCohortRecorded` now explicitly asserts that exact failed evaluation
  with `evaluation.ErrGate`, following the Phase24 negative-gate pattern. None
  of the eight answer expectations was reclassified as an expected refusal.
  The original red source/report at `c15e64b` remains immutable.

## Required gates actually executed

Planning-check passed. The affected route/execution/SDK unit-race packages emitted
960 passing test/subtest events and no failures. The existing
`TestSQLRecoveryMySQLCompletenessLocal` test was skipped; no local MySQL or full
repository qualification is claimed.

All six required native roots were explicitly verified as passed, rather than
accepting a regex command's exit code alone:

- `TestGeneratedAdversarialHeldOutRecorded`
- `TestPairedMeaningComparisonControls`
- `TestPairedExpectedFailedEvaluationMutations`
- `TestPairedCohortFrozenLineage`
- `TestPairedCohortRecorded`
- `TestPairedCohortGroundedCalendarV2`

The full native run emitted 31 pass events, zero failures and zero skips. The
mutation gate also ran separately before PostgreSQL/model-fixture work. Its nine
named mutation subtests and additional D corruption assertions reject false
quality-pass flags, wrong classifications, answer relabeling, missing cells,
business/shared substitutions, duplicate shared controls and changed frozen
questions/contracts. The overlapping standalone run is not another quality sample.

The SQL-context workflow selects the five paired roots and requires them plus
14 new route/continuation/SDK roots. Those names were statically checked against
actual Go test functions. Workflow wiring is not hosted execution evidence.

## Safety and predecessor boundaries

Red/green tests cover exact named-zone mismatch and ambiguity, private/quoted
boundaries, negation, scalar/retained targets, source-compatible grouping,
calendar/value alias overlap, parser downgrade and no-new-model replay.
The combined-policy regression reproduced a model changing checked month to
quarter. V2 now preserves its proved grouping with zero additional grouping-role
calls; a combined request without a proved v2 grouping clarifies before that role.
Normal embedding, reranking and SQL generation remain separately accounted for.

`53fecfc` first showed D 28/28 but did not select the pure meaning-comparison root:
its regex named nonexistent `TestPairedManualMeaningComparison`. That omission
was corrected and explicitly verified in later runs. Scalar/retained-zone and
parser downgrade holes found after that run were closed at `1338b70`; the
subsequently discovered combined-policy overwrite was closed before the final
candidate. These predecessors remain evidence, not final safety approvals.
Independent targeted source review found no remaining blockers at `6376e0a`;
static review and runtime results are distinct evidence.

This finite offline result does not close the original live business-gross review
refusal, broad SQL recovery, migration parity, all-engine, hosted, or release gates.
