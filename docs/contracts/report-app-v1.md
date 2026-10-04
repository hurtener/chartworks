# Manual report application v1

Status: implemented subsets under D-096/D-097, with exact-source qualification in
[the evidence record](../reviews/report-app-v1-evidence.md). The complete manual
journey and production host activation remain open. This continuation belongs to
phases 29/31. The previous read viewer URI and delivery contract remain supported
independently.

## One application, two adapters

The optional `ui://chartworks/report-app/v1` resource is public-data-free compiled
presentation. It reuses the existing document schema and retained-output renderer.
MCP Apps invokes registered tools; the embedded adapter invokes only the trusted
registered parent's bounded operation bridge. The app does not fetch credentials
or choose provider destinations. Both surfaces reach the same HTTP/MCP DTOs and
domain services. Manual use has no chat, conversation, hidden agent or model
prerequisite.

The separate `/apps/report/v1/embedded` asset is registered only when
`reporting.app.embedded_enabled=true`. Its public HTTPS parent-origin list comes
only from `reporting.app.registered_parent_origins` (1..16 exact unique origins).
The default is disabled with an empty list; CORS clients do not automatically
become embedded hosts. HTTP, null, wildcard, credential-bearing and non-origin
entries are rejected. Data-plane CORS, if needed, is independently configured.
The response supplies matching `frame-ancestors`; the HTML has an explicit
embedded bootstrap and never falls back from MCP. No deployed setting is changed
by this implementation.

The embedded host owns origin registration, frame instance/generation, one-use
initialization correlation, capability/entitlement checks and credential realm.
Every message pins origin and expected parent window; stale or duplicate replies
are fenced. Host-only nonce and Pengui bearer stay outside the iframe. No URL
bootstrap, browser-storage token, unrestricted fetch, arbitrary method relay or
wildcard authority is part of this contract. The host must close outstanding work
on logout, organization change, withdrawal, navigation or frame replacement.

## Native authority and presentation

Pengui resolves current user, Team, organization, audiences, entitlement and grants.
Read, write and grant remain separate central permissions. Chartworks validates
Pengui authority and applies exact `cw.<kind>.<permission>:<id>` references and
native actions to every actual domain dependency/context. Report IDs, publication,
profile names, saved provenance and guidance are not grants.

Builder/Consumer are derived UI hints, not roles. A hint is not proof that a target
exists, all dependencies are reachable or a future call will succeed. Each call
revalidates. No users, roles, memberships, policy database or credential issuer
is introduced. Private draft read still requires independent read and preview
reach; report-write alone cannot expose private retained values.

New document creation requires exact new report write plus signed tenant/container
write and the dependencies of all selected published blocks. The central host must
approve/allocate this exact target through its existing ownership flow before
supplying authority. An app editor grant alone cannot authorize creation.

## Host operation inventory

Initialized embedded hosts may supply `capabilities.supported_tools`; MCP hosts
preserve the same extension at `hostContext['chartworks/supported-tools']`.
Its closed shape is `{version:'chartworks-host-tools-v1',names:[exact names]}`:
16 KiB maximum, at most 96 unique names, each 1..128 ASCII alphanumeric,
underscore, dot, colon or hyphen characters. Invalid shapes, duplicate names,
unknown versions and exceeded bounds fail closed. Valid unknown names never
expand the app's fixed registered-operation allowlist. Missing metadata preserves
legacy full-capable compatibility; an explicit empty list disables provider
calls. The copied list is immutable for that frame/session. Context notifications
cannot change it; inventory changes require a fresh frame/session.

This is a presentation-only narrowing hint, independent of signed authority and
independent of host target-allocation metadata. Creation requires allocation plus
native capability/create/read/save tool availability, followed by the exact
allocated report's native `can_create` check. Tool names never grant read, write,
execute, preview or publish reach. The app transport independently rejects every
unsupported provider call before sending it.

Retained-only and text-only hosts hide unsupported chart/source/filter/execution
controls while keeping their real report operations. Text-only full-report CAS
save needs no widget-patch tool. Block/filter-bearing drafts remain inspection-only
when block authoring metadata is unavailable. New reports use schema 3 with a
single empty `Summary` page and omit top-level widgets/filters/defaults, so an
explicit title-only save uses the genuine empty-page domain behavior. Existing
schema-2 definitions are never implicitly converted. These deterministic source
checks do not substitute for browser/deployed-host evidence.

## Bounded manual operations

`/v1/reporting/authoring/v1/` carries closed POST DTOs and corresponding versioned
MCP tools. Capabilities, drafts and read are metadata/read operations. Create and
save delegate canonical document validation and immutable revision CAS. The draft
catalog applies current tenant, target and dependency eligibility before paging;
it does not add private state to the published catalog.

The initial UI composed headings and already-published block outputs. That path
remains supported: selecting an output retains its stored meaning and cannot
replace a chart mapping or reinterpret units. The current continuation also has
separate exact private mapping amendments and reviewed-dataset chart preparation,
described below. Report presentation itself never changes a query or mapping.
Manual operations disable dynamic query widgets and narratives. Filters are typed
canonical document parameters, not SQL or authority predicates; the complete
filter controls stage defaults and temporary choices independently; current hosted
browser qualification remains pending.

