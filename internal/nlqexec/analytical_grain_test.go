package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func grainAdmission(question string) admission {
	a := analyticalAdmission()
	a.publications[0].Definition.Measures[0].Name = "Revenue"
	a.publications[0].Definition.Measures[0].Aliases = []string{"Ingresos"}
	a.publications[0].Definition.Dimensions = []semantics.Dimension{
		{ID: "region", Name: "Region", Aliases: []string{"región", "sales region"}, Role: semantics.DimensionCategorical, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "region"}},
		{ID: "order", Name: "Order", Aliases: []string{"orden"}, Role: semantics.DimensionIdentifier, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "id"}},
	}
	a.route.Request.Question, a.route.Request.Locale = question, nlq.LanguageEnglish
	a.route.Selection.Topics[0].Roots = []nlqroute.SelectedRoot{
		{Reference: semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}, Reason: "catalog_term"},
		{Reference: semantics.Reference{Kind: semantics.KindDimension, ID: "region"}, Reason: "catalog_term"},
		{Reference: semantics.Reference{Kind: semantics.KindDimension, ID: "order"}, Reason: "interpreted_value"},
	}
	analyticalReseal(&a)
	return a
}
func TestSQLRecoveryGrainCompilerEnglishSpanishAndLists(t *testing.T) {
	for _, tc := range []struct {
		question string
		columns  []string
	}{
		{"Revenue by Region", []string{"region_native"}},
		{"Ingresos por región", []string{"region_native"}},
		{"Revenue grouped by sales region", []string{"region_native"}},
		{"Revenue per Region", []string{"region_native"}},
		{"Revenue by Region and Order", []string{"id_native", "region_native"}},
		{"Ingresos por región, y orden", []string{"id_native", "region_native"}},
		{"Revenue by Order, Region", []string{"id_native", "region_native"}},
	} {
		t.Run(tc.question, func(t *testing.T) {
			a := grainAdmission(tc.question)
			before, _ := json.Marshal(a.route)
			c, err := compileAnalytical(context.Background(), a)
			if err != nil || c == nil || c.Version != exec.AnalyticalGrainVersion || c.Grain == nil || !reflect.DeepEqual(c.Grain.Columns, tc.columns) {
				t.Fatal("incorrect grouping compilation", err)
			}
			after, _ := json.Marshal(a.route)
			if string(before) != string(after) {
				t.Fatal("compiler mutated route")
			}
			if !strings.Contains(analyticalGrainGuidance(c), "region_native") {
				t.Fatal("missing exact grouping guidance")
			}
		})
	}
}
func TestSQLRecoveryGrainCompilerDoesNotInventIntent(t *testing.T) {
	for _, question := range []string{
		"Revenue for Region", "Revenue", "Revenue ordered by Region", "Revenue not by Region", "Revenue not grouped by Region", "Revenue sin agrupar por región", "Revenue for 'by Region'", "Revenue by Unknown", "Revenue by 'Region'",
	} {
		t.Run(question, func(t *testing.T) {
			c, err := compileAnalytical(context.Background(), grainAdmission(question))
			if err != nil || c == nil || c.Grain != nil {
				t.Fatal("unknown/filter/negative/literal became grouping", err)
			}
		})
	}
	a := grainAdmission("Revenue per order")
	a.publications[0].Definition.Measures[0].Name = "Revenue per order"
	analyticalReseal(&a)
	if c, err := compileAnalytical(context.Background(), a); err != nil || c.Grain != nil {
		t.Fatal("metric name reinterpreted as grain", err)
	}
	// Dependency facts alone must not become the user's grouping decision.
	a = grainAdmission("Revenue by Region")
	a.route.Selection.Topics[0].Roots[1].Reason = "required_rule"
	analyticalReseal(&a)
	if c, err := compileAnalytical(context.Background(), a); err != nil || c.Grain != nil {
		t.Fatal("required dependency masqueraded as requested grouping", err)
	}
}
func TestSQLRecoveryGrainCompilerRejectsPartialAndAmbiguousProof(t *testing.T) {
	for _, question := range []string{"Revenue by Region and", "Revenue by Region extra words", "Revenue by Region for last quarter", "Revenue by Region and Unknown", "Revenue by Region,", "Revenue by Region by Order"} {
		if c, err := compileAnalytical(context.Background(), grainAdmission(question)); c != nil || !errors.Is(err, exec.ErrAnalyticalUnsupported) {
			t.Fatal("partial grouping proof", question, err)
		}
	}
	a := grainAdmission("Revenue by Region")
	a.publications[0].Definition.Dimensions[1].Aliases = []string{"Region"}
	analyticalReseal(&a)
	if _, err := compileAnalytical(context.Background(), a); !errors.Is(err, exec.ErrAnalyticalUnsupported) {
		t.Fatal("ambiguous grain", err)
	}
}
func TestSQLRecoveryGrainCompilerRejectsUnprovedMappings(t *testing.T) {
	for _, change := range []func(*admission){
		func(a *admission) { a.publications[0].Definition.Dimensions[0].Role = semantics.DimensionTemporal },
		func(a *admission) {
			a.publications[0].Definition.Dimensions[0].Filters = []semantics.SemanticFilter{{ID: "x", Field: a.publications[0].Definition.Dimensions[0].Field, Operator: "not_null"}}
		},
		func(a *admission) { a.publications[0].Definition.Dimensions[0].Field.ID = "missing" },
		func(a *admission) { a.binding.Relations[0].Columns[2].Safe = false },
		func(a *admission) { a.binding.Relations[0].Columns[2].Nullable = true },
	} {
		a := grainAdmission("Revenue by Region")
		change(&a)
		analyticalReseal(&a)
		if c, err := compileAnalytical(context.Background(), a); c != nil || err == nil {
			t.Fatal("unproved grouping admitted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := compileAnalytical(ctx, grainAdmission("Revenue by Region")); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored cancellation", err)
	}
}
func TestSQLRecoveryGrainCompilerPrivateAnswerCannotSelect(t *testing.T) {
	a := grainAdmission("Revenue for by Region")
	value := "by Region"
	a.route.Request.Answers = []semantics.ClarificationAnswer{{Topic: "sales_topic", Pattern: "customer", Slot: "name", Value: &semantics.ClarificationValue{Text: &value}}}
	a.route.Resolutions = []semantics.ClarificationResolution{{Topic: "sales_topic", Pattern: "customer", Slot: "name", Sensitivity: semantics.LiteralSensitive, Value: value}}
	c, err := compileAnalytical(context.Background(), a)
	if err != nil || c.Grain != nil {
		t.Fatal("sensitive value became grouping", err)
	}
	// The public redaction marker itself cannot resolve to a reviewed label.
	a = grainAdmission("Revenue by [redacted answer]")
	a.publications[0].Definition.Dimensions[0].Name = "redacted answer"
	analyticalReseal(&a)
	if c, err = compileAnalytical(context.Background(), a); err != nil || c.Grain != nil {
		t.Fatal("redaction marker became grouping", err)
	}
}
func TestSQLRecoveryGrainReceiptVersionedReplay(t *testing.T) {
	a := grainAdmission("Revenue by Region")
	for _, version := range []int{1, 2} {
		c, err := compileAnalyticalVersion(context.Background(), a, version)
		if err != nil {
			t.Fatal(err)
		}
		q := QueryRecord{AnalyticalVersion: version, Route: a.route, SQL: `SELECT region_native,sum(amount_native) FROM analytics.sales GROUP BY region_native`}
		q.Analytical = &exec.AnalyticalReceipt{Version: c.Version, Scope: exec.AnalyticalMetricScope, Contract: exec.Hash(*c), Query: exec.AnalyticalQueryDigest(q.SQL, nil), Metrics: []string{c.Metrics[0].ID}}
		if version == 2 {
			q.Analytical.Scope = exec.AnalyticalGrainScope
			q.Analytical.Grouping = append([]string(nil), c.Grain.Dimensions...)
		} else if c.Grain != nil {
			t.Fatal("v1 acquired a grain proof")
		}
		if !AnalyticalRecordValid(q) {
			t.Fatal("valid receipt shape rejected")
		}
		actual, err := expectedAnalytical(context.Background(), q, a)
		if err != nil || !reflect.DeepEqual(c, actual) {
			t.Fatal("versioned replay", err)
		}
		if version == 1 {
			continue
		}
		for _, change := range []func(*QueryRecord){
			func(q *QueryRecord) { q.Analytical.Grouping = nil },
			func(q *QueryRecord) { q.Analytical.Grouping[0] = "other:dimension:region" },
			func(q *QueryRecord) { q.Analytical.Scope = exec.AnalyticalMetricScope; q.Analytical.Grouping = nil },
			func(q *QueryRecord) { q.AnalyticalVersion = 1; q.Analytical.Version = exec.AnalyticalVersion },
		} {
			bad := q
			bad.Analytical = cloneAnalyticalReceipt(q.Analytical)
			change(&bad)
			if _, err := expectedAnalytical(context.Background(), bad, a); err == nil {
				t.Fatal("grain proof removed/downgraded/substituted")
			}
		}
		changed := a
		changed.route.Request.Question = "Revenue by Order"
		if _, err := expectedAnalytical(context.Background(), q, changed); err == nil {
			t.Fatal("different grain replayed original proof")
		}
	}
}
