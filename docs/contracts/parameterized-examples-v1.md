# Parameterized learned examples v1

Status: AP-06B implementation checkpoint in PR #62. Integration and native
qualification are pending until the required-toolchain recovery workflow runs.
This contract does not authorize SQL or certify generic parameter values.

## Data ownership and lifecycle

A reviewed SQL demonstration may have `parameter_schema` version
`example-parameters-v1`. It contains only dense, one-based slot positions and
`text`, `integer`, `number`, `boolean` or `null` kinds, with at most 64 slots.
There is deliberately no value, default or original query binding in this type.
Nil schema preserves the original parameter-free example contract; an empty
versioned schema is invalid. Constructors and validation are bounded and detached.

After the existing feedback authority and native validation checks, the service
can propose a parameterized candidate using the SQL shape and slot types. Known
binding spellings are redacted from its question label with the existing literal
redactor. The candidate digest binds topic, redacted question, exact SQL and typed
schema under a distinct versioned domain. Feedback can still be recorded when a
query is unsuitable for reusable learning. Service-owned scalar/time/predicate
bindings are not proposed automatically as reusable examples, even if the query
has no explicit clarification-answer list.

The post-validation disclosure check uses the existing bounded SQL scanner, not a
new permissive safety parser. Unsupported comments/quoted forms and known binding
spellings copied into literals or identifiers make that query ineligible for
learning. Numeric comparison uses exact decimal normalization where supported;
marker positions are not mistaken for integer values. This is a conservative
known-literal check, **not general DLP**: it does not prove that encoded,
paraphrased, computed or unknown source content is non-sensitive. It never grants
execution, relaxes native validation or changes the accepted source query.

## Review, import and current-source validation

Activation retains its existing signed actions, topic/context/source reach,
review note, evidence threshold, origin verification and revision CAS. The native
validator receives fixed public validation-only probes of each kind; those probes
are not saved or shown as example/default values. Validation does not execute the
query. A type-dependent cast that rejects the public probe remains unreviewable:
there is no fallback to historical private values or a bypass of native planning.
This deliberately bounded probe strategy does not assert validity for every
possible future binding.

Protected import applies the same exact current-origin and native checks, then
creates a candidate requiring separate review. Typed portable rows use version 2;
parameter-free rows retain version 1. A bundle containing any typed row uses
version 2 and may include legacy rows. Version/schema mismatches, malformed slots
and substituted content fail. A portable import with stripped schema cannot claim
the version-two contract. There is no value-bearing import extension.

Ordinary example views still honor SQL-inspection permissions; only safe slot
metadata is added. Export/import remain protected SQL operations. Public SDK
aliases expose the typed schema and slots without bypassing the service.

## Generation and persistence

Reviewed demonstrations contain the redacted question, SQL markers and typed slot
schema. The existing generator is explicitly instructed to resolve values from
its **current** question. Actual generated bindings still go through native and
analytical validation; no historical binding is substituted into a new query.
The existing ranking, precedence, bounded example fitting, actual-use evidence
and maximum retry policy are unchanged. No extra model role/call is introduced.

Migration 057 adds nullable `parameter_schema` to the existing example table, with
strict bounded JSON shape validation and an immutable-schema trigger. The existing
SQL/question/digest/origin and review protections remain. All insert, list, exact
read and state-change paths preserve the new schema; malformed persisted typed
content fails before use. JSON `null` is not a typed schema: legacy absence is SQL
NULL. Schema/probe slices returned to callers are detached.

Legacy digests and nil-schema records are not rewritten or backfilled. Historical
parameterized SQL that never had a schema does not acquire guessed bindings;
normal native activation without bindings can reject it. `ExampleParametersValid`
does not retroactively authenticate arbitrary legacy rows. Database immutability,
portable row versions, current origin checks and native validation own their
separate integrity boundaries. The template digest is not an authority token.

## Tests and remaining qualification

The core `internal/nlq/exampleparams` package is standard-library-only and can be
race-tested independently. New service, execution, public SDK and PostgreSQL/
recorded-provider tests cover slot/digest substitution, private-value redaction,
public-probe validation, current-question reuse with changed result IDs, schema
immutability, protected import, static-probe rejection and ineligible annotations.

The completion report must distinguish actually executed tests from added tests.
Public-probe acceptance, the forward migration and recorded full-flow tests require
the normal native/toolchain/dependency environment. Neither source review nor the
pure-schema test substitutes for those gates. Broader parameter-domain validation,
per-dialect qualification and live-owner result quality remain open. Frozen report
refresh, source authorization and inference-free retained execution are unchanged.
