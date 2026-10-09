# Physical-column filter core qualification

D-108 extends the native typed compiler and shared parameter/report core. The
Builder UI and physical option-search integration remain unfinished. This is a
single-agent implementation and adversarial self-review, following the owner's
explicit no-subagent instruction. No deployed or hosted-CI claim is made.

## Local evidence

- Full `internal/reporting` and `internal/reportingapi` tests pass, including race.
  Declaration tests cover exact large integers/decimals, text/UUID/boolean sets,
  numeric/date/civil/instant ranges, mixed-union refusals and old JSON bytes.
- Compiler/SQL tests cover physical fields in both topic and source origins,
  forged types/columns/context/schema, OR/negation, casts, parameter reuse,
  wrong range operators, source drift and cross-context report bindings.
- Real PostgreSQL `TestReportAppColumnFiltersNative` uses no topic permissions.
  Schema-checked HTTP preparation, private consume, changed dependency authority,
  invalid native amendments, validation/publication, retained exact values,
  report-default bindings and a temporary range override pass. The override
  crosses a 23-hour calendar day in an explicit named zone. Publication remains
  immutable after amendment and original custody survives cleanup/compaction.
- `TestReportAppTypedFieldsNative` adds physical text/numeric/instant filters to
  each of two unrelated renamed synthetic schemas. Both prepare, consume and
  preview the expected actual source population without reviewed dimensions.
- Existing source-origin and reviewed-filter journeys pass. Phase29/AC06 now
  maps the physical filter lifecycle alongside its legacy binding acceptance.
  Phase22/AC03 and Phase23/AC02 registry/surface checks pass.
- Existing report-app JavaScript/resource contracts and planning checks pass;
  AGENTS.md and CLAUDE.md remain byte-identical. UI assets are unchanged here.

## Review boundary and remaining work

Self-review checked that the new optional fields preserve old canonical bytes,
that type declarations are checked against actual source metadata, and that the
native author/import path cannot relabel another column or weaken predicate
semantics. Text values remain parameters, including SQL-looking content. Report
filter compatibility now compares the entire column pin and temporal policy.
No concrete P0/P1 finding remains in this native change.

Native catalog capability advertises physical filter kinds but explicitly leaves
`option_lookup=false`. Physical controls, option lookup custody, host proof and
visual acceptance remain required next steps. The broader audience-management,
upload and final author-to-reader-to-amendment goal remains active. The previously
recorded Phase23/AC01 analytical error-registration issue remains open and is not
covered by this pass. Hosted CI remains billing-blocked.
