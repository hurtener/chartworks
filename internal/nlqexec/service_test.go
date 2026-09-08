package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

type sequenceValidator struct {
	errors []error
	calls  int
}

func (v *sequenceValidator) Validate(context.Context, identity.Envelope, exec.Request) (exec.Plan, error) {
	v.calls++
	if len(v.errors) == 0 {
		return exec.Plan{}, nil
	}
	err := v.errors[0]
	v.errors = v.errors[1:]
	return exec.Plan{}, err
}

type sequenceGateway struct {
	responses []gateway.Generated
	calls     int
}

func (g *sequenceGateway) Generate(context.Context, gateway.Call, *gateway.Budget, string, string, string, *gateway.Schema) (gateway.Generated, error) {
	g.calls++
	if len(g.responses) == 0 {
		return gateway.Generated{}, gateway.ErrOutput
	}
	response := g.responses[0]
	g.responses = g.responses[1:]
	return response, nil
}

func (*sequenceGateway) Embed(context.Context, gateway.Call, *gateway.Budget, string, []string) (gateway.Embedded, error) {
	return gateway.Embedded{}, nil
}
func (*sequenceGateway) Rerank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	return gateway.Ranked{}, nil
}
func (*sequenceGateway) VisualRank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	return gateway.Ranked{}, nil
}
func (*sequenceGateway) Space() string                          { return "test" }
func (*sequenceGateway) EmbeddingSpace() gateway.EmbeddingSpace { return gateway.EmbeddingSpace{} }
func (*sequenceGateway) Close()                                 {}

func testEnvelope(t *testing.T) identity.Envelope {
	t.Helper()
	e, err := identity.FromVerified("tenant", "actor", "session", []string{
		"query.plan", "query.execute", "cw.topic.read:topic", "cw.source.query:source",
		"cw.dataset.query:dataset", "cw.execution_context.use:context",
	}, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func testGeneration(t *testing.T, e identity.Envelope) (admission, nlq.GenerationContext, gateway.Call, *gateway.Budget) {
	t.Helper()
	assembler, err := nlq.NewDefaultContextAssembler()
	if err != nil {
		t.Fatal(err)
	}
	assembled, err := assembler.Assemble(context.Background(), nlq.ContextInput{
		Locale: nlq.LanguageEnglish, Strategy: nlq.StrategySingleTopic, Topic: "topic", TopicVersion: "v1", Question: "What is revenue?",
		Constraints: &nlq.ConstraintState{Allowed: true, Required: []nlq.MandatoryConstraint{{ID: "required-filter", Kind: "filter", Text: "tenant_id is the signed tenant"}}},
		Metrics:     []nlq.PinnedMetric{{ID: "revenue", Text: "sum of amount"}},
	}, nlq.TierMedium)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := assembler.ResolvePrecedence(context.Background(), nlq.GenerationInput{
		Context: assembled,
		Default: []nlq.Instruction{{Key: "default", Text: "use the reviewed metric"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resources := []accessResourceForTest{
		{kind: "topic", permission: "read", id: "topic"},
		{kind: "source", permission: "query", id: "source"},
		{kind: "dataset", permission: "query", id: "dataset"},
		{kind: "execution_context", permission: "use", id: "context"},
	}
	accessResources := make([]access.Resource, len(resources))
	for i, resource := range resources {
		accessResources[i] = access.Resource{Tenant: e.Tenant(), Kind: resource.kind, Permission: resource.permission, ID: resource.id}
	}
	call, err := gateway.Authorize(e, "query.plan", "nlq:test", accessResources...)
	if err != nil {
		t.Fatal(err)
	}
	budget, err := gateway.NewBudget(call, gateway.Limits{Calls: 3, Tokens: 1 << 20, Duration: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return admission{assembled: assembled, binding: exec.Binding{Dialect: "postgres"}, source: "source", context: "context"}, generation, call, budget
}

// Keep the test's resource construction readable without exporting a helper
// from access solely for this package's failure-path tests.
type accessResourceForTest struct{ kind, permission, id string }

func TestGenerateAndValidateUsesOneValidationCorrection(t *testing.T) {
	e := testEnvelope(t)
	admitted, generation, call, budget := testGeneration(t, e)
	invalid := gateway.Generated{JSON: json.RawMessage(`{"sql":"DELETE FROM analytics.sales","parameters":[],"assumptions":[],"ambiguities":[]}`), Receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: "sqlgen"}}}}
	valid := gateway.Generated{JSON: json.RawMessage(`{"sql":"SELECT id FROM analytics.sales","parameters":[],"assumptions":[],"ambiguities":[]}`), Receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: "sqlfix"}}}}
	validator := &sequenceValidator{errors: []error{exec.ErrUnsafe, nil}}
	model := &sequenceGateway{responses: []gateway.Generated{invalid, valid}}
	service := &Service{validator: validator, engine: model}

	candidate, fixes, receipt, _, err := service.generateAndValidate(context.Background(), e, admitted, generation, call, budget, "")
	if err != nil {
		t.Fatal(err)
	}
	if fixes != 1 || validator.calls != 2 || model.calls != 2 || len(receipt.Calls) != 2 {
		t.Fatalf("correction accounting: fixes=%d validator=%d model=%d receipt=%#v", fixes, validator.calls, model.calls, receipt)
	}
	if candidate.SQL != "SELECT id FROM analytics.sales" {
		t.Fatalf("unexpected corrected candidate: %#v", candidate)
	}
}

func TestExecutionRepairableOnlyAcceptsRegisteredQueryError(t *testing.T) {
	if !executionRepairable(exec.ExecutionReport{Attempt: exec.Attempt{Status: "failed", Code: "query_error"}}, exec.ErrQuery) {
		t.Fatal("registered query error should spend the bounded correction budget")
	}
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{name: "binding", err: exec.ErrBinding},
		{name: "limit", err: exec.ErrLimit},
		{name: "uncertain", err: exec.ErrUncertain},
		{name: "transport", err: store.ErrUnavailable},
		{name: "unclassified", err: errors.New("unclassified")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if executionRepairable(exec.ExecutionReport{Attempt: exec.Attempt{Status: "failed", Code: "query_error"}}, tc.err) {
				t.Fatalf("unsafe correction admitted for %v", tc.err)
			}
		})
	}
	if executionRepairable(exec.ExecutionReport{Attempt: exec.Attempt{Status: "uncertain", Code: "query_error"}}, nil) {
		t.Fatal("uncertain remote outcome must never be corrected")
	}
}

