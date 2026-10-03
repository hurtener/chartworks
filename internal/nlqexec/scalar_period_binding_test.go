package nlqexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
)

func TestSQLRecoveryScopedReceiptVersionCustody(t *testing.T) {
	q := QueryRecord{Analytical: &exec.AnalyticalReceipt{Scope: strings.ReplaceAll(exec.AnalyticalMetricScope, "single_base_relation", "independent_scoped_singleton_populations")}, AnalyticalVersion: 9, SQL: "private bound SQL", Parameters: []exec.Parameter{{Kind: "text", Value: "PRIVATE_CURRENT_INTERVAL"}}, Clarification: &ClarificationEvidence{SchemaVersion: 1, BaseSQL: "value-free model base", Binding: exec.BusinessBindingReceipt{SchemaVersion: 2, PopulationPolicy: exec.AnalyticalScalarPopulationPolicy, Bindings: []exec.BusinessParameterBinding{{Population: "orders"}, {Population: "refunds"}}}}}
	if !clarificationBindingSchemaValid(q) {
		t.Fatal("scoped receipt rejected")
	}
	base, parameters, err := refinementSQLBase(q)
	if err != nil || base != q.Clarification.BaseSQL || len(parameters) != 0 {
		t.Fatal("bound values became refinement base", err)
	}
	for _, change := range []func(*QueryRecord){
		func(q *QueryRecord) { q.AnalyticalVersion = 8 },
		func(q *QueryRecord) { q.Clarification.Binding.SchemaVersion = 1 },
		func(q *QueryRecord) { q.Clarification.Binding.PopulationPolicy = "" },
		func(q *QueryRecord) { q.Clarification.Binding.Bindings = nil },
	} {
		copy := q
		e := *q.Clarification
		copy.Clarification = &e
		change(&copy)
		if clarificationBindingSchemaValid(copy) {
			t.Fatal("receipt downgraded or placement dropped")
		}
	}
	legacy := q
	legacy.AnalyticalVersion = 8
	e := *q.Clarification
	legacy.Clarification = &e
	e.Binding = exec.BusinessBindingReceipt{SchemaVersion: 1}
	if !clarificationBindingSchemaValid(legacy) {
		t.Fatal("legacy receipt1 no longer accepted")
	}
	service := &Service{}
	if _, _, err := service.scalarPeriodAdmission(context.Background(), testEnvelope(t), q, admission{}); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("missing replay provider accepted", err)
	}
}

func TestSQLRecoveryScopedAnalyticalRecord(t *testing.T) {
	a := scalarPeriodAdmission(t, false)
	c, err := compileAnalyticalVersion(context.Background(), a, 9)
	if err != nil {
		t.Fatal(err)
	}
	q := QueryRecord{AnalyticalVersion: 9, Route: a.route, SQL: "SELECT reviewed scalar population outputs"}
	r := &exec.AnalyticalReceipt{Version: exec.AnalyticalScopedPopulationsVersion, Scope: strings.ReplaceAll(exec.AnalyticalMetricScope, "single_base_relation", "independent_scoped_singleton_populations"), Contract: exec.Hash(*c), Query: exec.AnalyticalQueryDigest(q.SQL, nil), Intent: exec.AnalyticalIntentPolicy, QueryPopulation: exec.AnalyticalQueryPopulationPolicy}
	for _, m := range c.Metrics {
		r.Metrics = append(r.Metrics, m.ID)
		r.Outputs = append(r.Outputs, exec.AnalyticalOutput{Metric: m.ID, Column: 0})
	}
	q.Analytical = r
	if !AnalyticalRecordValid(q) || analyticalVersionForReceipt(r) != 9 {
		t.Fatal("scoped record rejected")
	}
	if _, err := expectedAnalytical(context.Background(), q, a); err != nil {
		t.Fatal("scoped receipt reconstruction", err)
	}
	for _, change := range []func(*QueryRecord){
		func(q *QueryRecord) { q.AnalyticalVersion = 8 },
		func(q *QueryRecord) { q.Analytical.Version = exec.AnalyticalGroupedProgramsVersion },
		func(q *QueryRecord) { q.Analytical.Scope = exec.AnalyticalMetricScope },
		func(q *QueryRecord) { q.Analytical.Intent = "" },
		func(q *QueryRecord) { q.Analytical.QueryPopulation = "" },
	} {
		copy := q
		copy.Analytical = cloneAnalyticalReceipt(q.Analytical)
		change(&copy)
		if AnalyticalRecordValid(copy) {
			t.Fatal("scoped receipt version/policy confusion")
		}
	}
}
