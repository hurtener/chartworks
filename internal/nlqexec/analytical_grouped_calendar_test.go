package nlqexec

import (
	"context"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"testing"
)

func TestSQLRecoveryGroupedCalendarCompiler(t *testing.T) {
	a := groupedAdmission()
	col := semantics.Column{ID: "ordered_at", SourceName: "ordered_at", NativeType: "date", Category: "temporal", Nullable: true}
	a.publications[0].Definition.Datasets[2].Columns = append(a.publications[0].Definition.Datasets[2].Columns, col)
	a.binding.Relations[2].Columns = append(a.binding.Relations[2].Columns, exec.Column{Name: "ordered_at", NativeType: "date", Category: "temporal", Nullable: true, Safe: true})
	a.publications[0].Definition.Dimensions = append(a.publications[0].Definition.Dimensions, semantics.Dimension{ID: "order_date", Name: "Order date", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "customers", ID: "ordered_at"}, Role: semantics.DimensionTemporal, Temporal: &semantics.TemporalPolicy{Grains: []semantics.TimeGrain{semantics.GrainMonth}, Calendar: "gregorian"}})
	a.route.Selection.Topics[0].Roots = append(a.route.Selection.Topics[0].Roots, nlqroute.SelectedRoot{Reference: semantics.Reference{Kind: semantics.KindDimension, ID: "order_date"}, Reason: "catalog_term"})
	a.route.Request.Question = "Net revenue by month of Order date"
	analyticalReseal(&a)
	c, err := compileAnalytical(context.Background(), a)
	if err != nil || c.GroupedPopulations == nil || len(c.Grain.Buckets) != 1 || c.Grain.Buckets[0].Column != "customers/ordered_at" {
		t.Fatal("shared calendar compilation", err)
	}
	if _, err := compileAnalyticalVersion(context.Background(), a, 7); err == nil {
		t.Fatal("v7 adopted calendar population policy")
	}
	receipt := &exec.AnalyticalReceipt{Version: exec.AnalyticalGroupedProgramsVersion, Scope: "selected_metric_expression_population_and_calendar_grouping;independent_grouped_populations", Grouping: c.Grain.Dimensions, Intent: exec.AnalyticalIntentPolicy, QueryPopulation: exec.AnalyticalQueryPopulationPolicy}
	if !analyticalReceiptScopeValid(receipt) {
		t.Fatal("v8 calendar scope rejected")
	}
	receipt.Version = exec.AnalyticalGroupedPopulationsVersion
	if analyticalReceiptScopeValid(receipt) {
		t.Fatal("v7 widened calendar receipt")
	}
}
