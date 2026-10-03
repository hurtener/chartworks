# Captured query variants

A `captured_variant` query widget references an exact independently published
captured-block revision. It is distinct from dynamic replayable questions and
private session-bound query widgets. It uses the existing frozen execution
engine and review/publication lifecycle, not a second SQL mutation engine.

## Workflow

1. Run an authorized query, then use the existing capture operation to create a
   private draft. Planned/failed, foreign-session and SQL-inspection-denied
   queries cannot be captured. Capture seals private content provenance.
2. Declare typed existing slots, or explicitly apply the supported AST-proved
   period parameterization proposal. Validate and publish the resulting block
   through its ordinary explicit lifecycle. Capture alone is not approval.
3. POST `/v1/reporting/query-variant` (`prepare_captured_query_variant` in MCP,
   `PrepareCapturedQueryVariant` in the Go SDK) with `block`, exact `revision`
   and optional `outputs`. It returns a report-ready `query` payload and the
   reviewed parameter catalog. It returns neither SQL nor source-query IDs.
4. Use that payload on a `kind=query` report widget. Its `bindings`, `literals`
   and allowed `overrides` use the existing typed report parameter resolver.
5. Publish the report and run with concrete report filters. Each execution
   rechecks current block/source/topic/rule reach, exact revision/digests and
   runtime values. Changing values changes accepted execution identity without
   modifying the source query or generating new SQL.

The reference pins block identity, revision, definition digest and capture
provenance digest. It has no floating revision or caller-supplied source/SQL
fields. Document commit repeats source-backed block eligibility and provenance
checks. Composition stores the variant reference separately while executing the
existing frozen block group; its accepted identity includes the exact variant.
A source query's identity never substitutes for block execution permission.
Older captured revisions without the server-sealed capture digest cannot be
prepared as variants; they remain usable through their existing block contract
or can be explicitly recaptured and reviewed.
Published captured content is an independent authored definition: its ordinary
retention lifecycle remains separate from the originating private query.

## Scope

Only already-reviewed parameter slots can be bound. Arbitrary new report
predicates, model-invented filter columns, changing SQL at refresh, and converting
an unreviewed dynamic question into a public variant are unsupported. Dynamic
query widgets retain their existing behavior and still cannot accept these
bindings. Explicit output definitions are retained; refresh does not invoke
chart selection or other model inference. Scheduled report execution uses the
same pinned frozen path; a saved-question dynamic target is not a variant.

PNG/PDF export and richer narrative reasoning are independent features. This
contract does not claim whole-product reporting qualification.

## Qualification boundaries

Recorded-provider tests with actual PostgreSQL cover capture, explicit period
parameterization, publication, HTTP/MCP/SDK discovery, changed report values,
current-source rejection, scheduling pins and exact deletion replay. These are
bounded variant-consumer results, not a claim that all reporting is qualified.
The reporting recovery workflow requires the variant/deletion/renderer cases and
invokes the existing actual-browser contract on a supported hosted runner.

Repeated isolated-worker experiments still expose intermittent Go runtime
allocation failure under the existing 1 GiB virtual-address ceiling, including
when one whole-suite iteration passes. Fixed single-CPU execution does not
resolve virtual-address reservations. The ceiling remains unchanged; a different
memory-accounting contract requires an explicit decision and deployment proof.
A locally denied Chromium IPC socket also prevents local browser qualification.
Neither limitation may be converted to a silent skip or a renderer-stability
claim. Hosted browser results and any future memory-policy decision must be
recorded separately.
