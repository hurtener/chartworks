package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/exampleparams"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func ownedLearningFixture(t *testing.T, sql string, parameters []exec.Parameter) (QueryRecord, admission, *Service, *unitValidator) {
	t.Helper()
	q, binding, constraints := cw07InterpretationQuery(t, "PRIVATE-NORTH-871")
	bound, err := exec.BindBusinessConstraints(context.Background(), binding, sql, parameters, constraints)
	if err != nil {
		t.Fatal(err)
	}
	bound.Receipt.Validation = q.Clarification.Binding.Validation
	q.SQL, q.Parameters = bound.SQL, bound.Parameters
	q.Clarification = &ClarificationEvidence{SchemaVersion: 1, BaseSQL: sql, BaseParameters: append([]exec.Parameter(nil), parameters...), Binding: bound.Receipt}
	q.Question = "Previous query for PRIVATE-NORTH-871 in March 2025"
	a := admission{source: "source", context: "context", binding: binding, relationScope: []exec.RelationScope{{Dataset: binding.Relations[0].ID, Columns: []string{"id"}}}, route: q.Route}
	validator := &unitValidator{}
	service := &Service{router: &cw07ReplayRouter{constraints: constraints, binding: exec.Hash(binding)}, validator: validator}
	return q, a, service, validator
}

func TestSQLRecoveryOwnedLearningBaseDoesNotCarryPredicates(t *testing.T) {
	sql := "SELECT id FROM analytics.sales WHERE id>$1 ORDER BY id"
	q, a, s, validator := ownedLearningFixture(t, sql, []exec.Parameter{{Kind: "integer", Value: "41"}})
	before := exec.Hash(q)
	base, policy, ok, err := s.reusableLearningBase(context.Background(), unitEnvelope(t), q, a, "")
	if err != nil || !ok || policy != OwnedExamplePolicy || base.SQL != sql || !parametersEqual(base.Parameters, q.Clarification.BaseParameters) || len(validator.requests) != 1 {
		t.Fatal("unbound proof", err)
	}
	if strings.Contains(base.Question, "871") || strings.Contains(base.Question, "March") || strings.Contains(base.Question, "2025") || !neutralOwnedExampleQuestion(base.Question) {
		t.Fatal("historical predicate/question carried")
	}
	if !reflect.DeepEqual(validator.requests[0].Parameters, base.Parameters) || validator.requests[0].SQL != sql {
		t.Fatal("did not dry-check base")
	}
	base.Parameters[0].Value = "changed"
	if before != exec.Hash(q) {
		t.Fatal("learning modified original bound query")
	}
}

func TestSQLRecoveryOwnedLearningRequiresExactBindingProof(t *testing.T) {
	for _, mutate := range []func(*QueryRecord, *admission){
		func(q *QueryRecord, a *admission) { q.Clarification = nil },
		func(q *QueryRecord, a *admission) { q.Clarification.BaseSQL += " WHERE id=9" },
		func(q *QueryRecord, a *admission) { q.Parameters[0].Value = "other-private" },
		func(q *QueryRecord, a *admission) { q.Clarification.Binding.SourceBinding = exec.Hash("foreign") },
		func(q *QueryRecord, a *admission) { q.Clarification.Binding.Validation = nil },
		func(q *QueryRecord, a *admission) { a.binding.Revision++ },
	} {
		q, a, s, v := ownedLearningFixture(t, "SELECT id FROM analytics.sales", nil)
		mutate(&q, &a)
		if _, _, ok, err := s.reusableLearningBase(context.Background(), unitEnvelope(t), q, a, ""); ok || err == nil || len(v.requests) != 0 {
			t.Fatal("unproved base reached native review", err)
		}
	}
	q, a, s, v := ownedLearningFixture(t, "SELECT id FROM analytics.sales", nil)
	s.router = &cw07ReplayRouter{err: exec.ErrBinding}
	if _, _, ok, err := s.reusableLearningBase(context.Background(), unitEnvelope(t), q, a, ""); ok || !errors.Is(err, exec.ErrBinding) || len(v.requests) != 0 {
		t.Fatal("replay drift ignored", err)
	}
}

