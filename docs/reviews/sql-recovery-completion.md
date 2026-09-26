# SQL recovery — completion tracker

PR #62; implementation baseline `ea38c0774ac9f16bc7a7d3ffeeb4f26b666320c7`.
Latest qualified implementation: `996db063d214ae086f4fe4383028348abe9d7282`;
qualified tree: `56f8d6ad5cdffad092b1df86b85679b6ded12a77`.
Updated 2026-09-26. This is the current AP-00–AP-08 tracker. Historical
checkpoint prose in the [recovery ledger](sql-context-recovery.md) is evidence
at its stated revision, not the current completion status.

**Overall: in progress.** All nine phases have delivered increments; none is
claimed fully closed. A fixed defect review, a safe unsupported disposition, and
a passing recorded-provider test do not complete missing product behavior or
establish migration parity. Phase 16–18 shipped labels describe their existing
named criteria, not blanket completion of this subsequent recovery extension.
No scope has been discarded and no release or cutover has been approved.

## Phase status

| Phase | Delivered implementation | Remaining implementation | Qualification still required |
|---|---|---|---|
| AP-00 | Exact source snapshots; engine and effective provider-wire capture; EN/ES synthetic fixtures | Paired cohort execution/reporting as needed by the existing evaluation runtime | Same-snapshot original/replacement owner cohort with frozen expected results; opt-in live diagnostics |
| AP-01 | Catalog-backed atomic dependencies; nested KPI closure; selected roots shared with rules and clarification; provenance, omissions and replay; S1 dependent applicability qualified at 6273c80 | S2: reviewed, bounded paraphrase selection | Held-out selection/ambiguity calibration, not similarity-as-confidence |
| AP-02 | Metric/dimension/multi-topic prompt projection; ranked examples and actual-use evidence; strategy and effective-envelope fitting | S3: minimal confirmed-join-aware projection | Operator tokenizer/model-window/framing qualification |
| AP-03 | Native-first selected metrics, exact arithmetic, independent populations, direct/calendar grain, typed WHERE/HAVING and versioned replay | S4: reviewed intent for order/limit/filter and broader expressions; S5: physically backed join/fan-out proof and required nested/multi-relation forms; S6: required dialect proof | Business-result cohorts and actual required engines; safe rejection is not support |
| AP-04 | Targeted unbound SQL repair, private binding restoration, bounded attempts, terminal exclusions and actual durable source-error path | S7: additional safe diagnostic/correction classes where acceptance requires them | Broader privacy, repair/result and engine matrix within existing attempt limits |
| AP-05 | Protected parent lineage; typed reference/metric edits; model-slot custody/replacement; S8 governed scalar/calendar inheritance, replacement/removal, empty-state policy, SDK, saved views and replay qualified at 93539cc; S9 grouping inheritance/replacement/totals/calendar and pending refinement qualified at 996db06 | S9 remainder: wider native parameter-role proof for required complex query shapes | Wider language and owner journeys beyond the checked EN/ES continuation grammar |
| AP-06 | Durable accepted redacted explanations; value-free typed learned examples through review, persistence, import/export and generation | S10: reviewed service-owned predicate learning and additional required parameter domains | Parameter-domain/probe and dialect matrix; owner result quality |
| AP-07 | One generator/validator name vocabulary across six dialect profiles | S11: proven syntax/function signature/type matrix shared at generation and validation boundaries | Actual required engines plus selected analytical cases |
| AP-08 | Strict ready/clarify/insufficient-context outcomes; blocks cannot yield an executable plan; bounded redacted questions and HTTP/MCP/SDK parity | S12: durable pending-question/resumption and richer reviewed choices | Calibrated ambiguity detection, paired result quality and release evidence |

## Completion checklist

The S IDs identify a finite remaining backlog, not replacement phases. An entry
closes only with implemented consumers, negative tests, applicable persistence and
surface behavior, and exact-source executed evidence. Narrow delivered subsets
remain described explicitly; they do not close the parent requirement.

- [x] S1 — Answer-dependent cross-pattern applicability, fixed-point convergence,
  inactive answers, contradiction/cycle bounds and deterministic replay.
