# Governed option dependencies

### D-103 — Original lookup custody and bounded target discovery

Accepted integration scope, 2026-10-05. Pengui consumes native metadata for
governed option search, status and control. Original tenant/actor/login/target
custody takes precedence over current definitions; missing status/control custody
never falls back to a query. New searches reuse complete native publication/report
closures. Independent current policy remains required for every action/resource.
No source work, option values or browser credentials enter discovery.

See [the dependency contract](../contracts/report-dependencies-v1.md). No database
migration or MCP tool is added. Actual provider/issuer/browser evidence is recorded
separately; restricted no-chat MCP remains outstanding.
