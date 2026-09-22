package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
)

type testTopics struct {
	contract  topics.Contract
	events    *[]string
	binding   readexec.Binding
	summaries []topics.Summary
}

func (t *testTopics) Contract(context.Context, identity.Envelope, string) (topics.Contract, error) {
	*t.events = append(*t.events, "contract")
	return t.contract, nil
}

func (t *testTopics) List(context.Context, identity.Envelope, topics.ListRequest) ([]topics.Summary, error) {
	return append([]topics.Summary(nil), t.summaries...), nil
}

func (t *testTopics) ClarificationBinding(context.Context, identity.Envelope, string, string) (readexec.Binding, error) {
	if !t.binding.Valid() {
		return readexec.Binding{}, readexec.ErrBinding
	}
	return t.binding.Clone(), nil
}

type testRules struct {
	published rulesets.Published
	err       error
}

func (r testRules) Read(context.Context, identity.Envelope, string, string) (rulesets.Published, error) {
	if r.err != nil {
		return rulesets.Published{}, r.err
	}
	return r.published, nil
}
func (r testRules) Evaluate(context.Context, identity.Envelope, string, rulesets.EvaluateRequest) (rulesets.Evaluation, error) {
	return rulesets.Evaluation{Topic: r.published.Definition.Topic, TopicVersion: r.published.Definition.TopicVersion, PackDigest: r.published.Definition.PackDigest, RuleVersion: r.published.Definition.Version, RuleDigest: r.published.Digest, Result: semantics.ConstraintEvaluation{Allowed: true}}, nil
}

type testIndex struct {
	hit    vindex.Hit
	events *[]string
}

func (i *testIndex) Search(_ context.Context, _ identity.Envelope, queries []vindex.Query) ([]vindex.Result, error) {
	*i.events = append(*i.events, "search")
	out := make([]vindex.Result, len(queries))
	for n, query := range queries {
		out[n] = vindex.Result{ID: query.ID, Publication: vindex.Publication{Version: "v1", Generation: "generation", Revision: 1}, Hits: []vindex.Hit{i.hit}}
	}
	return out, nil
}
func (i *testIndex) Explain(context.Context, identity.Envelope, vindex.Query) (json.RawMessage, error) {
	*i.events = append(*i.events, "explain")
	return json.RawMessage(`[{"Plan":{"Node Type":"Index Scan"}}]`), nil
}

type testEngine struct {
	descriptor gateway.EmbeddingSpace
	events     *[]string
	embeds     int
	reranks    int
}

func (e *testEngine) Generate(context.Context, gateway.Call, *gateway.Budget, string, string, string, *gateway.Schema) (gateway.Generated, error) {
	return gateway.Generated{}, gateway.ErrDisabled
}
func (e *testEngine) Embed(_ context.Context, _ gateway.Call, _ *gateway.Budget, expected string, texts []string) (gateway.Embedded, error) {
	*e.events = append(*e.events, "embed")
	e.embeds++
	if expected != e.descriptor.Key() || len(texts) != 1 {
		return gateway.Embedded{}, gateway.ErrSpace
	}
	return gateway.Embedded{Vectors: [][]float32{{1, 2}}, Space: e.descriptor.Key(), Descriptor: e.descriptor}, nil
}
func (e *testEngine) Rerank(_ context.Context, _ gateway.Call, _ *gateway.Budget, _ string, candidates gateway.Candidates) (gateway.Ranked, error) {
	*e.events = append(*e.events, "rerank")
	e.reranks++
	items := candidates.Items()
	if len(items) == 0 {
		return gateway.Ranked{}, gateway.ErrInput
	}
	out := make([]gateway.RankedItem, len(items))
	for i := range items {
		score := float64(len(items) - i)
		out[i] = gateway.RankedItem{ID: items[i].ID, Score: &score}
	}
	return gateway.Ranked{Items: out}, nil
}
func (e *testEngine) VisualRank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	return gateway.Ranked{}, gateway.ErrDisabled
}
func (e *testEngine) Space() string                          { return e.descriptor.Key() }
func (e *testEngine) EmbeddingSpace() gateway.EmbeddingSpace { return e.descriptor }
func (e *testEngine) Close()                                 {}

