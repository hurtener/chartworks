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

func confirmedJoinFixture(t *testing.T) ContextInput {
	t.Helper()
	in := scopedMultiProjectionFixture()
	in.Relations = []SourceRelation{
		{Topic: "north", Dataset: "sales", Name: "analytics.sales", Columns: []string{"id", "amount", "private_filter", "north_unused"}},
		{Topic: "north", Dataset: "labels", Name: "analytics.labels", Columns: []string{"sale_id", "label", "north_other"}},
		{Topic: "south", Dataset: "sales", Name: "analytics.sales", Columns: []string{"id", "amount", "south_unused"}},
		{Topic: "south", Dataset: "labels", Name: "analytics.labels", Columns: []string{"sale_id", "label", "south_other"}},
	}
	in.Metrics = []PinnedMetric{{ID: "south:revenue", Text: "Revenue", Dependencies: []MetricDependency{scopedProjectionColumn("sales", "amount")}}}
	in.Constraints.Required = []MandatoryConstraint{scopedProjectionSelection("north"), scopedProjectionSelection("south"), scopedProjectionDependency("north", scopedProjectionColumn("labels", "label")), scopedProjectionDependency("north", scopedProjectionColumn("sales", "private_filter"))}
	for _, topic := range []string{"north", "south"} {
		var relations []SourceRelation
		for _, r := range in.Relations {
			if r.Topic == topic {
				relations = append(relations, r)
			}
		}
		c, err := JoinProjectionConstraint(ConfirmedJoinProjection{Version: "confirmed-joins-v1", Topic: topic, ID: topic + "-join", Type: "inner", Cardinality: "one_to_one", Left: JoinProjectionColumn{Dataset: "sales", ID: "id", Name: "id"}, Right: JoinProjectionColumn{Dataset: "labels", ID: "sale_id", Name: "sale_id"}}, relations)
		if err != nil {
			t.Fatal(err)
		}
		in.Constraints.Required = append(in.Constraints.Required, c)
	}
	return in
}

func TestSQLRecoveryConfirmedJoinProjectionMinimal(t *testing.T) {
	in := confirmedJoinFixture(t)
	before := projectionIdentity(in)
	a, _ := NewDefaultContextAssembler()
	got, err := a.Assemble(context.Background(), in, TierHigh)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"confirmed-join-closure-v3", "relation[north/sales]:analytics.sales columns:id,private_filter", "relation[north/labels]:analytics.labels columns:sale_id,label", "relation[south/sales]:analytics.sales columns:id,amount", "relation[south/labels]:analytics.labels columns:sale_id"} {
		if !strings.Contains(got.Prompt, want) {
			t.Fatal("confirmed key or selected column missing", want)
		}
	}
	if strings.Contains(got.Prompt, "unused") || strings.Contains(got.Prompt, "north_other") || strings.Contains(got.Prompt, "south_other") || !reflect.DeepEqual(got.Relations, in.Relations) || before != projectionIdentity(in) {
		t.Fatal("projection overexposed or mutated authority")
	}
	for i := range in.Relations {
		for j := 0; j < 160; j++ {
			in.Relations[i].Columns = append(in.Relations[i].Columns, fmt.Sprintf("noise%d", j))
		}
	}
	grown, err := a.Assemble(context.Background(), in, TierHigh)
	if err != nil || got.Prompt != grown.Prompt || got.Tokens != grown.Tokens {
		t.Fatal("shared schema growth changed required prompt", err)
	}
	out, _, err := promptRelations(in)
	if err != nil {
		t.Fatal(err)
	}
	out[0].Columns[0] = "changed"
	if in.Relations[0].Columns[0] != "id" {
		t.Fatal("projected columns alias full schema")
	}
}

