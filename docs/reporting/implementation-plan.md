# Reporting implementation and migration execution

The active specifications are the numbered plans and dependency/coverage registry under `docs/plans/`. This guide does not replace them. Shared ownership/security lives in RFC-001; reporting semantics in RFC-002; Bifrost SDK-only remote inference in `docs/contracts/model-gateway.md`. No local issuer, grant-policy service or host qualification remains to be decided.

## Milestones

| Milestone | Owning phases | Observable result |
|---|---|---|
| Foundations | 01–10 and early21–23 | Pengui JWT enforcement, actual read safety, Bifrost remote gateway, real durable authority adapter and thin consumable surfaces |
| Governed slice | 12,15,20,27,28 | Source/topic -> block draft -> actual validation -> publish/certify -> selected outputs -> retained result without NLQ re-entry |
| Composition and delivery | 16–19,29–32 | Hybrid reports/dashboard pages; viewer31 and schedules30 can proceed in parallel;32 adds genuine SSR/BFF delivery |
| Onboarding and full continuity | 11,13,14,24,26,33 | Uploads, managed engineering, required adapters, replay/learning and resumable semantic setup |
| Cutover/release | 34 then25 | Every required feature/cohort/engine has applicable evidence and schedule handoff/rollback is rehearsed |

Numbers identify phases, not chronological order. Follow hard dependencies. Phase06 consumes the real Pengui authority adapter before early durable jobs; phase30 reuses it for report/block/saved-query targets. The concrete platform binding API is wired or extended in Pengui, not asserted already available or replaced by local auth. The gateway's local SDK client is remote inference, never model loading.

## Development procedure

Implement each phase with its first consumer, migrations, registered HTTP/SDK and assigned MCP operations, typed signed authority, errors/audit/usage and named TestPhaseNN/ACxx assertions. Real source/store/SDK boundaries matter. Provider wire fixtures are not live model accuracy. All model operations, including optimization, embeddings and rerank, use the Bifrost-only contract; pure local tokenization/rules/SQL parsing/pgvector remain allowed.

No standalone builder, second queue, local IAM, embed auth service, local inference fallback or arbitrary executable schedule kind. Frozen execution uses approved definitions; retained result reading/rerendering invokes zero source/model calls. Private previews, exact numbers, context partitions and logical occurrence windows survive every surface.

## Migration runbook

Inventory source definitions, semantic versions/templates/rules, learned examples/questions, blocks/revisions/attestations/output mappings, reports/private states, dashboards, schedules and artifacts. Include orphaned/disabled records. Private source schema/identity mappings stay outside the repository; committed fixtures are neutral synthetic examples.

Dry-run import lists transformed, incompatible, unmapped and quarantined fields. Normalize external IDs/revisions, parameter precedence/time policies, exact definition pins, section layouts and output types. Uncertain lifecycle defaults private, not published. Legacy bearers/passwords/roles/grants never become new authority; Pengui issues those decisions.

Preserve historical certification and result evidence, but require current accepted checks before new certification. Compare exact schemas, values/aggregates, period/partial/freshness behavior and privacy on fixed source-controlled data. Compare generated SQL semantically rather than just text; validate narrative claims against evidence, not exact wording. Embedding/model changes create new generations and never masquerade as compatible solely because dimensions match.

Cut over per tenant/source/report cohort. Pause old schedules, record last accepted occurrences and deduplicate the handoff before enabling new dispatch. Keep old authorized reads during rollback window. Rollback restores explicit routing/pointers and disables new dispatch, while recording already performed effects instead of promising to unsend messages or atomically undo external DDL.

Expire/delete data and derived renditions consistently. Validate actual source-context reach on old/imported/new artifacts. An unsupported required feature/engine leaves the cohort open rather than disappearing behind a flag.

## Closure

The 63 source rows are B01–B20, R01–R16, Q01–Q11, N01–N16. Q11 is deliberately discarded stubs with rejection tests; every other row requires implementation/equivalence evidence. G01–G40 retain their corrected ownership/trust scope, with G27 testing Chartworks Apps rather than the host and G28 using the BFF. G41 adds the Bifrost-only remote inference and strict embedding/rerank boundary. All map to 224 criteria over34 plans.

`make planning-check` verifies graph/links/mappings and tooling tests only. `make preflight-full` reports unimplemented plans as explicit skips while testing implemented phases. `make release-check` rejects skips/missing tests even when a development skip flag is present. Phase34 owns complete migration parity; phase25 owns final operational release. A first slice, a file inventory, a planned status or a green documentation check is not that release evidence.
