package nlqroute

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
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

func TestSQLRecoveryOptionalCandidateRetainsPhysicalMappings(t *testing.T) {
	p := recoveryPublication()
	hit := recoveryFacet(t, p, "dimension", "family", p.Definition.Dimensions[0])
	service, _ := recoveryRouteService(t, p, hit)
	// "product category" is not a reviewed alias of "Product family". Revenue
	// is selected, but the dimension must remain a candidate rather than being
	// silently pinned. A smaller mandatory schema must not orphan its mapping.
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue by product category"})
	if err != nil || out.Context == nil || out.Selection == nil || len(out.Selection.Topics[0].Roots) != 1 || len(out.Context.Evidence) != 1 {
		t.Fatal("partial selection lost candidate", err)
	}
	var group semanticEvidenceGroup
	if err := json.Unmarshal([]byte(out.Context.Evidence[0].Text), &group); err != nil {
		t.Fatal(err)
	}
	if group.Version != "semantic-evidence-v2" || len(group.Relations) != 1 || group.Relations[0].Name != "analytics.products" || len(group.Relations[0].Columns) != 1 || group.Relations[0].Columns[0] != "family_name" {
		t.Fatal("candidate physical mapping missing or expanded", group.Relations)
	}
	assembled, err := out.GenerationContext()
	if err != nil {
		t.Fatal(err)
	}
	generation, err := service.assembler.ResolvePrecedence(context.Background(), nlq.GenerationInput{Context: assembled})
	if err != nil || !strings.Contains(generation.Prompt, "analytics.products") || !strings.Contains(generation.Prompt, "family_name") || !strings.Contains(generation.Prompt, "additional reviewed mappings") {
		t.Fatal("retained candidate lost its grounding at generation", err)
	}
	if out.Evidence[0].Text != hit.Text {
		t.Fatal("mutated vector evidence")
	}
}

func TestSQLRecoveryProjectionDoesNotBypassFullDatasetAuthority(t *testing.T) {
	p := recoveryPublication()
	p.Definition.Datasets = append(p.Definition.Datasets, topics.Dataset{ID: "unrelated", Source: topics.Binding{Source: "source", Context: "ctx", Dataset: "unrelated", SourceRevision: 1}, Columns: []semantics.Column{{ID: "value", SourceName: "value", Name: "Unused", NativeType: "text", Category: "text"}}})
	hit := recoveryFacet(t, p, "measure", "revenue", p.Definition.Measures[0])
	service, engine := recoveryRouteService(t, p, hit)
	e, err := identity.FromVerified("tenant", "user", "session", []string{"topics.read", "cw.topic.read:topic", "cw.source.read:source", "cw.dataset.query:dataset", "cw.dataset.query:products", "cw.execution_context.use:ctx"}, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Route(context.Background(), e, RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue"})
	if err == nil || engine.embeds != 0 || engine.reranks != 0 {
		t.Fatal("prompt projection bypassed full source authority")
	}
}