func TestSQLRecoveryConfirmedJoinProjectionRejectsInvalidSets(t *testing.T) {
	for _, name := range []string{"partial", "duplicate", "foreign", "unknown-field", "wrong-name", "wrong-id", "wrong-direction", "wrong-type", "wrong-cardinality", "borrowed-column", "missing-relation", "single-topic", "different-physical-relation"} {
		t.Run(name, func(t *testing.T) {
			in := confirmedJoinFixture(t)
			index := len(in.Constraints.Required) - 1
			var j ConfirmedJoinProjection
			_ = json.Unmarshal([]byte(in.Constraints.Required[index].Text), &j)
			switch name {
			case "partial":
				in.Constraints.Required = in.Constraints.Required[:index]
			case "duplicate":
				in.Constraints.Required = append(in.Constraints.Required, in.Constraints.Required[index])
			case "foreign":
				j.Topic = "other"
			case "unknown-field":
				in.Constraints.Required[index].Text = strings.TrimSuffix(in.Constraints.Required[index].Text, "}") + `,"authority":true}`
			case "wrong-name":
				j.Left.Name = "south_unused"
			case "wrong-id":
				j.Left.ID = "different"
			case "wrong-direction":
				j.Left, j.Right = j.Right, j.Left
				j.Type = "left"
			case "wrong-type":
				j.Type = "cross"
			case "wrong-cardinality":
				j.Cardinality = "one_to_many"
			case "borrowed-column":
				j.Left.Name = "private_filter"
			case "missing-relation":
				in.Relations = in.Relations[:3]
			case "single-topic":
				in.Strategy = StrategySingleTopic
			case "different-physical-relation":
				in.Relations[2].Name = "analytics.foreign"
			}
			if name != "partial" && name != "duplicate" && name != "unknown-field" {
				raw, _ := json.Marshal(j)
				in.Constraints.Required[index].Text = string(raw)
			}
			a, _ := NewDefaultContextAssembler()
			if _, err := a.Assemble(context.Background(), in, TierHigh); !errors.Is(err, ErrInvalid) {
				t.Fatal("incomplete/mismatched join set accepted", err)
			}
		})
	}
}

func TestSQLRecoveryConfirmedJoinProjectionDirectionAndLegacy(t *testing.T) {
	in := confirmedJoinFixture(t)
	index := len(in.Constraints.Required) - 1
	var j ConfirmedJoinProjection
	_ = json.Unmarshal([]byte(in.Constraints.Required[index].Text), &j)
	j.Left, j.Right = j.Right, j.Left
	raw, _ := json.Marshal(j)
	in.Constraints.Required[index].Text = string(raw)
	a, _ := NewDefaultContextAssembler()
	if _, err := a.Assemble(context.Background(), in, TierHigh); err != nil {
		t.Fatal("symmetric confirmed inner relationship", err)
	}
	for n := index - 1; n <= index; n++ {
		_ = json.Unmarshal([]byte(in.Constraints.Required[n].Text), &j)
		j.Type = "left"
		raw, _ = json.Marshal(j)
		in.Constraints.Required[n].Text = string(raw)
	}
	if _, err := a.Assemble(context.Background(), in, TierHigh); !errors.Is(err, ErrInvalid) {
		t.Fatal("reversed left join accepted", err)
	}
	in = confirmedJoinFixture(t)
	in.Constraints.Required = in.Constraints.Required[:len(in.Constraints.Required)-2]
	got, err := a.Assemble(context.Background(), in, TierHigh)
	if err != nil || !strings.Contains(got.Prompt, "north_unused") || strings.Contains(got.Prompt, "confirmed-join-closure") {
		t.Fatal("legacy relation scope was reinterpreted", err)
	}
}

func TestSQLRecoveryConfirmedJoinProjectionFallbackAndRefit(t *testing.T) {
	in := confirmedJoinFixture(t)
	in.Constraints.Required[0].Text = `[{"reference":{"kind":"column"},"reason":"interpreted_value"}]`
	a, _ := NewDefaultContextAssembler()
	got, err := a.Assemble(context.Background(), in, TierHigh)
	if err != nil || !strings.Contains(got.Prompt, "north_unused") || strings.Contains(got.Prompt, "south_unused") {
		t.Fatal("filter-only topic became complete output projection", err)
	}
	in = confirmedJoinFixture(t)
	in.Evidence = []Evidence{{ID: "optional", Text: strings.Repeat("optional_canary ", 40)}}
	got, err = a.Assemble(context.Background(), in, TierHigh)
	if err != nil {
		t.Fatal(err)
	}
	gen, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: got})
	if err != nil {
		t.Fatal(err)
	}
	fit, err := a.RefitGeneration(context.Background(), gen, func(prompt string) (bool, error) { return !strings.Contains(prompt, "optional_canary"), nil })
	if err != nil || !strings.Contains(fit.Prompt, "confirmed-join-closure-v3") || !strings.Contains(fit.Prompt, "sale_id") || strings.Contains(fit.Prompt, "south_unused") || !reflect.DeepEqual(fit.Context.Relations, in.Relations) {
		t.Fatal("refit pruned required join or narrowed scope", err)
	}
	raw, _ := json.Marshal(fit)
	var retained GenerationContext
	_ = json.Unmarshal(raw, &retained)
	if _, err = a.RefitGeneration(context.Background(), retained, func(string) (bool, error) { return true, nil }); err == nil {
		t.Fatal("retained join metadata issued a seal")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = a.Assemble(ctx, in, TierHigh); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
