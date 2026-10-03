# Reviewed amount completeness

This optional reporting contract preserves missing-amount disclosure without
claiming that an edited block retains its source query's analytical proof.
Existing definitions and outputs with no declaration keep their existing wire
and digest behavior. Absence means no amount-completeness assessment.

## Authoring and authority

A successful query capture first repeats the current native and analytical
output check. Only proof-issued metric/output ordinals seed a proposed private
`amount_completeness` declaration; aliases and labels never identify the amount
or its companion count. No historical count values enter the declaration.

Each declaration binds an ID, reviewed label and metric identities to exact
ordered expected fields, including their names, types, native types and encoding.
Outputs explicitly bind declaration IDs with `amount` or `unknown_count` roles.
Capture proposes these bindings in an unvalidated draft. Ordinary explicit
validation/publication is required, including after SQL or parameter edits.
Publication, current source checks and signed resource scopes remain unchanged.

Published block results use evidence origin `reviewed_definition` and the exact
published definition digest. This is the reviewer's declared business meaning,
not a claim that the original analytical receipt survived amendment. Dynamic
query results retain `analytical_receipt` evidence from their owning query run.

An ordinary whole-definition edit cannot accidentally erase an existing
declaration. A mapped declared amount/counter cannot omit its output binding.
Unknown-count KPIs and tables are supported explicitly, with count units and no
currency/percentage formatting. Amounts and counts cannot occupy interchangeable
value/comparison/target slots of one series or KPI. Independent geometric slots
retain their declared roles. Labels do not grant display or source authority.

## Results and row identity

Each successful retained output contains immutable disclosure metadata, bound by
its content digest. Counts are exact nonnegative integers. Positive counts mean
`incomplete`; zero means `complete` within the declared returned query population.
Null, malformed, negative or fractional counts, missing schema evidence and
truncated results yield `unknown`. Missing amounts remain NULL; no zero is
invented. Empty complete populations have no synthetic result rows.

Amount completeness is separate from chart/result transport completeness. Its
aggregate scope is `returned_query_rows`, never a source-wide coverage claim.
Sorted/paged tables retain original source-row ordinals, including stable ties
and NULL rows; values are never used to guess row identity. A visible page's
row disclosures use `visible_source_rows` while the aggregate status continues
to describe the returned query population. A page containing zero unknowns
cannot rewrite an incomplete aggregate as complete.

Legacy chart building is unchanged. Completeness-enabled reporting asks for the
explicit source-row companion builder; it checks alignment and existing byte
bounds. Null-omitted non-table points retain their existing source ordinals.

## Consumers and exports

HTTP, MCP and SDK retained views expose the same typed disclosure. The bundled
viewer and static HTML/SVG show the distinct evidence origin, scope, exact
unknown count and any truncation warning. Count outputs remain visibly counts.
A viewport too small for required static disclosure fails rather than clipping
it away.

CSV adds bounded per-row status/count/evidence/field-identity columns. It maps
counts through retained source ordinals even after sorting and pagination.
Header-only CSV contains no amount rows and is not a complete standalone
coverage disclosure: overall evidence/status is carried by the retained
rendition envelope's `projection.amount_coverage`. No fake data row is inserted.
Raw-report access does not grant model-evidence access. Narrative generation
continues to enforce the existing field sensitivity/redaction policy; retained
disclosure metadata is never appended to model prompts.

Every export format carries this content-free coverage summary; JSON also
retains the full typed output. Row values are not copied into the summary.

Renderer input/output, namespace, memory and timeout limits are unchanged.
Functional disclosure tests do not qualify a previously failing worker runtime
or a browser that cannot start in its execution environment.
