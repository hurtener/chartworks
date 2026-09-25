# Explicit refinement parameter replacements

Status: AP-05B implementation in PR #62. Extends [parameter custody](refinement-parameters-v1.md),
not general free-text parameter inference or full conversational interpretation.
The existing `refineNLQ` operation / `POST /v1/nlq/refinements` accepts an optional
`parameter_edits` list. No new operation, model role, permission or migration.

## Typed wire contract

```json
{
  "query_id": "existing-private-query",
  "parameter_edits": [
    {"position": 1, "replacement": {"kind": "number", "value": "20.125"}}
  ]
}
```

Positions are one-based within the original **model-owned unbound base**. They are
not positions in the final parameter list after service-owned predicates have
been appended. Each position can appear once, at most 64 edits are admitted, and
the replacement must be a valid value of exactly the existing kind. Values use
strings so decimal and integer precision is not lost through JSON floating-point
numbers. Text must be valid UTF-8, with the existing parameter limits. Null can
only replace an existing null slot; this operation does not introduce a nullable
kind union. Duplicate positions, nonexistent slots, malformed values and type
changes fail atomically without creating a child. A parameter-free or pending
parent cannot acquire new model slots through an edit.

This is replacement only: it cannot remove/add/reorder slots or change their
column/operator/Boolean role. The original PostgreSQL clause/source-identity proof
still applies; other dialects retain their unchanged-SQL restriction. No lexical
comparison is substituted for a native proof. SQL outside the parameter-bearing
clauses still requires complete native and analytical validation.

## Private values and trusted state

The service first reloads the exact parent under signed tenant/actor/session
scope, checks dependencies and its protected binding receipt, then separates
model slots from owned filters. Replacements are copied into request-local private
state fenced by the observed parent ID, revision and lineage digest. They cannot
be supplied directly as trusted state or inherited from a client-provided plan.
The existing transactional child insert rechecks the parent. The parent itself
is never changed.

Only slot positions and kinds enter the editing instructions; this operation
adds neither original nor replacement values to model context. Returned placeholder
values are discarded and the server-selected bindings are restored before owned
predicate binding, native validation and analytical checks. The same selected
values survive the existing one validation correction. Ordinary typed edits do
not create another model call or correction allowance beyond Refine's normal path.
Caller-written question/hint text is still caller text, not a general DLP boundary.
Ordinary logging of the edit value is redacted; JSON remains the private input
transport, not a public receipt.

Owned answer replacement stays on the existing reviewed clarification path. It
can be combined with replacement of an independent model slot. Selecting an
owned slot via its index in the final bound parameter list fails rather than
bypassing review. Known parent/current values continue to be redacted from
accepted explanations. These are known-literal safeguards, not general DLP.

## Interpretation and durable compatibility

The unchanged original question remains the lineage's text anchor. Typed edits
explicitly override the values of its retained slots, not the meaning of the SQL
around them. A changed free-text question remains unsupported when retaining
model slots, even when typed replacements are present: it could request a new
field or Boolean rule, which this API does not infer. Use a fresh Plan for that
case. Existing structural instructions and typed semantic/reference/owned-answer
edits can still operate within their established checks.

The accepted child stores its effective SQL/parameters and, when present, updated
owned-binding/analytical evidence in the existing protected fields. A subsequent
refinement starts from that child, not the grandparent's values. Saved reads and
terminal replay need neither the original request object nor a new provider call.
Historical rows and no-edit requests retain their behavior. Parameters are not
added to public Plan/Run responses and SQL-inspection permission is unchanged.
The generated closed request schema exposes the field only on Refine; no duplicate
HTTP/MCP/SDK/CLI business implementation is introduced.

## Tests and remaining scope

New unit/schema checks exercise scalar kinds, exact large decimals, Unicode,
invalid/duplicate/out-of-range edits, input immutability, private-state logging,
parent fences, correction, clause-role rejection, owned-slot rejection, concurrency
and cancellation. Real PostgreSQL/recorded-provider acceptance covers EN/ES restart,
exact record sets after repeated replacements, original-parent immutability,
saved-result privacy, zero-work replay, typed owned-answer replacement and denied
actors/sessions. Exact executed results belong in the PR and recovery logs.

This does not implement inferred value/time inheritance, field-role remapping,
free-form question updates, new parameter types, joins, unsupported SQL shapes or
live-model qualification. AP-05A's bounds stay intact except for the explicit
same-type value replacement authorized here.
