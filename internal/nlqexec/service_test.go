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
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
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

type unitRepository struct {
	sessions   map[string]SessionRecord
	queries    map[string]QueryRecord
	operations map[string]QueryRecord
	examples   map[string]ExampleRecord
	feedback   []FeedbackRecord
}

type errorUnitRepository struct {
	*unitRepository
	operationErr error
	queryErr     error
	updateErr    error
}

func (r *errorUnitRepository) ReadOperation(ctx context.Context, scope store.Scope, operation string) (QueryRecord, error) {
	if r.operationErr != nil {
		return QueryRecord{}, r.operationErr
	}
	return r.unitRepository.ReadOperation(ctx, scope, operation)
}

func (r *errorUnitRepository) ReadQuery(ctx context.Context, scope store.Scope, id string) (QueryRecord, error) {
	if r.queryErr != nil {
		return QueryRecord{}, r.queryErr
	}
	return r.unitRepository.ReadQuery(ctx, scope, id)
}

func (r *errorUnitRepository) UpdateQuery(ctx context.Context, scope store.Scope, value QueryRecord, expected int64) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	return r.unitRepository.UpdateQuery(ctx, scope, value, expected)
}

func newUnitRepository() *unitRepository {
	return &unitRepository{sessions: map[string]SessionRecord{}, queries: map[string]QueryRecord{}, operations: map[string]QueryRecord{}, examples: map[string]ExampleRecord{}}
}

func (r *unitRepository) CreateSession(_ context.Context, _ store.Scope, value SessionRecord) error {
	if _, exists := r.sessions[value.ID]; exists {
		return store.ErrConflict
	}
	r.sessions[value.ID] = value
	return nil
}

func (r *unitRepository) ReadSession(_ context.Context, _ store.Scope, id string) (SessionRecord, error) {
	value, ok := r.sessions[id]
	if !ok {
		return SessionRecord{}, store.ErrNotFound
	}
	return value, nil
}

func (r *unitRepository) CreateQuery(_ context.Context, _ store.Scope, value QueryRecord) error {
	if _, exists := r.queries[value.ID]; exists {
		return store.ErrConflict
	}
	r.queries[value.ID] = value
	return nil
}

func (r *unitRepository) ReadQuery(_ context.Context, _ store.Scope, id string) (QueryRecord, error) {
	value, ok := r.queries[id]
	if !ok {
		return QueryRecord{}, store.ErrNotFound
	}
	return value, nil
}

func (r *unitRepository) ReadOperation(_ context.Context, _ store.Scope, operation string) (QueryRecord, error) {
	value, ok := r.operations[operation]
	if !ok {
		return QueryRecord{}, store.ErrNotFound
	}
	return value, nil
}

func (r *unitRepository) ReadExample(_ context.Context, _ store.Scope, id string) (ExampleRecord, error) {
	value, ok := r.examples[id]
	if !ok {
		return ExampleRecord{}, store.ErrNotFound
	}
	return value, nil
}

func (r *unitRepository) UpdateQuery(_ context.Context, _ store.Scope, value QueryRecord, expected int64) error {
	current, ok := r.queries[value.ID]
	if !ok {
		return store.ErrNotFound
	}
	if current.Revision != expected {
		return store.ErrConflict
	}
	r.queries[value.ID] = value
	if value.Operation != "" {
		r.operations[value.Operation] = value
	}
	return nil
}

func (r *unitRepository) RecordFeedback(_ context.Context, _ store.Scope, value FeedbackRecord) error {
	r.feedback = append(r.feedback, value)
	return nil
}

func (r *unitRepository) UpsertExample(_ context.Context, _ store.Scope, value ExampleRecord) (ExampleRecord, error) {
	for id, existing := range r.examples {
		if existing.Topic == value.Topic && existing.Digest == value.Digest {
			existing.EvidenceCount++
			existing.Weight += 0.05
			r.examples[id] = existing
			return existing, nil
		}
	}
	r.examples[value.ID] = value
	return value, nil
}

