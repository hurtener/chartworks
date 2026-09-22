# EXP-03/EXP-11 consumer conformance review

Date: 2026-09-22. Scope: D-086 and the installed-operation interaction/error
matrix. Fixtures are synthetic.

## Adversarial findings closed

| Risk | Resolution and evidence |
| --- | --- |
| A generated client showed a status but dropped the owner error codes and receipt shape. | `ParseOperations` extracts the stable status/code/receipt inventory. `Invoke` preserves bounded registered codes even when one HTTP status has several codes, rejects decoded inventory drift, and never retains arbitrary response content. CLI output includes only the validated code and HTTP status. |
| A late filter, renderer or onboarding operation existed in its owner package but was absent from generic consumers. | The cumulative test composes foundation, security, work, source/topic/NLQ/BYO/chart/reporting/rendition/onboarding/MCP registries and exercises enabled/disabled registry selections. It requires exact row count plus named late operations. Every HTTP row has an SDK dispatch and CLI command. |
| Missing MCP tools were mistaken for parity failures or silently invented. | No-query rows say `not_queried`. After `tools/list`, exact metadata joins as `bound`; all other rows receive public, transport or explicit HTTP-only disposition. Action/effect/audit/interaction, resource loader, input/output/result schema and error-contract drift are rejected. |
| A slow earlier response replaced a newer question/refinement or a completed outcome. | `ApplyInteraction` ignores older generation and duplicate/older sequence observations. Query identity cannot change inside a generation. Every terminal outcome is preserved against delayed progress, cancel and refine events; disconnect and feedback use separate fields. |
| Reporting execution was present but absent from the shared journey. | Admission, progress/receipt inspection, explicit cancel, execution/result reads and retained row/output/view operations carry exact interaction roles. The cumulative test asserts named operations in each family rather than merely checking that every role is nonempty. |
| Rendition MCP bindings failed at startup because effects were unclassified or weakly annotated. | Static export, durable static generation and bounded deletion have explicit conservative annotations. Foundation assembly covers durable-renderer startup and bound invocation tests cover all three effects, including destructive deletion. |
| Closing a socket displayed a cancelled result although no cancel was accepted. | `transport_disconnected`, `cancel_requested` and `cancelled` are distinct states. The reducer rejects a disconnect labelled cancelled. |
| Missing, partial or uncertain output collapsed to empty success. | Eight terminal statuses remain separate. Unknown `success` is rejected; owner DTOs retain actual results and attempt receipts. |
| Shared UI state became a new leak channel. | The ordering envelope has only generation, sequence, query ID, kind, status and view. The test confirms its JSON has no prompt, SQL or row field. |

## Verification

- Focused API, MCP, reporting, SDK and CLI tests pass with `CGO_ENABLED=0`; the
  MCP package also passes with the local native toolchain. The focused foundation
  rendition-startup test passes; the complete foundation package requires the
  separately configured real PostgreSQL test store.
- `go vet` passes for the touched packages with `CGO_ENABLED=0`.
- AGENTS.md and CLAUDE.md remain byte-identical; `git diff --check` passes.
- The repository-wide lint baseline still reports existing Phase 32/33 findings
  outside this diff. The local planning check also hits the existing macOS
  `/var` versus `/private/var` temporary-path assertion in coverage-gate unit
  tests; the plan/reference checks before that point pass.

This review proves the checked source contract and focused runtime behavior. It
does not claim a deployed host, live external MCP discovery or a standalone
authoring application.
