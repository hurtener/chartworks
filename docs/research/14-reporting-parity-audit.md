# Brief 14 — Reporting parity and current-source audit

Status: reviewed-source analysis, 2026-09-04; design recommendations remain proposed in RFC-002.

## 1. Method and provenance

The primary source is the functional authority when the two implementations diverge. The secondary source supplies useful semantic-generation and deployment ideas, not automatic authority to replace primary behavior. Names, private repository links, source code, confidential schemas, and examples are intentionally not reproduced.

Observed primary commit: `6f44a56223dd4e6995e9ddf656bcde654d47e2db`. Observed secondary main commit: `311a0fd0e043589d0c362d758e4394eb82a59008`. The source owners map these fingerprints to authorized local checkouts; no checkout is distributed with this brief.

The review inspected current source models, selected execution and scheduling implementations, integration documents, source trees, selected tests, ecosystem JWT validators/minter contracts, and the accessible Chartworks planning history. It did not execute the test suites, inspect every line of every pipeline, or prove deployment parity. Older research is retained as historical context and must not be mistaken for a fresh diff of the current linked source trees.

Evidence labels: CODE = inspected source/range; DOC = inspected documentation; TEST = inspected test definition only; INVENTORY = discovered surface without complete behavioral verification; STUB = inspected incomplete implementation; NEW = proposed extension.

### Evidence register

| ID | Neutral source location / fingerprint | Coverage |
|---|---|---|
| P01 | Primary `src/<pkg>/domain/reporting.py`, blob `980581396c2499f341621ee5f118d568af084e15` | CODE, selected ranges through line 1775: blocks, outputs, parameters, trust, reports, widgets, dashboards, adapters, runs/artifacts. |
| P02 | Primary reporting-block agent integration guide | DOC: discovery, execution, SQL visibility, pagination, retained output behavior. |
| P03 | Primary reporting authoring integration guide | DOC: author/validate/publish/certify, preview privacy, report lifecycle, scheduling and catalog delivery. |
| P04 | Primary `services/reporting_execution.py`, blob `97d2a34f5ab055d0523a9a3e76dc680e41e8b981` | CODE, lines 1–290: frozen-path stage prohibition, result typing, idempotent output-expiry behavior. |
| P05 | Primary `scheduling/contracts.py`, blob `caa885db33391691a29aaf9be88a7b6df59c83b7` | CODE, lines 1–245: triggers, targets, failure policy, budgets, revision policies, catalog delivery. |
| P06 | Primary condition/event trigger modules | STUB: both explicitly return no next occurrence. Enum presence is not implemented triggering. |
| P07 | Primary `scheduling/targets/custom.py`, blob `1fdff3a8973254bb089ef7bf8c1df5b02acb1687` | CODE: only bounded evidence/artifact/output-retention cleanup is supported; unknown jobs fail. |
| P08 | Primary `scheduling/targets/saved_query.py`, blob `c410a5d2ba72629151be389fd8991e519e528575` | CODE, lines 1–205: current service-account/topic authorization, row-cap clamp, prior occurrence/window parameters, saved chart recipe handling. |
| P09 | Primary `services/reporting_impact.py`, blob `551dc589775de666545a8f37f3b75e8c6f82d684` | CODE: exact rename identification, semantic/schema change classification, removed dependency failure. |
| P10 | Primary `services/nlq_pipeline.py`, blob `9bb7ddfc059dee68dc711156da2fb9add77bbb8e` | CODE, lines 1–220: stage composition, previous context/SQL, explicit pinned metrics, rule-evaluation controls, timings and signals. |
| P11 | Primary README and API/service/presentation trees | DOC / INVENTORY: NLQ, datasets, templates, rules, feedback/evaluation, topic lifecycle, output catalog and report APIs. |
| P12 | Primary `tests/services/test_reporting_governance.py`, blob `9766d3383cd12e93618a4f5fd4375022519583e5` | TEST: CTE allowance, qualified-table escape denial, bounded rule snapshots, unknown-topic nondisclosure. Not run here. |
| P13 | Primary reporting service test tree | INVENTORY: separate tests for blocks, execution, dashboards, reports/hybrid widgets, narratives, parameters, period language, outputs, revalidation and external adapters. Presence alone does not establish pass status. |
| S01 | Secondary `enhancers/semantic.py`, blob `fd91a4e69004f7dac87a197539e46c616d94d0fb` | CODE, lines 1–215: stable measure/dimension entities, bounded batches/retries/concurrency, normalized semantic context, omitted-entity fallback warning. |
| S02 | Secondary vector-search implementation summary | DOC: batched retrieval and source tracking; recorded POC timings are not reproduced production measurements. |
| C01 | Chartworks main `8de9641ddba33bd86d4aeb2a080a8a4fedddf01d` | RFC-001, contributor rules, research index/diff brief, PR history and selected amendments. |
| C02 | Planning branch `5992dad9f1ebfd8b093c8406e78c665936e0d93d` | Branch comparison and D-043 comment; provider/rerank follow-up is not a reporting implementation. |
| E01 | Soundings `internal/auth/auth.go` and `validator.go` | CODE: shared HTTP/MCP validation, issuer modes, asymmetric allowlist, surface audiences, fail-closed JWKS and bounded authority sets. |
| E02 | Stowage `internal/auth/validator.go`, blob `6a55422276b1f872564c67bebfaca973a936c513` | CODE, selected range: verification-only JWT path, asymmetric algorithms, expiration/not-before and identity triple. |
| E03 | Pengui `internal/minter/minter.go`, blob `244fa1bd267e3cea74d8b0f0f22498741d3b25f2` | CODE, lines 1–385: signed tenant/user/session projection, service subjects, provider-scope seam, audience binding, distinct knowledge authorities. |

