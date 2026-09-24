package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
)

func populationAdmission() (admission, []exec.BusinessConstraint) {
	a := analyticalAdmission()
	a.binding.Tenant = "tenant"
	a.binding.Contract = "contract"
	a.binding.Fingerprint = exec.Hash("synthetic-source")
	a.route.Resolutions = []semantics.ClarificationResolution{{Topic: "sales_topic", Pattern: "threshold", Slot: "amount"}}
	a.route.SourceBindingDigest = exec.Hash(a.binding)
	constraints := []exec.BusinessConstraint{{Resolution: exec.Hash("reviewed-threshold"), Dataset: "sales", Column: "amount_native", SourceRevision: 1, Kind: "number", Operator: "gte", Nulls: "exclude", Unit: "USD", Precision: 38, Scale: 6, Value: "9007199254740993.125"}}
	return a, constraints
}

func TestSQLRecoveryQueryPopulationCompilerRequiresOwnedConstraints(t *testing.T) {
	a, constraints := populationAdmission()
	// Serialized resolutions are not an in-process router seal.
	if c, err := compileCurrentAnalytical(context.Background(), a); err == nil || c != nil {
		t.Fatal("unsealed route fabricated a query-population proof")
	}
	c, err := compileAnalyticalVersion(context.Background(), a, 4, constraints)
	if err != nil || c == nil || c.QueryPopulation == nil || c.Version != exec.AnalyticalQueryPopulationVersion {
		t.Fatal("verified constraints were not compiled", err)
	}
	before := exec.Hash(c.QueryPopulation)
	constraints[0].Value = "1"
	if before != exec.Hash(c.QueryPopulation) {
		t.Fatal("caller mutation changed the compiled population")
	}
	if strings.Contains(analyticalPopulationGuidance(c), "9007199254740993.125") || !strings.Contains(analyticalPopulationGuidance(c), "WHERE") {
		t.Fatal("guidance exposed values or omitted the policy")
	}
	for _, mutate := range []func(*admission){
		func(a *admission) { a.route.SourceBindingDigest = exec.Hash("other") },
		func(a *admission) { a.route.Resolutions = nil },
	} {
		a, constraints := populationAdmission()
		mutate(&a)
		if _, err := compileAnalyticalVersion(context.Background(), a, 4, constraints); !errors.Is(err, exec.ErrBinding) {
			t.Fatal("foreign or unsolicited predicate set accepted", err)
		}
	}
}

func TestSQLRecoveryQueryPopulationRetainedReplayAndEvidence(t *testing.T) {
	a, constraints := populationAdmission()
	for _, version := range []int{1, 2, 3} {
		c, err := compileAnalyticalVersion(context.Background(), a, version)
		if err != nil || c == nil || c.QueryPopulation != nil {
			t.Fatal("retained query acquired a new population claim", version, err)
		}
	}
	c, err := compileAnalyticalVersion(context.Background(), a, 4, constraints)
	if err != nil {
		t.Fatal(err)
	}
	q := QueryRecord{AnalyticalVersion: 4, Route: a.route, SQL: "SELECT sum(amount_native)/NULLIF(count(DISTINCT id_native),0) FROM analytics.sales WHERE amount_native >= $1", Parameters: []exec.Parameter{{Kind: "decimal", Value: constraints[0].Value}}}
	q.Analytical = &exec.AnalyticalReceipt{Version: c.Version, Scope: exec.AnalyticalMetricScope, Contract: exec.Hash(*c), Query: exec.AnalyticalQueryDigest(q.SQL, q.Parameters), Metrics: []string{c.Metrics[0].ID}, QueryPopulation: exec.AnalyticalQueryPopulationPolicy}
	if !AnalyticalRecordValid(q) {
		t.Fatal("new receipt shape rejected")
	}
	actual, err := expectedAnalytical(context.Background(), q, a, constraints)
	if err != nil || !reflect.DeepEqual(actual, c) {
		t.Fatal("reconstructed population differs", err)
	}
	if _, err := (&Service{}).expectedAnalytical(context.Background(), identity.Envelope{}, q, a); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("persisted JSON replaced authenticated router replay", err)
	}
	for _, mutate := range []func(*QueryRecord){
		func(q *QueryRecord) { q.Analytical.QueryPopulation = "" },
		func(q *QueryRecord) { q.Analytical.QueryPopulation = "unknown" },
		func(q *QueryRecord) { q.AnalyticalVersion = 3; q.Analytical.Version = exec.AnalyticalCalendarVersion },
		func(q *QueryRecord) { q.Analytical.Contract = exec.Hash("other-predicates") },
	} {
		bad := q
		bad.Analytical = cloneAnalyticalReceipt(q.Analytical)
		mutate(&bad)
		if _, err := expectedAnalytical(context.Background(), bad, a, constraints); err == nil {
			t.Fatal("population receipt removed or substituted")
		}
	}
	wire, _ := json.Marshal(q.Analytical)
	if strings.Contains(string(wire), constraints[0].Value) {
		t.Fatal("public population receipt leaked a scalar")
	}
}

func TestSQLRecoveryQueryPopulationAbsentIsNotTotalPopulation(t *testing.T) {
	a := analyticalAdmission()
	c, err := compileCurrentAnalytical(context.Background(), a)
	if err != nil || c == nil || c.QueryPopulation != nil || analyticalPopulationGuidance(c) != "" {
		t.Fatal("unresolved population became an exhaustive predicate claim", err)
	}
	a, _ = populationAdmission()
	c, err = compileAnalyticalVersion(context.Background(), a, 4, []exec.BusinessConstraint{})
	if err != nil || c == nil || c.QueryPopulation != nil {
		t.Fatal("reference-only answer acquired a row-population proof", err)
	}
	if code := analyticalDiagnostic(&exec.AnalyticalError{Code: "analytical_query_population_mismatch"}); code != "analytical_query_population_mismatch" {
		t.Fatal("lost targeted repair diagnostic")
	}
	if code := analyticalDiagnostic(&exec.AnalyticalError{Code: "private-value-731"}); strings.Contains(code, "731") {
		t.Fatal("unreviewed diagnostic leaked")
	}
}
