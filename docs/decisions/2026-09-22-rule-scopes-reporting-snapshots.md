### D-075 — Closed compound/template scopes and immutable reporting rule snapshots

Date: 2026-09-22. Status: accepted implementation disposition for RUL-01 and
BLK-02.

Rules add two safe scope forms: compound scopes require every exact semantic
target, and template scopes require equality with one validated reviewed
identifier. Existing entity scopes remain any-target and topic scopes remain
unconditional. Selection produces deterministic evidence. No scope embeds SQL,
regular expressions or an authority predicate, and rules cannot widen signed
source/topic/context reach.

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
