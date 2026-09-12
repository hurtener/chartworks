# Phase 29 — reports-dashboards

Status: in_progress. Owner: internal/reporting. Hard dependencies: 18, 28.

## Authority and design

RFC-002 §§2–6, D-045/D-047 and [COMMON.md](COMMON.md) apply. Reports compose the existing execution lanes. Dashboards reference exact report revisions; neither creates another SQL/visualization engine.

## Brief findings incorporated

Briefs 06, 14; source coverage R01–R16. Preserve grid/filter/presentation metadata, private review state, hybrid widget durability and retained per-widget provenance rather than reducing reports to frozen charts only.

## Findings I'm departing from

Publishing the enclosing report cannot certify dynamically generated SQL or make old private previews public. Creator/audience/origin labels are not grants. Old section layouts are projected without mutating historical revisions.

## Scope and implementation tasks

1. Add report/revision and dashboard/revision domain storage with grid/filter/widget definitions, exact page references and publication workflows.
2. Compose block, explicitly dynamic replayable/session-bound query, and safe text widgets; normalize older section layouts into the canonical read projection.
3. Resolve run references/filter bindings once, deduplicate equivalent block execution, preserve per-widget trust/provenance and explicit partial failure.

A report run resolves all floating pointers once before execution. Deduplicate only within identical query/parameter/context semantics and fan out the union of selected outputs; changed output subsets remain explicit. Query widgets carry their own query/semantic evidence and never inherit a block's certificate. Static text is sanitized plain/Markdown, not a custom script. Page redaction omits unauthorized content without disclosing hidden names.

## Non-goals

No drag-and-drop builder, arbitrary code widgets, source-session impersonation or second dashboard execution model.

## Config and persistence

Reports widgets/filters/pages default ceilings=100, live_queries=false, session_bound=false, partial_failure=fail_closed; bounded safe text/presentation options. Persist independent draft/review/published pointers and artifact privacy. Reports/dashboard revisions, widget definitions and external-reference metadata use tenant-composite keys. Parent audience labels remain descriptive only.

## Acceptance criteria

1. **AC01** — Report/dashboard create/edit/review/publish/reject/archive and localized metadata have CAS, reference integrity and signed resource scope.
2. **AC02** — Private draft/review previews remain private after later publication; preview permission never comes from creator metadata alone.
3. **AC03** — Floating references resolve once into a manifest; page order/exact revisions and shared-block output fan-out stay stable during concurrent publication.
4. **AC04** — Dynamic widgets require explicit enablement and query authority; replayable versus session-bound context is distinguished and never mislabeled certified.
5. **AC05** — Strict/partial policies record omissions, output errors, mixed freshness and budget failures without a false complete result.
6. **AC06** — Filter-to-parameter typing/precedence, safe presentation overrides and grid/text limits are validated; filters are not row-security authority.
7. **AC07** — Legacy section projection and external-reference/version semantics preserve meaning without mutating old revisions; unsupported records are quarantined.
8. **AC08** — Dashboard redaction discloses no hidden page names/data and may return zero visible pages; metadata summaries do not fetch raw results.

## Tests, coverage and smoke

Implement `TestPhase29/AC01` through `TestPhase29/AC08`. Use mixed block/query/text reports, repeated blocks, concurrent publication, private-preview-to-public-transition, missing sessions, disabled live execution and zero-visible-page fixtures. Verify no unsafe shared execution across contexts. COMMON.md sets coverage; `scripts/smoke/phase-29.sh` requires all eight results.

## Glossary, decisions and deviations

Report publication, widget origin, query durability and artifact privacy are independent. D-047 applies. The implementation is submitted for review on `feat/phase-29-reports-dashboards`; `in_progress` is retained until review/merge, not to bypass acceptance.

The eight named criteria now exercise the HTTP/SDK lifecycle and cumulative Phase 21 registry, real PostgreSQL/native source execution, output-subset isolation, publication races, saved-query routing/session identity, durable replay, current-scope retained reads and real expiry. Additional AC01/AC03/AC05/AC08 scenarios require transactional audit/first-seal rollback, recovery after a lost generated-plan reply without more model calls, explicit uncertainty when no plan exists, and bounded expiry erasure with audit-failure rollback. No immutable expiry or authority proof is rewritten to make these tests pass.

An unavailable optional narrative is an output-local failed receipt in an explicitly opted-in partial frozen run; composition still enforces its own strict/partial policy and the full declared narrative budget. Strict standalone frozen admission retains its prior fail-fast behavior. A failed strict report does not expose a completed-looking widget payload.

The [runtime contract](../contracts/reporting-composition-v1.md), [runnable request fixtures](../../examples/report-create.json), and [adversarial review](../reviews/phase-29-adversarial.md) describe the implemented scope. All eight strict acceptance results and exact-source CI remain mandatory. Phase 29 does not claim the Phase 30–34 schedules/viewer/rendering/cutover capabilities or the Phase 25 release gate.
