# Behavioral gap analysis and parity expansion

> Review artifact for the current migration baseline. This document records confirmed gaps, narrowed behaviors, intentionally reworked boundaries, explicitly pending work, and unassessed frontiers. It is not a parity score and does not replace phase plans, acceptance tests, release gates, or migration decisions.

## Contents

- [Scope and evidence boundary](#scope-and-evidence-boundary)
- [Assessment](#assessment)
- [Finding index](#finding-index)
- [Detailed findings](#detailed-findings)
- [Unassessed expansion frontiers](#unassessed-expansion-frontiers)
- [Clarification and underspecification UX expansion](#clarification-and-underspecification-ux-expansion)
- [Chart breadth and richness expansion](#chart-breadth-and-richness-expansion)
- [Preserved and deliberately reworked behavior](#preserved-and-deliberately-reworked-behavior)
- [Phased corrective sequence](#phased-corrective-sequence)
- [Comparison method and sentinel corpus](#comparison-method-and-sentinel-corpus)
- [63-feature continuity ledger](#63-feature-continuity-ledger)
- [Review disposition](#review-disposition)

## Scope and evidence boundary

- Current target baseline: `dc6fb297dc6379c0093d1e6305d5a6112f08f1a7`. The audit boundary is this implementation head; stale prose in other status ledgers is not silently corrected here.
- Reference provenance: the supplied manifest fingerprints 638 source/test files; SHA-256 `43deea9256ba79f424c9cfbf15498649123364c365d5db44b5868285c303de9f`. This is provenance, not a coverage or parity claim.
- Phases 26 and 28 are treated as in progress per owner direction. Unmerged work is neither counted as shipped nor judged absent. Remaining planned phases retain their existing obligations.
- The reference is a supplied local source snapshot. This document uses neutral behavior descriptions and abstract source evidence IDs only; exact private source locations remain in the owner-local audit bundle's external `source-evidence-map.md` (SHA-256 `33a2dfc2ed4d333845a1b6e14a008d6878eb85c6c380b483a94470c56638c373`) and are not reproduced in the repository.
- Current repository links below are evidence pointers anchored to this checkout and line number. They show implementation shape, not a passing parity run.
- No runtime, live-model, cloud, migration, or deployment tests were run for this document. Prior test results are historical evidence only and do not establish behavioral equivalence.

## Assessment

The migration preserves substantial governance, source safety, typed execution, bounded parsing, immutable lifecycle and a real chart specification boundary. It also narrows several behaviorally important systems: semantic fields and dependency context, interpretation and clarification, template/feedback selection, reporting output intent, multi-value chart bindings and calibrated reuse. Pending phases explain unfinished consumers, but they do not erase confirmed gaps already visible in merged contracts. A feature name, stored field, registered operation or green phase acceptance is insufficient without tracing the behavior through authoring, persistence, retrieval/selection, context, validation, execution, output, import and replay.

The planning checker covers 63 broad source-feature IDs and maps them to acceptance criteria; it does not prove every variant, field, consumer, calibration or live workload. Treat every status below as a disposition to close with evidence, not as a completion percentage.

## Finding index

| ID | Area | Finding | Status | Owner / phases |
|---|---|---|---|---|
| SEM-01 | Semantics | Rich semantic fields are not represented end to end | confirmed gap | 15/17 with 33 generation and 34 import |
| CTX-01 | Context | Selected metrics do not retain their full dependency context | confirmed gap | 17/18 |
| SEM-02 | Semantics | Enhancement is column classification rather than rich semantic authoring | confirmed gap | 15/33 |
| RTE-01 | Routing | Routing confidence and topic choice are reduced | confirmed gap | 17/24 |
| LRN-01 | Learning | Stored examples are not equivalent to retrieval-selected templates | confirmed gap | 15/17/18; optimization remains 24 |
| LRN-02 | Learning | Feedback weights are fixed increments | confirmed gap | 18; evaluation 24 |
| BLK-01 | Reporting | Output enablement and localized output metadata are missing from definitions | confirmed gap | 27 before 28/29 consumers |
| BLK-02 | Reporting | Business-rule snapshots are absent from block dependencies | confirmed gap | 27 with 16/18; preserve in 28 |
| BLK-03 | Reporting | Period wording no longer participates in certification | confirmed gap | 27 |
| BLK-04 | Reporting | Question overlap assessment is lexical only | confirmed gap | 27; gateway05 |
| BLK-05 | Reporting | Sensitive-column metadata is missing at the narrative handoff | definition gap; runtime pending | 27/28/33/34 |
| BLK-06 | Reporting | Parameterization assistance supports a narrower workflow | narrowed; equivalent mapping needed | 27/34 |
| BLK-07 | Reporting | Per-block limits and richer narrative policies need explicit mappings | pending contract risk | 27/28/34 |
| CLR-01 | Clarification | Required clarification slots lack question-specific activation | confirmed gap | 16/17 |
| RUL-01 | Rules | Compound and template scopes have no equivalent current representation | confirmed gap | 16/17/18 |
| RTE-02 | Interpretation | Value, geography and temporal normalization is not an equivalent runtime stage | confirmed gap | 17/18; source metadata 15/33 |
| MIG-01 | Portability | Topic-only portability does not carry the calibrated topic environment | narrowed; full migration pending | 15 subset; 34 full migration |
| VIS-01 | Outputs | Rich KPI and table authoring options are absent | confirmed gap | 20/27 definitions; 28 and 31/32 execution/display |
| VIS-02 | Charts | Multi-measure chart slots are reduced to singular bindings | confirmed gap | 20/27 before rendering |
| VIS-03 | Formatting | Stored formatting and display labels lose authored intent | confirmed gap | 20/27; consume31/32; map34 |
| VIS-04 | Selection diagnostics | Selection rationale carries less structured evidence | narrowed; equivalence decision needed | 20/24 |
| EVAL-01 | Evaluation | Deterministic acceptance is not calibrated behavioral equivalence | explicitly pending | 24/34/25 |
| CLR-02 | Clarification | Typed clarification answers can have no planning effect | confirmed gap | 16/17/18 |
| DATA-01 | Profiling to semantics | Safe profiles no longer supply governed example values to semantic authoring | intentional redesign; replacement needed | 12/15/33; consumer17 |
| DATA-02 | Discovery and onboarding | Physical discovery is not equivalent to semantic role and relationship discovery | pending richer authoring | 15/33; coordinate26 |
| PERF-01 | Latency and reuse | Cache behavior must be compared under the new authority model | intentional redesign; performance unmeasured | 17/24; reuse28; qualification34/25 |

## Detailed findings

### SEM-01 — Rich semantic fields are not represented end to end

- **Disposition:** confirmed gap.
- **Owner / phases:** 15/17 with 33 generation and 34 import.
- **Reference behavior (neutral):** Original metric and dimension definitions carry business/SQL meaning, aliases, samples, filters, and temporal metadata; the context packer consumes several of these fields.
- **Current boundary:** The Go model retains basic measures/dimensions/KPIs and stable references, but has no typed dimension value aliases, sample values, supported temporal grains or equivalent complete richer definition.
- **Consequence:** A value spelling, stored code or time grain can become an inference problem again after migration. KPIs do retain expressions and exact inputs in storage; the separate CTX-01 gap concerns carrying that calculation and its dependencies into generation.
- **Contract, storage and import impact:** Extend the versioned semantic topic, facet, publication, portable-pack and generation-context contracts together. Persist the richer fields with explicit null/unsupported policy, update import/export and downstream serializers, and preserve source/context reach pins.
- **Closure requirements:** Round-trip a synthetic calculated metric, alias, month-only temporal field, synonym and required filter; assert each fact appears in the generation context and changes the resulting plan or produces typed insufficiency.
- **Source evidence IDs:** REF-SEM-01-A, REF-SEM-01-B.
- **Current repository evidence:** [internal/semantics/model.go:165](../internal/semantics/model.go#L165).

### CTX-01 — Selected metrics do not retain their full dependency context

- **Disposition:** confirmed gap.
- **Owner / phases:** 17/18.
- **Reference behavior (neutral):** The original packer enriches selected KPIs with constituent measures and constructs table, dimension and join payloads.
- **Current boundary:** The Go packer independently drops retrieved evidence by budget. Its mandatory metric pin is a name plus aggregation, or just a KPI name; there is no structural dependency group on evidence.
- **Consequence:** A pinned KPI label can survive while its formula or columns are absent from the model input. This is a prompt-completeness gap, not proof of an observed wrong answer.
- **Contract, storage and import impact:** Add a dependency-closure representation to the sealed context and its budget/drop audit. The closure must survive persistence, refinement, portable capture and model-gateway serialization; incomplete closure must be an explicit typed outcome.
- **Closure requirements:** Under budget pressure, a pinned derived metric must retain its formula, constituent measures and required columns/joins as one closure group, or fail explicitly before model work.
- **Source evidence IDs:** REF-CTX-01-A, REF-CTX-01-B.
- **Current repository evidence:** [internal/nlq/context.go:377](../internal/nlq/context.go#L377); [internal/nlqroute/service.go:1038](../internal/nlqroute/service.go#L1038); [internal/nlqexec/service.go:858](../internal/nlqexec/service.go#L858).

### SEM-02 — Enhancement is column classification rather than rich semantic authoring

- **Disposition:** confirmed gap.
- **Owner / phases:** 15/33.
- **Reference behavior (neutral):** The original has dedicated semantic enhancement and rich business-context entities.
- **Current boundary:** The current enhancement input is column name/category/nullability; its closed output produces a measure, dimension or unresolved item, without descriptions, KPI expressions, joins or value aliases.
- **Consequence:** An imported rich topic and a freshly generated topic cannot have equivalent richness using this generator alone.
- **Contract, storage and import impact:** Define the semantic proposal schema and migration for grain, units, formulas, relationships, aliases, filters and unresolved evidence. Ensure draft/edit/publication and SDK representations consume the same contract rather than creating a second model.
- **Closure requirements:** Generate and store evidence-backed grain, joins, measures, dimensions, KPIs, units and time/null semantics; verify downstream context uses them and unresolved items remain reviewable.
- **Current repository evidence:** [internal/semantics/drafts/service.go:511](../internal/semantics/drafts/service.go#L511); [internal/semantics/drafts/service.go:554](../internal/semantics/drafts/service.go#L554); [internal/semantics/enhance.go:20](../internal/semantics/enhance.go#L20); [docs/plans/phase-33-guided-onboarding.md:37](../docs/plans/phase-33-guided-onboarding.md#L37).

### RTE-01 — Routing confidence and topic choice are reduced

- **Disposition:** confirmed gap.
- **Owner / phases:** 17/24.
- **Reference behavior (neutral):** The original routes using span/evidence aggregation and explicit topic decisions.
- **Current boundary:** The Go route requires caller-selected topics, derives confidence from the closest facet distance, and considers the top two global facets ambiguous within a fixed distance margin. Reranking does not recalibrate this confidence.
- **Consequence:** Two complementary facets from one topic may cause clarification; reported confidence is not demonstrated calibrated query correctness.
- **Contract, storage and import impact:** Define routing evidence, topic choice, confidence and clarification outcomes as versioned route metadata. Preserve same-topic complementary evidence and make calibration/replay artifacts inspectable without weakening authority.
- **Closure requirements:** Compare competing topics and same-topic complementary facets before/after reranking; assert route, clarification, confidence and context tier with calibration evidence.
- **Source evidence IDs:** REF-RTE-01-A.
- **Current repository evidence:** [internal/nlqroute/service.go:457](../internal/nlqroute/service.go#L457); [internal/nlqroute/service.go:388](../internal/nlqroute/service.go#L388); [internal/nlqroute/service.go:914](../internal/nlqroute/service.go#L914).

### LRN-01 — Stored examples are not equivalent to retrieval-selected templates

- **Disposition:** confirmed gap.
- **Owner / phases:** 15/17/18; optimization remains 24.
- **Reference behavior (neutral):** The original has template selection, similarity/weight/diversity controls and active/fallback prompt packs.
- **Current boundary:** Generation reads topic examples and turns active entries into question/SQL instructions. The topic definition and facet publisher have no equivalent query-pattern/template lifecycle.
- **Consequence:** The tested precedence function preserves the order of supplied instruction lanes but does not preserve how the system discovers and selects the appropriate SQL pattern.
- **Contract, storage and import impact:** Add a versioned template/pattern/example selection contract, including active/fallback state, relevance, diversity and provenance. Import/export must report unsupported records and generation must persist the selected rationale.
- **Closure requirements:** Replay structurally different verified examples and a follow-up; assert exact/adapt/few-shot choice, version validity, deduplication, diversity and provenance.
- **Source evidence IDs:** REF-LRN-01-A, REF-LRN-01-B.
- **Current repository evidence:** [internal/nlqexec/service.go:425](../internal/nlqexec/service.go#L425); [internal/nlqexec/service.go:946](../internal/nlqexec/service.go#L946); [internal/semantics/topics/facets.go:30](../internal/semantics/topics/facets.go#L30).

### LRN-02 — Feedback weights are fixed increments

- **Disposition:** confirmed gap.
- **Owner / phases:** 18; evaluation 24.
- **Reference behavior (neutral):** The original contains evidence-aware template weighting; the current RFC explicitly retains Wilson/recency/evidence weighting.
- **Current boundary:** Positive/corrected feedback creates an example at 0.5; duplicates add 0.05, capped at 1. Selection orders stored weight/evidence/update time. Negative feedback has no equivalent weight reduction in this path.
- **Consequence:** Repeated evidence can promote confidence without preserving the prior calibration or response to negative outcomes.
- **Contract, storage and import impact:** Define feedback outcome, evidence, aging and negative-signal storage before changing ranking. Keep bounded labels, tenant scope and reproducible replay; update the selection and evaluation contracts together.
- **Closure requirements:** Replay identical positive, negative, aged and contradictory feedback; compare eligibility, bounded weight and selection, including a documented policy for negative evidence.
- **Source evidence IDs:** REF-LRN-02-A.
- **Current repository evidence:** [internal/nlqexec/service.go:266](../internal/nlqexec/service.go#L266); [internal/store/postgres/nlq_runtime.go:354](../internal/store/postgres/nlq_runtime.go#L354); [internal/store/postgres/nlq_runtime.go:384](../internal/store/postgres/nlq_runtime.go#L384); [current authority/design RFC:111](../RFC-001-Chartworks.md#L111).

### BLK-01 — Output enablement and localized output metadata are missing from definitions

- **Disposition:** confirmed gap.
- **Owner / phases:** 27 before 28/29 consumers.
- **Reference behavior (neutral):** Original output definitions include enabled state, localized name/description and display order. The execution/output builders filter disabled outputs.
- **Current boundary:** Go Output contains ID, kind and mapping/narrative only. Empty selection means all stored outputs. Array order is preserved, but there is no disabled-output state or equivalent localized output label contract.
- **Consequence:** An imported disabled output cannot retain its behavior without removal or a schema extension; stable ID retention matters to later widgets and schedules.
- **Contract, storage and import impact:** Extend the block output definition with enabled state, localized labels and stable display order. Define import behavior and selected-disabled policy before frozen-run consumers read the definition.
- **Closure requirements:** Import enabled/disabled localized outputs; default execution excludes disabled outputs, IDs/order/labels survive, and selected-disabled behavior follows one reviewed policy.
- **Source evidence IDs:** REF-BLK-01-A, REF-BLK-01-B.
- **Current repository evidence:** [internal/reporting/model.go:162](../internal/reporting/model.go#L162); [internal/reporting/definition.go:227](../internal/reporting/definition.go#L227); [docs/contracts/reporting-blocks-v1.md:104](../docs/contracts/reporting-blocks-v1.md#L104).

### BLK-02 — Business-rule snapshots are absent from block dependencies

- **Disposition:** confirmed gap.
- **Owner / phases:** 27 with 16/18; preserve in 28.
- **Reference behavior (neutral):** Original block governance snapshots applicable rule IDs/hashes and compares current rules against the saved manifest.
- **Current boundary:** Current block dependencies pin source catalogs and topic packs, but have no ruleset reader or rule snapshot. Query capture does not transfer query rule-version pins into the block definition.
- **Consequence:** A rule-only change cannot be represented by the same explicit dependency comparison. This finding concerns business meaning and stale-approval detection, not an authority-provider bypass.
- **Contract, storage and import impact:** Add rule-set identifiers/digests and applicability evidence to block dependencies and capture manifests. Revalidation must compare rule-only changes without altering source authorization or published immutability.
- **Closure requirements:** Certify a rule-governed block, change only the rule, and require a stale/review-required outcome before reuse or certification.
- **Source evidence IDs:** REF-BLK-02-A, REF-BLK-02-B.
- **Current repository evidence:** [internal/reporting/dependencies.go:52](../internal/reporting/dependencies.go#L52); [internal/reporting/model.go:169](../internal/reporting/model.go#L169); [internal/nlqexec/capture.go:17](../internal/nlqexec/capture.go#L17); [internal/reporting/query_capture.go:32](../internal/reporting/query_capture.go#L32).

### BLK-03 — Period wording no longer participates in certification

- **Disposition:** confirmed gap.
- **Owner / phases:** 27.
- **Reference behavior (neutral):** The original compares each localized canonical question against period policy and requires acknowledgement of contradictory wording during certification.
- **Current boundary:** Go resolves calendar periods carefully, but certification does not check or acknowledge question-period inconsistency.
- **Consequence:** A block labelled as one period can be certified while its defaults resolve another period, losing a useful business review safeguard.
- **Contract, storage and import impact:** Add a certification-time period-language check and reviewed disposition to the block revision. Preserve locale, canonical period and default-resolution provenance in storage and import.
- **Closure requirements:** Attempt certification with conflicting period wording/defaults in English and Spanish; require visible discrepancy and reviewed resolution.
- **Source evidence IDs:** REF-BLK-03-A, REF-BLK-03-B, REF-BLK-03-C.
- **Current repository evidence:** [internal/reporting/lifecycle.go:39](../internal/reporting/lifecycle.go#L39).

### BLK-04 — Question overlap assessment is lexical only

- **Disposition:** confirmed gap.
- **Owner / phases:** 27; gateway05.
- **Reference behavior (neutral):** The original shortlists questions and performs bounded semantic assessment of metric, grain, population, filters and period, with an explicit deterministic fallback.
- **Current boundary:** Go compares normalized word sets above a threshold, within one bounded list page and matching locale. It correctly reports incompleteness but does not supply semantic intent assessment.
- **Consequence:** Paraphrases may be missed and similarly worded questions with materially different periods may be conflated.
- **Contract, storage and import impact:** Define bounded overlap analysis inputs/results and deterministic fallback. Persist candidate scope and decision evidence; keep lexical comparison as a safe fallback rather than silently presenting it as semantic equivalence.
- **Closure requirements:** Compare paraphrases, populations and periods; distinguish duplicate, overlap and unique using bounded authorized candidates and deterministic fallback.
- **Source evidence IDs:** REF-BLK-04-A, REF-BLK-04-B.
- **Current repository evidence:** [internal/reporting/questions.go:30](../internal/reporting/questions.go#L30); [internal/reporting/service.go:358](../internal/reporting/service.go#L358).

### BLK-05 — Sensitive-column metadata is missing at the narrative handoff

- **Disposition:** definition gap; runtime pending.
- **Owner / phases:** 27/28/33/34.
- **Reference behavior (neutral):** The original expected schema flags sensitive columns, and narrative evidence construction consumes those flags in addition to explicit redactions.
- **Current boundary:** Go expected schema uses exec.Field without that sensitivity field. Narrative definitions allow explicit RedactedFields, but no equivalent inherited sensitivity contract is present.
- **Consequence:** Before phase28, define how sensitivity survives import and capture; explicit manual redactions alone are not the same inherited behavior. No live narrative disclosure is asserted because runtime is pending.
- **Contract, storage and import impact:** Carry sensitivity classification from expected schema/result capture into bounded narrative evidence. Add migration rules for inherited sensitivity and ensure ordinary logs/provider inputs exclude protected values.
- **Closure requirements:** Capture a sensitive result and narrative selection; prove only the permitted projection enters the narrative path and model input.
- **Source evidence IDs:** REF-BLK-05-A, REF-BLK-05-B, REF-BLK-05-C.
- **Current repository evidence:** [internal/reporting/model.go:140](../internal/reporting/model.go#L140); [internal/reporting/model.go:179](../internal/reporting/model.go#L179).

### BLK-06 — Parameterization assistance supports a narrower workflow

- **Disposition:** narrowed; equivalent mapping needed.
- **Owner / phases:** 27/34.
- **Reference behavior (neutral):** Original assistance proposes dialect-aware period edits and carries question/template/paraphrase dispositions with protected original authoring provenance.
- **Current boundary:** Go safely replaces an explicitly selected PostgreSQL half-open predicate and creates a new draft. It has no equivalent question/template disposition workflow.
- **Consequence:** The AST-constrained edit is valuable, but calling it full assisted-parameterization parity omits authoring intent preservation and non-PostgreSQL variants.
- **Contract, storage and import impact:** Define a dialect-aware parameterization result with original-question/template provenance, transformed predicate and unsupported disposition. Preserve all unrelated predicates and create a new immutable draft revision.
- **Closure requirements:** Run the reviewed period edit across supported dialects; preserve unrelated predicates, intent and provenance or emit an explicit unsupported result.
- **Source evidence IDs:** REF-BLK-06-A, REF-BLK-06-B.
- **Current repository evidence:** [internal/reporting/assistance.go:23](../internal/reporting/assistance.go#L23).

### BLK-07 — Per-block limits and richer narrative policies need explicit mappings

- **Disposition:** pending contract risk.
- **Owner / phases:** 27/28/34.
- **Reference behavior (neutral):** Original definitions carry query row/time caps and narrative claim ceilings, analysis types, tones and caveat policies.
- **Current boundary:** Current block definitions have no per-block query limit fields; narratives have bounded rows/bytes/calls/tokens but a different policy vocabulary and no max-claims field.
- **Consequence:** Phase28 must decide and test the mapping before freezing run manifests. Stricter defaults may be intentional, but silently dropping saved behavior is not a migration rule.
- **Contract, storage and import impact:** Map per-block query limits and narrative policy fields into the frozen-run manifest. Every dropped or tightened field requires an explicit disposition, persisted audit evidence and an acceptance negative case.
- **Closure requirements:** Dry-run nondefault query/narrative policies; enumerate transformations and prove equivalence or reviewed rejection before freezing a run manifest.
- **Source evidence IDs:** REF-BLK-07-A, REF-BLK-07-B.
- **Current repository evidence:** [internal/reporting/model.go:140](../internal/reporting/model.go#L140); [internal/reporting/model.go:169](../internal/reporting/model.go#L169); [docs/plans/phase-28-reporting-execution-artifacts.md:1](../docs/plans/phase-28-reporting-execution-artifacts.md#L1).

### CLR-01 — Required clarification slots lack question-specific activation

- **Disposition:** confirmed gap.
- **Owner / phases:** 16/17.
- **Reference behavior (neutral):** Original underspecification evaluates keyword/entity/regex and enriched triggers, and skips patterns whose trigger strength is zero.
- **Current boundary:** Go ClarificationPattern defines slots but no matcher. Routing loops every published pattern and returns its first missing required slot, without matching the question or pattern targets.
- **Consequence:** A clarification intended for one ambiguous metric can interrupt unrelated questions in the same topic.
- **Contract, storage and import impact:** Add conditional clarification activation to the published policy contract: target/effect, trigger, priority, lifecycle and version. The route must evaluate only applicable patterns and expose deterministic ordering.
- **Closure requirements:** A metric-specific required slot fires only for its reviewed matching condition; an unrelated complete question reaches retrieval without that blocker.
- **Source evidence IDs:** REF-CLR-01-A.
- **Current repository evidence:** [internal/semantics/rules.go:139](../internal/semantics/rules.go#L139); [internal/nlqroute/service.go:598](../internal/nlqroute/service.go#L598); [internal/nlqroute/service_test.go:224](../internal/nlqroute/service_test.go#L224).

### RUL-01 — Compound and template scopes have no equivalent current representation

- **Disposition:** confirmed gap.
- **Owner / phases:** 16/17/18.
- **Reference behavior (neutral):** Original rules distinguish topic, compound-AND and template scopes.
- **Current boundary:** Go supports topic and any-selected-entity scopes; entity applicability is OR, and hard constraints are limited to reference presence/absence.
- **Consequence:** An original rule attached to the combination of two entities cannot be mechanically migrated to the current entity scope without changing when it applies. Keeping arbitrary source SQL/comment mechanisms is not required; equivalent safe semantics are.
- **Contract, storage and import impact:** Extend rule applicability to compound and template scopes or record a typed, reviewed transformation. Persist scope truth-table evidence and keep safe constraint compilation separate from raw source SQL.
- **Closure requirements:** Evaluate neither entity, each entity, both entities and a template-scoped case; assert the reviewed truth table using typed safe constraints.
- **Source evidence IDs:** REF-RUL-01-A.
- **Current repository evidence:** [internal/semantics/rules.go:24](../internal/semantics/rules.go#L24); [internal/semantics/rules.go:58](../internal/semantics/rules.go#L58); [internal/semantics/rules_evaluate.go:90](../internal/semantics/rules_evaluate.go#L90).

### RTE-02 — Value, geography and temporal normalization is not an equivalent runtime stage

- **Disposition:** confirmed gap.
- **Owner / phases:** 17/18; source metadata 15/33.
- **Reference behavior (neutral):** Original context packing infers governed value filters, normalizes geography aliases and injects temporal dimensions based on extracted spans.
- **Current boundary:** Go embeds the raw question once and receives explicit references/choices; it has no equivalent deterministic span-to-filter stage. Embeddings and the generator may still infer some intent, which is different from preserving the mechanism.
- **Consequence:** The model must rediscover calibrated value/time decisions, and those decisions cannot be inspected or replayed at the same stage.
- **Contract, storage and import impact:** Introduce a sealed interpretation result for governed values, geography and temporal spans. Persist canonical values, locale/parser versions and source/topic pins, then feed only typed constraints into generation and validation.
- **Closure requirements:** Run aliases, negated geography, ambiguous places and month-only dates; compare normalized filters and selected dimensions before SQL generation.
- **Source evidence IDs:** REF-RTE-02-A, REF-RTE-02-B, REF-RTE-02-C.
- **Current repository evidence:** [internal/nlqroute/service.go:345](../internal/nlqroute/service.go#L345); [internal/nlqroute/service.go:431](../internal/nlqroute/service.go#L431).

### MIG-01 — Topic-only portability does not carry the calibrated topic environment

- **Disposition:** narrowed; full migration pending.
- **Owner / phases:** 15 subset; 34 full migration.
- **Reference behavior (neutral):** Original topic export includes template closure, semantic samples, applicable rules, reporting blocks and dependent records.
- **Current boundary:** Go PortablePack intentionally transfers a safe semantic subset through explicit destination mappings. Broader migration is assigned to phase34.
- **Consequence:** A successful topic import cannot be treated as a migrated calibrated system. Keep the safe mapping, but explicitly transfer the surrounding authored/learned behavior with lifecycle revalidation.
- **Contract, storage and import impact:** Define a bundle manifest covering semantic, rules, examples/templates and dependent outputs. Import must enumerate carried/transformed/rejected fields, revalidate current authority and never treat a topic-only import as calibrated-system migration.
- **Closure requirements:** Export/import templates, rules, examples and dependent mappings; report every carried/transformed/rejected record and verify behavior after reauthorization.
- **Source evidence IDs:** REF-MIG-01-A, REF-MIG-01-B.
- **Current repository evidence:** [internal/semantics/portable.go:28](../internal/semantics/portable.go#L28); [docs/plans/phase-34-migration-parity-cutover.md:17](../docs/plans/phase-34-migration-parity-cutover.md#L17).

### VIS-01 — Rich KPI and table authoring options are absent

- **Disposition:** confirmed gap.
- **Owner / phases:** 20/27 definitions; 28 and 31/32 execution/display.
- **Reference behavior (neutral):** Original KPI output computes comparison/delta/percent delta, target difference, threshold state and sparkline; table definitions carry visibility, labels, sort, page size and show-totals intent.
- **Current boundary:** Go saved KPI mapping exposes one value binding without comparison/trend/target/threshold slots. Generic tables support ordered selected columns, sorting and exact additive totals, but not all of the original saved display controls.
- **Consequence:** Four output kind names do not prove parity within each kind. Totals are not wholly missing; the missing part includes saved display intent and rich KPI semantics.
- **Contract, storage and import impact:** Extend output definitions and retained result manifests for KPI comparison/target/threshold/trend behavior and table display intent. Update import, chart mapping, viewer, static rendering and export contracts before consuming artifacts.
- **Closure requirements:** Port KPI comparison/target/threshold/trend and table visibility/labels/page-size/totals intent; compare values and display policy separately from generic chart totals.
- **Source evidence IDs:** REF-VIS-01-A, REF-VIS-01-B.
- **Current repository evidence:** [internal/charts/model.go:135](../internal/charts/model.go#L135); [internal/charts/model.go:230](../internal/charts/model.go#L230); [internal/reporting/model.go:162](../internal/reporting/model.go#L162).

### VIS-02 — Multi-measure chart slots are reduced to singular bindings

- **Disposition:** confirmed gap.
- **Owner / phases:** 20/27 before rendering.
- **Reference behavior (neutral):** Original line generation consumes all bound y-axis columns and emits one series per measure when no series column is selected.
- **Current boundary:** Current chart bindings have singular value/X/Y fields; a list binding is provided for tables, not multiple measure axes.
- **Consequence:** A saved two-measure time series cannot be represented directly with the same query shape. A tested long-form transformation could be an equivalent alternative, but must preserve types/units/order without rerunning SQL.
- **Contract, storage and import impact:** Add a typed repeated-measure/series binding or a documented, tested long-form transform. Preserve units, labels, ordering, null gaps and provenance without rerunning the source query.
- **Closure requirements:** Build a two-measure time series with distinct units and missing values; require two labelled series or an explicit typed long-form equivalent.
- **Source evidence IDs:** REF-VIS-02-A, REF-VIS-02-B.
- **Current repository evidence:** [internal/charts/model.go:135](../internal/charts/model.go#L135).

### VIS-03 — Stored formatting and display labels lose authored intent

- **Disposition:** confirmed gap.
- **Owner / phases:** 20/27; consume31/32; map34.
- **Reference behavior (neutral):** Original column format hints include locale, date format and currency-symbol fallback, and column metadata has a human-facing display label. Generators transmit formatting side-channel metadata.
- **Current boundary:** Go retains unit/currency/percent/fraction digits and provenance, but lacks those per-column locale/date/display-label fields.
- **Consequence:** A later renderer can choose defaults, but cannot reconstruct the original author choice from the current persisted spec.
- **Contract, storage and import impact:** Add per-column display label, locale, date pattern and currency fallback fields where authored intent must survive. Carry them through chart specs, report outputs, viewer, static renderer and exports.
- **Closure requirements:** Round-trip a renamed result column with locale/date/currency formatting; verify interactive, static and export consumers agree.
- **Source evidence IDs:** REF-VIS-03-A, REF-VIS-03-B, REF-VIS-03-C.
- **Current repository evidence:** [internal/charts/model.go:79](../internal/charts/model.go#L79); [internal/charts/model.go:99](../internal/charts/model.go#L99).

### VIS-04 — Selection rationale carries less structured evidence

- **Disposition:** narrowed; equivalence decision needed.
- **Owner / phases:** 20/24.
- **Reference behavior (neutral):** Original suitability exposes base score and ordered contributing events, with richer column query-role/cardinality/semantic metadata.
- **Current boundary:** Current candidates keep score/reason and a validated mapping, with a simpler metadata vocabulary.
- **Consequence:** The selector is real and deterministic, but explaining or recalibrating a choice has less portable evidence. Not every original metadata field needs duplication if its decision can be reproduced otherwise.
- **Contract, storage and import impact:** Define portable selection evidence: candidate features, reasons, alternatives and calibration version. Preserve the closed mapping and safe fallback while allowing an equivalence decision for omitted metadata.
- **Closure requirements:** For a contested chart choice, capture contributing features/reasons and verify calibration changes do not silently alter unrelated choices.
- **Source evidence IDs:** REF-VIS-04-A, REF-VIS-04-B.
- **Current repository evidence:** [internal/charts/model.go:192](../internal/charts/model.go#L192); [internal/charts/select.go:1](../internal/charts/select.go#L1).

### EVAL-01 — Deterministic acceptance is not calibrated behavioral equivalence

- **Disposition:** explicitly pending.
- **Owner / phases:** 24/34/25.
- **Reference behavior (neutral):** Original code exposes prompt packs, templates, evaluation and optimization workflows; deployed calibrated state may live outside the code snapshot.
- **Current boundary:** Phase24 evaluation and phase34 differential cutover are pending. Existing recorded provider fixtures establish controlled behavior, not whether the same calibrated workload yields the same answer.
- **Consequence:** Actual calibrated prompt versions, published topic content, examples, rules, thresholds, models, source schemas and data snapshots must be included in the comparison boundary.
- **Contract, storage and import impact:** Create a private comparison manifest for calibrated semantic content, rules, examples, model/configuration, source schema/data and budgets. Store normalized context/SQL/result/output comparisons without copying confidential material into the repository.
- **Closure requirements:** Run both systems against the same pinned semantic/rule/template/model/source/data state; compare normalized context, SQL meaning, results and outputs, reporting latency/cost separately.
- **Source evidence IDs:** REF-EVAL-01-A.
- **Current repository evidence:** [docs/plans/phase-24-eval.md:17](../docs/plans/phase-24-eval.md#L17); [docs/plans/phase-34-migration-parity-cutover.md:38](../docs/plans/phase-34-migration-parity-cutover.md#L38); [docs/reviews/phase-15-18-current-evidence.md:68](../docs/reviews/phase-15-18-current-evidence.md#L68).

### CLR-02 — Typed clarification answers can have no planning effect

- **Disposition:** confirmed gap.
- **Owner / phases:** 16/17/18.
- **Reference behavior (neutral):** The reference workflow collects and sanitizes clarification responses, appends delta context, and uses that context in a model-driven rewrite/replan. This inspection did not establish universal typed parameter conversion.
- **Current boundary:** Go accepts nonempty date/number/text/boolean slot answers, but only choice slots receive kind-specific handling. Non-choice values may remain in the route/request record but are not included in the assembled model context.
- **Consequence:** A supplied date can satisfy the missing-slot check yet never constrain generation, unless the caller also rewrites it into the question. This is a concrete data-flow defect, not just less metadata.
- **Contract, storage and import impact:** Make typed clarification resolutions first-class route/context inputs and persist their policy, slot, parser, locale, session and replacement pins. Invalid values must fail before provider work and valid values must reach validation parameters.
- **Closure requirements:** Define a typed resolution endpoint/record for non-reference date/number/boolean/text answers, reserving semantic-reference IDs for reference choices; valid values become sealed constraints, invalid values fail before provider work, and refinements replace rather than retain stale values.
- **Source evidence IDs:** REF-CLR-02-A.
- **Current repository evidence:** [internal/nlqroute/service.go:603](../internal/nlqroute/service.go#L603); [internal/nlqroute/service.go:625](../internal/nlqroute/service.go#L625); [internal/nlqroute/service.go:629](../internal/nlqroute/service.go#L629); [internal/nlqroute/service.go:421](../internal/nlqroute/service.go#L421); [internal/nlq/context.go:243](../internal/nlq/context.go#L243); [internal/nlq/context.go:576](../internal/nlq/context.go#L576).

### DATA-01 — Safe profiles no longer supply governed example values to semantic authoring

- **Disposition:** intentional redesign; replacement needed.
- **Owner / phases:** 12/15/33; consumer17.
- **Reference behavior (neutral):** The original dataset topic builder carries sample values and value information into enhanced dimensions.
- **Current boundary:** Go profiles deliberately avoid raw rows and top-value lists. They retain bounded aggregate/family evidence and selectively permitted numeric/temporal ranges. Current semantic enhancement cannot consume a reviewed value vocabulary.
- **Consequence:** The privacy improvement is legitimate, but the original value-aware query behavior needs a bounded authorized replacement, such as explicitly reviewed semantic value mappings. Re-enabling unrestricted samples is not the recommendation.
- **Contract, storage and import impact:** Define an authorized reviewed-value artifact derived from safe profile evidence. It must carry sensitivity/policy provenance into semantic drafts and context while excluding unrestricted raw samples and supporting erasure.
- **Closure requirements:** Use a permitted synthetic low-cardinality column to produce reviewed value mappings; verify prompt/filter use and prove sensitive values stay excluded.
- **Source evidence IDs:** REF-DATA-01-A.
- **Current repository evidence:** [internal/engineering/profile_types.go:94](../internal/engineering/profile_types.go#L94); [internal/engineering/profile_stats.go:103](../internal/engineering/profile_stats.go#L103); [docs/plans/phase-12-engineering-profiling.md:35](../docs/plans/phase-12-engineering-profiling.md#L35); [internal/semantics/drafts/service.go:554](../internal/semantics/drafts/service.go#L554).

### DATA-02 — Physical discovery is not equivalent to semantic role and relationship discovery

- **Disposition:** pending richer authoring.
- **Owner / phases:** 15/33; coordinate26.
- **Reference behavior (neutral):** The original discovery model and workflow distinguish column roles, relationship evidence and rejected/reviewed joins before rich topic generation.
- **Current boundary:** Go source discovery returns safe physical relation/type/context data; the existing enhancement is column classification. Richer joins/grain/evidence belong to the planned onboarding workflow.
- **Consequence:** A source being discoverable and a profile being complete must not be interpreted as a complete semantic environment. Preserve the extra inference/review steps with real consumers.
- **Contract, storage and import impact:** Add reviewed semantic role, grain, relationship and rejected-join evidence to onboarding outputs. Persist it through draft/publication/import and require an explicit decision before query retrieval uses it.
- **Closure requirements:** Inspect an ambiguous fact/dimension key; preserve role, grain, candidate/rejected join evidence and require review before publication.
- **Source evidence IDs:** REF-DATA-02-A.
- **Current repository evidence:** [internal/sources/sources.go:84](../internal/sources/sources.go#L84); [internal/semantics/enhance.go:20](../internal/semantics/enhance.go#L20); [docs/plans/phase-33-guided-onboarding.md:37](../docs/plans/phase-33-guided-onboarding.md#L37).

### PERF-01 — Cache behavior must be compared under the new authority model

- **Disposition:** intentional redesign; performance unmeasured.
- **Owner / phases:** 17/24; reuse28; qualification34/25.
- **Reference behavior (neutral):** The original has routing/retrieval/context/validation cache layers with lifecycle keys and operational controls.
- **Current boundary:** Current routing/vector evidence explicitly has no equivalent evidence cache, while the gateway has separate cache/usage behavior. Execution has durable idempotency and reuse boundaries of its own.
- **Consequence:** Neither historical latency nor old cache savings transfer automatically. This is not a claim that caching is absent everywhere or that the target is slower. Preserve correct signed reach and establish warm/cold behavior experimentally.
- **Contract, storage and import impact:** Define identity-aware cache keys, retention and invalidation for route/context/validation/reuse. Measure cold/warm behavior and source/model/service attribution without allowing stale or broader signed reach.
- **Closure requirements:** Compare cold/warm/repeated/concurrent requests and rule/source/context changes; attribute calls and latency separately and prove no stale or broader-context reuse.
- **Source evidence IDs:** REF-PERF-01-A, REF-PERF-01-B.
- **Current repository evidence:** [internal/nlqroute/service.go:192](../internal/nlqroute/service.go#L192); [docs/contracts/vector-sources-validation.md:111](../docs/contracts/vector-sources-validation.md#L111); [docs/plans/phase-24-eval.md:29](../docs/plans/phase-24-eval.md#L29).

## Unassessed expansion frontiers

These 12 items are explicit rebaseline work packages, not newly proven defects. They prevent unexamined behavior from being assumed equivalent. Each needs an owner, concrete input/output evidence, an intentional disposition and a negative case.

### EXP-01 — Conversational continuity; 17/18/24

- **Status:** unassessed/pending frontier; not a new confirmed gap.
- **Boundary:** Reference keeps context cards, context merge reports, follow-up intent and clarification events. Current refinement retains request choices/references, injects protected prior SQL, and revalidates current authority (`internal/nlqexec/service.go:84`, `:523`). Richer history equivalence is not established.
- **Contract, storage and import impact:** Session/context records, follow-up resolution, route seals and privacy-safe replay metadata must agree across request storage, generation, execution and reauthorization.
- **Required comparison and closure evidence:** Ask, add dimension, replace filter, remove filter, change metric, correct a clarification, return after semantic publication. Inspect selected context and results, not SQL text alone. Explicit removal must not silently retain stale constraints. Define history limits and when to ask again. Cross-session/context reuse must fail. Preserve privacy rather than copying the reference cache key design.
- **Source evidence:** Unassigned; this is an unassessed frontier and requires current owner evidence.
- **Current repository evidence:** [internal/nlqexec/service.go:84](../internal/nlqexec/service.go#L84); [internal/nlqexec/service.go:523](../internal/nlqexec/service.go#L523).

### EXP-02 — Calibrated operating configuration; 05/18/24/34

- **Status:** unassessed/pending frontier; not a new confirmed gap.
- **Boundary:** Reference has active/fallback prompt packs, template thresholds and optimization records. Source checkout is not the owner's calibrated deployed database.
- **Contract, storage and import impact:** Calibration manifests must be externalized as versioned configuration/state with no credentials; import/export and rollback need explicit unknown-field handling.
- **Required comparison and closure evidence:** Inventory approved prompts/settings/example weights/model and embedding versions outside repo; map every retained setting and explicitly reject unknown ones. Reproduce a held-out result set before/after rollback. Record exact revisions, model configuration, source/data snapshot and budget. Never import credentials or auto-promote old calibration.
- **Source evidence:** Unassigned; this is an unassessed frontier and requires current owner evidence.
- **Current repository evidence:** Pending owner evidence at the completed implementation head.

### EXP-03 — Query interaction and diagnostics; 18/21/22/23/31

- **Status:** unassessed/pending frontier; not a new confirmed gap.
- **Boundary:** Core cancellation/refinement exists; historical UI had SQL/table/chart, confidence/risk, feedback, progress and fallback interactions (research brief06). Ownership of a standalone authoring app has intentionally changed.
- **Contract, storage and import impact:** Consumer event schemas, cancellation/error states and stale-response rules must be shared by every supported surface; no standalone authoring product is implied.
- **Required comparison and closure evidence:** Contract journey: start, progress, clarify, cancel, inspect result, switch view, send feedback, refine. Verify stale responses cannot replace newer results; disconnect differs from explicit cancellation; truncated/empty/failed/uncertain are distinguishable. Plain-language actions must map to actual operations through each supported consumer. No requirement to recreate a standalone app or IAM.
- **Source evidence:** Unassigned; this is an unassessed frontier and requires current owner evidence.
- **Current repository evidence:** Pending owner evidence at the completed implementation head.

### EXP-04 — Upload fidelity and downstream closure; 11/12/15/33/34

- **Status:** unassessed/pending frontier; not a new confirmed gap.
- **Boundary:** Phase11 supports bounded CSV/XLSX/Parquet, exact declared values and explicit sheet selection; parser subset limitations are documented.
- **Contract, storage and import impact:** Upload parser, profile, semantic draft, query, report and erasure contracts must preserve supported format decisions and explicit unsupported diagnostics.
- **Required comparison and closure evidence:** Matrix of supported encodings/delimiters/headers, sheet choices, null/empty, locale decimals, timestamps/timezones, large identifiers and unsupported formulas/encodings. Follow representative upload through profile, semantic draft, clarification, query and report. Verify full erasure across derived data and retained outputs; unsupported variants get explicit diagnostics. Do not mark arbitrary spreadsheet execution as parity.
- **Source evidence:** Unassigned; this is an unassessed frontier and requires current owner evidence.
- **Current repository evidence:** [docs/plans/phase-11-uploads-workspace.md:31](../docs/plans/phase-11-uploads-workspace.md#L31); [docs/plans/phase-12-engineering-profiling.md:31](../docs/plans/phase-12-engineering-profiling.md#L31).

### EXP-05 — Rule interaction and semantic edits; 15/16/17/24

- **Status:** unassessed/pending frontier; not a new confirmed gap.
- **Boundary:** Deterministic rule replay and topic lifecycle retained; rich scope/context gaps tracked SEM/CTX/RUL.
- **Contract, storage and import impact:** Rule and semantic-edit revisions need conflict/selection evidence, immutable publication boundaries and rollback/replay records.
- **Required comparison and closure evidence:** Multiple simultaneously matching rules, exclusions versus pinned metrics, changed entity identities, retired relationships, conflict explanation and rollback. Verify a choice cannot weaken authority and a semantic edit cannot mutate a previously published definition. Save attributable selection/conflict evidence without leaking hidden metadata.
- **Source evidence:** Unassigned; this is an unassessed frontier and requires current owner evidence.
- **Current repository evidence:** [internal/semantics/rules.go:24](../internal/semantics/rules.go#L24); [docs/reviews/phase-16-rules-evidence.md:1](../docs/reviews/phase-16-rules-evidence.md#L1).

### EXP-06 — Report filter and layout semantics; 28/29/31/34

- **Status:** unassessed/pending frontier; not a new confirmed gap.
- **Boundary:** Pending planned consumers; R01–R16 remain obligations, not regressions.
- **Contract, storage and import impact:** Report filter/layout/run-manifest contracts must preserve precedence, distinct parameterized runs, empty states, locale labels and private previews.
- **Required comparison and closure evidence:** Table of global/local/default/explicit values, typed parameter bindings and precedence. Shared block with distinct parameters must run separately; truly equivalent approved runs may share only within exact authority/context. Test empty selections, zero visible pages, locale labels, partial failure, widget order and private preview retained after publication.
- **Source evidence:** Unassigned; this is an unassessed frontier and requires current owner evidence.
- **Current repository evidence:** Pending owner evidence at the completed implementation head.

### EXP-07 — Scheduled business windows and delivery; 06/28/30/34

- **Status:** unassessed/pending frontier; not a new confirmed gap.
- **Boundary:** Queue foundation present, report handlers pending30.
- **Contract, storage and import impact:** Occurrence, authority, artifact and delivery records must be separate and idempotent, with accepted windows and revisions pinned across retries.
- **Required comparison and closure evidence:** First/prior occurrence, DST gap/fold, leap boundaries, missed windows, retry after midnight, pause/edit/resume and latest-published pinning. Compare accepted due time/window and exact manifest across crashes. Artifact creation, catalog delivery and notification receipts are separate outcomes. Cutover creates one logical occurrence stream. Excluded event/condition stubs stay excluded.
- **Source evidence:** Unassigned; this is an unassessed frontier and requires current owner evidence.
- **Current repository evidence:** Pending owner evidence at the completed implementation head.

### EXP-08 — Portability, retention and provenance; 28/29/32/34

- **Status:** unassessed/pending frontier; not a new confirmed gap.
- **Boundary:** Safe topic portability exists, full calibration/report import pending.
- **Contract, storage and import impact:** Portability manifests must include provenance, retention and erasure dispositions at every dependent layer, with idempotent import.
- **Required comparison and closure evidence:** Inventory fields at every layer: topic, rule, clarification, templates/examples, chart bindings/formats, block, report, schedule and historical certificate. Dry run must report every dropped/transformed/unsupported field, not merely valid JSON. Replay import idempotently. Erase source-derived retained payloads/renditions without claiming backup overwrite or deleting unrelated objects. Imported certificates remain historical, not current approval.
- **Source evidence:** Unassigned; this is an unassessed frontier and requires current owner evidence.
- **Current repository evidence:** [internal/semantics/portable.go:28](../internal/semantics/portable.go#L28); [docs/plans/phase-34-migration-parity-cutover.md:17](../docs/plans/phase-34-migration-parity-cutover.md#L17).

### EXP-09 — Source and dialect semantic equivalence; 09/10/14/18/19/24/34

- **Status:** unassessed/pending frontier; not a new confirmed gap.
- **Boundary:** Six engine contracts are not six live calibrated workload proofs.
- **Contract, storage and import impact:** Each dialect adapter needs a contract matrix and independent live/fixture evidence; no dialect fallback may silently change meaning.
- **Required comparison and closure evidence:** Per engine: identifier case, quoting, dates/timezones, decimals/overflow, null ordering, aggregation, safe functions, parameters, limits, cancellation and schema drift. Compare execution meaning for NLQ, BYO and frozen blocks. Record live versus fixture evidence independently; unsupported engine/cohort cannot silently fall back to another dialect.
- **Source evidence:** Unassigned; this is an unassessed frontier and requires current owner evidence.
- **Current repository evidence:** [docs/plans/phase-14-warehouse-drivers.md:42](../docs/plans/phase-14-warehouse-drivers.md#L42).

### EXP-10 — Budget/failure/recovery richness; 01/05/06/10/18/24/28/30

- **Status:** unassessed/pending frontier; not a new confirmed gap.
- **Boundary:** Current typed uncertainty and validation boundaries are improvements.
- **Contract, storage and import impact:** Budgets, typed failures, uncertain physical attempts, retries and retained artifacts need one truthful receipt model and bounded recovery tests.
- **Required comparison and closure evidence:** Separate route/generate/correct/rerank/narrative budgets and cost unknowns. Provider timeout, malformed output, warehouse cancellation, unavailable metadata, crash after physical acceptance, expired artifact replay. Assert bounded retries and truthful state; missing output must never count as an empty correct answer. Stress only after these scenarios have behavioral acceptance.
- **Source evidence:** Unassigned; this is an unassessed frontier and requires current owner evidence.
- **Current repository evidence:** [internal/exec/execution.go:446](../internal/exec/execution.go#L446); [internal/exec/results.go:25](../internal/exec/results.go#L25).

### EXP-11 — Consumer parity after later phases; 21/22/23 plus each domain

- **Status:** unassessed/pending frontier; not a new confirmed gap.
- **Boundary:** Thin surfaces share core operations now; later operations are not proved by earlier shell completion.
- **Contract, storage and import impact:** Every new operation needs registration, scope, schema/error, client exposure, idempotency and cancellation checks in one conformance matrix.
- **Required comparison and closure evidence:** After every new domain operation, verify registration, scope loader, schema/error, SDK/CLI/MCP accessibility, idempotency and cancellation meaning. Complete end-user journeys, not endpoint-count equality. Data/SQL visibility and export scopes remain distinct.
- **Source evidence:** Unassigned; this is an unassessed frontier and requires current owner evidence.
- **Current repository evidence:** Pending owner evidence at the completed implementation head.

### EXP-12 — Rendering/export fidelity and interaction; 20/28/31/32

- **Status:** unassessed/pending frontier; not a new confirmed gap.
- **Boundary:** Current specification layer is not finished viewer/static rendering.
- **Contract, storage and import impact:** Chart specification, output policy, viewer, static renderer and export fields must be versioned together; read/render paths must remain source/model-free.
- **Required comparison and closure evidence:** Compare supported data shape × chart/output × locale × interactive/static/export. Exact values, order, unit, omissions, nulls, labels and provenance must agree. Viewer redraw and retained pagination make no model/source call; filters that change data create explicit authorized runs. SVG/HTML/CSV safety and credential-free rendering stay required. PNG/PDF support must be explicitly scoped, not inferred from predecessor UI export.
- **Source evidence:** Unassigned; this is an unassessed frontier and requires current owner evidence.
- **Current repository evidence:** [internal/charts/model.go:135](../internal/charts/model.go#L135); [docs/plans/phase-32-reporting-rendering-embed.md:17](../docs/plans/phase-32-reporting-rendering-embed.md#L17).

### Frontier rebaseline worksheet

At the completed implementation head record: implementation SHA and active phase/decision; reference behavior or target-only design; status (`preserved`, `equivalent`, `confirmed-gap`, `pending`, `unassessed` or `excluded`); synthetic reproduction and expected result; current result and evidence type; owner/dependency; contract/storage/migration impact; acceptance and negative case; approved resolution or remaining limitation. An empty evidence cell is unknown, not pass.

## Clarification and underspecification UX expansion

This section expands the parity audit’s clarification findings into a redesign brief. It preserves the useful behavior of asking only when a question is materially underspecified, while replacing an opaque authoring surface and untyped answer handling with a governed, inspectable flow.

“Hard to understand” is a user report, not a measured usability conclusion. The inspected reference UI shows a multi-part authoring editor and a blocking answer panel, but the audit did not run usability sessions or measure completion, error, or abandonment. The proposals below are decisions to validate, not claims about the reference system’s production quality.

Evidence labels are abstract IDs mapped outside this repository. This section contains neutral behavior descriptions only.

### What the inspection verified

| Evidence | Verified behavior | Limit |
|---|---|---|
| U-01 | Topic-scoped patterns describe an information category, presentation text, suggestions, optional free text, triggering signals, validity signals, temporal-grain hints, severity, and active/shadow state. | Field presence does not prove every field affects generated SQL. |
| U-02 | Detection is conditional: literal terms, semantic labels, and regular expressions can produce a firing strength. A supplied extracted value can suppress or re-open a request according to policy and validity signals. | The matcher is heuristic, not calibrated semantic-understanding proof; invalid regular expressions are skipped. |
| U-03 | Pattern evaluation can create multiple requests; request construction merges duplicates and orders by severity, confidence, and stable identifier. Some first-match paths depend on stored pattern order. | Deterministic does not mean understandable or best for users. |
| U-04 | A response can carry a selected option or free text plus a slot/category identifier. Refinement distinguishes some corrective responses from additive context. | Several values remain strings or generic payloads. This does not establish typed parsing, parameter binding, or data-safety proof for every category. |
| U-05 | The inspected end-user flow is wired: the panel submits responses through session refinement; the backend sanitizes text, adds it as delta context, and invokes a language-model query rewrite before the next planning pass. | This confirms a working conversational path, but it is a natural-language rewrite path rather than typed constraint propagation. A sanitization step is not semantic validation. |
| U-06 | The user panel exposes suggested choices, optional text input, reason/impact explanation, and requires every displayed semantic clarification before submit. Separate ambiguity UI distinguishes resolved assumptions from blocking questions. | Component tests assert rendering/submission shape, not comprehension or accessibility beyond tested mechanics. |
| U-07 | Topic authors can create, edit, activate, and replay patterns. The editor exposes triggers, policy, validation values/patterns, temporal-grain lists, examples, severity, and free-text controls. | This proves authoring breadth, not that precedence, conflicts, or downstream effects are understood. |
| C-01 | The current rule contract supports five presentation input kinds and published ruleset lifecycle/replay/shadow. Its contract says question matching is later work. | Lifecycle is real but does not provide conditional activation. |
| C-02 | Current routing iterates every published pattern. A missing required slot stops before retrieval; only a choice slot has kind-specific handling and can add a semantic reference. | Direct runtime data flow, not an observed production incident. |
| C-03 | Current non-choice values pass only a nonempty check and do not enter assembled context or the generation prompt. | The original question might contain similar text; that does not make the supplied answer a governed constraint. |
| C-04 | Current acceptance tests cover required choice blocking and known choice target evaluation. | No inspected test covers conditional matching, typed parsing, value propagation, ordering/conflicts, session resume, or bilingual clarification behavior. |

### Design conclusion

The target should preserve the useful conversational loop, but not use a model rewrite as the sole semantic effect of a clarification or a free-form regular-expression editor as the ordinary authoring surface. It should implement a small reviewed clarification policy whose answers become typed, sealed planning inputs.

A clarification is justified only when all are true:

1. The routed topic/version and signed reach are current and admitted.
2. A reviewed policy condition matches the question or a typed interpretation result.
3. The value is absent, invalid, conflicting, or cannot safely default.
4. A valid answer can become a semantic constraint, reference choice, or explicit no-plan outcome.

If conversion has no defined safe effect, return a typed unsupported/clarify outcome instead of accepting text with no planning effect.

### Proposed authoring contract

This is a proposal, not current implementation.

A published clarification policy contains:

- Stable policy ID/version, exact topic version/digest, lifecycle state, locale-independent semantic references, review evidence, and deterministic priority.
- A safe reviewed “when” clause: routed semantic reference, canonical intent/tag, typed temporal/entity interpretation, or approved literal vocabulary match. The ordinary authoring UI accepts no arbitrary regular expressions.
- A “needs” clause: absent, invalid, ambiguous, or conflicting typed value.
- Stable slot ID, sensitivity, answer type, allowed resolution modes, display keys, option source, and default policy.
- An “effect”: add exact semantic reference; bind typed time/filter/sort/limit constraint; or stop with declared unsupported outcome. A policy with no effect cannot publish.
- Explicit conflict group and ordering key.

A migration tool may translate a legacy regular-expression condition only where it can produce an equivalent reviewed safe matcher. Otherwise it reports manual rewrite. Raw pattern execution is not retained only to preserve a field.

### Recommended slot families

| Family | Accepted answer | Required typed result | Safe effect |
|---|---|---|---|
| Reference choice | Reviewed option ID | Exact semantic reference/revision | Add reference to sealed route selection. |
| Time window | Named reviewed window or locale-aware date input | Canonical start/end, calendar/time-zone policy, temporal dimension | Bind validated temporal constraint. |
| Entity/filter | Reviewed dimension and value mapping, or permitted normalized value | Dimension, operator, normalized value(s), null semantics | Bind validated filter constraint. |
| Numeric threshold | Locale-aware number plus closed operator/unit | Exact decimal/range, unit, operator, target | Bind validated numeric constraint. |
| Boolean | Closed true/false/unknown choice | Boolean value and target | Bind predicate or return explicit unsupported outcome. |
| Bounded text | Short policy-permitted text | Resolved governed value or unresolved result | Never treat raw text as an implied filter. Ask again, map it, or stop. |

Existing presentation kinds may remain an API compatibility layer, but they are insufficient semantic types. A date is not any nonempty string, and text does not imply a safe warehouse predicate.

### Proposed interaction sequence

1. Route and interpret: admit topic and signed context before clarification. Run deterministic locale-aware interpretation over sealed question and prior typed resolutions.
2. Evaluate policies: active policies are pinned to admitted topic revision. Each emits not-applicable, satisfied, invalid, needs-answer, or conflicts.
3. Present short ordered set: blockers first, then optional refinements. Each says what is needed and why it changes results; reviewed choices precede free text. Unrelated policies cannot block a complete question.
4. Validate and resolve: submit policy/version, slot ID, answer source, locale, session/query binding. Parse before provider work and return localized field errors; never silently coerce invalid values.
5. Seal and plan: convert accepted answers to typed resolved constraints. Add them to route/context seal, render an auditable non-sensitive mandatory context representation when generation needs it, and carry typed values to validation/parameter binding. Refinement replaces values only in same session under freshly checked authority.

The normal UI presents one concise question at a time when answers depend on one another. It may display independent blockers together with an explicit order and explanation. This proposal aims to reduce cognitive load and must be usability-tested.

### Ordering, defaults, conflicts, locale, session

These are proposed target rules.

- Sort by blocker status, policy specificity, author priority, then stable policy ID; never storage order.
- Merge only identical policy ID/effect. Different effects for same slot become typed conflict, never first-match wins.
- Optional reviewed defaults are visible and sealed as defaulted. Required policies never silently default or execute.
- Invalid supplied value prompts repair; absent value prompts choice. Neither becomes free-form prompt text.
- Persist locale-neutral semantic IDs and canonical typed values. Localize labels, parsing feedback, examples, and date formats at boundary. Add English/Spanish fixtures now; make no broader multilingual claim.
- Bind pending clarification set to session/query ID, topic/policy versions, and source/context reach. Resume rechecks current authority/publication; changed state returns stale-clarification and re-evaluates. Never retain/replay bearer tokens.
- Classify sensitive answers before storage/audit. Ordinary logs and provider prompts omit raw sensitive text by default.

### Typed propagation requirement

The answer must be observable through the whole path:

question + admitted topic/version + policy version -> typed resolution -> sealed route constraint -> assembled context -> generation/validation parameter binding -> query receipt/refinement replay.

A resolution records policy/version and slot; answer source; canonical typed and protected display values; target reference/operator/null behavior/time-zone or calendar where applicable; locale/parser version; validation/replacement relation; session/query and exact topic/ruleset/source-context pins.

Generated SQL still requires validated-read proof. Typed clarification is semantic evidence, not authority, raw SQL, or a bypass of dependency/function/statement enforcement.

### Authoring and review experience

Organize ordinary authoring around outcome:

1. Select reviewed semantic target.
2. Choose when it matters from constrained conditions.
3. Choose answer and typed resolution.
4. Preview matching/nonmatching synthetic questions in author locale.
5. Review effect, conflict group, default, and privacy.
6. Save draft; replay/shadow protected historical cases; publish after review.

Advanced migration detail, raw legacy matcher text, and ambiguous translations belong in protected migration review, not ordinary authoring. Every published policy needs a human-readable explanation derived from typed conditions/effect.

### Acceptance corpus

| ID | Case | Required assertion |
|---|---|---|
| CLAR-AC01 | Matching versus unrelated question | Metric-specific required policy fires only for reviewed condition; unrelated complete question reaches retrieval. |
| CLAR-AC02 | Required time window | Missing window blocks before provider work; valid English/Spanish answers produce same canonical bounds under declared calendar/time-zone. |
| CLAR-AC03 | Invalid typed value | Invalid date, number, boolean, unsupported grain, and unauthorized option fail before provider work with no partial constraint. |
| CLAR-AC04 | Choice and entity mapping | Reviewed option resolves exact semantic reference; mapped entity produces declared dimension/operator/value/null constraint. |
| CLAR-AC05 | Typed propagation | Accepted answer appears in sealed route and mandatory generation/validation inputs; resulting plan preserves it through refinement. |
| CLAR-AC06 | Conflicts and ordering | Incompatible policies produce typed conflict; independent blockers have stable order independent of persistence order. |
| CLAR-AC07 | Defaults and skips | Optional reviewed default is visibly sealed; required answer cannot skip or silently default. |
| CLAR-AC08 | Version/session safety | Changed policy/topic/current reach returns stale/re-evaluate; same-session replacement supersedes old resolution without widening authority. |
| CLAR-AC09 | Budget and privacy | Mandatory resolved group fits completely or returns typed insufficiency; sensitive raw answer absent from ordinary log/provider fixtures. |
| CLAR-AC10 | Authoring/replay | Draft preview, shadow/replay, publication, retirement deterministic; unsafe matcher imports transformed or rejected. |
| CLAR-AC11 | UX comprehension study | With representative authors/users, measure completion, correction, time, and explanation comprehension. This is separate usability research, not a unit-test substitute. |

### Delivery sequence

1. Add neutral policy/resolution contract and decide target mapping for every imported legacy pattern category.
2. Fix current typed-response data flow before further NLQ/reporting consumers depend on it.
3. Implement deterministic conditions, parser/normalizer seams, typed propagation, conflict/default/session behavior, and focused acceptance.
4. Add authoring preview/shadow and protected migration translation report.
5. Evaluate query outcomes and UX separately in planned quality/cutover work. Structural tests do not prove accuracy or usability parity.

## Chart breadth and richness expansion

### Scope and terminology

Both implementations name fourteen catalog kinds. That is useful baseline coverage, but **a named kind is not equivalent to every executable binding shape, selected form, formatting choice, or reporting-output policy**.

The current Go package is deliberately a sealed specification/data transformation boundary. It accepts caller-provided qualified result data, produces a typed output with exact labels, and does not emit pixels, ECharts option dictionaries, or client interaction code. The predecessor presentation path built renderer-oriented options for most kinds. This document treats that difference as a planned rendering boundary unless a field needed to preserve authored meaning is already absent from the current specification.

External evidence maps provide exact locations. The body uses neutral source identifiers rather than copying source code, prompts or private source details.

| Source ID | Neutral description |
|---|---|
| O-CATALOG | predecessor chart catalog and declared slot contracts |
| O-GEN | predecessor deterministic per-kind option builders |
| O-SELECT | predecessor slot binding, suitability, alternatives, and optional ranking |
| O-REPORT | predecessor reporting-output definition and execution formatting |
| C-CORE | current provider-neutral chart model, selector, mapping validation, and build |
| C-CONTRACT | current documented chart specification boundary |
| C-PLAN | current phase ownership and planned viewer/render/export boundary |

### Executive assessment

- **Catalog breadth is nominally retained:** both surfaces have area, bar, column, donut, grouped bar, heatmap, KPI, line, pie, scatter, stacked bar, stacked column, table, and treemap. Current names `kpi`; predecessor names `kpi_card`. The 14 chart catalog entries are 12 graphical plot kinds plus KPI and table; narrative is a separate reporting-output union.
- **Current strengths are substantive:** typed closed mappings, exact text labels with explicit null state, strict type/semantic pins, no renderer-originated SQL, deterministic fallback, non-lossy exact totals where mathematically valid, and explicit unsuitable/mapping-changed failures.
- **The principal definition gaps are multi-value bindings, a scatter size channel, deep treemap hierarchy, per-column display/format fields, and output-specific KPI/table settings.** These are information-model dependencies that viewer/rendering consumers will inherit; an eventual renderer cannot infer discarded fields.
- **The principal behavior changes are intentional or require an equivalence decision:** Go requires one row for a KPI, refuses duplicate non-scatter category tuples rather than silently choosing a value, rejects negative pie/donut/treemap geometry, and uses no question text in deterministic selection. These may be safer than predecessor behavior but are not output-equivalent without an explicit migration policy.
- **Do not claim current interaction or export parity.** The predecessor frontend has browser CSV download, best-effort chart PNG download, and a table-view toggle. Current Phase 32 explicitly plans JSON/CSV/HTML/SVG exports while excluding advertised PNG/PDF/page-layout support. PNG is therefore an intentional current scope difference requiring an equivalence decision, not a Phase 20 core defect. Full drilldown remains unestablished in this bounded review.

### Core model comparison

| Concern | Current retained behavior | Missing/narrower behavior or decision |
|---|---|---|
| Result fidelity | `Cell` retains null separately from exact textual value; `Value` carries exact label plus optional approximate geometry; totals preserve exact rational arithmetic for eligible supplied result columns. | Renderer-ready formatted value text is not created by the chart core; current `Format` cannot hold locale, date pattern, or currency-symbol fallback. |
| Provenance and drift | Every saved binding pins type, role, grain, aggregation, format, source/topic revision, and semantic ID. Rebinding is explicit and rejects incompatible drift. | No separate human-facing display label, query-role, semantic subtype, cardinality bucket, or per-column source-ref variants equivalent to the predecessor presentation metadata. |
| Mapping | Closed slots prevent arbitrary scripts/URLs. Tables use an ordered selected-column list. | Every non-table slot is scalar. There is no general repeated slot model. |
| Selection | Rules-first selection supplies a bounded selected candidate plus alternatives; invalid candidates are excluded and table fallback is labeled. | The deterministic selector examines result shape only. `Intent` reaches only optional ranking, so it cannot affect rules-stage candidate eligibility/score. The predecessor scoring uses question intent, cardinality and semantic metadata. |
| Validation | Current validates row shape, typed values, bounds, slot suitability, and exact saved pins; it rejects changed mappings rather than rebinding silently. | Strict pre-aggregation/uniqueness rules change behavior for duplicate category tuples; a compatibility decision is needed where predecessor generators aggregate or overwrite. |
| Renderer boundary | Build returns sealed, bounded typed data and no source/model action. | It does not produce renderer options, legends, tooltip behavior, client grid payloads, or pixels. This is planned work, not a Phase 20 defect. |

### Per-kind matrix

“Current executable shape” describes what the current validator and builder can actually accept, rather than only the catalog name. “Reference shape” records the declared presentation contract and the implemented generator behavior where it differs.

| Kind | Reference shape and implemented behavior | Current executable shape | Parity and required decision |
|---|---|---|---|
| Area | Temporal x; **multiple** measure y columns; optional dimension series. Without series, each y measure is rendered as a series; with series, the first y measure is split by series. Area uses the same line construction with filled area styling. | One temporal `Category` and one numeric `Value`; no `Series` slot is permitted for area. Rows with null time are omitted; missing values form gaps in the typed output. | **Narrower.** No multi-measure or series-breakdown form. Preserve a long-form transformation only if it is typed, does not rerun SQL, and preserves units/order/gaps. |
| Bar | Category plus **multiple** values; optional series breakdown. Horizontal orientation. With series, the first value is grouped by category/series; otherwise each value is a series. | One `Category`, one `Value`. The base bar catalog has no permitted `Series` slot. | **Narrower.** The current name covers a single-measure categorical comparison, not the reference’s multi-measure or optional-series forms. |
| Column | Same declared slots as bar and vertical orientation; multi-measure and optional-series forms are implemented through the shared builder. | One `Category`, one `Value`; no base-column series slot. | **Narrower** for the same reason as bar. |
| Donut | One category and one value; low-cardinality composition preference; deterministic builder aggregates categories, sorts descending, and uses a ring radius. | One `Category`, one `Value`; negative values are unsuitable and null category/value rows are omitted. Output remains a typed point set, not an option dictionary. | **Comparable basic shape, behavior changed.** Current explicit negative rejection avoids invalid geometry; a migration must decide whether to reject, transform, or display predecessor negative compositions. Ring styling is rendering-owned. |
| Grouped bar | Category plus **multiple** values and optional series. It renders multiple unstacked measures when no series is bound; when series exists, it uses the first measure split by series. | `Category`, `Series`, and `Value` are all required; only one measure is allowed. | **Narrower and differently shaped.** Current requires a series dimension where reference can compare multiple measures directly. |
| Heatmap | Two dimension axes and one value. The builder constructs category axes, a visual scale, and a cell grid; duplicate x/y values overwrite by last observed row. | `X`, `Y`, `Value`, each scalar; x/y must be category-eligible and duplicate coordinate pairs are rejected by current mapping validation. | **Same nominal shape, changed duplicate policy.** Current requires the query to be unambiguous/pre-aggregated; this is safer but not a transparent migration of last-row-wins. Visual scale and palette await rendering. |
| KPI / KPI card | Generic presentation catalog: one value and optional label; the generator uses the first row and returns a renderer-specific card payload. Separate reporting-output KPI definitions additionally support comparison, delta, percent delta, trend, target, thresholds and sparkline. | Chart core: one numeric `Value`, exactly one result row. Reporting `Output` only wraps the shared chart mapping; it has no KPI comparison/trend/target/threshold fields. | **Basic card shape retained but materially narrower for reporting.** The generic predecessor card is not evidence of rich reporting KPI parity; the reporting KPI contract is the correct comparator for targets/deltas/etc. |
| Line | Temporal x; **multiple** measure y columns; optional series. Without series it emits one line per y measure. With series it uses the first y measure and preserves gaps for missing values. | One temporal `Category`, one numeric `Value`; sorted ascending by current mapping. No `Series` slot permitted. | **Narrower.** Current represents one-measure time series only. It preserves exact labels and null gaps at its typed boundary. |
| Pie | One category and one value; sorted composition values with item tooltip/legend. | One `Category`, one `Value`; negative values are unsuitable; typed output does not include legend/tooltip settings. | **Comparable basic shape, behavior changed.** Negative policy and rendering presentation need an explicit equivalence decision. |
| Scatter | x and y measures; optional categorical series and optional **size** measure. Size is rendered as a declarative bubble visual scale. | Numeric scalar `X` and `Y`; optional scalar `Series`; repeated observations are permitted. There is no size/bubble slot or value channel. | **Narrower.** Grouped scatter is retained; bubble scatter is not representable. |
| Stacked bar | Category, one value, and required series; values aggregate by category/series into horizontal stacks. | Same three scalar slots and current exact typed values. | **Basic shape retained.** Current has no renderer stack/legend options yet, which is Phase 31/32. Current validation still differs where source rows are duplicate/invalid. |
| Stacked column | Same as stacked bar, vertical orientation. | Same three scalar slots. | **Basic shape retained** at the sealed-data level; rendering orientation/legend remains pending. |
| Table | All selected columns in order; predecessor generic presentation emits table rows plus per-column formatting hints. Reporting table definitions add per-column visibility/label/format, default sort, page size and show-totals. | `Columns []` is ordered and supports selected-column projection; `Order` supports stable current result sort. Build carries every projected row and creates exact additive totals for eligible result columns. | **Core table projection/sort/totals retained.** Missing persisted visibility, localized column labels, page size, per-column formatting and show-totals intent. Do not state “totals are missing”: current totals exist, but cannot encode whether a saved reporting table wants them shown. |
| Treemap | One or **more** category levels plus one value. Builder creates arbitrary-depth nested hierarchy and rolls totals upward. | One `Category`, optional one `Parent`, and one `Value`; maximum hierarchy depth is two. Negative values are unsuitable. | **Narrower.** Current supports only a one-parent hierarchy, not arbitrary category-depth paths. Geometry/palette/tooltip remain rendering-owned. |

### Selection and validation details that alter observable breadth

1. **Question-aware deterministic selection is missing.** The reference rules score temporal, comparison, composition, and correlation language, together with cardinality and semantic metadata. Current `charts.Select` has no question/intent parameter and assigns fixed shape-driven scores. `chartservice.Select` accepts `Intent`, but passes it only to an optional ranker after the sealed deterministic candidate set exists. A ranker cannot introduce a kind/binding that rules excluded. This is a Phase 20 selection-rules gap, while calibration/evaluation is Phase 24.

2. **Current candidate space is smaller before ranking.** Current automatic selection never proposes multi-measure bar/column/line/area, generic bar/column series splitting, bubble scatter, or deep treemap. Optional rank can reorder only current candidates. Resolve this dependency risk during expansion, before viewer/rendering consumers freeze the shape, or record an explicit typed transformation.

3. **Current validation is more conservative in useful ways.** It preserves exact column pins and rejects incompatible remapping. It also rejects duplicate category/series tuples for non-scatter forms instead of silently aggregating or replacing values. Retain that safety property unless a reviewed compatibility transform explicitly names aggregation and its provenance.

4. **Reference behavior contains its own limitations and should not be treated as a perfect target.** Several reference builders use floating conversion for geometry, and some multi-value/series combinations intentionally use only the first value column. The reference line builder is last-observation-wins for duplicate x/series points; heatmap is last-observation-wins for duplicate cells. These are behavior facts, not requirements to reproduce silently.

5. **Ranker/cache equivalence needs caution.** The reference ranker uses question plus metadata/candidate descriptors and an in-memory shape-keyed cache. Current optional ranking receives only bounded intent and candidate kind/reason descriptors, and ranking is constrained to a current signed authority and sealed candidate set. Do not reintroduce predecessor cache scope if it conflicts with the current authority model; compare chart choice quality under the approved identity model instead.

### Formatting, labels, nulls, and exactness

| Topic | Current behavior | What must be added or explicitly mapped |
|---|---|---|
| Exact numbers | Exact textual values are kept even when geometric coordinates are approximate. Exact totals are available only for sum/count numeric columns without percent formatting, with complete-result vs returned-row scope. | A renderer must use exact labels/totals rather than reformatting float coordinates. This is a retained strength. |
| Format fields | Unit, ISO currency, percent convention, and fraction digits; validation rejects contradictory numeric semantics. | Per-column locale, date format, currency-symbol fallback, and a formatted-display policy are absent. Add these to the portable spec if authored output must survive. |
| Display labels | `Column.Name` and chart `Options.Title` exist. | Neither is a per-column localized display-label contract distinct from the executed alias. Use an explicit per-column label field rather than overloading stable IDs or SQL aliases. |
| Nulls | Cells have explicit nulls. Current kind rules omit invalid category/coordinate rows; line/area preserve missing value as a gap after a non-null temporal category; tables retain nulls. | Define how reference generator skipping/last-wins behavior maps per kind. Do not replace null with zero. |
| Labels and truncation | Current preserves exact labels and emits a label display bound for future renderers; output includes omitted-row and truncation warnings. | Viewer/static renderer must show the complete value accessibly and make `returned_rows` totals visibly distinct from source totals. |

### Reporting-output layer: separate from generic charts

The reusable chart package and the reporting block output definition have different responsibilities. Generic chart capability cannot prove reporting-output parity.

| Reporting concern | Current state | Disposition |
|---|---|---|
| Output identity/order/selection | Stable output IDs and definition array order; selection rejects duplicate/unknown IDs. | Retained core. |
| Output enabled state, localized output name/description, display order | Not represented on `reporting.Output`; block-level metadata is not a substitute. | Definition gap in Phase 27, before frozen-artifact consumption. |
| KPI comparison, delta, percent delta, trend, target, thresholds, sparkline | Not representable by the shared `charts.Mapping`, which only has scalar chart slots. | Definition gap in Phase 27; Phase 28/31/32 must consume it later. |
| Table selected column order and sort | Representable through `charts.Bindings.Columns` and `Mapping.Order`. | Retained. |
| Table saved visibility, localized labels, page size, per-column display format, show-totals preference | Not represented by current reporting Output/chart mapping. Generic core totals remain available. | Definition gap in Phase 27; do not describe it as absence of totals. |
| Narrative | Current narrative is bounded and has explicit redacted fields, type/tone/evidence/caveat requirements. | Narrative policy variants and inherited sensitivity metadata need their own comparison; they are not chart renderer defects. |

### Interaction and export boundaries

- The reference presentation files inspected build static ECharts option dictionaries (including tooltip, legend, heatmap visual scale, and bubble scale) and special payloads for KPI/table. Its query viewer also exposes browser-side CSV download, best-effort ECharts PNG download, and a user table-view toggle. This proves those client affordances existed; it does not prove a broader interaction product.
- Current Phase 31 owns the Apps viewer, locale/theme/resize/accessibility, result paging and filter-triggered authorized reruns. Phase 32 owns static HTML/SVG, exact-value agreement, and explicitly scoped JSON/CSV/HTML/SVG exports. It expressly does **not** advertise PNG/PDF/page-layout export. Treat PNG as an intentional current scope difference that needs an approved migration/equivalence disposition, rather than as a missing Phase 20 chart-core capability.
- This bounded review still found no supported predecessor contract establishing drilldown, brush, or click-to-filter as mandatory behavior. Do not add those as parity findings without a source consumer and test.
- The **portable fields required by a future viewer/exporter**—multi-slot bindings, full formatting, localized output/table labels, and output policies—must be decided before frozen run artifacts, because renderer code cannot reconstruct them from a scalar mapping.

### Suggested acceptance corpus

Use a synthetic, table-driven corpus that records expected behavior at the sealed chart output and later renderer boundary. Each case must pin source-independent columns, mappings, expected null/total/completeness state, and whether a requested behavior is rejected or transformed.

1. Two-measure time series with distinct units and missing values; expected two named series or documented long-form equivalent.
2. Categorical multi-measure bar/column and categorical series-split bar; verify no silent first-measure loss.
3. Bubble scatter with x/y/size/series; verify typed size preservation or explicit unsupported result.
4. Three-level treemap; verify path order, sums, null path handling, and negative-value policy.
5. Duplicate category/series and heatmap-cell inputs; assert current rejection or a reviewed, named aggregation rather than last-row selection.
6. Single-value KPI and reporting KPI with comparison/target/threshold/trend; distinguish generic card from reporting output semantics.
7. Reporting table with selected/hidden columns, localized labels, sort, page size, and show-totals preference; verify current additive totals separately.
8. Decimal/large integer/percentage/currency/date values with locale and date pattern; assert exact display and no float-derived label corruption.
9. Truncated result with eligible total; assert `returned_rows` scope survives viewer and export.
10. Selection corpus varying only question intent (comparison/composition/trend/correlation) and cardinality; assert the intended candidate set before optional ranking.

### Limits of this artifact

No runtime tests were run for this document. Existing static catalog/acceptance tests demonstrate the current specified boundary; they do not establish predecessor equivalence, interactive UI behavior, pixel output, live-provider quality, performance, or deployment acceptance. The predecessor source snapshot may contain incomplete or transitional behavior; its source presence alone does not make it a mandatory requirement. Any imported behavior needs a current owner, authority-preserving contract, migration disposition, and executable acceptance case.

### Stable chart subitems under the existing findings

These are stable subitems of VIS-02 and VIS-04, not additional finding IDs. The 26-finding count and the 63-feature ledger remain unchanged.

- **VIS-02a — Multi-measure and series bindings:** the catalog names are present, but the current non-table mappings are scalar. Closure requires a typed repeated-measure/series shape or a tested long-form transform that preserves units, labels, order, null gaps and provenance without rerunning the query. Evidence IDs: `O-CATALOG`, `O-GEN`, `C-CORE`; current anchors: [internal/charts/model.go:135](../internal/charts/model.go#L135), [internal/charts/mapping.go:9](../internal/charts/mapping.go#L9).
- **VIS-02b — Bubble scatter and deep treemap:** the reference accepts a scatter size channel and arbitrary-depth treemap levels; the current model has no size channel and caps the hierarchy at one parent. Closure requires either typed support or explicit unsupported/transformation records with deterministic negative-value and null-path policy. Evidence IDs: `O-CATALOG`, `O-GEN`, `C-CORE`; current anchor: [internal/charts/model.go:230](../internal/charts/model.go#L230).
- **VIS-04a — Question-aware deterministic selection:** intent/cardinality/semantic signals must participate before the candidate set is sealed, or the target must document a safe equivalent. Post-selection ranking cannot restore a candidate rejected by deterministic rules. Evidence IDs: `O-SELECT`, `C-CORE`; current anchors: [internal/charts/select.go:1](../internal/charts/select.go#L1), [internal/chartservice/service.go:193](../internal/chartservice/service.go#L193).

## Preserved and deliberately reworked behavior

- **Source and execution safety:** bounded uploads/profiles, exact typed result handling, source/context/revision fences, read-only validation, cancellation and uncertain-attempt reconciliation are real target boundaries. The expanded connector matrix is broader than the reference adapter baseline, but live per-engine qualification remains separate.
- **Privacy and authority:** identity, issuer and durable authority are owned by the authority provider. The target service verifies and enforces signed action/resource reach, source/context partitions, retention and execution boundaries. Reproducing a predecessor cache hit must not weaken those controls.
- **Profiles and onboarding:** the target deliberately avoids unrestricted raw samples and top-value lists. The replacement requirement is a reviewed, policy-bound value vocabulary, not a return to unrestricted sample exposure.
- **Charts:** the target retains typed closed mappings, exact labels, explicit nulls, table projection/order/sort and eligible exact totals. Missing multi-value bindings, richer KPI/table settings, full formatting and deep hierarchy are information-model issues; renderer/UI/export behavior remains phase31/32 work.
- **Reporting:** output kinds, immutable definitions and selected mappings are present, but frozen-run consumers must resolve the output enablement, rule/sensitivity provenance, policy and limits before they claim parity.
- **Explicit exclusion:** event/condition/custom-code scheduling stubs remain excluded by the active contract; their absence is not a gap to fix.

## Phased corrective sequence

Follow this order while retaining every mandatory phase and release obligation. The sequence organizes dependency risk; it does not waive acceptance criteria or turn planned status into shipped behavior.

1. **Finish planned implementation phases on the dependency DAG.** Complete in-progress 26/28 work and every remaining required phase in its declared dependency order, with each phase's named acceptance, schema, lifecycle and review obligations. Do not treat planned status as shipped, and do not silently widen a phase contract to absorb unresolved parity fields. Existing phase obligations remain in force.
2. **Rebaseline the merged implementation.** After the planned phase work is merged, pin the resulting SHA, active decisions, phase state, 26 findings, 63 ledger rows and 12 frontiers. Recheck each disposition against merged code. Do not repair stale phase counts or registry/status files as part of this artifact. Unmerged work is neither counted as shipped nor judged absent.
3. **Expand the behavior contracts.** Resolve semantic/context, clarification, rules, output, chart, sensitivity, policy and portability gaps with owners, migrations, serializers, import/export, SDK/client shapes and real consumers. Required parity/cutover work remains owned by phase34; separately approved optional expansions may be scheduled without being forced into phase34. Preserve source safety and explicit exclusions.
4. **Functional and quality closure.** Run deterministic negative/positive sentinel tests, then phase24 evaluation and phase34 differential/cutover comparisons over approved synthetic/private boundaries. Phase34 parity/cutover and phase25 release cannot be called complete until their evidence exists. Compare meaning/results, not SQL text alone; separate model, source and service measurements.
5. **Stress after functional and quality closure.** Exercise cold/warm reuse, concurrency, cancellation, retries, crash recovery, DST/leap windows, retention/erasure and viewer/static/export paths. Require truthful receipts, no stale authority/context reuse and no renderer-triggered source/model work. Only then make final release claims.

## Comparison method and sentinel corpus

For each behavior trace input/model → normalization → persistence → retrieval/selection → context → validation/execution → output → export/replay. A field only present in storage, a method name, or a generic success test is insufficient. A different architecture is acceptable when the observable outcome and safety boundary are equivalent and the mapping is explicit.

Use three evidence levels:

1. **Deterministic contract tests:** synthetic fields, rule truth tables, typed slots, dependency closure, periods, output selection, chart shapes, normalization and lifecycle transitions; no paid model required.
2. **Pipeline replay:** hold reviewed semantic/rule/template content, resolved time, provider responses and source data fixed; compare route, normalized filters, context facts, safety decisions, result values and outputs.
3. **Calibrated comparison:** pin approved model/configuration, calibrated state and source/data snapshot; compare correctness, clarifications, corrections, tokens/calls/cost and latency separately. Never infer a parity percentage from file or test counts.

Minimum cases include: derived KPI under context pressure; English/Spanish aliases and geography; month-only grain; matching versus unrelated clarification; compound rules; feedback conflicts; follow-up correction/removal; rule-only block change; period wording/default conflict; KPI target/threshold/trend; multi-measure chart; localized table; sensitive narrative column; schedule retry across DST/publication change; and import of calibrated dependent assets. The clarification and chart sections below add their dedicated acceptance matrices.

## 63-feature continuity ledger

Every B01–B20, R01–R16, Q01–Q11 and N01–N16 row is preserved below. This is a continuity inventory, not a score; “retained” means the inspected core boundary exists, not that every variant or live workload has passed.

This cross-reference preserves every source continuity ID. It is an audit disposition ledger, not a parity score or a replacement for the repository registry. “Retained” means inspected code implements the stated core boundary; it does not certify every variant or a new passing runtime run. Pending report/schedule rows are plan-owned obligations, not findings that unfinished phases regressed. The detailed audit lists atomic gaps beneath these broad feature labels.

The repository coverage map is linked here for traceability only. No mapping alone proves that the original behavior is implemented.

| ID | Original capability | Current assessment | Detail / next evidence | Existing AC mapping |
|---|---|---|---|---|
| B01 | Stable block identity, localized name/question, canonical question and aliases | partial; block metadata retained | BLK-01; per-output localized identity incomplete | 27.AC01 |
| B02 | Mutable draft versus immutable published revision | retained core inspected | CAS and immutable revisions; no new runtime rerun | 27.AC02 |
| B03 | Publication, certification and current trust/health are separate | retained core with dependency gap | BLK-02; separate publication/certification/health exists | 27.AC04 |
| B04 | Exact topic/template/dependency references and definition hashes | gap | BLK-02 / LRN-01; missing rule/template dependency continuity | 27.AC03, 27.AC07 |
| B05 | Real validation evidence bound to content and observed schema | retained core with incomplete dependency domain | Real schema/query evidence exists; BLK-02 remains | 27.AC03 |
| B06 | Read metadata without automatically exposing SQL | retained core inspected | Separate SQL-read projection and action | 27.AC06, 04.AC04 |
| B07 | One saved query can feed chart, KPI, table and narrative outputs | runtime pending28; definition gap | VIS-01; saved output kinds exist, richer KPI behavior absent | 28.AC01, 28.AC02 |
| B08 | Enabled/default output selection, output identifiers and mappings | gap | BLK-01; IDs/subsets/order retained, enabled state missing | 27.AC06, 28.AC02 |
| B09 | Date/datetime/relative-period/dimension/number/integer/boolean/grain/top-N parameters | retained typed resolution; runtime pending28 | Dates/periods/scalars/grain/top-N and explicit dimension refs | 27.AC05, 28.AC03 |
| B10 | Locale, report timezone and explicit parameter provenance | partial | VIS-03; timezone resolution retained, formatting intent narrowed | 28.AC03 |
| B11 | Period authoring: explicit range, previous period, rolling periods, schedule window | retained resolution with authoring gap | BLK-03/06; period maths exists, wording checks/workflow narrowed | 27.AC05, 30.AC03 |
| B12 | Assisted parameterization and question-duplicate assessment | gap | BLK-04/06; lexical duplicate assessment and narrow parameterization | 27.AC05, 27.AC01 |
| B13 | Exact revision and latest-published/latest-certified selection | execution policies pending28 | Exact revision reads exist; compare all floating-policy variants at run admission | 29.AC03, 30.AC04 |
| B14 | Published/certified-only/explicit-stale/private-preview trust policies | execution policies pending28 | Separate current trust exists; full execution trust modes remain pending | 27.AC04, 29.AC02 |
| B15 | Expected ordered columns, types, nullability and sensitive-field metadata | partial; gap | BLK-05; ordered exact typed schema exists, sensitivity handoff missing | 28.AC03, 10.AC03 |
| B16 | Execution traces prove absence of interpret/generate/rewrite/select-chart stages | pending28 | Forbidden-stage spies must cover actual frozen runtime | 28.AC01 |
| B17 | Bounded narrative generation, approved columns, evidence, tone/locale and budgets | pending28; definition mapping needed | BLK-05/07; redaction, max claims, analysis/tone/caveat policies | 28.AC04 |
| B18 | Schema/semantic impact, exact rename detection and dependent health | partial | Source/topic impact and safe rename exist; rule-only dependencies absent BLK-02 | 27.AC07 |
| B19 | Revalidation / withdrawn approval / unavailable source | partial | Current source/topic health and withdrawal exist; rule snapshot gap BLK-02 | 27.AC04, 27.AC07 |
| B20 | Idempotency and expired retained block outputs | pending28 | Expired-artifact idempotent replay must not re-execute | 28.AC05, 28.AC07 |
| R01 | Reports compose multiple approved blocks and outputs | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 29.AC01, 29.AC03 |
| R02 | Draft, pending review, publication, rejection and amendment lifecycle | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 29.AC01, 29.AC02 |
| R03 | Grid layouts and safe per-widget presentation overrides | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 29.AC06 |
| R04 | Global/local filter definitions and parameter bindings | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 29.AC06, 28.AC03 |
| R05 | Frozen block, dynamic query and safe text widget kinds | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 29.AC04, 29.AC06 |
| R06 | Replayable dynamic question versus session-bound query reference | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 29.AC04 |
| R07 | Dynamic widgets disabled by default, explicit report/schedule opt-in | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 29.AC04, 30.AC01 |
| R08 | Strict versus explicit partial report failure | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 29.AC05 |
| R09 | Exact report revision, widget provenance, block run references and usage | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 28.AC08, 29.AC03 |
| R10 | Immutable report artifacts, metadata summaries and cursor pagination | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 28.AC07, 28.AC08 |
| R11 | Cache-aware materialization joins an in-flight equivalent run | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 28.AC05, 28.AC06 |
| R12 | Private draft/review previews remain private after future publication | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 29.AC02 |
| R13 | Versioned dashboards of exact report revision pages | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 29.AC01, 29.AC08 |
| R14 | Source-shaped external import, external identifiers and revision sequencing | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 29.AC07, 34.AC01 |
| R15 | Older section-based reports project to the canonical layout | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 29.AC07 |
| R16 | Localized names/descriptions, intended business audience label | pending29/28/34 | Mapped plans retain this obligation; this audit does not claim completed target runtime or exhaustively verify the original implementation for this row | 29.AC01, 04.AC03 |
| Q01 | Saved-question schedules and reviewed-SQL schedules are distinct | pending30 over shared06 foundation | Target/window/delivery behavior must be compared when the actual reporting handlers land | 30.AC01 |
| Q02 | Direct block schedules pin block revision and selected outputs | pending30 over shared06 foundation | Target/window/delivery behavior must be compared when the actual reporting handlers land | 30.AC01 |
| Q03 | Report schedules pin by default; latest-published is explicit | pending30 over shared06 foundation | Target/window/delivery behavior must be compared when the actual reporting handlers land | 30.AC04 |
| Q04 | Cron, interval, timezone and prior-occurrence window | pending30 over shared06 foundation | Target/window/delivery behavior must be compared when the actual reporting handlers land | 30.AC03 |
| Q05 | Pause/resume/retire, test runs and run history | pending30 over shared06 foundation | Target/window/delivery behavior must be compared when the actual reporting handlers land | 30.AC04 |
| Q06 | Retry, backoff, timeout and final-failure policy | pending30 over shared06 foundation | Target/window/delivery behavior must be compared when the actual reporting handlers land | 30.AC05 |
| Q07 | Current service-account status and topic grants | pending30 over shared06 foundation | Target/window/delivery behavior must be compared when the actual reporting handlers land | 30.AC02 |
| Q08 | Stored row cap also clamps to current deployment ceiling | pending30 over shared06 foundation | Target/window/delivery behavior must be compared when the actual reporting handlers land | 10.AC04, 30.AC05 |
| Q09 | Catalog delivery, partial-delivery policy and recipient metadata | pending30 over shared06 foundation | Target/window/delivery behavior must be compared when the actual reporting handlers land | 30.AC06 |
| Q10 | Evidence/report-artifact/output retention cleanup | shared foundation; full cleanup pending | Bounded retention target exists in06; new reporting payload/rendition cleanup28/30 remains | 06.AC03, 28.AC07 |
| Q11 | Event and condition trigger enum variants | intentional exclusion | Original event/condition stubs; do not implement to satisfy an enum inventory | 06.AC03, 30.AC08 |
| N01 | Routing and span/entity extraction | gap | RTE-01/02; caller topic selection and missing interpretation stages | 17.AC01, 17.AC02 |
| N02 | Lean context engineering | gap | CTX-01; budget mechanics retained, dependency closure lost | 17.AC03, 17.AC04 |
| N03 | Semantic retrieval and batching | partial | Authorized batches retained; richer selection inputs absent SEM-01/RTE-02; cache PERF-01 | 07.AC03, 17.AC02 |
| N04 | Template precedence and lifecycle | gap | LRN-01/02; lane precedence is not template lifecycle or calibrated selection | 18.AC02, 18.AC05 |
| N05 | SQL validation and bounded correction | retained/reworked with explicit constraints | Opaque plans and bounded corrections; engine/dialect/live matrix still requires per-behavior comparison | 09.AC02, 18.AC03 |
| N06 | Clarification, underspecification and follow-up refinement | partial; gap | Prior SQL/session safety retained; CLR-01/02 typed/triggered clarification incomplete | 16.AC03, 18.AC01, 18.AC06 |
| N07 | Multi-topic relationships / queries | narrowed | Confirmed same-source one-to-one only; other cardinalities need evidence and explicit safe disposition | 17.AC05, 18.AC01 |
| N08 | Business rules, replay/shadow comparisons and feedback | partial; gap | RUL-01/BLK-02; deterministic reference-rule replay exists, broader SQL/result replay pending24 | 16.AC05, 16.AC06 |
| N09 | Learned examples, positive feedback and evaluation/optimization | partial; gap and pending24 | LRN-01/02 and EVAL-01; feedback persistence exists, calibrated weighting/optimization incomplete | 18.AC05, 24.AC04 |
| N10 | Topic generation and entity editing | partial; gap | SEM-01/02; strong lifecycle, narrower rich generation/model | 15.AC02, 15.AC03, 15.AC05 |
| N11 | Source health, table rename/reference rewrite and source recheck | retained core; broader authoring pending | Safe source recheck/rename and immutable amendments; richer relationship semantics DATA-02 | 15.AC03, 15.AC04 |
| N12 | Sharing, access groups, portability and onboarding profiles | intentional ownership change; full bundle pending | the authority provider owns authority; safe semantic portability exists; MIG-01 broader bundle34 | 15.AC06, 04.AC01, 34.AC03 |
| N13 | Upload/dataset mode, SQL workspaces and preprocessing | retained foundation; richer semantic handoff gap | Bounded uploads/ordinary governed sources; DATA-01/02 semantic environment is narrower | 11.AC01, 11.AC05 |
| N14 | Stage timing, cache attribution, cost and cancellation | partial / unmeasured | Cancellation and stage receipts retained; PERF-01 warm/cold cost/latency needs comparison | 01.AC03, 17.AC06, 05.AC03 |
| N15 | Source adapters and dialect coverage | per-engine qualification incomplete | Six target engine contracts; recorded cloud evidence is not live workload equivalence | 14.AC01, 14.AC05 |
| N16 | Operational setup and generated semantic environment | pending26/33 | Evidence-backed richer setup and generation; DATA-01/02, SEM-02 | 33.AC01, 33.AC03 |

## Review disposition

The canonical document incorporates the 26 finding records from the audit, the full 63-feature ledger, the 12 explicit expansion frontiers, and the neutral clarification/underspecification and chart-breadth briefs. Source-side evidence is represented only by abstract IDs; current target evidence uses exact relative line links. No files beyond this documentation artifact are changed by this task, no phase registry/status is modified, and no runtime or live parity test is claimed.
