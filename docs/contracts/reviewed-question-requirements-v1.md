# Reviewed question requirements and amount evidence

This bounded interpretation layer does not certify arbitrary natural language.
It composes explicit requests with current reviewed catalog identities before
SQL generation, and refuses recognized unresolved requirements.

- English and Spanish local-calendar year phrases retain the reviewed dimension,
  calendar and timezone. An explicit selected measure restricts an otherwise
  unnamed temporal choice to its aggregate-input fact; filter/join dependencies
  cannot supply an unrelated default date. Multiple remaining dates clarify
- Reviewed KPI period mappings may bind repeated identical year clauses to
  distinct fact-owned dimensions. Different years, mixed periods and unsupported
  connectors cannot collapse into one interval. Current source pins, typed
  references, mapping digest and resolved intervals are sealed and reconstructed
  during replay; JSON cannot supply the private seal
- Explicit net requests cannot substitute a gross metric. Missing reviewed net
  meaning and competing reviewed definitions produce distinct clarification
  reasons. Physical column names do not establish net meaning. Typed measure and
  KPI identities remain distinct even when their local IDs match
- Recognized requests for definitive totals despite unknown amounts require a
  missing-value decision. Repeated local-clock requests require disambiguation;
  hourly proof remains unsupported. Quoted values are not interpreted as these
  requirements. These are finite language rules, not a universal semantic oracle

## Required completeness outputs

A directly selected SUM may declare an exact reviewed unknown-count companion.
The companion is a separate required output root. It has no independent period,
uses the same population and grouping, and cannot be explicitly omitted while
retaining the owning measure. A dependency-only SUM inside another KPI does not
silently add outputs. The compiler rechecks both semantic and current effective
physical nullability, including outer-join null extension.

Version 9 native proof records each selected metric's final output ordinal and
exact completeness obligation pairs. Result annotations use these ordinals,
never driver aliases. A positive exact unknown count marks the returned row
incomplete; zero marks it complete. Missing, negative, fractional or incompatible
values, stale/failed reads and truncated results cannot claim completeness.
The public `amount_completeness` field scopes this evidence to returned query
rows; it does not certify upstream source coverage. Companion columns are
explicitly identified so consumers can distinguish them from amount series.

Earlier proof versions cannot acquire this evidence by decoding new receipt
fields. Cached version-9 results re-establish current native and analytical
receipt equality before using retained ordinals. Live model quality and other
unrecognized language remain separate qualification obligations.

## Original Plan operation replay

New successful explicit Plan operations retain an immutable private operation
key and digest of the canonical caller submission, before inferred grouping or
private-instruction redaction. They are separate from mutable Run-attempt keys.
Repeating the original request returns the same opaque query identity before or
after execution, including after service reconstruction. A different request,
parent or session cannot reuse that reservation. PlanAndRun and operation
resolution use the same retained identity; Run still re-establishes current
source, authority and analytical proof before returning rows.

Migration 073 keeps existing rows unchanged, protects the new fields from
mutation and serializes cross-query Plan/Run operation collisions within the
existing tenant/actor namespace. The request digest is not a public field or
executable authority. Historical rows lacking the original submission retain
their prior exact-route replay rules rather than inventing a caller request.
