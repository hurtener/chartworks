package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func scalarPeriodAdmission(t *testing.T, activity bool) admission {
	t.Helper()
	a := independentAdmission()
	a.binding.Tenant = "tenant"
	a.binding.Contract = "contract"
	a.binding.Fingerprint = exec.Hash("scalar-source")
	def := &a.publications[0].Definition
	for i := range def.Datasets {
		d := &def.Datasets[i]
		for _, column := range []semantics.Column{{ID: "period", SourceName: "period", NativeType: "timestamptz", Category: "temporal", Nullable: true}, {ID: "division", SourceName: "division", NativeType: "int4", Category: "numeric"}} {
			d.Columns = append(d.Columns, column)
			a.binding.Relations[i].Columns = append(a.binding.Relations[i].Columns, exec.Column{Name: column.SourceName, NativeType: column.NativeType, Category: column.Category, Nullable: column.Nullable, Safe: true})
		}
		def.Dimensions = append(def.Dimensions, semantics.Dimension{ID: d.ID + "-period", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: "period"}, Role: semantics.DimensionTemporal, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "America/New_York", Grains: []semantics.TimeGrain{semantics.GrainYear}}})
		if d.ID == "refunds" {
			d.Columns = append(d.Columns, semantics.Column{ID: "order", SourceName: "order_id", NativeType: "int4", Category: "numeric"})
			a.binding.Relations[i].Columns = append(a.binding.Relations[i].Columns, exec.Column{Name: "order_id", NativeType: "int4", Category: "numeric", Safe: true})
		}
		scope := exec.RelationScope{Dataset: d.ID}
		for _, column := range d.Columns {
			scope.Columns = append(scope.Columns, column.SourceName)
		}
		a.relationScope = append(a.relationScope, scope)
	}
	a.binding.Relations[0].UniqueKeys = [][]string{{"division", "id_native"}}
	def.Joins = []semantics.Join{{ID: "refund-parent", Type: semantics.JoinInner, Cardinality: semantics.CardinalityManyToOne, Left: semantics.Reference{Kind: semantics.KindColumn, Dataset: "refunds", ID: "order"}, Right: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "id"}, AdditionalKeys: []semantics.JoinKeyPair{{Left: semantics.Reference{Kind: semantics.KindColumn, Dataset: "refunds", ID: "division"}, Right: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "division"}}}, Evidence: semantics.RelationshipEvidence{ID: "reviewed-parent", Provenance: "reviewed-fixture", LeftGrain: "refund", RightGrain: "order"}}}
	def.Measures[2].Filters = []semantics.SemanticFilter{{ID: "paid-parent", Relationship: "refund-parent", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "region"}, Operator: "eq", Values: []string{"paid"}}}
	dim := "sales-period"
	if activity {
		dim = "refunds-period"
	}
	def.KPIs[0].Periods = &semantics.MetricPeriodBindings{Policy: semantics.MetricPeriodBindingsPolicy, Bindings: []semantics.MetricPeriodBinding{
		{Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: "refunds"}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: dim}},
		{Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: "sales-period"}},
	}}
	analyticalReseal(&a)
	app := nlqroute.MetricPeriodApplication{Policy: nlqroute.MetricPeriodApplicationPolicy, Topic: def.Topic, KPI: def.KPIs[0].ID, TopicVersion: def.Version, PackDigest: a.publications[0].Digest, MappingDigest: exec.Hash(def.KPIs[0].Periods), SourceBindingDigest: exec.Hash(a.binding)}
	for _, mapping := range def.KPIs[0].Periods.Bindings {
		fact := "sales"
		if mapping.Measure.ID == "refunds" {
			fact = "refunds"
		}
		target := "sales"
		if mapping.Dimension.ID == "refunds-period" {
			target = "refunds"
		}
		app.Bindings = append(app.Bindings, nlqroute.MetricPeriodBinding{Measure: mapping.Measure, Dimension: mapping.Dimension, Population: fact, Constraint: exec.BusinessConstraint{Resolution: exec.Hash(mapping), Dataset: target, Column: "period", SourceRevision: 1, Kind: "time_window", Operator: "range", Nulls: "exclude", Bounds: "[)", TemporalType: "timestamptz", Calendar: "gregorian", TimeZone: "America/New_York", Grain: "year", Value: "2026-01-01T05:00:00Z", Upper: "2027-01-01T05:00:00Z"}})
	}
	a.metricPeriods = []nlqroute.MetricPeriodApplication{app}
	return a
}

