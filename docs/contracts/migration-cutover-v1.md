# Migration and cutover v1

This contract defines the neutral Phase 34 migration bundle and the only supported
cohort cutover state. It transfers domain definitions and retained evidence through
public Chartworks service seams. It is not an authority, credential, source database
copy, or compatibility API for another product.

## Manifest and graph

`chartworks-migration-v1` accepts a maximum of 10,000 objects and 10 MiB. Every
object has a stable external reference, positive source revision, a `v1` JSON payload
encoded as a bounded string on the public wire,
explicit lifecycle, privacy flag, origin, retention, exhaustive top-level field
ledger and zero or more dependency references. The closed dependency order is:

`source -> upload -> profile -> topic -> rule -> template -> runtime_pack -> evaluation_suite -> block -> report -> dashboard -> filter -> schedule -> run -> artifact -> rendition -> certificate -> tombstone -> calibration`.

Parents must exist earlier in that order and the graph must be acyclic. A tombstone
also identifies the exact target kind, external reference and source revision; its
durable target fence prevents that revision or any later bundle from resurrecting
the deleted object. Source,
dialect and engine identifiers are explicit. A source object carries the exact
engine, dialect, source revision, context and canonical source-binding snapshot and
requires an exact destination mapping. Dry run and every apply re-resolve that
destination, repeat its bounded health probe and reject any mismatch. Upload and
profile objects map to current healthy governed objects at the exact revision;
the manifest does not carry uploaded bytes, connection strings or warehouse
credentials. Topics, rules, reviewed examples, blocks, documents and schedules
use their owning public services. Filters remain part of immutable document JSON
and have a separate reference checkpoint. Historical runs, artifacts, renditions
and certificates remain private quarantined evidence until a new operation checks
current signed authority and creates fresh target evidence.

Phase 24 runtime packs and evaluation suites use the owning evaluation service.
Both import only as immutable drafts. An exact conflict is reconciled through that
public service after a crash between its commit and the migration checkpoint;
different material fails closed. Imported runtime packs never select a default,
and imported suites never gain an acceptance receipt. Historical evaluation runs
remain ordinary quarantined `run` evidence.

Each top-level payload field has exactly one `retained`, `transformed`, `dropped`
or `unsupported` disposition. Non-retained fields require a reason. Unknown fields
are rejected because they have no loss-ledger row. Token, password, secret, API-key,
credential, user, role and grant shaped keys are recursively rejected after case
and punctuation normalization, including camel-case and separator variants. Every
object is private; public import is rejected before persistence. A calibration
candidate is a `calibration` object whose closed payload is the owning evaluation
service's proposal request. It is imported as a durable private, unreviewed
optimization candidate and never activates a prompt, model, threshold or learned
example. The legacy top-level `calibration` member is rejected because it has no
apply/checkpoint path; its presence cannot produce a completed import.

Installed typed payloads also pass the owning closed decoder, so a ledger row
cannot make an unknown nested domain field silently disappear. An object whose
declared retention already expired is quarantined before any owner adapter runs.
Resume repeats current bearer, retention, owner state, destination
health/revision/context and adapter validation immediately before each apply. A
stored dry-run plan is never treated as current authority.

## Feature evidence and readiness

The bundle contains exactly the required B01-B20, R01-R16, Q01-Q10 and N01-N16
feature rows plus Q11. Required rows must be marked `required`; a cohort is ready
only when each row resolves to a distinct owner-produced case comparison inside a
current accepted live evaluation suite and report. The exact feature case must be
held out and passed, with its expected and observed values bound by a comparison
hash to the source engine, dialect, snapshot and revision. The exact suite digest,
run ID and report evidence hash must match, with a passed gate and zero security
failures. A suite frontier or caller-provided outcome text cannot unlock readiness. Q11
is always `excluded` and `unsupported`. Evidence records identify their migration
feature, source, exact source version, reference, evidence hash and case-comparison
hash. `owner_feature` must equal the migration feature, so it cannot redirect a row
to a generic suite frontier.
A file name or inventory does not count as a passing comparison. Private owner-run
comparisons can supply references without placing private fixtures in this repository.

