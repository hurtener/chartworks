# Reviewed-dataset chart preparation, version 1

This is the finite dataset-first continuation of [D-097](../decisions/2026-10-03-visual-chart-authoring.md).
Implementation and qualification are tracked separately in
[the evidence report](../reviews/report-app-dataset-preparation.md).
It does not add a query language, model-generated SQL or an identity service.

## Deliberate lifecycle

1. Read an authorized immutable topic and dataset projection. The projection
   preserves reviewed field identity, aggregation, unit and refusal reasons.
2. Stage logical fields and a chart mapping locally. Editing does not query data.
3. Explicitly prepare under a unique operation ID and exact new-block target.
   The server compiles closed intent, validates it through the existing native
   validator and performs a bounded read to observe actual output schema.
4. Consume the exact preparation digest into an unvalidated private block.
   Creation does not reuse preparation as approval or execute a second query.
5. Validate that exact draft through the separate native validation action.
   This is another deliberate bounded source read, never an automatic side effect.
6. Save the private revision/digest in a version-three report and explicitly
   preview through the existing private composition lane. Publication is separate.

No result rows or SQL enter the authoring response. Report edits, status reads,
retained redraw and page navigation cannot implicitly restart source work.

## Shared HTTP and MCP operations

All routes are closed POST DTOs below `/v1/reporting/authoring/v1/`.
MCP names are `reporting_authoring_<suffix>_v1`; the SDK uses the same types.

- `dataset`: retained reviewed-field metadata; native `topics.read` and exact
  topic/source/dataset/context reach.
- `prepare_chart`: explicit bounded source work; native `reporting.validate`,
  chart binding, tenant/new-target/topic/dependency and source-query authority.
- `preparation`: pure retained status by preparation ID or original operation;
  original actor, session, tenant, exact target and current dependency authority.
- `create_prepared`: native `reporting.write`; exact custody digest and fresh
  authority/pin fences, consumed atomically with native block insertion.
- `preparation_control`: explicit inspection/cancellation/reconciliation of an
  existing native source attempt. It is a mutation-capable, open-world operation,
  never marked read-only and never used to start a replacement query.

Every operation rechecks its authoritative native domain contract. Resource IDs,
UI modes, host profiles and the metadata projection never grant authority. Pengui
allocates/authorizes new targets and keeps credentials outside the iframe.

The assembled registry has 91 default tools, or 96 with all five optional
rendition operations, within the existing 96-entry metadata ceiling. Per-schema,
aggregate response, request, concurrency, execution and renderer bounds remain
unchanged. Bootstrap guidance stays bounded rather than dumping the entire catalog.
The four [governed option operations](governed-authoring-options-v1.md) provide
separately explicit dataset/page-filter searches and original-attempt recovery.

## Finite compiler and custody

The first compiler supports PostgreSQL, zero to two direct reviewed dimensions,
one unfiltered reviewed measure, and genuine KPI/table/bar/column/line/area/pie/
donut mappings. It rejects unsupported rule, group-domain, population,
completeness and calendar policies explicitly. No joins, arbitrary expressions,
client SQL, client schema or caller-selected aggregation are accepted. Server
field IDs bind the actual schema; truncated preparation results are rejected.

An immutable operation/input digest is reserved before source work. Identical
replays inspect existing custody, while changed inputs conflict. Unknown or
active attempts retain their liability until native controls establish their
state. Expiry prevents consumption; it does not erase an unresolved attempt.
Current actor/session/tenant/target and source/topic/policy fences are required
before source work, payload projection and atomic consumption. Prepared-origin
rule-absence provenance must survive edits/copies/restores, preventing a later
rule activation from being bypassed by validating an older no-rules draft.

Custody is bounded by tenant and actor record/byte quotas with conservative
reservation charges. The versioned fresh-admission and bounded terminal-cleanup contract below now
supersedes the original no-cleanup disposition. Capacity exhaustion still fails
closed; consumption TTL never proves execution settlement.

## Versioned fresh admission and terminal retention (2026-10-04)

This section supersedes the earlier no-cleanup disposition. It changes fresh
admission explicitly without adding an HTTP route or MCP tool. Existing retained
requests keep their exact hashes: omitted/empty `operation_version` is omitted
from canonical JSON and is never filled in during recovery.

New `prepare_chart` requests must supply `operation_version: "prepare-v1"` and an
operation key `prepare:<canonical Unix seconds>:<32 lowercase hexadecimal digits>`.
Seconds have 1–12 digits and no leading zero. Admission accepts timestamps from
server now minus five minutes through server now plus thirty seconds, inclusive.
The SDK's `NewPreparationOperation(now)` generates a key; callers explicitly set
`PreparationOperationVersion`. Neither the SDK nor a host adapter may silently
replace a key/body after an uncertain response. Retained lookup, exact ownership,
current signed target/dependency/source-query authority and input/target conflict
checks precede new-key, compiler and current-publication admission checks.

