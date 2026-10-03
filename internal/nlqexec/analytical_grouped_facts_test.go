package nlqexec

import (
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
)

// A reviewed row predicate on sales.region_native cannot be a final group
// selection: this program groups on customers.region, and refunds is a separate
// selected aggregate population. The service must restrict only the sales lane.
func groupedFactAdmission(t *testing.T) (admission, []exec.BusinessConstraint) {
	t.Helper()
	a, constraints := selectedGroupAdmission(t, false)
	a.publications[0].Definition.GroupedPopulation.GroupDomains = []semantics.GroupedPopulationDomain{
		{Dataset: "refunds", Domain: exec.AnalyticalGroupDomainRaw},
		{Dataset: "sales", Domain: exec.AnalyticalGroupDomainRaw},
	}
	for i := range a.publications[0].Definition.Joins {
		a.publications[0].Definition.Joins[i].Type = semantics.JoinInner
	}
	analyticalReseal(&a)
	constraints[0].Dataset, constraints[0].Column = "sales", "region_native"
	constraints[0].Value = "REVIEWED_SALES_SEGMENT"
	return a, constraints
}

func TestSQLRecoveryGroupedFactCompilerPlacementRegression(t *testing.T) {
	a, constraints := groupedFactAdmission(t)
	// The current compiler preserves old receipt versions while dispatching this
	// newly supported fact predicate to its distinct v12 proof.
	c, err := compileAnalyticalVersion(t.Context(), a, 12, constraints)
	if err != nil {
		t.Fatalf("reviewed fact-row predicate needs service-owned sales-lane placement: %v", err)
	}
	if c.Version != exec.AnalyticalGroupedFactsVersion || c.GroupedPopulations == nil || c.GroupSelection != nil {
		t.Fatal("fact restriction became a final group selection")
	}
}

// The original LEFT-join red remains an explicit unsupported boundary. It is
// not relabeled as the now-supported INNER-only increment.
func TestSQLRecoveryGroupedFactLeftRemainsUnsupported(t *testing.T) {
	a, c := groupedFactAdmission(t)
	for i := range a.publications[0].Definition.Joins {
		a.publications[0].Definition.Joins[i].Type = semantics.JoinLeft
	}
	analyticalReseal(&a)
	if _, err := compileAnalyticalVersion(t.Context(), a, 12, c); err == nil {
		t.Fatal("outer-join fact target gained an unproved binder capability")
	}
}

func TestSQLRecoveryGroupedFactCompilerBoundaries(t *testing.T) {
	for name, change := range map[string]func(*admission, []exec.BusinessConstraint){
		"source":             func(a *admission, c []exec.BusinessConstraint) { a.binding.Source = "other" },
		"context":            func(a *admission, c []exec.BusinessConstraint) { a.binding.Context = "other" },
		"revision":           func(a *admission, c []exec.BusinessConstraint) { a.binding.Revision++ },
		"predicate_revision": func(a *admission, c []exec.BusinessConstraint) { c[0].SourceRevision++ },
		"evidence":           func(a *admission, c []exec.BusinessConstraint) { a.route.Resolutions = nil },
		"aggregate":          func(a *admission, c []exec.BusinessConstraint) { c[0].Aggregation = "count" },
		"domain": func(a *admission, c []exec.BusinessConstraint) {
			a.publications[0].Definition.GroupedPopulation.GroupDomains = nil
			analyticalReseal(a)
		},
		"scope": func(a *admission, c []exec.BusinessConstraint) { a.relationScope = a.relationScope[1:] },
	} {
		t.Run(name, func(t *testing.T) {
			a, c := groupedFactAdmission(t)
			change(&a, c)
			if _, err := compileAnalyticalVersion(t.Context(), a, 12, c); err == nil {
				t.Fatal("unproved fact compiler custody")
			}
		})
	}
	a, c := groupedFactAdmission(t)
	for _, v := range []int{7, 8, 9, 10, 11} {
		if _, err := compileAnalyticalVersion(t.Context(), a, v, c); err == nil {
			t.Fatal("old policy acquired fact predicate", v)
		}
	}
	a, c = selectedGroupAdmission(t, false)
	old, err := compileAnalyticalVersion(t.Context(), a, 11, c)
	if err != nil {
		t.Fatal(err)
	}
	current, err := compileAnalyticalVersion(t.Context(), a, 12, c)
	if err != nil || exec.Hash(old) != exec.Hash(current) {
		t.Fatal("pure group selection lost immutable v11 policy", err)
	}
}

