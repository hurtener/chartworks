# External-agent context and SQL contract (version 1)

Phase 19 composes the phase-17 router, reviewed topic/rule publications and the
phase-09/10 validator/reader. It does not add an agent runtime, SQL-generation
loop, local capability issuer or alternate executor. The operation manifest is
[chartworks-byo-operations.json](chartworks-byo-operations.json); the composed
runtime `/openapi.json` is the closed wire-schema authority.

## Operations and authority

| HTTP operation | SDK method | Required action |
|---|---|---|
| `POST /v1/nlq/contexts` | `GetQueryContext` | `query.context` |
| `POST /v1/nlq/contexts/read` | `ReadQueryContext` | `query.context` |
| `POST /v1/nlq/sql` | `SubmitSQL` | `query.submit` |

Every operation requires a current verified Pengui JWT. In addition to its
operation action, context access requires `topics.read`, `sources.read` and
addressed `cw.topic.read`, `cw.source.read`, `cw.dataset.query` and
`cw.execution_context.use` reach covering the selected data. Submission also
requires `sources.query` and addressed `cw.source.query` reach. The exact issuer
vocabulary and wildcard semantics remain in [Pengui authority](pengui-authority.md).
There is no token/signing-key setting for bundles.

Context-only authority is intentionally useful without `query.submit` or
`sources.query`. Source metadata is read through `sources.Service.ContextBinding`;
the shared validator still uses `Binding`, which requires query authority before
native SQL planning. Adding a submit action cannot manufacture missing data reach.
A separately authorized submit-only call can use an existing context without the
`query.context` action, provided its captured data reach is unchanged.

New context construction is advertised only when the real router is configured.
Already stored references remain readable/submittable without the router or any
model dependency; the real source/validator/reader and PostgreSQL store must still
be available. Disabling new construction does not waive any current authorization.

## Exact context, not a bearer capability

Creation accepts `schema_version: 1` and a phase-17 `route` request. Clarification
or no-route outcomes contain no bundle and must be handled explicitly. A bundle
contains a cryptographically random opaque `bundle_id`, execution `context`,
creation/expiry timestamps, exact topic/rule version and digest pins, source,
semantic context, explicit SQL requirements, step ceiling, context model usage,
warnings and provenance. Its reference is the tuple `schema_version`, `bundle_id`,
`context`; possessing it grants no authority.

SQL requirements specify the native dialect/catalog, parameter style and allowed
kinds, SQL/parameter/row/byte/time bounds and the intersection of reviewed semantic
datasets/columns with the current source binding. Mandatory constraints stay
explicit even when advisory context is pruned. Caller-provided examples are labeled
`external_input_unreviewed`, never promoted to reviewed examples or certifications.
The contract supplies business constraints; SQL safety validation is not a claim
that arbitrary external SQL faithfully implements every business interpretation.

The private stored record also binds tenant, user, session, complete source binding
and captured data reach. Each lookup/submission rechecks all coordinates, the
current data reach, exact publications and source revision. Widening or narrowing
that reach, rotating source bindings, publishing/retiring rules, archiving topics,
or reaching expiry requires new context. Current semantics are never silently
substituted. Refreshing a JWT alone does not extend bundle expiry. The stored
snapshot cannot be updated.

A syntactically valid absent, foreign, expired or stale reference returns HTTP 409
with `replan_required`, without exposing which private coordinate differed.
Malformed request bodies and incompatible versions return 400; missing operation authority
returns 401/403 as appropriate. Bundle/step ceilings return 429 with
`bundle_budget_exhausted`. Unknown request fields, duplicate fields, URL query
parameters and content encodings are rejected. Identity, source, dialect and
resource limits cannot be overridden in a submit request.

## Explicit steps and truthful retries

An external agent first obtains context, chooses its own next step, then submits
one SQL statement and typed parameters with a stable `operation` identifier. For
example, using the actual reference and the relation/parameter names returned in
the bundle:

```json
{
  "schema_version": 1,
  "bundle_id": "<returned lowercase 64-hex bundle_id>",
  "context": "<returned context>",
  "operation": "analysis-step-1",
  "sql": "SELECT id FROM analytics.sales ORDER BY id",
  "parameters": []
}
```

The example SQL is illustrative, not permission to access that relation. Source,
dialect, dependencies and columns are derived from the captured bundle and passed
through the same enumerated validator/native-planning checks as ordinary generated
queries. Publication pins are checked again after planning and before dispatch;
the ordinary source and execution fences remain in force.

A step is transactionally reserved before validation. Rejected SQL therefore
consumes one step, as does a dispatched query. The step deadline is bounded by
bundle expiry, the current JWT, the request context, configured step timeout and
the reader limits. Each distinct operation invokes at most one explicit execution
attempt; Chartworks neither generates a correction nor retries a warehouse query
on behalf of an external agent.

Reusing an operation with identical input returns its existing receipt without
running SQL again. Reusing it with different SQL/parameters conflicts. A duplicate
while work is still accepted is not a new worker; after the accepted deadline an
unresolved attempt is reported as uncertain rather than retried. This is duplicate
suppression, not a guarantee of exactly-once effects across every remote failure.

An HTTP-successful submission can contain a rejected, failed or uncertain receipt.
Clients must inspect `step.status`, `step.code`, `replayed` and `values_available`.
Only the original successful response can contain transient result values;
replays return `values_available: false` and never claim to have retained those
values. A failed receipt commit suppresses result delivery. The client may read
context/step history or retry the same operation to reconcile evidence, but should
not choose a new operation solely to disguise an uncertain earlier attempt.

Stored step evidence consists of operation/number, input and bundle digests,
semantic version pins, timestamps, fixed status/code, execution-attempt evidence
and `model_calls: 0`. It does not retain submitted SQL, parameter values, result
rows or bearer tokens. Creating context may use the configured routing gateway;
lookup and SQL submission do not invoke models. The caller owns every further
analysis step and explicit new context request.

## Limits, retention and verification

See the `query_bundles` table in [configuration](../configuration.md) and the
[configuration excerpt](../../examples/chartworks.byo.json). Bundle and step
admission are serialized within PostgreSQL transactions, including audit writes.
A failed audit commits neither a bundle nor a spent step. Terminal step evidence
is sealed once; snapshots and pinned provenance cannot be rewritten.

Retained expired bundles continue to count toward tenant/session quotas until
cleanup. Admission performs bounded tenant-local cleanup of up to 64 records whose
retention deadline has passed; the normal retention sweep also removes due bundle
evidence. Deleting a retained bundle cascades its step receipts. Expiry ends use;
retention bounds how long its evidence is kept, not how long authority lasts.

`TestPhase19/AC01`–`AC06` cover versioning, authority, shared-validator parity,
HTTP/SDK interoperability, duplicate suppression and explicit replanning.
`TestBYOStoreFencesAndAudit` additionally checks real-database audit rollback,
immutable evidence, concurrent quotas, terminal replay and tenant-scoped cleanup.
The [review record](../reviews/phase-19-byo-mode.md) distinguishes these checks from
live-provider qualification and the still-planned full-release gate.
