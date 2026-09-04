# Durable authority sequencing correction

### D-055 — First real platform authority adapter lands with the queue

Accepted implementation sequencing correction, 2026-09-04. Phase06 delivers the thin Pengui authority provider and its first bounded durable consumer. Phase30 extends that same adapter's tests for reporting targets instead of being its first implementation. This closes the hidden late dependency for phases12/15/28 and supersedes earlier phase-30-only wording in the recovered plans and authority handoff.

Pengui still owns all decisions and issuance. The concrete execution-binding/renewal API was not proven deployed by this source review; the owning implementation reads the real broker/minter contract and reuses it or makes a Pengui-owned extension, recording exact fixtures. No guessed endpoint, local issuer, persisted end-user token or ambient-account fallback is allowed. The dependency graph and number of criteria are unchanged: the real first-consumer requirement is part of 06.AC06, and reporting target authority remains 30.AC02.

This is a deliverable before durable-worker acceptance, not host compatibility or authentication reimplementation. Existing retained-result reads do not wait for the platform broker, model providers or scheduling.
