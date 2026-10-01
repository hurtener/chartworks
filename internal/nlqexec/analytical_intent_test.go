package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hurtener/chartworks/internal/semantics"
	"go/parser"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
)

func TestSQLRecoveryReviewedAnalyticalIntentCompiler(t *testing.T) {
	for _, tc := range []struct {
		question     string
		count, limit int
		desc         bool
	}{
		{"Revenue by Region ordered by Revenue descending nulls last, Region ascending limit 3", 2, 3, true},
		{"Ingresos por región ordenados por Ingresos descendente nulos últimos y región ascendente límite 3", 2, 3, true},
		{"Revenue by Region sorted by Region", 1, 0, false},
		{"top 3 Revenue by Region", 1, 3, true},
		{"bottom 2 Revenue by Region", 1, 2, false},
		{"mayores 3 Ingresos por región", 1, 3, true},
	} {
		t.Run(tc.question, func(t *testing.T) {
			a := grainAdmission(tc.question)
			before, _ := json.Marshal(a.route)
			c, err := compileCurrentAnalytical(context.Background(), a)
			if err != nil || c == nil || c.Version != exec.AnalyticalGroupedProgramsVersion || c.Intent == nil {
				t.Fatal("missing intent", err)
			}
			if len(c.Intent.Order) != tc.count || c.Intent.Limit != tc.limit || c.Intent.Order[0].Descending != tc.desc || c.Grain == nil || len(c.Grain.Columns) != 1 || c.Grain.Columns[0] != "region_native" {
				t.Fatalf("incorrect intent/grain: %#v %#v", c.Intent, c.Grain)
			}
			if c.QueryPopulation == nil {
				t.Fatal("unowned filters not fenced")
			}
			after, _ := json.Marshal(a.route)
			if string(before) != string(after) {
				t.Fatal("mutated route")
			}
			if !strings.Contains(analyticalGrainGuidance(c), "Required reviewed ordering") {
				t.Fatal("missing generation guidance")
			}
		})
	}
}

func TestSQLRecoveryReviewedAnalyticalIntentRejectsAmbiguity(t *testing.T) {
	for _, question := range []string{
		"Revenue by Region ordered by Unknown limit 3",
		"Revenue by Region ordered by Revenue descending limit 0",
		"Revenue by Region ordered by Revenue descending limit 100001",
		"Revenue by Region ordered by Revenue and Revenue",
		"Revenue by Region ordered by Revenue sideways",
		"Revenue by Region ordered by Revenue limit 3 trailing",
		"top 0 Revenue by Region",
		"top -3 Revenue by Region",
		"Revenue by Region ordered by Revenue limit -3",
		"top 3 Revenue by Region ordered by Region",
	} {
		_, err := compileCurrentAnalytical(context.Background(), grainAdmission(question))
		if !errors.Is(err, exec.ErrAnalyticalUnsupported) {
			t.Fatalf("%q: %v", question, err)
		}
	}
	a := grainAdmission("Revenue by Region ordered by Revenue")
	a.publications[0].Definition.Dimensions[0].Aliases = append(a.publications[0].Definition.Dimensions[0].Aliases, "Revenue")
	analyticalReseal(&a)
	if _, err := compileCurrentAnalytical(context.Background(), a); !errors.Is(err, exec.ErrAnalyticalUnsupported) {
		t.Fatal("ambiguous reviewed label", err)
	}
}

func TestSQLRecoveryAnalyticalIntentRetainedVersions(t *testing.T) {
	a := grainAdmission("top 3 Revenue by Region")
	c, err := compileAnalyticalVersion(context.Background(), a, 5)
	if err != nil || c.Intent != nil || c.Version != exec.AnalyticalGroupingVersion {
		t.Fatal("upgraded retained v5", err)
	}
	c, err = compileAnalyticalVersion(context.Background(), a, 6)
	if err != nil || c.Intent == nil {
		t.Fatal(err)
	}
	q := QueryRecord{AnalyticalVersion: 6, Route: a.route, SQL: "SELECT region_native,sum(amount_native) FROM analytics.sales GROUP BY 1 ORDER BY 2 DESC NULLS LAST LIMIT 3"}
	q.Analytical = &exec.AnalyticalReceipt{Version: c.Version, Scope: exec.AnalyticalGrainScope, Contract: exec.Hash(*c), Query: exec.AnalyticalQueryDigest(q.SQL, nil), Metrics: []string{c.Metrics[0].ID}, Grouping: c.Grain.Dimensions, QueryPopulation: exec.AnalyticalQueryPopulationPolicy, Intent: exec.AnalyticalIntentPolicy}
	if _, err := expectedAnalytical(context.Background(), q, a); err != nil || !AnalyticalRecordValid(q) {
		t.Fatal("v6 replay", err)
	}
	q.Analytical.Intent = ""
	if _, err := expectedAnalytical(context.Background(), q, a); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("missing marker", err)
	}
}

func TestSQLRecoveryReviewedUnaryExpressionCompiler(t *testing.T) {
	a := analyticalAdmission()
	c := analyticalCompiler{expandedExpressions: true, ctx: context.Background(), definition: a.publications[0].Definition, binding: a.binding, visiting: map[semantics.Reference]bool{}}
	for _, formula := range []string{"-x", "+x", "x / -2", "-(-2) * x"} {
		node, err := parser.ParseExpr(formula)
		if err != nil {
			t.Fatal(err)
		}
		expr, err := c.expression(node, map[string]semantics.Reference{"x": {Kind: semantics.KindMeasure, ID: "revenue"}}, map[string]bool{}, nil, 0)
		if err != nil || expr.Op == "" {
			t.Fatal(formula, err)
		}
	}
	node, _ := parser.ParseExpr("-x")
	c.expandedExpressions = false
	if _, err := c.expression(node, map[string]semantics.Reference{"x": {Kind: semantics.KindMeasure, ID: "revenue"}}, map[string]bool{}, nil, 0); !errors.Is(err, exec.ErrAnalyticalUnsupported) {
		t.Fatal("legacy compiler widened", err)
	}
}

func TestSQLRecoveryReviewedLimitWithoutRanking(t *testing.T) {
	a := grainAdmission("Revenue by Region limit 3")
	c, err := compileCurrentAnalytical(context.Background(), a)
	if err != nil || c.Intent == nil || c.Intent.Order != nil || c.Intent.Limit != 3 || c.Grain == nil || c.Grain.Columns[0] != "region_native" {
		t.Fatal("limit-only intent", err)
	}
}

func TestSQLRecoveryReviewedCalendarOrdering(t *testing.T) {
	for _, question := range []string{"Revenue by month of Order date ordered by month of Order date descending", "Ingresos por mes de fecha de pedido ordenados por mes de fecha de pedido descendente"} {
		c, err := compileCurrentAnalytical(context.Background(), calendarAdmission(question, "timestamptz"))
		if err != nil || c.Intent == nil || len(c.Intent.Order) != 1 || c.Intent.Order[0].Bucket == nil || c.Grain == nil || exec.Hash(*c.Intent.Order[0].Bucket) != exec.Hash(c.Grain.Buckets[0]) {
			t.Fatal("calendar order lost reviewed bucket", err)
		}
	}
}
