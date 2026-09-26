# SQL recovery adversarial P0/P1 review

## SQL recovery status — 2026-09-25

The [AP-00–AP-08 completion tracker](sql-recovery-completion.md) records the current
PR #62 implementation and qualification gaps. The recovery is **in progress**.
Existing shipped phase labels and historical defect-review results do not
close this subsequent extension. No required behavior is discarded by this
tracker correction; prior named acceptance criteria and historical evidence stay.

Current qualified runtime: `93539cc` (tree `193cf00`). S1 and S8 are complete
within their documented software scope. S8's Go 1.26.4/1.27.1 native suites passed;
S2–S7, S9–S12 and Q1–Q3 remain open. Exact counts and acceptance boundaries live
in the linked completion tracker. This mirror does not recertify historical code.


Baseline: PR #62, `c8c0a8db9327b147f313830037fcc56353536909`.
This is an adversarial review of the current recovery implementation, not a new
capability slice or a blanket release approval. Exact final CI/source evidence
and outstanding findings are recorded in the PR and delivery audit.

## Findings and regression oracles

### P2-A — analytical parent-only gap already blocked by source admission

`SELECT sum(amount) FROM ONLY analytics.sales` names the same parent relation but
excludes descendant/partition rows. The analytical-only checker ignored RangeVar.inh.
Its isolated regression fails on the baseline, but actual integration established
that source admission rejects inherited/partitioned relations, before generation or
native planning, for both ONLY and ordinary scans. This is defense-in-depth (P2),
not a demonstrated baseline execution bypass or P1. Admission was not weakened to
manufacture reachability or to make an impossible two-model-call assertion pass.

The analytical proof now requires the ordinary descendant-inclusive scan. Its
isolated regression checks ONLY's semantic difference. The native fixture first
runs the supported parent-only schema, verifying total 30, saved metadata and
model/read-free replay. It then creates a child row and independently observes
ONLY total 30 versus ordinary total 130 using the privileged fixture connection.
Both new Plan attempts and a previously approved but unexecuted plan are blocked
by current source capability checks before provider or physical read work. No
application result of 130 or bounded ONLY correction is claimed for that rejected
source topology. Historical terminal results are not silently recertified.

### P1-B — AST literal storage can conceal integer division

Numeric-looking strings are SQL unknown-type literals, not decimal evidence.
`count(id)/'2'` returns an integer; `(count(id)/'2')::numeric` only widens the already
truncated value. The previous checker parsed the string as a rational constant
and treated every non-ival node as decimal, incorrectly proving a decimal ratio.

The same bug affects unquoted integer constants that PostgreSQL's raw parser
stores in fval when they exceed int32 but still fit bigint. For example,
`count(id)/2147483648` is still integer division. The checker now accepts arithmetic
literal evidence only from actual integer/numeric nodes and independently checks
the signed-bigint boundary. Explicit numeric widening before division and true
numeric literals remain supported; typed comparison literals are unchanged.

Tests include bigint/int32 boundaries, values above 2^53, outer casts that do not
repair truncation, ordinary decimal literals, explicit numeric operands and a
beyond-bigint numeric positive. Actual PostgreSQL returns 1 for the quoted divisor
and 1.5 for the correctly widened three-row case. Native safety, failed/no-read
planning, one correction, exact results and terminal replay are exercised.

References: PostgreSQL 17 `SELECT` documentation (FROM ONLY/inheritance), lexical
structure section 4.1.2.6 (numeric constant typing), and mathematical operators
section 9.3 (integer division). The actual result assertions are the primary
software regression oracle, not string matching or model confidence.

### P1-C — durable query failures do not reach correction; setup must finalize

The real Executor finalizes a failed read in its durable receipt and returns a
nil Go error when journaling succeeds. NLQ required both code=query_error and a
Go ErrQuery, so real data-dependent failures never reached the promised bounded
correction path. The original integration test exposed this boundary mismatch;
it was not changed to inject an artificial error into the real executor.

The consumer now admits a nil-error query failure only with failed/query_error,
confirmed stopped remote state, a terminal timestamp and no result rows. Unknown,
running, unissued, cancelled, timeout, context-changed and generic error outcomes
remain terminal without correction. A real PostgreSQL division-error fixture gets
exactly one equivalent correction and two failed read attempts, then immediate
terminal replay without more model/read work. It does not invent a different
meaning merely to make the source query succeed.

The adjacent correction-context error path previously returned before finalizing
the query operation after a typed failed attempt; its baseline unit reproduction
strands replay. Now that real durable failures can reach it, finishRun is essential:
it records preparation failure with zero model corrections and preserves accepted
SQL, bindings, notes and proof. Real PostgreSQL plus old/invalid retained generation
context covers durable failure and bounded replay. The ordinary Executor interface
and its durable outcome semantics are unchanged.

Cancellation, repository outage and process-crash recovery remain governed by the
existing execution infrastructure, not claimed solved by this local terminal path.

## Verification strategy

