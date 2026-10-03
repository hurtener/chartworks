package nlq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func scopedProjectionColumn(dataset, id string) MetricDependency {
	return MetricDependency{Kind: "column", ID: dataset + ":" + id, Text: fmt.Sprintf(`{"dataset":%q,"column":{"id":%q,"source_name":%q,"native_type":"text"}}`, dataset, id, id)}
}

func scopedProjectionSelection(topic string) MandatoryConstraint {
	return MandatoryConstraint{ID: "selection-" + projectionIdentity(topic), Kind: "selected_semantics", Text: `[{"reference":{"kind":"dimension","id":"label"},"reason":"catalog_term"}]`}
}

func scopedProjectionDependency(topic string, dependency MetricDependency) MandatoryConstraint {
	raw, _ := json.Marshal(dependency)
	return MandatoryConstraint{ID: "semantic-" + projectionIdentity([]string{topic, dependency.Kind, dependency.ID}), Kind: "semantic_dependency", Text: string(raw)}
}

func scopedProjectionFixture() ContextInput {
	return ContextInput{Locale: LanguageEnglish, Strategy: StrategySingleTopic, Topic: "topic", TopicVersion: "v1", Question: "List labels", Relations: []SourceRelation{
		{Topic: "topic", Dataset: "labels", Name: "analytics.labels", Columns: []string{"label", "unused"}},
		{Topic: "topic", Dataset: "other", Name: "analytics.other", Columns: []string{"unrelated"}},
	}, Constraints: &ConstraintState{Allowed: true, Required: []MandatoryConstraint{scopedProjectionSelection("topic"), scopedProjectionDependency("topic", scopedProjectionColumn("labels", "label"))}}}
}

func scopedMultiProjectionFixture() ContextInput {
	in := scopedProjectionFixture()
	in.Topic, in.TopicVersion, in.Strategy = "north", "v1", StrategyMultiTopic
	in.Topics = []TopicRevision{{Topic: "north", Version: "v1"}, {Topic: "south", Version: "v1"}}
	in.Relations = []SourceRelation{
		{Topic: "north", Dataset: "north_labels", Name: "analytics.north_labels", Columns: []string{"label", "unused_north"}},
		{Topic: "north", Dataset: "shared", Name: "analytics.shared", Columns: []string{"join_key", "north_reviewed"}},
		{Topic: "south", Dataset: "south_sales", Name: "analytics.south_sales", Columns: []string{"amount", "unused_south"}},
		{Topic: "south", Dataset: "shared", Name: "analytics.shared", Columns: []string{"join_key", "south_reviewed"}},
	}
	in.Constraints.Required = []MandatoryConstraint{scopedProjectionSelection("north"), scopedProjectionSelection("south"), scopedProjectionDependency("north", scopedProjectionColumn("north_labels", "label"))}
	in.Metrics = []PinnedMetric{{ID: "south:revenue", Text: "Revenue", Dependencies: []MetricDependency{scopedProjectionColumn("south_sales", "amount")}}}
	return in
}

func TestSQLRecoveryScopedProjectionDimensions(t *testing.T) {
	a, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatal(err)
	}
	in := scopedProjectionFixture()
	before, _ := json.Marshal(in)
	got, err := a.Assemble(context.Background(), in, TierLow)
	if err != nil || !strings.Contains(got.Prompt, "physical_projection:topic-selected-closure-v2") || !strings.Contains(got.Prompt, "analytics.labels columns:label") || strings.Contains(got.Prompt, "unused") || strings.Contains(got.Prompt, "analytics.other") {
		t.Fatal("dimension-only closure was not projected", err)
	}
	if !reflect.DeepEqual(got.Relations, in.Relations) || len(got.Metrics) != 0 {
		t.Fatal("projection changed authorization or invented a metric")
	}
	after, _ := json.Marshal(in)
	if string(before) != string(after) {
		t.Fatal("assembly mutated caller input")
	}
	for i := 0; i < 20; i++ {
		r := SourceRelation{Topic: "topic", Dataset: fmt.Sprintf("extra%d", i), Name: fmt.Sprintf("analytics.extra%d", i)}
		for j := 0; j < 100; j++ {
			r.Columns = append(r.Columns, fmt.Sprintf("irrelevant%d", j))
		}
		in.Relations = append(in.Relations, r)
	}
	grown, err := a.Assemble(context.Background(), in, TierLow)
	if err != nil || grown.Prompt != got.Prompt || grown.Tokens != got.Tokens || len(grown.Relations) != 22 {
		t.Fatal("unrelated schema consumed dimension budget", err)
	}
	original := scopedProjectionFixture()
	projected, ok, err := promptRelations(original)
	if err != nil || !ok {
		t.Fatal(err)
	}
	projected[0].Columns[0] = "mutated"
	unchanged, _ := json.Marshal(original)
	if string(unchanged) != string(before) {
		t.Fatal("projected columns alias caller state")
	}
}

