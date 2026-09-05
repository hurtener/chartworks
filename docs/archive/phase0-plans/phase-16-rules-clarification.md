# Phase 16 — rules-clarification (Wave 5)

> **Status:** draft
> **Owner:** orchestrator
> **Depends on:** phase-15-topics-lifecycle

Governed business rules and proactive clarification — the client predecessor's
largest capability lead over the fork (briefs 03/05) — shipped V1-scoped per D-027:
scoped/categorized/prioritized/structurally-validated rules with a
`proposed → active → retired` lifecycle, an independently-budgeted injection lane
whose dropped rules and contradictions are named output fields, sensitive-literal
enforcement, and topic-scoped clarification patterns that run before generation and
produce named clarification slots.

---

## RFC / request sections

- **RFC-001-Chartworks.md §8.4** — Governed rules & clarification (V1-scoped; D-027) — the
  primary contract this phase implements.
- **RFC-001-Chartworks.md §8.3** — Context engineering: the governed-rules lane gets its
  *own independent token budget*; dropped rules and contradictions are named output fields,
  never silent; one tokenizer-backed budget currency.
- **RFC-001-Chartworks.md §2** — Vocabulary & the P6 forbidden-word list (the `revalidate` /
  `re-check source` renaming of the predecessors' `repair`).
- **RFC-001-Chartworks.md §12** — the budgeted `rules` table (this phase's only owned table).
- **RFC-001-Chartworks.md §13** — the `clarify` and `enhance` gateway roles (P5).
- **RFC-001-Chartworks.md §14** — config: the `nlq.rules_lane_budget` key (default 300,
  tokenizer-backed).
- **RFC-001-Chartworks.md §7.3** — the per-tenant **canonical identity registry** (owned by
  phase 13, stored in the semantic layer, consumed read-only here).

Decisions: **D-027** (governed rules + clarification are V1 scope; shadow eval + replay
deferred), **D-020** (the one access resolver / grants — tenant + topic scope, `manage`
authority), **D-003** (the one intelligence seam; the `clarify` role is schema-constrained),
**D-025** (the one generic job queue — clarification-pattern generation rides the topic
enhancement job).

## Depends on

- **phase-15-topics-lifecycle** (hard dependency): the topic pack model + versioned mutation
  path (clarification patterns are authored/edited through it), the topic-enhancement/
  generation job (the `clarify`-role pattern generation hooks into it), and the resolver-
  backed transition-authority matrix. Phase 16 extends the pack and the enhancement job; it
  does not re-open the lifecycle.

Consumed interfaces that ship in earlier waves (available, not re-implemented here):

- **phase-02-store-migrations** — the budgeted `rules` table and `audit_events`.
- **phase-04-access-grants** — the one resolver: rule authoring/lifecycle authority needs
  topic `manage`; every rule/pattern read takes non-optional `(tenant, …)` scope.
- **phase-05-gateway** — the `clarify` role (schema-constrained pattern generation) + the
  `mock` driver (the sanctioned boundary mock) + a recorded-fixture test.
- **phase-13-engineering-pipelines** — the **canonical identity registry** read-only lookup
  (entity references in rule scopes / clarification patterns resolve through it).

Phase 16 is itself consumed by **phase-17** (the `ContextAssembler` places the rules lane it
produces) and **phase-18** (`preflight`/`plan` surface the clarification slots it produces).
Those seams are opened here and closed there; see Non-goals.

## Informing briefs

- `docs/research/03-predecessor-nlq-pipeline.md` — **§2.6** (business-rule injection: a
  parallel, independently-budgeted lane) and **§7** (proactive, topic-scoped
  underspecification detection before generation). This is the design backbone.
- `docs/research/05-predecessor-diff.md` — the rule-lifecycle divergence evidence: the
  client-only `rules/` subsystem is the "headline divergence"; the fork lost it entirely.
  Also the transition-audit and resolver-authority discipline carried from §8.2.

## Brief findings incorporated

- **The independently-budgeted lane (brief 03 §2.6).** Rules inject their *own* block into
  the structured context with a token budget decoupled from §8.3's complexity-tier evidence
  budgets, so governance never crowds out semantic evidence or vice versa. Carried as the
  `nlq.rules_lane_budget` currency, measured by the *tokenizer-backed* estimator (never
  char/4 — the brief's estimator was char-count/4; we fix that scar per §8.3).
- **Structured, scoped, validated — not free text (brief 03 §2.6).** Rule categories,
  targets/scope, and inline wording are structurally validated *before* injection, never
  passed as free text. Carried as a mandatory validation gate producing typed errors.
- **Precedence ordering (brief 03 §2.6).** Selection orders by category
  (`computation < semantic < structural`), then scope specificity
  (`tenant < topic < measure|dimension`, most-specific wins), then priority, then recency —
  and trims to budget. Carried as one deterministic, unit-tested comparator.
- **Exclusions are visible (brief 03 §2.6).** *Dropped* rules (over budget) and
  *contradictions* (conflicting active rules on the same target) are named output fields,
  never silently discarded (P4).
- **Proactive, topic-scoped clarification before generation (brief 03 §7).** A dedicated
  detector runs topic-specific ambiguity *patterns* (generated at topic-enhancement time via
  the `clarify` role, mergeable with manual patterns, cached per topic) against the NLQ,
  producing explicit named slots ("which region", "which time grain") *before* generation —
  not a post-hoc confidence score. Carried verbatim as the clarification engine.
- **The rule lifecycle is first-class governance (brief 05).** The client-only rules
  subsystem is carried as a governed, independently-lifecycled artifact (`proposed → active →
  retired`) with a typed audit row per transition and resolver-computed authority — the same
  discipline §8.2 pins for topic transitions.

## Findings I'm departing from

- **Shadow evaluation + historical replay (brief 03 §2.6 machinery around rule impact).**
  Deferred to a later wave per **D-027** — the machinery is additive and does not change the
  V1 injection/authoring contract. See Non-goals.
- **The predecessor's `template`/`compound` specificity tiers (brief 03 §2.6).** The
  predecessors keyed specificity on a template/compound/topic-wide axis. Chartworks has no
  template artifact at rule grain; specificity is re-expressed on the settled scope grains
  (`measure|dimension < topic < tenant`), a deliberate simplification, not an oversight.
- **The predecessor's `char-count/4` budget estimator (brief 03 §2.6).** Not carried — the
  rules lane uses the same tokenizer-backed estimator §8.3 mandates for every lane.

## Scope

Delivered in **`internal/semantics`** (the RFC places both governed rules and clarification
under §8, "The semantic model (`internal/semantics`)"; §3.2's coarser table lists
"clarification" under `nlq`, but §8.4 is the more specific placement — both live in
`internal/semantics` so this phase depends only on phase 15, not on the yet-unbuilt `nlq`
routing package). Concretely, two cohesive units (subpackages `rules` and `clarify`):

1. **The rule model + store interaction.** The typed `Rule` (scope, category, priority,
   `definition`, status, provenance) over the budgeted `rules` table (`scope_json`,
   `category`, `definition_json`, `status`, `priority`). Scope grains:
   `tenant | topic | measure | dimension`. Categories: `computation | semantic | structural`.
2. **Structural validation** (before any store write and before any injection): valid
   category, resolvable scope (entity references resolved through the canonical registry),
   well-formed `definition`, and **sensitive-literal marking** — every literal value embedded
   in a rule definition must carry an explicit `sensitive` boolean; an *unmarked* literal is
   rejected with a typed error (fail-loud).
3. **The rule lifecycle** `proposed → active → retired`: legal transitions only, each a typed
   `audit_events` row (actor + decision, content-free); resolver-computed authority
   (topic `manage`); only `active` rules enter selection.
4. **The injection lane producer** — `RulesLane(ctx, scope, budget) → RulesLaneResult`:
   selects active rules in scope, orders by the precedence comparator, trims to the
   independent `nlq.rules_lane_budget` (tokenizer-backed), and returns the injectable block
   plus named `dropped []DroppedRule` and `contradictions []Contradiction` fields, each rule
   carrying provenance. Pure over per-call copies; never mutates the source rule set.
5. **The clarification pattern model** — a typed `ClarificationPattern` (topic-scoped: a
   trigger + the named slot it emits) stored **inside the topic pack** (`topic_versions.
   pack_json`, versioned with the pack — no new table; the schema budget holds), authored two
   ways: generated at enhancement via the `clarify` gateway role (schema-constrained, P5) and
   manually edited through phase 15's versioned mutation path.
6. **The clarification detector** — `Detect(ctx, topicVersion, question) → []ClarificationSlot`:
   runs the pack's patterns against an NLQ *before* generation, producing named slots; cached
   per topic version. The slot shape is the stable contract `preflight`/`plan` (phase 18)
   surface.
7. **Config:** the `nlq.rules_lane_budget` field (§14) materialized in the typed config with a
   fail-loud validator, wired into the example config, smoke-checked.

## Non-goals

- **Shadow evaluation & historical replay** of rule impact — **deferred per D-027** (later
  wave; additive machinery, no V1 contract change).
- **The `ContextAssembler` itself** and the placement of the rules lane into the assembled
  query context — **phase 17**. This phase produces the `RulesLaneResult`; phase 17 consumes it.
- **The `preflight`/`plan`/`refine` services** that surface clarification slots to callers —
  **phase 18**. This phase produces slots against fixtures; phase 18 wires the surfacing.
- **Rule/pattern authoring HTTP + MCP surfaces** — **phases 21/22** (thin callers over this
  core, P7).
- **Semantic (LLM-judged) contradiction detection.** V1 contradiction detection is
  *structural* (same target + conflicting directive markers). An LLM ranker is out of scope.
- **Versioning rules inside the pack.** Rules are first-class, independently-lifecycled rows
  (the `rules` table), *not* pack-versioned — a rule can be retired without republishing the
  topic. Clarification *patterns*, by contrast, are pack content (versioned with the pack).
- **Injecting `proposed` or `retired` rules** — only `active` rules are ever selected.

## Design

**Data flow.**

```
authoring ─▶ StructuralValidate ─▶ store(rules) ─▶ lifecycle(proposed→active→retired, audited)
                     │                                         │
        canonical registry (phase 13, read-only)              only `active`
                                                                   ▼
NLQ scope ─▶ RulesLane(scope, budget) ─▶ select ∩ order(precedence) ∩ trim(tokenizer)
                                          ─▶ RulesLaneResult{ block, dropped, contradictions, provenance }
                                                                   ▼ (consumed by phase 17 ContextAssembler)

enhancement job (phase 15) ─▶ clarify role (schema-constrained) ─▶ patterns in pack_json
NLQ question ─▶ Detect(topicVersion, question) ─▶ []ClarificationSlot  (surfaced by phase 18)
```

**Key types (indicative).**

- `Rule{ RuleID; Scope; Category; Priority; Definition; Status; Provenance }` where
  `Scope ∈ {tenant|topic|measure|dimension, resourceRef}` and `Definition` carries the
  directive text plus a typed list of `Literal{ Value; Sensitive bool }`.
- `RulesLaneResult{ Block []InjectedRule; Dropped []DroppedRule; Contradictions []Contradiction }`
  — dropped/contradictions are *always present* (empty, never absent) so callers branch on
  content, not on key existence (the §8.3 "one stable envelope" keeper).
- `ClarificationPattern{ Trigger; SlotName; Prompt }` (pack content) →
  `ClarificationSlot{ Name; Prompt; Provenance }` (detector output).

**Precedence comparator (one deterministic function, unit-tested):**
`(categoryRank, scopeSpecificityRank, priority, recency)`, higher precedence retained first;
trimming drops the tail once the tokenizer-backed running total would exceed the budget.

**Sensitive-literal enforcement (P4/P6/§7).** Structural validation rejects any rule whose
definition embeds an unmarked literal (typed `rules.literal_unmarked` error). A literal marked
`sensitive` is redacted from every content-free surface — `audit_events`, structured logs, and
scope-debug — and flows *only* into the generation context lane. This is a standing redaction
regression guard, not a convention.

**Canonical identity registry interaction (consume; interface owned by phase 13).** Rule
scopes and clarification patterns reference business entities by name. Phase 16 resolves each
reference through the phase-13 registry's read-only lookup —
`Resolve(ctx, tenant, term) → (CanonicalEntity, ok)` — "resolve then compare, never compare
names" (brief 11). An unresolvable reference is a typed validation error, never a name-string
match. Phase 16 declares only this consumer-side interface; it never writes the registry and
never introduces a second vocabulary (P7).

**How preflight surfaces slots.** The detector is a pure function returning the stable
`[]ClarificationSlot` contract. Phase 18's `preflight` (route + slot detection, no SQL, no
execution) and `plan` call `Detect` and place the slots on their response envelope; phase 16
proves slot *production* against fixtures and freezes the slot shape as a golden.

**Seam / P-property fit.** P5: the `clarify` role is schema-constrained (no free-text JSON
parse of model output). P4: dropped/contradiction/validation outcomes are typed + named,
never silent. P3: every rule/pattern read takes a non-optional tenant scope; a rule never
crosses tenants. P6: `revalidate`/`re-check source` vocabulary only — no `repair`. P7: rules
+ clarification implemented once in the core; surfaces are thin callers.

## Config keys added

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `nlq.rules_lane_budget` | int (tokens) | 300 | no | The governed-rules injection lane's **independent** token budget (§8.3/§14). Tokenizer-backed measurement, decoupled from the complexity-tier evidence budgets. Fail-loud validator: must be > 0. Documented in the plan, the example config, and smoke-checked. |

## Acceptance criteria

1. **Structural validation gate.** A rule with an invalid category, an unresolvable scope
   reference, or a malformed `definition` is rejected with a typed error *before* it can be
   stored or injected — never passed as free text.
2. **Sensitive-literal enforcement.** A rule definition containing an *unmarked* literal is
   rejected with a typed `rules.literal_unmarked` error (fail-loud); a literal marked
   `sensitive` validates and is absent from `audit_events`, structured logs, and scope-debug.
3. **Lifecycle + audit.** `proposed → active → retired` transitions each emit exactly one
   typed `audit_events` row; an illegal transition is a typed error; only `active` rules are
   ever returned by selection (proposed/retired excluded).
4. **Independent rules-lane budget.** The injected rules block never exceeds
   `nlq.rules_lane_budget` under the tokenizer-backed estimator, and trimming the rules lane
   leaves the complexity-tier evidence budget untouched (independence assertion). [token-count]
5. **Deterministic precedence.** Selection orders by `(category, scope specificity, priority,
   recency)`; the same rule set yields the same order every run (golden comparator test).
6. **Dropped rules enumerated.** When selection exceeds budget, every dropped rule appears in
   the named `Dropped` field with a reason; nothing is silently discarded.
7. **Contradictions named.** Two active rules targeting the same measure/dimension with
   conflicting directives surface in the named `Contradictions` field.
8. **Canonical-registry resolution.** An entity reference in a rule scope or clarification
   pattern resolves through the phase-13 registry lookup; an unresolvable reference is a typed
   error (resolve-then-compare, never a raw name match).
9. **Schema-constrained clarification generation.** `clarify`-role pattern generation is
   schema-constrained (typed output; no free-text JSON parse); a malformed generated pattern
   is rejected; patterns persist in `pack_json` and are editable through phase 15's versioned
   mutation path.
10. **Clarification slot production.** Running a topic's patterns against an *ambiguous* NLQ
    fixture produces named slots pre-generation; an *unambiguous* fixture produces none; the
    slot shape matches the frozen golden the phase-18 surfaces consume.
11. **Concurrency + tenant isolation.** The `RulesLane` producer and the clarification
    detector are safe under concurrent reuse (`-race`), operate on per-call copies (never
    mutate the source rule set or pack), and never select a rule from another tenant
    (cross-tenant probe returns nothing).

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven — structural validation (valid/invalid categories, scopes,
  unmarked-literal), the precedence comparator (golden ordering), budget trimming +
  independence, contradiction detection, lifecycle transition legality, slot production
  (ambiguous/unambiguous fixtures).
- **Integration:** required — this phase closes seams phases 17/18 open and consumes phase
  13's registry, phase 05's `clarify` role, and the phase-02 `rules` table. Against a real
  **Docker Postgres** (`make pg-up`): author → validate → store → lifecycle → `RulesLane`
  producing a budgeted block with named dropped/contradiction fields. `clarify`-role
  generation goes through the gateway **`mock`** driver (the one sanctioned boundary mock)
  paired with a **recorded-fixture** test against the real wire format.
- **Adversarial:** a **cross-tenant rule-selection probe** (tenant A's rule never selected for
  tenant B's scope), the **empty-active-set** case (no active rules ⇒ empty lane, not an
  error, no fabricated content), and a **sensitive-literal leakage regression guard** (a
  marked-sensitive literal never appears in an audit row, a log line, or scope-debug output).
- **Fuzz:** `FuzzRuleValidate` over `definition_json` (seed corpus; invariant: never panics,
  never stores an unmarked sensitive literal, never accepts a free-text-only definition) — a
  parse/validate surface per §11.
- **Bench:** `BenchmarkRulesLane` — the lane producer runs per query (a hot reusable
  artifact); baseline only, not a CI gate.

## Coverage targets

Per CLAUDE.md §11 defaults — 80% for the new `internal/semantics` rules/clarification code
(not in the 85% store/vindex/auth/access/exec conformance band; not CLI/tooling).

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/semantics/rules` | 80% | New `internal/` package — default band. |
| `internal/semantics/clarify` | 80% | New `internal/` package — default band. |

(Entries added to `scripts/coverage-bands.conf` in this PR.)

## Smoke checks

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestRuleStructuralValidation` — invalid category/scope/definition rejected typed |
| 2 | `TestSensitiveLiteralEnforced` — unmarked literal rejected; marked literal absent from audit/log/scope-debug |
| 3 | `TestRuleLifecycleAudited` — legal transitions audited, illegal rejected, only active selected |
| 4 | `TestRulesLaneBudgetIndependent` — block ≤ budget (tokenizer); evidence budget untouched |
| 5 | `TestRulePrecedenceDeterministic` — golden ordering by (category, specificity, priority, recency) |
| 6 | `TestDroppedRulesEnumerated` — over-budget drops named in `Dropped`, never silent |
| 7 | `TestContradictionsNamed` — conflicting active rules surfaced in `Contradictions` |
| 8 | `TestCanonicalRegistryResolution` — entity refs resolved; unresolvable ⇒ typed error |
| 9 | `TestClarifyGenerationSchemaConstrained` — typed clarify output; malformed rejected; pack-stored |
| 10 | `TestClarificationSlotsProduced` — ambiguous fixture ⇒ named slots; unambiguous ⇒ none |
| 11 | `TestRulesLaneConcurrentTenantIsolation` — `-race`, per-call copies, cross-tenant probe empty |

## Glossary additions

New terms this phase introduces (landed in `docs/glossary.md` in the same PR). `Governed rule`
and `Clarification slot` already exist — not re-added.

- **Rules lane** — the governed-rules injection lane: `active`, in-scope rules selected by a
  fixed precedence order and trimmed to an *independent* tokenizer-backed token budget
  (`nlq.rules_lane_budget`), decoupled from the evidence budgets so governance never crowds
  out evidence; its dropped rules and contradictions are always-present named output fields
  (RFC §8.3/§8.4).
- **Clarification pattern** — a topic-scoped ambiguity pattern (a trigger + the named slot it
  emits) stored in the topic pack, generated at enhancement via the `clarify` role and
  manually editable; distinct from a **clarification slot**, which is a pattern's runtime
  output (RFC §8.4).
- **Sensitive literal** — a literal value embedded in a governed rule's definition, explicitly
  marked `sensitive`; an *unmarked* literal is rejected at validation (fail-loud), and a
  marked one is redacted from every content-free surface, flowing only into generation context
  (RFC §8.4, §7).

## Decisions filed

No new decision entries. This phase relies on existing decisions:

- **D-027** — governed rules + proactive clarification are V1 scope (scoped down); shadow
  evaluation + historical replay deferred (Non-goals).
- **D-020** — the one access resolver / three-grain grants: rule-lifecycle authority is
  resolver-computed (`topic manage`); every read is tenant-scoped.
- **D-003 / P5** — the `clarify` role generates patterns schema-constrained through the one
  gateway seam.
- **D-025** — clarification-pattern generation rides the one generic job queue (the topic
  enhancement job), no dedicated worker class.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3): each reasonable deviation, why, and
     confirmation this file was updated in the same PR. Empty at authoring time. -->
