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
manual filter editor remains unfinished.

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
schema version and repository contract references. No hidden repository agent,
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

Schema binding returns data_validation=not_performed. block_validate explicitly
uses native bounded source validation, returns narrowed evidence/schema and CAS
coordinates, and never publishes. Its source work may carry cost and is not
idempotent or automatically retried. Metadata alone cannot attribute an unknown
request. Source validation, private report preview and public eligibility changes
remain separate user actions. Mounted inventory is 83 default/88 with optional
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
unsupported filters, calendar/population/completeness policies, rules, joins and
arbitrary expressions. Create consumes exact private preparation custody into an
unvalidated draft; validation and private report preview remain separate explicit
source reads. Preparation cleanup is not yet implemented; bounded quotas fail
closed without erasing unresolved execution liability.

## Remaining manual-product work

- Governed option search, single/multiselect and staged date-range controls;
  authored defaults versus temporary choices; explicit widget/filter applicability.
  New dataset charts currently have no query parameters. Private chart parameter
  discovery must use the authoring projection, not published-only delivery.
- Editable field labels and number/date/currency display settings, legend controls,
  data-point limits and additional qualified creation shapes. Saved semantic units
  and aggregation cannot be replaced by unvalidated display input. Combo has no
  native contract and is not offered as another kind.
- In-app block publication, explicit private-to-published reference rebind and
  report review/publication. Current private authoring does not publish or expand
  visibility. Existing native lifecycle APIs retain their independent authority.
- Real Pengui no-chat launch, exact target allocation, dependency projection,
  authority refresh/withdrawal and production embedded registration. Hosted
  synthetic-browser proof does not close those integration gates.
