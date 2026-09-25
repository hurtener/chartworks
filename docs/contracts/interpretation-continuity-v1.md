# Retained inferred values and time windows

Status: S8 implementation checkpoint in PR #62. Exact runtime qualification is
recorded in the completion tracker. This extends existing deterministic value/time
interpretation, typed business binding and parent lineage; it is not a second SQL
executor, an authority snapshot or unrestricted conversation interpretation.

## Retained intent and precedence

`interpretation_selections` contains at most 64 reviewed dimension selections. A
selection is either a reviewed **non-sensitive governed value ID** with eq/ne, or a
half-open civil `period` with start/end/grain. It contains no SQL, source grant,
private scalar literal, timezone override or old physical binding. Current source
admission and the current reviewed column/calendar policy resolve every selection.
Invalid, foreign or sensitive-value IDs fail before embedding/generation.

Refine first verifies the parent actor/session, exact source/semantic admission and
existing business replay. It then derives a detached selection snapshot from the
parent's resolved Interpretation. Exact absolute civil boundaries and the original
anchor survive omitted words and service reconstruction. The child's ordinary
Route request persists these choices; native and analytical execution gates remain
unchanged. A saved selection is intent, not permission or a claim of past approval.

New explicit language replaces the retained selection(s) for the same dimension.
Other dimensions survive. A supplied typed selection replaces the parent's baseline
for its dimension. Explicit interpretation edits remove/replace exact value/time
targets; temporal replacement uses an aligned, increasing `period` instead of a
value ID. Current publication policy still decides permitted grain/calendar/zone.
An old utterance's edits are not blindly applied to a different new utterance; the
already-resolved parent state is the baseline. Same-question refinements keep those
edits so old words cannot resurrect a removed filter. Grouping is a separate S9
obligation, not inferred from every filter dimension.

The existing deterministic parser remains unchanged for retained requests without
selection snapshots. Continuations additionally recognize this/last quarter and
year (English/Spanish) against the retained anchor. This does not advertise general
fresh-query date-language support. Conflicting periods remain clarification errors.
No recognized change means retention, not inference of an unspecified new period.

## Surfaces, replay and privacy

HTTP/MCP use the existing closed domain request and generated schemas. SDK aliases
expose the same selections, periods and edits without implementing interpretation.
Saved routing carries explicit selections/edits/anchors rather than silently
replanning an abbreviated question without its filters. A pending answer context
cannot be reused with a changed selection/edit/anchor set. Current parent lineage,
source-revision checks and immutable analytical proof checks remain in place.

The router rebuilds canonical values and physical date bounds under current signed
reach before producing sealed business constraints. Persistence does not issue a
seal. Replay reconstructs the same selector state and compares the existing hashes;
a tampered seed cannot substitute a different filter under the old receipt.
No provider or source execution is added to terminal replay or frozen reporting.

Sensitive clarification values keep their separate existing reviewed-answer custody.
They are not promoted into public governed-value selections. Model-owned parameter
rules remain intact: a changed free-text question cannot silently retain/repurpose
those private slots. Same-kind explicit parameter edits keep their existing API.
No migration or analytic receipt-version rewrite is required; absent selection
fields retain historical wire and replay behavior.

## Completion evidence

The new unit/SDK fixtures cover exact dates/anchors, eq/ne coexistence, replacement,
removal, re-selection, cloned intervals, invalid unions and ranges, sensitive/foreign
IDs, source rotation, replay substitution, cancellation and saved-state forwarding.
The two PostgreSQL/recorded-provider acceptance tests cover EN/ES multi-turn results,
restart, original parent immutability, exact lineage/intervals, saved result privacy,
zero-work replay, typed date edits and cross-session denial. Tests must actually run;
this inventory is not green evidence. General paraphrases, grouping changes,
new scalar domains and live owner qualification remain separately tracked.
