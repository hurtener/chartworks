# Raw source-amount fixtures v1 and v2

This is a separate test-only input and two-question fixture. It does not repair,
rename or qualify the original all-order gross question. The original live
`needs_review/ambiguous_meaning` result remains preserved in
[the reconstruction qualification record](reconstruction-qualification-2026-10-03.json).
Its source amount components and business calculation remain unspecified.

## Input and numeric contract

[generated_raw_source_amount_v1.json](../../test/acceptance/testdata/generated_raw_source_amount_v1.json)
preserves the original topic identity, description, exact question text, source-fixture hash
and fixed numeric oracle together. The meaning supplied is only
`SUM(orders.total_usd)`, in USD, across all order rows, one row per `order_id`, with
Gregorian UTC calendar grouping by `ordered_at`. Status does not restrict the
population. Gross/net revenue and tax/discount/refund treatment are not supplied.
The original gross and paid input descriptions and questions are unchanged.
The independently versioned [v2 fixture](../../test/acceptance/testdata/generated_raw_source_amount_v2.json)
changes only topic/query identities and the monthly question to supported wording:
“by UTC calendar month in 2026”. The description, source and recorded SQL remain
unchanged. Its live lane explicitly selects the existing `grounded-calendar-v2` interpretation
policy; the shared harness keeps the legacy default for original fixtures.

The source remains the original six-row synthetic commerce fixture. The fixed
sum is **700.00**, with January **320.00**, February **240.00** and March **140.00**.
The harness independently reads source rows and sums exact rational values in Go,
then compares those totals to the fixed fixture values. Query outputs must equal
those independent results. Recorded SQL and generated semantic expressions never
define the result oracle.

## Execution and refusal boundaries

Both recorded and opt-in live lanes use real source profiling, unresolved profile
onboarding, paginated enhancement, whole-candidate advisory, exact explicit
operator review/publication, embeddings, reranked retrieval, NLQ generation and
the native validated SQL execution path. The new lane supplies its own topic name
and question text through small shared harness seams. It does not inject or patch
semantic entities in a generated candidate.

The raw input has a separate operator review: exactly one SUM amount, no population
filters or additional KPI/relationship meaning, USD, order identifier and Gregorian
UTC month policy. Measure descriptions are limited to the declared raw contract and explicit
missing-meaning disclosures; other wording requires review. Unsupported generated
business labels or advisory findings stop the harness before publication.
The name/alias and description checks are exact allowlists: even a truthful
paraphrase can stop for review. A first live attempt may legitimately stop there.
This fixture is not broad generated-business quality evidence, and observed paid
output is not permission to expand the allowlists or waive the advisory. Existing live role settings, budgets, output
reserves, receipt fuse and SQL execution limits are unchanged. A recorded
`ambiguous_meaning` control checks the existing gross refusal gate; it is synthetic
contract evidence, not a replacement for the actual live refusal.

`TestLiveGeneratedRawSourceAmountV2E2E` requires all three explicit settings:
`CHARTWORKS_LIVE_E2E=1`, `CHARTWORKS_LIVE_GENERATED_TOPICS=1`, and
`CHARTWORKS_LIVE_RAW_SOURCE_AMOUNT_V2=1`. Its artifacts use the
`generated-raw-source-amount-v2` prefix and retain the fixture identity/digest,
input/questions, generated candidate, advisory and execution receipts. The original
live tests retain their original artifact names. A future paid execution still
requires the parent budget runner and separate authorization.

## Verification scope

The bounded offline gate includes `TestGeneratedRawSourceAmountV1Contract`,
`TestGeneratedRawSourceAmountV2Contract`,
`TestGeneratedRawSourceAmountV2PipelineRecorded`,
`TestGeneratedOriginalGrossRefusalRecorded`, the original recorded gross and paid
pipelines, and `TestLiveGeneratedTopicConfig`. It does not execute a live provider,
access credentials, change a budget or make a live quality/learning improvement
claim. No broader cohort, scalar schema 6, front-end or release gate is closed.

## Preserved predecessor failure

The first recorded raw-source v1 attempt used the shared legacy interpretation
policy. Its monthly query retained a `2026` date restriction that this route had
not selected, and correctly stopped with `analytical_population_mismatch` before
execution. The total, original gross and original paid controls passed. That
predecessor build began before final test edits and is not exact-source
qualification. The input, questions and recorded SQL remain byte-identical to
fixture SHA-256 `be9f3d0e29aab3002bba6aac5745c0c9ee718463f0c19e8bd13226a5b225796c`.
A second exact-source run selected `grounded-calendar-v2` and still stopped before
SQL generation. Source inspection shows v1’s “by Gregorian calendar month” wording is outside its closed
grouping grammar. Source inspection identifies the unsupported grouping wording; no typed refusal
reason was exposed by the closed diagnostic in that run.
The new v2 identity uses the supported UTC grouping wording and the existing policy.
It retains the year restriction and exact recorded SQL; v1 is not relabeled as passing.

The [predecessor receipts](raw-source-amount-predecessors.json) preserve both v1
failures with their original expected-answer disposition and log hashes. A passing
fixture identity check does not convert either failed answer into quality success.

## Versioned SQL recording correction

V2's first recording also failed with `analytical_population_mismatch`. The
synthetic [diagnosis](raw-source-amount-recording-diagnosis.json) showed that the
route already selected the exact UTC 2026 interval and month grouping. The
recorded provider SQL incorrectly duplicated that service-owned interval with
literal predicates before the service bound its own parameters.

[SQL recording v2](../../test/acceptance/testdata/generated_raw_source_amount_v2_sql_v2.json)
is separately identified and hashed against the unchanged v2 input fixture. It
removes only those premature model-owned predicates. The production service then
inserts validated parameterized date bounds. The test requires the exact UTC
2026 interval, its `ordered_at` binding receipt, both bound date parameters in
final SQL, and the same independent fixed numeric results. Neither v2 question
nor its numeric oracle changes. The initial failed SQL remains in the frozen
input file and predecessor receipt; it is not relabeled as successful.
