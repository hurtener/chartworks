package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"testing"
)

func TestMetricPeriodApplicationSealedReplay(t *testing.T) {
	p := cw07Publication("topic")
	measure := semantics.Reference{Kind: semantics.KindMeasure, ID: "events"}
	p.Definition.Measures = []semantics.Measure{{ID: "events", Name: "Event count", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "dataset", ID: "occurred"}, Aggregation: semantics.Aggregation("count"), Unit: "count"}}
	p.Definition.KPIs = []semantics.KPI{{ID: "period_events", Name: "Period event count", Expression: "events", Inputs: []semantics.Reference{measure}, Periods: &semantics.MetricPeriodBindings{Policy: semantics.MetricPeriodBindingsPolicy, Bindings: []semantics.MetricPeriodBinding{{Measure: measure, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: "event_date"}}}}}}
	service, _ := cw07Service(t, p, cw07Binding(1))
	ctx := context.Background()
	e := testEnvelope(t, true)
	out, err := service.Route(ctx, e, RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Period event count in March 2026", MetricIDs: []string{"period_events"}, InterpretationAnchor: "2026-10-01"})
	if err != nil || out.Context == nil {
		t.Fatal("route", err, out.Clarification)
	}
	apps, err := out.ResolvedMetricPeriodApplications()
	if err != nil || len(apps) != 1 || len(apps[0].Bindings) != 1 {
		t.Fatal("application", err, len(apps))
	}
	generic, err := out.ResolvedBusinessConstraints()
	if err != nil || len(generic) != 0 {
		t.Fatal("scoped interval became generic", err)
	}
	if apps[0].Bindings[0].Constraint.Value != "2026-03-01" {
		t.Fatal("wrong interval")
	}
	apps[0].Bindings[0].Constraint.Value = "2000-01-01"
	again, err := out.ResolvedMetricPeriodApplications()
	if err != nil || again[0].Bindings[0].Constraint.Value != "2026-03-01" {
		t.Fatal("mutable application", err)
	}
	raw, _ := json.Marshal(out)
	var restored RouteResult
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if _, err = restored.ResolvedMetricPeriodApplications(); !errors.Is(err, readexec.ErrBinding) && !errors.Is(err, ErrNoRoute) {
		t.Fatal("wire supplied seal", err)
	}
	replay, digest, err := service.ReplayMetricPeriodApplications(ctx, e, restored)
	if err != nil || len(replay) != 1 || digest != out.SourceBindingDigest || readexec.Hash(replay) != readexec.Hash(again) {
		t.Fatal("replay", err)
	}
	restored.Interpretation.Temporal[0].Start = "2000-01-01"
	if _, _, err = service.ReplayMetricPeriodApplications(ctx, e, restored); err == nil {
		t.Fatal("tampered period replay")
	}
	out.metricPeriods[0].Bindings[0].Population = "other"
	if _, err = out.ResolvedMetricPeriodApplications(); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("placement escaped seal", err)
	}
	missing, err := service.Route(ctx, e, RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Period event count", MetricIDs: []string{"period_events"}})
	if err != nil || missing.Clarification == nil || missing.Clarification.Reason != "metric_period_required" {
		t.Fatal("missing period", err, missing.Clarification)
	}
}