- [ ] S2 — Free-text paraphrase selection grounded in authorized reviewed concepts,
  confidence/ambiguity handling, complete closure and no incidental-root promotion.
- [ ] S3 — Confirmed join coordinates reach render-only projection; join keys stay,
  unrelated columns drop, full validation authority and ambiguous-path behavior stay.
- [ ] S4 — Reviewed ordering/limit/filter intent and required analytical expression
  coverage; exact semantics and bounded correction rather than SQL string equality.
- [ ] S5 — Physical uniqueness/cardinality-backed joins, aggregation placement and
  accepted nested/multi-relation shapes; adversarial duplicate/NULL/missing-row cases.
- [ ] S6 — Per-dialect analytical proof for the required engine/query matrix.
- [ ] S7 — Additional bounded diagnostic/correction coverage, never raw driver text
  or uncertain execution as permission to repair.
- [x] S8 — Inferred reviewed governed-value/calendar context inherited across
  abbreviated follow-ups; explicit replacement/removal, exact retained anchor,
  empty-state parser policy, private binding boundaries and replay. Qualified
  93539cc in the documented deterministic EN/ES scope; general language remains S2.
- [ ] S9 — Grouping continuity, explicit totals/calendar replacement, saved state and
  pending refinement qualified at 996db06. Wider required native parameter-role
  proofs remain open; no authority comes from prior SQL.
- [ ] S10 — Reviewed reusable learning for service-owned predicates and required
  parameter domains, with no historical value, authority or default leakage.
- [ ] S11 — Proven syntax/signature/type matrix and generator/validator agreement.
- [ ] S12 — Protected durable pending-question state, exact choice/answer origin,
  resumable resolution, expiry/current context checks and bounded model work.
- [ ] Q1 — Protected original/replacement comparison on the same reviewed definitions,
  source snapshot, question/locale/anchor and independently frozen expected results.
- [ ] Q2 — Required real-engine/model/tokenizer configuration qualification; recorded
  fixtures and opt-in skipped live tests never counted as live passes.
- [ ] Q3 — Final integrated regression/adversarial/release and owner result evidence.

Implementation order: S1/S8/S9 (continuity), S2 (selection), S3/S5/S4/S6
(analytical breadth), S7/S10/S11/S12, then the paired and release qualification.
S5/S6 must not be bypassed with an allowlist-only or metadata-only certificate.
Q1/Q2/Q3 require their actual protected inputs and applicable engines; missing
inputs are qualification blockers, not grounds to mark software implemented.

## Qualified baseline and review

At `ea38c07`, standard CI run **36141722201** and expanded recovery run
**36141722169** succeeded. Each Go 1.26.4/1.27.1 job recorded 1,630 passing
unit/subtest events across 20 packages and 187 acceptance events with no
acceptance skips. The existing paid live-reranker unit test was skipped and is
not a live pass. The two toolchains repeat the same scenarios.

Four P1 findings (literal typing, durable query-error correction, abandoned
terminal custody and stale result rows) and one P2 ONLY defense-in-depth finding
were closed within that reviewed scope. See the
[adversarial review](sql-adversarial-p0-p1.md). This historical green baseline
is not evidence for changes made after `ea38c07` and does not certify all possible
P0/P1 absence. New checkpoints must state their own validation results.

## Tracker ownership and synchronization

This file controls AP completion. The PR body mirrors its delivered/open state.
The [recovery ledger](sql-context-recovery.md), [projection review](sql-scoped-projection.md),
[plans index](../plans/README.md), [gap analysis](../gap-analysis.md), and owning
plans 16, 17, 18, 24 and 34 link here. The existing 34-phase registry and
224-criterion coverage map retain their established scope/status; AP increments
are not new numbered phases and do not alter historical acceptance counts.
Historical checkpoint logs, decisions and named criteria are not overwritten.

## S1 implementation checkpoint — dependent clarification

The evaluator now resolves answer-dependent reference/effect applicability to a
bounded positive fixed point. Inactive submitted answers cannot seed it; invalid
or conflicting answers return no partial resolution group. Original choices stay
separate from derived facts. Reachable typed branches are source-pinned during
initial preflight so a later controlling answer does not invalidate its own
answer context. This preflight hint is not retained or accepted from JSON.

