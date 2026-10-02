package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
)

func selectedGroupAdmission(t *testing.T, periods bool) (admission, []exec.BusinessConstraint) {
	t.Helper()
	a := groupedAdmission()
	dataset, column := "customers", "region"
	if periods {
		a = groupedPeriodAdmission(t, false)
		dataset, column = "sales", "region_native"
	}
	a.binding.Tenant, a.binding.Contract, a.binding.Fingerprint = "tenant", "contract", exec.Hash("selected-group-source")
	if periods {
		a.metricPeriods[0].SourceBindingDigest = exec.Hash(a.binding)
	}
	a.relationScope = nil
	for _, r := range a.binding.Relations {
		scope := exec.RelationScope{Dataset: r.ID}
		for _, c := range r.Columns {
			scope.Columns = append(scope.Columns, c.Name)
		}
		a.relationScope = append(a.relationScope, scope)
	}
	a.route.Resolutions = []semantics.ClarificationResolution{{Topic: "sales_topic", Pattern: "group", Slot: "region"}}
	a.route.SourceBindingDigest = exec.Hash(a.binding)
	return a, []exec.BusinessConstraint{{Resolution: exec.Hash("private selection"), Dataset: dataset, Column: column, SourceRevision: 1, Kind: "text", Operator: "eq", Nulls: "exclude", Value: "PRIVATE_SELECTED_GROUP"}}
}

func TestSQLRecoveryGroupedSelectionCompiler(t *testing.T) {
	for _, periods := range []bool{false, true} {
		a, constraints := selectedGroupAdmission(t, periods)
		before := exec.Hash(a.publications)
		c, err := compileAnalyticalVersion(t.Context(), a, 11, constraints)
		if err != nil {
			t.Fatal("compile selection", periods, err)
		}
		if c.Version != exec.AnalyticalGroupedSelectionVersion || c.GroupedPopulations == nil || c.GroupSelection == nil || len(c.GroupSelection.Constraints) != 1 || len(c.QueryPopulation.Constraints) != 0 || exec.Hash(a.publications) != before {
			t.Fatal("selection changed independent lane meaning")
		}
		for _, lane := range c.GroupedPopulations.Lanes {
			if (lane.QueryPopulation != nil) != periods {
				t.Fatal("fact period custody changed")
			}
		}
		guidance := analyticalGrainGuidance(c)
		if strings.Contains(guidance, "PRIVATE_SELECTED_GROUP") || strings.Contains(guidance, "05:00") || strings.Contains(guidance, `"constraints"`) || !strings.Contains(guidance, "final key-spine") || strings.Contains(guidance, "CTE or derived SELECT") {
			t.Fatal("private or ambiguous grouped guidance")
		}
		constraints[0].Value = "changed"
		if c.GroupSelection.Constraints[0].Value != "PRIVATE_SELECTED_GROUP" {
			t.Fatal("caller changed compiled selection")
		}
		for _, v := range []int{7, 8, 9, 10} {
			if _, err := compileAnalyticalVersion(t.Context(), a, v, constraints); err == nil {
				t.Fatal("prior version gained shared selection", v)
			}
		}
		raw, _ := json.Marshal(a.route)
		_ = json.Unmarshal(raw, &a.route)
		if _, err := compileCurrentAnalytical(t.Context(), a); err == nil {
			t.Fatal("wire route replaced live seal")
		}
	}
	// An ordinary query still has the original v8 query-wide population proof.
	a, constraints := populationAdmission()
	a.relationScope = []exec.RelationScope{{Dataset: "sales", Columns: []string{"amount_native", "id_native", "region_native"}}}
	got, err := compileAnalyticalVersion(t.Context(), a, 11, constraints)
	want, wantErr := compileAnalyticalVersion(t.Context(), a, 8, constraints)
	if err != nil || wantErr != nil || exec.Hash(got) != exec.Hash(want) {
		t.Fatal("ordinary query changed policy", err, wantErr)
	}
	if analyticalDiagnostic(&exec.AnalyticalError{Code: "analytical_group_selection_unsupported", Unsupported: true}) != "analytical_group_selection_unsupported" {
		t.Fatal("missing closed diagnostic")
	}
}