## 2. Reporting capabilities that must not disappear

The following rows are requirements to close, not assertions that Chartworks already implements them. Detailed HTTP paths and test gates are in the reporting contract and implementation plan.

| ID | Capability / invariant | Evidence | Target disposition |
|---|---|---|---|
| B01 | Stable block identity, localized name/question, canonical question and aliases | P01/P02 CODE+DOC | Preserve in block catalog; aliases improve discovery but never choose a new query during frozen execution. |
| B02 | Mutable draft versus immutable published revision | P01/P03 CODE+DOC | Preserve with compare-and-swap and new-draft amendments. |
| B03 | Publication, certification and current trust/health are separate | P01 CODE | Preserve separate state and timestamps; no single misleading trusted boolean. |
| B04 | Exact topic/template/dependency references and definition hashes | P01 CODE | Pin semantic execution meaning and make impact analysis queryable. |
| B05 | Real validation evidence bound to content and observed schema | P01/P03 CODE+DOC | Publication/certification must reject stale evidence after an edit. |
| B06 | Read metadata without automatically exposing SQL | P02/P03 DOC | Dedicated SQL-read operation and scope. |
| B07 | One saved query can feed chart, KPI, table and narrative outputs | P01/P04 CODE | One logical query attempt; selected outputs fan out from the same normalized result. |
| B08 | Enabled/default output selection, output identifiers and mappings | P01/P02 CODE+DOC | Stable identifiers and validated subset selection; reject unknown outputs. |
| B09 | Date/datetime/relative-period/dimension/number/integer/boolean/grain/top-N parameters | P01 CODE | Typed and bounded binding; no SQL text substitution. |
| B10 | Locale, report timezone and explicit parameter provenance | P01/P03 CODE+DOC | Retain in manifest and output formatting. |
| B11 | Period authoring: explicit range, previous period, rolling periods, schedule window | P03 DOC; P08 CODE | Preserve behavior through canonical period selectors and migration goldens. |
| B12 | Assisted parameterization and question-duplicate assessment | P03 DOC; service inventory | Preserve authoring workflows; suggestions cannot mutate published SQL. |
| B13 | Exact revision and latest-published/latest-certified selection | P01 CODE | Resolve once; a pin never silently follows a newer revision. |
| B14 | Published/certified-only/explicit-stale/private-preview trust policies | P01 CODE | Explicit policy and current authority; stale permission is never an allowed trust fallback. |
| B15 | Expected ordered columns, types, nullability and sensitive-field metadata | P01/P04 CODE | Fail visibly on incompatible result shape; no guessed rebinding. |
| B16 | Execution traces prove absence of interpret/generate/rewrite/select-chart stages | P01/P04 CODE | Instrumented negative assertions in Go tests. |
| B17 | Bounded narrative generation, approved columns, evidence, tone/locale and budgets | P01 CODE; narrative service inventory | Optional post-query model call with grounded claims and preserved output snapshot. |
| B18 | Schema/semantic impact, exact rename detection and dependent health | P09 CODE | Classify; propose new draft; do not auto-edit a published revision. |
| B19 | Revalidation / withdrawn approval / unavailable source | P01/P03/P09 | Current state visible to readers and schedules; historical evidence retained. |
| B20 | Idempotency and expired retained block outputs | P04 CODE | Same operation key never silently reruns an expired payload. |
| R01 | Reports compose multiple approved blocks and outputs | P01 CODE | First-class report identity and immutable revisions. |
| R02 | Draft, pending review, publication, rejection and amendment lifecycle | P01/P03 CODE+DOC | Explicit review operations; private work never leaks into public reads. |
| R03 | Grid layouts and safe per-widget presentation overrides | P01 CODE | Canonical bounded layout; do not impose a source-adapter-only fixed grid on all reports. |
| R04 | Global/local filter definitions and parameter bindings | P01 CODE | Validate references, types, precedence, and authorization. |
| R05 | Frozen block, dynamic query and safe text widget kinds | P01 CODE | Discriminated union; preserve all three without trust conflation. |
| R06 | Replayable dynamic question versus session-bound query reference | P01 CODE | Different execution requirements and scheduling eligibility. |
| R07 | Dynamic widgets disabled by default, explicit report/schedule opt-in | P01/P05 CODE | Preserve; report publication is not dynamic-query certification. |
| R08 | Strict versus explicit partial report failure | P01 CODE | Missing widgets, failed outputs and mixed freshness cannot masquerade as a complete success. |
| R09 | Exact report revision, widget provenance, block run references and usage | P01/P59-range CODE | Persist in a resolved run manifest and public evidence projection. |
| R10 | Immutable report artifacts, metadata summaries and cursor pagination | P01 CODE | Retrieval independent of execution; authorize every page. |
| R11 | Cache-aware materialization joins an in-flight equivalent run | P01 CODE | Preserve operation semantics with policy-aware identity and fenced ownership. |
| R12 | Private draft/review previews remain private after future publication | P01 CODE | Persist artifact privacy; never derive it solely from the report's current pointer. |
| R13 | Versioned dashboards of exact report revision pages | P01 CODE | Lightweight composition; preserve page order, publication and redacted-page behavior. |
| R14 | Source-shaped external import, external identifiers and revision sequencing | P01/P03 CODE+DOC | Neutral offline adapter, explicit lifecycle mapping, idempotency and quarantine. |
| R15 | Older section-based reports project to the canonical layout | P01 CODE | Preserve read projection without rewriting historical revisions. |
| R16 | Localized names/descriptions, intended business audience label | P01 CODE | Preserve metadata; a free-text audience label is not an access-control grant. |
| Q01 | Saved-question schedules and reviewed-SQL schedules are distinct | P05/P08 CODE; P03 DOC | Preserve explicit dynamic versus frozen behavior. |
| Q02 | Direct block schedules pin block revision and selected outputs | P05 CODE | No hidden report; check current eligibility before each occurrence. |
| Q03 | Report schedules pin by default; latest-published is explicit | P05 CODE | Resolve once at occurrence start; no live repinning during retries. |
| Q04 | Cron, interval, timezone and prior-occurrence window | P05/P08 CODE | Preserve with defined missed-run/DST behavior and temporal tests. |
| Q05 | Pause/resume/retire, test runs and run history | P03 DOC; scheduling surface inventory | Preserve through API and durable status. |
| Q06 | Retry, backoff, timeout and final-failure policy | P05 CODE | Preserve; recheck permission and budget on every attempt. |
| Q07 | Current service-account status and topic grants | P08 CODE | Preserve revocation checks and prohibit authority amplification during schedule creation. |
| Q08 | Stored row cap also clamps to current deployment ceiling | P08 CODE | Preserve; interactive and unattended paths cannot diverge on safety limits. |
| Q09 | Catalog delivery, partial-delivery policy and recipient metadata | P03/P05 CODE+DOC | Preserve as pull delivery; do not claim outbound email. |
| Q10 | Evidence/report-artifact/output retention cleanup | P07 CODE | Preserve in existing maintenance scheduling; not arbitrary executable custom jobs. |
| Q11 | Event and condition trigger enum variants | P05 CODE; P06 STUB | Record unimplemented source debt; build only under a separately accepted extension. |