Dry run validates the complete graph, current destination reach, mappings, field
ledger and evidence. It writes no domain state. Import stores the immutable manifest
and plan, then applies dependency-ordered checkpoints with exact revision CAS.
Replaying the same batch and digest returns the existing result; changing the digest,
external revision, destination or tombstone state fails closed before domain effects.
The same external reference and source revision cannot be repointed by a new
manifest. Resume locks and rechecks the exact reservation immediately before an
owning adapter effect, and holds it through the matching checkpoint. A later source
revision may replace an idle reservation, but an already superseded batch cannot
call its adapter. Once an owner effect begins, a later revision waits for that
effect and checkpoint to finish. Domain adapters are required
to reconcile their stable destination or use an owning idempotency seam
before returning success; local checkpointing is not a claim of cross-system
exactly-once execution.

Export returns bounded pages of the stored neutral manifest only with both
`migration.read` and the existing `ops.read` plus signed `cw.tenant.export` reach.
It rechecks object retention before disclosing any page. It does not export a
bearer, current approval, warehouse secret or new certificate. Erasure is bounded,
requires current `erase` reach, refuses an active cutover, removes online manifest
payload/checkpoints and retains only the non-secret external-reference/tombstone
continuity needed to prevent replay resurrection. Backups, replicas and WAL expire
under operator retention; the API states this boundary explicitly.
A declared legal hold blocks online erasure; releasing a hold is an owner/operator
retention action outside the immutable imported batch.

## Authority and lifecycle

Every request uses a current Pengui verified envelope. `migration.read` needs tenant
read, `migration.write` and `migration.cutover` need tenant write, and
`migration.erase` needs tenant erase. Adapters additionally enforce the normal
source, topic, reporting, job and execution-context actions and reaches of each
addressed object. A migration action never widens those checks. No historic token,
user, role, grant, certificate or privacy label becomes current authority.

Imports default to private draft or review-candidate state. Published definitions
remain immutable; import creates a new private revision or quarantine record.
Historical approval and current health are separate. Retained values are not exposed
by this coordinator; their normal read services revalidate tenant, source context,
privacy and current signed reach.

## Schedule handoff, cutover and rollback

A cutover requires a completed batch, all required evidence verified, one imported
and checkpointed disabled target schedule, a current enabled prior schedule, and
signed `scheduling.write` reach to both. The prior route revision and actual last
accepted operation/due time must equal the manifest boundary. Critical source or
schedule quarantine, including expiry after import, blocks cutover. Immediately
before route mutation, the source adapter rechecks observed health, exact
engine/dialect/revision/snapshot and the cutover actor's signed source and execution
context reach. The boundary
contains one stream identifier, prior schedule version, last accepted
occurrence/due time and resume-after time. The cohort generation CAS selects one active route. Replaying the identical route
is idempotent; a stale or competing generation conflicts. The cutover transaction
disables the prior schedule and enables the target. Occurrence admission locks the
current generation, rejects the inactive route and due times at or before the resume
boundary, and deduplicates both schedules by logical stream and exact due time.
Queue claim repeats the stream, generation, route, schedule revision and resume
boundary checks against the durable occurrence admission. Claims and route changes
share the queue's transaction advisory fence. Rollback advances the
generation and route revisions, so queued work from the prior route cannot be
claimed afterward. Imported schedules are created paused.

Rollback atomically disables the target, enables the retained prior route under a
new generation and records every
known irreversible effect. Delivered notifications, committed external writes and
expired remote side effects are not described as undone. Cutover and rollback
events are append-only and attributed to the current actor plus an operator drill
reference.

## Surfaces and limits

The seven HTTP operations in
[`chartworks-migration-operations.json`](chartworks-migration-operations.json) are
also typed SDK methods, MCP tools in the optional `migration` group and generic CLI
operations. CLI use reads JSON from stdin and requires the existing explicit
`--execute` acknowledgement, for example:

```sh
chartworks client call migrationDryRun --execute --input - < manifest-request.json
```

MCP/HTTP are thin adapters over the same service and do not retry mutations.
Malformed/unknown input is 400, absent records are 404, stale state is 409, body
limits are 413, unsupported/not-ready cohorts are 422 and dependency failures are
redacted as 503/504. Migration state never stores raw SQL, result rows or credentials
unless they already belong to a protected domain definition accepted by that
domain's public seam.

Phase 24 supplies the reviewed suite/runtime-pack lifecycle and Phase 33 supplies
guided setup objects through these neutral hooks. Owner-run live comparison hashes
remain required and cannot be replaced by fixture or planning status. Phase 25
remains the final release and operational qualification gate.
