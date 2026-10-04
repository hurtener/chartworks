# Reviewed-dataset chart preparation, version 1

This is the finite dataset-first continuation of [D-097](../decisions/2026-10-03-visual-chart-authoring.md).
Implementation and qualification are tracked separately in
[the evidence report](../reviews/report-app-dataset-preparation.md).
It does not add a query language, model-generated SQL or an identity service.

## Deliberate lifecycle

1. Read an authorized immutable topic and dataset projection. The projection
   preserves reviewed field identity, aggregation, unit and refusal reasons.
2. Stage logical fields and a chart mapping locally. Editing does not query data.
3. Explicitly prepare under a unique operation ID and exact new-block target.
   The server compiles closed intent, validates it through the existing native
   validator and performs a bounded read to observe actual output schema.
4. Consume the exact preparation digest into an unvalidated private block.
   Creation does not reuse preparation as approval or execute a second query.
5. Validate that exact draft through the separate native validation action.
   This is another deliberate bounded source read, never an automatic side effect.
6. Save the private revision/digest in a version-three report and explicitly
   preview through the existing private composition lane. Publication is separate.

No result rows or SQL enter the authoring response. Report edits, status reads,
retained redraw and page navigation cannot implicitly restart source work.

## Shared HTTP and MCP operations

All routes are closed POST DTOs below `/v1/reporting/authoring/v1/`.
MCP names are `reporting_authoring_<suffix>_v1`; the SDK uses the same types.

- `dataset`: retained reviewed-field metadata; native `topics.read` and exact
  topic/source/dataset/context reach.
- `prepare_chart`: explicit bounded source work; native `reporting.validate`,
  chart binding, tenant/new-target/topic/dependency and source-query authority.
- `preparation`: pure retained status by preparation ID or original operation;
  original actor, session, tenant, exact target and current dependency authority.
- `create_prepared`: native `reporting.write`; exact custody digest and fresh
  authority/pin fences, consumed atomically with native block insertion.
- `preparation_control`: explicit inspection/cancellation/reconciliation of an
  existing native source attempt. It is a mutation-capable, open-world operation,
  never marked read-only and never used to start a replacement query.

Every operation rechecks its authoritative native domain contract. Resource IDs,
UI modes, host profiles and the metadata projection never grant authority. Pengui
allocates/authorizes new targets and keeps credentials outside the iframe.

The assembled registry has 83 default tools, or 88 with all five optional
rendition operations, within the existing 96-entry metadata ceiling. Per-schema,
aggregate response, request, concurrency, execution and renderer bounds remain
unchanged. Bootstrap guidance stays bounded rather than dumping the entire catalog.

## Finite compiler and custody

The first compiler supports PostgreSQL, zero to two direct reviewed dimensions,
one unfiltered reviewed measure, and genuine KPI/table/bar/column/line/area/pie/
donut mappings. It rejects unsupported rule, group-domain, population,
completeness and calendar policies explicitly. No joins, arbitrary expressions,
client SQL, client schema or caller-selected aggregation are accepted. Server
field IDs bind the actual schema; truncated preparation results are rejected.

An immutable operation/input digest is reserved before source work. Identical
replays inspect existing custody, while changed inputs conflict. Unknown or
active attempts retain their liability until native controls establish their
state. Expiry prevents consumption; it does not erase an unresolved attempt.
Current actor/session/tenant/target and source/topic/policy fences are required
before source work, payload projection and atomic consumption. Prepared-origin
rule-absence provenance must survive edits/copies/restores, preventing a later
rule activation from being bypassed by validating an older no-rules draft.

Custody is bounded by tenant and actor record/byte quotas with conservative
reservation charges. Automated terminal-record cleanup is not implemented yet;
capacity exhaustion fails closed. Unknown liabilities must never be purged merely
because the consumption TTL has elapsed.