`P59-range` in R09 refers to the run/artifact portion of P01 (lines 1531–1775), not a separate source.

## 3. NLQ, semantics, and pipeline continuity

The report extension must not shrink Chartworks into a frozen-SQL library. The following broader inventory needs a maintained behavior-to-test map before migration cutover.

| ID | Area | Current evidence / required disposition |
|---|---|---|
| N01 | Routing and span/entity extraction | P10 CODE and P11 DOC. Preserve authorized topic filtering, language, confidence/ambiguity/assumption signals; unknown context does not expand access. |
| N02 | Lean context engineering | Existing brief 03/05 plus P10. Rich semantic packs project into bounded cards; selected/pinned metrics must survive trimming, and insufficient budget must be explicit. |
| N03 | Semantic retrieval and batching | S01/S02 plus existing briefs. Preserve batch/source attribution and policy filters; recorded POC speedups are not a promised Go performance result. |
| N04 | Template precedence and lifecycle | P10/P11 and historic brief 05. Preserve exact/adapted/free-generation decisions, candidate/active/deprecated lifecycle, validation and draft amendments. |
| N05 | SQL validation and bounded correction | Existing D-038 and P12 TEST. Preserve native dialects, relation scope and rule constraints; adapters need a fresh adversarial corpus. |
| N06 | Clarification, underspecification and follow-up refinement | P10 CODE fields and P11 surface inventory. Preserve prior context/SQL, delta instructions, explicit metric choices and rejection of foreign sessions. |
| N07 | Multi-topic relationships / queries | P11 inventory and historical briefs. Inspect deployed behavior and capture goldens before assigning any part to a later investigation wave. No blanket parity claim here. |
| N08 | Business rules, replay/shadow comparisons and feedback | P10 CODE controls, P11 inventory. Preserve provenance and review; feedback is not automatic production semantic publication. |
| N09 | Learned examples, positive feedback and evaluation/optimization | P11 inventory and existing brief 03. Inventory supported workflows and choose Go equivalents; neither carrying a Python optimizer nor deleting the capability is assumed. |
| N10 | Topic generation and entity editing | S01 CODE, P11 and historical brief 05. Preserve measures/dimensions/KPIs/joins, business context, draft/review/promotion, rollback/diff and stable IDs. |
| N11 | Source health, table rename/reference rewrite and source recheck | Historical brief 05, P09 report impact. Keep hardened primary behavior; freshly verify each underlying lifecycle operation. |
| N12 | Sharing, access groups, portability and onboarding profiles | P11 route inventory / historical brief 05. Preserve product outcomes through current signed authority, not old headers or local password flows. |
| N13 | Upload/dataset mode, SQL workspaces and preprocessing | P11 inventory and RFC-001. Dataset-mode querying must use the same safety, policy, result and report contracts. |
| N14 | Stage timing, cache attribution, cost and cancellation | P10 CODE and source docs. Keep per-stage observability with bounded labels and content-free default logs. |
| N15 | Source adapters and dialect coverage | RFC-001 and source inventory. Release claims are per tested engine; a PostgreSQL proof does not close production-warehouse parity. |
| N16 | Operational setup and generated semantic environment | S01 verifies parts of semantic generation. A complete automatic environment-provisioning flow was NOT verified in the linked secondary main. Treat the proposed resumable setup flow as a new implementation commitment. |

