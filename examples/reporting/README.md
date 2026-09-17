# Native reporting output intent example

`output-intent-v2.json` is one complete typed `BlockOutput`/`reporting.Output`,
not a runnable block or an authorization grant. Its data and labels are synthetic.
Include it in an otherwise valid definition with `schema_version=2`; its permitted
`total` and redacted `record_id` fields must exist in the reviewed expected schema.
The configured narrative model/prompt versions must match deployment. Normal
source/context authority, validation and publication remain required.

Authored block/widget/request query limits use the shared `BlockQueryLimits`:

```json
{"max_rows": 100, "max_bytes": 65536, "timeout_ms": 10000, "query_attempts": 1}
```

These are narrowing caps, not scan/cost guarantees. Omitted output selection uses
enabled defaults on v2; `[]` rejects; `["monthly-summary"]` requests this output
explicitly. Set `enabled=false` to retain its ID/labels while preventing execution.
Neither metadata nor manual allowlisting can override reviewed sensitivity.

Use `ReadBlockSQL` with existing SQL-plus-read authority for exact native authoring
export, then `MigrateBlockDefinition` for v1 inputs and normal create/edit for
imports. Ordinary block/viewer metadata is a projection, not an export. This
example introduces no endpoint, issuer or standalone authoring surface.
