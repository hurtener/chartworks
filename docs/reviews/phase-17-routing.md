# Phase 17 routing slice evidence

Status remains `in_progress`. This record covers the bounded routing consumer
introduced on the phase-17 branch; it is not a phase closure claim.

The consumer admits each current topic through `topics.Service.Contract`,
which rechecks source discovery, source revision, dataset bindings, current
publication state, and all dependency reach before the gateway. A read-only
vindex explain admission also checks that the current context has a ready
generation in the exact embedding space before question text reaches Bifrost.
It then uses
the existing `rulesets.Service` read/evaluate seam, the Bifrost `gateway.Engine`
for one query embedding and optional reranking, and the existing `vindex.Service`
for one repeatable-read batched search. Candidates are copied and admitted by
the gateway before reranking. Multi-topic requests require every selected
publication to independently confirm the same normalized one-to-one relationship
with the same source and execution context; unrelated same-source joins, other
cardinality, or unconfirmed joins return typed clarification. The sealed,
token-budgeted context records the full ordered topic/version set. An unqualified
metric ID shared by multiple selected topics is rejected before Bifrost instead
of being resolved by topic order.

The HTTP registry is `POST /v1/nlq/routes` with the existing `topics.read`
action, and the SDK forwards the current Pengui bearer through `RouteNLQ`.
The vector service remains a deliberate no-evidence-cache boundary. The only
embedding cache available to this slice is the gateway cache keyed by the full
verified call, embedding-space descriptor, and exact input text.

The initial full review found two reachable P1 defects: multi-topic join admission did
not prove the same relationship across every selected topic, and unqualified duplicate
metric IDs could resolve by topic order while the model context exposed only a singular
topic/version projection. Checkpoint
`ba65fa7b731a69182a8d93be86d5c5b86130b93f` requires independently authored matching
relationships, rejects unrelated same-source joins before Bifrost, rejects ambiguous
unqualified metrics, and seals the complete ordered topic/version set inside the
token-budgeted model context. Focused multi-topic, duplicate-metric and actual prompt
assertions cover those corrections.

Root independently verified `ba65fa7`: all `internal/nlq`, `internal/nlqroute` and
`internal/nlqapi` race tests passed, and strict `TestPhase17/AC01` through `AC06` passed
with zero skips. The second independent review of the corrected head reported no open
P0/P1 or actionable local P2, closing the bounded routing review round. Earlier root
HTTP/SDK/runtime-OpenAPI checks also passed after the concrete route and auth metadata
were integrated, and targeted package coverage was 82.85% for `internal/nlqroute` and
83.33% for `internal/nlqapi`.

Phase 17 remains `in_progress` pending hosted CI and dependent release integration.
Its acceptance uses recorded Bifrost responses; live semantic quality has not been
measured and is not claimed here.

## Current integrated disposition

The routing review round remains clear: the two P1 fixes are recorded at
`ba65fa7b731a69182a8d93be86d5c5b86130b93f`, and root's strict cumulative routing
check at `492c9fbf6f7fc7f6fe2f5659d2499afdbb3409b3` passed all six Phase 17 children
with zero skips. The exact 513 cumulative race/coverage run passed every configured
band, and full lint, vet and build passed. This updates the earlier bounded wording
without changing the phase's `in_progress` status; hosted CI and release integration
remain open. Recorded gateway fixtures do not establish live semantic quality.
