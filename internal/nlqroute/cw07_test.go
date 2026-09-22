package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
)

func cw07Binding(revision int64) readexec.Binding {
	return readexec.Binding{Tenant: "tenant", Source: "source", Context: "ctx", Revision: revision, Dialect: "postgres", Contract: "contract", Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Relations: []readexec.Relation{{ID: "dataset", Schema: "analytics", Name: "sales", Columns: []readexec.Column{{Name: "region_code", NativeType: "text", Category: "text", Safe: true}, {Name: "occurred_on", NativeType: "date", Category: "date", Safe: true}}}}}
}

func cw07Publication(topic string) topics.Published {
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if topic != "topic" {
		digest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	}
	return topics.Published{State: topics.State{Topic: topic, Version: "v1", Revision: 1, Active: true}, Digest: digest, Definition: topics.Definition{SchemaVersion: 1, Topic: topic, Version: "v1", Datasets: []topics.Dataset{{ID: "dataset", Source: topics.Binding{Source: "source", Context: "ctx", Dataset: "dataset", SourceRevision: 1}, Columns: []semantics.Column{{ID: "region", SourceName: "region_code", Name: "Region", NativeType: "text", Category: "text", Sensitivity: semantics.LiteralNonSensitive}, {ID: "occurred", SourceName: "occurred_on", Name: "Occurred", NativeType: "date", Category: "date", Sensitivity: semantics.LiteralNonSensitive, SemanticRole: semantics.SemanticRoleEventTime}}}}, Dimensions: []semantics.Dimension{{ID: "sales_region", Name: "Sales region", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "dataset", ID: "region"}, Role: semantics.DimensionCategorical, Geography: true, Aliases: []string{"geography", "región"}, Values: []semantics.GovernedValue{{ID: "north", Value: "NORTH", Aliases: []string{"north", "norte"}, Sensitivity: semantics.LiteralNonSensitive, Provenance: semantics.ValueProvenance{Kind: "review", Evidence: "ev1", Policy: "p1"}}, {ID: "south", Value: "SOUTH", Aliases: []string{"south", "sur"}, Sensitivity: semantics.LiteralNonSensitive, Provenance: semantics.ValueProvenance{Kind: "review", Evidence: "ev2", Policy: "p1"}}}}, {ID: "event_date", Name: "Event date", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "dataset", ID: "occurred"}, Role: semantics.DimensionTemporal, Aliases: []string{"date", "fecha"}, Temporal: &semantics.TemporalPolicy{Grains: []semantics.TimeGrain{semantics.GrainMonth}, Calendar: "gregorian", Timezone: "UTC"}}}}}
}

func cw07Service(t *testing.T, publication topics.Published, binding readexec.Binding) (*Service, *testEngine) {
	t.Helper()
	events := []string{}
	descriptor := gateway.EmbeddingSpace{Provider: "fixture", Route: "embedding", Endpoint: "default", Model: "embedding-model", Revision: "generation-1", Dimensions: 2, Preprocessing: "raw", InputType: "text", Normalization: "l2"}
	engine := &testEngine{descriptor: descriptor, events: &events}
	reader := &testTopics{contract: topics.Contract{Publication: publication}, events: &events, binding: binding}
	index := &testIndex{events: &events, hit: vindex.Hit{ID: "facet", Kind: "measure", SourceID: "source", Text: "revenue measure", Generation: "generation", Version: "v1", SourceGeneration: "source-generation", Distance: 0.2}}
	service, err := New(reader, testRules{err: store.ErrNotFound}, index, engine)
	if err != nil {
		t.Fatal(err)
	}
	return service, engine
}

