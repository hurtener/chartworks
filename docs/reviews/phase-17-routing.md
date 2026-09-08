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
the gateway before reranking. Multi-topic requests require one published
one-to-one join per topic with the same source and execution context; other
cardinality or unconfirmed joins return typed clarification.

The HTTP registry is `POST /v1/nlq/routes` with the existing `topics.read`
action, and the SDK forwards the current Pengui bearer through `RouteNLQ`.
The vector service remains a deliberate no-evidence-cache boundary. The only
embedding cache available to this slice is the gateway cache keyed by the full
verified call, embedding-space descriptor, and exact input text.

The six named `TestPhase17/AC01`–`AC06` real PostgreSQL/Bifrost acceptance
subtests and Linux race/coverage evidence remain required before this phase can
move from `in_progress` to `shipped`. Phase 18 generation/execution remains a
separate in-progress dependency.