## 4. Visualization inventory

The primary presentation tree contains generator/suitability entries for area, bar, column, donut, grouped bar, heatmap, KPI card, line, pie, scatter, stacked bar, stacked column, table, and treemap. This is an INVENTORY of fourteen entries, not a claim that every renderer was exercised.

Carry the supported kind/binding/format contracts into a synthetic catalog fixture suite. Verify axis labels, aggregation, ordering, missing values, decimal labels, small/large categories, legend behavior, negative values, stacking, and accessibility. A table fallback may keep an unsupported client useful, but it does not count as full visual parity for a required chart type.

## 5. Source behaviors to improve rather than copy blindly

The inspected result-normalization path converts Decimal-like values toward floating point. The Go contract should preserve exact decimals and large integers in transport and labels. Some older external-record contracts default omitted lifecycle to published; the neutral import must require an explicit lifecycle mapping, defaulting new records to private draft. Historic request models include convenient tenant defaults and source-specific identity conventions; authority must instead come only from verified envelopes.

The source's frozen-path and schema checks are valuable, but one warehouse call in a successful source test is not proof of physical exactly-once execution under crash recovery. Post-execution budget comparison is also insufficient as a spend-control guarantee. Retain the outcomes and strengthen the boundaries.

## 6. New delivery requirements

MCP Apps, secure iframe delivery, genuine chart SSR, cross-surface renderer conformance, and explicit artifact-policy partitions are new or strengthened requirements in this proposal. They must be tested, not inferred from a chart-spec generator or ordinary MCP tool registration.

## 7. Closure rule

For every row, the implementation ledger records target contract, source fixture reference held privately, newly authored synthetic regression, adapter coverage, runtime evidence, and migration disposition. Valid terminal dispositions are implemented-and-proven, deliberately superseded with equivalent behavior and an approved migration rule, or explicitly excluded by the product owner. An unverified inventory row is not silently converted to complete.
