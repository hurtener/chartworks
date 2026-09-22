# CW-08 reviewed-learning adversarial record

## Scope reviewed

This review covers LRN-01 and LRN-02 at the implementation head: feedback and
example persistence, applicability, retrieval/reranking, precedence, immutable
selection evidence, review/retirement, portability, and HTTP/MCP/SDK/CLI
registration. It does not claim phase-24 live quality calibration or the full
phase-34 cohort migration.

## Findings and corrections

1. **Import replay could amplify evidence.** The initial import reused the feedback
   aggregation upsert, so a repeated bundle could add counts. Import now has its own
   atomic insert-or-read path. Exact replay returns the existing row without changing
   evidence; a matching digest with a different origin returns a conflict.
2. **Candidate truncation could hide the most relevant row.** Reading only eight
   score-ordered rows made lexical relevance ineffective beyond that prefix. The
   selector now evaluates a bounded 64-row pool, records exclusions, and still admits
   no more than seven examples.
3. **Review rationale was not durable.** Activation now stores the review note as
   protected metadata alongside reviewer, time and CAS version. It is excluded from
   ordinary projections, prompts, labels and logs.
4. **Feedback identity over-collapsed corrected outcomes.** The uniqueness boundary
   now ignores note-only changes while allowing distinct corrected SQL outcomes.
   Deterministic exact retries remain no-ops.
5. **Origin conflicts returned a driver error.** Feedback/upsert origin conflicts now
   map missing conflict-update rows to the typed store conflict boundary.
6. **Portability was missing an MCP consumer.** Protected export and revalidated
   candidate-only import now bind to the same registered HTTP operations as the SDK
   and generated CLI surface.

No P0/P1 finding remained after these corrections. Learned state never supplies
authority, changes a publication, bypasses the native validator, or modifies a
retained query.

## Verification

The focused CGO-free package tests and vet pass for `internal/nlqexec`,
`internal/nlqapi`, `internal/store/postgres`, `internal/mcpserver`, and
`sdk/chartworks`. The acceptance package compiles with `CGO_ENABLED=0`, including
the real-store concurrency cases. `git diff --check` passes.

The local PostgreSQL acceptance run could not execute because the existing Docker
engine storage returned an I/O error reading PostgreSQL metadata, and creating a new
container failed with a Docker image-blob I/O error. No container was restarted or
modified. Hosted CI therefore remains the required real-PostgreSQL evidence.

`make planning-check` reaches the planning checks but its pre-existing Python
coverage-gate unit tests fail on macOS temporary-path aliasing (`/private/var` versus
`/var`). This change does not modify that gate. D-074 remains intact: no automatic
heavy workflow was added.