func (r *unitRepository) ListExamples(_ context.Context, _ store.Scope, topic string, limit int) ([]ExampleRecord, error) {
	out := make([]ExampleRecord, 0, len(r.examples))
	for _, value := range r.examples {
		if value.Topic == topic && (value.State == "candidate" || value.State == "active") {
			out = append(out, value)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *unitRepository) SetExampleState(_ context.Context, _ store.Scope, id, state string) (ExampleRecord, error) {
	value, ok := r.examples[id]
	if !ok {
		return ExampleRecord{}, store.ErrNotFound
	}
	value.State = state
	r.examples[id] = value
	return value, nil
}

type unitTopicReader struct {
	current  map[string]topics.Contract
	retained map[string]topics.Contract
}

func (r *unitTopicReader) Contract(_ context.Context, _ identity.Envelope, topic string) (topics.Contract, error) {
	value, ok := r.current[topic]
	if !ok {
		return topics.Contract{}, store.ErrNotFound
	}
	return value, nil
}

func (r *unitTopicReader) RetainedContract(_ context.Context, _ identity.Envelope, topic, version string) (topics.Contract, error) {
	value, ok := r.retained[topic+"/"+version]
	if !ok {
		return topics.Contract{}, store.ErrNotFound
	}
	return value, nil
}

type unitValidator struct {
	errors   []error
	requests []exec.Request
}

func (v *unitValidator) Validate(_ context.Context, _ identity.Envelope, request exec.Request) (exec.Plan, error) {
	v.requests = append(v.requests, request)
	if len(v.errors) == 0 {
		return exec.Plan{}, nil
	}
	err := v.errors[0]
	v.errors = v.errors[1:]
	return exec.Plan{}, err
}

type unitExecutor struct {
	reports []exec.ExecutionReport
	errors  []error
	calls   int
}

func (x *unitExecutor) Execute(_ context.Context, _ identity.Envelope, _ exec.Plan, _ exec.Options) (exec.ExecutionReport, error) {
	x.calls++
	var report exec.ExecutionReport
	if len(x.reports) > 0 {
		report = x.reports[0]
		x.reports = x.reports[1:]
	}
	if len(x.errors) == 0 {
		return report, nil
	}
	err := x.errors[0]
	x.errors = x.errors[1:]
	return report, err
}

func unitEnvelope(t *testing.T) identity.Envelope {
	t.Helper()
	e, err := identity.FromVerified("tenant", "actor", "session", []string{
		"query.plan", "query.execute", "feedback.write", "reporting.sql.read",
		"cw.topic.read:topic", "cw.source.query:source", "cw.dataset.query:dataset", "cw.execution_context.use:context",
	}, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func unitContract(topic, version, source, contextID, dataset string, active, archived bool) topics.Contract {
	return topics.Contract{Publication: topics.Published{
		State: topics.State{Topic: topic, Version: version, Active: active, Archived: archived},
		Definition: topics.Definition{Topic: topic, Version: version, Datasets: []topics.Dataset{{
			ID: dataset, Source: topics.Binding{Source: source, Context: contextID, Dataset: dataset, SourceRevision: 1},
		}}},
	}}
}

func unitQuery(e identity.Envelope, id, topic, version, contextID string, stale bool) QueryRecord {
	return QueryRecord{ID: id, Session: e.Session(), Topic: topic, Topics: []string{topic}, TopicVersions: []string{version}, Context: contextID, Locale: nlq.LanguageEnglish, Question: "What is revenue?", SQL: "SELECT id FROM analytics.sales", Status: "planned", EvidenceStale: stale, Route: nlqroute.RouteResult{Topic: topic, Topics: []string{topic}, TopicVersions: []string{version}, Outcome: nlq.StrategySingleTopic}, Revision: 1}
}

func unitResult(status string) exec.ExecutionReport {
	return exec.ExecutionReport{Attempt: exec.Attempt{Status: status}, Result: &exec.Result{Outcome: "complete", Schema: []exec.Field{{Name: "id", Type: "integer"}}, Rows: [][]json.RawMessage{{json.RawMessage(`1`)}}, Bytes: 1}}
}

func TestServiceValidationAndMetadataBoundaries(t *testing.T) {
	e := unitEnvelope(t)
	if _, err := New(nil, nil, nil, nil, nil, nil, nil); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("nil production dependencies accepted")
	}
	if !canInspect(e) || canInspect(testEnvelope(t)) {
		t.Fatal("inspection scope was not isolated")
	}
	for _, item := range []struct {
		name string
		in   []nlq.Instruction
		want bool
	}{
		{name: "valid", in: []nlq.Instruction{{Key: "hint", Text: "Use revenue"}}, want: true},
		{name: "duplicate", in: []nlq.Instruction{{Key: "hint", Text: "one"}, {Key: "hint", Text: "two"}}},
		{name: "invalid-key", in: []nlq.Instruction{{Key: "bad key", Text: "one"}}},
		{name: "empty", in: []nlq.Instruction{{Key: "hint"}}},
		{name: "nul", in: []nlq.Instruction{{Key: "hint", Text: "bad\x00text"}}},
		{name: "too-long", in: []nlq.Instruction{{Key: "hint", Text: strings.Repeat("x", maxInstructionText+1)}}},
	} {
		t.Run("instruction-"+item.name, func(t *testing.T) {
			if got := instructionValid(item.in); got != item.want {
				t.Fatalf("instruction validity=%v want %v", got, item.want)
			}
		})
	}
	validQuestion := QuestionRequest{Topic: "topic", Context: "context", Locale: nlq.LanguageEnglish, Question: "show revenue", EditBase: []nlq.Instruction{{Key: "edit", Text: "preserve filters"}}}
	if err := validateQuestion(validQuestion); err != nil {
		t.Fatal("valid question rejected", err)
	}
	for _, question := range []QuestionRequest{
		{Topic: "topic", Context: "context", Locale: nlq.LanguageEnglish, Question: " leading"},
		{Topic: "topic", Context: "context", Locale: nlq.LanguageEnglish, Question: "line\nfeed"},
		{Topic: "topic", Context: "context", Locale: "fr", Question: "question"},
		{Topic: "topic", Context: "bad context", Locale: nlq.LanguageEnglish, Question: "question"},
	} {
		if !errors.Is(validateQuestion(question), ErrInvalid) {
			t.Fatalf("invalid question accepted: %#v", question)
		}
	}
	if !errors.Is(requireQuestionAction(identity.Envelope{}, "query.plan", validQuestion), access.ErrForbidden) {
		t.Fatal("unauthorized question action accepted")
	}
	if !errors.Is(requireQuestionAction(e, "query.plan", QuestionRequest{Context: "context", Locale: nlq.LanguageEnglish, Question: "question"}), ErrInvalid) {
		t.Fatal("question without topic accepted")
	}
	if err := requireQuestionAction(e, "query.plan", QuestionRequest{Topic: "topic", Topics: []string{"topic", "topic"}, Context: "context"}); err != nil {
		t.Fatal("duplicate topic reach rejected", err)
	}
	for _, candidate := range []struct {
		name  string
		value generatedCandidate
		want  bool
	}{
		{name: "valid", value: generatedCandidate{SQL: "SELECT 1"}, want: true},
		{name: "trim", value: generatedCandidate{SQL: " SELECT 1"}},
		{name: "nul", value: generatedCandidate{SQL: "SELECT\x001"}},
		{name: "long-assumption", value: generatedCandidate{SQL: "SELECT 1", Assumptions: []string{strings.Repeat("x", 1025)}}},
	} {
		t.Run("candidate-"+candidate.name, func(t *testing.T) {
			if got := validCandidate(candidate.value); got != candidate.want {
				t.Fatalf("candidate validity=%v want %v", got, candidate.want)
			}
		})
	}
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "unsafe", err: exec.ErrUnsafe, want: "validation_unsafe"},
		{name: "limit", err: exec.ErrLimit, want: "validation_limit"},
		{name: "unsupported", err: exec.ErrUnsupported, want: "validation_unsupported"},
		{name: "other", err: errors.New("validation"), want: "validation_failed"},
		{name: "nil", want: ""},
	} {
		t.Run("validation-code-"+tc.name, func(t *testing.T) {
			if got := validationCode(tc.err, "SELECT 1"); got != tc.want {
				t.Fatalf("validation code=%q want %q", got, tc.want)
			}
		})
	}
	for status, want := range map[string]error{"failed": ErrExecutionFailed, "uncertain": exec.ErrUncertain, "interrupted": exec.ErrUncertain, "cancelled": exec.ErrCancelled, "timed_out": exec.ErrTimeout, "succeeded": nil} {
		if !errors.Is(replayError(status), want) {
			t.Fatalf("replay status %s: got %v want %v", status, replayError(status), want)
		}
	}
	if executionErrorCode(exec.ExecutionReport{Attempt: exec.Attempt{Code: "registered"}}, nil) != "registered" || executionErrorCode(exec.ExecutionReport{}, exec.ErrBinding) != "context_changed" || executionErrorCode(exec.ExecutionReport{}, exec.ErrLimit) != "limit_exceeded" || executionErrorCode(exec.ExecutionReport{}, errors.New("other")) != "execution_failed" {
		t.Fatal("execution error code classification changed")
	}
	joined := appendReceipts(gateway.Receipt{Calls: []gateway.Usage{{Role: "first"}}}, gateway.Receipt{Calls: []gateway.Usage{{Role: "second"}}, Warning: "degraded"})
	if len(joined.Calls) != 2 || joined.Warning != "degraded" {
		t.Fatalf("receipt append lost observed calls: %#v", joined)
	}
	route := nlqroute.RouteResult{Topic: "topic", Topics: []string{"topic", "topic-two"}, TopicVersions: []string{"v1", "v2"}, Context: nil}
	if routeVersion(route, "topic-two") != "v2" || routeVersion(route, "missing") != "" || assumptions(route) != nil || ambiguities(route) != nil {
		t.Fatal("route metadata helpers changed")
	}
	if routeDigest(route) != "topic:topic,topic-two:v1,v2" || executionPartition(unitQuery(e, "query-partition", "topic", "v1", "context", false)) == "" {
		t.Fatal("route/execution partitions lost stable identity")
	}
	route.Clarification = &nlqroute.Clarification{Reason: "choose"}
	if len(ambiguities(route)) != 1 || ambiguities(route)[0] != "choose" {
		t.Fatal("clarification metadata was not retained")
	}
}

func TestServicePublicBoundaryValidation(t *testing.T) {
	service := &Service{}
	question := QuestionRequest{Topic: "topic", Context: "context", Locale: nlq.LanguageEnglish, Question: "show revenue"}
	var nilContext context.Context
	if _, err := service.Preflight(nilContext, identity.Envelope{}, PreflightRequest{QuestionRequest: question}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("nil preflight context was accepted")
	}
	if _, err := service.Preflight(context.Background(), testEnvelope(t), PreflightRequest{QuestionRequest: question}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("preflight without its action was accepted")
	}
	if _, err := service.Plan(context.Background(), identity.Envelope{}, PlanRequest{QuestionRequest: question}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("unauthenticated plan was accepted")
	}
	if _, err := service.Refine(context.Background(), testEnvelope(t), RefineRequest{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("malformed refinement was accepted")
	}
	if _, err := service.Run(context.Background(), testEnvelope(t), RunRequest{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("malformed run was accepted")
	}
	if err := service.Feedback(nilContext, testEnvelope(t), FeedbackRequest{QueryID: "query", Verdict: "positive"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("nil feedback context was accepted")
	}
	withoutFeedback, err := identity.FromVerified("tenant", "actor", "session", []string{"query.plan", "cw.topic.read:topic"}, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ExampleState(context.Background(), withoutFeedback, ExampleStateRequest{ExampleID: "example", State: "active"}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("example state without feedback authority was accepted")
	}
	withoutActions, err := identity.FromVerified("tenant", "actor", "session", nil, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Examples(context.Background(), withoutActions, "topic", 1); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("example read without an action was accepted")
	}
}

func TestServiceDurableFailureBoundaries(t *testing.T) {
	e := unitEnvelope(t)
	repo := newUnitRepository()
	current := unitContract("topic", "v1", "source", "context", "dataset", true, false)
	reader := &unitTopicReader{current: map[string]topics.Contract{"topic": current}, retained: map[string]topics.Contract{"topic/v1": current}}
	validator := &unitValidator{}
	executor := &unitExecutor{reports: []exec.ExecutionReport{unitResult("succeeded")}}
	service := &Service{topics: reader, sources: retainedSourceReader{}, validator: validator, executor: executor, engine: &sequenceGateway{}, repo: repo}
	q := unitQuery(e, "query-1", "topic", "v1", "context", false)
	repo.queries[q.ID] = q
	if err := service.ensureSession(context.Background(), e, QuestionRequest{Context: "context", Locale: nlq.LanguageEnglish}, []string{"topic"}); err != nil {
		t.Fatal("session creation", err)
	}
	if err := service.ensureSession(context.Background(), e, QuestionRequest{Context: "context", Locale: nlq.LanguageSpanish}, []string{"topic"}); err != nil {
		t.Fatal("session conflict read", err)
	}
	if err := service.ensureSession(context.Background(), e, QuestionRequest{Context: "other-context", Locale: nlq.LanguageEnglish}, []string{"topic"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("session context changed without conflict")
	}
	admitted, err := service.currentAdmission(context.Background(), e, q)
	if err != nil || admitted.source != "source" || admitted.binding.Context != "context" {
		t.Fatalf("current admission: %#v %v", admitted, err)
	}
	q.EvidenceStale = true
	retained, err := service.retainedAdmission(context.Background(), e, q)
	if err != nil || retained.source != "source" {
		t.Fatalf("retained admission: %#v %v", retained, err)
	}
	result, err := service.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "run-1", Rows: 10, Bytes: 4096})
	if err != nil || result.Status != "succeeded" || result.Execution.Result == nil || result.SQL == "" {
		t.Fatalf("successful run boundary: %#v %v", result, err)
	}
	replayed, err := service.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "run-1"})
	if err != nil || replayed.Status != "succeeded" {
		t.Fatalf("idempotent run replay: %#v %v", replayed, err)
	}
	other := unitQuery(e, "query-other", "topic", "v1", "context", false)
	repo.operations["run-other"] = other
	if _, err = service.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "run-other"}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("operation key was reused for another query: %v", err)
	}

	feedbackQuery := unitQuery(e, "query-feedback", "topic", "v1", "context", false)
	repo.queries[feedbackQuery.ID] = feedbackQuery
	if err = service.Feedback(context.Background(), e, FeedbackRequest{QueryID: feedbackQuery.ID, Verdict: "positive", Note: "accepted"}); err != nil {
		t.Fatal("positive feedback", err)
	}
	if len(repo.feedback) != 1 || len(repo.examples) != 1 {
		t.Fatalf("positive feedback did not persist learning state: feedback=%d examples=%d", len(repo.feedback), len(repo.examples))
	}
	if err = service.Feedback(context.Background(), e, FeedbackRequest{QueryID: feedbackQuery.ID, Verdict: "negative", Note: "rejected"}); err != nil {
		t.Fatal("negative feedback", err)
	}
	if len(repo.feedback) != 2 || len(repo.examples) != 1 {
		t.Fatal("negative feedback unexpectedly created an example")
	}
	withoutResources, err := identity.FromVerified("tenant", "actor", "session", []string{"feedback.write"}, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	feedbackCount := len(repo.feedback)
	if err = service.Feedback(context.Background(), withoutResources, FeedbackRequest{QueryID: feedbackQuery.ID, Verdict: "positive"}); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("feedback crossed addressed-resource authority: %v", err)
	}
	if len(repo.feedback) != feedbackCount {
		t.Fatal("unauthorized feedback mutated the learning receipt")
	}
	if err = service.Feedback(context.Background(), e, FeedbackRequest{QueryID: feedbackQuery.ID, Verdict: "positive", Correction: "SELECT id FROM analytics.sales"}); err != nil {
		t.Fatal("validated correction feedback", err)
	}
	for _, in := range []FeedbackRequest{{QueryID: "missing", Verdict: "positive"}, {QueryID: feedbackQuery.ID, Verdict: "bad"}, {QueryID: feedbackQuery.ID, Verdict: "positive", Note: string([]byte{0})}} {
		if err = service.Feedback(context.Background(), e, in); !errors.Is(err, ErrInvalid) && !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("invalid feedback result: %v", err)
		}
	}
	foreign := feedbackQuery
	foreign.ID = "query-foreign"
	foreign.Session = "other-session"
	repo.queries[foreign.ID] = foreign
	if err = service.Feedback(context.Background(), e, FeedbackRequest{QueryID: foreign.ID, Verdict: "positive"}); !errors.Is(err, ErrForeignSession) {
		t.Fatal("foreign feedback accepted")
	}
	preflight := feedbackQuery
	preflight.ID, preflight.Status, preflight.SQL = "query-preflight", "preflight", ""
	repo.queries[preflight.ID] = preflight
	if err = service.Feedback(context.Background(), e, FeedbackRequest{QueryID: preflight.ID, Verdict: "positive"}); !errors.Is(err, ErrNoPlan) {
		t.Fatal("preflight feedback accepted")
	}

	var exampleID string
	for id := range repo.examples {
		exampleID = id
	}
	active, err := service.ExampleState(context.Background(), e, ExampleStateRequest{ExampleID: exampleID, State: "active"})
	if err != nil || active.State != "active" {
		t.Fatalf("example state transition: %#v %v", active, err)
	}
	if listed, listErr := service.Examples(context.Background(), e, "topic", 2); listErr != nil || len(listed) != 1 || listed[0].State != "active" {
		t.Fatalf("example listing: %#v %v", listed, listErr)
	}
	if _, err = service.ExampleState(context.Background(), e, ExampleStateRequest{ExampleID: "missing", State: "active"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("missing example state did not remain non-disclosing")
	}
	if _, err = service.Examples(context.Background(), e, "topic", 0); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid example limit accepted")
	}

	failedRepo := newUnitRepository()
	failedRepo.queries["failed"] = unitQuery(e, "failed", "topic", "v1", "context", false)
	failedRepo.queries["failed"] = func() QueryRecord { value := failedRepo.queries["failed"]; value.Status = "planned"; return value }()
	failedService := &Service{topics: reader, sources: retainedSourceReader{}, validator: &unitValidator{}, executor: &unitExecutor{}, engine: &sequenceGateway{}, repo: failedRepo}
	if _, err = failedService.finishRun(context.Background(), e, failedRepo.queries["failed"], exec.ExecutionReport{Attempt: exec.Attempt{Status: "failed"}}, 0, nil); !errors.Is(err, ErrExecutionFailed) {
		t.Fatal("failed execution did not return typed error")
	}
	if _, err = failedService.finishRun(context.Background(), e, failedRepo.queries["failed"], exec.ExecutionReport{Attempt: exec.Attempt{Status: "uncertain"}}, 0, nil); !errors.Is(err, exec.ErrUncertain) {
		t.Fatal("uncertain execution did not remain uncertain")
	}
}