func TestSQLRecoveryScopedProjectionTopicsAndSharedJoinKeys(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	in := scopedMultiProjectionFixture()
	got, err := a.Assemble(context.Background(), in, TierHigh)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"topic-selected-closure-v2", "relation[north/north_labels]:analytics.north_labels columns:label", "relation[south/south_sales]:analytics.south_sales columns:amount", "relation[north/shared]:analytics.shared columns:join_key,north_reviewed", "relation[south/shared]:analytics.shared columns:join_key,south_reviewed"} {
		if !strings.Contains(got.Prompt, required) {
			t.Fatalf("lost topic or shared join grounding: %s", required)
		}
	}
	if strings.Contains(got.Prompt, "unused_") || !reflect.DeepEqual(in.Relations, got.Relations) {
		t.Fatal("unrelated fields retained or authorization narrowed")
	}
	for i := 0; i < 30; i++ {
		topic := "north"
		if i%2 == 0 {
			topic = "south"
		}
		in.Relations = append(in.Relations, SourceRelation{Topic: topic, Dataset: fmt.Sprintf("extra%d", i), Name: fmt.Sprintf("analytics.extra%d", i), Columns: []string{"irrelevant"}})
	}
	grown, err := a.Assemble(context.Background(), in, TierHigh)
	if err != nil || grown.Prompt != got.Prompt || grown.Tokens != got.Tokens || len(grown.Relations) != 34 {
		t.Fatal("multi-topic projection depends on unrelated catalog growth", err)
	}
	projected, _, err := promptRelations(in)
	if err != nil {
		t.Fatal(err)
	}
	projected[1].Columns[0] = "mutated"
	if in.Relations[1].Columns[0] != "join_key" {
		t.Fatal("preserved shared relation aliases input")
	}
}

func TestSQLRecoveryScopedProjectionOwnership(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	for _, name := range []string{"foreign relation", "foreign metric", "unqualified metric", "foreign dependency", "wrong dependency owner", "missing column", "missing metric mapping", "conflicting shared identity", "malformed dependency", "mixed unscoped selection", "borrowed shared column"} {
		t.Run(name, func(t *testing.T) {
			in := scopedMultiProjectionFixture()
			switch name {
			case "foreign relation":
				in.Relations[0].Topic = "foreign"
			case "foreign metric":
				in.Metrics[0].ID = "foreign:revenue"
			case "unqualified metric":
				in.Metrics[0].ID = "revenue"
			case "foreign dependency":
				in.Constraints.Required[2] = scopedProjectionDependency("foreign", scopedProjectionColumn("north_labels", "label"))
			case "wrong dependency owner":
				in.Constraints.Required[2] = scopedProjectionDependency("south", scopedProjectionColumn("north_labels", "label"))
			case "missing column":
				in.Relations[0].Columns = []string{"unused_north"}
			case "missing metric mapping":
				in.Relations[2].Columns = []string{"unused_south"}
			case "conflicting shared identity":
				in.Relations[3].Name = "analytics.other"
			case "malformed dependency":
				in.Constraints.Required[2].Text = "not json"
			case "mixed unscoped selection":
				in.Constraints.Required[0].ID = "legacy"
			case "borrowed shared column":
				in.Constraints.Required = append(in.Constraints.Required, scopedProjectionDependency("south", scopedProjectionColumn("shared", "north_reviewed")))
			}
			if _, err := a.Assemble(context.Background(), in, TierHigh); !errors.Is(err, ErrInvalid) {
				t.Fatal("incomplete topic-owned projection accepted", err)
			}
		})
	}
	in := scopedProjectionFixture()
	dep := scopedProjectionColumn("labels", "label")
	dep.Text = `{"dataset":"labels","column":{"id":"label","source_name":"unused"}}`
	if _, err := DependencyRelations("topic", in.Relations, []MetricDependency{scopedProjectionColumn("labels", "label"), dep}); !errors.Is(err, ErrInvalid) {
		t.Fatal("conflicting physical mappings for one semantic column accepted", err)
	}
}

func TestSQLRecoveryScopedProjectionConservativeFallback(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	in := scopedProjectionFixture()
	in.Constraints.Required = in.Constraints.Required[:1]
	got, err := a.Assemble(context.Background(), in, TierHigh)
	if err != nil || !strings.Contains(got.Prompt, "analytics.other") || strings.Contains(got.Prompt, "physical_projection:") {
		t.Fatal("selection without a column closure guessed a projection", err)
	}
	in = scopedMultiProjectionFixture()
	in.Constraints.Required = in.Constraints.Required[1:2]
	got, err = a.Assemble(context.Background(), in, TierHigh)
	if err != nil || !strings.Contains(got.Prompt, "unused_north") || strings.Contains(got.Prompt, "unused_south") {
		t.Fatal("unselected topic was pruned or selected topic not projected", err)
	}
	in = scopedMultiProjectionFixture()
	in.Metrics = append(in.Metrics, PinnedMetric{ID: "south:legacy", Text: "opaque retained metric"})
	got, err = a.Assemble(context.Background(), in, TierHigh)
	if err != nil || !strings.Contains(got.Prompt, "unused_south") || strings.Contains(got.Prompt, "unused_north") {
		t.Fatal("opaque metric lost its original topic schema", err)
	}
}

func TestSQLRecoveryScopedProjectionGenerationAndRefit(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	in := scopedProjectionFixture()
	in.Evidence = []Evidence{{ID: "optional", Text: strings.Repeat("optional evidence ", 100)}}
	assembled, err := a.Assemble(context.Background(), in, TierHigh)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: assembled})
	if err != nil {
		t.Fatal(err)
	}
	fitted, err := a.RefitGeneration(context.Background(), generation, func(prompt string) (bool, error) {
		return !strings.Contains(prompt, "optional evidence"), nil
	})
	if err != nil || !strings.Contains(fitted.Prompt, "topic-selected-closure-v2") || !strings.Contains(fitted.Prompt, "columns:label") || strings.Contains(fitted.Prompt, "unused") || !reflect.DeepEqual(fitted.Context.Relations, in.Relations) {
		t.Fatal("provider refit lost mandatory grounding or full scope", err)
	}
	raw, _ := json.Marshal(fitted)
	var retained GenerationContext
	if err := json.Unmarshal(raw, &retained); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RefitGeneration(context.Background(), retained, func(string) (bool, error) { return true, nil }); err == nil {
		t.Fatal("wire projection manufactured an in-process generation seal")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Assemble(ctx, in, TierHigh); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled assembly continued", err)
	}
}