func testEnvelope(t *testing.T, sourceReach bool) identity.Envelope {
	t.Helper()
	scopes := []string{"topics.read", "cw.topic.read:topic"}
	if sourceReach {
		scopes = append(scopes, "cw.source.read:source", "cw.dataset.query:dataset", "cw.execution_context.use:ctx")
	}
	e, err := identity.FromVerified("tenant", "user", "session", scopes, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func testPublication() topics.Published {
	return topics.Published{State: topics.State{Topic: "topic", Version: "v1", Revision: 1, Active: true}, Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Definition: topics.Definition{
		SchemaVersion: 1, Topic: "topic", Version: "v1", Datasets: []topics.Dataset{{ID: "dataset", Source: topics.Binding{Source: "source", Context: "ctx", Dataset: "dataset", SourceRevision: 1}}},
	}}
}

func newTestService(t *testing.T, rules RuleReader) (*Service, *testEngine, *[]string) {
	t.Helper()
	events := []string{}
	descriptor := gateway.EmbeddingSpace{Provider: "fixture", Route: "embedding", Endpoint: "default", Model: "embedding-model", Revision: "generation-1", Dimensions: 2, Preprocessing: "raw", InputType: "text", Normalization: "l2"}
	engine := &testEngine{descriptor: descriptor, events: &events}
	publication := testPublication()
	reader := &testTopics{contract: topics.Contract{Publication: publication}, events: &events}
	index := &testIndex{events: &events, hit: vindex.Hit{ID: "facet", Kind: "measure", SourceID: "source", Text: "revenue measure", Generation: "generation", Version: "v1", SourceGeneration: "source-generation", Distance: 0.2}}
	service, err := New(reader, rules, index, engine)
	if err != nil {
		t.Fatal(err)
	}
	return service, engine, &events
}

func TestRouteAdmitsCurrentTopicBeforeEmbeddingAndAssemblesContext(t *testing.T) {
	service, engine, events := newTestService(t, testRules{err: store.ErrNotFound})
	out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "What is revenue?", Rerank: false})
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome != nlq.StrategySingleTopic || out.Context == nil || len(out.Evidence) != 1 || engine.embeds != 1 {
		t.Fatalf("unexpected route result: %#v", out)
	}
	if !reflect.DeepEqual(*events, []string{"contract", "explain", "embed", "search"}) {
		t.Fatalf("work did not preserve admission order: %#v", *events)
	}
}

