# Phase 33 — guided-onboarding

Status: planned. Owner: internal/engineering, internal/semantics, internal/reporting. Hard dependencies: 11, 12, 13, 15, 27.

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

## Glossary, decisions and deviations

Setup progress and semantic confidence are evidence, not permission or certification. D-052 applies. No runtime completion is claimed.