func TestSQLRecoveryGroupedSelectionCompilerCustody(t *testing.T) {
	for name, change := range map[string]func(*admission, []exec.BusinessConstraint){
		"stale source":    func(a *admission, c []exec.BusinessConstraint) { a.binding.Revision++ },
		"foreign context": func(a *admission, c []exec.BusinessConstraint) { a.binding.Context = "other" },
		"foreign source":  func(a *admission, c []exec.BusinessConstraint) { a.binding.Source = "other" },
		"no evidence":     func(a *admission, c []exec.BusinessConstraint) { a.route.Resolutions = nil },
		"non group field": func(a *admission, c []exec.BusinessConstraint) {
			c[0].Column = "id"
			c[0].Kind = "number"
			c[0].Precision = 38
			c[0].Value = "1"
		},
		"aggregate filter": func(a *admission, c []exec.BusinessConstraint) { c[0].Aggregation = "count" },
		"foreign scope":    func(a *admission, c []exec.BusinessConstraint) { a.relationScope = a.relationScope[:2] },
		"extra dialect": func(a *admission, c []exec.BusinessConstraint) {
			a.binding.Dialect = "mysql"
			a.route.SourceBindingDigest = exec.Hash(a.binding)
		},
		"stale predicate": func(a *admission, c []exec.BusinessConstraint) { c[0].SourceRevision++ },
	} {
		t.Run(name, func(t *testing.T) {
			a, c := selectedGroupAdmission(t, false)
			change(&a, c)
			if _, err := compileAnalyticalVersion(t.Context(), a, 11, c); err == nil {
				t.Fatal("unproved shared selection admitted")
			}
		})
	}
	a, c := selectedGroupAdmission(t, false)
	for _, supplied := range [][][]exec.BusinessConstraint{nil, {{}}, {c, c}} {
		if _, err := compileAnalyticalVersion(t.Context(), a, 11, supplied...); err == nil {
			t.Fatal("missing or multiple predicate custody accepted")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := compileAnalyticalVersion(ctx, a, 11, c); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
}

func TestSQLRecoveryGroupedSelectionReceiptCustody(t *testing.T) {
	for _, periods := range []bool{false, true} {
		a, constraints := selectedGroupAdmission(t, periods)
		c, err := compileAnalyticalVersion(t.Context(), a, 11, constraints)
		if err != nil {
			t.Fatal(err)
		}
		q := QueryRecord{AnalyticalVersion: 11, Route: a.route, SQL: "bound SQL", Parameters: []exec.Parameter{{Kind: "text", Value: "PRIVATE_SELECTED_GROUP"}}}
		q.Analytical = &exec.AnalyticalReceipt{Version: c.Version, Scope: strings.ReplaceAll(exec.AnalyticalGrainScope, "single_base_relation", "independent_selected_grouped_populations"), Contract: exec.Hash(*c), Query: exec.AnalyticalQueryDigest(q.SQL, q.Parameters), Intent: exec.AnalyticalIntentPolicy, QueryPopulation: exec.AnalyticalQueryPopulationPolicy, Grouping: c.Grain.Dimensions}
		for _, m := range c.Metrics {
			q.Analytical.Metrics = append(q.Analytical.Metrics, m.ID)
			q.Analytical.Outputs = append(q.Analytical.Outputs, exec.AnalyticalOutput{Metric: m.ID, Column: 1})
		}
		q.Clarification = &ClarificationEvidence{SchemaVersion: 1, BaseSQL: "value-free base", Binding: exec.BusinessBindingReceipt{SchemaVersion: 4, PopulationPolicy: exec.AnalyticalGroupedSelectionPolicy, Bindings: []exec.BusinessParameterBinding{{Dataset: constraints[0].Dataset, Column: constraints[0].Column}}}}
		if periods {
			q.Clarification.Binding.Bindings = append(q.Clarification.Binding.Bindings, exec.BusinessParameterBinding{Population: "refunds"}, exec.BusinessParameterBinding{Population: "sales"})
		}
		if !AnalyticalRecordValid(q) || !clarificationBindingSchemaValid(q) || analyticalVersionForReceipt(q.Analytical) != 11 {
			t.Fatal("valid v11 shape rejected")
		}
		if _, err := expectedAnalytical(t.Context(), q, a, constraints); err != nil {
			t.Fatal("reconstruction", err)
		}
		if base, params, err := refinementSQLBase(q); err != nil || base != q.Clarification.BaseSQL || len(params) != 0 {
			t.Fatal("private binding reached refinement", err)
		}
		if _, err := (&Service{}).expectedAnalytical(t.Context(), testEnvelope(t), q, a); err == nil {
			t.Fatal("missing authenticated router accepted")
		}
		for _, v := range []int{1, 2, 3, 7, 8, 9, 10} {
			bad := q
			bad.AnalyticalVersion = v
			if AnalyticalRecordValid(bad) || clarificationBindingSchemaValid(bad) {
				t.Fatal("version downgrade accepted", v)
			}
		}
		for _, v := range []int{1, 2, 3} {
			bad := q
			copy := *q.Clarification
			bad.Clarification = &copy
			copy.Binding.SchemaVersion = v
			if clarificationBindingSchemaValid(bad) {
				t.Fatal("binding schema downgrade accepted", v)
			}
		}
		bad := q
		bad.Analytical = cloneAnalyticalReceipt(q.Analytical)
		bad.Analytical.Grouping = nil
		if AnalyticalRecordValid(bad) {
			t.Fatal("group selection lost grouping")
		}
	}
}
