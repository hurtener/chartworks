package nlqroute

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func TestSQLRecoveryScopedProjectionDimensionRoute(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			p := recoveryPublication()
			p.Definition.Dimensions[0].Aliases = []string{"familia de producto"}
			question := "Product family"
			if locale == nlq.LanguageSpanish {
				question = "Familia de producto"
			}
			hit := recoveryFacet(t, p, "dimension", "family", p.Definition.Dimensions[0])
			service, _ := recoveryRouteService(t, p, hit)
			request := RouteRequest{Topic: "topic", Context: "ctx", Locale: locale, Question: question}
			original, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), request)
			if err != nil || original.Context == nil || len(original.Context.Metrics) != 0 || original.Selection == nil {
				t.Fatal("dimension route", err)
			}
			if !strings.Contains(original.Context.Prompt, "physical_projection:topic-selected-closure-v2") || !strings.Contains(original.Context.Prompt, "analytics.products columns:family_name") || strings.Contains(original.Context.Prompt, "sales_amount") {
				t.Fatal("actual producer did not supply scoped dimension mappings")
			}
			for i := 0; i < 20; i++ {
				id := fmt.Sprintf("irrelevant_%d", i)
				dataset := topics.Dataset{ID: id, Source: topics.Binding{Source: "source", Context: "ctx", Dataset: id, SourceRevision: 1}}
				for j := 0; j < 100; j++ {
					name := fmt.Sprintf("unused_%d", j)
					dataset.Columns = append(dataset.Columns, semantics.Column{ID: name, SourceName: name, Name: name, NativeType: "text", Category: "text"})
				}
				p.Definition.Datasets = append(p.Definition.Datasets, dataset)
			}
			service, _ = recoveryRouteService(t, p, hit)
			grown, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), request)
			if err != nil || grown.Context == nil || grown.Context.Prompt != original.Context.Prompt || grown.Context.Tokens != original.Context.Tokens || len(grown.Context.Relations) != 22 {
				t.Fatal("dimension route spent tokens on unrelated schema or narrowed source scope", err)
			}
			if _, _, err := service.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), grown); err != nil {
				t.Fatal("render-only change modified semantic replay identity", err)
			}
		})
	}
}

// Exercise the actual selection/closure/constraint producer for independently
// confirmed topics. No assembler test fixture invents the ownership hashes.
func TestSQLRecoveryScopedProjectionMultiProducer(t *testing.T) {
	build := func(extra bool) nlq.AssembledContext {
		p, q := recoveryPublication(), recoveryPublication()
		q.State.Topic, q.Definition.Topic = "related", "related"
		p.Definition.Joins[0].Cardinality = semantics.CardinalityOneToOne
		q.Definition.Joins[0].Cardinality = semantics.CardinalityOneToOne
		items := []admittedTopic{{id: "topic", publication: p}, {id: "related", publication: q}}
		for i := range items {
			if extra {
				for j := 0; j < 12; j++ {
					id := fmt.Sprintf("%s_extra_%d", items[i].id, j)
					items[i].publication.Definition.Datasets = append(items[i].publication.Definition.Datasets, topics.Dataset{ID: id, Source: topics.Binding{Source: "source", Context: "ctx", Dataset: id, SourceRevision: 1}, Columns: []semantics.Column{{ID: "unused", SourceName: "unused", Name: "Unused", NativeType: "text", Category: "text"}}})
				}
			}
			for _, dataset := range items[i].publication.Definition.Datasets {
				relation := readexec.Relation{ID: dataset.ID, Schema: "analytics", Name: dataset.ID}
				for _, column := range dataset.Columns {
					relation.Columns = append(relation.Columns, readexec.Column{Name: column.SourceName, NativeType: column.NativeType, Category: column.Category, Safe: true})
				}
				items[i].relations = append(items[i].relations, relation)
			}
		}
		choices := []JoinChoice{{Topic: "topic", JoinID: "sales_products"}, {Topic: "related", JoinID: "sales_products"}}
		if clarification := confirmJoins(items, choices); clarification != nil {
			t.Fatal("independent join confirmation", clarification)
		}
		if clarification := confirmJoins(items, choices[:1]); clarification == nil {
			t.Fatal("projection must not waive join confirmation")
		}
		if err := initialSemanticSelection(context.Background(), RouteRequest{Locale: nlq.LanguageEnglish, Question: "Product family"}, items, nil); err != nil {
			t.Fatal(err)
		}
		for i := range items {
			if err := expandSelectedFacts(context.Background(), &items[i]); err != nil {
				t.Fatal(err)
			}
		}
		metrics, err := applySelectedContext(items, nil)
		if err != nil || len(metrics) != 0 {
			t.Fatal("dimension producer invented metrics", err)
		}
		relations, err := sourceRelations(items)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := nlq.NewDefaultContextAssembler()
		got, err := a.Assemble(context.Background(), nlq.ContextInput{Locale: nlq.LanguageEnglish, Strategy: nlq.StrategyMultiTopic, Topic: "topic", TopicVersion: "v1", Topics: topicRevisions(items), Question: "Product family", Relations: relations, Metrics: metrics, Constraints: mergeConstraints(items)}, nlq.TierHigh)
		if err != nil || !reflect.DeepEqual(got.Relations, relations) {
			t.Fatal("topic-scoped assembly changed full reviewed relations", err)
		}
		return got
	}
	base, grown := build(false), build(true)
	if base.Prompt != grown.Prompt || base.Tokens != grown.Tokens || len(grown.Relations) != 28 {
		t.Fatal("unrelated multi-topic growth changed physical prompt")
	}
	for _, required := range []string{"topic-selected-closure-v2", "relation[topic/dataset]", "relation[related/products]", "product_id", "family_name"} {
		if !strings.Contains(grown.Prompt, required) {
			t.Fatalf("projection orphaned shared source or join key: %s", required)
		}
	}
}
