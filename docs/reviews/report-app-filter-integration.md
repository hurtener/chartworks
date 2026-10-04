# Manual filter integration checkpoint

2026-10-04. Local source checkpoint; Git publication and actual hosted Chromium
qualification remain pending. This does not qualify the unfinished real host
allocation service or the separate publication UI.

## Implemented user flow

New reports start as a saveable schema3 Summary page without raw target-ID inputs.
The host's immutable tool inventory narrows controls; native authority remains
independent. Text-only hosts can create, edit pages/headings and reopen through
report CAS. Source/chart operations are not offered when absent from registration.

Dataset creation can add up to four reviewed filters: text select, 1–16-value
multiselect or native PostgreSQL date range. Choosing fields, typing search text
and changing staged values never query data. Explicit Search first obtains the
host-allocated exact chart target, then reads a bounded governed option page.
Unknown lookup custody survives closing/reopening setup and fences Prepare or
retargeting until explicit inspection/control establishes a terminal outcome.

Saved report filters discover private parameters through authoring metadata.
Their saved default is `Filters[].Parameter.Default`; the separate block-parameter
`Defaults` fallback layer stays unchanged. Temporary preview/Consumer selections
use `PageInput.Filters` and never overwrite a saved default. Date inputs use
inclusive civil dates and commit only on Done. Empty text remains a real category,
not the implicit default or SQL NULL. Shared filter applicability is explicit.
Retained values are marked stale after temporary selections change; no redraw
re-executes a query.

## Evidence and boundaries

- Integrated native `ea86fdc`: full reporting API, MCP registry and SDK race
  packages passed; focused option/filter domain/store race and all four real
  PostgreSQL option journeys passed, including the final quota correction.
- `617a62f`: real PostgreSQL dataset/private/published option journey passed,
  exported a bounded synthetic SQL/credential-excluding local DTO and the UI
  modules decoded actual metadata, canonical defaults and the exact199-limit
  request/response. The Node check executes no source or model work.
- Generated resource `00e33df`: 236 lightweight tests included five tests of the
  actual bundled MCP/embedded script. Go resource/CSP checks and actual factory
  inventory passed (87 default/92 optional, ceiling96). HTML was214,129bytes,
  below the unchanged262,144-byte cap. No local Chromium was used.
- A subsequent saved-default regression corrected the UI to update the native
  filter declaration, preserving block fallback defaults and detached-page fences.
  It is covered by staged-input tests and requires regenerated-resource recheck.

The pinned esbuild build-time artifact is reproducible and source-hash checked.
The production resource contains no runtime compiler, remote module, source map,
credential or tenant data. Authored renderer code remains single-source and
readable; the report app omits the unused viewer controller.

Actual hosted screenshots and full browser interactions for these later controls
are still required. Earlier screenshots and renderer qualification belong to their
exact earlier heads and are not transferred to this checkpoint.
