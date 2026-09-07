# Phase 12 — engineering-profiling

Status: shipped. Owner: internal/engineering. Hard dependencies: 05, 06, 08, 10.

## Authority and design

RFC-001 §7/8, D-049/D-052 and [COMMON.md](COMMON.md) apply. Profiles are versioned evidence, not approved semantic meaning. Deterministic profiling remains useful without an optional model summary.

## Brief findings incorporated

Briefs 02, 09, 11, 12, 14: quality dimensions, freshness, value families, selective profiling and dependency health.

## Findings I'm departing from

A bounded returned sample is not proof of a bounded source scan. Do not publish incomplete inferred semantics or rewrite approved reports because a profile changes.

## Scope and implementation tasks

1. Create versioned profiles with sampling provenance, normalized summaries/value families, quality checks, observation timestamps and freshness buckets.
2. Implement schema rediscovery/diff and dependent-topic/report health signals; avoid treating profile generation as semantic publication.
3. Run optional gateway summaries on sanitized bounded context; keep useful deterministic profiling available without an LLM.

## Non-goals

No full automatic data catalog, invisible scan-cost assumptions, autonomous semantic publication or required LLM for deterministic quality checks.

## Config and persistence

Profiling sample rows/bytes/cost/time, sensitive-field policy references, freshness thresholds, optional summary enablement. Persist profile version, sampling strategy, source context/version, observation time and findings. Use idempotent dependency events and shared queue operations.

The recovered implementation defaults to 1,000 sample rows, 1MiB returned evidence, an optimizer-cost ceiling of 10,000,000 and a 30-second sampling timeout. The common validator and executor enforce these as additional lower limits, never authority to exceed their own ceilings. The prefix sample is not random or necessarily representative; actual scanned bytes remain unknown, and `scan_bounded` is false. Freshness defaults are 24 hours for fresh and seven days for stale, with explicit unknown/partial-sample states and observation timestamps distinct from dataset event time.

Profiles retain immutable caller/session/source-context/input manifests, deterministic checkpoints and explicit native-read intent before dispatch. Resume reuses the accepted version, policy, bounds and optional-summary decision with current supplied authority. A lost summary attempt is not silently retried with a new model budget. The source registry, existing read-attempt journal and shared operation ledger remain the authorities for their respective states; no separate scheduler or credential-renewal system is introduced.

Ranges are redacted by default; operator-configured `profiling.policies` can permit numeric/temporal range columns for a specific tenant/source. Optional summaries use the existing gateway with bounded aggregate context rather than raw samples, names, SQL or credentials. They do not alter deterministic findings or publish semantic definitions. Recorded provider fixtures remain test fixtures, not live-model quality or latency evidence.

Uploaded-source erasure now clears both complete profile results and unpublished checkpoint values for the exact tenant/source, including profiles created by another actor with source reach. Profile heads, dependency/health references and pending derived work are handled inside the fenced upload-erasure completion transaction. An interruption after workspace deletion leaves the source tombstoned and the operation incomplete until metadata cleanup is reconciled. Unrelated source/tenant data must remain unchanged. Minimal immutable replay manifests are retained; logical live-data cleanup does not imply physical backup/WAL erasure.

## Acceptance criteria

1. **AC01** — Profile schema and quality dimensions match synthetic goldens; sensitive values are excluded/redacted before model calls.
2. **AC02** — Configured sample/cost/time limits are enforced by adapter capability; a row sample is not mislabeled as proof of no source scan.
3. **AC03** — Freshness classification, unknown freshness and timezone boundaries are deterministic and traceable.
4. **AC04** — Schema diff identifies affected dependencies and emits idempotent health/invalidation events without auto-editing approved SQL.
5. **AC05** — Cancellation/retry/resume preserves profile version and stage evidence; source failures are typed.
6. **AC06** — Profile outputs feed real semantic generation/inspection consumers with scoped integration fixtures and measured cost/latency.

## Tests, coverage and smoke

Implement `TestPhase12/AC01` through `TestPhase12/AC06`. Real source profiling/limits and source-change fixtures supplement pure summary/freshness tests; semantic consumers close the boundary before their phase closure. COMMON.md supplies coverage; `scripts/smoke/phase-12.sh` requires all six results.

The shipped implementation contains named acceptance tests plus source-backed checkpoint, cancellation, provider-redaction, schema-diff and inspection fixtures. `TestUploadErasureRemovesDerivedProfileValuesAcrossActors` covers complete and unpublished evidence, another actor, interrupted erasure, same-operation resume and an unrelated surviving source. Historical failures remain in the [adversarial review](../reviews/phase-11-12-adversarial.md) and [request-recovery record](../reviews/phase-11-12-request-recovery.md). The [current evidence ledger](../reviews/phase-11-12-current-evidence.md) records the accepted local gates and pending final cloud rerun.

Permanent read-only CI requires all 76 implemented criteria, the full race-enabled coverage suite and bounded engineering/parser fuzz checks without reducing earlier gates. AC06 still requires actual measured source/service/model attribution as applicable; an inspection seam does not finish the later semantic-generation or publishing phases.

## Glossary, decisions and deviations

Observation time, freshness and sampling provenance are distinct. D-049/D-052 apply. Later semantic generation/publication and live provider quality remain obligations of their owning phases.