func TestRouteResultRequestIsDetachedFromCaller(t *testing.T) {
	service, _, _ := newTestService(t, testRules{err: store.ErrNotFound})
	confidence := 0.75
	request := RouteRequest{
		Topic: "topic", Topics: []string{"topic"}, Context: "ctx", Locale: nlq.LanguageEnglish, Question: "What is revenue?",
		Kinds: []string{"measure"}, Examples: []nlq.OptionalItem{{ID: "example", Text: "Use the reviewed monthly example", Confidence: &confidence}},
	}
	result, err := service.Route(context.Background(), testEnvelope(t, true), request)
	if err != nil {
		t.Fatal(err)
	}
	request.Topics[0] = "mutated-topic"
	request.Kinds[0] = "dimension"
	request.Examples[0].Text = "mutated example"
	*request.Examples[0].Confidence = 0.1
	if result.Request.Topics[0] != "topic" || result.Request.Kinds[0] != "measure" || result.Request.Examples[0].Text != "Use the reviewed monthly example" || result.Request.Examples[0].Confidence == nil || *result.Request.Examples[0].Confidence != 0.75 {
		t.Fatalf("route result retained caller-owned request memory: %#v", result.Request)
	}

	allConfidence := 0.9
	all := RouteRequest{
		Topics:      []string{"topic", "topic-two"},
		Kinds:       []string{"measure"},
		References:  []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}},
		Choices:     []ChoiceSelection{{Pattern: "pattern", Slot: "metric", Value: "revenue"}},
		JoinChoices: []JoinChoice{{Topic: "topic", JoinID: "sales-items"}},
		MetricIDs:   []string{"revenue"},
		Examples:    []nlq.OptionalItem{{ID: "example", Text: "reviewed example", Confidence: &allConfidence}},
	}
	cloned := cloneRouteRequest(all)
	all.Topics[0] = "changed"
	all.Kinds[0] = "dimension"
	all.References[0].ID = "margin"
	all.Choices[0].Value = "margin"
	all.JoinChoices[0].JoinID = "other-join"
	all.MetricIDs[0] = "margin"
	all.Examples[0].Text = "changed"
	*all.Examples[0].Confidence = 0.2
	if cloned.Topics[0] != "topic" || cloned.Kinds[0] != "measure" || cloned.References[0].ID != "revenue" || cloned.Choices[0].Value != "revenue" || cloned.JoinChoices[0].JoinID != "sales-items" || cloned.MetricIDs[0] != "revenue" || cloned.Examples[0].Text != "reviewed example" || cloned.Examples[0].Confidence == nil || *cloned.Examples[0].Confidence != 0.9 {
		t.Fatalf("request clone retained mutable aliases: %#v", cloned)
	}
}

func TestRouteDeniesBeforeGatewayWhenDependencyReachIsMissing(t *testing.T) {
	service, engine, _ := newTestService(t, testRules{err: store.ErrNotFound})
	_, err := service.Route(context.Background(), testEnvelope(t, false), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "What is revenue?"})
	if !errors.Is(err, access.ErrNotFound) || engine.embeds != 0 {
		t.Fatalf("missing dependency reach was not denied before embed: err=%v embeds=%d", err, engine.embeds)
	}
}

func TestRouteRejectsUnknownPinnedMetricBeforeGateway(t *testing.T) {
	service, engine, _ := newTestService(t, testRules{err: store.ErrNotFound})
	_, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{
		Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "What is revenue?", MetricIDs: []string{"missing"},
	})
	if !errors.Is(err, ErrInvalid) || engine.embeds != 0 {
		t.Fatalf("unknown metric was not rejected before gateway: err=%v embeds=%d", err, engine.embeds)
	}
}

