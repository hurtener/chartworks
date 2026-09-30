# Read function signatures v3

Generation and validation share one server-owned signature registry across
PostgreSQL, MySQL, SQL Server, BigQuery, Snowflake and Databricks. The detached
profile digest covers names, dialect, parser, parameter conventions, function
syntax flags, special-expression signatures, calendar-unit grammar roles, result-family
maps and every registered signature. Compact `kind(arguments)->result`
strings are generated from the same typed definitions used by validation; they
avoid repeating object-field labels inside the fitted provider envelope.
A caller cannot mutate live registry state by editing a profile.

## Runtime consumers

PostgreSQL's native AST resolver and the warehouse call-evidence consumer both
invoke `sqlpolicy.AllowsCall` before creating an executable candidate. They check
argument count, optional/variadic positions, star forms, scalar/aggregate/window
classification, DISTINCT, FILTER, aggregate ordering, OVER, and supported
primitive argument families. Ordered-set calls are not part of this vocabulary.

The warehouse companion uses exactly `polyglot-sql=0.2.0`, matching the existing
pinned warehouse parser. It exposes bounded structural AST evidence from that
grammar; it does not implement a second SQL lexer, rewrite SQL, decide read
safety, or authorize dependencies. The existing positive native inspection runs
first. The only specially handled primary-parser function label is the
`window_function` structural wrapper: its actual nested call must independently
pass the shared signature gate. Both native passes use the validator semaphore. A separate shared two-slot
companion budget also applies across validators and analytical consumers.

The companion bounds input size, pre-parser lexical work, tree nodes/depth,
serialized evidence size and accepted dialects. It uses an explicitly sized,
joined worker stack so parsing is independent of the calling thread's stack.
Release workers use 16 MiB each (32 MiB process-wide stack reservation);
unoptimized Rust tests use 64 MiB because debug parser frames are larger.
FFI errors and caught panics return a fixed failure without SQL or values.
Unavailable native support fails closed. Cancellation is checked before and
after bounded native work and during the Go evidence walk. Queued admission
can be canceled immediately. In-flight FFI cannot be forcibly interrupted; its
bounded worker is joined and a canceled result is discarded before planning.

## Typing and dialect rules

Known literal, cast and nested-result evidence includes integer, numeric,
float, text, binary, boolean, temporal and interval families. Unknown types are
not inferred from column spellings: aliases, CTEs, derived outputs, parameters,
unresolved polymorphic results, collation and exact overloads retain mandatory source
EXPLAIN. MySQL, SQL Server, Snowflake and Databricks implicit conversions are
explicitly delegated to that native check instead of falsely applying
PostgreSQL's coercion rules to them. PostgreSQL and BigQuery reject proven
primitive mismatches before source planning. No static signature can issue a
plan or substitute for exact native type resolution.

