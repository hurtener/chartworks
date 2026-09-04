# Governed reporting — implementation entry point

The owner merged the research in PR #2 and clarified the boundary: Pengui alone owns authentication/authorization policy; Harbor/Pengui MCP Apps compatibility is established; functional scheduling is retained and source stubs are discarded.

Start at [the active phase plan](../plans/README.md), then [RFC-001](../../RFC-001-Chartworks.md), [RFC-002](../../RFC-002-Governed-Reporting.md) and [the signed-authority contract](../contracts/pengui-authority.md). [Contracts](contracts.md), [delivery](delivery.md), [implementation](implementation-plan.md) and [review corrections](planning-review.md) now describe executable work, not a second proposed design layer.

[Brief 14](../research/14-reporting-parity-audit.md) preserves source evidence levels. Its 63 feature IDs and the earlier 40 gate IDs are assigned in the phase coverage map. CODE means inspected source, TEST means inspected test definitions, not executed tests. Historical proposals and the original detailed phase notes remain under `docs/archive/`; they are not current instructions.

All phases are initially planned. The first usable slice is not a complete migration; phase 34 closes functional parity and phase 25 closes release readiness.
