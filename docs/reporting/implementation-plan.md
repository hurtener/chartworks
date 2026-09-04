# Reporting implementation and migration execution

The active work is in `docs/plans/phase-*.md` and its dependency/coverage registry. This file is the delivery/cutover guide, not an alternative set of phase specifications. RFC-001/RFC-002 are reconciled; there is no remaining task to decide whether rendering is in scope or whether Chartworks should issue tokens.

## Milestones

| Milestone | Required work | Observable outcome |
|---|---|---|
| Foundation | 01–10 plus early shells 21–23 | Pengui JWT enforcement, real source execution, bounded jobs and immediately consumable interfaces; no local IAM |
| First governed slice | 12,15,20,27,28 | Authenticated source/topic -> draft -> actual validation -> publish/certify -> selected outputs -> retained artifact with no LLM re-entry |
| Complete reporting surfaces | 16–19,29–32 | Hybrid reports/dashboard pages, functional schedules, Apps viewer and real SSR/BFF iframe delivery |
| Onboarding and deeper parity | 11,13,14,24,26,33 | Structured uploads, managed engineering, source adapters, retained NLQ/replay/learning behavior and resumable semantic setup |
| Migration and release | 34 -> 25 | Every required cohort/feature/driver proven, schedule handoff/rollback rehearsed and full operational release evidence |

Phase numbers are stable identifiers, not a requirement to implement numerically. The dependency graph is checked for cycles. Work can start in parallel where the graph permits; no final phase is marked shipped until all its acceptance criteria pass. A subset demo is labeled a demo, not migration parity.

## Development procedure

Read the owning phase and its contracts/briefs. Implement its first concrete consumer through the early HTTP/MCP/SDK registration shells. Add domain migrations with that consumer. Implement the listed `TestPhaseNN/ACxx` acceptance cases and their real source/store boundary tests; update supported capabilities and configuration reference. Keep all prior shipped phase tests green. Update coverage/evidence for every source feature touched.

Use one queue, one gateway, one signed-scope enforcement path and one reporting domain. No local issuer/role/grant/admin service, no host compatibility project, no separate embed auth service, no generic workflow engine and no arbitrary executable schedule kinds. Operational data safety and business lifecycle checks still belong in Chartworks.

## Migration runbook

Inventory current source definitions, active semantic/templates/rules, questions/examples, blocks/revisions/attestations, output mappings, reports/private states, dashboards, schedules and retained artifacts. Include disabled and orphaned records so none disappear accidentally. Keep actual private source schema/identity mappings outside this repository; committed fixtures are neutral synthetic examples.

Run dry import first. The report lists each transformed, unmapped, incompatible or quarantined field. Normalize external ID/revision semantics, parameter precedence/time policies, exact definition pins, older section layouts and output kinds. Default uncertain lifecycle to private draft; never infer publication from a historical adapter's omitted field. No bearer/password/role/grant import becomes authority: Pengui issues the new access decisions.

Preserve historical certificate/result provenance, but do not promote imported evidence to a new current certification without the accepted checks. Compare exact schema/values/aggregates and visibility/period/partial/freshness behavior on fixed source-controlled fixtures. Generated SQL is judged semantically, not only textually; narrative claims are checked against evidence, not exact wording.

Cut over per tenant/source/report cohort. Pause old schedules, record their last logical occurrence and deduplicate the transition before enabling new dispatch. Do not run two independent delivery streams. Keep old read access during the accepted rollback window. Restore pointers/routing and disable new dispatch on rollback; record effects that cannot be undone rather than promising to unsend or globally roll back them.

Expire/delete raw values and all derived renditions consistently. Verify source-context authority on imported and newly created artifacts. Unsupported engines or features keep the cohort open; they cannot disappear behind a feature flag.

## Closure

The feature map contains B01–B20, R01–R16, Q01–Q11 and N01–N16. Q11 is discarded source stubs with explicit rejection tests. Every other row must close with implementation/equivalence evidence. The original G01–G40 identifiers remain mapped, with G27 corrected to test Chartworks Apps output rather than host compatibility, G28 corrected to a Pengui/BFF delivery boundary, and auth/revocation gates corrected to the issuer-owned contract.

Planning checks validate dependency/reference/coverage consistency only. They do not certify source execution, UI fidelity, model quality or cloud-driver behavior. Missing/skipped runtime evidence is reported as incomplete. Phase 34 owns migration parity; phase 25 owns the cumulative release decision.
