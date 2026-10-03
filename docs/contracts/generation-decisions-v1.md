# Generation readiness v1

Status: AP-08A software slice in PR #62. A readiness declaration is not objective
natural-language correctness, source authority, approval or an analytical proof.
Actual passing results belong in the exact-head PR qualification record.

## Closed model response

Every new sqlgen/sqlfix response must include `decision` and `questions`, in
addition to the existing sql/parameters/assumptions/ambiguities fields. The single
strict root-object schema does not require a second inference call. The consumer
revalidates it even for an alternate Engine implementation; no absent-field default
silently marks a fresh response ready. The mutually exclusive runtime states are:

| decision | SQL and bindings | questions | notes |
|---|---|---|---|
| ready | Nonempty candidate; normal native/analytical/private-binding checks apply | Empty array | Bounded descriptive assumptions/nonblocking caveats |
| clarify | Empty SQL and parameter array | 1–8 bounded user clarification questions | Empty assumptions and ambiguities |
| insufficient_context | Empty SQL and parameter array | 1–8 questions about missing reviewed evidence | Empty assumptions and ambiguities |

The mandatory system instruction distinguishes material unresolved metric, row,
time, grouping and output choices from nonblocking presentation caveats. A material
choice belongs in a blocked decision, not a warning appended to executable SQL.
Malformed, unknown, contradictory or missing declarations are generation errors,
never accepted SQL or a guessed clarification. Questions are at most 512 UTF-8
bytes each, nonempty, without NUL/CR/LF and without surrounding whitespace.

This is a model-reported readiness gate. It cannot prove that a model correctly
recognized every ambiguity. Deterministic routing, reviewed clarification and
native/analytical conformance remain independent mandatory controls. Do not label
a ready declaration as complete intent or result-quality certification.

## Control flow

An initial blocked decision returns before SQL validation, parameter rebinding,
plan persistence or warehouse execution. Earlier ordinary routing/admission or
embedding work and session creation may already have happened; do not report those
as zero. The attempted provider call is retained by the gateway and the core
error path; no fictitious correction call is counted. A material ambiguity is not
a malformed SQL candidate to send through the one SQL-fix allowance.

A validation correction can itself choose clarify/insufficient_context; it stops
without a second validation or execution. An execution correction can also stop:
the original failed attempt is finalized, no second physical query is executed,
and the prior accepted SQL/proof/notes are not replaced by the blocked response.
Existing terminal replay remains model-free and reports its recorded execution
outcome; it does not reconstruct transient model questions.

There is no executable query ID for an initial blocked decision. PlanAndRun does
not call Run on it; Refine does not publish an unresolved child. The caller must
clarify its question or repair reviewed context, then use the ordinary authorized
planning flow. These questions are not approved choice IDs and cannot mint an
AnswerContext or bind a scalar. This initial slice does not add a durable pending
model-question conversation. A repeated failed Plan is not a persisted successful
idempotent operation and must not be silently retried by the caller.

## Public and private boundaries

HTTP 422 distinguishes `generation_clarification_required` from
`generation_context_insufficient`; an optional `generation` object carries the
version, outcome and redacted questions. It is separate from deterministic reviewed
`clarification` questions. Error()/ordinary logging remain closed and content-free.
The existing MCP error surface and SDK use the same bounded projection. MCP keeps
its existing started-operation `unknown` side-effect disposition: no executable SQL
does not imply that no routing/model cost occurred. SDK decoding accepts only the
known version/shape/code association and does not automatically retry a rejection.

Reuse accepted-explanation redaction for known current/parent canonical values,
provided answer spellings, reviewed aliases and private parameter bindings. Old
and selected refinement bindings remain server-side. Redaction expansion beyond
the question bound replaces the entire question with a fixed withheld marker.
This is known-literal checking, not general DLP or a semantic guarantee about the
model's prose. The questions are untrusted descriptive content, not SQL to execute,
source facts or authorization. Unknown native/provider error text is never wrapped
as a generation problem. Missing authority returns no questions and makes no
provider call; the model decision does not move those checks later in the flow.

## Compatibility and qualification

No database migration, analytical-proof version or permission is introduced. Old
stored explanations are not reclassified: earlier ambiguities were unstructured
notes, not machine-readable materiality decisions. Existing terminal/frozen result
paths gain no model work. A new generation or repair must use the new schema even
when its parent is old; there is no fallback that executes an old fresh response
without the required discriminator. Recorded ready fixtures explicitly declare the
new contract. No fake gateway fills absent declarations automatically.

Tests cover the discriminated union, contradictory/missing fields, consumer-side
validation, bounds/Unicode, detachment/concurrency, private values, code-safe error
projection, exact envelope fitting, validation and execution correction stops,
legacy notes, EN/ES HTTP/MCP/SDK paths, no plan/attempt creation and successful fresh
planning after clarification. API/MCP tests stay under the existing signed surface.
No test inventory substitutes for observed runtime results or live-owner ambiguity
calibration. Durable pending outcomes, richer interpretation, cross-pattern answer
applicability and paired business-result qualification remain AP-08 obligations.
