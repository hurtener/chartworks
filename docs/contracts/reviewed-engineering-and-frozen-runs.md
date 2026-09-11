# Reviewed engineering and frozen runs

Phases 26 and 28 extend the existing source, validation, managed pipeline,
request-operation and reporting services. They do not create an issuer, grant
resolver, scheduler or model client. Implementation is in progress; the owning
phase criteria and exact-source CI are the completion gates.

## Frozen execution

`POST /v1/blocks/{id}/runs` reserves the caller's key and seals the exact block
revision, typed parameters, logical window, selected outputs, source partition,
privacy, policy and limits. `POST /v1/reporting-runs/{id}/execute` executes one
bounded attempt; `resume:true` explicitly resumes a recoverable interrupted
operation with current supplied Pengui authority. No bearer is persisted. This
uses the shared leased request runner. Unattended reporting schedules and their
fresh broker authority remain owned by phase 30.

`GET /v1/reporting-runs/{id}` reads artifact summary metadata. `/receipt` reads
actor/session-private execution metadata. `/rows?offset=0&limit=100` pages exact
normalized values; `/output?output=output-id` reads one saved output. Listing is
`GET /v1/reporting-runs?after=run-id&limit=20`. Current signed target and actual
context reach are required. Private previews also retain original actor/session
and preview restrictions after subsequent publication. These reads do not call a
warehouse, broker or model.

`POST /v1/reporting-runs/{id}/cancel` records durable cancellation intent.
`POST /v1/reporting-retention` accepts `{"limit":100}` and erases expired values
and outputs while retaining content-free tombstones. The latter requires
`reporting.retention` and exact signed tenant erase reach. An expired key or
artifact never silently executes SQL again.

One normalized query feeds selected deterministic outputs. An explicit selection
preserves request order; an empty selection uses definition order. Narratives
are opt-in and use the shared gateway's `narrative` role. Their reviewed model
policy version must match `reporting.execution.model_version`; their response
schema is `grounded-narrative-v1`. Allowed and redacted field lists are disjoint.
The bounded provider response selects evidence claims; local formatting only
emits supported claims with retained model, prompt, schema and usage provenance.
No query tools, SQL correction or chart selection run in this lane.

The operation fence guards every result/output checkpoint and final commit.
A lost checkpoint reply reuses committed normalized values. Lost successful
source values are reported incomplete; indeterminate native attempts are not
silently repeated. Paid narrative reservations survive interruption.

## Reviewed engineering

`POST /v1/engineering-proposals` accepts the source/context, destination pipeline,
goal, expected pipeline version and freshness bound. It plans blind requirements,
then matches authorized source relations and validates proposed SQL through the
ordinary validator. Every proposed pipeline/dataset object retains evidence,
alternatives, rationale, author, model/prompt versions and usage.

`GET /v1/engineering-proposals/{id}` reads material and actual effects. `PUT`
appends edited material and invalidates approval. `/review` separately approves
or rejects exact version/revision/digest, with signed reviewer identity;
submitters and material authors cannot approve themselves. `/apply` uses normal
pipeline write, publication, execution, ownership and quality gates in addition
to `engineering.autopilot.apply`. Proposal approval alone grants none of them.

`/compensate` retires only a new, still-current, owned managed generation without
published dependents. It records quarantine, not physical deletion or global
rollback. `/drift` records deduplicated schema/freshness/quality evidence and
currently authorized affected topic/block identifiers. `/amend` accepts the exact
drift ID and creates one ordinary editable proposal with immutable parent/evidence
links, a current source partition and expected pipeline version. It retains no old
approval. A changed pipeline head or unavailable current context fails closed.
Physical schema observation uses the existing read-only source probe. Published business meaning
is never automatically changed.

Each operation registers its closed schema, signed action, resource loader,
side-effect classification, errors and audit contract through
`reportingapi.RuntimeRegistry`. Typed SDK methods consume the same domain wire
contracts; the registry-driven CLI exposes the registered operations.

## Configuration

`autopilot` defaults disabled. Enablement requires an existing configured gateway
and managed pipeline runner. Limits default to 8 steps, 4 provider calls, 65,536
tokens, one minute, 512 KiB proposal material, 64 KiB evidence, 10,000 proposals
and a one-hour amendment deduplication interval. `autopilot.model_version` is
required when enabled. These operational settings never grant authority.

`reporting.execution` defaults to seven-day retention, one-day private preview
retention, one-minute attempts, 1,000 rows, 1 MiB normalized results, 16 MiB per
artifact, 256 MiB retained bytes per tenant, 20,000 requests and 200 rows per
page. Reuse requires explicit request consent and defaults to at most one hour.
Narratives allow at most four calls, 32,768 tokens and 15 seconds. An empty
`model_version` leaves narrative execution unavailable while deterministic
execution and artifact reading remain usable. All values have constructor and
configuration bounds in `internal/config`.

## Private topic changes in L2

An optional `topic` goal names the topic, profile, version, name, description and
expected private draft revision. Preparation reads exact authorized profile
metadata without creating a draft. New topics start as profile-backed scaffolds;
existing drafts preserve their semantic entities. The workflow never invents
business meaning to fill an unresolved scaffold. `PUT` may include an edited
`topic` pack; compilation, canonical references and exact source/profile checks
apply, and the material receives a new proposal digest with no inherited review.

Applying the independently reviewed proposal saves the exact private topic through
normal topic authoring after the managed pipeline succeeds. Topic read/write and
profile/source authority remain necessary. The effect journal checks the actual
draft head, digest, actor/session and proposal note before marking the proposal
applied. A failed topic save leaves the already completed pipeline visible as a
partial effect; retry reconciles the same private draft instead of claiming a
cross-system transaction. Compensation is blocked for these multi-object applies.
The ordinary topic review/publication operations remain necessary for publication.

This consumer currently uses one exact profile-backed dataset per topic change.
A schema-drift amendment may supply `topic_profile` selecting freshly prepared
profile evidence; it cannot manufacture a profile or silently rebind old evidence.


## Reviewed schedule changes

An optional L2 `schedule` goal contains `binding_id`, a real cron/interval/manual
`spec`, and an optional existing `id` with `expected_revision`. Planning records a
schedule decision without creating a schedule. An edit may replace the reviewed
`schedule_spec`; the original request remains immutable and approval is invalidated.
Apply requires current scheduling authority in addition to ordinary pipeline
permissions. It first completes managed execution and any requested private topic
change, then creates or replaces the exact reviewed pipeline schedule. The final
proposal effect verifies the actual schedule, revision, request and retry key.
A topic/schedule proposal is not marked applied after only its pipeline succeeds.

Schedule amendments replace the recorded schedule ID with CAS rather than creating
a second refresh. Already accepted occurrences retain their old windows, revision
and target; replacement governs future admissions. Paused schedules remain paused.
The exact revision receipt reconciles a lost replacement reply; an intervening
pause or unrelated change cannot be adopted as that replacement. Multi-object
compensation remains explicitly blocked rather than claiming to undo schedule or
warehouse effects atomically. Service execution obtains fresh platform authority;
no user bearer is persisted in either proposal or schedule.
