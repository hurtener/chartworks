# Performance evidence v1

Status: bounded PERF-01 harness implemented for review, 2026-09-22. Phase 25
still owns the final stress execution and release decision.

Performance evidence is valid only after every case passes a correctness probe.
The harness then records every raw wall-time observation and separately retains
service, source and model time, source/model calls, retries, tokens and cost.
Unknown model time, token usage or cost remains absent. Summary percentiles are
derived from the retained raw observations and never replace them.

The immutable reuse identity contains hashes of tenant, signed context reach,
signed actions, source revision, reviewed rule revision, topic publication and
reviewed runtime pack. The required profile exercises cold, warm, repeated and
concurrent access plus an exact one-field change for source, rule, context,
topic and runtime pack. Cross-tenant and same-tenant/different-context cases
and altered signed-action cases must deny before source or model work. Concurrent cold access must produce one
physical execution for one exact identity; broader or stale reuse is a failed
gate, not a timing sample.

Evidence modes are explicit. `synthetic` uses no source or model and measures
only the harness and identity-aware reuse implementation. `integration` uses a
real PostgreSQL/source boundary with a recorded model adapter. `live` uses the
real source and reviewed live runtime pack. A report records the OS,
architecture, CPU count, Go version, dataset digest and row count, source/model
mode, and the exact Phase 24 suite/report hashes. Results from different modes
are not interchangeable.

`chartworks eval perf-smoke --profile PATH [--report PATH]` executes only a
bounded synthetic `smoke` profile. The optional report path is written by
atomic replacement with mode `0600`. `perf-inspect` validates and displays
either profile without executing it. Integration/live adapters and a
`final_stress` profile execute only from the Phase 25 release runtime after the
Phase 34 migration head is selected; the local smoke command refuses them.

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
