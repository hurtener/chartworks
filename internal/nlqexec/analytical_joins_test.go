package nlqexec

import (
	"context"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func independentAdmission() admission {
	a := analyticalAdmission()
	a.binding.Relations = append(a.binding.Relations, exec.Relation{ID: "refunds", Schema: "analytics", Name: "refunds", Columns: []exec.Column{{Name: "amount", NativeType: "numeric", Category: "numeric", Nullable: true, Safe: true}}})
	def := &a.publications[0].Definition
	def.Datasets = append(def.Datasets, topics.Dataset{ID: "refunds", Source: topics.Binding{Source: "source", Context: "context", Dataset: "refunds", SourceRevision: 1}, Columns: []semantics.Column{{ID: "amount", SourceName: "amount", NativeType: "numeric", Category: "numeric", Nullable: true}}})
	def.Measures = append(def.Measures, semantics.Measure{ID: "refunds", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "refunds", ID: "amount"}, Aggregation: semantics.AggregationSum})
	def.KPIs[0].Expression = "revenue - refunds"
	def.KPIs[0].Inputs[1] = semantics.Reference{Kind: semantics.KindMeasure, ID: "refunds"}
	analyticalReseal(&a)
	return a
}

func TestSQLRecoveryIndependentMetricCompiler(t *testing.T) {
	a := independentAdmission()
	c, err := compileAnalytical(context.Background(), a)
	if err != nil || len(c.Populations) != 2 || len(c.Joins) != 0 || c.Metrics[0].Expression.Args[1].Column != "refunds/amount" {
		t.Fatal("independent compilation", c, err)
	}
	if _, err = compileAnalyticalVersion(context.Background(), a, 5); err == nil {
		t.Fatal("retained policy widened")
	}
}

func TestSQLRecoveryJoinedDimensionCompiler(t *testing.T) {
	a := analyticalAdmission()
	a.publications[0].Definition.KPIs[0].Expression = "revenue / orders"
	def := &a.publications[0].Definition
	cols := []semantics.Column{{ID: "id", SourceName: "id", NativeType: "int4", Category: "numeric"}, {ID: "region", SourceName: "region", NativeType: "text", Category: "text"}}
	def.Datasets = append(def.Datasets, topics.Dataset{ID: "customers", Source: topics.Binding{Source: "source", Context: "context", Dataset: "customers", SourceRevision: 1}, Columns: cols})
	a.binding.Relations = append(a.binding.Relations, exec.Relation{ID: "customers", Schema: "analytics", Name: "customers", Columns: []exec.Column{{Name: "id", NativeType: "int4", Category: "numeric", Safe: true}, {Name: "region", NativeType: "text", Category: "text", Safe: true}}, UniqueKeys: [][]string{{"id"}}})
	def.Joins = []semantics.Join{{ID: "customer", Left: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "id"}, Right: semantics.Reference{Kind: semantics.KindColumn, Dataset: "customers", ID: "id"}, Type: semantics.JoinLeft, Cardinality: semantics.CardinalityManyToOne}}
	// The explicit grouping selector supplies its independently reviewed field.
	c := &exec.AnalyticalContract{Version: exec.AnalyticalIntentVersion, Dataset: "sales", Metrics: []exec.AnalyticalMetric{{ID: "revenue", Expression: exec.AnalyticalExpression{Op: "sum", Column: "amount_native"}}}, Grain: &exec.AnalyticalGrain{Policy: exec.AnalyticalGroupingPolicy, Columns: []string{"customers/region"}, Dimensions: []string{"region"}}}
	if err := compileAnalyticalJoins(context.Background(), a, c); err != nil || len(c.Joins) != 1 {
		t.Fatal("physical many-to-one", err)
	}
	a.binding.Relations[1].UniqueKeys = nil
	if err := compileAnalyticalJoins(context.Background(), a, c); err == nil {
		t.Fatal("reviewed label substituted for source proof")
	}
}

func TestSQLRecoveryReviewedArithmeticWords(t *testing.T) {
	for _, formula := range []string{"revenue minus refunds", "revenue menos refunds", "revenue - refunds"} {
		a := independentAdmission()
		a.publications[0].Definition.KPIs[0].Expression = formula
		analyticalReseal(&a)
		c, err := compileAnalytical(context.Background(), a)
		if err != nil || c.Metrics[0].Expression.Op != "-" {
			t.Fatal("reviewed operator spelling", formula, err)
		}
	}
	a := independentAdmission()
	a.publications[0].Definition.KPIs[0].Expression = "revenue minus refunds extra"
	analyticalReseal(&a)
	if _, err := compileAnalytical(context.Background(), a); err == nil {
		t.Fatal("unknown words silently discarded")
	}
}
