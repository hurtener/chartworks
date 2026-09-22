# Performance evidence v1

Status: bounded PERF-01 harness and authority-bound Phase 25 prerequisite
implemented for review, 2026-09-22. The original Plan→Run adapter remains a
query-ledger prerequisite and fails closed for result reuse. The draft frozen
adapter now admits distinct reporting run IDs and reads the product's canonical
reuse identity, reuse origin, persisted native source-only duration and
uncopied narrative receipts. A bounded real-PG17 recorded-model test exercises
cold, warm, repeat and concurrent reuse. The selected Phase-34 current-revision
resolver also reads the protected case's published block pins. These are
prerequisites, not the final stress execution or Phase 25 release decision.

Performance evidence is valid only after every case passes a correctness probe.
The harness then records every raw wall-time observation and separately retains
service, source and model time, source/model calls, retries, tokens and cost.
Unknown model time, token usage or cost remains absent. Summary percentiles are
derived from the retained raw observations and never replace them.

P95 uses the nearest-rank definition `ceil(0.95*n)-1` for zero-based samples.
Measurement uses at most the declared concurrency as joined workers;
cancellation is context-bound and does not create one goroutine per iteration.
The evidence hash seals the complete persisted report, including schema,
manifest identity/digest, environment, start/end time, samples and summaries.

The profile binding records hashes of tenant, signed context reach, signed
actions, source revision, reviewed rule revision, topic publication and
reviewed runtime pack. A binding digest supplied by the harness is not proof
that the product uses that key for reuse. The required profile exercises cold,
warm, repeated and concurrent access plus an exact one-field change for source,
rule, context, topic and runtime pack. Cross-tenant and
same-tenant/different-context cases and altered signed-action cases must deny
before source or model work.
Concurrent cold access must produce one physical execution for one exact
identity; broader or stale reuse is a failed gate, not a timing sample.

`allowed` is only the reviewed expected outcome. Authority fields in a manifest
are fixture expectations and can never construct an identity envelope. The
test-only synthetic adapter receives authority from a `_test.go` resolver and
calls the same resolved-resource execution enforcer as the protected runtime;
it never branches on `allowed`. A separate test signs a bearer and obtains its
envelope from the configured verifier before exercising that protected seam.
Concrete adapters return independently collected source/model receipts. The harness
derives execution when a receipt contains physical calls and derives reuse only
when it does not. Every permitted measured request is exactly one of executed
or reused. Reuse permits service overhead only and rejects source/model time,
calls, retries, tokens or cost. Denials independently record zero physical work.

Evidence modes are explicit. `synthetic` uses no source or model and measures
only the harness and identity-aware reuse implementation. `integration` uses a
real PostgreSQL/source boundary with a recorded model adapter. `live` uses the
real source and reviewed live runtime pack. A report records the OS,
architecture, CPU count, Go version, dataset digest and row count, source/model
mode, and the Phase 24 suite ID/revision/digest, report ID/hash and workload
case ID. Results from different modes are not interchangeable. The sealed
report also retains the current cold binding itself (tenant, context reach and
signed actions as hashes plus exact
source, rule, topic and runtime-pack revisions); the samples retain each
scenario binding digest.

`chartworks eval perf-inspect --profile PATH` validates and displays a profile
without executing it. The production CLI rejects `perf-smoke`: an
operator-controlled manifest cannot mint verified authority. The bounded smoke
script invokes only the test adapter, where fixture envelope construction is
confined to test code. An authority-bound release runtime verifies a supplied
bearer through the configured HTTP verifier independently of the manifest. It
loads the exact accepted Phase 24 suite, passing stored report, selected case
and independently accepted runtime pack under that envelope, then checks
profile expectations against those records.

Every allowed invalidation scenario pins its own accepted consumer case and
exact accepted report ID/hash. The current-revision resolver is called for each
resolved scenario; its selected source, context, rule, topic and dataset
evidence must produce the step's binding. The runtime-pack scenario resolves a
separate report so its selected pack and protected input also change together.
The harness rejects a changed binding when the resolver returns only the cold
scenario's evidence. This evidence selection alone does not prove cache
invalidation.

