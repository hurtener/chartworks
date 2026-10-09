# Manual publication transport and SDK qualification

Local source qualification, 2026-10-04. This transport continuation is based on
native domain checkpoint `28d4ad465f572f0a3501059ce0c20b0ac9f55b75`, including
exact imported-default preservation, the 3 MiB aggregate inspection projection
bound and native report transition capability hints. It does not qualify the
separately developed UI, hosted browser journey or deployed host activation.

## Implemented surface

Four closed POST operations under `/v1/reporting/authoring/v1/` bind the existing
native `Authoring` methods directly, with the same request/response DTOs on HTTP,
MCP and the typed SDK:

- `lifecycle` / `reporting_authoring_lifecycle_v1` / `InspectManualLifecycle`:
  `reporting.read`, retained metadata only.
- `block_publish` / `reporting_authoring_block_publish_v1` /
  `PublishManualChart`: `reporting.publish`, native metadata publication commit.
- `rebind_published` / `reporting_authoring_rebind_published_v1` /
  `RebindPublishedManualCharts`: `reporting.write`, native report CAS amendment.
- `report_transition` / `reporting_authoring_report_transition_v1` /
  `TransitionManualReport`: `reporting.write` editor-entry ceiling plus native
  `reporting.publish` and exact report publish reach for publish/reject.

All operations retain exact current resource/dependency, actor/preview, evidence,
revision/digest and CAS checks. Inspection discloses every output of each entire
block revision. Audience eligibility is neither a new grant nor an audience
count. Publication, selected reference rebind, report review and report
publication remain separate; rejected rebind does not undo chart publication.
The compiled guide requires explicit user confirmation before each publication
and separate confirmation for rebind. Unknown mutation outcomes require exact
inspection, never automatic replay or inferred rollback. No added operation
executes a source or model.

The actual factory inventory is 91 default and 96 with five optional rendition
tools, at the existing 96-tool cap. No old binding, request/response/execution
budget or source-work limit changed. The SDK operation matrix's stale 64-tool
metadata check now uses the existing server cap; it also applies the server's
lowercase tool-name grammar. Existing per-schema and aggregate catalog limits
remain unchanged.

## Completed checks

The restored workspace-local Go 1.27.1/native CGo toolchain was used with the
existing constrained build settings. No PostgreSQL, browser fixture, external
publication or upload was invoked for these checks.

- `go test ./internal/reportingapi ./sdk/chartworks ./internal/mcpserver -count=1`
  passed all three complete package suites: 12.712 s, 13.321 s and 1.482 s.
- `go test ./internal/foundation -run '^TestReportAppFullFactoryInventory$' -count=1 -v`
  passed in 5.002 s, composing all actual default and optional service factories.
- `go test -race ./internal/reportingapi ./sdk/chartworks ./internal/mcpserver ./internal/foundation -run '^(TestAuthoringLifecycle.*|TestAuthoringRegistryParityAndClosedAuthorityInputs|TestReportAppGuide.*|TestReportAppLifecycle.*|TestOperationMatrix.*|TestRegisteredCatalogCapacity|TestManualChartMutationEffects|TestReportAppFullFactoryInventory)$' -count=1`
  passed in 22.759 s, 37.620 s, 1.135 s and 28.350 s respectively.
- The planning checker and its 61 Python unit tests passed. Planning is not
  runtime or security evidence. `git diff --check` and the mirrored
  AGENTS.md/CLAUDE.md comparison also passed.

Initial local registration checks rejected overlong descriptive resource-loader
strings. Those descriptions were shortened to the existing 256-byte bound;
registration limits and native guards were not changed. The successful checks
above include that correction.

## Scope of the assertions

`TestAuthoringLifecycleClosedSchemasAndExactCoordinates` rejects injected
identity/grants/SQL/content fields, missing or null scalar coordinates, out-of-bound
revisions and widget counts, replacement widget content and non-native transition
names. The native service still owns mutually exclusive inspection targets,
unique-widget and nullable-collection cardinality checks.

`TestAuthoringLifecycleHTTPMCPNativeAuthorityAndSafeFaults` uses a synthetic
asymmetric trusted issuer and both real wire handlers, with no repository,
warehouse or model installed. It verifies independent read/write/publish guards,
exact resource denial, the editor-entry ceiling, required rejection note,
wildcard denial, closed DTOs and safe shared error codes with honest MCP
`not_started` versus `unknown` classification.

Registry/manifest parity checks cover DTOs, actions, effects, resource loader,
audit and error contracts. Whole-revision response checks retain all outputs and
explicit false evidence/capability bits, and reject false selected-output or
broadened-audience disclosures. Typed SDK tests preserve exact revision, digest,
evidence, selected widget and rejection note coordinates; every call acquires
current bearer authority and makes only one request, including conflict and
unavailable responses. Generic calls reject automatic lifecycle replay.

Operation-matrix tests use actual lifecycle bindings plus isolated metadata
capacity fixtures for 64, 91, 96 and 97 tools, duplicate names, duplicate operation
IDs and malformed names. The foundation test separately proves actual 91/96
factory composition and unchanged transport/execution settings. These assertions
do not replace the independently recorded native PostgreSQL lifecycle acceptance,
complete foundation/race suites, browser publication journey, deployed authority
projection or final release gate.
