# Read function signatures v2

Generation and validation share one server-owned signature registry across
PostgreSQL, MySQL, SQL Server, BigQuery, Snowflake and Databricks. The detached
profile digest covers names, dialect, parser, parameter conventions, function
syntax flags and every registered signature. Compact `kind(arguments)->result`
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
polymorphic results, collation and exact overloads retain mandatory source
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

## Finite remaining recovery boundary

This checkpoint covers the existing registered function vocabulary plus reviewed
warehouse NULLIF for zero-denominator guards. It does not close the broader
S6/S11 analytical vocabulary: Snowflake/Databricks DATE_TRUNC; BigQuery
DATE_TRUNC, DATETIME_TRUNC and TIMESTAMP_TRUNC; SQL Server DATETRUNC; and MySQL
DATE_FORMAT, MAKEDATE, YEAR and QUARTER still need jointly reviewed signatures,
native AST topology and date-part keyword/dependency handling. PostgreSQL's
special-expression forms retain their existing native type-resolution path;
shared special-expression signature metadata is not claimed here. These are
software items, distinct from the actual required-engine execution and owner
cohort evidence in Q1/Q2/Q3. Missing implementations are not relabeled as
qualification-only work.
