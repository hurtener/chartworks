# Reporting output intent and narrative policy decision

### D-073 — Immutable versioned output intent and bounded evidence policy

Date: 2026-09-16. Status: implementation decision for CW-03 review, following the
explicit assignment. Applies to BLK-01, BLK-05 and BLK-07 only. Existing signed
authority, lifecycle, read validation, shared sensitivity vocabulary, query/queue
fences and chart contracts remain authoritative.

Use definition v2 for authored output intent, query ceilings and closed narrative
policies. Preserve v1 JSON and selection interpretation; migration produces a new
unvalidated draft instead of rewriting publication. Separate display order from
accepted execution order; reject explicit empty, duplicate, unknown and disabled
v2 selections, while default selection skips disabled outputs.

Carry exact intent, effective sensitivity, selected IDs and accepted limits through
publication, manifests, reuse, artifacts, composition and delivery. Expose native
authored export only through existing SQL-plus-read authority; normal metadata
must not become a private definition export or data grant.

Without source-expression lineage proof, inherit reviewed sensitivity
conservatively across query dependencies. Unknown/conflicting/sensitive fields
and manual redactions are excluded before reduction/model input. This is an
explicit safe narrowing, not a claim of per-expression parity. Reuse the existing
semantic sensitivity type; do not invent parallel classifications or grants.

Version deterministic narrative claim/type/tone rendering; use closed evidence
claims with bounded rows/bytes/characters/calls/tokens/time and mandatory caveats.
Unsupported mapping requires an explicit authoring disposition, never a silently
dropped field. Deterministic no-evidence and retained paths stay independent of
irrelevant model availability.

Intersect accepted caps with current deployment/authorized budgets at execution.
Keep uncertain attempts and reservations honest. Persist original selections,
limits and revisions through queue/catalog retries. None of this permits refresh
on retained read, question interpretation on frozen refresh, or a new issuer.

Field-level defaults, unsupported mappings and executable evidence are in the
[v2 contract](../contracts/reporting-output-intent-v2.md) and
[adversarial review](../reviews/cw-03-adversarial.md). Foreign import/cutover,
expression lineage, other reporting gaps, chart internals and clarification policy
remain with their existing owners.
