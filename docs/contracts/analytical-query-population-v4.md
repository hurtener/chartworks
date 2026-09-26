# Analytical query population v4

AP-03C1 adds an optional owned-query-predicates-v1 certificate to the existing
native-first PostgreSQL single-base analytical proof. Current authoring chooses
record v4. Retained records 0 through 3 are rebuilt with their original policy.

The normal service obtains typed constraints from the in-process route seal.
Durable execution and terminal replay reconstruct them through the authenticated
clarification/interpretation replayer; saved resolution JSON alone is insufficient.
A pure metric compiler without those verified values cannot issue this certificate.

Every WHERE/HAVING conjunct must match a predicate produced by the existing typed
business binder, or (in WHERE only) a reviewed filter common to all selected metric
leaves. All required predicates must be present. Extra narrowing, changed operators,
changed parameter values, unexpected Boolean structure and hidden HAVING conditions
fail under the existing one-correction allowance. Equivalent-looking expressions
are not guessed: only locations, resolved qualification and parameter numbering
are normalized. Constants, casts, operators and concrete parameter kinds/values
remain exact. The matcher is bounded and does not execute the reference statement.

The separate metric checker continues to enforce aggregate-local populations;
filters for different metrics are not moved into one global intersection. Native
validation remains the only issuer of executable plans. Signed reach, relation
scope, exact source pins, current/retained authority and frozen refresh are unchanged.

The optional query_population receipt marker explicitly names the added policy.
Private constraints exist only in the local contract comparison; public receipts
contain the policy marker plus the existing contract/query digests, not values.
Provider guidance is fixed value-free text. The existing immutable-proof trigger
protects the marker and contract; migration 056 does not rewrite earlier rows.

Scope limits: this certifies predicate provenance/completeness relative to the
resolved typed constraints, not exhaustive natural-language comprehension, intended
ORDER BY/LIMIT, joins/fan-out or general SQL equivalence. With no selected metric or
no typed query constraints, no query-population certificate is added. Broad original
SQL support and other dialects remain separate qualifications, not implied passes.

Tests must cover typed/private values, inclusive/exclusive bounds, NULL policies,
aggregate predicates, independent metric filters, extra restrictions, missing or
changed predicates, immutable old-policy replay and actual PostgreSQL results.
Executed results belong in the PR checkpoint, not inferred from this inventory.