func TestPinnedKPIResolvesTransitiveRichDependencyClosure(t *testing.T) {
	column := func(dataset, id string) semantics.Reference {
		return semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset, ID: id}
	}
	def := topics.Definition{
		Datasets: []topics.Dataset{
			{ID: "orders", Columns: []semantics.Column{{ID: "amount", Name: "Amount", Aliases: []string{"Importe"}}, {ID: "customer_id", Name: "Customer"}}},
			{ID: "customers", Columns: []semantics.Column{{ID: "id", Name: "Customer"}}},
		},
		Measures:   []semantics.Measure{{ID: "revenue", Name: "Revenue", Field: column("orders", "amount"), Aggregation: semantics.AggregationSum, Unit: "currency", Filters: []semantics.SemanticFilter{{ID: "known_customer", Field: column("customers", "id"), Operator: "not_null"}}}},
		Dimensions: []semantics.Dimension{{ID: "customer", Name: "Customer month", Field: column("customers", "id"), Role: semantics.DimensionTemporal, Aliases: []string{"Buyer month", "Mes del cliente"}, Values: []semantics.GovernedValue{{ID: "active", Value: "A", Aliases: []string{"Active", "Activo"}, Sensitivity: semantics.LiteralNonSensitive, Provenance: semantics.ValueProvenance{Kind: "reviewed_profile", Evidence: "profile_v1", Policy: "low_cardinality"}}}, Temporal: &semantics.TemporalPolicy{Grains: []semantics.TimeGrain{semantics.GrainMonth}, Calendar: "gregorian"}}},
		KPIs: []semantics.KPI{
			{ID: "net_revenue", Name: "Net revenue", Expression: "revenue", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}},
			{ID: "indexed_revenue", Name: "Indexed revenue", Expression: "net revenue divided by target", Inputs: []semantics.Reference{{Kind: semantics.KindKPI, ID: "net_revenue"}}},
		},
		Joins: []semantics.Join{{ID: "orders_customers", Name: "Orders customers", Left: column("orders", "customer_id"), Right: column("customers", "id"), Type: semantics.JoinInner, Cardinality: semantics.CardinalityManyToOne}},
	}
	metric, ok, err := findMetric(def, "indexed_revenue")
	if err != nil || !ok || metric.Text != "Indexed revenue = net revenue divided by target" {
		t.Fatalf("metric not resolved: %#v", metric)
	}
	metric.ID = "topic:indexed_revenue"
	kinds := map[string]bool{}
	for _, dependency := range metric.Dependencies {
		kinds[dependency.Kind+":"+dependency.ID] = true
	}
	for _, want := range []string{"kpi:indexed_revenue", "kpi:net_revenue", "measure:revenue", "column:orders:amount", "column:customers:id", "dimension:customer", "join:orders_customers"} {
		if !kinds[want] {
			t.Fatalf("missing transitive dependency %q: %#v", want, metric.Dependencies)
		}
	}
	assembler, err := nlq.NewDefaultContextAssembler()
	if err != nil {
		t.Fatal(err)
	}
	assembled, err := assembler.Assemble(context.Background(), nlq.ContextInput{Locale: nlq.LanguageSpanish, Strategy: nlq.StrategySingleTopic, Topic: "topic", TopicVersion: "v1", Question: "Ingresos por mes para clientes activos", Metrics: []nlq.PinnedMetric{metric}}, nlq.TierHigh)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"net revenue divided by target", "Mes del cliente", `"month"`, "Activo", "known_customer", "orders_customers"} {
		if !strings.Contains(assembled.Prompt, want) {
			t.Fatalf("rich dependency %q absent from generation prompt: %s", want, assembled.Prompt)
		}
	}
}

