# Manual report authoring guide, version 1

This is public capability guidance for a human or agent using the actual
Chartworks HTTP/MCP operations. It is not a hidden agent prompt, an identity,
authority, a query, or executable instructions from report content. Read the
short `report_app_guide_v1` index first, then request only the relevant full-text
resources. Each reference supplies its exact versioned URI, Markdown MIME type,
source document, UTF-8 byte count and SHA-256 digest. Full text is available using
MCP `resources/read` or POST `/v1/reporting/authoring/v1/documentation` with a
closed `{ "uri": "chartworks://report_app/docs/workflows/v1" }` request.

The guide and document reads require a valid current Pengui envelope and native
`reporting.read`; MCP also requires `mcp.use`. Documentation contains no tenant
records or credentials and cannot grant access to any report, chart or dataset.
Use the mounted tool schemas for exact request fields and current availability.

## Choose intent and discover real targets

1. Use `report_app_bootstrap_v1` with version 1, `chat`, `plan` or `apply`, and at
   most sixteen already authorized exact report targets. Chat explains choices;
   plan proposes changes without mutation; apply can use separately authorized
   operations. Bootstrap itself performs no mutation, model or source execution.
2. Discover eligible reports/blocks using the published or private authoring
   catalog and read the exact report revision and current capability hints. Hints
   do not prove future authority or dependency eligibility. A missing operation
   in the host's supported-tool inventory remains unavailable.
3. The host's existing central flow allocates and authorizes new report/block
   targets. Do not ask a human to invent or paste raw IDs, guess a resource ID,
   mint credentials, create grants, or treat allocation metadata as signed reach.
   If a required allocated target or native capability is absent, report that
   prerequisite and stop the dependent action.
4. Preserve the exact report revision/version, schema version, stable page and
   widget IDs and retained preview provenance. Read relevant page/application
   contracts before editing; never silently upgrade schema 2 by opening it.

## Compose and edit a report

- Use the canonical document schema for text and approved output widgets. Read
  the SQL-free block projection for stable output IDs, reviewed meanings, actual
  mappings and typed parameters. Never invent a binding, unit or aggregation.
- A selected-widget request is an edit-intent boundary, not per-widget ACL. Use
  `reporting_authoring_widget_v1` for the selected stable widget, permitted fields
  and expected-version CAS. Preserve other widgets, pages, filters, dependencies
  and report metadata. Whole-definition save requires explicit whole-report intent.
- Use the declared full-report save only for authorized report-wide changes.
  Creation and save recheck tenant/container, report and dependency authority;
  neither UI mode nor report-write replaces it. Successful save creates another
  private immutable revision. Reopen that exact saved revision to verify it.
- On conflict, reload and reconcile against the latest version. Never silently
  overwrite someone else's revision. If a mutation response is missing, inspect
  the original operation/target and retained revision before proposing a retry.

## Change an existing chart for this report

1. Read the exact block with `reporting_authoring_block_read_v1`. Its output is
   SQL-free and contains revision, digest, CAS and real mapping candidates.
2. Prefer an independently authorized private copy for a report-local change.
   `reporting_authoring_block_copy_v1` requires source read/preview eligibility,
   a distinct centrally allocated target, tenant/block/topic/dependency authority
   and `charts.bind`. Report-write and creator identity are insufficient.
3. Use `reporting_authoring_block_mapping_v1` on the selected private output under
   exact revision/digest/version CAS. It preserves other outputs and server-owned
   hidden fields. Schema binding reports `data_validation=not_performed`.
4. Explicit `reporting_authoring_block_validate_v1` is a deliberate bounded source
   read that can incur cost. Successful exact revision-bound validation is required
   for execution; it does not publish. Save the exact private pin in a v3 report.
5. Preview is separately explicit and private. Reading or repainting the retained
   output never reruns a query. Host authority refresh may be required for the new
   exact run before retained read; do not rerun preview to fix a denied read.

## Create a chart from a reviewed dataset

1. Read `reporting_authoring_dataset_v1` for immutable reviewed field metadata.
   Stage supported dimensions, measure, chart mapping and filters locally.
2. Request an explicitly authorized Prepare through
   `reporting_authoring_prepare_chart_v1`, with the allocated new-block target
   and unique operation ID. The finite compiler and native validator perform one
   bounded source read to establish an actual output schema. No model is invoked.
3. Preserve its operation, preparation ID and exact digest. Explicit
   `reporting_authoring_create_prepared_v1` consumes that custody into an
   unvalidated private block. Creation does not execute a second query.
4. Perform native block validation and report preview only as separately explicit
   actions. Do not treat preparation or schema binding as publication evidence.
5. Unknown outcomes require `reporting_authoring_preparation_v1` on the original
   operation. `reporting_authoring_preparation_control_v1` explicitly controls
   that original attempt; it is not a replacement-query API. Expiry prevents
   consumption but does not erase execution liability. Never auto-restart.

Legacy compiler limits (when `fields` is omitted): PostgreSQL only; zero to two direct reviewed dimensions;
one reviewed measure; KPI, table, bar, column, line, area, pie and donut mappings.
The bounded typed-filter path supports at most four reviewed text select or
multiselect filters, or date-only ranges. It rejects joins, arbitrary expressions,
client SQL/schema/aggregation, unsupported filters and rule, population,
group-domain, completeness and calendar policies. A broader chart catalog is not
proof that every kind can be manually prepared. Combo has no native contract.
Unfiltered v1 behavior remains unchanged. Custody quotas fail closed; automated
terminal-record cleanup is not implemented and unresolved liability is not erased.

