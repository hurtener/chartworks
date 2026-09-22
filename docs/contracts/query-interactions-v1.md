# Query interaction and consumer conformance v1

Status: implemented for EXP-03/EXP-11 review. D-086, D-070 and the Pengui
authority contract apply.

## Generated surface contract

Every installed HTTP operation is projected from the immutable registry into the
OpenAPI document and `chartworks client operations`. The projection includes its
method/path, action, resource loader, effect, audit classification, replay policy,
closed request/response schemas, parameters and stable error codes grouped by
status. Receipt-bearing errors are marked without exposing a provider diagnostic.

The generic SDK `Invoke` and CLI `client call` paths cover every HTTP-audience
operation. The MCP transport is separate. With no MCP catalog, the matrix reports
`not_queried`. After a separately authorized `tools/list`, exact owner metadata is
joined and each row becomes `bound`, `public_http`, `transport`, or
`http_only_no_registered_mcp_binding`. The last value is an intentional reviewed
surface disposition: no typed owner binding was installed, so clients use the
registered HTTP SDK/CLI operation. No MCP tool is synthesized.

Late-domain conformance includes selectable report filters, static and durable
renditions, and guided onboarding. Their schemas, authority, errors and generic
client paths come from the same registrations as their handlers.

## Query journey

`x-chartworks-interaction` identifies these actual roles:

| Role | Meaning |
| --- | --- |
| `query_start_or_clarify` | Route/admit a question; may return a typed clarification before generation. |
| `query_progress_or_clarify` | Generate/validate a plan; may stop for clarification. |
| `query_cancel` | Persist explicit cancellation intent against the addressed execution. |
| `query_result` | Execute or inspect an exact retained attempt and its truthful outcome. |
| `query_view` | Select table/chart/output presentation from retained evidence without execution. |
| `query_feedback` | Record bounded governed feedback. |
| `query_refine_or_clarify` | Create a child plan in the same signed session or ask again. |

`InteractionJourney` fails closed if a complete installed composition lacks any
role. It returns operation IDs rather than local callbacks, so action/resource
requirements remain visible and current authority is rechecked by the service.

## Ordering and terminal states

`ApplyInteraction` accepts a consumer generation and sequence. A newer submitted
intent advances generation; any older or duplicate observation is ignored. The
same generation cannot switch query identity. This prevents a slow earlier result
from replacing the result for a newer question or refinement.

`transport_disconnected` is not `cancel_requested` or `cancelled`. Cancellation is
shown only after an explicit registered cancellation response. Terminal results
retain `succeeded`, `empty`, `truncated`, `failed`, `uncertain`, `cancelled`,
`timed_out` and `interrupted` as separate values. Missing output is never coerced
to empty success. View switches carry only `table`, `chart` or `sql` selection and
do not call a model or source. SQL remains subject to its separate read scope.

The ordering envelope structurally contains only generation, sequence, query ID,
kind, status and view. Raw prompts, SQL, rows, tokens and native diagnostics cannot
enter it. Owner result DTOs remain the authoritative typed data contract.

## Evidence and limits

The cumulative synthetic registry test composes every analytical domain registry,
including filter, rendition and onboarding operations, then parses the generated
OpenAPI matrix and verifies error/SDK/CLI completeness. Interaction tests cover
stale replacement, disconnect versus explicit cancel, all terminal states and the
content-free envelope. Existing phase 21/22/23 domain tests retain real authority,
PostgreSQL, source, MCP and CLI behavior.

This is contract/runtime conformance, not a claim that a separate authoring UI or
live external MCP host was deployed. Feature-disabled operations remain absent.
