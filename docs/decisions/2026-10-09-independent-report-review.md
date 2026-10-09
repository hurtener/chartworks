# Independent report review and host-owned sharing

### D-110 — Inspect and publish reports without borrowing edit permission

Accepted, 2026-10-09. Continues D-096/D-101–D-109 and supersedes the manual
lane's mandatory write permission for report worklists, opening and inspection.
The native private worklist intersects exact report read with exact write or
publish selections before metadata projection/pagination. Reopening requires
read and write or publish; all persisted dependency and private-custody checks
remain mandatory. No synthetic envelope or inferred role supplies authority.

Authoring read/drafts and report-transition dispatch use `reporting.read`.
The transition core still selects write for review submission and publish for
publication/rejection. Saving and rebinding require write. Private execution
requires write, preview, execute and source-query authority. A publisher-only
reviewer can inspect and publish a reviewed revision but cannot save, rebind,
create, execute a preview or publish child blocks without independent authority.
Capability booleans are current presentation hints, never dependency proof.

Pengui owns the people/Team dialog and canonical report access contributions.
The App sends only an exact report/revision selector through an advertised
`app/manage-report-access` parent method, for both iframe and restricted MCP.
Chartworks receives no manage scope and stores no access policy. Full Pengui
preflight checks current native dependencies and recipient data access before
canonical report/block grants; publication and granting remain separate actions.
Every subsequent Chartworks request still checks the supplied signed envelope.

No Chartworks migration or new source/model work is introduced. PostgreSQL
acceptance, HTTP/MCP wire tests and read-only reviewer UI tests establish this
native increment. Signed-in author-to-Team-to-amendment visual qualification and
hosted/deployed evidence remain separate in the flexible authoring plan.
