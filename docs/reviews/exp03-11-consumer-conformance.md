# EXP-03/EXP-11 consumer conformance review

Date: 2026-09-22. Scope: D-086 and the installed-operation interaction/error
matrix. Fixtures are synthetic.

## Adversarial findings closed

| Risk | Resolution and evidence |
| --- | --- |
| A generated client showed a status but dropped the owner error codes and receipt shape. | `ParseOperations` extracts the stable status/code/receipt inventory from generated OpenAPI, requires protected 401/403 outcomes and rejects malformed or unknown interaction metadata. The cumulative registry test checks every row. |
| A late filter, renderer or onboarding operation existed in its owner package but was absent from generic consumers. | The cumulative test composes current source/topic/NLQ/BYO/chart/reporting/rendition/onboarding/MCP registries and requires exact row count plus named late operations. Every HTTP row has an SDK dispatch and CLI command. |
| Missing MCP tools were mistaken for parity failures or silently invented. | No-query rows say `not_queried`. After `tools/list`, exact metadata joins as `bound`; all other rows receive public, transport or explicit HTTP-only disposition. MCP interaction drift is rejected with action/effect/audit drift. |
| A slow earlier response replaced a newer question/refinement. | `ApplyInteraction` ignores older generation and duplicate/older sequence observations. Query identity cannot change inside a generation. |
| Closing a socket displayed a cancelled result although no cancel was accepted. | `transport_disconnected`, `cancel_requested` and `cancelled` are distinct states. The reducer rejects a disconnect labelled cancelled. |
| Missing, partial or uncertain output collapsed to empty success. | Eight terminal statuses remain separate. Unknown `success` is rejected; owner DTOs retain actual results and attempt receipts. |
| Shared UI state became a new leak channel. | The ordering envelope has only generation, sequence, query ID, kind, status and view. The test confirms its JSON has no prompt, SQL or row field. |

## Verification

- Focused API, MCP, NLQ, source, reporting and SDK tests pass with the pinned
  native parser library.
- `go vet` passes for the touched packages with the same native dependency.
- AGENTS.md and CLAUDE.md remain byte-identical; `git diff --check` passes.
- The repository-wide lint baseline still reports existing Phase 32/33 findings
  outside this diff. The local planning check also hits the existing macOS
  `/var` versus `/private/var` temporary-path assertion in coverage-gate unit
  tests; the plan/reference checks before that point pass.

This review proves the checked source contract and focused runtime behavior. It
does not claim a deployed host, live external MCP discovery or a standalone
authoring application.
