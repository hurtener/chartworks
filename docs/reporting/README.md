# Governed reporting — implementation entry point

Current owner boundaries: Pengui alone owns authentication/access policy and issuance; Harbor/Pengui MCP Apps support is established; functional scheduling stays and source stubs are discarded; all learned-model operations use the embedded Bifrost SDK with remote providers, never local models.

Read [the active phase plan](../plans/README.md), [RFC-001](../../RFC-001-Chartworks.md), [RFC-002](../../RFC-002-Governed-Reporting.md), [authority contract](../contracts/pengui-authority.md) and [model gateway](../contracts/model-gateway.md). [Contracts](contracts.md), [delivery](delivery.md), [implementation](implementation-plan.md) and [review corrections](planning-review.md) explain the concrete work rather than a second proposed design layer.

[Brief14](../research/14-reporting-parity-audit.md) retains source evidence levels and63 feature IDs. The current map links these and41 review gates to224 criteria across34 phases. Source CODE/TEST means inspected definitions, not tests executed in this migration. Historical plans/proposals are under `docs/archive/`, not active instructions.

Phase06 delivers the first real durable Pengui authority adapter; reporting schedules30 reuse it. Viewer31 can ship independently of schedules. Phase05 pins Bifrost and strict embedding/rerank behavior; retained viewing and frozen no-narrative work make zero model calls. All runtime phases remain planned;34 closes migration and25 closes release.