func TestServiceRunFailureAndAdmissionBranches(t *testing.T) {
	ctx := context.Background()
	e := unitEnvelope(t)
	reader := &unitTopicReader{
		current:  map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)},
		retained: map[string]topics.Contract{"topic/v1": unitContract("topic", "v1", "source", "context", "dataset", true, true)},
	}

	newService := func(repo Repository, validator PlanValidator, executor PlanExecutor) *Service {
		return &Service{topics: reader, sources: retainedSourceReader{}, validator: validator, executor: executor, repo: repo}
	}
	newRepo := func(query QueryRecord) *unitRepository {
		repo := newUnitRepository()
		repo.queries[query.ID] = query
		return repo
	}

	if _, err := newService(newUnitRepository(), &unitValidator{}, &unitExecutor{}).Run(ctx, func() identity.Envelope {
		value, err := identity.FromVerified("tenant", "actor", "session", []string{"query.plan"}, time.Now().Add(time.Hour), nil)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}(), RunRequest{QueryID: "query", Operation: "operation"}); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("run without execute authority returned %v", err)
	}

	terminalRepo := newUnitRepository()
	terminalQuery := unitQuery(e, "query-terminal", "topic", "v1", "context", false)
	terminalRepo.operations["operation-terminal"] = func() QueryRecord {
		value := terminalQuery
		value.Status, value.Operation = "failed", "operation-terminal"
		return value
	}()
	terminalRepo.queries[terminalQuery.ID] = terminalRepo.operations["operation-terminal"]
	if _, err := newService(terminalRepo, &unitValidator{}, &unitExecutor{}).Run(ctx, e, RunRequest{QueryID: terminalQuery.ID, Operation: "operation-terminal"}); !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("terminal replay returned %v", err)
	}
	noResourceTerminal, err := identity.FromVerified("tenant", "actor", "session", []string{"query.execute"}, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	deniedTerminal := unitQuery(noResourceTerminal, "query-terminal-denied", "topic", "v1", "context", false)
	deniedTerminal.Status, deniedTerminal.Operation = "cancelled", "operation-terminal-denied"
	terminalDeniedRepo := newUnitRepository()
	terminalDeniedRepo.queries[deniedTerminal.ID] = deniedTerminal
	terminalDeniedRepo.operations["operation-terminal-denied"] = deniedTerminal
	if _, err = newService(terminalDeniedRepo, &unitValidator{}, &unitExecutor{}).Run(ctx, noResourceTerminal, RunRequest{QueryID: deniedTerminal.ID, Operation: "operation-terminal-denied"}); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("terminal replay exposed rows without retained resource authority: %v", err)
	}

	operationError := &errorUnitRepository{unitRepository: newUnitRepository(), operationErr: store.ErrUnavailable}
	if _, err := newService(operationError, &unitValidator{}, &unitExecutor{}).Run(ctx, e, RunRequest{QueryID: "query-operation-error", Operation: "operation"}); !errors.Is(err, store.ErrUnavailable) {
		t.Fatalf("operation read error returned %v", err)
	}
	queryError := &errorUnitRepository{unitRepository: newUnitRepository(), queryErr: store.ErrUnavailable}
	if _, err := newService(queryError, &unitValidator{}, &unitExecutor{}).Run(ctx, e, RunRequest{QueryID: "query-read-error", Operation: "operation"}); !errors.Is(err, store.ErrUnavailable) {
		t.Fatalf("query read error returned %v", err)
	}

	for name, mutate := range map[string]func(*QueryRecord){
		"foreign-session": func(value *QueryRecord) { value.Session = "other-session" },
		"preflight":       func(value *QueryRecord) { value.Status, value.SQL = "preflight", "" },
		"missing-sql":     func(value *QueryRecord) { value.SQL = "" },
	} {
		t.Run("no-plan-"+name, func(t *testing.T) {
			value := unitQuery(e, "query-"+name, "topic", "v1", "context", false)
			mutate(&value)
			repo := newRepo(value)
			if _, err := newService(repo, &unitValidator{}, &unitExecutor{}).Run(ctx, e, RunRequest{QueryID: value.ID, Operation: "operation-" + name}); !errors.Is(err, ErrNoPlan) {
				t.Fatalf("no-plan branch returned %v", err)
			}
		})
	}

	missingTopic := unitQuery(e, "query-missing-topic", "missing", "v1", "context", false)
	if _, err := newService(newRepo(missingTopic), &unitValidator{}, &unitExecutor{}).Run(ctx, e, RunRequest{QueryID: missingTopic.ID, Operation: "operation-missing-topic"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing current topic returned %v", err)
	}

	noResources, err := identity.FromVerified("tenant", "actor", "session", []string{"query.execute"}, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	unauthorizedQuery := unitQuery(noResources, "query-no-resources", "topic", "v1", "context", false)
	if _, err := newService(newRepo(unauthorizedQuery), &unitValidator{}, &unitExecutor{}).Run(ctx, noResources, RunRequest{QueryID: unauthorizedQuery.ID, Operation: "operation-no-resources"}); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("missing resource authority returned %v", err)
	}

	validatorError := unitQuery(e, "query-validator-error", "topic", "v1", "context", false)
	if _, err := newService(newRepo(validatorError), &unitValidator{errors: []error{exec.ErrUnsafe}}, &unitExecutor{}).Run(ctx, e, RunRequest{QueryID: validatorError.ID, Operation: "operation-validator-error"}); !errors.Is(err, exec.ErrUnsafe) {
		t.Fatalf("validator error returned %v", err)
	}

	stale := unitQuery(e, "query-stale", "topic", "v1", "context", true)
	staleRepo := newRepo(stale)
	staleExecutor := &unitExecutor{reports: []exec.ExecutionReport{unitResult("succeeded")}}
	if result, err := newService(staleRepo, &unitValidator{}, staleExecutor).Run(ctx, e, RunRequest{QueryID: stale.ID, Operation: "operation-stale"}); err != nil || !result.EvidenceStale {
		t.Fatalf("stale retained replay returned %#v %v", result, err)
	}

	failed := unitQuery(e, "query-executor-failure", "topic", "v1", "context", false)
	failedRepo := newRepo(failed)
	failedExecutor := &unitExecutor{reports: []exec.ExecutionReport{{Attempt: exec.Attempt{Status: "uncertain"}}}, errors: []error{exec.ErrUncertain}}
	if _, err := newService(failedRepo, &unitValidator{}, failedExecutor).Run(ctx, e, RunRequest{QueryID: failed.ID, Operation: "operation-executor-failure"}); !errors.Is(err, exec.ErrUncertain) {
		t.Fatalf("uncertain executor outcome returned %v", err)
	}

	updateErrorQuery := unitQuery(e, "query-update-error", "topic", "v1", "context", false)
	updateErrorRepo := &errorUnitRepository{unitRepository: newRepo(updateErrorQuery), updateErr: store.ErrUnavailable}
	updateErrorExecutor := &unitExecutor{reports: []exec.ExecutionReport{unitResult("succeeded")}}
	if _, err := newService(updateErrorRepo, &unitValidator{}, updateErrorExecutor).Run(ctx, e, RunRequest{QueryID: updateErrorQuery.ID, Operation: "operation-update-error"}); !errors.Is(err, store.ErrUnavailable) {
		t.Fatalf("query update error returned %v", err)
	}
}

