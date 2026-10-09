# Native publication and preparation requirements

### D-100 — Native metadata resolves reviewed data and original preparation custody

Accepted integration scope, 2026-10-05; source under local qualification. Extends
D-098/D-099 using the same reporting.discover BFF boundary. Pengui remains the
sole policy owner. The native endpoint returns only identifiers and pins from
existing tenant-composite topic dependencies or original private preparation
custody, including compact consumed receipts after native retention.

Topic discovery requires exact topic read and includes every dataset in the
publication. Preparation discovery requires exact block read/write/preview and
original actor/session/target custody. Retained operation metadata takes
precedence over proposed retry selections; it cannot restart source work. The
selected dataset's source/query/context requirements remain distinct from the
whole topic's read requirements. Ordinary content and effect paths retain all
existing checks and source/topic/operation fences.

The metadata endpoint is HTTP control-plane only, with a typed SDK method and
registered schema, limits, errors and audit/effect classification. It adds no
MCP tool, credential, authority store, source query, model call or migration.
See [the contract](../contracts/report-dependencies-v1.md). The bounded embedded
catalog accepts explicit cursors for empty and short policy-filtered pages.
