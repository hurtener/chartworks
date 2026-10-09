# Registered-source authoring checkpoint

D-107 and migration 089 extend the existing typed compiler and block lifecycle
with an exclusive registered source origin. No topic or reviewed meaning is
manufactured. Current source, dataset, execution context and block reach remain
independent. The source catalog supplies the schema digest and storage verifies
its exact retained relation and revision. The native API allows the topic field
to be omitted for this origin; the optional fields preserve legacy hashes.

Local qualification:

- The full `TestReportApp` group passes against real PostgreSQL (119 seconds).
- `TestReportAppSourceDatasetNative` includes schema-checked HTTP metadata and
  preparation, no topic permissions, changed pin rejection, revoked source/data/
  context authority, actor custody, exact retained numeric values, validation,
  publication, frozen execution, report composition, private amendment and replay
  after preparation cleanup. Final HTTP/legacy/discovery focused checks pass.
- Phase 27/28/29, frozen-run, document and composition regression groups pass
  locally (270 seconds). This is not a whole-product release or all-driver claim.
- Reporting, reporting API, SDK, source and source API race tests pass. PostgreSQL
  store race tests pass in an isolated run. One concurrent race run hit the
  preexisting expired-authority control test's canceled cleanup; the unchanged
  full store suite passed when run alone. The new migration inventory was updated.
- Planning, mirrored normative files and diff checks pass.

Single-agent adversarial inspection found the companion's topic-only chart
mapping/copy parent, the iframe's legacy field-count restriction, required topic
wire metadata and stale dataset limitation copy. These were corrected with focused
regressions. Source-origin definition guards reject mixed topic/rule/template
meaning; unreviewed sensitivity remains unknown. No independent agent was used.

The goal remains open. Paged source/table/upload discovery, the physical-table
picker, physical-field filters, independent report audience management and current
signed-in HTTP/MCP desktop/mobile journeys still require implementation and visual
qualification. Earlier snapshots and the team-report walkthrough do not establish
those new journeys. Hosted CI is billing-blocked; no deployment or merge occurred.
