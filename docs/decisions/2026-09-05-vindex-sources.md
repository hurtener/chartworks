# Vector/source/validation implementation decisions

## Initial qualified implementation

### D-064 — Atomic vector generations and qualified PostgreSQL source reads

Date: 2026-09-05. Status: accepted implementation decision.

Implement phases 07 and 08 with the existing hard dependency on the actual phase-09
validated-read contract. Preserve the graph and all individually numbered criteria.
Use the existing PostgreSQL store, authority verifier, service assembly and SDK.

The initial PostgreSQL source/catalog proof is explicitly qualified for major 17;
unqualified majors/exposures fail closed. Operator-owned tenant-bound aliases and
separate credential references define actual source contexts; request labels cannot
narrow broad credentials. Explicit testing reports current health rather than a
fake background-test stub. No issuer, integration vault or local grants are added.

Use native PostgreSQL grammar through the pinned WASM parser plus positive AST and
actual source-native checks. Preserve known-valid CTE/window/set operations while
rejecting unproven positional aliases and name-resolution ambiguity. The read plan
is opaque, nonzero, immutable and rechecked at execution. There is no raw-SQL escape.

Vector search uses full-space, exact-provenance generations, complete atomic
publication, stable bounded batches and no evidence cache. Keep enforced initial
bounds explicit; do not add speculative ANN tuning or a local inference path.
The production Bifrost semantic-generation consumer remains phase 15, the full read
execution product phase 10, and other warehouse drivers phase 14.

See [the implementation contract](../contracts/vector-sources-validation.md) and
[adversarial review](../reviews/phase-07-08-adversarial.md). Status and planning checks
are not substitutes for actual named acceptance, race or coverage evidence.