type retainedTopicReader struct {
	publications map[string]topics.Contract
	versions     []string
}

func (r *retainedTopicReader) Contract(context.Context, identity.Envelope, string) (topics.Contract, error) {
	return topics.Contract{}, store.ErrNotFound
}

func (r *retainedTopicReader) RetainedContract(_ context.Context, _ identity.Envelope, topic, version string) (topics.Contract, error) {
	r.versions = append(r.versions, topic+"/"+version)
	publication, ok := r.publications[topic+"/"+version]
	if !ok {
		return topics.Contract{}, store.ErrNotFound
	}
	return publication, nil
}

type retainedSourceReader struct{}

func (retainedSourceReader) Binding(context.Context, identity.Envelope, string, string) (exec.Binding, error) {
	return exec.Binding{Tenant: "tenant", Source: "source", Context: "context", Revision: 1, Dialect: "postgres", Contract: "contract", Fingerprint: strings.Repeat("a", 64), Relations: []exec.Relation{{ID: "dataset", Schema: "analytics", Name: "sales", Columns: []exec.Column{{Name: "id", NativeType: "integer"}}}}}, nil
}

func TestRetainedAdmissionUsesExactTopicPins(t *testing.T) {
	e := testEnvelope(t)
	published := func(topic, version, dataset string) topics.Contract {
		return topics.Contract{Publication: topics.Published{
			State: topics.State{Topic: topic, Version: version, Archived: true},
			Definition: topics.Definition{Topic: topic, Version: version, Datasets: []topics.Dataset{{
				ID: dataset, Source: topics.Binding{Source: "source", Context: "context", Dataset: dataset, SourceRevision: 1},
			}}},
		}}
	}
	reader := &retainedTopicReader{publications: map[string]topics.Contract{
		"topic/v1":     published("topic", "v1", "dataset"),
		"topic-two/v2": published("topic-two", "v2", "dataset-two"),
	}}
	service := &Service{topics: reader, sources: retainedSourceReader{}}
	query := QueryRecord{Topics: []string{"topic", "topic-two"}, TopicVersions: []string{"v1", "v2"}, Context: "context"}
	admitted, err := service.retainedAdmission(context.Background(), e, query)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reader.versions, []string{"topic/v1", "topic-two/v2"}) {
		t.Fatalf("retained versions were not read in query order: %#v", reader.versions)
	}
	if admitted.source != "source" || admitted.context != "context" || admitted.binding.Context != "context" || len(admitted.resources) != 6 {
		t.Fatalf("retained admission lost exact source/resource pins: %#v", admitted)
	}
}