The matrix includes PostgreSQL's numeric-only two-argument ROUND, encoded
bytea LENGTH, temporal functions and required window OVER; SQL Server's required
ROUND precision and optional truncation argument, with LEN as the SQL spelling
of the parser's normalized length node; and BigQuery/Snowflake ROUND mode
arguments and fixed-point restrictions. FILTER and ordered aggregate arguments
are gated by dialect, as is DISTINCT in windowed aggregates. Generation receives
these same syntax flags. Relevant public contracts include
[SQL Server ROUND](https://learn.microsoft.com/en-us/sql/t-sql/functions/round-transact-sql),
[BigQuery mathematical functions](https://docs.cloud.google.com/bigquery/docs/reference/standard-sql/mathematical_functions),
[BigQuery string functions](https://docs.cloud.google.com/bigquery/docs/reference/standard-sql/string_functions),
[Snowflake ROUND](https://docs.snowflake.com/en/sql-reference/functions/round), and
[Snowflake COALESCE](https://docs.snowflake.com/en/sql-reference/functions/coalesce).

## Build and qualification

`scripts/build-bruin.sh` builds the locked companion crate and places
`libchartworks_signatures.a` beside the existing native parser archive. The
existing CGO library path therefore loads both libraries. Docker copies the
companion source before that build; native caches include both archives and a
companion-source hash. Go embeds the exact Rust source, manifest and lockfile and compares them with
the linked archive contract, making stale native cache contents fail closed and
Rust edits invalidate the Go build cache. No module-cache mutation or
unpublished upstream patch is required. The accepted native/CGo build exception remains unchanged.

`TestFunctionSignaturesCoverVocabulary` covers every registered function in all
six profiles, arity and snapshot isolation. `TestFunctionSignatureTypesAndSyntax`
checks typed/modifier negatives. `TestPostgresFunctionSignatureNativeAST` uses
the actual PostgreSQL WASM grammar. `TestNativeStructuralEvidence` checks the
native companion across five warehouse dialects, bounds, cancellation and
concurrent reuse. `TestWarehouseFunctionSignaturesNative` exercises the full
warehouse validator with real native parsing: valid calls reach EXPLAIN once,
and wrong signatures reach it zero times. Its source adapter is a test fixture,
so these results establish parser/validator behavior, not warehouse execution.

Exact-source integrated regression and required real-engine/model/result
qualification remain separate release obligations. A supported signature is
bounded by this registered read vocabulary, not every built-in in a database.
No profile, native-parser test or recorded fixture substitutes for Q1/Q2/Q3.

## Calendar and special-expression coverage

PostgreSQL COALESCE, GREATEST, LEAST and NULLIF have shared special-expression
signatures without masquerading as ordinary function calls. Known incompatible
common types are rejected even when preceded by NULL. The native AST consumer
checks their exact forms before source planning.

The warehouse calendar vocabulary covers Snowflake/Databricks DATE_TRUNC;
BigQuery DATE_TRUNC, DATETIME_TRUNC and TIMESTAMP_TRUNC; SQL Server DATETRUNC;
and MySQL DATE_FORMAT, MAKEDATE and EXTRACT. Reviewed date parts are day, month,
quarter and year. A bare keyword is exempt from column binding only at the
registered BigQuery/SQL Server grammar position. Quoted date-part strings and
MySQL's native Extract.field enum have separate rules. A same-name occurrence
elsewhere remains an ordinary column dependency. MySQL's YEAR/QUARTER dedicated
AST spellings remain outside the primary grammar allowlist; equivalent authored
EXTRACT calls are supported, without rewriting user SQL.

Nested result inference follows explicit dialect maps. For example, AVG(integer)
is numeric in PostgreSQL/MySQL/Snowflake, integer in SQL Server and float in
BigQuery/Databricks. Unions of possible physical result types stay unknown.
BigQuery decimal-looking untyped literals are float, so its fixed-point ROUND
mode requires actual fixed-point cast/result evidence. Known common numeric
types are promoted; parameters and unresolved physical overloads still require
native planning. Interval aggregates follow the registered dialect matrix.

## Scoped warehouse dependency authority

The companion evidence is also used for occurrence-level binding after primary
read-safety inspection. Ordered CTEs and derived outputs can be referenced only
after their full child queries pass authority checks. Physical leaves must match
the full source catalog and narrower allowed binding; primary native physical
dependencies and root output names are cross-checked. A standalone ORDER BY
alias resolves only to a unique validated output. WHERE/GROUP BY occurrences
with the same spelling receive no alias exemption. Unused CTEs are checked too.

Ambiguous names, unknown output columns, recursive/forward CTE borrowing,
physical positional column aliases, NATURAL/USING implicit joins and unsafe
wildcards fail closed. Qualified correlation observes local shadowing; an
unqualified name cannot borrow an outer allowed field over a local hidden one.
Logical positional column aliases are based on checked projection positions.
Only BigQuery uses whole-backtick dotted path qualification semantics.
All signed authority checks and mandatory source EXPLAIN remain in place.

`TestPostgresSpecialExpressionNativeAST`, `TestWarehouseCalendarSignatureNative`,
`TestFunctionResultTypeDialectMatrix` and
`TestWarehouseScopedLogicalOutputsNative` cover these additions. Native call
regressions also check nested floating AVG and ROUND modes before EXPLAIN.
These are structural and validator tests; actual-engine qualification is
tracked separately. MySQL calendar, CTE/derived scalar populations and output
alias cases additionally have real source-result tests.

## Explicit remaining boundaries

This contract covers the registered read vocabulary, not every database built-in.
The primary native grammar still bounds accepted source syntax and lexical work;
unsupported nodes do not gain authority through companion extraction. In
particular, unspaced SQL Server/BigQuery `instant<@p1` is tokenized as the `<@`
operator; authored `instant < @p1` preserves the intended comparison. Native
parser compatibility fixes require a reviewed, reproducible dependency change.
Exact-source integrated regression and actual required-engine/model/result
qualification remain release obligations in Q1/Q2/Q3. No profile or fixture
substitutes for those runs.
