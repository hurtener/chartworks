# Phase 18 NLQ runtime evidence

Status: proposed shipped delivery for PR #11, based on the historical integration
`47345c47aedc9a5de13b35a95106de4afbfdebd7`, 2026-09-08. This record covers the durable
internal query consumer and its Phase 16 invalidation seam. The later public
HTTP/SDK surface is recorded below; this historical record does not by itself
claim live provider quality or release readiness.

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

The public Phase 18 HTTP/SDK operations landed at
`6801a28acbd728578ab1f3329a06bff4fc830f76`; root's strict Phase 18 check passed
all six children. The invalidation consumer landed at `c88dbcc`, and its author
acceptance plus the root full suite at `37f713c` passed all six Phase 18 criteria.
Public transport fixes at `459b91d6990fca2d0676bae6d5e0a76bdc81c3be` integrated as
`ecd08fa`.

The first complete Phase 18 review was `37f713c2ee492536ed15086ecd5e7afa53a364e1`;
the second complete review was `4efbb5a4b4fe61eda20ddf0efe488385660916bd`. The
bounded corrections are integrated as `7493899338f646382a219d336dd272258619a42f`
(unsafe correction rejection), `7bd27f16cf3885ff792ecc76d9053a51fc185efc`
(detached route requests), and `bef6cae` (detached confidence pointer). The
final narrow review found no P0/P1; the PostgreSQL boundary tests
`9422ee3fbdf07f3ae7da3939da2aa49f7c763338` are integrated in `513a3be`.

Correction remains bounded to the existing Bruin validator/executor path. Every
attempt retains exact SQL, bound parameters, plan coordinates and source/context
revision; PostgreSQL may additionally accept the same AST when only locations or
formatting differ. Any other SQL change returns `ErrUnsafeCorrection` without a
second execution, and every attempt has a receipt. Root's exact 513 cumulative
coverage run passed all configured bands, with `internal/store/postgres` at
3039/3593 (84.58%) and `internal/nlqexec` at 579/711 (81.43%); full lint, vet and
build passed. PR #11 records Phase 18 as shipped conditionally; that status becomes
effective after every required hosted check passes and the PR merges. Recorded model
fixtures establish deterministic service behavior only; they are not live provider
measurements.
