# Manual report product-quality acceptance — 2026-10-08

## Result and boundary

The manual Builder/Consumer now supports a representative five-output report and
its return/edit/republish lifecycle in the actual registered HTTP and MCP Apps.
This is a useful product foundation. It is not the phase-34 migration or phase-25
whole-product release, nor a claim of unrestricted dashboard capacity.

The synthetic report covers April–September 2026 across four regions, with total
revenue **4,374,060.000 USD**, **18,468 orders**, a monthly trend, regional bars,
and a regional table. The date filter applies explicitly to the table. All values
are native retained results from a real PostgreSQL source; no intercepted provider
responses or runtime model calls produce the screenshots.

## Product behavior

- A compact Pengui breadcrumb and document toolbar leave the composition canvas
  in the first desktop viewport. The component inspector scrolls independently.
- Library selection opens only an eligible public retained result of the exact
  current publication. It never starts an execution or silently opens an older
  publication, private preview, expired run or foreign target.
- Authorized editors return from the published reader to the existing draft or
  review, or amend the immutable publication into a new private draft. Separate
  review and publication confirmations remain explicit.
- All private chart statuses can be checked through bounded metadata reads. That
  does not validate, execute or clear unknown-effect custody.
- Compatible retained formatting previews change only supported display headers
  and precision. Exact rows, values, units and evidence remain unchanged; Cancel
  restores the original. Legacy KPIs without native presentation capability still
  cannot change field precision.
- Reading keeps the saved columns/coordinates and minimum row sizes while letting
  rows grow to expose full content. The editable grid retains fixed cells. Narrow
  screens stack reading cards; Builder preserves its horizontally scrollable grid.
  This does not change portable export geometry.

## Local evidence

Both modes completed actual browser creation, preparation, validation, saving,
private preview, whole-chart publication, explicit rebinding, report review and
publication, public execution, return/edit/save/preview and a second publication.
A separate ordinary Consumer opened every output, reloaded using keyboard
navigation, retained the exact revenue and saw no Builder/execution controls.
Native read authority denied execution, edits, private previews and guessed runs.
Withdrawing context access removed discovery and retained reads; restoring access
restored the same retained values.

The final-source component checks include 359 compiled-resource browser assertions
per transport and 725 chart-renderer assertions across 14 chart kinds.
The last local tab-contrast adjustment received a narrow live visual/color check;
the complete lifecycle preceded the final endpoint-label/contrast refinements. Local Go
resource tests, focused Pengui race tests, 52 client tests, the production frontend
build, zero-warning Svelte checks, scoped lint and planning coherence passed.
The chart suite used real native-engine fixtures produced in the Linux reference
build and rendered in local Chromium. The direct macOS acceptance binary could
not link the Linux-only native archives; that attempt is not counted as passing.
Planning needed `TMPDIR=/private/tmp` to avoid the macOS `/var` symlink mismatch
in pre-existing coverage-gate self-tests. Hosted CI was not used because of the
owner's billing/local-test constraint.

Local warm read measurements include host admission and five authorized output
reads; they exclude source execution. Raw samples and exact final source/binary/
document digests are retained with the external walkthrough. These small synthetic
samples are not a production latency percentile or load benchmark.

## Adversarial review and remaining limits

One single-agent adversarial pass covered exact-revision adoption, stale/late
replies, private/public separation, immutable publication, presentation-only
copies, scope overflow, unsupported formatting, minified property boundaries,
asset limits, scrolling and pointer/keyboard behavior. No subagents were used.
Concrete findings fixed during the pass: overflow wrongly looked like an unknown
effect; short Reader cards hid content; the new rejection missed `no-store`; SVG
endpoint labels were tooltip children; the active mode tab measured 4.31:1
contrast on the host tint and now uses the host foreground color. Narrow regression/recheck followed fixes.
No known reachable P0/P1 remains in these changed seams. This is not independent
review or a whole-branch security certification.

The shared issuer contract still allows **32 scopes / 4096 aggregate scope bytes**.
Five independent private charts sharing this source fit; the six-chart fixture
requires 34 scopes and receives `authority_limit_exceeded` before provider effect
admission. The draft remains usable. This is a dependency-closure limit, not a
universal five-widget cap; reusing blocks differs from adding independent blocks.
Splitting pages does not shrink a whole-report closure. Broader dashboard capacity
needs an explicit cross-service contract design before general product rollout.
The sealed HTML remains below its 256 KiB limit; generated-asset checks enforce it.

The UI uses the product's own tokens and identity. Private predecessor comparisons
and their screenshots remain outside the repository. Compared with the earlier
manual slice, hierarchy, report reopening and complete readable content are now
qualified from actual rendered screens. Remaining larger-product work includes
broader authoring/formatting coverage, capacity, production-scale performance and
the existing whole-release gates. Draft PRs remain open and unmerged; production
is unchanged.