func TestRefinementPreservesGovernedRouteSelections(t *testing.T) {
	e := unitEnvelope(t)
	old := unitQuery(e, "query-refine", "topic", "v1", "context", false)
	old.Route.Request = nlqroute.RouteRequest{
		Topic: "topic", Topics: []string{"topic"}, Context: "context", Locale: nlq.LanguageEnglish,
		Question: "What is revenue?", Kinds: []string{"measure"}, LimitPerKind: 2,
		References:  []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}},
		Choices:     []nlqroute.ChoiceSelection{{Pattern: "sales", Slot: "period", Value: "month"}},
		JoinChoices: []nlqroute.JoinChoice{{Topic: "topic", JoinID: "sales-items"}},
		MetricIDs:   []string{"revenue"}, Rerank: true,
	}
	question := refinementQuestion(old, QuestionRequest{
		Question: "Show revenue by month", References: []semantics.Reference{{Kind: semantics.KindDimension, ID: "month"}},
		Choices: []nlqroute.ChoiceSelection{{Pattern: "sales", Slot: "region", Value: "west"}},
		Joins:   []nlqroute.JoinChoice{{Topic: "topic-two", JoinID: "related-sales"}}, MetricIDs: []string{"margin"},
	})
	if question.Topic != old.Topic || !reflect.DeepEqual(question.Topics, old.Topics) || question.Context != old.Context || question.Locale != old.Locale || question.Question != "Show revenue by month" {
		t.Fatalf("refinement changed the signed session anchor: %#v", question)
	}
	if !reflect.DeepEqual(question.References, []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}, {Kind: semantics.KindDimension, ID: "month"}}) {
		t.Fatalf("reference base+delta was not retained: %#v", question.References)
	}
	if !reflect.DeepEqual(question.Choices, []nlqroute.ChoiceSelection{{Pattern: "sales", Slot: "period", Value: "month"}, {Pattern: "sales", Slot: "region", Value: "west"}}) {
		t.Fatalf("choice base+delta was not retained: %#v", question.Choices)
	}
	if !reflect.DeepEqual(question.Joins, []nlqroute.JoinChoice{{Topic: "topic", JoinID: "sales-items"}, {Topic: "topic-two", JoinID: "related-sales"}}) || !reflect.DeepEqual(question.MetricIDs, []string{"revenue", "margin"}) || !question.Rerank {
		t.Fatalf("governed route selections were not retained: %#v", question)
	}
	question.EditBase = replaceInstruction(question.EditBase, nlq.Instruction{Key: "previous_sql", Text: old.SQL})
	if len(question.EditBase) != 1 || question.EditBase[0].Text != old.SQL {
		t.Fatal("refinement did not retain the protected parent SQL base")
	}
}

