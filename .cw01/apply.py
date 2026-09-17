"""Apply the second bounded CW-01 review edit set, with checked preconditions."""
from pathlib import Path

replacements = [
    ("test/acceptance/cw01_consumers_test.go", "preview.Cases[1].Outcome != semantics.ClarificationSatisfied", "preview.Cases[1].Outcome != semantics.ClarificationNotApplicable"),
    ("test/acceptance/cw01_consumers_test.go", "shadow.BaselineClarifications[2].Outcome != semantics.ClarificationSatisfied", "shadow.BaselineClarifications[2].Outcome != semantics.ClarificationNotApplicable"),
    ("test/acceptance/cw01_consumers_test.go", "compatible.Preview.Cases[0].Outcome != semantics.ClarificationSatisfied", "compatible.Preview.Cases[0].Outcome != semantics.ClarificationNotApplicable"),
    ("test/acceptance/cw01_consumers_test.go", "retained.BaselineClarifications[0].Outcome != semantics.ClarificationSatisfied", "retained.BaselineClarifications[0].Outcome != semantics.ClarificationNotApplicable"),
]
new_files = {
 "internal/semantics/clarification_logging.go": r'''package semantics

import "log/slog"

// LogValue protects direct structured-log attributes. JSON remains the deliberate
// authorized wire/protected-storage representation, not an ordinary log format.
func (a ClarificationAnswer) LogValue() slog.Value { return slog.StringValue(a.String()) }
func (r ClarificationResolution) LogValue() slog.Value { return slog.StringValue(r.String()) }
func (ClarificationValue) LogValue() slog.Value { return slog.StringValue("clarification-value(redacted)") }
func (ClarificationValue) String() string { return "clarification-value(redacted)" }
func (v ClarificationValue) GoString() string { return v.String() }
''',
 "internal/semantics/clarification_logging_test.go": r'''package semantics

import (
 "bytes"
 "encoding/json"
 "log/slog"
 "strings"
 "testing"
)

func TestClarificationStructuredLoggingRedactsValues(t *testing.T) {
 secret := "synthetic-private-answer-731"
 value := ClarificationValue{Text: &secret}
 answer := ClarificationAnswer{Topic:"sales", Pattern:"customer", Slot:"customer", Value:&value}
 resolution := ClarificationResolution{Sensitivity:LiteralSensitive, Value:secret, Effect:&ClarificationEffect{Kind:"entity", Values:[]GovernedClarificationValue{{Canonical:secret, Label:"Customer"}}}}
 for _, jsonLogs := range []bool{false,true} {
  var output bytes.Buffer
  var handler slog.Handler
  if jsonLogs { handler = slog.NewJSONHandler(&output,nil) } else { handler = slog.NewTextHandler(&output,nil) }
  slog.New(handler).Info("clarification", slog.Any("answer",answer), slog.Any("resolution",resolution), slog.Any("value",value))
  if strings.Contains(output.String(),secret) || !strings.Contains(output.String(),"redacted") {
   t.Fatal("ordinary structured logging exposed a typed answer")
  }
 }
 // Redaction must not silently erase data from explicitly authorized DTOs or
 // canonical protected records used for replay and binding.
 for _, dto := range []any{answer,resolution,value} {
  raw,err := json.Marshal(dto)
  if err != nil || !bytes.Contains(raw,[]byte(secret)) { t.Fatal("log safety changed the wire/storage contract",err) }
 }
}
''',
 "internal/exec/business_logging.go": r'''package exec

import "log/slog"

// LogValue excludes exact business values from ordinary structured logs.
func (c BusinessConstraint) LogValue() slog.Value { return slog.StringValue(c.String()) }
''',
 "internal/exec/business_logging_test.go": r'''package exec

import (
 "bytes"
 "encoding/json"
 "log/slog"
 "strings"
 "testing"
)

func TestBusinessConstraintStructuredLoggingIsValueFree(t *testing.T) {
 constraint := businessFixtureConstraint()
 for _, jsonLogs := range []bool{false,true} {
  var output bytes.Buffer
  var handler slog.Handler
  if jsonLogs { handler = slog.NewJSONHandler(&output,nil) } else { handler = slog.NewTextHandler(&output,nil) }
  slog.New(handler).Info("binding",slog.Any("constraint",constraint))
  if strings.Contains(output.String(),constraint.Value) || !strings.Contains(output.String(),"redacted") {
   t.Fatal("ordinary structured logging exposed a business scalar")
  }
 }
 raw,err := json.Marshal(constraint)
 if err != nil || !bytes.Contains(raw,[]byte(constraint.Value)) { t.Fatal("log safety erased the protected constraint",err) }
}
''',
 "docs/contracts/conditional-clarification-v1.md": '''# Conditional clarification and typed answers v1

Status: CW-01 implementation contract submitted in PR #23. Exact-head execution
and the bounded review record determine readiness; this document is not test
execution evidence. It extends phases 16–18 without changing identity ownership,
immutable publication, source partitions, or validator-issued read proof.

## Reviewed activation and outcomes

A clarification policy is optional versioned data on a retained pattern. Version 1
uses only reviewed literal token phrases and exact semantic-reference conditions.
The conditions are a closed OR, not executable expressions, regular expressions,
or model-generated permission decisions. The topic and ruleset must already be
admitted under current signed reach. Explicit reference selections can satisfy a
reference-choice slot without asking again. Free-form question rewriting is not
an answer's semantic effect.

The evaluator distinguishes `not_applicable`, `satisfied`, `missing`, `invalid`,
and `conflicting`. An unrelated question has not-applicable slots and continues
without answering them. A submitted value for an inapplicable or foreign field is
invalid, not a hidden filter. Invalid or conflicting input returns no partially
accepted resolution group. Unresolved governed text returns a missing outcome and
an explicit `unresolved_value` field error.

Runtime ordering is independent of persistence order: blockers, specificity,
author priority, stable pattern ID, and stable slot order under declared
prerequisites. A dependency graph orders questions; only ready blockers are
presented. Independent blockers can be grouped with the explicit explanation that
each resolves a separate reviewed constraint. Conflicting effects on the same
semantic or physical target are evaluated as a group, including incompatible
units/time policies and jointly empty scalar intersections. Priority never makes
one incompatible mandatory policy win.

## Declared effects and exact values

Reference choices use only `option_id`, mapped to an exact reviewed semantic
reference. A display label is never a reference or warehouse identifier. All other
inputs use the typed `value` union and a declared effect:

| Family | Canonical effect |
|---|---|
| Time window | Exact local and UTC bounds, Gregorian calendar, named IANA timezone, temporal target/type, reviewed grain and half-open `[)` boundaries. |
| Number | Exact decimal or range strings, reviewed unit, precision, scale, comparison operator, null semantics and interval inclusivity. No floating-point intermediate or silent rounding. |
| Boolean | Locale-neutral true/false bound to the reviewed predicate target. Unknown spellings require repair rather than truthiness conversion. |
| Entity or bounded text | A reviewed spelling maps to one governed canonical value and an exact target/operator/null policy. Unmapped text does not become SQL or implied filtering. |

English/Spanish number and boolean inputs and named month periods normalize to
the same locale-neutral values. Explicit dates use `YYYY-MM-DD`. Calendar,
timezone and grain are required, not inferred from a display label or server
locale. Reviewed grains are day, week, month, quarter and year; both boundaries
must align. Time windows are half-open. Missing or ambiguous local-midnight
boundaries fail explicitly rather than guessing a daylight-saving offset. General
natural-language period interpretation and arbitrary calendars are not advertised.

Null behavior is explicit: exclude nulls, include nulls alongside the comparison,
or a null-only predicate. Numeric comparisons use exact declared units; unit
conversion is not inferred. Approximate source numeric types are not accepted for
an exact threshold. Adapter precision/type limits fail before model work.

Every required field must be answered. Required defaults are rejected at
compilation. Optional defaults are reviewed, visible as defaulted, and retained
with `reviewed_default` provenance. They are reevaluated on replay rather than
being converted into user answers. Explicit removal cannot skip a required field.

## Propagation, budgets and privacy

`QuestionRequest` and the routing request carry typed answers with topic,
topic-version, ruleset-version, pattern/version and slot pins. Resolution records
add the parser version, locale, canonical value/effect, provenance, exact digests
and question digest. The route seals the resolution set and live source-binding
identity. These records are business evidence, never bearer authority or an
executable plan.

Before embeddings, reranking or generation, the existing tokenizer counts the
whole mandatory group together with pinned metrics and existing hard rules. The
1500/3000/6500 tiers remain one token currency. A group that does not fit produces
typed insufficiency; no selected answer is dropped to make space. Provider context
contains reviewed effect metadata, not scalar values or governed dictionaries.
Classification uses the reviewed slot's sensitivity before persistence. Known
sensitive submitted/canonical spellings are redacted from questions and
instructions before storage or provider submission. This is not a general PII
classifier for arbitrary unrelated text.

Ordinary logs contain identifiers, reason codes and bounded outcome evidence, not
raw answers, SQL, result rows or prompts. Direct typed-answer, resolution and
business-constraint structured log attributes are redacted. Arbitrary serialized
wire DTOs or containers are not an ordinary logging interface. Authorized API JSON
and protected domain storage intentionally retain the canonical data needed for
binding and replay.

After generation, the service resolves exact semantic targets through the reviewed
column graph and current source metadata. It binds values as parameters, preserving
existing predicates by conjunction. Row targets become row predicates; aggregate
measure thresholds become reviewed aggregate/HAVING predicates, not filters on
individual measure input rows. The binding receipt identifies exact parameter
positions, statement/parameter digest, source identity and resolution IDs without
scalar values. The ordinary validator must then issue a nonzero read plan. Neither
a parsed statement nor a clarification digest substitutes for that proof.

The current binding transformer accepts bounded single SELECT statements over
qualified base relations, including joins, WHERE, grouping, HAVING, ordering and
limits. Ambiguous self-join targets, derived/nested SELECTs, CTEs, set operations
and unsupported dialect shapes return explicit unsupported outcomes. No constraint
is silently omitted to accept a broader SQL shape. Queries with no scalar business
constraints retain their existing validator behavior. Six dialect parameter/quoting
paths have synthetic transformation tests; this assignment's real warehouse
acceptance uses PostgreSQL and does not newly qualify live cloud engines.

## Sessions, correction and current reach

Submitting answers through preflight/planning requires the retained originating
`clarification_query` and `answer_context`. The service verifies owner, session,
question, topic set, selected references, metrics, joins and context before provider
work. A query ID or answer-context digest grants no access. New requests supply a
fresh bearer; no bearer is saved for replay.

Refinement rereads the parent under current authority and reevaluates its current
publication/source pins. Changed relevant state returns a stale/reevaluate error,
not an automatic adoption of a new meaning. Answer deltas replace or remove by
exact topic/pattern/slot. The child retains value-free supersession/removal lineage;
the parent's historical evidence remains immutable. Stored unbound base SQL is
separate from service-bound predicates, and answer edits do not inherit old bound
filters as generation instructions.

Before execution, the service reevaluates canonical resolutions and reconstructs
the binding from the protected base. It compares exact SQL, parameters and receipt,
then uses fresh ordinary validation and execution. Validation correction reapplies
the complete constraint group. Execution correction cannot alter bound SQL or
parameters to widen an answer. Cancellation returns no partial bound query.

## Authoring, portability and consumers

The existing topic API/SDK provides:

- `POST /v1/topics/{topic}/clarifications/preview` / `PreviewClarifications`;
- `POST /v1/topics/{topic}/clarifications/export` / `ExportClarifications`;
- `POST /v1/topics/{topic}/clarifications/import-preview` / `PreviewClarificationImport`.

Preview accepts bounded synthetic matching/nonmatching/conflicting cases, reports
human-readable conditions/effects and typed outcomes, and does not publish or call
a provider. Saving, review, publication and retirement reuse the existing ruleset
lifecycle and signed topic/dependency reaches. Retained replay and shadow reuse
the same deterministic evaluator and persist immutable comparison results. Review
is required before activation; no standalone authoring application is introduced.

Execution uses the existing preflight/plan/run/refine HTTP, MCP and SDK consumers.
Localized field errors expose field, code and repair message. Questions expose
concise prompts, why they matter, readable reviewed choices, prerequisites and
visible defaults. Client DTOs preserve canonical typed unions and exact decimal
strings rather than converting non-reference values into choice IDs.

Portable version 1 retains exact rule/topic digests and explicit per-pattern
migration dispositions. Import is a preview/proposal until normal reviewed
publication. Exact-topic roundtrips are supported; foreign topic/digest mapping
requires an explicit new semantic draft and review, never display-name matching.
Legacy imports require `preserve_reference_only`. Retained patterns without a
policy remain digest-stable explicit reference choices only; they never acquire
new automatic blocking. Legacy scalar values require a reviewed typed rewrite.
Unknown executable matcher fields are rejected by the closed API decoder.

Migrations 036 and 037 append protected NLQ clarification evidence and retained
comparison result columns after the shipped prefix. Existing migrations and
published definitions are not rewritten. Acceptance upgrades from the shipped
schema, verifies its checksums and retained tenant data, then checks the new
columns through the normal migration runner.

## Evidence and explicit limitations

`TestCW01/AC01` through `AC10` cover the software corpus together with the phase
16–18 regressions, parser/binding unit and fuzz cases, race tests, authoring and
actual HTTP/MCP/SDK journeys. Recorded provider responses prove deterministic
integration, not live model accuracy. SQL-shape and calendar limitations above are
typed boundaries, not fallback permission to ignore answers. Representative-user
completion, correction time and comprehension testing (`CLAR-AC11`) was not
performed and remains separate usability research. This change does not expand
routing calibration, rich topic generation, chart selection or reporting policies.
''',
}

contents = {}
for path,before,after in replacements:
 text = contents.get(path)
 if text is None: text = Path(path).read_text()
 if text.count(before) != 1: raise SystemExit(f"{path}: expected one reviewed replacement anchor")
 contents[path] = text.replace(before,after)
for path,text in new_files.items():
 if Path(path).exists(): raise SystemExit(f"{path}: refusing to replace an existing file")
 contents[path] = text
for path,text in contents.items(): Path(path).write_text(text)
