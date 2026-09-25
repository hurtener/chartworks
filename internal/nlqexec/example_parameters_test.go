package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func parameterLearningRecord(t *testing.T) ExampleRecord {
	t.Helper()
	q := QueryRecord{Question: "Amounts above 9007199254740993.125 for private-client-741", Parameters: []exec.Parameter{{Kind: "number", Value: "9007199254740993.125"}, {Kind: "text", Value: "private-client-741"}}}
	question, schema, err := learnedParameterSchema(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	x := ExampleRecord{ID: "template", Topic: "topic", Question: question, SQL: "SELECT id FROM analytics.sales WHERE amount>$1 AND name=$2", ParameterSchema: schema}
	x.Digest = parameterExampleDigest(x.Topic, x.Question, x.SQL, x.ParameterSchema)
	return x
}
func TestSQLRecoveryLearningParametersDoNotCarryHistoricalValues(t *testing.T) {
	q := QueryRecord{Question: "Importes superiores a 9007199254740993.125 para cliente\u00a0privado", Parameters: []exec.Parameter{{Kind: "number", Value: "9007199254740993.125"}, {Kind: "text", Value: "cliente privado"}}}
	before := exec.Hash(q)
	question, schema, err := learnedParameterSchema(context.Background(), q)
	if err != nil || strings.Contains(question, "9007199") || strings.Contains(question, "privado") || !strings.Contains(question, "[redacted answer]") {
		t.Fatal("known binding in question", err)
	}
	x := parameterLearningRecord(t)
	rendered := learnedExampleText(x)
	for _, secret := range []string{"9007199254740993.125", "private-client-741"} {
		if strings.Contains(rendered, secret) {
			t.Fatal("historical binding entered demonstration")
		}
	}
	if !strings.Contains(rendered, "parameter_schema:") || !strings.Contains(rendered, `"position":1`) || !strings.Contains(rendered, `"kind":"number"`) || strings.Contains(rendered, `"value":`) {
		t.Fatal("not a typed value-free demonstration")
	}
	if exec.Hash(q) != before {
		t.Fatal("mutated source query")
	}
	schema.Slots[0].Kind = "text"
	if q.Parameters[0].Kind != "number" {
		t.Fatal("aliased source parameters")
	}
}
func TestSQLRecoveryLearningParameterIdentityAndProbes(t *testing.T) {
	x := parameterLearningRecord(t)
	if !ExampleParametersValid(x) {
		t.Fatal("valid parameter schema")
	}
	probes, err := exampleValidationParameters(x)
	if err != nil || !reflect.DeepEqual(probes, []exec.Parameter{{Kind: "number", Value: "1"}, {Kind: "text", Value: "example"}}) {
		t.Fatal("native probes", err)
	}
	probes[0].Value = "mutated"
	again, _ := exampleValidationParameters(x)
	if again[0].Value != "1" {
		t.Fatal("shared validation probes")
	}
	for _, mutate := range []func(*ExampleRecord){func(x *ExampleRecord) { x.ParameterSchema.Version = "future" }, func(x *ExampleRecord) { x.ParameterSchema.Slots[0].Position = 2 }, func(x *ExampleRecord) { x.ParameterSchema.Slots[0].Kind = "text" }, func(x *ExampleRecord) { x.SQL += " AND false" }, func(x *ExampleRecord) { x.Question = "changed" }} {
		bad := x
		bad.ParameterSchema = x.ParameterSchema.Clone()
		mutate(&bad)
		if ExampleParametersValid(bad) {
			t.Fatal("substituted schema/content accepted")
		}
		if v, err := exampleValidationParameters(bad); v != nil || !errors.Is(err, exec.ErrBinding) {
			t.Fatal("invalid schema got probes", err)
		}
	}
	legacy := ExampleRecord{Topic: "topic", Question: "What records?", SQL: "SELECT id FROM analytics.sales"}
	legacy.Digest = exampleDigest(legacy.Topic, legacy.Question, legacy.SQL)
	if parameterExampleDigest(legacy.Topic, legacy.Question, legacy.SQL, nil) != legacy.Digest || learnedExampleText(legacy) != "question:"+legacy.Question+" sql:"+legacy.SQL {
		t.Fatal("legacy digest/text changed")
	}
	if values, err := exampleValidationParameters(legacy); err != nil || values != nil {
		t.Fatal("legacy became parameterized")
	}
}
func TestSQLRecoveryLearningParameterBoundsAndCancellation(t *testing.T) {
	for _, q := range []QueryRecord{{Question: "q", Parameters: make([]exec.Parameter, 65)}, {Question: "q", Parameters: []exec.Parameter{{Kind: "text", Value: string([]byte{0xff})}}}, {Question: strings.Repeat("x", 16384), Parameters: []exec.Parameter{{Kind: "text", Value: "x"}}}, {Question: "q", Parameters: []exec.Parameter{{Kind: "number", Value: "NaN"}}}} {
		if _, schema, err := learnedParameterSchema(context.Background(), q); schema != nil || err == nil {
			t.Fatal("unbounded/invalid evidence accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := learnedParameterSchema(ctx, QueryRecord{}); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored cancellation")
	}
	if _, _, err := learnedParameterSchema(nil, QueryRecord{}); err == nil {
		t.Fatal("nil context")
	}
}
func TestSQLRecoveryLearningParameterPortableVersion(t *testing.T) {
	x := parameterLearningRecord(t)
	row := PortableExample{SchemaVersion: 2, Question: x.Question, SQL: x.SQL, Digest: x.Digest, ParameterSchema: x.ParameterSchema.Clone()}
	if !portableExampleValid(row) {
		t.Fatal("valid portable")
	}
	raw, _ := json.Marshal(row)
	var decoded PortableExample
	if json.Unmarshal(raw, &decoded) != nil || !portableExampleValid(decoded) || parameterExampleDigest(x.Topic, decoded.Question, decoded.SQL, decoded.ParameterSchema) != row.Digest {
		t.Fatal("portable roundtrip")
	}
	row.SchemaVersion = 1
	if portableExampleValid(row) {
		t.Fatal("version downgrade")
	}
	row.SchemaVersion = 2
	row.ParameterSchema = nil
	if portableExampleValid(row) {
		t.Fatal("lost schema accepted as versioned example")
	}
	row.SchemaVersion = 1
	if !portableExampleValid(row) {
		t.Fatal("legacy portable rejected")
	}
}
func TestSQLRecoveryLearningParameterNativeReviewUsesOnlyProbes(t *testing.T) {
	e := unitEnvelope(t)
	binding, _ := retainedSourceReader{}.Binding(context.Background(), e, "source", "context")
	origin := ExampleOrigin{SchemaVersion: 1, Locale: "en", TopicVersion: "v1", Context: "context", SourceBindingDigest: exec.Hash(binding)}
	x := parameterLearningRecord(t)
	x.Origin = origin
	x.State = "candidate"
	x.Weight = 2.0 / 3.0
	x.PositiveEvidence = 1
	x.Version = 1
	repo := newUnitRepository()
	repo.examples[x.ID] = x
	reader := &unitTopicReader{current: map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)}}
	validator := &unitValidator{}
	service := &Service{repo: repo, topics: reader, sources: retainedSourceReader{}, validator: validator, router: &preflightRouter{result: learningRoute(origin, binding)}}
	active, err := service.ExampleState(context.Background(), e, ExampleStateRequest{ExampleID: x.ID, State: "active", ExpectedVersion: 1, ReviewNote: "Reviewed typed slots"})
	if err != nil || active.State != "active" || len(validator.requests) != 1 {
		t.Fatal("review", err)
	}
	want, _ := exampleValidationParameters(x)
	if !reflect.DeepEqual(validator.requests[0].Parameters, want) {
		t.Fatal("review used private bindings or omitted slots")
	}
	active.ParameterSchema.Slots[0].Kind = "text"
	if repo.examples[x.ID].ParameterSchema.Slots[0].Kind != "number" {
		t.Fatal("returned schema aliases repository")
	}
	// Candidate schema tampering is rejected before native access/state mutation.
	bad := repo.examples[x.ID]
	bad.ParameterSchema = bad.ParameterSchema.Clone()
	bad.ParameterSchema.Slots[0].Kind = "text"
	repo.examples[x.ID] = bad
	_, err = service.ExampleState(context.Background(), e, ExampleStateRequest{ExampleID: x.ID, State: "active", ReviewNote: "review"})
	if !errors.Is(err, exec.ErrBinding) || len(validator.requests) != 1 {
		t.Fatal("schema tampering reached validator", err)
	}
}