func TestSQLRecoveryOwnedLearningSuppressesLiteralsAliasesAndManualCorrections(t *testing.T) {
	for _, literal := range []string{"PRIVATE-NORTH-871", "hidden-north-alias"} {
		q, a, s, v := ownedLearningFixture(t, "SELECT id, '"+literal+"' AS note FROM analytics.sales", nil)
		q.Route.Resolutions = []semantics.ClarificationResolution{{Sensitivity: semantics.LiteralSensitive, Value: "PRIVATE-NORTH-871", Effect: &semantics.ClarificationEffect{Values: []semantics.GovernedClarificationValue{{Canonical: "PRIVATE-NORTH-871", Aliases: []string{"hidden-north-alias"}}}}}}
		if _, _, ok, err := s.reusableLearningBase(context.Background(), unitEnvelope(t), q, a, ""); ok || err != nil || len(v.requests) != 0 {
			t.Fatal("known private base text learned", err)
		}
	}
	q, a, s, v := ownedLearningFixture(t, "SELECT id FROM analytics.sales", nil)
	if _, _, ok, err := s.reusableLearningBase(context.Background(), unitEnvelope(t), q, a, q.SQL); ok || err != nil || len(v.requests) != 0 {
		t.Fatal("manual bound correction pretended to have a base", err)
	}
	for i := 0; i < 70; i++ {
		q.Route.Resolutions = append(q.Route.Resolutions, semantics.ClarificationResolution{Sensitivity: semantics.LiteralSensitive, Value: strings.Repeat("x", i+1)})
	}
	if _, _, ok, err := s.reusableLearningBase(context.Background(), unitEnvelope(t), q, a, ""); ok || err != nil {
		t.Fatal("overflow silently dropped secret spellings", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := s.reusableLearningBase(ctx, unitEnvelope(t), q, a, ""); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored cancellation", err)
	}
	if _, _, _, err := s.reusableLearningBase(nil, unitEnvelope(t), q, a, ""); err == nil {
		t.Fatal("nil context")
	}
}

func ownedExampleFixture(t *testing.T, typed bool) ExampleRecord {
	t.Helper()
	x := ExampleRecord{Topic: "topic", Question: "Reviewed unbound query template for current service-owned filters. Metric identities: [\"measure:revenue\"]", SQL: "SELECT id FROM analytics.sales", Origin: ExampleOrigin{SchemaVersion: 1, BindingPolicy: OwnedExamplePolicy}}
	if typed {
		x.ParameterSchema, _ = exampleparams.New([]string{"number"})
		x.SQL += " WHERE id>$1"
	}
	x.Digest = originExampleDigest(x.Topic, x.Question, x.SQL, x.ParameterSchema, x.Origin)
	return x
}
func TestSQLRecoveryOwnedLearningPortableIdentityAndProbes(t *testing.T) {
	for _, typed := range []bool{false, true} {
		x := ownedExampleFixture(t, typed)
		if !ExampleParametersValid(x) {
			t.Fatal("valid template rejected")
		}
		p, err := exampleValidationParameters(x)
		if err != nil || !typed && len(p) != 0 || typed && !reflect.DeepEqual(p, []exec.Parameter{{Kind: "number", Value: "1"}}) {
			t.Fatal("public probes", err)
		}
		row := PortableExample{SchemaVersion: 3, Question: x.Question, SQL: x.SQL, Digest: x.Digest, Origin: x.Origin, ParameterSchema: x.ParameterSchema.Clone()}
		raw, _ := json.Marshal(row)
		var decoded PortableExample
		if json.Unmarshal(raw, &decoded) != nil || !portableExampleValid(decoded) || originExampleDigest(x.Topic, decoded.Question, decoded.SQL, decoded.ParameterSchema, decoded.Origin) != x.Digest {
			t.Fatal("origin portability")
		}
		for _, mutate := range []func(*PortableExample){func(p *PortableExample) { p.SchemaVersion = 2 }, func(p *PortableExample) { p.Origin.BindingPolicy = "" }, func(p *PortableExample) { p.Origin.BindingPolicy = "unreviewed" }} {
			bad := row
			mutate(&bad)
			if portableExampleValid(bad) {
				t.Fatal("portable policy lost or downgraded")
			}
		}
		bad := x
		bad.SQL += " WHERE false"
		if ExampleParametersValid(bad) {
			t.Fatal("base changed outside digest")
		}
		bad = x
		bad.Origin.BindingPolicy = "unreviewed"
		if ExampleParametersValid(bad) {
			t.Fatal("unknown origin")
		}
	}
}
func TestSQLRecoveryOwnedLearningInstructionHasOneBoundedLane(t *testing.T) {
	a, err := nlq.NewDefaultContextAssembler()
	if err != nil {
		t.Fatal(err)
	}
	c, err := a.Assemble(context.Background(), nlq.ContextInput{Topic: "topic", TopicVersion: "v1", Locale: nlq.LanguageEnglish, Strategy: nlq.StrategySingleTopic, Question: "Current reviewed question"}, nlq.TierHigh)
	if err != nil {
		t.Fatal(err)
	}
	for _, typed := range []bool{false, true} {
		x := ownedExampleFixture(t, typed)
		x.SQL = strings.ReplaceAll(x.SQL, " FROM ", "\nFROM ")
		x.Digest = originExampleDigest(x.Topic, x.Question, x.SQL, x.ParameterSchema, x.Origin)
		text := learnedExampleText(x)
		if strings.ContainsAny(text, "\n\r") || !strings.Contains(text, OwnedExamplePolicy) || !strings.Contains(text, "current question") || strings.Contains(text, `"value":`) {
			t.Fatal("wrong template instructions")
		}
		g, err := a.ResolvePrecedence(context.Background(), nlq.GenerationInput{Context: c, Examples: []nlq.Instruction{{Key: "owned-example", Text: text}}})
		if err != nil || len(g.Selected) != 1 || g.Selected[0].Text != text {
			t.Fatal("template does not fit owner framing", err)
		}
		var decoded struct {
			Question, SQL string
			Parameters    *exampleparams.Schema `json:"parameter_schema"`
			Policy        string                `json:"binding_policy"`
		}
		d := json.NewDecoder(strings.NewReader(strings.TrimPrefix(text, "Reviewed unbound SQL demonstration: ")))
		d.DisallowUnknownFields()
		if d.Decode(&decoded) != nil || decoded.SQL != x.SQL || decoded.Policy != OwnedExamplePolicy || !reflect.DeepEqual(decoded.Parameters, x.ParameterSchema) {
			t.Fatal("rendering changed exact SQL/schema")
		}
	}
}
func TestSQLRecoveryOwnedLearningCannotTakeAuthorityFromJSON(t *testing.T) {
	x := ownedExampleFixture(t, false)
	q, binding, _ := cw07InterpretationQuery(t, "private")
	raw, _ := json.Marshal(q.Route)
	var r nlqroute.RouteResult
	if json.Unmarshal(raw, &r) != nil {
		t.Fatal("fixture")
	}
	if ownedExampleApplicable(x, admission{binding: binding, route: r}) || ownedExampleApplicable(x, admission{binding: binding}) {
		t.Fatal("origin marker/retained JSON authorized predicates")
	}
	x.Origin.BindingPolicy = ""
	if !ownedExampleApplicable(x, admission{}) {
		t.Fatal("legacy selection gate changed")
	}
}

func TestSQLRecoveryOwnedLearningNeutralPrefixParameter(t *testing.T) {
	for _, word := range []string{"current", "template", "query"} {
		q, a, s, _ := ownedLearningFixture(t, "SELECT id FROM analytics.sales WHERE name=$1", []exec.Parameter{{Kind: "text", Value: word}})
		base, policy, ok, err := s.reusableLearningBase(context.Background(), unitEnvelope(t), q, a, "")
		if err != nil || !ok {
			t.Fatal("owned base", err)
		}
		label, schema, err := learnedParameterSchema(context.Background(), base, policy)
		origin := ExampleOrigin{BindingPolicy: policy}
		example := ExampleRecord{Topic: "topic", Question: label, SQL: base.SQL, ParameterSchema: schema, Origin: origin}
		example.Digest = originExampleDigest(example.Topic, label, base.SQL, schema, origin)
		if err != nil || !ExampleParametersValid(example) {
			t.Fatal("neutral label changed by unrelated parameter", err)
		}
	}
}
