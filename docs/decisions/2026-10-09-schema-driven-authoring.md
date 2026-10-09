# Schema-driven manual authoring

### D-106 — Explicit typed fields without assumed business meaning

Accepted implementation scope, 2026-10-09. Extends deterministic manual chart
preparation without changing issuer ownership, private custody or publication.

Clients supply unknown schemas and analytical questions. The production authoring
contract derives choices from current authorized metadata, never names such as
revenue or region. Synthetic fixtures may illustrate a domain; the implementation
must work when its physical columns and display names are changed independently.
Optional starting layouts describe placements without selecting calculations.

The additive `typed-dataset-postgres-v3` compiler accepts ordered physical or
reviewed groupings and multiple explicit measures. Physical aggregation follows
actual native types; reviewed measures retain their aggregation and policies.
Count is an explicit source aggregate, not an invented reviewed semantic field.
Raw-row tables preserve duplicates. Date grouping uses an explicit calendar,
grain and, for instants, timezone. Numeric years and text periods are not coerced.
Configured execution and rendering budgets replace the previous analytical shape
restriction; they remain visible limits, not permission to truncate intent.

Legacy v1/v2 requests omit the new fields and retain their canonical hashes.
Preparation still observes actual schema through the existing validator and
execution ledger. Consume, validation, preview and publication remain distinct.
No new operation, identity policy or persistence migration is required for the
topic-backed compiler. Current unsupported policies retain explicit refusals.

Private predecessor review informs two neutral patterns: physical field identity
is separate from reviewed meaning; loaded-table registration carries a versioned
schema and recoverable operation identity. Domain alias lists, name-based type
guesses, implicit analysis choices and silent aggregation fallbacks are excluded.
No private source code or schema is incorporated.

The wider [flexible authoring plan](../plans/flexible-report-authoring.md) also owns
topic-independent tables/uploads and Pengui access management. Those are pending;
the topic-backed compiler does not establish their lifecycle or authority.
See [the typed contract](../contracts/typed-field-authoring-v3.md) for implemented
behavior and its explicit qualification boundary.
