# Phase 18 NLQ runtime evidence

Status: core delivery evidence based on integration `47345c47aedc9a5de13b35a95106de4afbfdebd7`,
2026-09-08. Phase 18 remains `in_progress`. This record covers the durable
internal query consumer and its Phase 16 invalidation seam; it does not claim
the final public HTTP/SDK integration, dual review, live provider quality, or
release readiness.

## Implemented boundary

`internal/nlqexec` consumes the sealed routed context and the existing gateway,
validator, and executor seams. It persists session, query, feedback, and
example state through the PostgreSQL repository. SQL is retained as protected
metadata and is returned only with the separate inspection authority. Validation
and execution correction each have one bounded retry, while uncertain and
non-query failures remain terminal. Refinement remains tied to the signed
session and original topic set.

The PostgreSQL NLQ reader consumes
`topic_rule_evidence_invalidations` on both query and operation reads. A matching
ordered topic/rule pin marks the query evidence stale and records a bounded
`rule_evidence_stale` error marker in the returned projection. `Run` then uses
the exact retained topic/version set through `RetainedContract`, resolves the
current signed source binding, and sends the original validated candidate
through the existing validator and executor. It does not call a second rule
evaluator, rewrite an immutable query pin, or widen authority after a rule
publication or retirement.

## Acceptance and unit evidence

The nested `TestPhase18/AC06/rule-evidence-invalidation` regression uses the
real PostgreSQL repository, native validator/executor, and recorded gateway
fixture. It proves that:

- a query planned before any active rules can activate and execute without an
  invented rule pin;
- publication of rules v1, replacement by v2, and retirement of v2 each mark
  queries carrying the old rule pin stale;
- a multi-topic query preserves ordered topic/rule arrays, including an empty
  rule slot for the topic without rules;
- stale replay remains successful through the exact retained topic version and
  current source binding; and
- retained v1/v2 rule definitions and the historical topic digest remain
  unchanged after publish, replay, and retirement.

The focused Linux/native race runs pass:

```text
go test -race -count=1 ./internal/nlqexec ./internal/store/postgres
go test -race -count=1 ./test/acceptance -run '^TestPhase18$'
```

The unit and acceptance profiles cover 392 of 482 `internal/nlqexec`
statements in their union (81.33%). The package is now included in
`scripts/coverage-bands.conf` at the required 80% band; the full repository
coverage gate remains a final integration check because it includes the public
Phase 18 surface and all earlier packages. The native acceptance evidence is
retained in `/tmp/chartworks-phase18-delivery-strict-47345c4.log`, with the
focused run in `/tmp/chartworks-phase18-delivery-acceptance-47345c4.log`.

## Remaining integration boundary

The public Phase 18 HTTP/SDK operations and their actual registered schemas are
owned by the separate public-surface handoff and must be verified together with
this core at the integration head. Phase 16 and Phase 18 retain `in_progress`
status until their cumulative named acceptance, coverage, independent review,
and final CI gates pass. Recorded model fixtures establish deterministic service
behavior only; they are not live provider measurements.
