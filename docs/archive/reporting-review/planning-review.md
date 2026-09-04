# Review of the accessible Chartworks planning proposal

Reviewed 2026-09-04. This is a design review, not an approval of unexecuted code.

## 1. Repository state and review scope

The accessible PR collection showed the original planning PR #1 as merged on 2026-07-07. Main was `8de9641ddba33bd86d4aeb2a080a8a4fedddf01d`; the inspected branch named for a publishing pass 2 pointed to the same commit. The separate `docs/phase0-planning` branch at `5992dad9f1ebfd8b093c8406e78c665936e0d93d` was ahead by two commits and behind by one relative to main; its visible changes addressed provider roles/reranking.

No distinct open reporting PR or two-pass reporting diff was available to inspect at that time. This package therefore reviews the actual main RFC/research plus available planning comments/branch comparison, and proposes a separate amendment. It must not be represented as an inline review of a diff that was not accessible.

## 2. What should remain

The existing direction is sound in several load-bearing areas: a Go core with thin surfaces; one durable queue; immutable verified identity; separate operation scopes and resource grants; read-only query execution separated from managed engineering writes; native-dialect validation; a shared model gateway; reviewed semantic publication; and high-value ideas from the secondary source without copying implementation-specific infrastructure.

Post-merge comments also improve the direction: explicit pipeline-runner dependency decisions, independent SQL safety, governed engineering proposals and operator-bounded autonomy, and per-role gateway providers. Preserve those decisions deliberately; do not rewind the project to an obsolete initial brief because its index has not caught up.

## 3. Findings and corrections

| Priority | Finding | Correction / closure |
|---|---|---|
| Blocking | RFC-001 excludes rendering while the requested product needs visual MCP and iframe delivery. | RFC-002 explicitly admits thin viewing and separately tested SSR; builder UI stays out of scope. |
| Blocking | Chart specifications are not a substitute for blocks/revisions, validation, certification, reports, dashboards and artifacts. | Add the domain/state and route contracts; map all primary reporting behaviors to tests. |
| Blocking | Source publication/trust/current health cannot be represented by one generic approved flag. | Preserve independent state; exact content-bound evidence; current state displayed separately from historical evidence. |
| Blocking | Stored artifacts, caches, service schedules and embed capabilities can cross an authorization boundary after creation. | Current policy checks on every read/delivery; explicit data partitions; private preview flag persisted; no broad service-result reuse. |
| Blocking | The external issuer contract is not yet a demonstrated match to Pengui's actual identity/minter conventions. | Implement and test an issuer-profile adapter, subject consistency, service/delegation constraints, per-surface audiences and real scope registry integration. |
| High | Deterministic refresh can be undermined by silent SQL repair, chart reselection or narrative-driven requery. | Dedicated frozen lane with forbidden-stage tests; amendments create drafts. Narrative operates only on bounded retained evidence. |
| High | “Atomic apply/revert” risks overpromising across external systems. | Transactional local publication plus staged external effects, compensation/reconciliation and truthful partial-failure states. |
| High | Stored query success does not prove exactly-once remote execution or delivery. | Fenced leases, idempotent logical operations, attempt evidence and provider reconciliation; no universal exactly-once claim. |
| High | Schedules need exact temporal/revision/identity semantics, not just a timer. | Persist logical occurrence/window/pins and recheck service grants; specify DST, overlap, missed runs and retirement. |
| High | Recipient metadata could be mistaken for implemented outbound email. | Preserve catalog delivery and distinguish optional outbound effect/receipt handling. |
| High | Source stubs could either inflate migration scope or be accidentally advertised as complete. | Event/condition triggers and the condition-check target are explicitly unsupported source debt; ordinary cron/interval/report/block/saved-query behavior remains required. |
| High | Go rewrite could lose decimal precision, parameter provenance, session-bound distinctions or old report compatibility. | Lossless data contract, explicit import normalization, source-to-target golden fixtures and quarantine instead of silent dropping. |
| High | Broad new autonomy can delay the core differentiated offering. | Deliver reusable governed blocks, reports and portable viewing before L3/autonomous investigations; retain managed engineering boundaries. |
| Medium | Secondary semantic generation has useful code, but a fully automatic environment flow was not established by current-source inspection. | Treat resumable operational setup as an explicit new capability with stage/retry/approval tests, not a promised existing feature. |
| Medium | Historical research/status may conflict with later decisions. | Mark old source-diff assumptions as historical; acknowledge Bruin's superseding decision; revalidate host/library pins at the integration gate. |
| Medium | No measured target performance or executed parity evidence is available from a documentation review. | Establish repeatable benchmarks and real-adapter regression suites; distinguish source tests inspected from tests run. |

## 4. MVP versus complete offering

The first slice should demonstrate an approved block refreshing into several trustworthy outputs, composed into a retained report and viewable with zero new query/model work. This validates the product's differentiator without a drag-and-drop builder, generic workflow platform or a second IAM system.

The full migration still includes advanced authoring, hybrid/dynamic reports, dependency impact, scheduling, deployed source adapters and the existing semantic/NLQ/feedback workflows. These cannot disappear merely because they are outside the first vertical slice. Brief 14 is the closure ledger, not a list of suggestions to cherry-pick.

## 5. Review limitations and next evidence

No source test suite, actual warehouse, running Go service, deployed MCP host, browser embedding integration or SSR worker was executed in this review. The source audit is deepest in reporting contracts and selected execution/scheduling/security paths; several broader pipeline and adapter areas are explicitly inventory/document-level evidence. There is no claim that every source file was inspected.

The next reviewer should challenge policy-partition derivation, current-user versus service authority, source-engine safety, all revision/period selection rules, hybrid-report trust, and host bridge behavior. The required tests and safe defaults are specified; the remaining questions are named rather than hidden behind a claim of full parity.
