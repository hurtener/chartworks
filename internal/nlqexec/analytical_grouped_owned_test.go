package nlqexec

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func groupedPeriodAdmission(t *testing.T, activity bool) admission {
	t.Helper()
	a := scalarPeriodAdmission(t, activity)
	def := &a.publications[0].Definition
	def.Dimensions = append(def.Dimensions, semantics.Dimension{ID: "sales_region", Name: "Sales region", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "region"}, Role: semantics.DimensionCategorical})
	def.GroupedPopulation = &semantics.GroupedPopulationPolicy{Policy: semantics.GroupedPopulationUnionPolicy, Datasets: []string{"refunds", "sales"}, GroupDomains: []semantics.GroupedPopulationDomain{{Dataset: "refunds", Domain: exec.AnalyticalGroupDomainQualifying}, {Dataset: "sales", Domain: exec.AnalyticalGroupDomainRaw}}}
	a.route.Request.Question = "Net revenue by Sales region"
	a.route.Selection.Topics[0].Roots = append(a.route.Selection.Topics[0].Roots, nlqroute.SelectedRoot{Reference: semantics.Reference{Kind: semantics.KindDimension, ID: "sales_region"}, Reason: "catalog_term"})
	analyticalReseal(&a)
	a.metricPeriods[0].PackDigest = a.publications[0].Digest
	return a
}

func TestSQLRecoveryGroupedOwnedCompiler(t *testing.T) {
	for _, activity := range []bool{false, true} {
		a := groupedPeriodAdmission(t, activity)
		before := exec.Hash(a.publications)
		c, err := compileAnalyticalVersion(t.Context(), a, 10)
		if err != nil {
			t.Fatal("compile grouped period", activity, err)
		}
		if c.Version != exec.AnalyticalGroupedOwnedPopulationsVersion || c.ScalarPopulations != nil || c.GroupedPopulations == nil || len(c.GroupedPopulations.Lanes) != 2 || exec.Hash(a.publications) != before {
			t.Fatal("wrong grouped period contract")
		}
		lane := c.GroupedPopulations.Lanes[0]
		target := "sales"
		if activity {
			target = "refunds"
		}
		if lane.Dataset != "refunds" || lane.QueryPopulation.Constraints[0].Dataset != target || lane.Domain != exec.AnalyticalGroupDomainQualifying || len(lane.Joins) != 1 || len(lane.Joins[0].LeftColumns) != 2 {
			t.Fatal("lost authenticated period meaning")
		}
		guidance := analyticalJoinGuidance(c)
		if strings.Contains(guidance, "2026") || strings.Contains(guidance, "05:00") || strings.Contains(guidance, `"constraints"`) || !strings.Contains(guidance, "fact-owned period") {
			t.Fatal("private guidance or missing placement")
		}
		for _, v := range []int{7, 8, 9} {
			if _, err := compileAnalyticalVersion(t.Context(), a, v); err == nil {
				t.Fatal("retained version upgraded", v)
			}
		}
	}
}

func TestSQLRecoveryGroupedOwnedCompilerCustody(t *testing.T) {
	for _, change := range []func(*admission){
		func(a *admission) { a.metricPeriods = nil },
		func(a *admission) { a.metricPeriods[0].Bindings = a.metricPeriods[0].Bindings[:1] },
		func(a *admission) { a.metricPeriods[0].SourceBindingDigest = exec.Hash("stale") },
		func(a *admission) { a.binding.Source = "other-source" },
		func(a *admission) { a.binding.Context = "other-context" },
		func(a *admission) { a.binding.Revision++ },
		func(a *admission) { a.metricPeriods[0].Bindings[0].Population = "sales" },
		func(a *admission) { a.metricPeriods[0].Bindings[0].Constraint.Dataset = "refunds" },
		func(a *admission) {
			a.publications[0].Definition.GroupedPopulation = nil
			analyticalReseal(a)
			a.metricPeriods[0].PackDigest = a.publications[0].Digest
		},
	} {
		a := groupedPeriodAdmission(t, false)
		change(&a)
		if _, err := compileAnalyticalVersion(t.Context(), a, 10); err == nil {
			t.Fatal("unproved grouped period accepted")
		}
	}
	a := groupedPeriodAdmission(t, false)
	raw, _ := json.Marshal(a.route)
	_ = json.Unmarshal(raw, &a.route)
	a.metricPeriods = nil
	if _, err := compileCurrentAnalytical(context.Background(), a); err == nil {
		t.Fatal("wire route recreated application seal")
	}
}

