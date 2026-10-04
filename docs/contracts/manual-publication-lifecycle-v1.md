# Manual publication lifecycle v1

This local authoring seam reuses `Authoring`, native blocks, document revisions,
and independent report lifecycle pointers. It creates no grant, audience list,
query engine, model call, publication rollback, or public-preview conversion.

## Operations and exact inputs

Four optional authoring operations suffice:

- `InspectLifecycle`: exactly one `report` or `block`, `revision`, and optional
  report `stage` (`draft` or `review`). Blocks require an exact positive revision.
  A report revision is exact when positive; otherwise its requested pointer is
  used. The absent stage defaults to draft, then review only if draft is missing.
- `PublishBlock`: `block`, `expected_version`, exact `revision`, exact `digest`,
  and current validation `evidence`. One native whole-revision publication call.
- `RebindPublished`: `report`, `expected_version`, exact report `revision` and
  `digest`, and 1–100 unique widget pins `{widget,block,revision,digest}`. One
  native report CAS edit, independently confirmed after block publication.
- `TransitionReport`: `report`, `expected_version`, `revision`, closed `operation`
  (`review`, `publish`, `reject`), and `note`. The transition body preserves native
  expected-version/revision/note semantics; rejection requires a nonempty note.

The transport may impose the manual editor's `reporting.write` entry ceiling;
that never replaces the method's native action and resource authority. Review
uses `reporting.write` plus exact report write; publish and reject use
`reporting.publish` plus exact report publish. No rejection-as-withdrawal or
combined review-and-publication transition exists.

All manual requests reject wildcard resource reach. Inspection reads metadata
under current tenant/target/dependency authority, never SQL or rows. Report
inspection and rebind require report write and ordinary read; private report
reads independently require preview. Published block dependencies use ordinary
block read and their own dependency reach, without borrowing a parent preview
requirement. A private block or private widget pin keeps its original block
actor and block-preview fence even after that revision becomes published.

## Consequences and recovery

Inspection discloses the entire block revision's safe metadata and all outputs,
including outputs that its report did not select. `validation_fresh` uses native
content/evidence/expiry/current-dependency/health checks. `can_publish` is only a
current hint: original actor, exact native publish authority, validated draft,
no archive/publication, and fresh evidence. Publication rechecks all conditions
and the native commit's revision, CAS, dependency and evidence fences. It performs
no validation, warehouse work, model work or certification.

Block publication makes the entire immutable revision eligible to readers
already entitled by centrally signed authority. `publication_scope` is always
`entire_revision`; `audience_effect` is `existing_authorized_readers`. Neither
field asserts an audience count, broader grant, or report publication.

Rebinding verifies each exact private widget and its already-published exact
block revision/digest first. It changes only the selected widget policy from
`private_preview` to `published` and removes that policy's private digest marker.
Pinned revisions, outputs, parameters, filters, defaults, layout, presentation,
other widgets, other pages and descriptive audiences are retained. Private v3
reports only are eligible; no legacy definition is silently upgraded.

Publication and rebind are separate operations. If block publication succeeds
and report CAS fails, the report remains as stored and the block remains
published. Reopen/inspect before explicitly confirming any new edit. Unknown
publication/transition outcomes are inspected at the original exact revision;
never automatically repeat publication or infer rollback. Inspection establishes
current state, not attribution of an ambiguous response to a particular caller.

## Review remains recoverable

Native review clears draft and sets review; it does not publish. The existing
manual private catalog now selects draft, or review when no draft exists, and
returns `stage`, `draft_revision` and `review_revision`. Tenant, exact write
selection and complete persisted dependency/private-block restrictions apply in
SQL before pagination and metadata projection. A newer draft does not replace an
independent review pointer. Explicit `Authoring.Read.stage=review` reopens it;
default read falls back from absent draft to review. Exact reads/inspection retain
full head pointers only under authoring write plus independent read authority.
The public catalog remains published-only. Rejection remains native publish
permission plus note, and a rejected immutable revision cannot be resubmitted
without a new edit.

All retained preview privacy survives block publication, rebind, report review,
and separate report publication. Consumer reads use their existing native
publication and centrally signed read requirements. No lifecycle operation
implicitly executes a Consumer run.

## Evidence boundary

`TestReportAppManualPublicationLifecycle` is the intended real PostgreSQL and
frozen-source acceptance test. `TestAuthoringLifecycle*` covers pure domain and
pre-I/O boundaries. Run results must be recorded separately; source presence is
not evidence that tests, transport parity, browser flow, or deployment passed.

## Local qualification, 2026-10-04

At source checkpoint `67c359f`, the following actual bounded checks passed:

- `go test ./internal/reporting -count=1` (1.677 seconds).
- `go test ./internal/store/postgres -run TestDraftCatalogDeniesBeforeDatabase -count=1`
  passed at the preceding checkpoint (0.041 seconds); its source is unchanged.
- `go test ./test/acceptance -run '^(TestReportAppAuthoring|TestReportPrivateBlockBridge|TestReportAppManualPublicationLifecycle)$' -count=1 -v`
  (8.231 seconds), using disposable real PostgreSQL 17 databases and the synthetic
  frozen-source fixture. The lifecycle test passed five named subtests, including
  actual native-import preservation of empty defaults during exact rebind.
- `git diff --check` passed.

The lifecycle journey asserts all-output disclosure, original actor/tenant and
revoked-dependency denial, stale pins and CAS, one-widget-only rebind with an
unselected private widget remaining private, publication surviving a failed
report rebind, review recovery, independent newer draft/review/publication state,
reject permission and note, exact outcome inspection after publication clears
private pointers, authorized Consumer metadata reads, and retained preview
privacy. It asserts zero source/model work throughout lifecycle operations.
Only its explicit validation and private-preview execution call the source.

These are non-race local domain/store/acceptance results. They do not qualify the
new transport registration, SDK, UI, browser journey, deployed host, audience
policy, or race suite. The initial acceptance fixture's unsupported named SQL
placeholder was corrected to the native PostgreSQL positional bind; no SQL
validator or production safety boundary changed.

### Aggregate inspection projection bound

Lifecycle inspection has one fixed 3 MiB serialized JSON response budget beneath
its app's 4 MiB response ceiling. The complete report, stage, every distinct
block revision, envelope and array separators are counted incrementally before
appending, followed by a final complete-response size check. Overflow returns
native `ErrBudget` with no partial report or block disclosure, and stops loading
remaining block projections. Every widget's private pin is checked before
block/revision deduplication; deduplication cannot bypass custody validation.

The follow-on `go test ./internal/reporting -run '^TestAuthoringLifecycle' -count=1 -v`
passed (0.167 seconds). `TestAuthoringLifecycleProjectionBudget` covers exact
serialized-byte/framing accounting, aggregate overflow across 100 individually
bounded revision projections, no partial output, early stop, and a stale private
digest on a deduplicated block/revision. No additional PostgreSQL run was needed
for this projection-only change; the preceding native lifecycle evidence remains
separately identified above.