func TestRunRepairRebuildsSealedContext(t *testing.T) {
	e := unitEnvelope(t)
	reader := &unitTopicReader{
		current:  map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)},
		retained: map[string]topics.Contract{"topic/v1": unitContract("topic", "v1", "source", "context", "dataset", true, true)},
	}
	admitted, _, _, _ := testGeneration(t, e)
	query := unitQuery(e, "query-repair", "topic", "v1", "context", false)
	query.SQL = "SELECT id, amount FROM analytics.sales WHERE created_at >= $1 ORDER BY id"
	query.Parameters = []exec.Parameter{{Kind: "text", Value: "2026-01-01"}}
	query.Generation.Context = admitted.assembled
	repo := newUnitRepository()
	repo.queries[query.ID] = query
	validator := &unitValidator{}
	executor := &unitExecutor{
		reports: []exec.ExecutionReport{
			{Attempt: exec.Attempt{Status: "failed", Code: "query_error"}},
			unitResult("succeeded"),
		},
		errors: []error{exec.ErrQuery},
	}
	inputTokens, outputTokens := 17, 9
	cost := 0.01
	engine := &sequenceGateway{responses: []gateway.Generated{
		{JSON: json.RawMessage(`{"sql":"SELECT id, amount FROM analytics.sales WHERE created_at >= $1  ORDER BY id","parameters":[{"kind":"text","value":"2026-01-01"}],"assumptions":[],"ambiguities":[]}`), Receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: "sqlfix", Attempts: 1, InputTokens: &inputTokens, OutputTokens: &outputTokens, CostUSD: &cost}}}},
	}}
	service := &Service{topics: reader, sources: retainedSourceReader{}, validator: validator, executor: executor, engine: engine, repo: repo}
	result, err := service.Run(context.Background(), e, RunRequest{QueryID: query.ID, Operation: "operation-repair"})
	if err != nil || result.Status != "succeeded" || result.ExecutionFixes != 1 || result.SQL != "SELECT id, amount FROM analytics.sales WHERE created_at >= $1  ORDER BY id" {
		t.Fatalf("execution repair did not complete through Run: result=%#v err=%v", result, err)
	}
	if len(validator.requests) != 2 || engine.calls != 1 {
		t.Fatalf("repair path did not validate/repair exactly once: validations=%d gateway_calls=%d", len(validator.requests), engine.calls)
	}
	stored := repo.queries[query.ID]
	if stored.ExecutionFixes != 1 || stored.Operation != "operation-repair" || stored.Generation.Strategy != nlq.GenerationEditBase || len(stored.Generation.Selected) != 1 || stored.Generation.Selected[0].Key != "previous_sql" || stored.Generation.Selected[0].Text != query.SQL {
		t.Fatalf("repair lost protected SQL delta or receipt metadata: %#v", stored)
	}
	if stored.Generation.Context.Topic != "topic" || stored.Generation.Context.TopicVersion != "v1" || len(stored.Generation.PinnedMetrics) != 1 || stored.Generation.PinnedMetrics[0].ID != "revenue" {
		t.Fatalf("repair rebuilt an incomplete governed context: %#v", stored.Generation)
	}
	if len(stored.Receipt.Calls) != 1 || stored.Receipt.Calls[0].Role != "sqlfix" || stored.Receipt.Calls[0].Attempts != 1 || stored.Receipt.Calls[0].InputTokens == nil || *stored.Receipt.Calls[0].InputTokens != inputTokens || stored.Receipt.Calls[0].OutputTokens == nil || *stored.Receipt.Calls[0].OutputTokens != outputTokens || stored.Receipt.Calls[0].CostUSD == nil || *stored.Receipt.Calls[0].CostUSD != cost {
		t.Fatalf("repair receipt was not durably preserved: %#v", stored.Receipt)
	}
}