func TestSQLRecoveryGroupedOwnedReceiptCustody(t *testing.T) {
	a := groupedPeriodAdmission(t, false)
	c, err := compileAnalyticalVersion(t.Context(), a, 10)
	if err != nil {
		t.Fatal(err)
	}
	q := QueryRecord{AnalyticalVersion: 10, Route: a.route, SQL: "bound SQL", Parameters: []exec.Parameter{{Kind: "text", Value: "private"}}}
	q.Analytical = &exec.AnalyticalReceipt{Version: c.Version, Scope: strings.ReplaceAll(exec.AnalyticalGrainScope, "single_base_relation", "independent_owned_grouped_populations"), Contract: exec.Hash(*c), Query: exec.AnalyticalQueryDigest(q.SQL, q.Parameters), Intent: exec.AnalyticalIntentPolicy, QueryPopulation: exec.AnalyticalQueryPopulationPolicy, Grouping: c.Grain.Dimensions}
	for _, m := range c.Metrics {
		q.Analytical.Metrics = append(q.Analytical.Metrics, m.ID)
		q.Analytical.Outputs = append(q.Analytical.Outputs, exec.AnalyticalOutput{Metric: m.ID, Column: 1})
	}
	q.Clarification = &ClarificationEvidence{SchemaVersion: 1, BaseSQL: "private-value-free SQL", Binding: exec.BusinessBindingReceipt{SchemaVersion: 3, PopulationPolicy: exec.AnalyticalGroupedOwnedPopulationPolicy, Bindings: []exec.BusinessParameterBinding{{Population: "refunds"}, {Population: "sales"}}}}
	if !AnalyticalRecordValid(q) || analyticalVersionForReceipt(q.Analytical) != 10 || !clarificationBindingSchemaValid(q) {
		t.Fatal("owned grouped record rejected")
	}
	if _, err := expectedAnalytical(t.Context(), q, a); err != nil {
		t.Fatal("contract reconstruction", err)
	}
	base, parameters, err := refinementSQLBase(q)
	if err != nil || base != q.Clarification.BaseSQL || len(parameters) != 0 {
		t.Fatal("private values reached refinement", err)
	}
	for _, v := range []int{7, 8, 9} {
		bad := q
		bad.AnalyticalVersion = v
		if AnalyticalRecordValid(bad) || clarificationBindingSchemaValid(bad) {
			t.Fatal("downgraded receipt accepted")
		}
	}
	for _, v := range []int{1, 2} {
		bad := q
		e := *q.Clarification
		bad.Clarification = &e
		e.Binding.SchemaVersion = v
		if clarificationBindingSchemaValid(bad) {
			t.Fatal("downgraded binding accepted")
		}
	}
	service := &Service{}
	if _, err := service.expectedAnalytical(t.Context(), testEnvelope(t), q, a); err == nil {
		t.Fatal("missing authenticated replay provider accepted")
	}
}

func TestSQLRecoveryGroupedOwnedRejectsUnreviewedGenericFilters(t *testing.T) {
	a := groupedPeriodAdmission(t, false)
	a.route.Resolutions = []semantics.ClarificationResolution{{Topic: "sales_topic", Pattern: "threshold", Slot: "amount"}}
	a.route.SourceBindingDigest = exec.Hash(a.binding)
	constraints := []exec.BusinessConstraint{{Resolution: exec.Hash("owned-extra-filter"), Dataset: "sales", Column: "amount_native", SourceRevision: 1, Kind: "number", Operator: "gte", Nulls: "exclude", Precision: 38, Scale: 6, Value: "10"}}
	if _, err := compileAnalyticalVersion(t.Context(), a, 10, constraints); err == nil {
		t.Fatal("generic filter acquired inferred fact/shared ownership")
	}
}

func TestSQLRecoveryGroupedOwnedCannotDropCompleteness(t *testing.T) {
	a := groupedPeriodAdmission(t, false)
	a.publications[0].Definition.Measures[0].Completeness = &semantics.KnownAmountCompleteness{Policy: semantics.KnownAmountCompletenessPolicy, UnknownCount: semantics.Reference{Kind: semantics.KindKPI, ID: "unknown"}}
	a.route.Selection.Topics[0].Roots = append(a.route.Selection.Topics[0].Roots, nlqroute.SelectedRoot{Reference: semantics.Reference{Kind: semantics.KindMeasure, ID: a.publications[0].Definition.Measures[0].ID}, Reason: "catalog_term"})
	analyticalReseal(&a)
	a.metricPeriods[0].PackDigest = a.publications[0].Digest
	for _, v := range []int{9, 10} {
		if _, err := compileAnalyticalVersion(t.Context(), a, v); err == nil {
			t.Fatal("mixed capability dropped completeness", v)
		}
	}
}

func TestSQLRecoveryGroupedOwnedLogicalGrain(t *testing.T) {
	a := groupedPeriodAdmission(t, false)
	c, err := compileAnalyticalVersion(t.Context(), a, 10)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := logicalGrouping(a, c.Grain)
	if err != nil || len(selection.Keys) != 1 || selection.Keys[0].Dimension != "sales_region" {
		t.Fatal("joined logical grain did not retain physical origin", err)
	}
	qualified := *c.Grain
	qualified.Columns = []string{"sales/region_native"}
	if selected, err := logicalGrouping(a, &qualified); err != nil || len(selected.Keys) != 1 || selected.Keys[0].Dimension != "sales_region" {
		t.Fatal("qualified reviewed grain denied", err)
	}
	bad := *c.Grain
	bad.Columns = []string{"other/region_native"}
	if _, err := logicalGrouping(a, &bad); err == nil {
		t.Fatal("foreign qualified field adopted reviewed dimension")
	}
}