The runtime requires a current source/rule/topic/context revision resolver and
an adapter whose declared evidence, source and model modes match `integration`
or `live`. Every permitted correctness probe and physical execution must carry
real source and selected model receipts. The original Plan→Run adapter binds
durable query/read ledgers but cannot prove product result reuse. The new
frozen adapter uses `reporting.Runs.Admit` and `Run` with fresh operation keys,
reads the protected `RunRecord`, validates `RunManifest.ReuseKey` against
`ReuseIdentity`, and checks `ReusedFrom` and physical source/model receipts.
The protected Phase 24 consumer uses the same frozen result digest. A reused
output's copied narrative receipt is not counted as another model call.
`releaseprofile.NewIntegration` composes a recorded gateway, published block,
request runner and PostgreSQL frozen-run repository. Its resolver selects
one reviewed Phase-34 cohort for each accepted Phase-24 consumer case, reloads
the active cutover and its source adapter, checks current topic/rule/source and
signed context reach, and hashes all rows of a bounded native PostgreSQL
dataset's validator-safe column projection. Multi-dataset source snapshots fail
closed until a shared native
transaction exists. Missing or changed owner evidence fails closed. The
required `runtime_pack_changed` branch still returns
`ErrPerformanceReuseUnproven`: the frozen narrative does not select its model
from an accepted pack in the product path. Final AC03 still needs a real
accepted Phase 24 case/report for each selected revision, one-field current
invalidation and stale-key substitution through the final correctness gate,
the exact final profile, and live owner evidence. Neither this composition nor
its recorded fixtures qualifies a live model or release stress run. The release
orchestration remains internal.

Release reports are atomically created as private `0600` files and an existing
path is never replaced. The one-hour profile bound includes correctness probes
as well as timed observations. A permitted integration/live correctness probe
must carry physical source and model receipts before any timed sample starts.
Read-attempt `created_at` to `finished_at` spans journaling, source work and
finalization; it is never labeled `source_ns`. A nullable `source_duration_ns`
on a PostgreSQL physical read attempt measures the native source work after
connection acquisition through transaction cleanup and subtracts synchronous
read journal calls. It survives the durable execution receipt. Legacy,
unissued and uncertain
attempts remain unknown. The frozen adapter consumes that exact nullable
receipt; missing duration fails the integration timing gate. The signed-action
negative needs a second short-lived Pengui bearer for the same subject/reach
with exactly the active consumer action removed (`reporting.execute` for a
frozen run, or a query action for Plan→Run). Both bearers are verified; the
altered envelope is passed through that consumer, and denial with zero
source/model work is required. Fixture scopes do not construct the envelope.

The checked-in smoke profile caps each step at 32 requests/concurrency and ten
seconds overall. The final release profile uses these required scenarios:

| scenario | iterations | concurrency | reset | expected physical executions |
| --- | ---: | ---: | --- | ---: |
| cold | 20 | 1 | yes | 1 |
| warm | 1,000 | 16 | no | 0 |
| repeat | 10,000 | 64 | no | 0 |
| concurrent cold reuse | 2,000 | 128 | yes | 1 |
| each source/rule/context/topic/runtime-pack change | 500 | 32 | no | 1 |
| cross-tenant denial | 1,000 | 64 | no | 0; all blocked |
| same-tenant/different-context denial | 1,000 | 64 | no | 0; all blocked |
| signed-action denial | 1,000 | 64 | no | 0; all blocked |

The final profile maximum is one hour. Release tooling must materialize current
environment, dataset, Phase 24 evidence and reviewed revision hashes into a
manifest, then use real PostgreSQL/source and separately selected recorded or
live model adapters. This contract defines the run; this change does not claim
that the final stress profile ran.
