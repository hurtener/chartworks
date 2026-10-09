# Physical option search qualification

D-109 extends existing option lookup to typed physical fields selected from
registered tables or reviewed datasets. No production selection uses business
names or infers a predefined analysis. Physical date/timestamp fields remain
explicit ranges. Source-only origins require no topic permissions.

Single-agent review covered closed exclusive targets, exact registered binding
and column type, reviewed rule refusal, original actor/session/operation custody,
parameterized search and cursor ordering, forward migration constraints, native
admission locks and Pengui's independent dependency checks. Search excludes NULL
and uses the full authorized population without hidden chart filters. Typed JSON
strings preserve exact decimal/integer values. Shared controls validate returned
values before accepting them. Unknown outcomes never trigger automatic reads.

Evidence is local:

- Full `internal/reporting` and `internal/reportingapi` suites pass.
- `TestReportAppPhysicalOptionsNative` proves raw/reviewed discovery, real typed
  ordering/search, exact decimal pages, no topic dependency for raw data, denied
  source/context/dataset reach, changed schema refusal and replay without queries.
- `TestReportAppPhysicalOptionScalarTypes` proves actual UUID/float search and
  typed paging, including NULL exclusion, through the validator and source driver.
- `TestReportAppColumnFiltersNative` covers private and published report option
  populations alongside source-only preparation/publication, temporary overrides,
  amendment and compacted replay. Its option actor has exact, non-wildcard reach.
- Native physical/report, original cancellation, receipt/rule fences, retention
  and dependency acceptance pass with the race detector. Legacy reviewed options
  and HTTP closed-schema tests pass against migration 090.
- Shared JavaScript/resource tests pass. Compiled Chromium runs pass 404 assertions
  per HTTP iframe/MCP mode. Desktop/mobile option controls were inspected. These
  hosts are synthetic; screenshots do not establish live policy enforcement.
- Pengui host race and embedded-host tests pass. Manifest negatives include
  otherwise authorized extra source/topic references rejected for raw targets.
  Svelte check reports zero errors and warnings.

Test fixtures were corrected to use advertised reviewed fields, explicit block
revision pins, deduplicated scopes and an exact option-reader envelope. Production
wildcard and typed admission guards were preserved. UUID/float results were
additionally checked against a freshly registered actual schema, not inferred
from conversion tests. No unresolved P0/P1 finding was identified in this slice.

People/Team audience management, uploaded-data signed-in author-to-reader flows
and the final standalone walkthrough remain active work. Hosted CI is billing
blocked, not passing. No production deployment, merge or whole-product release
is claimed.