func TestRunRepairRejectsUnsafeSemanticChanges(t *testing.T) {
	e := unitEnvelope(t)
	reader := &unitTopicReader{
		current:  map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)},
		retained: map[string]topics.Contract{"topic/v1": unitContract("topic", "v1", "source", "context", "dataset", true, true)},
	}
	admitted, _, _, _ := testGeneration(t, e)
	oldSQL := "SELECT id, amount FROM analytics.sales WHERE created_at >= $1 ORDER BY id"
	cases := []struct {
		name      string
		candidate string
	}{
		{name: "dropped-required-filter", candidate: "SELECT id, amount FROM analytics.sales ORDER BY id"},
		{name: "changed-governed-metric", candidate: "SELECT id, revenue FROM analytics.sales WHERE created_at >= $1 ORDER BY id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := unitQuery(e, "query-unsafe-"+tc.name, "topic", "v1", "context", false)
			query.SQL = oldSQL
			query.Parameters = []exec.Parameter{{Kind: "text", Value: "2026-01-01"}}
			query.Generation.Context = admitted.assembled
			repo := newUnitRepository()
			repo.queries[query.ID] = query
			executor := &unitExecutor{reports: []exec.ExecutionReport{{Attempt: exec.Attempt{Status: "failed", Code: "query_error"}}, unitResult("succeeded")}, errors: []error{exec.ErrQuery}}
			engine := &sequenceGateway{responses: []gateway.Generated{
				{JSON: json.RawMessage(`{"sql":"` + tc.candidate + `","parameters":[{"kind":"text","value":"2026-01-01"}],"assumptions":[],"ambiguities":[]}`), Receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: "sqlfix", Attempts: 1}}}},
			}}
			service := &Service{topics: reader, sources: retainedSourceReader{}, validator: &unitValidator{}, executor: executor, engine: engine, repo: repo}
			_, err := service.Run(context.Background(), e, RunRequest{QueryID: query.ID, Operation: "operation-unsafe-" + tc.name})
			if !errors.Is(err, ErrUnsafeCorrection) {
				t.Fatalf("unsafe correction returned %v", err)
			}
			if executor.calls != 1 {
				t.Fatalf("unsafe correction reached second execution: calls=%d", executor.calls)
			}
			stored := repo.queries[query.ID]
			if stored.SQL != oldSQL || len(stored.Receipt.Calls) != 1 || stored.Receipt.Calls[0].Role != "sqlfix" {
				t.Fatalf("unsafe correction lost protected query or receipt: %#v", stored)
			}
		})
	}
}

