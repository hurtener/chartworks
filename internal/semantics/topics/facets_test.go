package topics

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
)

func topicTestPack() semantics.TopicPack {
	column := func(dataset, id string) semantics.Reference {
		return semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset, ID: id}
	}
	return semantics.TopicPack{
		SchemaVersion: semantics.SchemaVersion,
		Topic:         "commerce",
		Version:       "v1",
		Name:          "Commerce",
		Description:   "Synthetic publication",
		Datasets: []semantics.Dataset{
			{ID: "orders", Name: "Orders", Source: semantics.SourceReference{Source: "warehouse-a", Context: "context-a", Dataset: "orders", ProfileVersion: "private-profile-a", ProfileDigest: strings.Repeat("a", 64), SourceRevision: 1}, Columns: []semantics.Column{{ID: "amount", SourceName: "private_amount_a", Name: "Amount", NativeType: "numeric", Category: "decimal"}}},
			{ID: "items", Name: "Items", Source: semantics.SourceReference{Source: "warehouse-b", Context: "context-b", Dataset: "items", ProfileVersion: "private-profile-b", ProfileDigest: strings.Repeat("b", 64), SourceRevision: 1}, Columns: []semantics.Column{{ID: "quantity", SourceName: "private_quantity_b", Name: "Quantity", NativeType: "integer", Category: "integer"}}},
		},
		Measures: []semantics.Measure{
			{ID: "revenue", Name: "Revenue", Description: "Order revenue", Field: column("orders", "amount"), Aggregation: semantics.AggregationSum, Unit: "currency"},
			{ID: "units", Name: "Units", Description: "Item units", Field: column("items", "quantity"), Aggregation: semantics.AggregationSum, Unit: "count"},
		},
	}
}

func topicTestSpace() gateway.EmbeddingSpace {
	return gateway.EmbeddingSpace{Provider: "openrouter", Route: "primary", Endpoint: "https://gateway.example.test/v1", Model: "embed", Revision: "r1", Dimensions: 2, Preprocessing: "utf8-exact;float32-finite", InputType: "text", Normalization: "no-normalization"}
}

func TestFacetPlanPartitionsContextsAndPublicProjection(t *testing.T) {
	model, err := semantics.Compile(topicTestPack())
	if err != nil {
		t.Fatal(err)
	}
	groups, err := facetPlan(model, topicTestSpace())
	if err != nil || len(groups) != 2 || groups[0].generation.Context != "context-a" || groups[1].generation.Context != "context-b" {
		t.Fatal("context partition", len(groups), err)
	}
	for _, group := range groups {
		for _, facet := range group.facets {
			if group.generation.Context == "context-a" && strings.Contains(facet.Text, "private_quantity_b") || group.generation.Context == "context-b" && strings.Contains(facet.Text, "private_amount_a") {
				t.Fatal("cross-context facet content", group.generation.Context, facet.Text)
			}
		}
		if group.generation.Space.Key() != topicTestSpace().Key() || !group.generation.Valid() {
			t.Fatal("gateway-owned space identity lost")
		}
	}
	definition := Project(model.Pack())
	if definition.Datasets[0].Source.Dataset != definition.Datasets[0].ID {
		t.Fatal("published dataset binding lost")
	}
	raw, _ := json.Marshal(definition)
	if strings.Contains(string(raw), "private-profile") || strings.Contains(string(raw), "profile_digest") || strings.Contains(string(raw), "actor") || strings.Contains(string(raw), "session") {
		t.Fatal("private provenance escaped projection", string(raw))
	}
	if !DigestValid(model.Digest()) || DigestValid(strings.ToUpper(model.Digest())) || DigestValid("not-a-digest") {
		t.Fatal("review digest validation")
	}
}

func TestFacetPlanRejectsCrossContextAndBoundOverflow(t *testing.T) {
	pack := topicTestPack()
	pack.KPIs = []semantics.KPI{{ID: "mixed", Name: "Mixed", Description: "Cross context", Expression: "revenue plus units", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}, {Kind: semantics.KindMeasure, ID: "units"}}}}
	model, err := semantics.Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = facetPlan(model, topicTestSpace()); !errors.Is(err, readexec.ErrUnsupported) {
		t.Fatal("cross-context semantic facet accepted", err)
	}

	pack = topicTestPack()
	pack.CanonicalEntities = []semantics.CanonicalEntity{{ID: "entity", Revision: 1, Name: "Entity", Aliases: []string{"registry entity"}, Keys: []semantics.Reference{{Kind: semantics.KindColumn, Dataset: "orders", ID: "amount"}}}}
	model, err = semantics.Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = facetPlan(model, topicTestSpace()); !errors.Is(err, readexec.ErrUnsupported) {
		t.Fatal("unapproved canonical entity accepted", err)
	}

	pack = topicTestPack()
	pack.Measures = nil
	for dataset := range pack.Datasets {
		pack.Datasets[dataset].Columns = nil
		for i := 0; i < 140; i++ {
			id := fmt.Sprintf("column-%03d", i)
			pack.Datasets[dataset].Columns = append(pack.Datasets[dataset].Columns, semantics.Column{ID: id, SourceName: id, Name: id, NativeType: "integer", Category: "integer"})
		}
	}
	model, err = semantics.Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	space := topicTestSpace()
	space.Dimensions = 16000
	if _, err = facetPlan(model, space); !errors.Is(err, readexec.ErrLimit) {
		t.Fatal("vector-value budget overflow accepted", err)
	}
	space = topicTestSpace()
	space.Endpoint = "http://insecure.test"
	if _, err = facetPlan(model, space); !errors.Is(err, readexec.ErrUnsupported) {
		t.Fatal("invalid embedding descriptor accepted", err)
	}
}