Targeted widget editing is an edit-intent boundary, not per-widget ACL. A separate
narrow patch receives a stable widget ID, expected revision/CAS and allowlisted
presentation/text fields. It loads the authorized baseline and constructs a new
revision while preserving untargeted widgets, filters and layout. A selected-widget
agent workflow must use this patch instead of whole-definition save. Whole-report
save is an explicit manual report-write operation. Broader layout or batch edits
need their own explicit scope and validation; target IDs cannot grant resource
reach.

Private preview reserves and executes an exact private revision through existing
composition services, with fresh execute/preview/dependency authority. Preview
privacy survives later publication. After admission/execution returns a new run
ID, the host obtains fresh exact run-read/preview authority before reading private
retained values. A denied read is retried only after host refresh, never by
rerunning the preview. Reading/repainting retained output makes no
source/model calls. An unknown execution outcome is inspected, never blindly
replayed. Stale CAS requires reload and reconciliation.

## Retained saved-layout presentation

Builder and Consumer use the existing retained-output renderer for headings, KPI,
trend and table cells in the saved grid. Presentation does not execute a query.
After an explicit preview, read-only fanout is limited to four concurrent reads,
100 selected cells and 16 MiB in aggregate (4 MiB per result). Each read validates
its exact run, target, widget, output, digest and page identity before reuse.
Table paging reads retained pages only. Navigation, expiry, denial and malformed
responses clear values; stale previews are marked and never automatically rerun.

Builder compatibility is established only by the captured admission snapshot,
not matching revision numbers alone, because runtime filters can differ. Safe
presentation-only edits retain that provenance. Existing retained query-widget
outputs remain readable without enabling query creation in the manual editor.
Required mapping arrays preserve nil versus non-nil emptiness through cloning;
frozen-output digest validation remains strict.

## Agent bootstrap and resources

`report_app_bootstrap_v1` / POST `bootstrap` accepts version 1, mode chat/plan/apply
and up to sixteen exact report targets. Each target requires current exact signed
reach before capability hints are returned. Mode is interaction intent only:
chat explains, plan proposes without mutation, apply uses separately authorized
mutations. Bootstrap itself never edits, starts a model or expands authority.

`report_app_guide_v1` / POST `guide` and the static resource
`chartworks://report_app/guide/v1` provide versioned steps, constraints, document
schema version, retained repository contract references and typed full-text resource
references. `chartworks://report_app/docs/workflows/v1` is the consumable
[manual authoring guide](report-authoring-guide-v1.md). The explicit catalog also
contains the full maintained application, pages, preparation, option search,
publication, native presentation, authority, filter, reporting and delivery contracts. `resources/list`
returns only descriptions/version/digest/source references; `resources/read`
returns the exact embedded Markdown. POST `documentation` accepts only an exact
`uri` and uses the same native-action-protected immutable reader as MCP; the typed
SDK method is `ReadReportAppDocumentation`. No new MCP tool is added.

The catalog admits at most sixteen documents, 64 KiB each and 256 KiB in aggregate,
under the unchanged transport response limits. Current content is eleven explicitly
embedded public documents, never arbitrary repository paths. Unknown URIs/versions,
encoded aliases, queries and traversal fail closed. Every read requires a current
verified envelope and `reporting.read`; MCP additionally requires `mcp.use`.
SHA-256 and byte count identify the exact returned UTF-8 bytes. Descriptors and
text are immutable per build; incompatible document revisions require a new
versioned URI, and consumers must not assume the same digest across builds.

No hidden repository agent,
copied third-party prompt, user context or secret enters that resource. Agent
profiles consume these same public interfaces and obey target operation bounds.

## Integration boundary and verification

Pengui's initial registered App lane is pending retained-read coverage using
operator-approved exact resource/context references. Provider metadata does not
yet supply a dynamically discovered whole-catalog dependency closure. Do not infer
that closure from browser-provided definitions. Manual host activation additionally
needs central exact-target allocation, action projection, no-chat admission and
both HTTP/MCP credential realms wired end to end.

Required evidence includes draft create/save/reopen/CAS/private preview on actual
domain storage; exact and missing target/dependency/context negatives; cross-tenant
and private-state exclusion; HTTP/MCP schemas/effects; selected-widget preservation;
credential-free resource; parent origin/generation failures; repeated/aborted save
and navigation; actual browser rendering and retained read. The evidence record
separates local native checkpoints from hosted browser checks. Deterministic
fixtures are not deployed host or live-provider evidence. Direct single-card
move/resize and inline page editing are now implemented; advanced layout tools,
arbitrary NLQ widget creation and optional export remain outside this checkpoint.

## Visual authoring and mapping continuation

[D-097](../decisions/2026-10-03-visual-chart-authoring.md) makes direct grid editing,
real chart/field authoring and private pages required increments, not optional
substitutes for a complete Builder. The grid supports one-card move/resize,
collision rejection, duplication and explicit arrangement; advanced layout tools
and arbitrary natural-language creation are not implied.