func TestRunRepairPersistsFailedCorrectionReceipt(t *testing.T) {
	e := unitEnvelope(t)
	reader := &unitTopicReader{
		current:  map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)},
		retained: map[string]topics.Contract{"topic/v1": unitContract("topic", "v1", "source", "context", "dataset", true, true)},
	}
	admitted, _, _, _ := testGeneration(t, e)
	query := unitQuery(e, "query-failed-correction", "topic", "v1", "context", false)
	query.Generation.Context = admitted.assembled
	repo := newUnitRepository()
	repo.queries[query.ID] = query
	executor := &unitExecutor{reports: []exec.ExecutionReport{{Attempt: exec.Attempt{Status: "failed", Code: "query_error"}}}, errors: []error{exec.ErrQuery}}
	inputTokens := 23
	cost := 0.02
	engine := &sequenceGateway{responses: []gateway.Generated{{JSON: json.RawMessage(`{"sql":`), Receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: "sqlfix", Attempts: 1, InputTokens: &inputTokens, CostUSD: &cost}}}}}}
	service := &Service{topics: reader, sources: retainedSourceReader{}, validator: &unitValidator{}, executor: executor, engine: engine, repo: repo}
	_, err := service.Run(context.Background(), e, RunRequest{QueryID: query.ID, Operation: "operation-failed-correction"})
	if !errors.Is(err, ErrGeneration) {
		t.Fatalf("failed correction returned %v", err)
	}
	stored := repo.queries[query.ID]
	if executor.calls != 1 || len(stored.Receipt.Calls) != 1 || stored.Receipt.Calls[0].Role != "sqlfix" || stored.Receipt.Calls[0].InputTokens == nil || *stored.Receipt.Calls[0].InputTokens != inputTokens || stored.Receipt.Calls[0].CostUSD == nil || *stored.Receipt.Calls[0].CostUSD != cost {
		t.Fatalf("failed correction receipt was not durably preserved: calls=%d receipt=%#v", executor.calls, stored.Receipt)
	}
}

