package nlqexec

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func calendarAdmission(question, native string) admission {
	a := grainAdmission(question)
	col := semantics.Column{ID: "event", Name: "Order date", SourceName: "event_native", NativeType: native, Category: "temporal", Nullable: true}
	a.publications[0].Definition.Datasets[0].Columns = append(a.publications[0].Definition.Datasets[0].Columns, col)
	a.binding.Relations[0].Columns = append(a.binding.Relations[0].Columns, exec.Column{Name: col.SourceName, NativeType: col.NativeType, Category: col.Category, Nullable: col.Nullable, Safe: true})
	a.publications[0].Definition.Dimensions = append(a.publications[0].Definition.Dimensions, semantics.Dimension{ID: "event", Name: "Order date", Aliases: []string{"fecha de pedido"}, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "event"}, Role: semantics.DimensionTemporal, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "America/New_York", Grains: []semantics.TimeGrain{"day", "month", "quarter", "year"}}})
	a.route.Selection.Topics[0].Roots = append(a.route.Selection.Topics[0].Roots, nlqroute.SelectedRoot{Reference: semantics.Reference{Kind: semantics.KindDimension, ID: "event"}, Reason: "catalog_term"})
	analyticalReseal(&a)
	return a
}

func TestSQLRecoveryCalendarCompilerReviewedBucket(t *testing.T) {
	for _, tc := range []struct {
		question, grain string
		columns         int
	}{
		{"Revenue by month of Order date", "month", 0},
		{"Ingresos por mes de fecha de pedido", "month", 0},
		{"Revenue by Region and quarter of Order date", "quarter", 1},
		{"Revenue by year of Order date, Region", "year", 1},
		{"Ingresos por día de fecha de pedido", "day", 0},
	} {
		t.Run(tc.question, func(t *testing.T) {
			a := calendarAdmission(tc.question, "timestamptz")
			before := exec.Hash(a.route)
			c, err := compileAnalytical(context.Background(), a)
			if err != nil || c == nil || c.Version != exec.AnalyticalCalendarVersion || c.Grain == nil || c.Grain.Policy != exec.AnalyticalCalendarPolicy || len(c.Grain.Buckets) != 1 || len(c.Grain.Columns) != tc.columns {
				t.Fatal("compiled bucket", err)
			}
			b := c.Grain.Buckets[0]
			if b.Grain != tc.grain || b.Column != "event_native" || b.Calendar != "gregorian" || b.Timezone != "America/New_York" || exec.Hash(a.route) != before {
				t.Fatal("wrong reviewed coordinates or mutation", b)
			}
			if !strings.Contains(analyticalGrainGuidance(c), "date_trunc") || !strings.Contains(analyticalGrainGuidance(c), "event_native") {
				t.Fatal("missing generation guidance")
			}
		})
	}
	for _, native := range []string{"date", "timestamp without time zone"} {
		c, err := compileAnalytical(context.Background(), calendarAdmission("Revenue by month of Order date", native))
		if err != nil || c.Grain.Buckets[0].Timezone != "" {
			t.Fatal("civil time acquired an instant timezone", err)
		}
	}
}

func TestSQLRecoveryCalendarCompilerRefusesUnreviewedPolicy(t *testing.T) {
	for _, change := range []func(*admission){
		func(a *admission) { a.publications[0].Definition.Dimensions[2].Temporal = nil },
		func(a *admission) { a.publications[0].Definition.Dimensions[2].Temporal.Calendar = "fiscal" },
		func(a *admission) {
			a.publications[0].Definition.Dimensions[2].Temporal.Grains = []semantics.TimeGrain{"day"}
		},
		func(a *admission) { a.publications[0].Definition.Dimensions[2].Temporal.Timezone = "" },
		func(a *admission) { a.publications[0].Definition.Dimensions[2].Temporal.Timezone = "Local" },
		func(a *admission) { a.publications[0].Definition.Dimensions[2].Role = semantics.DimensionCategorical },
		func(a *admission) { a.route.Selection.Topics[0].Roots[3].Reason = "required_rule" },
	} {
		a := calendarAdmission("Revenue by month of Order date", "timestamptz")
		change(&a)
		analyticalReseal(&a)
		if _, err := compileAnalytical(context.Background(), a); !errors.Is(err, exec.ErrAnalyticalUnsupported) {
			t.Fatal("unreviewed calendar admitted", err)
		}
	}
	for _, q := range []string{"Revenue by month of Unknown", "Revenue by month of", "Revenue by month of Order date and", "Revenue by Region and month of Unknown"} {
		if _, err := compileAnalytical(context.Background(), calendarAdmission(q, "date")); !errors.Is(err, exec.ErrAnalyticalUnsupported) {
			t.Fatal("partial proof", q, err)
		}
	}
}

