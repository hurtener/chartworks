# Report-owned inline pages v3

D-097 extends report definitions with independent canvases. Model/runtime support,
browser controls and deployed host qualification require separate evidence.
Pengui remains the only identity and policy authority.

## Version and identity

Version1 section imports and version2 flat reports keep their original retained
bytes/digests. The existing v1-to-v2 projection stays unchanged. Nothing silently
upgrades a stored report; an explicit save may submit version3.

A v3 report has an ordered, nonempty `report_pages` array. Each page contains
`id`, `title`, `widgets` (explicit empty array is valid), optional `filters` and
`defaults`, and optional `locale`/`timezone`. Page locale/timezone inherit the
report's retained settings when omitted. Root metadata, locale, timezone,
audience and partial_failure keep their meanings. Root widgets/filters/defaults,
legacy sections and dashboard pages cannot coexist with inline pages. Dashboards
remain v2 exact published-report references.

Page IDs are stable report-local coordinates, not resources or grants. Widget
IDs are unique across the whole report; filter/default names are page-local.
Pages may independently use the same dataset or immutable block. Empty pages
need no synthetic heading. Grid overlap is checked within a page. Existing
page/widget/filter/default/byte budgets bound the entire report, not each page.

One immutable report revision owns all page content. Page creation, renaming,
reordering, removal and edits append a report revision under existing CAS. Pages
have no separate lifecycle pointers, authority or hidden child reports. Existing
review/publication transitions govern the whole composition. Old private previews
stay private after later publication or draft page removal.

## Storage and authority

Inline pages alone require no page-resource migration: immutable JSON stores the
content; existing tenant-composite block/topic/query indexes retain report-global
widget coordinates and the union of all dependencies. Reference insertion and
revalidation visit every page. SQL filters the whole dependency set before
returning a definition or private catalog metadata. Missing reach denies the
whole definition rather than returning an apparently complete subset.

Manual create/save/preview reject wildcard envelopes. Creation requires exact new
report and tenant/container write; dependency actions/reaches remain independent.
Private retained runs are actor/session-bound and require current exact run-read,
report preview and actual execution-context reach. Creator labels supply no grant.

The presentation/text selected-widget patch adds `page`: omission still selects
`main` for v2, while v3 requires an exact authored page. The server reconstructs
only that widget's allowlisted amendment from the authorized immutable draft,
with report CAS, preserving every other page, widget, binding and layout.

## Execution and delivery

V3 PageInput.page is the authored page ID. Unknown page/filter/widget override
coordinates fail closed. Each page uses its own filters/defaults/timezone. Query
reuse still requires complete identical execution identity; same-dataset pages
with distinct parameters or partition/resolution settings stay separate.

New paged manifests use report-composition-v2 and pin report_page plus, for nested
dashboards, container_page. Historical flat manifests keep exact v1 shape/digest.
A dashboard reference expands every inline canvas in order, assigning a bounded
deterministic coordinate from container/page IDs. Old dashboard IDs remain
unchanged. Admission/checkpoint storage verifies source pages, exact references,
revision/digest/settings and complete page count. No child is silently published.

Descriptions, retained widget reads/navigation and rendering use the ordinary
retained page/widget contract. Existing chart/KPI/table/text rendering stays
unchanged. Edits and repaints do no query/model work. Preview and execution remain
separate explicit actions. Historical description reads use hard format and
existing delivery byte bounds, not lowered mutable authoring limits; execution
continues to apply aggregate run budgets. Stale values never auto-refresh.

Filter-options requests add page to exact report/revision/filter and to the
authenticated cursor. Omission is accepted only for v2. Options remain an explicit
governed source read for the current published revision. This adds no automatic
private-draft option query.

## Scheduling

Full-report v3 schedules pin every widget and preserve page defaults/settings
through retries under the original due time, lease, authority and budgets.
The old schedule DTO cannot identify page-qualified runtime arguments or a
page-qualified saved_question. V3 nonempty schedule arguments and saved_question
targets explicitly reject before execution; arguments are never broadcast or
dropped. Existing v2 schedules keep their semantics.

## Exact private block bridge

V3 block widgets may explicitly use policy private_preview with nonzero revision
and an exact 64-lowercase-hex digest. Digest is allowed only for this policy.
Published widget bodies remain unchanged. Blocks remain separate explicitly
authorized resources. Report-write cannot create, change, validate or execute
one. Default report-local chart edits use an independently authorized private
copy and never mutate the shared original.

Saving a private pin checks exact revision/digest, original block actor, block
read/preview, report write and all dependencies. It does no source/model work and
may retain an unvalidated draft. Explicit preview requires current execute/preview
reach and fresh successful revision-bound validation. Missing, expired or stale
evidence cannot borrow another revision's validation. Later block publication
does not erase the private reference's actor fence.

Private-reference execution is limited to private v3 reports. The existing frozen
private_preview engine owns validated source execution, attempts, query budgets,
actor/session privacy and retention. Existing freshValidation is checked at new
bridge admission and unfinished execution/checkpoints, even when a persisted
health observation remains healthy after evidence TTL expiry. Older standalone
or published run semantics are unchanged. Expiry never reruns or erases already
retained private values.

Migration083 adds immutable document_private_block_refs with tenant-composite
report/widget, exact block revision/digest and original actor. It preserves the
existing published-reference table/publication FK. SQL checks actor and exact
block-preview before loading report/draft-catalog payload. This is custody and
eligibility, not an identity-policy database.

Review/publication reject all unresolved private-reference policies even after
block publication. Storage triggers enforce the same lifecycle rule. An author
must explicitly save a published block policy without private digest, then use
normal report review/publication. Block publication is not private approval.
Report deletion uses existing ownership/fences to erase report-owned runs, retains
content-free reference evidence, and does not infer ownership of separately
created blocks.

## Tests and qualification

Pure tests cover version separation, detached projection, stable IDs, empty pages,
aggregate bounds, explicit schedule selectors and private custody. PostgreSQL
coverage includes immutable snapshots, exact/concurrent CAS, page-scoped patches,
independent same-dataset values, review/publication, nested dashboards, schedules,
page cursors, authority withdrawal, private actor/tenant/document isolation,
evidence expiry at admission/after sealing, explicit published rebind and deletion.

An executor reset required source reconstruction. Pre-reset results are historical
only; the reconstructed tree must rerun all applicable proofs. Browser and real
host activation, static-worker deployment qualification and dataset-preparation
migration084 remain separate evidence.
