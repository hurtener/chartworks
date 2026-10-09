# Integrated manual report journey: local qualification

2026-10-04. Source checkpoint `8108f4b538326140e1859dd80d6b86e363d68fde`,
tree `1220308f4550ceaf637aed7b4f4d90e891f350bc`. This checkpoint combines the
bounded dataset/filter option lane, shared generated presentation, trusted target
allocation client, and explicit chart/report publication controls.

## Qualified locally

- All 295 lightweight JavaScript tests pass. Nine evaluate the committed production
  IIFE in a constrained VM, including both MCP and registered embedded adapters.
  The publication journeys require separate confirmation for an entire chart
  revision/all outputs, selected-widget rebinding, report review, and report
  publication. They assert four metadata mutations and no source/model execution.
- The reproducible asset check passes: 217,194 JavaScript bytes, 17 authored modules,
  pinned esbuild 0.21.5. Production rendering shares the viewer's presentation
  implementation. Compiled resource tests enforce the unchanged 262,144-byte HTML
  limit, exact CSP hashes, no runtime code generation, and no remote imports.
- Focused native lifecycle race tests pass (3.149 seconds). The actual PostgreSQL
  manual publication lifecycle passes under the race detector (17.028 seconds).
  This covers private-preview preservation, publication authority/evidence fences,
  exact revision rebinding, failed report CAS after successful chart publication,
  imported empty defaults, review discovery/reopen, and independent newer drafts.
- Review-return history is derived from the existing native rejection event and
  survives a fresh service instance. A rejected revision cannot be resubmitted
  unchanged; a new CAS amendment restores eligibility. A newer draft is preserved.
- Earlier native option and filter qualifications remain documented in
  `report-app-option-custody.md` and `report-app-filter-integration.md`. Their source
  query limits, current-authority checks, unknown-operation custody and explicit
  Search admission have not been widened by publication controls.

## Remaining proof and product work

VM tests are not Chromium or visual proof. The hosted browser journeys and pixel
inspection for this unpublished checkpoint remain pending, as does real host
allocation integration. Earlier hosted screenshots and protected renderer proof
belong to their own exact commits. No new deployment or shared data mutation is
part of this qualification.

The creation/compiler bounds remain visible: reviewed PostgreSQL fields, bounded
filters, supported chart types, explicit preparation/preview, and no arbitrary
SQL, joins, conversational repair, or automatic query on editing/redraw. Essential
formatting, friendly labels/catalog search, and page locale/timezone editing remain
open product work. Unsupported chart combinations are not represented as complete.

The final complete affected race suites on the checkpoint passed: App 6.079 s,
viewer 1.078 s, reporting 16.931 s, reporting API 50.487 s, MCP 3.368 s and SDK
78.512 s. The follow-on test registration includes dataset-filter and both
publication module suites in the ordinary Go/CI resource test entry point;
standalone source qualification already exercised them. Planning coherence,
whitespace checks and the mirrored contributor instructions also passed.

## Final argument-precedence correction

A follow-on source review found that private chart revalidation used a filter name
as a page fallback key. The validation argument builder now mirrors native
`widgetArguments`: block-parameter fallback, widget literal, then bound filter
default; arguments follow declaration order. Duplicate names within one layer are
rejected, while legitimate higher-precedence overrides remain valid. Red tests
reproduced the mismatch before the fix. The 15 mapping tests and all 297 source/VM
tests pass afterward; the reproducible generated script is 217,390 bytes. This is
an argument-construction correction, with no native authority or execution change.
