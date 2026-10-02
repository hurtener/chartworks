# Grounded grouping-intent qualification

Date: 2026-10-02 UTC. This implementation follows the frozen PR #68 live and
held-out evidence and the PR #69 qualification note. The original three rich
question failures are retained in that evidence. This is a separate new opt-in
producer, not a retrospective relabeling of explicit-selection results.

## Implemented consumer

The ordinary Route/Preflight/Plan and SDK inputs accept
`grouping_intent_policy: "grounded-v1"`. Together with the independent concept
policy, it admits rich English/Spanish questions without caller metric/grouping
IDs. It selects only exact current reviewed catalog choices, binds the redacted
question and current source relations, and preserves unchanged native SQL proof.
Manual grouping and default requests do not pay for grouping inference.
[Contract and limits](../contracts/grounded-grouping-intent-v1.md).

## Executed local evidence

Go 1.27.1, Linux amd64, existing pinned native parser and PostgreSQL 17.6 with
pgvector 0.8.2. All provider responses in these tests are recorded synthetic data;
no live model was called by this gate.

- Complete affected routing, execution, API and SDK race suites: **752 passing
  unit/subtest events across four packages**, zero failures. The pre-existing
  opt-in local MySQL completeness test skipped because its separate database was
  not configured; that engine is not qualified by this run
- Actual generated-topic HTTP/SDK/PostgreSQL consumer plus existing grounded
  concept, private pending, grouping and saved/refinement lifecycle suites:
  **25 passing acceptance events**, zero failures or skips, 33.691 seconds
- Full `go build ./...`, `go vet ./...`, planning/document coherence, mirrored
  contributor instructions and source hygiene: passed
- The new generated-topic acceptance root passed all eight subcases: four
  automatic numeric queries, two early refusals and two pending/manual recoveries

The numeric cases use the same fixed rich questions as the earlier cohort, with
neither metric IDs, references nor grouping supplied by the caller. Independent
results are gross USD700, month320/240/140, paid/cancelled640/60 and Q1=700.
Retained Run issues no new model request. Net-after-refunds and hourly New York
requests stop before remote calls, and the Spanish hourly prompt is now Spanish.

Unit regressions cover unsupported policy/auth/cancellation, repeated spans,
competing date bases, conflicting grains, same local dimension ID in separate
topics, current-source relation drift, tampered and removed proof, scalar-origin
downgrade, actor-bound private origin, saved policy and changed-language versus
manual refinement. Pending `clarify`/`no_match` outcomes stay non-executable;
reviewed coordinates can drive a fresh explicit manual Plan with no grouping
model call. Existing business-answer continuation is not repurposed.

## Remaining qualification

Hosted checks and a live raw-question cohort must qualify the exact published
head separately. Recorded choices test the complete consumer and provider wire,
not multilingual model understanding or calibration. No wider fiscal/DST,
analytical-expression, missing-engine, renderer-kernel, performance or final
release requirement is closed by this increment. Full scope remains in progress.
