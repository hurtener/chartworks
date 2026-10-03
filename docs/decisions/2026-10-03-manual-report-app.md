# Optional manual report application

### D-096 — One shared app with Pengui-owned authority

Accepted owner scope, 2026-10-03. Supersedes only earlier exclusions of an optional
manual authoring application. API-first remains binding; no separate identity,
policy, query engine, duplicated report state or mandatory standalone UI is added.

## Decision

Build one Chartworks report application and reuse its document schema and retained
rendering components in MCP Apps and a registered embedded iframe. Thin host
adapters carry bounded operations. Preserve `ui://chartworks/report-viewer/v1`
and its read-only bridge; new authoring resources/tools are separately versioned.
Manual use creates no chat, model call or hidden agent dependency. Agent use gets
typed versioned bootstrap guidance with chat/plan/apply modes, scoped targets and
public contract references. Guidance is never authorization.

Builder and Consumer are capability-derived presentation profiles. Pengui alone
owns users, Teams, audiences, grants, entitlements, issuer and credential renewal.
Chartworks retains definitions, reference/dependency eligibility, revisions,
results and enforcement. Every HTTP and MCP operation checks native action and
exact resource/context reach. Read, write and grant are distinct; no role label,
creator label, whole-resource wildcard or profile grants authority. Selected
widget editing is an operation-intent boundary, not a new per-widget ACL. A
narrow patch accepts stable widget ID, expected revision/CAS and allowlisted
fields; the server reconstructs the revision and preserves every untargeted
widget, filter and layout. Whole-report manual save remains explicit report-write
authority. No new widget grant grammar or mandatory edit-intent database is added.

The trusted parent owns credentials and host-only nonce. No token or host-only
nonce enters iframe messages, URLs, browser persistence or logs. Frame challenges
are correlation only. Unknown methods, origins and stale frame generations deny.
Host admission is centrally registered and remains pending until the real no-chat
launch path and least-privilege projection work end to end.

## First slice

Manually create headings and select existing published block outputs (KPI, trend,
table); edit title, layout and typed filters; save immutable revisions under CAS;
reopen authorized drafts and request explicit private preview. Consumer lists
and reads authorized published/retained reports and uses explicitly authorized
run/filter operations. Reading or repainting retained output executes no query.
Published definitions remain immutable and previews remain private.

An authorized draft catalog must filter eligibility before pagination and counts.
Private metadata does not become a published catalog entry. New-object creation
requires explicit parent/container authority and exact new report reach, separate
from existing block/source/topic/execution-context dependencies.

Advanced drag/drop, multi-page authoring, arbitrary natural-language widget
creation, exports and whole-catalog dynamic authority metadata are later work.
A functional first slice is not complete parity or a production activation claim.

## Evidence and release

Use deterministic actual domain/SQL authority negatives, transport schema parity,
CAS/reload/private-preview tests and real browser flows. Test interrupted/repeated
operations, cross-origin and stale replies, capability withdrawal, and selected
widget escape attempts. Record unrun or blocked host paths explicitly. No provider
calls, secrets/configuration changes or deployment are implied by this decision.
