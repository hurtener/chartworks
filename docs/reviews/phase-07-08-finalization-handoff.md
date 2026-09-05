# Phase 07/08 finalization handoff

This is not a completion certificate. Recover this existing branch; do not rewrite
it, relax coverage or create a duplicate PR without checking the current PR list.
A draft PR creation request was issued during the tool-output outage, but its
response could not be verified.

## Published implementation checkpoint

The reviewed source snapshot is `8fecc21e96a3dc5db5dcb6bbd5507dcab1d6506c`.
Its development evidence is Actions run `33994776854`, artifact `9977749745`.
The source archive and raw acceptance/coverage logs are retained in that artifact.
Re-read current branch/CI state: later documentation commits may have advanced it.

The branch includes phases 07/08 and the actual phase-09 dependency, including
SQL name-resolution fixes and real PostgreSQL regressions. The earlier recovery
checkpoint's three SQL alias findings have now been addressed in this source:
base-table positional aliases, GROUP BY input/output collisions and nested
ORDER BY alias propagation. The parser mutation/unknown-window/VALUES guards,
source/vector rollback tests and atomic oversized vector-batch regression are
also part of the reviewed source. Preserve these tests.

## Remaining work before a completed PR

1. Finish operator documentation and truthful phase 07/08/09 status/evidence,
   source operation registration, source configuration example and D-064 decision.
   No phase dependency or acceptance count changes are needed: 18 new named tests,
   58 cumulative named criteria across phases 01–09, 25 later workstreams planned.
2. Complete the PostgreSQL-major qualification boundary: source probing currently
   relies on the PostgreSQL 17 catalog/parser behavior. A later major can introduce
   new executable column/relation behavior. Explicitly qualify or reject it;
   do not claim unsupported majors safe based only on parse/EXPLAIN success.
3. Run full uncached race coverage with unchanged 85/80/70 percent package gates,
   all named criteria, planning/drift/mirror, lint, build/vet, lifecycle smoke,
   native SQL/authority fuzzing and CGO-free Linux amd64/macOS arm64 cross-builds.
   Inspect actual results, not just the workflow title or a parent test pass.
4. Remove temporary source-writing helpers and development workflows. Update the
   permanent read-only CI to the pgvector/PostgreSQL 17 reference and all 58 named
   tests, then verify the exact final committed source and unchanged working tree.
5. Replace checkpoint/handoff documents with the final adversarial self-review and
   exact evidence. Mark the existing draft ready only after the final gates pass.

The previous source-writing workflow is temporary and must not be retained as the
final CI architecture. Pengui remains the only issuer/policy owner. Source secrets
stay in trusted references and private connection state, never metadata DTOs or
stored identity tokens. Production vector generation remains the Bifrost-backed
semantic consumer in phase 15; full execution products remain phase 10. Other
warehouse engines remain phase 14. No paid model call, deployment or merge is
claimed by this handoff.
