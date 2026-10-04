# Deterministic dataset preparation checkpoint

2026-10-03. Initial implementation for D-097; qualification pending.

## Finite behavior

PostgreSQL only. The server compiles zero to two direct reviewed dimensions and
one reviewed measure, preserving its exact aggregation. Selected filters,
completeness, calendar policies, group-domain/grouped-population policies, active
rules, joins and arbitrary expressions are unsupported and fail closed. Retained
metadata lists unsupported concepts with reasons. No client SQL, schema or rows
are accepted. No model generation or repair is used, and no hidden SQL LIMIT is
added.

Explicit Prepare reserves actor/session/tenant/target/operation custody before
native validation and bounded source execution. Actual returned schema determines
output types; truncated results are rejected. Immutable custody stores no source
rows. Same-operation replay never restarts source work. Pure status and explicit
attempt control are separate operations. Unknown outcomes remain uncertain and
use the existing attempt journal, inspection, cancellation and reconciliation.

Create consumes exact prepared custody in the native block transaction. Current
source/topic/rule-absence pins and signed target/dependency/source-query authority
are checked. The result is a private unvalidated draft. Native Validate is a
second deliberate source read, followed by the existing private-preview lane.
There is no implicit publication, certification or transferred validation.

## Bounds and incomplete lifecycle

Consumption expires after at most 15 minutes. Reservation serializes quota checks:
10000 records / 256 MiB per tenant and 128 records / 16 MiB per actor; each accepted
reservation charges its 2 MiB maximum. Automatic expired/terminal cleanup is not
implemented. Records, including unknown execution liability, remain retained;
quota exhaustion fails closed. Source/topic foreign keys preserve ownership
references. Independent report deletion does not own blocks or preparations.

Prepared-origin revisions now retain exact rule-absence topic pins in server-owned
provenance. Copy, edit, restore, rename and parameterization preserve those pins;
attempted topic/rule rebase fails closed. Native validation, preview, frozen runs
and composition checkpoints recheck absence. Transactional fences take sorted
exclusive topic-head locks before source locks, blocking concurrent first-rule
publication. Unrelated legacy definitions/provenance are not rewritten. Native
migration accepts only a closed CreateRequest, so a supplied revision/SQL-view
provenance payload is rejected rather than silently stripped. This correction is
implemented but still requires qualification.

Preparation status, replay and consume apply signed target/dependency reach in SQL
before projecting SQL-bearing custody. Errors return empty records. Reservation
and terminal transitions append content-free audit events atomically.

## Evidence and remaining gates

The first source set was reconstructed after workspace replacement. The recovery
was formatted and checksummed. This integration was statically reviewed against
the restored native interfaces; its application patch was checked. The initial
integrated snapshot passed focused Go race checks through the parent. Its first
real PostgreSQL journey failed at fixture dataset selection before Prepare; the
fixture now selects the reviewed revenue field's exact dataset. The subsequent
rule-origin, preprojection, audit and end-to-end tests have not yet run. Earlier
mapping, private-page and browser results do not qualify this compiler.

Required next checks include compiler/schema/service tests; real PostgreSQL
migration and concurrent prepare/create replay; changed-input conflicts;
cross-tenant/actor/session/target and missing-grant negatives; source/topic/first
rule activation races; partial-commit rollback; unknown/lost/expired attempt
control; overflow rejection; quota concurrency and erasure behavior. The complete
manual dataset-to-private-report UI journey still needs execution in both adapters.
The parent integrated the five operations through HTTP/MCP/SDK/bridge in a
separate checkpoint. The end-to-end test now includes saved private pages,
explicit composition execution and exact retained values, plus actual rule
publication before copied-target validation and after preview sealing. A second
real-source test pauses validation while publishing a rule set concurrently.
