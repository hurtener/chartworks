package nlqexec

import (
	"context"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func knownAmountAdmission(t *testing.T) admission {
	t.Helper()
	a := analyticalAdmission()
	d := &a.publications[0].Definition
	d.Datasets[0].Columns[1].SemanticRole = semantics.SemanticRoleFactKey
	d.Measures[1].Aggregation = semantics.AggregationCount
	d.Measures = append(d.Measures, semantics.Measure{ID: "known", Field: d.Measures[0].Field, Aggregation: semantics.AggregationCount})
	d.KPIs = []semantics.KPI{{ID: "unknown", Expression: "orders-known", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "orders"}, {Kind: semantics.KindMeasure, ID: "known"}}}}
	d.Measures[0].Completeness = &semantics.KnownAmountCompleteness{Policy: semantics.KnownAmountCompletenessPolicy, UnknownCount: semantics.Reference{Kind: semantics.KindKPI, ID: "unknown"}}
	a.route.Selection.Topics[0].Roots = []nlqroute.SelectedRoot{{Reference: semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}, Reason: "catalog_term"}, {Reference: semantics.Reference{Kind: semantics.KindKPI, ID: "unknown"}, Reason: "catalog_term"}}
	analyticalReseal(&a)
	return a
}

func TestSQLRecoveryCompletenessCompiler(t *testing.T) {
	a := knownAmountAdmission(t)
	c, err := compileCurrentAnalytical(context.Background(), a)
	if err != nil || c == nil || c.Version != exec.AnalyticalScopedPopulationsVersion || c.ScalarPopulations != nil || compiledKnownAmountCompleteness(c) == nil {
		t.Fatal("ordinary completeness not compiled", err)
	}
	if len(c.Completeness.Obligations) != 1 || c.Completeness.Obligations[0].Metric != "sales_topic:measure:revenue" || c.Completeness.Obligations[0].UnknownCount != "sales_topic:kpi:unknown" {
		t.Fatal("wrong output obligations")
	}
	for _, mutate := range []func(*admission){
		func(a *admission) { a.route.Selection.Topics[0].Roots = a.route.Selection.Topics[0].Roots[:1] },
		func(a *admission) { a.publications[0].Definition.KPIs[0].Expression = "known-orders" },
		func(a *admission) { a.publications[0].Definition.Measures[2].Field.ID = "id" },
		func(a *admission) { a.binding.Relations[0].Columns[1].Nullable = true },
		func(a *admission) { a.binding.Dialect = "sqlserver" },
		func(a *admission) {
			a.publications[0].Definition.Measures[0].Filters = []semantics.SemanticFilter{{ID: "parent", Relationship: "selected-parent", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "region"}, Operator: "eq", Values: []string{"paid"}}}
		},
		func(a *admission) {
			a.publications[0].Definition.KPIs[0].Periods = &semantics.MetricPeriodBindings{Policy: semantics.MetricPeriodBindingsPolicy}
		},
	} {
		a := knownAmountAdmission(t)
		mutate(&a)
		analyticalReseal(&a)
		if _, err := compileCurrentAnalytical(context.Background(), a); err == nil {
			t.Fatal("weaker completeness authoring/physical evidence accepted")
		}
	}
	// Exact retained v8 semantics remain unchanged; they acquire no output claim.
	old, err := compileAnalyticalVersion(context.Background(), a, 8)
	if err != nil || old.Completeness != nil || old.Version != exec.AnalyticalGroupedProgramsVersion {
		t.Fatal("retained v8 relabeled", err)
	}
	// A link on a nested SUM does not invent a required output for an ordinary KPI.
	a = knownAmountAdmission(t)
	a.publications[0].Definition.KPIs = append(a.publications[0].Definition.KPIs, semantics.KPI{ID: "ratio", Expression: "revenue/orders", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}, {Kind: semantics.KindMeasure, ID: "orders"}}})
	a.route.Selection.Topics[0].Roots = []nlqroute.SelectedRoot{{Reference: semantics.Reference{Kind: semantics.KindKPI, ID: "ratio"}, Reason: "catalog_term"}}
	analyticalReseal(&a)
	c, err = compileCurrentAnalytical(context.Background(), a)
	if err != nil || c.Version != exec.AnalyticalGroupedProgramsVersion || c.Completeness != nil {
		t.Fatal("transitive SUM invented an obligation", err)
	}
}

func TestSQLRecoveryCompletenessRejectsNullExtendedIdentity(t *testing.T) {
	for _, kind := range []semantics.JoinType{semantics.JoinLeft, semantics.JoinInner} {
		a := knownAmountAdmission(t)
		d := &a.publications[0].Definition
		columns := []semantics.Column{{ID: "id", SourceName: "id", NativeType: "int4", Category: "numeric"}, {ID: "region", SourceName: "region", NativeType: "text", Category: "text"}}
		d.Datasets = append(d.Datasets, topics.Dataset{ID: "anchor", Source: topics.Binding{Source: "source", Context: "context", Dataset: "anchor", SourceRevision: 1}, Columns: columns})
		a.binding.Relations[0].UniqueKeys = [][]string{{"id_native"}}
		a.binding.Relations = append(a.binding.Relations, exec.Relation{ID: "anchor", Schema: "analytics", Name: "anchor", Columns: []exec.Column{{Name: "id", NativeType: "int4", Category: "numeric", Safe: true}, {Name: "region", NativeType: "text", Category: "text", Safe: true}}, UniqueKeys: [][]string{{"id"}}})
		d.Measures = append(d.Measures, semantics.Measure{ID: "anchor_total", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "anchor", ID: "id"}, Aggregation: semantics.AggregationSum})
		d.Dimensions = []semantics.Dimension{{ID: "region", Name: "Region", Role: semantics.DimensionCategorical, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "anchor", ID: "region"}}}
		d.Joins = []semantics.Join{{ID: "anchor_sales", Type: kind, Cardinality: semantics.CardinalityOneToOne, Left: semantics.Reference{Kind: semantics.KindColumn, Dataset: "anchor", ID: "id"}, Right: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "id"}}}
		a.route.Request.Question = "Revenue by Region"
		a.route.Request.Locale = nlq.LanguageEnglish
		roots := []nlqroute.SelectedRoot{{Reference: semantics.Reference{Kind: semantics.KindMeasure, ID: "anchor_total"}, Reason: "catalog_term"}}
		roots = append(roots, a.route.Selection.Topics[0].Roots...)
		roots = append(roots, nlqroute.SelectedRoot{Reference: semantics.Reference{Kind: semantics.KindDimension, ID: "region"}, Reason: "catalog_term"})
		a.route.Selection.Topics[0].Roots = roots
		analyticalReseal(&a)
		c, err := compileCurrentAnalytical(context.Background(), a)
		if kind == semantics.JoinLeft {
			if err == nil {
				t.Fatal("null-extended completeness identity accepted by fresh compiler", c)
			}
		} else if err != nil {
			t.Fatal("physically unique INNER completeness rejected", err)
		}
	}
}
