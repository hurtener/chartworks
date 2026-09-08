# Warehouse read execution substrate

### D-067 — Adopt a pinned minimal Bruin leaf-client fork for new warehouse reads · accepted

Chartworks adopts the public fork `https://github.com/hurtener/bruin`, branch `codex/chartworks-read-contract`, based on upstream Bruin `v0.11.749` commit `e4ac0114dc18cf3b79b176a85dbef5060d5cf4d4`. The current reviewed adoption baseline is `dfbfa1746e7064b91de3066df152a19cc553377b`. The final qualified connector and runner source commits plus artifact digests remain implementation evidence and must replace this baseline where they advance it. This decision does not authorize a floating branch dependency.

New phase-14 engines use selective in-process leaf clients from that minimal fork. They do not use the universal connection manager or stock CLI query command. The fork contract must expose schema before rows, bounded iteration/backpressure, typed parameters, stable native type/value representations, pre-acknowledgement query identity, explicit cancellation/status reconciliation, and lifecycle close/credential rotation.

Chartworks remains the sole owner of Pengui authority, opaque validated plans, positive dependency/function safety, row/encoded-byte/time/cost limits, attempts, audit and retry decisions. Engine controls use the closed tagged identity contract. The qualified native PostgreSQL implementation remains until the forked PostgreSQL path passes parity; uniformity alone is not a migration reason.

Real PostgreSQL/MySQL and Linux-amd64 SQL Server fixtures qualify local subsets. BigQuery, Snowflake and Databricks may be implemented with synthetic recorded protocol tests under the accepted no-live-cloud boundary. Missing live evidence limits advertised cutover/support qualification. Explicitly configured execution still requires all available native runtime proofs, and each unknown individual capability denies explicitly.

This supersedes D-036 only for the warehouse **read** substrate. D-036's pinned Bruin CLI boundary remains active for phase-13 managed writes.
