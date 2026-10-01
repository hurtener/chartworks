package nlqexec

import (
	"context"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func groupedAdmission() admission {
	a := independentAdmission()
	def := &a.publications[0].Definition
	columns := []semantics.Column{{ID: "id", SourceName: "id", NativeType: "int4", Category: "numeric"}, {ID: "region", SourceName: "region", NativeType: "text", Category: "text", Nullable: true}}
	def.Datasets = append(def.Datasets, topics.Dataset{ID: "customers", Source: topics.Binding{Source: "source", Context: "context", Dataset: "customers", SourceRevision: 1}, Columns: columns})
	a.binding.Relations = append(a.binding.Relations, exec.Relation{ID: "customers", Schema: "analytics", Name: "customers", Columns: []exec.Column{{Name: "id", NativeType: "int4", Category: "numeric", Safe: true}, {Name: "region", NativeType: "text", Category: "text", Nullable: true, Safe: true}}, UniqueKeys: [][]string{{"id"}}})
	for i := range def.Datasets {
		if def.Datasets[i].ID == "refunds" {
			def.Datasets[i].Columns = append(def.Datasets[i].Columns, columns[0])
		}
	}
	for i := range a.binding.Relations {
		if a.binding.Relations[i].ID == "refunds" {
			a.binding.Relations[i].Columns = append(a.binding.Relations[i].Columns, exec.Column{Name: "id", NativeType: "int4", Category: "numeric", Safe: true})
		}
	}
	def.Dimensions = append(def.Dimensions, semantics.Dimension{ID: "customer_region", Name: "Customer region", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "customers", ID: "region"}, Role: semantics.DimensionCategorical})
	for _, id := range []string{"sales", "refunds"} {
		def.Joins = append(def.Joins, semantics.Join{ID: id + "-customer", Left: semantics.Reference{Kind: semantics.KindColumn, Dataset: id, ID: "id"}, Right: semantics.Reference{Kind: semantics.KindColumn, Dataset: "customers", ID: "id"}, Type: semantics.JoinLeft, Cardinality: semantics.CardinalityManyToOne})
	}
	def.GroupedPopulation = &semantics.GroupedPopulationPolicy{Policy: semantics.GroupedPopulationUnionPolicy, Datasets: []string{"refunds", "sales"}}
	a.route.Request.Question = "Net revenue by Customer region"
	a.route.Selection.Topics[0].Roots = append(a.route.Selection.Topics[0].Roots, nlqroute.SelectedRoot{Reference: semantics.Reference{Kind: semantics.KindDimension, ID: "customer_region"}, Reason: "catalog_term"})
	analyticalReseal(&a)
	return a
}

func TestSQLRecoveryGroupedPopulationCompiler(t *testing.T) {
	a := groupedAdmission()
	before := exec.Hash(a.publications)
	c, err := compileAnalytical(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != exec.AnalyticalGroupedProgramsVersion || c.GroupedPopulations == nil || len(c.GroupedPopulations.Lanes) != 2 || len(c.Joins) > 0 || len(c.Populations) > 0 || exec.Hash(a.publications) != before {
		t.Fatal("missing grouped source-backed contract", c)
	}
	if !strings.Contains(analyticalJoinGuidance(c), "IS NOT DISTINCT FROM") {
		t.Fatal("alignment guidance missing")
	}
	if _, err := compileAnalyticalVersion(context.Background(), a, 6); err == nil {
		t.Fatal("v6 borrowed v7 grouping policy")
	}
}

func TestSQLRecoveryGroupedPopulationPolicyDoesNotGrantReach(t *testing.T) {
	for _, mutate := range []func(*admission){
		func(a *admission) { a.publications[0].Definition.GroupedPopulation = nil },
		func(a *admission) {
			a.publications[0].Definition.GroupedPopulation.Datasets = []string{"customers", "refunds", "sales"}
		},
		func(a *admission) { a.publications[0].Definition.GroupedPopulation.Policy = "zero-fill" },
		func(a *admission) { a.binding.Relations[2].UniqueKeys = nil },
		func(a *admission) { a.publications[0].Definition.Joins = nil },
	} {
		a := groupedAdmission()
		mutate(&a)
		analyticalReseal(&a)
		if _, err := compileAnalytical(context.Background(), a); err == nil {
			t.Fatal("unreviewed or unproved grouped policy")
		}
	}
}

func TestSQLRecoveryGroupedReceiptScopeFences(t *testing.T) {
	good := exec.AnalyticalReceipt{Version: exec.AnalyticalGroupedPopulationsVersion, Scope: strings.ReplaceAll(exec.AnalyticalGrainScope, "single_base_relation", "independent_grouped_populations"), Grouping: []string{"topic:dimension:region"}, Intent: exec.AnalyticalIntentPolicy, QueryPopulation: exec.AnalyticalQueryPopulationPolicy}
	if !analyticalReceiptScopeValid(&good) {
		t.Fatal("valid grouped receipt denied")
	}
	for _, mutate := range []func(*exec.AnalyticalReceipt){
		func(r *exec.AnalyticalReceipt) { r.Version = exec.AnalyticalIntentVersion },
		func(r *exec.AnalyticalReceipt) {
			r.Scope = strings.ReplaceAll(exec.AnalyticalCalendarScope, "single_base_relation", "independent_grouped_populations")
		},
		func(r *exec.AnalyticalReceipt) { r.Grouping = nil },
	} {
		bad := good
		mutate(&bad)
		if analyticalReceiptScopeValid(&bad) {
			t.Fatal("unproved grouped scope accepted")
		}
	}
}

func TestSQLRecoveryGroupedPolicyPreservesSafeRawJoins(t *testing.T) {
	a := groupedAdmission()
	a.publications[0].Definition.GroupedPopulation = nil
	for i := range a.publications[0].Definition.Joins {
		a.publications[0].Definition.Joins[i].Type = semantics.JoinInner
	}
	a.binding.Relations[0].UniqueKeys = [][]string{{"id_native"}}
	a.binding.Relations[1].UniqueKeys = [][]string{{"id"}}
	analyticalReseal(&a)
	c, err := compileAnalytical(context.Background(), a)
	if err != nil || c.GroupedPopulations != nil || len(c.Joins) != 2 {
		t.Fatal("safe reviewed raw joins regressed", err)
	}
	a.publications[0].Definition.GroupedPopulation = &semantics.GroupedPopulationPolicy{Policy: semantics.GroupedPopulationUnionPolicy, Datasets: []string{"refunds", "sales"}}
	analyticalReseal(&a)
	c, err = compileAnalytical(context.Background(), a)
	if err != nil || c.GroupedPopulations == nil || len(c.Joins) != 0 {
		t.Fatal("reviewed grouped policy downgraded to raw joins", err)
	}
}