func testCW07InterpretsGovernedGeographyAndSpanishMonthBeforeProvider(t *testing.T) {
	service, engine := cw07Service(t, cw07Publication("topic"), cw07Binding(1))
	out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageSpanish, Question: "Ingresos sin norte en marzo de 2025", InterpretationAnchor: "2026-09-22"})
	if err != nil {
		t.Fatal(err)
	}
	if engine.embeds != 1 || out.Interpretation == nil || len(out.Interpretation.Values) != 1 || len(out.Interpretation.Temporal) != 1 {
		t.Fatalf("interpretation missing: %#v", out.Interpretation)
	}
	if value := out.Interpretation.Values[0]; value.CanonicalValue != "NORTH" || value.Operator != "ne" || !value.Geography {
		t.Fatalf("governed geography changed: %#v", value)
	}
	if span := out.Interpretation.Temporal[0]; span.Start != "2025-03-01" || span.End != "2025-04-01" || span.Grain != "month" {
		t.Fatalf("month changed: %#v", span)
	}
	constraints, err := out.ResolvedBusinessConstraints()
	if err != nil || len(constraints) != 2 {
		t.Fatalf("sealed constraints unavailable: %v %#v", err, constraints)
	}
	if constraints[0].Value != "NORTH" || constraints[1].Value != "2025-03-01" {
		t.Fatalf("typed constraints changed: %#v", constraints)
	}
	publication := cw07Publication("topic")
	publication.Definition.Dimensions[0].Geography = false
	service, _ = cw07Service(t, publication, cw07Binding(1))
	neutral, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in north", InterpretationAnchor: "2026-09-22"})
	if err != nil || neutral.Interpretation == nil || neutral.Interpretation.Values[0].Geography {
		t.Fatalf("label inferred geography without reviewed designation: err=%v out=%#v", err, neutral.Interpretation)
	}
}

func testCW07InterpretationCorrectionRemovalAndSourceFence(t *testing.T) {
	service, _ := cw07Service(t, cw07Publication("topic"), cw07Binding(1))
	out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in north", InterpretationAnchor: "2026-09-22", InterpretationEdits: []InterpretationEdit{{Target: "topic:sales_region:north", Action: "remove"}}})
	if err != nil || out.Interpretation == nil || len(out.Interpretation.Values) != 0 {
		t.Fatalf("removal not applied: %v %#v", err, out.Interpretation)
	}
	replaced, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in north", InterpretationAnchor: "2026-09-22", InterpretationEdits: []InterpretationEdit{{Target: "topic:sales_region:north", Action: "replace", Value: "south"}}})
	if err != nil || replaced.Interpretation == nil || len(replaced.Interpretation.Values) != 1 || replaced.Interpretation.Values[0].CanonicalValue != "SOUTH" {
		t.Fatalf("reviewed replacement not applied: %v %#v", err, replaced.Interpretation)
	}
	stale, engine := cw07Service(t, cw07Publication("topic"), cw07Binding(2))
	_, err = stale.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in north", InterpretationAnchor: "2026-09-22"})
	if !errors.Is(err, readexec.ErrBinding) || engine.embeds != 0 {
		t.Fatalf("stale source reached provider: %v embeds=%d", err, engine.embeds)
	}
}

func testCW07AmbiguousGovernedValueClarifiesBeforeProvider(t *testing.T) {
	publication := cw07Publication("topic")
	second := publication.Definition.Dimensions[0]
	second.ID, second.Name = "shipping_region", "Shipping region"
	publication.Definition.Dimensions = append(publication.Definition.Dimensions, second)
	service, engine := cw07Service(t, publication, cw07Binding(1))
	out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in north", InterpretationAnchor: "2026-09-22"})
	if err != nil || out.Outcome != nlq.StrategyClarify || out.Clarification == nil || out.Clarification.Reason != "ambiguous_governed_value" || engine.embeds != 0 {
		t.Fatalf("ambiguity widened: err=%v out=%#v embeds=%d", err, out, engine.embeds)
	}
	service, engine = cw07Service(t, cw07Publication("topic"), cw07Binding(1))
	out, err = service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in March and April", InterpretationAnchor: "2026-09-22"})
	if err != nil || out.Outcome != nlq.StrategyClarify || out.Clarification == nil || out.Clarification.Reason != "ambiguous_temporal_span" || engine.embeds != 0 {
		t.Fatalf("ambiguous time silently selected: err=%v out=%#v embeds=%d", err, out, engine.embeds)
	}
	unsupported := cw07Publication("topic")
	unsupported.Definition.Dimensions[1].Temporal.Grains = []semantics.TimeGrain{semantics.GrainYear}
	service, engine = cw07Service(t, unsupported, cw07Binding(1))
	out, err = service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in March", InterpretationAnchor: "2026-09-22"})
	if err != nil || out.Outcome != nlq.StrategyClarify || out.Clarification == nil || out.Clarification.Reason != "unsupported_temporal_grain" || engine.embeds != 0 {
		t.Fatalf("unsupported grain reached provider: err=%v out=%#v embeds=%d", err, out, engine.embeds)
	}
	service, engine = cw07Service(t, cw07Publication("topic"), cw07Binding(1))
	out, err = service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageSpanish, Question: "Ingresos en north norte", InterpretationAnchor: "2026-09-22"})
	if err != nil || out.Interpretation == nil || len(out.Interpretation.Values) != 1 || engine.embeds != 1 {
		t.Fatalf("aliases duplicated one governed target: err=%v out=%#v embeds=%d", err, out, engine.embeds)
	}
	service, engine = cw07Service(t, cw07Publication("topic"), cw07Binding(1))
	out, err = service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageSpanish, Question: "Ingresos en north sin norte", InterpretationAnchor: "2026-09-22"})
	if err != nil || out.Outcome != nlq.StrategyClarify || out.Clarification == nil || out.Clarification.Reason != "conflicting_governed_value" || engine.embeds != 0 {
		t.Fatalf("conflicting aliases silently selected: err=%v out=%#v embeds=%d", err, out, engine.embeds)
	}
}

