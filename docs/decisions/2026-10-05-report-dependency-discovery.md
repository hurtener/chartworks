# Native requirements for central App authority

### D-098 — Exact-target metadata discovery precedes Pengui operation projection

Accepted integration scope, 2026-10-05; source implementation under local
qualification. Extends D-096/D-097 without changing identity or policy ownership.

Ordinary reads already require full dependency authority and therefore cannot
bootstrap its discovery. Add a separate metadata-only HTTP projection under the
new `reporting.discover` action plus exact target read and private preview/custody.
Only existing native dependency coordinates, revisions and digests are returned.
Pengui independently resolves every requirement before issuing actual operation
authority. Definitions, values, SQL and warehouse/model calls are excluded.

This explicit metadata boundary does not permit ordinary content access with
missing dependencies, a fabricated envelope, a wildcard seed, or caller-supplied
dependency lists. It is a BFF control-plane operation, shared by HTTP and eventual
restricted MCP delivery, outside the App tool inventory. No tool ceiling,
execution budget, provider-user database, issuer or grant grammar changes.

The first consumers are Pengui's exact SQL-free block-read and manual report
reopening projections. Full Builder
and Consumer acceptance remains open. See the
[contract](../contracts/report-dependencies-v1.md) for schema, limits and tests.

Report editing selection resolves the native draft/review pointer without changing
publication selection. Pengui report reopening independently checks operation read
and write, exact report read/write, App editor and all private-preview needs.