The read-only recovery workflow copies only the new regression tests into an
isolated worktree at the exact baseline. All ten top-level baseline tests must
fail at the intended assertions, with no compilation failure, panic or skip.
Those expected-red tests are logged separately and never counted as passing
current-head tests. The current tree runs the complete existing race/acceptance
matrix plus six new result-based acceptance tests. Existing private-predicate,
readiness, lineage, saved-query, parameter, learning and effective-budget cases
remain selected. No test is removed to obtain a green result.

One additive migration (058) admits the already emitted detail-free query_error
journal code; old attempts and shipped migrations are not rewritten. No public
schema, analytical version, SQL positive-list expansion, broader authority, extra
model role or frozen-report generation is introduced. Hardening rejects incorrect previously accepted candidates; it
does not silently recertify historical executed rows under a new policy.

## Review coverage and release boundary

Reviewed areas include whole-tree SQL name/scope gates, selected graph and physical
projection, numerical/population/grain semantics, native-plan boundaries, complete
request budgeting, model readiness/error transports, owned/private parameter repair,
continuation, learned-example probes and disclosures, immutable persistence and
terminal replay. Existing unsupported joins/fan-out, generic language/order/limit
intent, live models and cloud-engine parity remain declared program obligations,
not silently marked complete by this defect review.

No P0 has been identified in this pass. The findings above remain unqualified until
the exact corrected-source tests finish. Final wording must distinguish no known
open P0/P1 findings within the reviewed/tested scope from proof that no possible
future defect exists. The PR remains draft while the full recovery program is open.

## First integration feedback

At 0bfd127, both baseline reproduction and full unit suites passed. The native
acceptance gate correctly failed two new assumptions: ONLY was already blocked by
native validation, and a real query-error receipt did not carry ErrQuery. Review
severity was revised to P2 for ONLY, and the production NLQ consumer was corrected
for the genuine durable-receipt mismatch. Neither the native whitelist nor fixture
executors were weakened. A new baseline-negative receipt test and native-result
acceptance case cover the real path before requalifying the whole matrix.


## Continued adversarial pass from 1ec3632

### P1-C continuation — source revision fence discarded the typed query error

The source driver correctly emits detail-free `exec.ErrQuery`, but `WithSource`
returns its callback through the metadata transaction sanitizer. That sanitizer
lacked ErrQuery and changed it to `store.ErrUnavailable`. Consequently the real
executor persisted source_unavailable and the stopped query-error branch still
could not be reached. The fix preserves only the preclassified sentinel. Raw
metadata PostgreSQL errors do not authorize source-SQL correction. Cancellation,
uncertainty, context/binding changes and unavailable/conflicting metadata retain
higher priority, and wrapped private driver text is stripped.

The final link was the database journal itself: migration 006's code constraint
omitted query_error even though the Executor emits it. After the revision-fence fix,
FinishRead therefore failed and the executor correctly returned ErrUncertain with
no receipt. Forward migration 058 adds only the closed query_error code, preserving
all previous codes, immutable manifests and terminal-status constraints. Existing
native-result tests now require two durable failed/query_error/stopped attempts,
not a fake Go error or an unavailable-metadata fallback. A real metadata outage is
still uncertain and cannot authorize a second source call.

### P1-D — missing physical receipt/cancelled caller abandoned query finalization

The actual Executor intentionally withholds its entire receipt after a journal
failure (for example failed FinishRead after source work) and returns ErrUncertain.
NLQ previously kept the query's old planned status. Its next idempotent attempt
could then join an operation that would never become terminal. The query now
records uncertain, without fabricated physical success/stopped evidence or rows.
Other absent outcomes are assigned conservative logical terminal status, never a
successful result. A terminal interrupted receipt remains a typed uncertainty.

Caller cancellation after a known terminal result also prevented UpdateQuery from
running under the cancelled request context. Final query persistence now uses a
three-second cancellation-detached metadata-only cleanup, as the existing native
executor does for its own ledger. It adds no source/model work and preserves the
original scoped context values. Persistent metadata outages still return failure;
this does not replace process-crash or remote reconciliation infrastructure.

Actual native/PostgreSQL tests inject only the journal failure or response-cancel
edge: the real executor, source read, observer, query store and authorization stay
in place. They check one physical attempt, no correction after uncertainty, durable
query outcome and immediate replay without another read or provider call.

### P1-E — failed rerun exposed a previous operation's successful rows

A query can be rerun under a new operation. If that run failed, finishRun assigned
a new result only when one existed, retaining the prior successful rows otherwise.
runResult then attached those stale rows to the failed response and its replay.
Results are now cleared per operation and only assigned for an error-free successful,
empty or truncated terminal receipt. Failed, cancelled, interrupted or uncertain
outcomes cannot return rows even if a malformed adapter response contains them.
The native fixture executes a valid result first, changes source data to trigger
zero division, then verifies bounded failure/correction, absent result persistence
and absent replay rows. It is not a test-only fake failure.

All new cases are part of exact-baseline expected-red reproduction and required
current-head test checks. Earlier failed runs remain failed evidence. The final PR
summary records observed qualification, not a promise of complete release parity.