An unseen legacy/noncanonical key or missing/unknown version returns HTTP 409 /
MCP `preparation_contract_required`. A canonical fresh-version key outside its
window returns HTTP 410 / MCP `preparation_operation_expired`. These are explicit
migration/clock dispositions, with no source execution. Existing legacy keys
remain readable and replayable while retained. After safely terminal unconsumed
custody is pruned, status is `not_found`; replay receives the migration or expiry
disposition and never automatically starts a new query. A legacy key that happens
to match the new syntax, including an arbitrary future timestamp, remains retained
until its encoded admission window is strictly closed. A database admission guard
also rejects unseen old-writer insertions. Rollback must retain that guard; an old
writer cannot be re-enabled against already-pruned custody.

Before execution, the server seals the validator receipt. The native journal
admission locks the exact preparation, binds its original manifest/attempt 1 and
closes delayed dispatch after terminal settlement. Missing journals, deadlines
and failed logical status alone are not settlement. Guarded pre-dispatch failures
and explicit cancel/reconcile can persist proof of no native admission while
holding that same lock. Otherwise terminal proof requires the original exact
native manifest, actor/session/source/context/operation, closed terminal status,
finished timestamp and stopped/not-issued state. Ambiguous legacy records remain
retained and conservatively charged. Native journal retention preserves linked
attempts until this durable witness exists. Recovery never reconstructs results.

One explicit fresh Prepare may commit a separate cleanup transaction, even if the
subsequent reservation is rejected. It considers at most 100 eligible records,
oldest settlement then preparation ID, using `FOR UPDATE SKIP LOCKED`. Only the
verified requesting actor's records in the signed tenant are candidates; there
is no tenant-wide maintenance grant or scheduler. A record must be prepared,
failed or consumed, have positive durable settlement, have passed consumption
expiry, and have spent at least 24 hours since settlement. Consumed custody also
spends 24 hours since consumption. Active, unknown, unfinished, contradictory or
unproved liability is never removed. Metadata reads and page navigation do not
run cleanup. Cleanup performs no source/model/control calls.

Consumed payload compaction inserts an immutable closed receipt of at most 64 KiB
before deletion in the same transaction. It retains original identity/digests,
source/context/dataset/topic references, terminal witness and exact native
revision 1 definition/execution digests. It contains no SQL, request prose, rows,
tokens or copied technical catalog. Its tenant-composite foreign key points to
the native revision; at most one receipt can exist for a created block. Native
revision/provenance/validation/publication/report and retained-result data are
untouched. Exact Create replay selects revision 1 under current signed authority,
not the latest draft or publication. Safe status schema/mapping comes from that
same authorized native revision. Legacy custody too large for the bounded compact
shape stays retained and charged instead of losing dependency references.

The existing full-payload ceilings remain 10,000 records / 256 MiB per tenant and
128 records / 16 MiB per actor. Receipt bytes and lifecycle witness metadata are
charged to the same byte ceilings; receipt population is additionally bounded by
native block population and its unique block reference. Unsettled, accepted and
uncertain custody reserve the 2 MiB payload maximum plus 132 KiB bounded lifecycle metadata (64 KiB validator receipt, 64 KiB admitted manifest and 4 KiB settlement). This reserves later seal/admission growth before any native work; it does not enlarge the existing byte ceilings. If the caller's own
eligible cleanup cannot relieve actor or tenant quota, admission fails closed.
This is database logical retention, not secure erasure of WAL/backups.

### Required quiesced-writer rollout

This migration supports retained legacy requests, not mixed old/new server
writers. Before applying migration 087 or enabling its cleanup, stop and drain
all older Chartworks writers, including non-authoring source/reporting runners
that can call native read admission or retention. Apply the forward migration
while they are quiesced; restart only upgraded binaries with these admission,
settlement and retention paths. Align the SDK and both application adapters with
the explicit fresh contract. Legacy clients can still recover retained requests;
unseen legacy requests receive the documented migration disposition.

The database fresh-insertion/native-admission guards prevent old or late writers
from silently restarting pruned preparation operations. They do not make old
retention code compatible: an old binary's generic native-journal DELETE can
remove an unwitnessed original attempt before upgraded code copies its proof.
The preparation remains fail-closed, but positive recovery evidence can be lost.
There is no database read-attempt DELETE guard and no claim of rolling/mixed-writer
retention safety. Do not restart old writers after this migration. After cleanup,
rollback must not remove these guards or restore old admission/retention logic;
use an upgraded, compatible forward fix instead.

Pre-087 custody is not automatically backfilled. Explicit control can settle it
only using its stored original attempt coordinates and a matching still-retained
native journal. Missing or insufficient original evidence remains full custody
and conservatively charged, including a logically prepared/consumed legacy row.
Its ordinary retained read and exact consumed Create replay remain available under
current authority. This conservative residual can exhaust admission capacity;
age, a missing journal or an operational rollback is never permission to erase it.

New native admission takes preparation/topic/source row fences with NOWAIT while
holding the existing native journal-retention fence. Compatible locks may admit
immediately; conflicting locks return the existing typed busy disposition rather
than waiting while blocking another source owner's journal progress. Explicit
controls refuse contradictory witnesses as uncertain, even if one historical
attempt was terminal. They never report whole-operation settlement from partial
or conflicting evidence.

This is an ordering requirement for a future authorized rollout. The source-only
implementation and fixture qualification perform no production migration, cleanup,
deployment, provider call or identity/grant change.