func TestLearningOutputsRedactProtectedSQLAndReauthorizeTopic(t *testing.T) {
	e := unitEnvelope(t)
	reader := &unitTopicReader{current: map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)}}
	repo := newUnitRepository()
	repo.examples["example-1"] = ExampleRecord{ID: "example-1", Topic: "topic", Question: "What is revenue?", SQL: "SELECT secret", State: "candidate", Digest: strings.Repeat("a", 64), EvidenceCount: 1}
	service := &Service{topics: reader, sources: retainedSourceReader{}, repo: repo}
	noInspection, err := identity.FromVerified("tenant", "actor", "session", []string{"feedback.write", "cw.topic.read:topic", "cw.source.query:source", "cw.dataset.query:dataset", "cw.execution_context.use:context"}, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.ExampleState(context.Background(), noInspection, ExampleStateRequest{ExampleID: "example-1", State: "active"})
	if err != nil || updated.SQL != "" || updated.State != "active" {
		t.Fatalf("example state leaked protected SQL or failed: %#v %v", updated, err)
	}
	listed, err := service.Examples(context.Background(), noInspection, "topic", 1)
	if err != nil || len(listed) != 1 || listed[0].SQL != "" {
		t.Fatalf("example listing leaked protected SQL: %#v %v", listed, err)
	}
	denied, err := identity.FromVerified("tenant", "actor", "session", []string{"feedback.write", "cw.topic.read:topic"}, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ExampleState(context.Background(), denied, ExampleStateRequest{ExampleID: "example-1", State: "retired"}); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("example state crossed dependency authority: %v", err)
	}
	if repo.examples["example-1"].State != "active" {
		t.Fatal("denied example state mutated the learning row")
	}
	inspect := e
	inspected, err := service.Examples(context.Background(), inspect, "topic", 1)
	if err != nil || len(inspected) != 1 || inspected[0].SQL != "SELECT secret" {
		t.Fatalf("authorized SQL inspection was not preserved: %#v %v", inspected, err)
	}
}

func TestServiceGenerationFailureBranches(t *testing.T) {
	e := unitEnvelope(t)
	admitted, generation, call, budget := testGeneration(t, e)
	validJSON := json.RawMessage(`{"sql":"SELECT id FROM analytics.sales","parameters":[],"assumptions":[],"ambiguities":[]}`)
	for _, tc := range []struct {
		name     string
		response gateway.Generated
		wantErr  error
	}{
		{name: "malformed", response: gateway.Generated{JSON: json.RawMessage(`{"sql":`)}, wantErr: ErrGeneration},
		{name: "invalid-candidate", response: gateway.Generated{JSON: json.RawMessage(`{"sql":" SELECT 1","parameters":[],"assumptions":[],"ambiguities":[]}`)}, wantErr: ErrGeneration},
		{name: "provider-output", response: gateway.Generated{}, wantErr: ErrGeneration},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &Service{engine: &sequenceGateway{responses: []gateway.Generated{tc.response}}}
			_, _, err := service.generate(context.Background(), e, admitted, generation, call, budget, "sqlgen", "")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("generate error=%v want %v", err, tc.wantErr)
			}
		})
	}
	service := &Service{engine: &sequenceGateway{responses: []gateway.Generated{{JSON: validJSON}}}, validator: &unitValidator{errors: []error{exec.ErrUnsafe, nil}}}
	if candidate, _, _, err := service.fixCandidate(context.Background(), e, admitted, call, budget, "SELECT bad", "validation_unsafe"); err != nil || candidate.SQL == "" {
		t.Fatalf("fix candidate failed: %#v %v", candidate, err)
	}
	for _, tc := range []struct {
		name   string
		errors []error
		want   error
	}{
		{name: "forbidden", errors: []error{access.ErrForbidden}, want: access.ErrForbidden},
		{name: "binding", errors: []error{exec.ErrBinding}, want: exec.ErrBinding},
		{name: "fix-generation", errors: []error{exec.ErrUnsafe}, want: ErrValidationBudget},
		{name: "fix-validation", errors: []error{exec.ErrUnsafe, exec.ErrUnsafe}, want: ErrValidationBudget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responses := []gateway.Generated{{JSON: validJSON}, {JSON: validJSON}}
			if tc.name == "fix-generation" {
				responses[1] = gateway.Generated{}
			}
			service := &Service{engine: &sequenceGateway{responses: responses}, validator: &unitValidator{errors: append([]error(nil), tc.errors...)}}
			_, _, _, _, err := service.generateAndValidate(context.Background(), e, admitted, generation, call, budget, "")
			if !errors.Is(err, tc.want) {
				t.Fatalf("generate/validate error=%v want %v", err, tc.want)
			}
		})
	}
}
