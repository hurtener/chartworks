# Saved validation, preview and retained-run requirements

### D-101 — Native metadata anchors manual effects and exact retained reads

Accepted integration scope, 2026-10-05. Extends D-098–D-100 with a closed
metadata-only effect discovery operation. Block validation resolves the saved
block's actual source/context, separately from whole-publication read reach.
Preview resolves the exact saved report and block pins. Execution resolves the
original private run and its immutable reference index, with the original
actor/session checked before projection. The native effect remains responsible
for current eligibility, CAS, validation evidence, idempotency and source safety.

An exact run discovery seed carries reporting.discover and cw.run.read, but no
reporting.read. It cannot read an artifact. Private records require original
actor/login custody before returning metadata; public report records may reveal
only coordinates to the trusted BFF. Pengui then independently checks canonical
read/preview policy for the original parent and complete native dependency
closure. Only after those checks may it mint fresh exact run read authority.
No run grant database, stored bearer, query retry, model permission or alternate
issuer is introduced. Retained reads use persisted context requirements and do
not reinterpret the current report or execute work.

The HTTP-only registered seam and SDK are specified in
[the dependency contract](../contracts/report-dependencies-v1.md). No migration
or MCP tool is added. Restricted no-chat MCP remains an outstanding integration.