type cw07Catalog struct {
	publications map[string]topics.Published
	events       *[]string
	binding      readexec.Binding
	tenant       string
}

func (c *cw07Catalog) List(_ context.Context, e identity.Envelope, _ topics.ListRequest) ([]topics.Summary, error) {
	if c.tenant != "" && e.Tenant() != c.tenant {
		return nil, access.ErrNotFound
	}
	out := make([]topics.Summary, 0, len(c.publications))
	for _, p := range c.publications {
		out = append(out, topics.Summary{Topic: p.State.Topic, Version: p.State.Version, Revision: p.State.Revision, Digest: p.Digest})
	}
	return out, nil
}
func (c *cw07Catalog) Contract(_ context.Context, _ identity.Envelope, id string) (topics.Contract, error) {
	p, ok := c.publications[id]
	if !ok {
		return topics.Contract{}, store.ErrNotFound
	}
	*c.events = append(*c.events, "contract")
	return topics.Contract{Publication: p}, nil
}
func (c *cw07Catalog) ClarificationBinding(context.Context, identity.Envelope, string, string) (readexec.Binding, error) {
	return c.binding.Clone(), nil
}

type cw07Index struct {
	distances map[string]float64
	events    *[]string
	omitLast  bool
}

func (i *cw07Index) Explain(context.Context, identity.Envelope, vindex.Query) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}
func (i *cw07Index) Search(_ context.Context, _ identity.Envelope, queries []vindex.Query) ([]vindex.Result, error) {
	if i.omitLast && len(queries) > 0 {
		queries = queries[:len(queries)-1]
	}
	out := make([]vindex.Result, len(queries))
	for n, q := range queries {
		distance := i.distances[q.Topic]
		out[n] = vindex.Result{ID: q.ID, Publication: vindex.Publication{Version: "v1", Generation: "generation", Revision: 1}, Hits: []vindex.Hit{{ID: "topic-facet", Kind: "topic", SourceID: "source", Text: q.Topic + " topic", Generation: "generation", Version: "v1", SourceGeneration: "source-generation", Distance: distance}, {ID: "measure-facet", Kind: "measure", SourceID: "source", Text: q.Topic + " measure", Generation: "generation", Version: "v1", SourceGeneration: "source-generation", Distance: distance + 0.02}}}
	}
	return out, nil
}

