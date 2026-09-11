# Phase 26/28 runtime completion audit

Status: final committed-source verification pending. The PR remains draft.
Review was performed directly; no subagents were used.

## Requirement traceability

| Criterion | Runtime evidence | Acceptance evidence |
| --- | --- | --- |
| 26 AC01: evidence and decisions for every object | `autopilot_planning.go` and `autopilot_model.go` retain bounded blind/matched planning, references, alternatives and provenance for pipeline, dataset, optional topic and schedule material. | `TestPhase26/AC01`; topic and schedule object consumers in AC06. |
| 26 AC02: authorized planning scope | Proposal preparation resolves the addressed source context, validates generated SQL and rejects foreign evidence; proposal reads recheck signed references. | AC02 cross-tenant, different-context and foreign model-evidence negatives. |
| 26 AC03: independent review and ordinary authority | Review CAS binds exact material; edits invalidate approval. Apply requires ordinary pipeline/topic/schedule authority. | AC03 self-review, competing review and stale approval; AC06 missing pipeline/schedule permission and private topic publication negatives. |
| 26 AC04: actual staged effects and bounded compensation | Pipeline, topic and schedule effects are recorded from real persisted consumers. Compensation checks current native ownership and published dependents. | AC04 lost draft reply and actual native quarantine; AC06 dependent consumer and schedule-commit interruption/reconciliation. |
| 26 AC05: deduplicated reviewed drift | Physical source probes plus freshness/quality evidence produce immutable drift and independent amendments. Impact projection requires all dependencies. | AC05 deduplication, current impact reach, amendment replay and no inherited approval; topic/schedule amendment consumers in AC06. |
| 26 AC06: goal to managed data | Production assembly wires HTTP/SDK L2 execution, private topic authoring and scheduled pipeline execution under the existing operation lease. | AC06 actual native materialization, private topic apply/amendment, schedule create/replace/recovery and accepted occurrence execution. |
| 28 AC01: frozen stage prohibitions | Frozen execution composes the validator/read executor and deterministic chart builder; the gateway is reached only for selected narratives. | AC01 exact frozen SQL and zero model calls without narratives. |
| 28 AC02: one logical result, selected outputs | The sealed ordered output list fans out from one retained typed result. | AC02 output selection, invalid outputs and physical attempt evidence. |
| 28 AC03: parameters and schema meaning | Admission resolves parameters/window once; execution checks exact ordered schema and pinned dependencies. | AC03 parameter precedence, actual schema drift and refusal to rebind. |
| 28 AC04: bounded grounded narratives | Evidence reduction/redaction precedes the gateway call; durable budgets and mechanically grounded claims retain exact provenance. | AC04 allowed evidence, model/prompt provenance, invalid output, retained paid usage and a lost accepted narrative response without model replay; numeric boundary unit tests. |
| 28 AC05: idempotency and recovery | A reserved operation seals one manifest; fenced checkpoints retain results and output reservations. | AC05 lost result/attempt/output/completion replies, changed-key conflict and forged checkpoint rejection. |
| 28 AC06: private/context-aware reuse and reads | Store queries enforce current target/context reach and original privacy; reuse binds revisions, parameters, locale and source partition. | AC06 concurrent admission/reuse, foreign tenant/context reads, original observation/expiry and private preview after publication. |
| 28 AC07: retention without execution | Retention deletes retained values/outputs and preserves bounded tombstones; output rebuild uses retained data. | AC07 actual deletion, expired read/rebuild refusal and no warehouse/model access. |
| 28 AC08: usable APIs, cancellation and budgets | Registered HTTP operations and SDK methods call the shared core; explicit cancellation reaches the owned active query. | AC08 real HTTP/SDK admission, execution, paging, output, cancellation, invalid wire requests and quota refusal. |

## Review findings resolved

- Added the missing application, HTTP and SDK consumers from the initial domain-only branch.
- Bound drift impact discovery to every required topic/block dependency and current action.
- Preserved explicit output order and exact scientific-notation narrative arithmetic.
- Preserved reviewed private topic material and advanced amendments from actual committed draft effects.
- Corrected nullable request schemas without widening accepted wire shapes or bearer limits.
- Bound queue admission to bearer expiry, including database lock waits.
- Pinned manual fire to the schedule revision whose target was authorized.
- Preserved the authority error contract before classifying completion targets.
- Added schedule effect-receipt recovery and concurrent frozen admission/reuse coverage.

## Verified evidence

- Linux run 34548849438 at a5d2e484a6cd8955c05d3fcaa9e9dd4059fa4b93 passed strict Phase 26 and 28 acceptance on 2026-09-11 at 01:15:06 UTC, including reviewed schedule creation and amendment.
- Linux run 34549071207 at db3da3f8e94f962f0aab946f2a1336dc031aec5a passed strict Phase 26 and 28 acceptance on 2026-09-11 at 01:17:06 UTC, including interruption after the ordinary schedule commits but before its proposal effect receipt.
- Local real-PostgreSQL Phase 28 race acceptance passed with concurrent admission/reuse and cross-tenant checks (19.577 seconds; source committed as e9709ca).
- At 382a730, all local core, SDK and CLI race tests passed. Focused queue/Phase 06/Phase 21 regressions and lint passed at their checked sources.
- These results do not establish final-head full coverage. The earlier failed cumulative runs are diagnostic evidence only.

- Linux run 34549736699 at 382a73023966d9854a90d824737f18ff83275fe7 passed strict Phase 26 and 28 acceptance on 2026-09-11 at 01:26:24 UTC, including execution of an accepted prior pipeline version after schedule replacement. Both native platform builds, lint, Clients and MCP also passed.
- The additional lost-narrative-output case passed locally with race instrumentation (16.141 seconds). It interrupts after the provider response but before the output checkpoint, then verifies one physical query, one model call, an unchanged reservation and an honest indeterminate output on resume. Focused lint reports zero issues.

## Additional persistence regression evidence

- At f43b115, real PostgreSQL proposal tests passed with race instrumentation for rejection/resubmission, concurrent edits, audit-write rollback, capacity enforcement, idempotent identity and target-redirection refusal. The planning-only fixture uses a non-executable runner sentinel; it does not substitute for native managed execution acceptance.
- At 4317073, Phase 28 AC07 passed with race instrumentation after adding an injected retention audit failure. Payloads and outputs remain intact on failure, and the subsequent sweep completes deletion. Focused lint passed.
- The a5d2e48 cumulative diagnostic profile measured engineering at 79.54% and PostgreSQL at 80.47%, below the required 80% and 84%. The new proposal/authority tests exercise 6 and 23 additional statements respectively in source files unchanged from that profile. These incremental counts are diagnostic, not a passing full-suite coverage claim; required coverage remains unresolved.

## Remaining release evidence

- Final delivery-head strict Linux acceptance, including the added lost-narrative-output regression.
- Successful full cumulative suite and unchanged package coverage bands, including the existing owner-approved 84% PostgreSQL band.
- Latest-head container/native, client, MCP, fuzz, preflight and clean-source checks.
- Final review of the resulting committed source and coherent phase status/evidence updates.

Chartworks owns pipeline schedules and execution. The separately prepared platform
issuer extension supplies fresh manifest-bound authority and must be available
before enabling this target in a deployment. No platform PR publication, merge,
production deployment, live provider-quality measurement or phase 25/34 release
completion is claimed here.
