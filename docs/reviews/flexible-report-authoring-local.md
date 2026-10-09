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

## Client experience checkpoint

The reader, chart setup, formatting, publication and recovery controls now use
plain language. Readers get a visible Refresh report action, filters/history
before the canvas, and optional details instead of raw status codes, revision
coordinates or validation identifiers. Data warnings, draft privacy, original
numeric values and uncertain-action fences remain. Pengui sharing explains data
prerequisites and separately granted access in English and Spanish. Field choices
remain schema-driven; useful source field names still distinguish display labels.

A composed synthetic report was created through the public services, published
by an independent reviewer, shared with a Team and read through both HTTP and
restricted MCP. It contains text, a summary number, bars, a daily line and a table
on two pages. This is authenticated API evidence, not signed-in visual acceptance.
The date chart exposed PostgreSQL's abbreviated whole-hour timezone offsets; the
temporal parser now accepts those actual source values without rewriting retained
cells. Native line/area regressions and browser-formatting checks cover those
offsets and the daylight-saving boundary.

Local module/compiled-resource checks, chart race tests, Pengui sharing tests and
the console build pass. Browser fixtures have updated expectations but the new
visual pass remains open: Chrome displayed ERR_BLOCKED_BY_CLIENT when reopening
the app. The owner was asked to restore the browser page. FA09/FA10, desktop/mobile
visual sign-off, the refreshed walkthrough and final service cleanup remain open;
this checkpoint does not establish predecessor visual parity or goal completion.