func TestMetricClosureRequiresUniqueConfirmedConnectingSubgraph(t *testing.T) {
	column := func(dataset, id string) semantics.Reference {
		return semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset, ID: id}
	}
	join := func(id, left, right string) semantics.Join {
		return semantics.Join{ID: id, Name: id, Left: column(left, "id"), Right: column(right, "id"), Type: semantics.JoinInner, Cardinality: semantics.CardinalityManyToOne}
	}
	base := topics.Definition{
		Datasets: []topics.Dataset{{ID: "a", Columns: []semantics.Column{{ID: "id"}}}, {ID: "b", Columns: []semantics.Column{{ID: "id"}}}, {ID: "c", Columns: []semantics.Column{{ID: "id"}}}, {ID: "d", Columns: []semantics.Column{{ID: "id"}}}},
		Measures: []semantics.Measure{{ID: "a_value", Name: "A", Field: column("a", "id"), Aggregation: semantics.AggregationSum, Filters: []semantics.SemanticFilter{{ID: "c_required", Field: column("c", "id"), Operator: "not_null"}}}},
		Joins:    []semantics.Join{join("ab", "a", "b"), join("bc", "b", "c")},
	}
	metric, ok, err := findMetric(base, "a_value")
	if err != nil || !ok {
		t.Fatal("three-dataset bridge", err)
	}
	var joinIDs []string
	for _, dependency := range metric.Dependencies {
		if dependency.Kind == "join" {
			joinIDs = append(joinIDs, dependency.ID)
		}
	}
	if !slices.Equal(joinIDs, []string{"ab", "bc"}) {
		t.Fatalf("bridge joins missing or unstable: %v", joinIDs)
	}
	foundBridgeColumn := false
	for _, dependency := range metric.Dependencies {
		foundBridgeColumn = foundBridgeColumn || dependency.Kind == "column" && dependency.ID == "b:id"
	}
	if !foundBridgeColumn {
		t.Fatalf("bridge join key metadata missing: %#v", metric.Dependencies)
	}
	budgeted := metric
	budgeted.ID = "topic:a_value"
	budgeted.Dependencies = append([]nlq.MetricDependency(nil), metric.Dependencies...)
	budgeted.Dependencies[0].Text = strings.Repeat("x", 10<<10)
	assembler, err := nlq.NewDefaultContextAssembler()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = assembler.Assemble(context.Background(), nlq.ContextInput{Locale: nlq.LanguageEnglish, Strategy: nlq.StrategySingleTopic, Topic: "topic", TopicVersion: "v1", Question: "A with C", Metrics: []nlq.PinnedMetric{budgeted}}, nlq.TierLow); !errors.Is(err, nlq.ErrInsufficient) {
		t.Fatalf("oversized bridge closure was partially admitted: %v", err)
	}
	shuffled := base
	shuffled.Joins = []semantics.Join{base.Joins[1], base.Joins[0]}
	stable, _, err := findMetric(shuffled, "a_value")
	if err != nil || !reflect.DeepEqual(metric.Dependencies, stable.Dependencies) {
		t.Fatalf("join input order changed closure: %v %#v", err, stable.Dependencies)
	}

	disconnected := base
	disconnected.Joins = disconnected.Joins[:1]
	if _, _, err = findMetric(disconnected, "a_value"); !errors.Is(err, ErrMetricContext) {
		t.Fatalf("disconnected closure accepted: %v", err)
	}
	competing := base
	competing.Joins = append(append([]semantics.Join(nil), base.Joins...), join("ad", "a", "d"), join("dc", "d", "c"))
	if _, _, err = findMetric(competing, "a_value"); !errors.Is(err, ErrMetricContext) {
		t.Fatalf("competing paths accepted: %v", err)
	}
	cycle := base
	cycle.Joins = append(append([]semantics.Join(nil), base.Joins...), join("ca", "c", "a"))
	if _, _, err = findMetric(cycle, "a_value"); !errors.Is(err, ErrMetricContext) {
		t.Fatalf("cycle selected arbitrarily: %v", err)
	}
}

func TestRouteRejectsUnevaluatedConstraintInputsBeforeGateway(t *testing.T) {
	service, engine, _ := newTestService(t, testRules{err: store.ErrNotFound})
	_, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{
		Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "What is revenue?",
		References: []semantics.Reference{{Kind: semantics.KindDataset, ID: "dataset"}},
	})
	if !errors.Is(err, ErrInvalid) || engine.embeds != 0 {
		t.Fatalf("unevaluated reference was not rejected before gateway: err=%v embeds=%d", err, engine.embeds)
	}
}

func compiledRouteChoiceRules(t *testing.T, conditional bool) rulesets.Published {
	t.Helper()
	target := semantics.Reference{Kind: semantics.KindDataset, ID: "dataset"}
	definition := semantics.RuleSetDefinition{
		SchemaVersion: 1, ID: "rules", Version: "rules-v1", Topic: "topic", TopicVersion: "v1", PackDigest: testPublication().Digest,
		Patterns: []semantics.ClarificationPattern{{ID: "metric-choice", Version: "pattern-v1", Targets: []semantics.Reference{target}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "synthetic-reviewed-choice"}, Slots: []semantics.ClarificationSlot{{ID: "metric", Prompt: "Choose a reviewed dataset", Required: true, Kind: semantics.SlotChoice, Sensitivity: semantics.LiteralNonSensitive, Choices: []semantics.ClarificationChoice{{ID: "revenue", Label: "Revenue", Target: &target}, {ID: "orders", Label: "Orders"}}}}}},
	}
	if conditional {
		definition.Patterns[0].Policy = &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyTerms: []string{"revenue", "ingresos"}}, Why: "Selects the reviewed dataset for this question."}
		definition.Patterns[0].Slots[0].Choices[1].Target = &target
	}
	subject, err := semantics.NewRuleSubject(semantics.TopicPack{SchemaVersion: 1, Topic: "topic", Version: "v1", Datasets: []semantics.Dataset{{ID: "dataset"}}}, testPublication().Digest)
	if err != nil {
		t.Fatal(err)
	}
	model, err := semantics.CompilePublishedRules(subject, definition)
	if err != nil {
		t.Fatal(err)
	}
	return rulesets.Published{State: rulesets.State{Topic: "topic", Version: "rules-v1", Revision: 1, Active: true}, Digest: model.Digest(), Definition: model.Definition()}
}

