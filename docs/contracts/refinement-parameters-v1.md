# Refinement parameter custody v1

Status: AP-05A implementation in PR #62. This is a bounded parameter-continuity
slice, not complete conversational interpretation or parameter editing.

## Retained values and the authorized parent

Refine reloads the exact parent under the signed actor/session scope, reauthorizes
its dependencies and replays its reviewed business constraints. For SQL-bearing
parents it also reconstructs and verifies the owned binding receipt before using
its unbound base. Reference-only change receipts do not replace valid SQL with an
empty base. Existing revision/lineage-digest fences still guard child insertion.

Only original model-authored parameters are retained for SQL editing. When a parent
has owned predicates, `BaseSQL`/`BaseParameters` provide that boundary; the final
bound SQL and the owned scalar values do not become editing context. The model
receives previous SQL and positional parameter kinds, not their values. A private
request-local context value holds the exact retained bindings. No public request
field, stored token, schema, migration or alternate identity is introduced.

After structured output validation, the service requires the same slot count and
kinds and restores the original private values before business binding and native
validation. Placeholder output values are deliberately ignored. The same check
runs after the existing one validation correction. Replacing a reviewed typed
answer can rebuild its owned predicate while preserving independent model slots.
Parent/current known private values continue to be redacted from explanatory text.

## Parameter roles, not only parameter count

For PostgreSQL, a bounded native-AST comparison requires one top-level SELECT,
the exact original source/alias namespace, and unchanged complete clauses that
contain parameters. Only parser locations are ignored. The count, positions,
comparison operators, Boolean structure and literal context cannot silently
change. SELECT/GROUP/ORDER edits outside parameter-bearing clauses are possible;
the entire edited query still passes normal native and analytical validation.
A parameterized LIMIT retains its companion LIMIT/FETCH option. Literal strings
or comments containing `$1` are not parameters.

This check cannot issue a native plan, grant authority or certify the full intent
of the edited query. Nested scopes, joins, windows and set operations are not
qualified for changed parameterized refinements. Other dialects accept unchanged
SQL with preserved bindings only; changed SQL requires a future native role proof.
They do not fall back to lexical guessing. Invalid continuity fails before a new
query is persisted or executed. No third correction or silent binding reset occurs.

There is intentionally no free-text mutation of private model slots in this slice.
A changed free-text question on a parameterized parent is rejected before new
model work instead of assuming that its old scalar values still answer it. The
same-question path can use explicit structural edit instructions or existing
typed reference/metric edits. Reviewed owned-answer edits remain separate. A
newly routed Plan is required for new free-text filter/value intent until a typed
model-slot editing contract is available. Parameter-free Refine remains unchanged.
Changing a retained parameter's value/type/position, adding/removing a slot, or
rewriting a bound clause requires a newly routed question until an explicit typed
parameter-edit contract is implemented. The existing reviewed answer edits remain
separate and supported. Parameter-free edits keep their existing generation path.

## Replay, compatibility and evidence

The final child uses the existing protected SQL/parameter fields. Run and terminal
replay use those values with ordinary current/retained source validation; neither
needs the ephemeral state or a model call. Saved projections retain the bindings
without exposing raw result rows. No old query is rewritten. The public registration
schema must not accept internal binding state. Frozen report refresh is unchanged.

Tests cover native clause ownership, exact source-record results after restart,
EN/ES follow-ups, private values absent from provider requests, value restoration
through correction, mixed model/owned parameter replacement, parent evidence
rejection and revision fences, input/output detachment and concurrent native use.
Exact executed results belong in the PR and recovery evidence, not this inventory.
Inferred scalar/time inheritance, grouping edits, mutable model-slot operations,
parameterized learning and general query-wide conformance remain open.

## Explicit same-kind replacement extension

[Parameter edits v1](refinement-parameter-edits-v1.md) adds the optional private
`parameter_edits` list on Refine. Model-owned values can now be replaced explicitly
without changing their kinds or SQL roles. The unchanged source/clause proof,
parent revision/lineage fence and owned-parameter separation still apply. The
original no-edit behavior and changed-free-text rejection remain. This supersedes
only the earlier absence of a typed model-value edit, not the broader continuity
or dialect limitations above.