func TestSQLRecoveryScopedPopulationCompiler(t *testing.T) {
	for _, activity := range []bool{false, true} {
		a := scalarPeriodAdmission(t, activity)
		c, err := compileAnalyticalVersion(context.Background(), a, 9)
		if err != nil || c == nil {
			t.Fatal("compile scalar periods", activity, err)
		}
		if c.Version != exec.AnalyticalScopedPopulationsVersion || c.ScalarPopulations == nil || len(c.ScalarPopulations.Lanes) != 2 || len(c.Populations)+len(c.Joins) != 0 {
			t.Fatal("wrong population program")
		}
		lane := c.ScalarPopulations.Lanes[0]
		want := "sales"
		if activity {
			want = "refunds"
		}
		if lane.Dataset != "refunds" || len(lane.Joins) != 1 || len(lane.Joins[0].LeftColumns) != 2 || lane.QueryPopulation.Constraints[0].Dataset != want {
			t.Fatal("wrong parent/period ownership")
		}
		guidance := analyticalScalarPopulationGuidance(c)
		if strings.Contains(guidance, "2026") || strings.Contains(guidance, "05:00") || !strings.Contains(guidance, "CROSS JOIN") {
			t.Fatal("private values entered guidance")
		}
	}
}

func TestSQLRecoveryScopedPopulationCompilerRejectsUnsealedMeaning(t *testing.T) {
	for _, change := range []func(*admission){
		func(a *admission) { a.metricPeriods[0].MappingDigest = exec.Hash("wrong") },
		func(a *admission) { a.metricPeriods[0].TopicVersion = "stale" },
		func(a *admission) { a.metricPeriods[0].Bindings = a.metricPeriods[0].Bindings[:1] },
		func(a *admission) { a.metricPeriods[0].Bindings[0].Population = "sales" },
		func(a *admission) { a.metricPeriods[0].Bindings[0].Constraint.Dataset = "refunds" },
		func(a *admission) { a.metricPeriods[0].Bindings[0].Constraint.TimeZone = "UTC" },
		func(a *admission) {
			a.binding.Relations[0].UniqueKeys = nil
			a.metricPeriods[0].SourceBindingDigest = exec.Hash(a.binding)
		},
		func(a *admission) {
			a.publications[0].Definition.Measures[2].Filters[0].Relationship = "wrong"
			analyticalReseal(a)
			a.metricPeriods[0].PackDigest = a.publications[0].Digest
		},
	} {
		a := scalarPeriodAdmission(t, false)
		change(&a)
		if _, err := compileAnalyticalVersion(context.Background(), a, 9); err == nil {
			t.Fatal("unproved period/relationship accepted")
		}
	}
	a := scalarPeriodAdmission(t, false)
	a.metricPeriods = nil
	_, err := compileAnalyticalVersion(context.Background(), a, 9)
	var detail *exec.AnalyticalError
	if !errors.As(err, &detail) || detail.Code != AnalyticalMetricPeriodReviewCode {
		t.Fatal("missing interval did not request review", err)
	}
	a = scalarPeriodAdmission(t, false)
	if _, err = compileAnalyticalVersion(context.Background(), a, 8); err == nil {
		t.Fatal("v8 borrowed scoped metadata")
	}
	// JSON round-trip of a public route does not recreate the private application.
	raw, _ := json.Marshal(a.route)
	var route nlqroute.RouteResult
	_ = json.Unmarshal(raw, &route)
	a.route = route
	a.metricPeriods = nil
	if _, err = compileAnalyticalVersion(context.Background(), a, 9); err == nil {
		t.Fatal("serialized route invented period authority")
	}
}
