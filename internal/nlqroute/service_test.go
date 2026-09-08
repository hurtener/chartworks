package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
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
	contract topics.Contract
	events   *[]string
}

func (t *testTopics) Contract(context.Context, identity.Envelope, string) (topics.Contract, error) {
	*t.events = append(*t.events, "contract")
	return t.contract, nil
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
	return rulesets.Evaluation{Result: semantics.ConstraintEvaluation{Allowed: true}}, nil
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

func TestRouteClarifiesRequiredRuleSlotBeforeGateway(t *testing.T) {
	published := rulesets.Published{State: rulesets.State{Topic: "topic", Version: "rules-v1", Active: true}, Digest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Definition: semantics.RuleSetDefinition{
		SchemaVersion: 1, ID: "rules", Version: "rules-v1", Topic: "topic", TopicVersion: "v1", PackDigest: testPublication().Digest,
		Patterns: []semantics.ClarificationPattern{{ID: "metric-choice", Version: "pattern-v1", Targets: []semantics.Reference{{Kind: semantics.KindDataset, ID: "dataset"}}, Slots: []semantics.ClarificationSlot{{ID: "metric", Prompt: "Choose a metric", Required: true, Kind: semantics.SlotChoice, Sensitivity: semantics.LiteralNonSensitive, Choices: []semantics.ClarificationChoice{{ID: "revenue", Label: "Revenue"}, {ID: "orders", Label: "Orders"}}}}}},
	}}
	service, engine, _ := newTestService(t, testRules{published: published})
	out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageSpanish, Question: "¿Qué ingresos?"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome != nlq.StrategyClarify || out.Clarification == nil || out.Clarification.Reason != "required_slot" || engine.embeds != 0 {
		t.Fatalf("required slot did not stop gateway: %#v embeds=%d", out, engine.embeds)
	}
}

func TestRouteEvaluatesChoiceTargetsAndRejectsUnknownChoices(t *testing.T) {
	choiceRules := rulesets.Published{
		State:  rulesets.State{Topic: "topic", Version: "rules-v1", Active: true},
		Digest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Definition: semantics.RuleSetDefinition{
			SchemaVersion: 1, ID: "rules", Version: "rules-v1", Topic: "topic", TopicVersion: "v1", PackDigest: testPublication().Digest,
			Patterns: []semantics.ClarificationPattern{{
				ID: "metric-choice", Version: "pattern-v1",
				Slots: []semantics.ClarificationSlot{{
					ID: "metric", Prompt: "Choose a metric", Required: true, Kind: semantics.SlotChoice,
					Sensitivity: semantics.LiteralNonSensitive,
					Choices: []semantics.ClarificationChoice{
						{ID: "revenue", Label: "Revenue", Target: &semantics.Reference{Kind: semantics.KindDataset, ID: "dataset"}},
						{ID: "orders", Label: "Orders"},
					},
				}},
			}},
		},
	}

	t.Run("target choice is evaluated", func(t *testing.T) {
		service, engine, _ := newTestService(t, testRules{published: choiceRules})
		out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{
			Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "What is revenue?",
			Choices: []ChoiceSelection{{Pattern: "metric-choice", Slot: "metric", Value: "revenue"}},
		})
		if err != nil || out.Context == nil || engine.embeds != 1 {
			t.Fatalf("target choice did not reach evaluated route: out=%#v err=%v embeds=%d", out, err, engine.embeds)
		}
	})

	t.Run("known choice without target uses default reference", func(t *testing.T) {
		service, engine, _ := newTestService(t, testRules{published: choiceRules})
		out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{
			Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "What is revenue?",
			Choices: []ChoiceSelection{{Pattern: "metric-choice", Slot: "metric", Value: "orders"}},
		})
		if err != nil || out.Context == nil || engine.embeds != 1 {
			t.Fatalf("known choice without target did not use default reference: out=%#v err=%v embeds=%d", out, err, engine.embeds)
		}
	})

	t.Run("unknown choice returns detached clarification", func(t *testing.T) {
		service, engine, _ := newTestService(t, testRules{published: choiceRules})
		out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{
			Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "What is revenue?",
			Choices: []ChoiceSelection{{Pattern: "metric-choice", Slot: "metric", Value: "missing"}},
		})
		if err != nil || out.Outcome != nlq.StrategyClarify || out.Clarification == nil || out.Clarification.Reason != "invalid_choice" || len(out.Clarification.Choices) != 2 || engine.embeds != 0 {
			t.Fatalf("unknown choice was not rejected before gateway: out=%#v err=%v embeds=%d", out, err, engine.embeds)
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