func TestRouteClarifiesRequiredRuleSlotBeforeGateway(t *testing.T) {
	published := compiledRouteChoiceRules(t, true)
	service, engine, _ := newTestService(t, testRules{published: published})
	out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageSpanish, Question: "¿Qué ingresos?"})
	if err != nil || out.Outcome != nlq.StrategyClarify || out.Clarification == nil || out.Clarification.Reason != "required_answers" || engine.embeds != 0 {
		t.Fatalf("reviewed required choice did not block: %v", err)
	}
	if len(out.Clarification.Questions) != 1 || out.Clarification.Questions[0].Why == "" {
		t.Fatal("reviewed explanation missing")
	}
	out, err = service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "List the complete dataset"})
	if err != nil || out.Context == nil || out.Clarification != nil || engine.embeds != 1 {
		t.Fatal("unrelated question was interrupted", err)
	}
}

func TestRouteEvaluatesChoiceTargetsAndRejectsUnknownChoices(t *testing.T) {
	t.Run("exact reviewed target reaches sealed context", func(t *testing.T) {
		service, engine, _ := newTestService(t, testRules{published: compiledRouteChoiceRules(t, true)})
		out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "What is revenue?", Choices: []ChoiceSelection{{Pattern: "metric-choice", Slot: "metric", Value: "revenue"}}})
		if err != nil || out.Context == nil || engine.embeds != 1 || len(out.Resolutions) != 1 || out.Resolutions[0].Reference == nil || out.Resolutions[0].Reference.ID != "dataset" {
			t.Fatal("exact choice lost its reviewed effect", err)
		}
	})
	t.Run("legacy effectless choice cannot silently default", func(t *testing.T) {
		service, engine, _ := newTestService(t, testRules{published: compiledRouteChoiceRules(t, false)})
		out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "What is revenue?", Choices: []ChoiceSelection{{Pattern: "metric-choice", Slot: "metric", Value: "orders"}}})
		if err == nil && (out.Clarification == nil || out.Context != nil) {
			t.Fatal("effectless legacy choice was accepted")
		}
		if engine.embeds != 0 {
			t.Fatal("effectless choice reached provider")
		}
	})
	t.Run("unknown choice fails atomically with detached repair", func(t *testing.T) {
		published := compiledRouteChoiceRules(t, true)
		service, engine, _ := newTestService(t, testRules{published: published})
		_, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "What is revenue?", Choices: []ChoiceSelection{{Pattern: "metric-choice", Slot: "metric", Value: "missing"}}})
		var failure *Clarification
		if !errors.As(err, &failure) || failure.Outcome != semantics.ClarificationInvalid || len(failure.Errors) == 0 || engine.embeds != 0 {
			t.Fatal("foreign choice was not rejected before provider", err)
		}
		if len(failure.Questions) > 0 && len(failure.Questions[0].Choices) > 0 {
			failure.Questions[0].Choices[0].Label = "mutated"
			if published.Definition.Patterns[0].Slots[0].Choices[0].Label == "mutated" {
				t.Fatal("repair aliases publication")
			}
		}
	})
	t.Run("legacy patterns do not introduce new blockers", func(t *testing.T) {
		service, engine, _ := newTestService(t, testRules{published: compiledRouteChoiceRules(t, false)})
		out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "What is revenue?"})
		if err != nil || out.Context == nil || out.Clarification != nil || engine.embeds != 1 {
			t.Fatal("legacy pattern accidentally became required", err)
		}
	})
}