func cw07DiscoveryEnvelope(t *testing.T) identity.Envelope {
	t.Helper()
	e, err := identity.FromVerified("tenant", "user", "session", []string{"topics.read", "cw.topic.read:*", "cw.source.read:*", "cw.dataset.query:*", "cw.execution_context.use:*"}, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func cw07DiscoveryService(t *testing.T, reader *cw07Catalog, distances map[string]float64) (*Service, *testEngine) {
	t.Helper()
	descriptor := gateway.EmbeddingSpace{Provider: "fixture", Route: "embedding", Endpoint: "default", Model: "embedding-model", Revision: "generation-1", Dimensions: 2, Preprocessing: "raw", InputType: "text", Normalization: "l2"}
	engine := &testEngine{descriptor: descriptor, events: reader.events}
	service, err := New(reader, testRules{err: store.ErrNotFound}, &cw07Index{distances: distances, events: reader.events}, engine)
	if err != nil {
		t.Fatal(err)
	}
	return service, engine
}

func testCW07ServerSelectsCurrentAuthorizedTopicWithEvidencePolicy(t *testing.T) {
	events := []string{}
	descriptor := gateway.EmbeddingSpace{Provider: "fixture", Route: "embedding", Endpoint: "default", Model: "embedding-model", Revision: "generation-1", Dimensions: 2, Preprocessing: "raw", InputType: "text", Normalization: "l2"}
	engine := &testEngine{descriptor: descriptor, events: &events}
	reader := &cw07Catalog{publications: map[string]topics.Published{"alpha": cw07Publication("alpha"), "beta": cw07Publication("beta")}, events: &events, binding: cw07Binding(1)}
	service, err := New(reader, testRules{err: store.ErrNotFound}, &cw07Index{distances: map[string]float64{"alpha": 0.1, "beta": 0.8}, events: &events}, engine)
	if err != nil {
		t.Fatal(err)
	}
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue by region", InterpretationAnchor: "2026-09-22", Rerank: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Topic != "alpha" || out.Decision == nil || out.Decision.SelectedTopic != "alpha" || out.Decision.Policy != routingPolicyVersion || len(out.Decision.Evidence) != 2 {
		t.Fatalf("server topic decision changed: %#v", out.Decision)
	}
	if engine.embeds != 2 || engine.reranks != 2 || out.Decision.Evidence[0].RerankPosition == nil {
		t.Fatalf("expected bounded choice and context calls, embeds=%d reranks=%d", engine.embeds, engine.reranks)
	}
}

func testCW07TopicDecisionAmbiguityDoesNotChooseByOrder(t *testing.T) {
	events := []string{}
	descriptor := gateway.EmbeddingSpace{Provider: "fixture", Route: "embedding", Endpoint: "default", Model: "embedding-model", Revision: "generation-1", Dimensions: 2, Preprocessing: "raw", InputType: "text", Normalization: "l2"}
	engine := &testEngine{descriptor: descriptor, events: &events}
	reader := &cw07Catalog{publications: map[string]topics.Published{"alpha": cw07Publication("alpha"), "beta": cw07Publication("beta")}, events: &events, binding: cw07Binding(1)}
	service, err := New(reader, testRules{err: store.ErrNotFound}, &cw07Index{distances: map[string]float64{"alpha": 0.2, "beta": 0.21}, events: &events}, engine)
	if err != nil {
		t.Fatal(err)
	}
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue", InterpretationAnchor: "2026-09-22"})
	if err != nil || out.Outcome != nlq.StrategyClarify || out.Clarification == nil || out.Clarification.Reason != "ambiguous_topic" || engine.embeds != 1 {
		t.Fatalf("ambiguous topics silently selected: %v %#v", err, out)
	}
	service.index = &cw07Index{distances: map[string]float64{"alpha": 0.2, "beta": 0.21}, events: &events, omitLast: true}
	_, err = service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue", InterpretationAnchor: "2026-09-22"})
	if !errors.Is(err, gateway.ErrOutput) {
		t.Fatalf("incomplete candidate results accepted: %v", err)
	}
}

func testCW07CandidateReachDeniedBeforeProvider(t *testing.T) {
	events := []string{}
	descriptor := gateway.EmbeddingSpace{Provider: "fixture", Route: "embedding", Endpoint: "default", Model: "embedding-model", Revision: "generation-1", Dimensions: 2, Preprocessing: "raw", InputType: "text", Normalization: "l2"}
	engine := &testEngine{descriptor: descriptor, events: &events}
	reader := &cw07Catalog{publications: map[string]topics.Published{"alpha": cw07Publication("alpha"), "beta": cw07Publication("beta")}, events: &events, binding: cw07Binding(1)}
	service, err := New(reader, testRules{err: store.ErrNotFound}, &cw07Index{distances: map[string]float64{"alpha": 0.1, "beta": 0.8}, events: &events}, engine)
	if err != nil {
		t.Fatal(err)
	}
	e, err := identity.FromVerified("tenant", "user", "session", []string{"topics.read", "cw.topic.read:alpha", "cw.source.read:source", "cw.dataset.query:dataset", "cw.execution_context.use:ctx"}, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Route(context.Background(), e, RouteRequest{Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue", InterpretationAnchor: "2026-09-22"})
	if !errors.Is(err, access.ErrNotFound) || engine.embeds != 0 {
		t.Fatalf("foreign candidate reached provider: %v embeds=%d", err, engine.embeds)
	}

	service, engine = cw07DiscoveryService(t, &cw07Catalog{publications: map[string]topics.Published{"alpha": cw07Publication("alpha")}, events: &events, binding: cw07Binding(1)}, map[string]float64{"alpha": 0.1})
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Context: "other-context", Locale: nlq.LanguageEnglish, Question: "Revenue", InterpretationAnchor: "2026-09-22"})
	if err != nil || out.Outcome != nlq.StrategyNoRoute || engine.embeds != 0 {
		t.Fatalf("different-context candidate reached provider: err=%v out=%#v embeds=%d", err, out, engine.embeds)
	}

	service, engine = cw07DiscoveryService(t, &cw07Catalog{publications: map[string]topics.Published{"alpha": cw07Publication("alpha")}, events: &events, binding: cw07Binding(1), tenant: "tenant"}, map[string]float64{"alpha": 0.1})
	foreign, foreignErr := identity.FromVerified("other-tenant", "user", "session", []string{"topics.read", "cw.topic.read:*", "cw.source.read:*", "cw.dataset.query:*", "cw.execution_context.use:*"}, time.Now().Add(time.Hour), nil)
	if foreignErr != nil {
		t.Fatal(foreignErr)
	}
	_, err = service.Route(context.Background(), foreign, RouteRequest{Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue", InterpretationAnchor: "2026-09-22"})
	if !errors.Is(err, access.ErrNotFound) || engine.embeds != 0 {
		t.Fatalf("cross-tenant catalog reached provider: err=%v embeds=%d", err, engine.embeds)
	}
}

func testCW07InterpretationReplayIsDeterministic(t *testing.T) {
	service, _ := cw07Service(t, cw07Publication("topic"), cw07Binding(1))
	out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in north last month", InterpretationAnchor: "2026-09-22"})
	if err != nil {
		t.Fatal(err)
	}
	want, err := out.ResolvedBusinessConstraints()
	if err != nil {
		t.Fatal(err)
	}
	got, binding, err := service.ReplayClarifications(context.Background(), testEnvelope(t, true), out)
	if err != nil || readexec.Hash(want) != readexec.Hash(got) || binding != out.SourceBindingDigest {
		t.Fatalf("interpretation replay drifted: %v %#v %#v", err, want, got)
	}
	parserDrift := out
	parserCopy := *out.Interpretation
	parserCopy.Parser = "retired-parser"
	parserDrift.Interpretation = &parserCopy
	if _, _, err := service.ReplayClarifications(context.Background(), testEnvelope(t, true), parserDrift); err == nil {
		t.Fatal("parser drift replayed")
	}
	vocabulary := cw07Publication("topic")
	vocabulary.Definition.Dimensions[0].Values[0].Value = "NORTH_REVIEWED"
	vocabularyService, _ := cw07Service(t, vocabulary, cw07Binding(1))
	if _, _, err := vocabularyService.ReplayClarifications(context.Background(), testEnvelope(t, true), out); err == nil {
		t.Fatal("governed vocabulary drift replayed")
	}
	publication := cw07Publication("topic")
	publication.State.Version, publication.Definition.Version = "v2", "v2"
	publication.Digest = strings.Repeat("c", 64)
	publicationService, _ := cw07Service(t, publication, cw07Binding(1))
	if _, _, err := publicationService.ReplayClarifications(context.Background(), testEnvelope(t, true), out); err == nil {
		t.Fatal("publication drift replayed")
	}
}

func testCW07TemporalConnectorsAndInstantBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		locale   nlq.Language
		question string
		start    string
		end      string
	}{
		{"spanish-de", nlq.LanguageSpanish, "Ingresos en marzo de 2025", "2025-03-01", "2025-04-01"},
		{"spanish-del", nlq.LanguageSpanish, "Ingresos en marzo del 2025", "2025-03-01", "2025-04-01"},
		{"english-of", nlq.LanguageEnglish, "Revenue in March of 2025", "2025-03-01", "2025-04-01"},
		{"leap-month", nlq.LanguageEnglish, "Revenue in February of 2024", "2024-02-01", "2024-03-01"},
		{"year-end", nlq.LanguageEnglish, "Revenue in December of 2025", "2025-12-01", "2026-01-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, _ := cw07Service(t, cw07Publication("topic"), cw07Binding(1))
			out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: tc.locale, Question: tc.question, InterpretationAnchor: "2026-09-22"})
			if err != nil || out.Interpretation == nil || len(out.Interpretation.Temporal) != 1 || out.Interpretation.Temporal[0].Start != tc.start || out.Interpretation.Temporal[0].End != tc.end {
				t.Fatalf("connector span changed: err=%v out=%#v", err, out.Interpretation)
			}
		})
	}
	for _, tc := range []struct {
		locale   nlq.Language
		question string
	}{
		{nlq.LanguageEnglish, "Revenue in March of revenue"},
		{nlq.LanguageSpanish, "Ingresos en marzo del veinte"},
		{nlq.LanguageEnglish, "Revenue in March 20"},
		{nlq.LanguageEnglish, "Revenue in March de 2025"},
		{nlq.LanguageEnglish, "Revenue in March of 2025 or 2026"},
		{nlq.LanguageEnglish, "Revenue in 2025 March"},
		{nlq.LanguageSpanish, "Ingresos en 2025 marzo"},
		{nlq.LanguageEnglish, "Revenue in ٢٠٢٥ March"},
		{nlq.LanguageSpanish, "Ingresos en ٢٠٢٥ marzo"},
	} {
		service, engine := cw07Service(t, cw07Publication("topic"), cw07Binding(1))
		out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: tc.locale, Question: tc.question, InterpretationAnchor: "2026-09-22"})
		if err != nil || out.Outcome != nlq.StrategyClarify || out.Clarification == nil || out.Clarification.Reason != "invalid_temporal_span" || engine.embeds != 0 {
			t.Fatalf("malformed connector reached provider: err=%v out=%#v embeds=%d", err, out, engine.embeds)
		}
	}

	publication := cw07Publication("topic")
	publication.Definition.Datasets[0].Columns[1].NativeType = "timestamptz"
	publication.Definition.Datasets[0].Columns[1].Category = "timestamp"
	publication.Definition.Dimensions[1].Temporal.Timezone = "America/New_York"
	binding := cw07Binding(1)
	binding.Relations[0].Columns[1].NativeType = "timestamptz"
	binding.Relations[0].Columns[1].Category = "timestamp"
	service, _ := cw07Service(t, publication, binding)
	out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in March of 2025", InterpretationAnchor: "2026-09-22"})
	if err != nil || out.Interpretation == nil || len(out.Interpretation.Temporal) != 1 {
		t.Fatal("instant span unavailable", err)
	}
	span := out.Interpretation.Temporal[0]
	if span.TemporalType != "timestamptz" || span.LocalStart != "2025-03-01" || span.LocalEnd != "2025-04-01" || span.Start != "2025-03-01T05:00:00Z" || span.End != "2025-04-01T04:00:00Z" {
		t.Fatalf("DST month did not preserve reviewed wall boundaries: %#v", span)
	}
	want, err := out.ResolvedBusinessConstraints()
	if err != nil || want[0].Value != span.Start || want[0].Upper != span.End {
		t.Fatalf("instant constraint mismatch: err=%v constraints=%#v", err, want)
	}
	got, _, err := service.ReplayClarifications(context.Background(), testEnvelope(t, true), out)
	if err != nil || readexec.Hash(got) != readexec.Hash(want) {
		t.Fatalf("instant replay changed: err=%v got=%#v", err, got)
	}
	relative, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue last month", InterpretationAnchor: "2025-04-15"})
	if err != nil || relative.Interpretation == nil || len(relative.Interpretation.Temporal) != 1 || relative.Interpretation.Temporal[0].Start != span.Start || relative.Interpretation.Temporal[0].End != span.End {
		t.Fatalf("relative instant span changed: err=%v out=%#v", err, relative.Interpretation)
	}

	wallPublication := cw07Publication("topic")
	wallPublication.Definition.Datasets[0].Columns[1].NativeType = "timestamp"
	wallPublication.Definition.Datasets[0].Columns[1].Category = "timestamp"
	wallPublication.Definition.Dimensions[1].Temporal.Timezone = "America/New_York"
	wallBinding := cw07Binding(1)
	wallBinding.Relations[0].Columns[1].NativeType = "timestamp"
	wallBinding.Relations[0].Columns[1].Category = "timestamp"
	wallService, _ := cw07Service(t, wallPublication, wallBinding)
	wall, err := wallService.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in March of 2025", InterpretationAnchor: "2026-09-22"})
	if err != nil || wall.Interpretation == nil || len(wall.Interpretation.Temporal) != 1 || wall.Interpretation.Temporal[0].TemporalType != "timestamp" || wall.Interpretation.Temporal[0].Start != "2025-03-01" || wall.Interpretation.Temporal[0].End != "2025-04-01" {
		t.Fatalf("wall-clock encoding changed: err=%v out=%#v", err, wall.Interpretation)
	}

	for _, tc := range []struct {
		zone, question string
	}{
		{"America/Havana", "Revenue in April of 2001"},
		{"America/Havana", "Revenue in November of 2020"},
	} {
		publication.Definition.Dimensions[1].Temporal.Timezone = tc.zone
		service, engine := cw07Service(t, publication, binding)
		out, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: tc.question, InterpretationAnchor: "2026-09-22"})
		if err != nil || out.Outcome != nlq.StrategyClarify || out.Clarification == nil || out.Clarification.Reason != "ambiguous_temporal_boundary" || engine.embeds != 0 {
			t.Fatalf("gap/fold boundary reached provider: zone=%s err=%v out=%#v embeds=%d", tc.zone, err, out, engine.embeds)
		}
	}
}