func TestSQLRecoveryGroupedFactCompilerMixedPlacement(t *testing.T) {
	a, c := groupedFactAdmission(t)
	group := c[0]
	group.Resolution = exec.Hash("final-region")
	group.Dataset = "customers"
	group.Column = "region"
	group.Value = "PRIVATE_FINAL_GROUP"
	c = append(c, group)
	before := exec.Hash(a.publications)
	contract, err := compileAnalyticalVersion(t.Context(), a, 12, c)
	if err != nil {
		t.Fatal(err)
	}
	if contract.GroupSelection == nil || len(contract.GroupSelection.Constraints) != 1 || contract.GroupSelection.Constraints[0].Dataset != "customers" || exec.Hash(a.publications) != before {
		t.Fatal("mixed selection mutated or merged populations")
	}
	facts := 0
	for _, lane := range contract.GroupedPopulations.Lanes {
		if lane.FactPopulation != nil {
			facts++
			if lane.Dataset != "sales" || lane.FactPopulation.Constraints[0].Column != "region_native" {
				t.Fatal("wrong fact owner")
			}
		}
	}
	if facts != 1 {
		t.Fatal("fact predicate copied to other population")
	}
	c[0].Value = "changed"
	for _, lane := range contract.GroupedPopulations.Lanes {
		if lane.FactPopulation != nil && lane.FactPopulation.Constraints[0].Value == "changed" {
			t.Fatal("caller mutated compiled population")
		}
	}
	guidance := analyticalGrainGuidance(contract)
	if strings.Contains(guidance, "REVIEWED_SALES_SEGMENT") || strings.Contains(guidance, "PRIVATE_FINAL_GROUP") || strings.Contains(guidance, `"fact_population"`) {
		t.Fatal("private fact data reached provider guidance")
	}
}

func TestSQLRecoveryGroupedFactRecordCustody(t *testing.T) {
	a, constraints := groupedFactAdmission(t)
	c, err := compileAnalyticalVersion(t.Context(), a, 12, constraints)
	if err != nil {
		t.Fatal(err)
	}
	q := QueryRecord{AnalyticalVersion: 12, Route: a.route, SQL: "bound fact SQL", Parameters: []exec.Parameter{{Kind: "text", Value: constraints[0].Value}}}
	q.Analytical = &exec.AnalyticalReceipt{Version: c.Version, Scope: strings.ReplaceAll(exec.AnalyticalGrainScope, "single_base_relation", "independent_filtered_grouped_populations"), Contract: exec.Hash(*c), Query: exec.AnalyticalQueryDigest(q.SQL, q.Parameters), Intent: exec.AnalyticalIntentPolicy, QueryPopulation: exec.AnalyticalQueryPopulationPolicy, Grouping: c.Grain.Dimensions}
	for _, m := range c.Metrics {
		q.Analytical.Metrics = append(q.Analytical.Metrics, m.ID)
		q.Analytical.Outputs = append(q.Analytical.Outputs, exec.AnalyticalOutput{Metric: m.ID, Column: 1})
	}
	q.Clarification = &ClarificationEvidence{SchemaVersion: 1, BaseSQL: "value-free original base", Binding: exec.BusinessBindingReceipt{SchemaVersion: 5, PopulationPolicy: exec.AnalyticalGroupedFactPolicy, Bindings: []exec.BusinessParameterBinding{{Population: "sales", Dataset: "sales", Column: "region_native"}}}}
	if !AnalyticalRecordValid(q) || !clarificationBindingSchemaValid(q) || analyticalVersionForReceipt(q.Analytical) != 12 {
		t.Fatal("valid fact record shape rejected")
	}
	if _, err := expectedAnalytical(t.Context(), q, a, constraints); err != nil {
		t.Fatal("fact receipt reconstruction", err)
	}
	if base, params, err := refinementSQLBase(q); err != nil || base != q.Clarification.BaseSQL || len(params) != 0 {
		t.Fatal("private fact values reached edit context", err)
	}
	for _, v := range []int{1, 2, 3, 4, 7, 8, 9, 10, 11} {
		bad := q
		bad.AnalyticalVersion = v
		if AnalyticalRecordValid(bad) || clarificationBindingSchemaValid(bad) {
			t.Fatal("fact record downgraded", v)
		}
	}
	for _, schema := range []int{1, 2, 3, 4, 6} {
		bad := q
		copy := *q.Clarification
		bad.Clarification = &copy
		copy.Binding.SchemaVersion = schema
		if clarificationBindingSchemaValid(bad) {
			t.Fatal("fact binder family substituted", schema)
		}
	}
	if learningPolicyForBinding(5) != "" {
		t.Fatal("unqualified fact learning policy")
	}
	if _, err := (&Service{}).expectedAnalytical(t.Context(), testEnvelope(t), q, a); err == nil {
		t.Fatal("saved JSON replaced authenticated replay")
	}
}
