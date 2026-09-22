### D-076 — Closed compound/template scopes and immutable reporting rule snapshots

Date: 2026-09-22. Status: accepted implementation disposition for RUL-01 and
BLK-02.

Rules add two safe scope forms: compound scopes require every exact semantic
target, and template scopes require equality with a server-verified reviewed
selection pinned to the exact topic and ruleset publication. Existing entity
scopes remain any-target and topic scopes remain
unconditional. Selection produces deterministic evidence. No scope embeds SQL,
regular expressions or an authority predicate, and rules cannot widen signed
source/topic/context reach.

Template selection is part of the protected query definition: routing
canonicalizes it before generation, template-scoped omission clarifies, stale or
substituted coordinates fail closed, and preflight/plan/refinement/saved replay,
query capture and replay/shadow comparison retain the same input. Migration 039
adds bounded immutable query/comparison evidence without reinterpreting existing
rows.

Reporting definition v2 may pin one exact immutable ruleset per pinned topic
that had an active reviewed ruleset.
Validation, certification health, frozen manifests, reuse identity, compositions
and schedules retain those pins. A replacement or retirement marks dependent
health stale and blocks refresh/reuse until a new draft is explicitly validated
and reviewed. Published definitions, old rule versions and historical
attestations remain immutable.

Existing definitions without rule pins keep their bytes and explicitly have no
captured rule snapshot. Import/export carries exact native pins; unsupported
foreign mappings fail explicitly rather than inventing a rule publication.
Migration 038 adds tenant-composite immutable pin rows. The ordinary API/MCP/SDK/
CLI registration remains thin because these are fields on existing first-consumer
operations, not new parallel domain operations.
Capture fences the current rule publication heads inside its block transaction;
concurrent replacement/retirement returns stale with no partial block state.