func TestSQLRecoveryCalendarCompilerUnknownAndLiteralIntent(t *testing.T) {
	for _, q := range []string{"Revenue by month", "Revenue filtered by month of Order date", "Revenue sin agrupar por mes de fecha de pedido", "Revenue for “by month of Order date”", "Revenue not grouped by month of Order date"} {
		c, err := compileAnalytical(context.Background(), calendarAdmission(q, "timestamptz"))
		if err != nil || c == nil || c.Grain != nil {
			t.Fatal("unknown/literal/negative became calendar", q, err)
		}
	}
	a := calendarAdmission("Revenue by Month", "date")
	a.publications[0].Definition.Dimensions[0].Name = "Month"
	analyticalReseal(&a)
	c, err := compileAnalytical(context.Background(), a)
	if err != nil || len(c.Grain.Buckets) != 0 || !reflect.DeepEqual(c.Grain.Columns, []string{"region_native"}) {
		t.Fatal("direct reviewed label lost precedence", err)
	}
	a = calendarAdmission("Revenue for by month of Order date", "date")
	value := "by month of Order date"
	a.route.Request.Answers = []semantics.ClarificationAnswer{{Topic: "sales_topic", Pattern: "customer", Slot: "name", Value: &semantics.ClarificationValue{Text: &value}}}
	a.route.Resolutions = []semantics.ClarificationResolution{{Topic: "sales_topic", Pattern: "customer", Slot: "name", Sensitivity: semantics.LiteralSensitive, Value: value}}
	c, err = compileAnalytical(context.Background(), a)
	if err != nil || c.Grain != nil {
		t.Fatal("private value selected a calendar", err)
	}
}

func TestSQLRecoveryCalendarRetainedVersionsAndReplay(t *testing.T) {
	a := calendarAdmission("Revenue by month of Order date", "timestamptz")
	for _, v := range []int{1, 2} {
		c, err := compileAnalyticalVersion(context.Background(), a, v)
		if err != nil || c == nil || c.Grain != nil || c.Version == exec.AnalyticalCalendarVersion {
			t.Fatal("retained policy reinterpreted", v, err)
		}
	}
	c, err := compileAnalytical(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	q := QueryRecord{AnalyticalVersion: 3, Route: a.route, SQL: "SELECT date_trunc('month',event_native,'America/New_York'),sum(amount_native) FROM analytics.sales GROUP BY 1"}
	q.Analytical = &exec.AnalyticalReceipt{Version: c.Version, Scope: exec.AnalyticalCalendarScope, Contract: exec.Hash(*c), Query: exec.AnalyticalQueryDigest(q.SQL, nil), Metrics: []string{c.Metrics[0].ID}, Grouping: append([]string(nil), c.Grain.Dimensions...)}
	if !AnalyticalRecordValid(q) {
		t.Fatal("new receipt shape rejected")
	}
	got, err := expectedAnalytical(context.Background(), q, a)
	if err != nil || !reflect.DeepEqual(c, got) {
		t.Fatal("new replay", err)
	}
	for _, change := range []func(*QueryRecord){
		func(q *QueryRecord) { q.Analytical.Grouping = nil },
		func(q *QueryRecord) { q.Analytical.Contract = exec.Hash("wrong-calendar") },
		func(q *QueryRecord) { q.AnalyticalVersion = 2; q.Analytical.Version = exec.AnalyticalGrainVersion },
		func(q *QueryRecord) { q.Analytical.Scope = exec.AnalyticalMetricScope; q.Analytical.Grouping = nil },
	} {
		bad := q
		bad.Analytical = cloneAnalyticalReceipt(q.Analytical)
		change(&bad)
		if _, err := expectedAnalytical(context.Background(), bad, a); err == nil {
			t.Fatal("calendar proof removed or downgraded")
		}
	}
	changed := calendarAdmission("Revenue by month of Order date", "timestamptz")
	changed.publications[0].Definition.Dimensions[2].Temporal.Timezone = "UTC"
	analyticalReseal(&changed)
	if _, err := expectedAnalytical(context.Background(), q, changed); err == nil {
		t.Fatal("different timezone replayed")
	}
}