func TestConfirmJoinsRequiresPublishedSameSourceOneToOne(t *testing.T) {
	definition := topics.Definition{
		Datasets: []topics.Dataset{
			{ID: "left", Source: topics.Binding{Source: "source", Context: "ctx", Dataset: "left"}},
			{ID: "right", Source: topics.Binding{Source: "source", Context: "ctx", Dataset: "right"}},
		},
		Joins: []semantics.Join{{ID: "join", Left: semantics.Reference{Kind: semantics.KindColumn, Dataset: "left", ID: "id"}, Right: semantics.Reference{Kind: semantics.KindColumn, Dataset: "right", ID: "id"}, Type: semantics.JoinInner, Cardinality: semantics.CardinalityOneToOne}},
	}
	admitted := []admittedTopic{{id: "one", publication: topics.Published{Definition: definition}}, {id: "two", publication: topics.Published{Definition: definition}}}
	choices := []JoinChoice{{Topic: "one", JoinID: "join"}, {Topic: "two", JoinID: "join"}}
	if got := confirmJoins(admitted, choices); got != nil {
		t.Fatalf("confirmed join rejected: %#v", got)
	}
	reversed := definition
	reversed.Joins = append([]semantics.Join(nil), definition.Joins...)
	reversed.Joins[0].Left, reversed.Joins[0].Right = reversed.Joins[0].Right, reversed.Joins[0].Left
	admitted[1].publication.Definition = reversed
	if got := confirmJoins(admitted, choices); got != nil {
		t.Fatalf("symmetric inner relationship rejected: %#v", got)
	}
	reversed.Joins[0].Type = semantics.JoinLeft
	definition.Joins[0].Type = semantics.JoinLeft
	admitted[0].publication.Definition = definition
	admitted[1].publication.Definition = reversed
	if got := confirmJoins(admitted, choices); got == nil || got.Reason != "unconfirmed_relationship" {
		t.Fatalf("directionally different left joins were admitted: %#v", got)
	}
	definition.Joins[0].Type = semantics.JoinInner
	admitted[0].publication.Definition = definition
	definition.Joins[0].Cardinality = semantics.CardinalityManyToOne
	admitted[0].publication.Definition = definition
	if got := confirmJoins(admitted, choices); got == nil || got.Reason != "ambiguous_cardinality" {
		t.Fatalf("ambiguous cardinality was admitted: %#v", got)
	}
}

func TestRouteRejectsUnqualifiedAmbiguousRuleSlot(t *testing.T) {
	published := rulesets.Published{State: rulesets.State{Topic: "topic", Version: "rules-v1", Active: true}, Digest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Definition: semantics.RuleSetDefinition{
		SchemaVersion: 1, ID: "rules", Version: "rules-v1", Topic: "topic", TopicVersion: "v1", PackDigest: testPublication().Digest,
		Patterns: []semantics.ClarificationPattern{
			{ID: "first", Version: "pattern-v1", Slots: []semantics.ClarificationSlot{{ID: "metric", Prompt: "Choose a first metric", Required: true}}},
			{ID: "second", Version: "pattern-v1", Slots: []semantics.ClarificationSlot{{ID: "metric", Prompt: "Choose a second metric", Required: true}}},
		},
	}}
	service, _, _ := newTestService(t, testRules{published: published})
	_, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{
		Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "What is revenue?", Choices: []ChoiceSelection{{Slot: "metric", Value: "revenue"}},
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("ambiguous unqualified slot was accepted: %v", err)
	}
}

func TestReferenceIdentityPreservesRevision(t *testing.T) {
	first := semantics.Reference{Kind: semantics.KindCanonicalEntity, ID: "customer", Revision: 1}
	second := first
	second.Revision = 2
	if referenceID(first) == referenceID(second) || referenceText(first) == referenceText(second) {
		t.Fatalf("canonical revision was dropped: %q/%q", referenceID(first), referenceID(second))
	}
}
