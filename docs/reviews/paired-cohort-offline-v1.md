# Paired topic and reviewed SQL-example cohort, offline v1

Status: implementation candidate; independent review and runtime qualification
pending. This is a newly constructed finite cohort, not a recovered historical
96-case suite. It does not qualify live model improvement or a release gate.

## Frozen scope and lineage

`test/acceptance/testdata/paired_cohort_v1/manifest.json` binds the original
warehouse, author inputs, net business meanings, independent oracle and 19
regression controls by SHA-256. It separately binds the original design proposal,
12 new EN/ES paraphrases and 12 predeclared training-only questions. The partition
is frozen and checked before gateway construction, and its actual evaluation
suite is registered and reviewed before any recorded provider request. Exact
question identities are disjoint. Semantic families and source rows are shared;
this is not a test of unseen business concepts. Training expectations come from
the same independent exact-rational source-row calculation, never generated SQL.

The test derives a fixed source snapshot hash, runs all arms on the same physical
source/context/profile revisions, and checks that source rows are unchanged
before the separate destructive stale-profile control. Expected outcomes are
never rewritten after observing a response. The original live amount-meaning
refusal and its review instructions are untouched.

## Meaning of the arms

- A: independently coded manual definitions from the same profile scaffold and
  business contract; ordinary explicit owner-authenticated review/publication;
  no learning examples.
- B: the existing profile → paginated enhancement → complete-topic advisory →
  explicit owner-authenticated review/publication path with recorded responses;
  no learning examples.
- C: B's exact publication, after training-only successful native results produce
  feedback and separately reviewed SQL examples. All training plans run before
  activation, so training collection is also example-free. Each held-out plan
  has a fresh operation/query identity and measured gateway dispatch.
- D: absent. No production proof-consumer change is made by this harness.

Metric IDs are pinned equally in every arm. The semantic-equivalence digest
compares enforced physical, population, temporal, completeness, vocabulary,
relationship and KPI bindings, excluding prose, aliases and lifecycle identity.
This is a controlled SQL-consumer comparison. It does not establish automatic
metric selection or exact language-meaning equivalence. C specifically measures
reviewed SQL-example learning, not topic-meaning improvement. The separately
existing semantic-feedback proposal workflow is not silently folded into C.

Topic drafts are actor/session-private. Their current API requires the owner's
explicit review and does not permit cross-actor draft review. The harness retains
that boundary and an independent deterministic contract check; it does not clone
a generated draft to remove advisory requirements. Synthetic operator checks and
test-generated review receipts are lifecycle evidence, not independent human
approval of the experiment. The evaluation manifest uses its actual distinct
reviewer lifecycle. Its calibration is unknown and the service must refuse to
execute it as a reviewed passing gate.

## Oracles and learning receipts

Native numerical results are checked against independently computed exact money,
count, NULL/month and amount-completeness outputs. Deliberate wrong-money and
NULL-to-zero overlays must fail the same scoring path on genuine native results.
The five clarification and three unsafe-SQL controls remain paired and
nonexecuting. Three original lifecycle controls (private authoring-note exclusion,
wrong-context authoring vocabulary and stale generated advisory) execute once
in `shared_controls`, bound to the exact generated publication. They are not
counted three times or claimed as manual-A freshness: the comparison has 84
paired cells (16 original controls plus 12 fresh paraphrases, across three arms)
and three shared lifecycle controls. Security failure tolerance is zero. Failed answers remain failed
answers; a unsupported learning disposition cannot excuse their result mismatch.

C retains eligibility/selection plus used/omitted receipts and exact reviewed
training example digests. Actual used demonstrations must occur in both the
persisted final generation prompt and intercepted recorded gateway request.
An eligible but omitted example is explicitly not consumed learning. The
predeclared `C/cohort-known-net` control must consume a supported reviewed example.
Any consumed ID outside training lineage fails. Schema-6 learning remains
unsupported and may not produce or consume examples; its ordinary A/B/C answer
oracle remains unchanged. No token-cap increase or reduced semantic packet is
used to make learning fit.

## Running and interpreting results

Run `TestPairedCohortFrozenLineage` and `TestPairedCohortRecorded` with the existing
native-pinned acceptance toolchain, real disposable PostgreSQL/pgvector and
recorded gateway fixture. The final test log emits `PAIRED_COHORT_REPORT` with exact
manifest, source and semantic digests, typed cells, native-result digests, model
call/read-attempt counts, training records and actual-use receipts. Cost stays
unknown rather than being represented as paid-provider zero. No provider
credentials or live traffic are required or authorized by these tests.

Qualification must report source revision, patch, binary hash, full raw test
output and failures separately. A green recorded run would establish only this
finite lifecycle/consumer contract. The unknown quality gate, independent
external review, live model quality, source-engine breadth and any future
semantic-learning comparison remain separate open work.