func testCW07InterpretationBudgetFailsBeforeProvider(t *testing.T) {
	publication := cw07Publication("topic")
	publication.Definition.Dimensions = nil
	phrases := make([]string, 0, 64)
	for i := 0; i < 64; i++ {
		id := fmt.Sprintf("value_%03d_%s", i, strings.Repeat("x", 108))
		phrase := fmt.Sprintf("place%03d", i)
		phrases = append(phrases, phrase)
		publication.Definition.Dimensions = append(publication.Definition.Dimensions, semantics.Dimension{
			ID:    fmt.Sprintf("dimension_%03d_%s", i, strings.Repeat("y", 105)),
			Name:  "Reviewed dimension " + id,
			Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "dataset", ID: "region"},
			Role:  semantics.DimensionCategorical,
			Values: []semantics.GovernedValue{{
				ID: id, Value: strings.ToUpper(phrase), Aliases: []string{phrase}, Sensitivity: semantics.LiteralNonSensitive,
				Provenance: semantics.ValueProvenance{Kind: "review", Evidence: "evidence", Policy: "policy"},
			}},
		})
	}
	service, engine := cw07Service(t, publication, cw07Binding(1))
	_, err := service.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: strings.Join(phrases, " "), InterpretationAnchor: "2026-09-22"})
	if !errors.Is(err, nlq.ErrInsufficient) || engine.embeds != 0 {
		t.Fatalf("oversized interpretation reached provider: err=%v embeds=%d", err, engine.embeds)
	}
}

func TestCW07(t *testing.T) {
	t.Run("AC01_server_owned_topic_choice_and_rerank", testCW07ServerSelectsCurrentAuthorizedTopicWithEvidencePolicy)
	t.Run("AC02_competing_topic_ambiguity", testCW07TopicDecisionAmbiguityDoesNotChooseByOrder)
	t.Run("AC03_bilingual_value_geography_time", testCW07InterpretsGovernedGeographyAndSpanishMonthBeforeProvider)
	t.Run("AC04_correction_and_source_fence", testCW07InterpretationCorrectionRemovalAndSourceFence)
	t.Run("AC05_ambiguous_value_fail_closed", testCW07AmbiguousGovernedValueClarifiesBeforeProvider)
	t.Run("AC06_candidate_authority_before_provider", testCW07CandidateReachDeniedBeforeProvider)
	t.Run("AC07_deterministic_replay", testCW07InterpretationReplayIsDeterministic)
	t.Run("AC08_interpretation_budget_before_provider", testCW07InterpretationBudgetFailsBeforeProvider)
	t.Run("AC09_temporal_connectors_and_instant_boundaries", testCW07TemporalConnectorsAndInstantBoundaries)
}
