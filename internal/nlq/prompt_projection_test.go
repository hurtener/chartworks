package nlq

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func projectionFixture() ContextInput {
	in := minimalInput()
	in.Relations = []SourceRelation{{Topic: in.Topic, Dataset: "sales", Name: "analytics.sales", Columns: []string{"amount", "family_id", "unneeded_field"}}, {Topic: in.Topic, Dataset: "family", Name: "analytics.family", Columns: []string{"id", "label"}}}
	column := func(dataset, id string) MetricDependency {
		return MetricDependency{Kind: "column", ID: dataset + ":" + id, Text: fmt.Sprintf(`{"dataset":%q,"column":{"id":%q,"source_name":%q,"native_type":"numeric","nullable":false}}`, dataset, id, id)}
	}
	in.Metrics = []PinnedMetric{{ID: "revenue", Text: "sum of amount", Dependencies: []MetricDependency{column("sales", "amount")}}}
	in.Constraints = &ConstraintState{Allowed: true, Required: []MandatoryConstraint{{ID: "selected", Kind: "selected_semantics", Text: "reviewed revenue by family"}}}
	for i, dep := range []MetricDependency{column("sales", "family_id"), column("family", "id"), column("family", "label")} {
		raw, _ := json.Marshal(dep)
		in.Constraints.Required = append(in.Constraints.Required, MandatoryConstraint{ID: fmt.Sprintf("dep-%d", i), Kind: "semantic_dependency", Text: string(raw)})
	}
	return in
}

func TestSQLRecoveryProjectionKeepsAuthorizationScopeAndTypedJoinClosure(t *testing.T) {
	a, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatal(err)
	}
	in := projectionFixture()
	before, _ := json.Marshal(in)
	got, err := a.Assemble(context.Background(), in, TierHigh)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Relations, in.Relations) {
		t.Fatal("physical authorization scope changed")
	}
	for _, required := range []string{"physical_projection:selected-closure-v1", "analytics.sales columns:amount,family_id", "analytics.family columns:id,label", `\"native_type\":\"numeric\"`} {
		if !strings.Contains(got.Prompt, required) {
			t.Fatalf("missing required projection/metadata %q", required)
		}
	}
	if strings.Contains(got.Prompt, "unneeded_field") {
		t.Fatal("unselected field leaked into prompt")
	}
	after, _ := json.Marshal(in)
	if string(before) != string(after) {
		t.Fatal("mutated caller")
	}
	gen, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: got})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gen.Context.Relations, in.Relations) {
		t.Fatal("generation narrowed authority")
	}
}

func TestSQLRecoveryProjectionIgnoresUnrelatedCatalogGrowth(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	in := projectionFixture()
	base, err := a.Assemble(context.Background(), in, TierLow)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		r := SourceRelation{Topic: in.Topic, Dataset: fmt.Sprintf("irrelevant-%d", i), Name: fmt.Sprintf("analytics.irrelevant_%d", i)}
		for j := 0; j < 100; j++ {
			r.Columns = append(r.Columns, fmt.Sprintf("irrelevant_column_%d", j))
		}
		in.Relations = append(in.Relations, r)
	}
	grown, err := a.Assemble(context.Background(), in, TierLow)
	if err != nil {
		t.Fatal(err)
	}
	if grown.Prompt != base.Prompt || grown.Tokens != base.Tokens || len(grown.Relations) != 22 {
		t.Fatal("unrelated schema consumed selected context budget or narrowed scope")
	}
}

func TestSQLRecoveryProjectionRejectsForeignOrMissingDependencies(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	for _, change := range []func(*ContextInput){
		func(in *ContextInput) { in.Relations = in.Relations[:1] },
		func(in *ContextInput) { in.Relations[0].Columns = []string{"amount"} },
		func(in *ContextInput) {
			in.Metrics[0].Dependencies[0].Text = `{"dataset":"foreign","column":{"id":"amount","source_name":"amount"}}`
		},
		func(in *ContextInput) { in.Metrics[0].Dependencies[0].Text = `not json` },
		func(in *ContextInput) { in.Relations[0].Topic = "foreign" },
	} {
		in := projectionFixture()
		change(&in)
		if _, err := a.Assemble(context.Background(), in, TierHigh); err == nil {
			t.Fatal("incomplete or foreign projection accepted")
		}
	}
}

func TestSQLRecoveryProjectionRetainsLegacyAndUnselectedRendering(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	for _, selection := range []bool{false, true} {
		in := projectionFixture()
		if selection {
			in.Metrics = nil
		} else {
			in.Constraints = nil
		}
		got, err := a.Assemble(context.Background(), in, TierHigh)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Prompt, "unneeded_field") || strings.Contains(got.Prompt, "physical_projection:") {
			t.Fatal("legacy/unselected shape silently reinterpreted")
		}
	}
}
