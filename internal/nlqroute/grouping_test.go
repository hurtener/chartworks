package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestSQLRecoveryGroupingShapeAndCopies(t *testing.T) {
	x := &GroupingSelection{Policy: GroupingPolicy, Keys: []GroupingKey{{Topic: "topic", Dimension: "z"}, {Topic: "topic", Dimension: "a", Grain: semantics.GrainMonth}}}
	before, _ := json.Marshal(x)
	if err := ValidateGrouping(x); err != nil {
		t.Fatal(err)
	}
	copy := CloneGrouping(x)
	copy.Keys[0].Dimension = "mutated"
	after, _ := json.Marshal(x)
	if string(before) != string(after) {
		t.Fatal("caller mutated")
	}
	for _, bad := range []*GroupingSelection{{Policy: "future"}, {Policy: GroupingPolicy, Keys: []GroupingKey{{Topic: "t", Dimension: "d", Grain: "week"}}}, {Policy: GroupingPolicy, Keys: []GroupingKey{{Topic: "t", Dimension: "d"}, {Topic: "t", Dimension: "d"}}}, {Policy: GroupingPolicy, Keys: []GroupingKey{{Topic: "", Dimension: "d"}}}, {Policy: GroupingPolicy, Keys: make([]GroupingKey, 17)}} {
		if ValidateGrouping(bad) == nil {
			t.Fatal("invalid grouping admitted")
		}
	}
	if ValidateGrouping(nil) != nil || ValidateGrouping(&GroupingSelection{Policy: GroupingPolicy}) != nil {
		t.Fatal("nil and explicit total must be valid distinct states")
	}
	if CloneGrouping(nil) != nil || CloneGrouping(&GroupingSelection{Policy: GroupingPolicy}).Keys == nil {
		t.Fatal("empty state normalization")
	}
}

func TestSQLRecoveryGroupingReplacesInferredRootsBeforeRules(t *testing.T) {
	p := recoveryPublication()
	p.Definition.Dimensions = append(p.Definition.Dimensions, semantics.Dimension{ID: "amount_group", Name: "Amount grouping", Field: recoveryColumn("dataset", "amount"), Role: semantics.DimensionNumeric})
	service, _ := recoveryRouteService(t, p, recoveryFacet(t, p, "kpi", "margin_pct", p.Definition.KPIs[1]))
	in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Gross margin percentage by product family", Grouping: &GroupingSelection{Policy: GroupingPolicy, Keys: []GroupingKey{{Topic: "topic", Dimension: "amount_group"}}}}
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
	if err != nil || out.Context == nil {
		t.Fatal("new grouping not routed", err, out.Clarification)
	}
	group, stale := false, false
	for _, root := range out.Selection.Topics[0].Roots {
		group = group || root.Reference.ID == "amount_group" && root.Reason == "grouping"
		stale = stale || root.Reference.ID == "family"
	}
	if !group || stale {
		t.Fatal("old question grouping leaked into selected rule roots")
	}
	if !strings.Contains(out.Context.Prompt, "amount_group") {
		t.Fatal("mandatory grouping dependencies lost")
	}
	if _, _, err = service.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), out); err != nil {
		t.Fatal("grouping replay", err)
	}
	out.Request.Grouping.Keys[0].Dimension = "family"
	if _, err = out.ResolvedBusinessConstraints(); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("in-process grouping could change outside seal", err)
	}
}

func TestSQLRecoveryGroupingRejectsMissingOrOmittedRootsBeforeModel(t *testing.T) {
	p := recoveryPublication()
	for _, key := range []GroupingKey{{Topic: "other", Dimension: "family"}, {Topic: "topic", Dimension: "missing"}, {Topic: "topic", Dimension: "family", Grain: semantics.GrainMonth}} {
		service, engine := recoveryRouteService(t, p, recoveryFacet(t, p, "measure", "revenue", p.Definition.Measures[0]))
		_, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue", Grouping: &GroupingSelection{Policy: GroupingPolicy, Keys: []GroupingKey{key}}})
		if err == nil || engine.embeds != 0 {
			t.Fatal("foreign/unsupported grouping reached embedding", err)
		}
	}
	service, engine := recoveryRouteService(t, p, recoveryFacet(t, p, "measure", "revenue", p.Definition.Measures[0]))
	_, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue", OmittedRoots: []semantics.Reference{{Kind: semantics.KindDimension, ID: "family"}}, Grouping: &GroupingSelection{Policy: GroupingPolicy, Keys: []GroupingKey{{Topic: "topic", Dimension: "family"}}}})
	if err == nil || engine.embeds != 0 {
		t.Fatal("contradictory explicit state admitted", err)
	}
}