New unit/route regressions cover English/Spanish sequential and simultaneous
answers, exact values, inactive/disabled branches, cycles, defaults, contradiction,
reference bounds, independent choice provenance, detached concurrent reuse, source
rotation and canonical pending/accepted replay. S1 qualification completed on the exact published tree: standard CI run
36168685672 and recovery run 36168685555 passed. Both Go 1.26.4 and 1.27.1
artifacts were downloaded and their JSON terminal events parsed: 1,647 passing
unit/subtest events and 193 passing acceptance events per toolchain, with no
failures or acceptance skips. The sole unit skip is the existing opt-in paid
reranker. All ten required S1 unit tests and both new PostgreSQL lifecycle/invalid
acceptance tests passed. Recorded model responses are not live quality evidence.

## S8 implementation checkpoint — retained inferred intent

[Interpretation continuity v1](../contracts/interpretation-continuity-v1.md) makes
resolved reviewed value IDs/operators and exact calendar intervals durable through
abbreviated refinements. The current router re-resolves all fields and owned
predicates; source/parent authority is unchanged. New recognized language replaces
the same dimension; explicit edits can remove/replace values or intervals. The
original anchor is retained and current/retained requests replay deterministically.
SDK and saved/clarification-origin paths use the same request contract. This
checkpoint was initially unqualified; the exact corrected S8 qualification below
now controls its status. Historical failed runs are not relabeled passes.

## S8 removal correction — historical failing checkpoint

Observed baseline `d4d64b4`: the existing suite passed units, but typed removal of
all inferred predicates failed acceptance. Its unrelated, empty answer context
was being carried as if it authenticated an answer. The correction clears that
input pin only after parent authorization/replay when no reviewed answer or legacy
choice remains. Explicit mismatched pins and answered forms stay fenced. The
acceptance test additionally checks restarted terminal replay and stale-pin denial.
The corrected implementation was subsequently qualified at 93539cc, as recorded
below. The failing d4d64b4 acceptance run remains historical failed evidence.

## S8 empty-state review follow-up

The bounded continuation grammar is now explicitly retainable even with zero
remaining filters. Earlier nonempty selections opted into it implicitly; dropping
all selections could otherwise change a later `last quarter` request into a
historical unrecognized phrase. The additive `continuation-v1` policy is carried
by Refine/saved selections/SDK and pinned by pending-origin and replay checks.
Absent policy preserves legacy behavior. Exact EN/ES empty-state and multi-turn
source-result tests passed at the qualified revision below. A policy marker is
parser mode, not source permission, a private scalar value or an analytical proof.


## S8 qualified result — governed values and calendar continuation

