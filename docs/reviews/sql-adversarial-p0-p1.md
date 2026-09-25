# SQL recovery adversarial P0/P1 review

Baseline: PR #62, `c8c0a8db9327b147f313830037fcc56353536909`.
This is an adversarial review of the current recovery implementation, not a new
capability slice or a blanket release approval. Exact final CI/source evidence
and outstanding findings are recorded in the PR and delivery audit.

## Findings and regression oracles

### P1-A — parent-only scan can obtain a full-relation metric proof

Native-safe `SELECT sum(amount) FROM ONLY analytics.sales` names the same parent
relation as the reviewed metric, but excludes descendant/partition rows. The
analytical checker previously compared relation names and ignored RangeVar.inh.
A receipt could therefore claim the selected metric/population while returning a
smaller aggregate (or NULL for a partitioned parent without physical rows).

The proof now requires the ordinary descendant-inclusive scan; ONLY returns the
closed analytical relation mismatch. Native read authorization is unchanged:
being safe to read is separate from answering the selected metric. Explicit
ordinary inheritance (`table *`), aliases and existing successful shapes stay
supported. Validation correction receives the unbound candidate and existing
closed diagnostic. A retained hash cannot bypass the fresh analytical check
before nonterminal execution; terminal results remain historical evidence.

The independent PostgreSQL fixture compares parent-only total 30 with ordinary
relation total 130, proves that the failing proposal passes native validation,
then checks rejection without a physical attempt and successful bounded repair,
exact result, persistence/saved projection and no-work terminal replay.

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

### P1-C — correction-context failure can strand an execution operation

After a repairable failed physical read, inability to reconstruct retained
context returned before the query operation was finalized. A retry encountered
a finished physical attempt but a nonterminal query record; it could wait until
the caller deadline rather than replay the original failure.

The existing finishRun path now records that terminal preparation failure with
zero model corrections. SQL, bindings, accepted notes and analytical evidence
are not replaced. No extra model/source call is added. Unit and PostgreSQL tests
cover old/invalid retained context, a real runtime division error, durable failed
status and bounded idempotent replay. Cancellation, repository outage and process
crash recovery remain governed by their existing execution infrastructure; this
fix does not claim to solve every external failure or invent a successful result.

## Verification strategy

The read-only recovery workflow copies only the new regression tests into an
isolated worktree at the exact baseline. All four top-level baseline tests must
fail at the intended assertions, with no compilation failure, panic or skip.
Those expected-red tests are logged separately and never counted as passing
current-head tests. The current tree runs the complete existing race/acceptance
matrix plus three new result-based acceptance tests. Existing private-predicate,
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
