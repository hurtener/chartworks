# Phase 33 — guided-onboarding

Status: in_progress. Owner: internal/onboarding, internal/engineering, internal/semantics, internal/reporting. Hard dependencies: 11, 12, 13, 15, 27.

## Authority and design

RFC-001 §7/8, RFC-002 §9, D-052 and [COMMON.md](COMMON.md) apply. Guided setup is a resumable composition of existing domain operations. It does not create credentials, infrastructure privileges or approved business meaning autonomously.

## Brief findings incorporated

Briefs 05, 11, 14: stable semantic entities, bounded generation, demand-driven reuse, setup assistance and evidence-bearing unresolved questions. A complete secondary-source provisioning flow was not proven; this phase makes the new workflow an explicit implementation commitment.

## Findings I'm departing from

Do not force a medallion build before querying usable data, silently fill missing semantic entities with approved defaults, or treat automatic setup as automatic certification.

## Scope and implementation tasks

1. Implement resumable setup operations separating deployment/connectivity/permission checks from source profiling and semantic inference.
2. Generate evidence-backed semantic drafts, unresolved questions and safe example/draft-block suggestions using stable IDs and bounded stages.
3. Offer optional managed transformations only when required; approval/publication remains explicit and calls existing domain services.

Persist stage inputs/checksums, completed object references, unresolved choices and next required action. Each stage resumes by reference/idempotency rather than recreating objects. Apply selective source inspection, low-cardinality/value summaries and schema/join evidence. Prompts contain only sanitized allowed context; credentials and authority claims are never inferred from data. Public API progress/cancel/resume/answer operations serve human UIs and coding agents equally.

## Non-goals

No cloud provisioner, builder UI, local admin bootstrap, credential generation, arbitrary architecture planner or semantic self-approval.

## Config and persistence

Onboarding stage/call/token/time/entity ceilings, supported locales and optional pipeline assistance; no automatic cloud provisioner or local admin bootstrap. Reuse shared operations and ordinary source/profile/topic/block stores, with a bounded stage ledger rather than another workflow engine.

## Acceptance criteria

1. **AC01** — Connect/upload -> inspect -> profile -> semantic draft -> review/publish -> examples/block proposals works through the API without a builder UI.
2. **AC02** — Each failed/cancelled stage resumes without duplicate sources/workspaces/topics/blocks and reports exact stage/required action.
3. **AC03** — Join cardinality/grain, measures/dimensions/KPIs, units/currency, time/null semantics and sensitivities carry evidence/uncertainty; omitted model entities remain unresolved.
4. **AC04** — No model invents credentials, infrastructure permissions or new data authority; calls operate only within Pengui-signed source scope.
5. **AC05** — Human semantic publication and block certification remain separate; auto-setup never auto-certifies generated business meaning.
6. **AC06** — Already usable sources bypass optional materialization; needed transformations pass the existing managed-write/quality gates.
7. **AC07** — Source drift proposes affected-only changes using stable identities; active semantics/approved queries do not mutate in place.
8. **AC08** — Budgets, English/Spanish context, progress, cancellation and complete/private onboarding artifacts are tested with real source fixtures.

## Tests, coverage and smoke

Implement `TestPhase33/AC01` through `TestPhase33/AC08` with real source/workspace/semantic/block services, bounded model fixtures and failure injection at every stage. Include a directly queryable source and one requiring managed transformation; prove no duplicate objects on resume. COMMON.md sets coverage; `scripts/smoke/phase-33.sh` requires all eight results.

## Implementation submission

The submitted runtime adds a tenant/actor/session-private PostgreSQL run ledger,
one-stage resume and CAS recovery, deterministic domain operation keys, bounded
evidence and unresolved questions, independent semantic review/publication,
private proposal references, durable affected-only drift amendments, cancellation,
English/Spanish status, and HTTP/MCP/Go SDK consumers. The executable operation
manifest is `docs/contracts/chartworks-onboarding-operations.json`; D-083 fixes the
ownership and recovery contract.

Every external stage is preceded by a durable lease/fence CAS. Topic publication
also persists a non-refundable bounded gateway reservation and an uncertain receipt
before inference. A retry with an outstanding publication lease performs immutable
version reconciliation only; it never repeats embedding work blindly. Cancellation
while a lease is live records intent and becomes final only after the same operation
is reconciled. Answer and cancellation races are covered with real concurrent CAS
tests.

Question answers and cancellation reasons are closed semantic enums with optional
identifier references; arbitrary prompt, SQL, credential or row-like strings are
not accepted or returned by progress reads. Profiling is deterministic in this
workflow, while topic publication receives the run's bounded gateway allowance.
The applied-transformation branch verifies the proposal digest, original exact
source/context/revision and published managed output, then profiles that output.
An unrelated applied proposal cannot satisfy the gate.

Drift input contains only run identity and CAS version. The server reads current
source discovery, compares retained per-column schema digests, rejects unchanged or
missing datasets, and returns only affected dependencies bound to the exact current
source/context/revision. Query/block/report outputs are explicitly run-owned intent
coordinates; callers must enter the ordinary authoring/review lifecycle to create
definitions. They are not resolvable as block or report objects and confer no
execution or certification authority.

CW-10/D-082 is consumed at the reporting handoff: onboarding retains only a
content-free private report proposal reference. Actual report authoring and viewer
option reads use the existing exact block/revision/dimension-bound filter-option
service, so the coordinator cannot copy stale values or bypass sensitivity policy.

`TestPhase33/AC01` through `AC08` cover the public journey, failure recovery,
authority negatives, evidence, distinct review gates, transformation choice,
drift immutability, budgets, locale and concurrent CAS. Shipped status remains
pending independent review and the required real-boundary release evidence.
`TestPhase33/AC01` composes `NewDomains` with the real PostgreSQL
source, profile, draft, publication and recorded gateway boundaries through the
public SDK and verifies the MCP binding set; the injected adapter remains
only for deterministic stage failure and race injection.
`TestPhase33/AC10` adds a real PostgreSQL pre-effect lease/cancellation race beyond
the eight planned acceptance rows.

## Glossary, decisions and deviations

Setup progress and semantic confidence are evidence, not permission or certification. D-052, D-082 and D-083 apply. Runtime submission is not a shipped-status claim.

## CW-04 prerequisite delivered

The semantic authoring substrate required by AC03 is now present: reviewed roles,
grains, aliases, units, governed values, KPI formulas, confirmed join evidence and
candidate/rejected relationship decisions persist through the normal draft,
publication and portability lifecycle. The bounded enhancement role has a first real
consumer for rich column/KPI/relationship proposals. This does not mark phase 33
shipped: its resumable connect-to-publish composition, progress/answer operations,
managed-transformation branch and full real-source acceptance remain outstanding.

## CW-07 downstream consumer

Published reviewed governed values, aliases, explicit categorical geography
designations, temporal policies and relationship
evidence now have a first query-time consumer. Routing never promotes onboarding
proposals or unresolved fields: only the exact active publication participates, and
candidate/rejected relationships remain non-executable. The broader resumable phase
33 workflow remains outstanding.
