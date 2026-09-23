package nlqroute

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func TestSQLRecoveryFreeTextProjectionIsIndependentOfUnrelatedSchema(t *testing.T) {
	p := recoveryPublication()
	hit := recoveryFacet(t, p, "kpi", "margin_pct", p.Definition.KPIs[1])
	service, _ := recoveryRouteService(t, p, hit)
	in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Gross margin percentage by product family"}
	original, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
	if err != nil || original.Context == nil {
		t.Fatal("original route", err)
	}
	// No new entity is selected by the question. Source admission still sees
	// and checks the entire publication, independent of model input size.
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("unrelated_%d", i)
		dataset := topics.Dataset{ID: id, Source: topics.Binding{Source: "source", Context: "ctx", Dataset: id, SourceRevision: 1}}
		for j := 0; j < 100; j++ {
			name := fmt.Sprintf("unused_%d", j)
			dataset.Columns = append(dataset.Columns, semantics.Column{ID: name, SourceName: name, Name: name, NativeType: "text", Category: "text"})
		}
		p.Definition.Datasets = append(p.Definition.Datasets, dataset)
	}
	service, _ = recoveryRouteService(t, p, hit)
	expanded, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
	if err != nil || expanded.Context == nil {
		t.Fatal("expanded route", err)
	}
	if expanded.Context.Prompt != original.Context.Prompt || expanded.Context.Tokens != original.Context.Tokens || len(expanded.Context.Relations) != 22 {
		t.Fatal("unrelated growth consumed prompt space or narrowed authorization")
	}
	assembled, err := expanded.GenerationContext()
	if err != nil {
		t.Fatal(err)
	}
	generation, err := service.assembler.ResolvePrecedence(context.Background(), nlq.GenerationInput{Context: assembled})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"physical_projection:selected-closure-v1", "sales_cost", "family_name", "sales_products"} {
		if !strings.Contains(generation.Prompt, required) {
			t.Fatalf("missing %s", required)
		}
	}
	if strings.Contains(generation.Prompt, "unrelated_") || len(generation.Context.Relations) != 22 {
		t.Fatal("final packet confused relevant schema with authority")
	}
}
