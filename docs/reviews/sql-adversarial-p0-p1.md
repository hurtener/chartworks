# SQL recovery adversarial P0/P1 review

Baseline: PR #62, `c8c0a8db9327b147f313830037fcc56353536909`.
This is an adversarial review of the current recovery implementation, not a new
capability slice or a blanket release approval. Exact final CI/source evidence
and outstanding findings are recorded in the PR and delivery audit.

## Findings and regression oracles

### P2-A — analytical parent-only gap already blocked by native validation

`SELECT sum(amount) FROM ONLY analytics.sales` names the same parent relation but
excludes descendant/partition rows. The analytical-only checker ignored RangeVar.inh.
Its isolated regression fails on the baseline, but actual integration established
that the existing native validator rejects ONLY before a plan reaches this checker.
Therefore this is defense-in-depth (P2), not a demonstrated baseline execution
bypass or P1. Native validation was not relaxed to manufacture a counterexample.

The proof now requires the ordinary descendant-inclusive scan; ONLY returns the
closed analytical relation mismatch. Native read authorization is unchanged:
being safe to read is separate from answering the selected metric. Explicit
ordinary inheritance (`table *`), aliases and existing successful shapes stay
supported. Validation correction receives the unbound candidate and existing
closed diagnostic. A retained hash cannot bypass the fresh analytical check
before nonterminal execution; terminal results remain historical evidence.

The independent PostgreSQL fixture compares parent-only total 30 with ordinary
relation total 130. It preserves and asserts the native rejection, then checks
no physical attempt, successful bounded repair, exact result, saved projection
and no-work replay. A pre-fix-looking receipt still cannot bypass native revalidation.
The actual fail/pass analytical-only test is kept separately from reachability.

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
isolated worktree at the exact baseline. All five top-level baseline tests must
fail at the intended assertions, with no compilation failure, panic or skip.
Those expected-red tests are logged separately and never counted as passing
current-head tests. The current tree runs the complete existing race/acceptance
matrix plus four new result-based acceptance tests. Existing private-predicate,
readiness, lineage, saved-query, parameter, learning and effective-budget cases
remain selected. No test is removed to obtain a green result.

No migration, public schema, analytical version, stored-row rewrite, positive-list
expansion, broader SQL authority, extra model role, or frozen-report generation
is introduced. Hardening rejects incorrect previously accepted candidates; it
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
