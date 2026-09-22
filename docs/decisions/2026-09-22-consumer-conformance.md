### D-086 — Query interaction and cross-surface parity derive from installed operations

Date: 2026-09-22. Status: accepted implementation decision for EXP-03/EXP-11.

The immutable HTTP registration remains the source of truth for every shipped
operation. OpenAPI now carries the owner error inventory and an optional closed
query-interaction role. The generic SDK and CLI consume every installed HTTP
operation. MCP exposes only operations with a typed owner binding; after an MCP
catalog is actually queried, each matrix row records either the matching tool or
the explicit `http_only_no_registered_mcp_binding` disposition. Absence from MCP
does not fabricate a tool or imply that the HTTP operation is unavailable.

The supported query journey is start/clarify, progress/clarify, explicit cancel,
result inspection, retained view selection, feedback and refinement. Consumer
ordering uses a monotonically increasing generation and sequence. Responses from
an older generation or sequence cannot replace current state. A transport
disconnect is recorded separately from cancellation intent. Successful, empty,
truncated, failed, uncertain, cancelled, timed-out and interrupted outcomes remain
distinct. The shared ordering envelope contains only IDs and lifecycle metadata;
it has no prompt, SQL, row or native diagnostic field.

This decision adds no standalone authoring application, stream service, identity
store, query executor or success-returning placeholder. Domain services continue
to own authority, resource resolution, state transitions and audit effects.
