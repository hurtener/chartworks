# Grounded grouping intent v1

## Consumer and configuration

The normal NLQ Route, Preflight, Plan and saved-question inputs accept
`grouping_intent_policy: "grounded-v1"`. It is an explicit opt-in, independent of
`concept_policy: "grounded-v1"` (metric/concept selection). Neither is enabled by
default. An application that wants both automatic metric selection and grouping
from a rich question supplies both policies, leaving references, metric IDs and
`grouping` absent. The existing `clarify` gateway role supplies these bounded
structured-output operations. No new provider, identity authority or endpoint is
introduced. SDK exports `NLQGroundedGroupingIntentPolicy`.

An explicit `grouping` with policy `reviewed-grouping-v1` takes precedence and
clears the effective grouping-intent policy. That grouping selection makes zero
model calls. Default deterministic routing and its call costs remain unchanged.
Each automatic producer uses one gateway operation with a two-attempt, 30-second
budget, under the configured clarify role's input/output caps and actual strict
provider schema reservation. Retries consume the same budget. Enabling both
policies may add two operations; there is no hidden enablement or fallback.

## Admitted choices, proof and refusal

Only current admitted publications contribute candidates. Each choice is either
an explicit scalar total or one reviewed dimension with its exact declared
calendar grain, calendar and timezone. Supported grains are day, month, quarter
and year under Gregorian policy; filtered dimensions are excluded. Omitted roots
are excluded. The complete catalog is bounded at 64 candidates and 128 KiB; an
oversized catalog fails rather than being truncated. Opaque SHA-256 identifiers
bind topic/version/publication, the current physical relations, and the exact
choice. Provider enums contain only these quote-free identifiers.

The model sees the complete catalog and redacted question. A selection must cite
unique, non-overlapping exact spans in that question. It cannot invent IDs,
calendar policy, SQL, predicates or values. Multiple grains for one dimension,
scalar mixed with grouping, repeated/ambiguous spans, or competing date bases
without a unique reviewed label in the selected span yield typed clarification.
Equal local dimension IDs in different topics stay distinct; colliding aliases
cannot be resolved by model preference. `no_match` is always non-executable;
scalar total is a distinct catalog choice, never the meaning of `no_match`.
Existing unsupported temporal/business requirements still run before this call.

This is structural attribution, not calibrated confidence or proof that a model
understood language correctly. The existing exact grouping compiler, analytical
conformance, native validation, source binding and signed authority checks remain
mandatory. No lexical suffix is stripped to force a query to pass.

## Retention and continuation

`grouping_intent` retains policy, input/catalog digests, the structural choice,
reviewed choice coordinates and exact reconstructed grouping. It joins the route
seal and the semantic-selection digest, including for scalar choices with no
dimension root. Retained execution reauthorizes current publications/source
relations and verifies the choice without a model call. Missing proof, changed
question, metadata drift, altered coordinates or stripped origin markers fail.
It is not an authorization token and no public request accepts a proof object.

A protected business-answer continuation may reuse an already selected grouping
through the existing actor/session-bound private origin path. An unresolved
`clarify` or `no_match` cannot be promoted to that origin. Its reviewed choices
are presentation coordinates: the operator starts a fresh Plan with explicit
reviewed `grouping`, retaining the original question if desired. An unresolved
proposal is not a business-answer form and its query ID is not a substitute for
an answer-context pin.

Unchanged refinement pins the retained reviewed grouping without re-inference;
changed language with no explicit grouping requests a fresh automatic choice.
An explicit replacement remains manual. Saved-question inputs retain the opt-in,
not the old model output, so a fresh saved plan uses current admitted bindings.
No migration is needed: optional fields are stored in existing protected route
JSON, and absent fields preserve old hashes and old routing behavior.

## Qualification boundary

The required regression roots cover rich English/Spanish and trailing prose,
scalar/manual/default costs, ambiguity and overlapping spans, current-source
changes, cross-topic identity collisions, replay stripping, protected origins,
saved/refinement behavior, and the ordinary HTTP/SDK Plan/Run path. The generated
six-row PostgreSQL fixture uses unchanged held-out questions and independent
scalar/month/status/quarter result oracles, without caller-selected metric or
grouping IDs. Recorded provider outputs qualify transport and lifecycle only.
Live language quality requires a separately declared cohort on the exact frozen
candidate. Wider time semantics, arbitrary grouping expressions, fiscal/DST and
all-source qualification remain outside this slice.

### Decision-bound provider shape

The grouping role's structured output is `{ "choice": { ... } }`, with a nested
select/clarify/no_match union. A select response must contain at least one admitted
ID/quote pair. Scalar intent requires exactly one pair naming the scalar catalog
card, even though the resulting reviewed grouping has zero keys. Clarify requires
at least two alternatives and no selections; no_match requires empty arrays.
The strict provider projection preserves those cardinalities; original domain
uniqueness and structural grounding still run before any consumer. This producer
shape does not change the public request policy or retained proof schema.
