# SQL recovery — completion tracker

PR #62; implementation baseline `ea38c0774ac9f16bc7a7d3ffeeb4f26b666320c7`.
Latest qualified implementation: `6273c80d677dfb6c396d3747422c5001b93ea2c0`;
qualified tree: `9c4c21575bb9d82f1187d8db746043908c9f2d18`.
Updated 2026-09-25. This is the current AP-00–AP-08 tracker. Historical
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
| AP-01 | Catalog-backed atomic dependencies; nested KPI closure; selected roots shared with rules and clarification; provenance, omissions and replay | S1 qualified at 6273c80; S2: reviewed, bounded paraphrase selection | Held-out selection/ambiguity calibration, not similarity-as-confidence |
| AP-02 | Metric/dimension/multi-topic prompt projection; ranked examples and actual-use evidence; strategy and effective-envelope fitting | S3: minimal confirmed-join-aware projection | Operator tokenizer/model-window/framing qualification |
| AP-03 | Native-first selected metrics, exact arithmetic, independent populations, direct/calendar grain, typed WHERE/HAVING and versioned replay | S4: reviewed intent for order/limit/filter and broader expressions; S5: physically backed join/fan-out proof and required nested/multi-relation forms; S6: required dialect proof | Business-result cohorts and actual required engines; safe rejection is not support |
| AP-04 | Targeted unbound SQL repair, private binding restoration, bounded attempts, terminal exclusions and actual durable source-error path | S7: additional safe diagnostic/correction classes where acceptance requires them | Broader privacy, repair/result and engine matrix within existing attempt limits |
| AP-05 | Protected parent lineage; typed reference/metric edits; model-slot custody; explicit same-kind parameter replacement; public SDK | S8 implemented for resolved governed values/calendar intervals; current checkpoint qualification pending; S9: grouping changes and broader native role proof | Multi-turn EN/ES business journeys, restart/replay and language quality |
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
- [ ] S8 — Inferred scalar/time context inherited across abbreviated follow-ups;
  explicit replacement/removal, exact retained anchor, private bindings and replay.
- [ ] S9 — Grouping edits and broader native parameter-role proofs without silently
  retaining obsolete grain or granting authority from prior SQL.
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
SDK and saved/clarification-origin paths use the same request contract. S8 remains
unchecked until this checkpoint's native and surface qualification succeeds.

## S8 removal correction — current qualification checkpoint

Observed baseline `d4d64b4`: the existing suite passed units, but typed removal of
all inferred predicates failed acceptance. Its unrelated, empty answer context
was being carried as if it authenticated an answer. The correction clears that
input pin only after parent authorization/replay when no reviewed answer or legacy
choice remains. Explicit mismatched pins and answered forms stay fenced. The
acceptance test additionally checks restarted terminal replay and stale-pin denial.
This source checkpoint remains unqualified until the updated suite completes.
