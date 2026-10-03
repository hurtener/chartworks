# Offline paired cohort v1: complete red baseline

Observed 2026-10-03. This is an offline, recorded-provider / real PostgreSQL
baseline. It is **red on unchanged new answer contracts and not ready to integrate
as a green gate**. No production repair, paid request, publication, or live-model
improvement is established.

## Exact measured identity

- Tested source commit: `c15e64baf5f654466965a3d6c844067e13cb7a69`
- Tested source tree: `9af13748d952ec5b50af5c0869871ffe349b6f39`
- Parent source: `8562a36023aa9892199c9c283759fab107c2f08a`
- Full patch SHA-256: `9e40396f443d6e240870912a5362f0b99ba4ed731d3f1133cdcd5e0e4f5b5b3b`
- Native race binary SHA-256: `8533ac936299367938621c122404aa74abc23975adb44cd8a2d116c34a7fac69`
- Complete test log SHA-256: `700111b04eb50589ea9318df4d18d754a53975a14e1177d7a675cae5a3a5cc5b`
- Parsed full report SHA-256: `ba45e765784b117cccad39a2a8bc4782d4b71e679884c4b33b708d5fe7e219c7`
- Frozen cohort manifest SHA-256: `c9fd45583522ca122e8c1bffabec1b3e732b910796f00066ff09e9aa5c7e0df0`
- Observed full source-row snapshot digest: `a0603234683ce70896142f975d5065271205b467da9df609501a2f9cf3696be8`

The commit was created after the run from the exact unchanged patch above; the
post-run diff matched the pre-build archive byte for byte. No executable inputs
changed during that run. The later reporting-label correction described below
has **not** been rerun and must not inherit this exact-source qualification.

The binary was built using Go 1.27.1, `-race -tags nativepinnedlibs`, the existing
pinned native parser/runner, and real disposable PostgreSQL 17/pgvector. The run
selected the original generated-adversarial regression, frozen-lineage controls,
meaning-comparison controls, and complete paired cohort with a 600-second ceiling.
It exited 1. The unchanged original regression passed in 53.73 seconds; both pure
controls passed; the paired test completed all cells and failed in 120.34 seconds.
Planning-check passed on the earlier measured `2f277cf1` harness revision;
no claim of a full repository, hosted, or final-head release gate is made.

## Observed results

- A, B and C each matched 20 of 28 paired contracts: all 16 original paired
  contracts and 4 of 12 fresh paraphrases. The other 8 fresh expectations remain
  `answer`; none was changed to an expected refusal.
- All three shared generated-lifecycle controls passed once: `sensitive-note`,
  `wrong-context`, and `stale-profile`. They are not counted as three measurements
  per arm or as manual-topic profile freshness.
- Training matched 3 of 12 fixed answer contracts: `train-3-en`, `train-4-en`, and
  `train-6-en`. The other nine returned `invalid_temporal_span`, were left failed,
  and supplied no feedback or active example.
- The three successful training cases produced three separately authenticated synthetic
  example-review activations: two `current-owned-predicates-v1` examples and one
  `current-owned-scalar-populations-v1` example. Held-out queries supplied none.
- C actually used reviewed demonstrations in `gross-en`, `month-en`,
  `count-orders`, `count-known`, `cohort-known-net`, and `fresh-known-gross-en`.
  Used IDs/digests were checked against training lineage and the actual rendered
  gateway payload. The required cohort-net consumption control passed.
- `activity-known-net` selected an eligible example that was omitted from the
  rendered prompt. Its full amount/unknown-count oracle was preserved. This is
  not a consumed-learning result, and no token cap was increased to change it.
- No schema-6 binding was observed in this finite cohort. Its unsupported
  learning boundary remains unchanged; this run adds no schema-6 qualification.

Recorded responses make the same SQL-consumer programs available in all arms.
The successful actual-use controls establish transport/lifecycle behavior, not
live improvement. Every arm had the same fresh failures, the declared training
contract failed, and calibration remains unknown.

## Unchanged failed case IDs and reasons

The following seven cases returned `invalid_temporal_span` in every arm:

- `fresh-known-gross-es`
- `fresh-paid-count-en`
- `fresh-known-count-en`
- `fresh-known-count-es`
- `fresh-cohort-net-en`
- `fresh-activity-net-en`
- `fresh-activity-net-es`

`fresh-monthly-known-en` exhausted validation correction with
`analytical_group_domain_review_required` in every arm.

A separately labelled post-comparison B preflight of that unchanged monthly
question resolved the Gregorian 2026 interval in `America/New_York` to
`2026-01-01T05:00:00Z` through `2027-01-01T05:00:00Z`, but retained no requested
month grouping. The selected semantic roots included the order-time dimension,
gross measure and required completeness companion, with no grouping/domain
choice. This supports investigating the bounded phrase producer, rather than
weakening the group-domain validator. This diagnostic made one recorded gateway
request and zero read-execution attempts. Its source metadata/discovery calls
were not independently instrumented and remain unknown. It is excluded from the
84 paired cells and three shared controls.

The finite temporal connectors also require source-backed review before any
extension for named-zone phrases; ignoring “New York” on a UTC or other-zone
topic would be incorrect. No question, oracle, review prompt or production
validator was changed after these observations. A later producer repair must
have a separate immutable runtime/policy revision and preserve this predecessor.

## Reporting-only continuation, not yet rerun

In the measured predecessor, C cells whose Plan failed inherited a default
`no_examples` label. That label is **not authoritative** when there is no final
actual-use receipt. Twelve such cells were pre-dispatch clarifications; four
had recorded gateway work but no final Plan usage receipt. The six used cells
and one omitted cell have verified receipts and remain valid observations.

The subsequent narrow test-harness correction labels those cases
`learning_not_reached` or `usage_unavailable_after_plan_failure`, respectively.
It changes neither outcomes, questions, expected answers, nor production policy.
This correction and this evidence note await the next exact-source run, ideally
alongside the separately reviewed intent-producer policy. Raw predecessor logs
and reports remain immutable; they have not been rewritten to claim this fix ran.

## Later explicit-policy continuation

The separately versioned [grounded calendar v2 qualification](grounded-calendar-v2-offline.md)
replays this exact failed baseline, including the reporting-label correction, and
compares a no-learning D arm. This original red measurement and its expectations
remain unchanged; its historical failure is not a learning-quality success.