Closed block_read, block_mapping, block_copy and block_validate operations extend
the versioned namespace. Responses omit SQL, narrative instructions and execution
custody details. Read exposes exact CAS/revision/digest/schema/mapping candidates.
Mapping changes only one current private output; copy needs a distinct authorized
new target and independent source read/preview eligibility. Both preserve all
server-owned hidden definition fields and other outputs. charts.bind/tenant-read
and native block/parent/dependency authority remain separate from report-write.

The same mapping/copy transports also admit an exclusive, non-null presentation
patch under [native presentation v1](chart-presentation-v1.md). Only advertised
visible table-header labels and supported non-percent decimal places can change;
canonical columns and calculated exact values do not. Source snapshot checks for
copy happen at admission, while creation is fenced by the target's atomic CAS.
No new tool or implicit validation/execution is added.

Schema binding returns data_validation=not_performed. block_validate explicitly
uses native bounded source validation, returns narrowed evidence/schema and CAS
coordinates, and never publishes. Its source work may carry cost and is not
idempotent or automatically retried. Metadata alone cannot attribute an unknown
request. Source validation, private report preview and public eligibility changes
remain separate user actions. Mounted inventory is 91 default/96 with optional
renditions under the unchanged 96-tool ceiling; execution and transport budgets
are unchanged.

Version-three [report-owned inline pages](report-pages-v3.md) have independent
widgets, filters/defaults and settings, stable page IDs and report-global widget
IDs. One report owns their immutable revision, CAS and authority. The UI supports
explicit upgrade, add/rename/reorder/remove and empty pages; it never rewrites an
older stored definition merely by opening it.

The five [reviewed-dataset preparation operations](manual-chart-preparation-v1.md)
provide metadata, explicit Prepare, status, Create and original-attempt control.
The finite compiler is PostgreSQL-only, with zero to two direct reviewed
dimensions, one reviewed measure and eight native initial chart kinds. It rejects
unsupported filter forms, calendar/population/completeness policies, rules, joins
and arbitrary expressions. Up to four reviewed text select/multiselect or date-only
range filters compile through the bounded typed v2 path; unfiltered v1 bytes remain
unchanged. Create consumes exact private preparation custody into an
unvalidated draft; validation and private report preview remain separate explicit
source reads. Preparation cleanup is not yet implemented; bounded quotas fail
closed without erasing unresolved execution liability.

The four [governed option operations](governed-authoring-options-v1.md) expose
explicit dataset/page-filter Search, retained status and original-attempt control
through shared HTTP/MCP/SDK DTOs. No typing, selection, page navigation or retained
redraw implies a source read. Lost values are distinct from empty choices;
new_operation_allowed permits only a separately explicit new search after
liability is resolved. Browser controls and their integration require separate
qualification.

## Manual lifecycle transport and agent guidance

The four [manual lifecycle operations](manual-publication-lifecycle-v1.md) use the
same closed domain DTOs on HTTP, MCP and the typed Go SDK: `lifecycle`,
`block_publish`, `rebind_published` and `report_transition`. Inspection is retained
metadata only; the other operations write metadata without source or model work.
`report_transition` has a `reporting.write` editor-entry ceiling plus the native
`reporting.publish` action and exact publish reach for publish/reject. Review,
publish and reject are the only accepted transition values. No profile, mode,
publication or advertised tool grants authority.

The bounded guide requires explicit user confirmation before chart publication
and separately before report publication. It discloses every output in the entire
chart revision, even outputs not selected by a report. Existing-authorized-readers
eligibility creates no grant and supplies no audience count. Chart publication,
selected private-to-published rebind, report review and report publication remain
separate actions. Rebind needs separate confirmation and cannot undo publication
if its own CAS fails. Private retained previews remain private.

Unknown mutation outcomes require inspection of the original exact revision,
never automatic repetition or inferred rollback. Review clears draft and can be
reopened through the independent review pointer. Transport/SDK source and focused
unit qualification do not establish PostgreSQL lifecycle, browser or deployed-host
qualification; their respective evidence remains independently required. The
[transport qualification record](../reviews/manual-publication-transport-v1.md)
names the exact checks and their limits.

## Remaining manual-product work

- Hosted browser qualification for the locally implemented governed option search,
  single/multiselect, staged date-range controls and applicability editing. Dataset
  filter defaults and saved report filter defaults are distinct from temporary
  `PageInput.Filters`; private metadata discovery uses the authoring projection.
- Broader caption, date/currency display settings, percent precision, data-point
  limits and additional qualified creation shapes. The bounded presentation
  extension covers table headers and advertised non-percent decimal places; its
  UI and renderer qualification must be tracked at the exact source. Saved semantic units
  and aggregation cannot be replaced by unvalidated display input. Combo has no
  native contract and is not offered as another kind.
- Browser qualification of explicit chart publication, selected private-to-published
  reference rebind and independent report review/publication. Native domain and
  shared HTTP/MCP/SDK lifecycle operations retain their independent authority.
- Real Pengui no-chat launch, exact target allocation, dependency projection,
  authority refresh/withdrawal and production embedded registration. Hosted
  synthetic-browser proof does not close those integration gates.