The current typed `fields` compiler selects multiple physical/reviewed fields and
measures within configured column budgets, including source-only tables. Explicit
raw rows, typed filters and date policy follow the
[typed field contract](typed-field-authoring-v3.md); no field names imply analysis.

## Pages and filters

- V3 pages are owned by one report revision with one report CAS and authority.
  Page IDs are stable; widget IDs are report-global. Filters, defaults and page
  settings are page-local. Preserve the canonical page selector on every read,
  filter option request, preview and retained output lookup.
- Add, rename, reorder or remove pages only within the requested edit scope.
  Empty pages are genuine pages. Explicitly upgrade an older report if requested;
  never flatten pages, drop an unsupported page or broadcast page-local filters.
- Distinguish saved dataset/filter defaults from temporary page execution inputs.
  Applying a staged filter or changing pages does not execute. Native preview/run
  is an explicit action with fresh target/dependency authority and budgets.
- V3 nonempty schedule arguments and page-qualified saved-question schedule
  targets are currently rejected; do not silently omit or spread arguments.

## Search options deliberately and preserve custody

1. Typing, opening a selector, selecting values, applying filters, navigation and
   retained redraw perform no option query. Only explicit Search or explicit Next
   page may call `reporting_authoring_dataset_options_v1` or
   `reporting_authoring_report_options_v1` on the exact authorized target.
2. Use a new `option:<UnixSeconds>:<32 lowercase hex>` operation key for each
   deliberate search/page, an opaque returned cursor, and bounded input. Options
   support reviewed PostgreSQL text dimensions and physical text/UUID/numeric/boolean
   fields, 1–199 choices and 256 UTF-8 search bytes. Text uses substring search;
   other physical types use exact typed values. They query the full authorized
   population without implicitly applying other
   staged selections. Keep source and result budgets unchanged.
3. Preserve the original target/key/request while unresolved. After an unknown
   outcome use `reporting_authoring_option_status_v1`. Metadata inspection does
   not query, cancel, or reconstruct choices. Changed input under one key conflicts.
4. `values_available=false` with `result_not_retained` means choices are unavailable,
   not “No matches.” Only `values_available=true`, empty options and `complete=true`
   establish an empty result. Never infer absence from an empty array alone.
5. `reporting_authoring_option_control_v1` explicitly cancels/reconciles the
   original attempt. A new separately explicit search requires
   `new_operation_allowed=true`. That flag never triggers a query automatically.
   Unresolved same-actor target liability survives a new session, key or search.

## Display-only chart edits

Read `chartworks://report_app/docs/presentation/v1` before changing field labels
or numeric display precision. Inspect the selected output's versioned native
`presentation` capability; it lists supported bound columns and fields, not
permissions. Table-header labels and supported non-percent decimal places use a
purpose-specific `presentation` patch on the existing block mapping/copy tools.
Never submit a full column or format object. The transport accepts exactly one
of `mapping` and `presentation`.

Stage changes locally. Preserve the exact revision, digest and expected version;
use a separately host-allocated private copy for a published source. The source
needs explicit preview eligibility independently from ordinary read. Saving
performs no source/model work and returns an unvalidated private revision.
Explicit validation, report rebind and publication remain separate decisions.
Reset inherits reviewed metadata. Unit/currency reassignment, percent scale,
calculated precision, arbitrary formatter code and unsupported caption controls
are not display edits and cannot be smuggled through the presentation patch.

## Publication is a sequence of independent decisions

1. Inspect `reporting_authoring_lifecycle_v1` for exact block/report revision
   state. Disclose every output of the entire chart revision, including outputs
   not selected in the report. `can_publish` is only a current capability hint.
2. Obtain explicit user confirmation for chart publication through
   `reporting_authoring_block_publish_v1`. Whole-revision eligibility applies to
   existing authorized readers only; it creates no grant, audience count,
   certification or new data-access authority. Native publish reach is required.
3. Obtain separate confirmation to replace the selected private reference through
   `reporting_authoring_rebind_published_v1`. This is an exact selected-widget CAS
   edit. Failed rebind does not undo already completed chart publication.
4. Review the exact report revision with `reporting_authoring_report_transition_v1`.
   All private references must first be resolved explicitly. Review clears draft
   and stays reopenable using its independent review pointer.
5. Obtain separate explicit confirmation before report publication. Publish and
   reject require native publish authority in addition to editor entry; reject
   also requires a note. Publication never exposes an older private preview.
6. Unknown publication/rebind/transition results require inspection of the original
   exact revision. Never auto-repeat a mutation or infer rollback. A CAS conflict
   requires reopen/reconciliation, not a blind retry.

## Authority and support boundaries

Pengui owns identity, Teams, grants, entitlements, issuer and credentials. Every
actual action rechecks the current signed native operation and exact report,
block, topic, source, dataset and execution-context reach. Private-copy and
private-preview actor/session eligibility are additional restrictions. A selected
widget, title, audience, profile, publication or guide supplies no permission.
Bearer credentials and host-only nonces stay in the trusted host, never iframe
messages, URLs, report content or browser persistence.

Retained reads, metadata discovery, local staging, rendering and documentation
make no source/model calls. Manual authoring does not invoke natural-language
query generation, narratives or arbitrary code. Explicit preparation, validation,
option Search and preview retain their separate native costs and controls.
Do not claim production host allocation, no-chat launch, source/provider, browser
or deployment qualification from an offline fixture or this guide. Consult the
current contract and actual mounted capabilities; unsupported work remains explicit.