Runtime head `93539ccdf6cea974c50597fce568306d2d62f6c7`, tree
`193cf00efa78ad3e15f2d137d61efb61f03babda`, tested merge
`9974ba43f8712a5ae2d8608c7d8d7b2b9933c5ee`. Standard CI
[36213684087](https://github.com/hurtener/chartworks/actions/runs/36213684087) and
expanded recovery [36213684093](https://github.com/hurtener/chartworks/actions/runs/36213684093)
passed. Each Go 1.26.4/1.27.1 artifact independently records **1,672 passing
unit/subtest events across 20 packages**, zero failures, one existing opt-in paid
live-reranker skip, and **198 passing PostgreSQL/native acceptance events**, zero
failures/skips. All 17 required S8 unit/SDK tests and three acceptance tests passed.
The two toolchains repeat the same scenarios; these counts are not additive coverage.

The acceptance sequence covers North/March, South retaining March, last quarter
retaining South, explicit interval replacement, removing both filters, restarting
and replaying the unfiltered child, then selecting a quarter again from that empty
state. Exact source record IDs, source/session fences, immutable parent lineage,
current policy reconstruction and no-extra-provider/read terminal replay are checked.
Saved-query default and explicit anchors are separately tested. Marker/anchor
omission or substitution cannot borrow another pending clarification origin.

The two review corrections were necessary: carrying an empty form pin rejected
removal of the last inferred predicate; dropping parser mode on empty state lost
a later relative-period interpretation. The fixes preserve answered-form pins,
legacy parsing, typed model-parameter custody and source authorization. No database
migration, new model role/public operation, additional inference call or frozen
report interpretation is introduced. Local formatting/diff/planning checks passed;
local native Go tests were blocked before compilation by absent offline modules.
Runtime evidence is from Actions, not a claimed local/live-model run.

S8's finite software slice is closed. S9 grouping edits, S2 generalized language,
all other unchecked S items and Q1/Q2/Q3 remain open. The earlier P0/P1 baseline
review is not a blanket guarantee for every new source revision.

## S9 grouping implementation checkpoint — historical qualification pending

Baseline `cccf55c761d3404da2ccd1d277ff3f67f44ca710`. The
[grouping continuity contract](../contracts/grouping-continuity-v1.md) describes
complete typed grouping sets, inherited verified grains, exact reviewed EN/ES
replacement, deliberate scalar totals, calendar re-selection and private-parameter
composition. Migration 059 distinguishes v5 from retained v0-v4. Pending origins,
SDK/saved state and source/analytical replay retain the grouping identity.

S9 is not marked complete: runtime qualification for this checkpoint is pending,
and broader native parameter-role proofs for required nested/multi-relation shapes
remain unimplemented. Existing S8 scalar/time behavior is preserved; S2 language,
S4 analytical breadth and S5/S6 native/engine obligations are not reduced.


## S9 grouping-continuity qualification work

The branch now implements explicit complete grouping sets, verified inheritance,
reviewed EN/ES replacement and explicit scalar totals, composing with S8 filters,
calendar anchors, private model slots and service-owned predicates. Exact version-5
proofs, SDK/saved-state transport, pending origins and shared-field calendar mapping
have regressions. S9 remains open for broader required native parameter-role shapes;
this does not reduce S2/S4/S5/S6 or the owner qualification backlog.

Baseline `799eeb1` passed standard CI and 1,684 expanded unit/subtest events, but
its acceptance test exposed a real pending-admission gap: an unplanned preflight
was incorrectly required to carry a nonempty executable scope. A separate strictly
unplanned refinement path now resolves current authority before original-form
replay, while planned execution keeps its scope fence. Negative tests cover
executable-evidence substitution, current source drift, partition/session/action
checks and cancelled requests. Expanded results for the corrected head must be
observed before this checkpoint is counted as qualified.


## Qualified S9 grouping slice — pending-refinement boundary closed

Runtime `996db063d214ae086f4fe4383028348abe9d7282`, tree
`56f8d6ad5cdffad092b1df86b85679b6ded12a77`, passed standard
[CI 36240900999](https://github.com/hurtener/chartworks/actions/runs/36240900999)
and [recovery 36240901003](https://github.com/hurtener/chartworks/actions/runs/36240901003).
Each Go 1.26.4/1.27.1 job records 1,708 passing unit/subtest events across 20
packages and 207 passing PostgreSQL/native acceptance events. No failure or
acceptance skip occurred; the existing paid live-rerank smoke is the sole opt-in
unit skip. All 16 grouping/pending unit/SDK tests and four S9 acceptance tests
passed. Toolchains repeat scenarios; counts are not additive coverage.

The previously failing 799eeb1 pending private-answer/group-change case now passes.
A strictly unplanned form resolves current reviewed scope before original-form
replay; it does not borrow the executable-query scope fallback. Planned records
still require nonempty exact scope. Tests reject executable evidence on a pending
form, action/session/context/source drift, cancelled work, unplanned Run and a
changed stateless Plan submission. The source pending form remains immutable.

The broader S9 implementation now has integrated evidence for reviewed EN/ES
replacement and inheritance, exact scalar totals, calendar reselection after a
total, S8 value/time independence, private parameter roles and owned predicates,
managed saved questions, immutable versions and zero-work terminal replay.
No join/nested/window parameter-role capability was added. **S9 remains open for
those required broader roles**, and the remaining S2-S7/S10-S12/Q1-Q3 obligations
are unchanged. Recorded fixtures do not establish live model/owner/engine parity.

Publishing transport/checksum failures remain infrastructure failures, not runtime
passes. All temporary authoring files are excluded from this runtime tree. Local
formatting/diff/planning checks passed; missing modules prevented local native Go
compilation, so all full runtime qualification above is from Actions.
