package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func analyticalAdmission() admission {
	columns := []semantics.Column{
		{ID: "amount", SourceName: "amount_native", NativeType: "numeric", Category: "numeric", Nullable: true, Sensitivity: semantics.LiteralNonSensitive},
		{ID: "id", SourceName: "id_native", NativeType: "int4", Category: "numeric"},
		{ID: "region", SourceName: "region_native", NativeType: "text", Category: "text", Sensitivity: semantics.LiteralNonSensitive},
	}
	binding := exec.Binding{Dialect: "postgres", Source: "source", Context: "context", Revision: 1, Relations: []exec.Relation{{ID: "sales", Schema: "analytics", Name: "sales"}}}
	for _, col := range columns {
		binding.Relations[0].Columns = append(binding.Relations[0].Columns, exec.Column{Name: col.SourceName, NativeType: col.NativeType, Category: col.Category, Nullable: col.Nullable, Safe: true})
	}
	def := topics.Definition{SchemaVersion: 1, Topic: "sales_topic", Version: "v1", Datasets: []topics.Dataset{{ID: "sales", Source: topics.Binding{Source: "source", Context: "context", Dataset: "sales", SourceRevision: 1}, Columns: columns}}, Measures: []semantics.Measure{
		{ID: "revenue", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "amount"}, Aggregation: semantics.AggregationSum},
		{ID: "orders", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "id"}, Aggregation: semantics.AggregationDistinctCount},
	}, KPIs: []semantics.KPI{{ID: "ratio", Expression: "revenue / orders", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}, {Kind: semantics.KindMeasure, ID: "orders"}}}}}
	pub := topics.Published{Definition: def, Digest: exec.Hash(def), State: topics.State{Topic: def.Topic, Version: def.Version, Active: true}}
	selected := &nlqroute.SemanticSelection{Version: "catalog-selection-v1", Topics: []nlqroute.SelectedTopic{{Topic: def.Topic, TopicVersion: def.Version, PackDigest: pub.Digest, Roots: []nlqroute.SelectedRoot{{Reference: semantics.Reference{Kind: semantics.KindKPI, ID: "ratio"}, Reason: "catalog_term"}}}}}
	selected.Digest = exec.Hash(selected)
	return admission{binding: binding, publications: []topics.Published{pub}, route: nlqroute.RouteResult{Selection: selected}}
}
func analyticalReseal(a *admission) {
	for i := range a.publications {
		a.publications[i].Digest = exec.Hash(a.publications[i].Definition)
		a.route.Selection.Topics[i].PackDigest = a.publications[i].Digest
	}
	a.route.Selection.Digest = ""
	a.route.Selection.Digest = exec.Hash(a.route.Selection)
}
func TestSQLRecoveryAnalyticalCatalogCompilation(t *testing.T) {
	a := analyticalAdmission()
	before, _ := json.Marshal(a.publications)
	contract, err := compileAnalytical(context.Background(), a)
	if err != nil || contract == nil || contract.Dataset != "sales" || len(contract.Metrics) != 1 {
		t.Fatal("compile", err)
	}
	x := contract.Metrics[0].Expression
	if x.Op != "/" || len(x.Args) != 2 || x.Args[0].Op != "sum" || x.Args[0].Column != "amount_native" || x.Args[1].Op != "distinct_count" || x.Args[1].Column != "id_native" {
		t.Fatal("incorrect typed dependency compilation")
	}
	after, _ := json.Marshal(a.publications)
	if string(before) != string(after) {
		t.Fatal("mutated publication")
	}
	// Each metric's own population remains on its aggregate, not a common WHERE.
	for i := range a.publications[0].Definition.Measures {
		a.publications[0].Definition.Measures[i].Filters = []semantics.SemanticFilter{{ID: "population", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "region"}, Operator: "eq", Values: []string{[]string{"A", "B"}[i]}}}
	}
	analyticalReseal(&a)
	contract, err = compileAnalytical(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	args := contract.Metrics[0].Expression.Args
	if args[0].Filters[0].Values[0] != "A" || args[1].Filters[0].Values[0] != "B" || args[0].Filters[0].Column != "region_native" {
		t.Fatal("collapsed different populations")
	}
}
func TestSQLRecoveryAnalyticalCompilationRejectsForeignOrUnsupported(t *testing.T) {
	for _, mutate := range []func(*admission){
		func(a *admission) { a.publications[0].Definition.KPIs[0].Expression = "revenue divided by orders" },
		func(a *admission) { a.publications[0].Definition.KPIs[0].Expression = "unknown / orders" },
		func(a *admission) { a.publications[0].Definition.KPIs[0].Expression = "revenue" },
		func(a *admission) { a.publications[0].Definition.KPIs[0].Expression = "revenue * 1e99999999 + orders" },
		func(a *admission) {
			a.publications[0].Definition.KPIs[0].Inputs[0] = semantics.Reference{Kind: semantics.KindKPI, ID: "ratio"}
		},
		func(a *admission) { a.publications[0].Definition.Measures[0].Field.ID = "missing" },
		func(a *admission) { a.binding.Relations[0].Columns[0].Safe = false },
		func(a *admission) { a.binding.Revision++ },
		func(a *admission) { a.binding.Dialect = "mysql" },
	} {
		a := analyticalAdmission()
		mutate(&a)
		analyticalReseal(&a)
		if c, err := compileAnalytical(context.Background(), a); err == nil || c != nil {
			t.Fatal("unsupported/foreign contract accepted")
		}
	}
	a := analyticalAdmission()
	a.route.Selection.Topics[0].PackDigest = exec.Hash("wrong-publication")
	a.route.Selection.Digest = ""
	a.route.Selection.Digest = exec.Hash(a.route.Selection)
	if _, err := compileAnalytical(context.Background(), a); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("wrong pin", err)
	}
}
func TestSQLRecoveryAnalyticalRequiredDependenciesAreNotOutputRoots(t *testing.T) {
	a := analyticalAdmission()
	a.route.Selection.Topics[0].Roots = append(a.route.Selection.Topics[0].Roots, nlqroute.SelectedRoot{Reference: semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}, Reason: "required_rule"})
	analyticalReseal(&a)
	c, err := compileAnalytical(context.Background(), a)
	if err != nil || len(c.Metrics) != 1 {
		t.Fatal("required ingredient became extra output", err)
	}
	a.route.Selection.Topics[0].Roots[0].Reason = "required_rule"
	analyticalReseal(&a)
	if c, err = compileAnalytical(context.Background(), a); err != nil || c != nil {
		t.Fatal("invented user-selected metric", err)
	}
}
func TestSQLRecoveryAnalyticalPrivateDefinitionsAndCancellation(t *testing.T) {
	a := analyticalAdmission()
	a.publications[0].Definition.Datasets[0].Columns[2].Sensitivity = semantics.LiteralSensitive
	a.publications[0].Definition.Measures[0].Filters = []semantics.SemanticFilter{{Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "region"}, Operator: "eq", Values: []string{"private-canary"}}}
	analyticalReseal(&a)
	if _, err := compileAnalytical(context.Background(), a); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("private population literal", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := compileAnalytical(ctx, analyticalAdmission()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestSQLRecoveryAnalyticalReplayEvidenceIntegrity(t *testing.T) {
	a := analyticalAdmission()
	c, err := compileAnalytical(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	q := QueryRecord{AnalyticalVersion: 1, Route: a.route, SQL: "SELECT sum(amount_native)/NULLIF(count(DISTINCT id_native),0) FROM analytics.sales"}
	q.Analytical = &exec.AnalyticalReceipt{Version: exec.AnalyticalVersion, Scope: "selected_metric_expression_and_population;single_base_relation", Contract: exec.Hash(*c), Query: exec.AnalyticalQueryDigest(q.SQL, nil), Metrics: []string{c.Metrics[0].ID}}
	if !AnalyticalRecordValid(q) {
		t.Fatal("record shape")
	}
	if actual, err := expectedAnalytical(context.Background(), q, a); err != nil || !reflect.DeepEqual(c, actual) {
		t.Fatal("receipt reconstruction", err)
	}
	q.Parameters = []exec.Parameter{}
	if _, err := expectedAnalytical(context.Background(), q, a); err != nil {
		t.Fatal("nil vs empty normalization", err)
	}
	for _, change := range []func(*QueryRecord){func(q *QueryRecord) { q.Analytical = nil }, func(q *QueryRecord) { q.SQL = "SELECT 0" }, func(q *QueryRecord) { q.AnalyticalVersion = 0 }, func(q *QueryRecord) { q.Analytical.Query = exec.Hash("other-query") }, func(q *QueryRecord) { q.Analytical.Contract = exec.Hash("other-metric") }, func(q *QueryRecord) { q.Analytical.Metrics[0] = "wrong" }} {
		mutated := q
		mutated.Analytical = cloneAnalyticalReceipt(q.Analytical)
		change(&mutated)
		if _, err := expectedAnalytical(context.Background(), mutated, a); err == nil {
			t.Fatal("substituted/dropped evidence accepted")
		}
	}
	legacy := q
	legacy.AnalyticalVersion = 0
	legacy.Analytical = nil
	if out, err := expectedAnalytical(context.Background(), legacy, a); err != nil || out != nil {
		t.Fatal("legacy evidence was retroactively certified", err)
	}
}
func TestSQLRecoveryAnalyticalDiagnosticRemainsClosed(t *testing.T) {
	for _, err := range []error{exec.ErrAnalyticalMismatch, &exec.AnalyticalError{Code: "private-canary", Unsupported: true}} {
		code := validationCode(err, "private-sql")
		if code == "private-canary" {
			t.Fatal("raw diagnostic leaked")
		}
	}
}
