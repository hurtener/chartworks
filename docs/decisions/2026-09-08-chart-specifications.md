# Output specification implementation decision

### D-069 — Bounded provider-neutral output specifications and executable HTTP registration

Status: implemented for review, 2026-09-08. Owns phase 20 and the cumulative phase-21 continuation. RFC-001 §§10–11, RFC-002 §5 and D-047/D-049 remain binding; this does not move renderer or reporting ownership.

## Decision

Use a pure, deterministic `internal/charts` core with a closed version-one specification rather than exposing a renderer's arbitrary options. All fourteen catalog entries have explicit bindings and suitability checks. Exact decimal/integer labels and totals are independent of approximate coordinates. Total scope describes all provided rows, not an inferred full-source aggregate. Frozen mappings pin column metadata and never reselect; rebinding is a detached review proposal.

The first real consumers are five HTTP/Go SDK operations on `internal/chartservice`. Their data is explicitly caller-supplied, not a source reference or proof of business approval. Every operation requires a current Pengui action plus signed tenant read reach; the service has no store, source reader or local authority constructor. The SDK's typed read-result adapter preserves exact values without requerying. No second chart-state store is introduced. Later block revisions persist selected definitions and retained artifacts own privacy.

Optional author-requested ranking uses the existing Bifrost gateway only. It receives a bounded author intent and a sealed set of suitable kind IDs/rule descriptions, not query rows, labels, source metadata or saved definitions. Invalid/incomplete ranking leaves deterministic rules intact. Late interruption suppresses the result while preserving attempted usage in a content-free typed failure receipt. Reservations, provider usage and unknown cost are distinct.

HTTP registration becomes executable at the composition boundary: unregistered paths cannot dispatch and protected routes require a valid bearer and their registered action before domain handlers. Domain services still load and enforce real resources; descriptive loader/audit metadata is not a replacement for those checks. Existing isolation tests remain, and the cumulative registry includes implemented NLQ, BYO and chart consumers. Nullable collection schemas do not permit null required scalars. Request decoding and service work each have bounded admission.

## Preserved scope and limits

This is specification selection/building, not rendering, a standalone authoring UI, query execution or automatic report approval. Rendering/pixel goldens remain in phases 31–32; saved publication is phase 27. The flat/two-level treemap contract, temporal line/area coordinates, no implicit aggregation, explicit unsuitable bindings and bounded table fallback are documented rather than advertised as arbitrary visualization support.

No issuer, IAM, credential, scope-renewal or Pengui policy change is implemented. Operators must configure the actual chart actions on their existing integration. No live provider/cutover qualification is inferred from recorded fixtures or a chart screenshot.

## Evidence

[The version-one contract](../contracts/chart-specifications-v1.md), [owning phase](../plans/phase-20-charts-spec.md) and [review record](../reviews/phase-20-21-adversarial.md) enumerate the named tests, real read-to-spec integration, fixtures and current verification boundaries. Shipping requires review and the committed head's full required checks.
