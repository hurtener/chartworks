# Published Consumer dependencies and original retained history

### D-102 — Native published execution and original-run discovery

Accepted integration scope, 2026-10-05. Extend D-098 and D-101 so Consumer search,
exact describe, explicit published execution and retained block/report history
use complete native requirements. Pengui checks each requirement through its
canonical policy; App visibility, editor status and legacy ContextIDs do not grant
content access. Published execution requires independent execute and source-query
permissions, with no write, preview or model permission.

A bounded HTTP-only run-candidate operation exposes original run coordinates to
the trusted BFF under exact parent discovery reach. Private candidates require
original actor/login custody and independent preview. The BFF authorizes each
run's original immutable dependency closure before requesting an exact summary.
Hidden candidates and their cursors never reach the browser. Retained values stay
in Chartworks; the BFF stores no artifact or per-run policy. Candidate enumeration,
summary reads and dependency discovery perform no source/model work.

See [the dependency contract](../contracts/report-dependencies-v1.md). No migration
or new MCP tool is added. Both delivery modes must reuse this policy boundary;
restricted no-chat MCP and full real-service Consumer acceptance remain open.

Subsequent local HTTP evidence: the independent ordinary-reader issuer/PostgreSQL
journey is qualified in [the integration review](../reviews/pengui-app-integration-2026-10-05.md).
Restricted no-chat MCP remains open; this does not close both-mode acceptance.
