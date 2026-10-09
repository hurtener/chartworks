# Flexible authoring local review checkpoint

Date: 2026-10-09. Single-agent implementation and adversarial self-review as
explicitly requested. Companion draft PRs remain unmerged. This is not independent
review or a complete product release.

## Findings corrected

- Independent publishers previously borrowed write admission. Read intersects
  write/publish for worklists; Save, rebind, preview and publication retain their
  own native permission checks. An ordinary viewer-admitted reviewer publishes
  without report write, execute or source query authority.
- A delayed sharing dialog response could target a newly selected report. Each
  request now retains its local cancellation controller; report/session switches
  reset search, selection and preflight state. Delayed read/search/check/write
  and reload regressions pass.
- Newly selected numeric results defaulted to zero decimal places. D-111 preserves
  retained precision, with explicit rounding and reset that preserve old mapping
  bytes and prepared custody. Browser, native formatter, overlay and real-upload
  regression tests pass.
- Mobile multi-column tables compressed values into narrow character strips.
  Readable minimum column widths retain a bounded, keyboard-scrollable table.
  Inspection at 390 pixels confirms page width remains 390 pixels.

## Evidence and limits

The source is schema-driven: physical names/types come from authorized metadata;
reviewed meaning is explicit. Renamed unrelated synthetic schemas, raw duplicate
rows, multiple groups/measures, count and calendar boundaries are covered. No
client schema, business-name heuristic or copied predecessor implementation was
introduced. Source/context fingerprints reject changed physical locations.

Real uploaded CSV data passed author preparation/create/validate/publication,
independent report review, Team-only discovery/run/read, amendment to a different
uploaded schema, missing-data preflight and revocation through public HTTP and
restricted no-chat MCP. Removing one report contribution preserved another
report's chart grant. Data prerequisites were granted explicitly by the local
administrator, never silently by report sharing. Models were disabled.

Go/race checks cover charts, rendering, reporting, reporting API and relevant
native acceptance. Pengui host race, three-store report-sharing conformance,
dialog/bridge/client tests, Svelte checks and console build passed locally.
Planning and documentation checks are separate from runtime evidence.

The screenshot walkthrough uses current compiled UI over captured synthetic
public DTOs, with no credentials or live mutation relay. The authenticated browser
journey is pending the user's local certificate hand-off. Hosted CI is billing
blocked. Final service cleanup and goal closure remain pending; do not interpret
this checkpoint as release approval.
