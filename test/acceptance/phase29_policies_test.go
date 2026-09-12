package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
)

func TestReportingCompositionPolicies(t *testing.T) {
	t.Run("strict-and-partial-disabled-dynamic", testPhase29PartialPolicies)
	t.Run("typed-filter-precedence-and-provenance", testPhase29FilterBindings)
	t.Run("query-and-retention-budgets", testPhase29Budgets)
}

func (f *phase29ExecutionFixture) withLimits(t *testing.T, limits config.Reporting) *reporting.Compositions {
	t.Helper()
	queries := reporting.DocumentsFromQueries(f.query)
	documents, err := reporting.NewDocuments(f.f.f.db, f.blocks, queries, limits)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRequestRunner(f.f.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	runs := phase28RunService(t, f.f, f.blocks, f.f.f.db, nil, limits.Execution)
	compositions, err := reporting.NewCompositions(documents, f.f.f.db, runs, queries, runner)
	if err != nil {
		t.Fatal(err)
	}
	return compositions
}

func testPhase29PartialPolicies(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	f.block(t, "policy-block", f.base)
	for _, policy := range []string{"fail_closed", "allow_partial"} {
		d := phase29Text("Explicit failure policy")
		d.PartialFailure = policy
		d.Widgets = append(d.Widgets, phase29BlockWidget("frozen", "policy-block", 1, "table-main"), f.queryWidget())
		state := f.report(t, "policy-"+policy, d, true)
		beforeQueries, beforeModels := f.attemptCount(t), f.f.model.requests.Load()
		admitted, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "policy-" + policy})
		if err != nil {
			t.Fatal("explicit disabled dynamic omission should be retained", err)
		}
		if admitted.Pages[0].Widgets[2].Code != "live_queries_disabled" || admitted.Complete {
			t.Fatal("disabled live widget was not explicit", admitted)
		}
		finished, err := f.compositions.Run(ctx, f.execute, admitted.ID, false)
		if policy == "fail_closed" {
			if !errors.Is(err, reporting.ErrIncomplete) || finished.State != "failed" || finished.Complete || f.attemptCount(t) != beforeQueries {
				t.Fatal("strict omission performed work or manufactured completeness", finished, err)
			}
			if _, err := f.compositions.Widget(ctx, f.execute, admitted.ID, "main", "intro"); !errors.Is(err, reporting.ErrIncomplete) {
				t.Fatal("strict failed report exposed an apparently complete artifact", err)
			}
		} else {
			if err != nil || finished.State != "partial" || finished.Complete || f.attemptCount(t) != beforeQueries+1 {
				t.Fatal("partial policy lost a successful independent block", finished, err)
			}
			missing, err := f.compositions.Widget(ctx, f.execute, admitted.ID, "main", "dynamic")
			if err != nil || missing.Code != "live_queries_disabled" || missing.State != "failed" || missing.Query != nil || len(missing.Outputs) != 0 {
				t.Fatal("omitted dynamic widget was invented", missing, err)
			}
			if kept, err := f.compositions.Widget(ctx, f.execute, admitted.ID, "main", "frozen"); err != nil || len(kept.Outputs) != 1 {
				t.Fatal("partial policy discarded completed governed output", kept, err)
			}
		}
		if f.f.model.requests.Load() != beforeModels {
			t.Fatal("disabled dynamic lane called a model")
		}
	}
}

func testPhase29FilterBindings(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	block := phase27Copy(t, f.base)
	block.SQL = "SELECT id, amount FROM analytics.sales WHERE id >= $1 ORDER BY id"
	block.Parameters = []reporting.Parameter{{Name: "minimum", Type: "integer", Required: true, Default: &reporting.Value{Literal: "1"}, Min: "1", Max: "2"}}
	f.block(t, "filter-block", block)
	d := phase29Text("Typed filter precedence")
	d.Defaults = []reporting.Argument{{Name: "minimum", Value: reporting.Value{Literal: "1"}}}
	d.Filters = []reporting.ReportFilter{{Label: "Minimum transaction", Parameter: reporting.Parameter{Name: "minimum_filter", Type: "integer", Required: true, Default: &reporting.Value{Literal: "1"}}}}
	one := phase29BlockWidget("filter-driven", "filter-block", 1, "table-main")
	one.Literals = []reporting.Argument{{Name: "minimum", Value: reporting.Value{Literal: "2"}}}
	one.Bindings = []reporting.FilterBinding{{Filter: "minimum_filter", Parameter: "minimum"}}
	two := phase27Copy(t, one)
	two.ID, two.Grid.Row = "override-driven", 2
	two.Overrides = []string{"minimum"}
	d.Widgets = append(d.Widgets, one, two)
	state := f.report(t, "typed-filter-report", d, true)
	input := reporting.CompositionRequest{Key: "typed-filter-run", Pages: []reporting.PageInput{{Page: "main",
		Filters:   []reporting.Argument{{Name: "minimum_filter", Value: reporting.Value{Literal: "1"}}},
		Overrides: []reporting.WidgetOverride{{Widget: "override-driven", Arguments: []reporting.Argument{{Name: "minimum", Value: reporting.Value{Literal: "1"}}}}}}}}
	admitted, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, input)
	if err != nil || admitted.QueryGroups != 1 {
		t.Fatal("equal typed values with distinct provenance did not share the query", admitted, err)
	}
	widgets := admitted.Pages[0].Widgets
	if len(widgets[1].Parameters) != 1 || widgets[1].Parameters[0].Provenance != "filter:minimum_filter" || widgets[2].Parameters[0].Provenance != "invocation_override" {
		t.Fatal("binding provenance was collapsed during query deduplication", widgets)
	}
	before := f.attemptCount(t)
	finished, err := f.compositions.Run(ctx, f.execute, admitted.ID, false)
	if err != nil || !finished.Complete || f.attemptCount(t) != before+1 {
		t.Fatal("typed equivalent widgets repeated source work", finished, err)
	}
	stored, err := f.compositions.Get(ctx, f.execute, admitted.ID)
	if err != nil || stored.Pages[0].Widgets[1].Parameters[0].Provenance != "filter:minimum_filter" || stored.Pages[0].Widgets[2].Parameters[0].Provenance != "invocation_override" {
		t.Fatal("retained provenance changed", stored, err)
	}
	for _, bad := range []string{"1 OR TRUE", "3", "1.5"} {
		request := phase27Copy(t, input)
		request.Key = "invalid-filter-" + string(rune('a'+len(bad)))
		request.Pages[0].Filters[0].Value.Literal = bad
		badRun, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, request)
		if err == nil {
			failed, runErr := f.compositions.Run(ctx, f.execute, badRun.ID, false)
			if runErr == nil || failed.Complete {
				t.Fatal("invalid literal became executable SQL or bypassed block constraints", failed, runErr)
			}
		} else if !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal("invalid filter classification", err)
		}
		if f.attemptCount(t) != before+1 {
			t.Fatal("rejected filter contacted the warehouse")
		}
	}
	wrongType := phase27Copy(t, d)
	wrongType.Filters[0].Parameter.Type = "number"
	if _, err := f.documents.Create(ctx, f.author, "report", "wrong-filter-type", wrongType); !errors.Is(err, reporting.ErrInvalid) {
		t.Fatal("filter and block parameter types were silently coerced", err)
	}
	unsafe := phase27Copy(t, d)
	unsafe.Widgets[0].Text.Text = "<script>run()</script>"
	if _, err := f.documents.Create(ctx, f.author, "report", "unsafe-markup", unsafe); !errors.Is(err, reporting.ErrInvalid) {
		t.Fatal("active markup accepted", err)
	}
	unsafe = phase27Copy(t, d)
	unsafe.Widgets[0].Grid.Width = 13
	if _, err := f.documents.Create(ctx, f.author, "report", "invalid-grid", unsafe); !errors.Is(err, reporting.ErrInvalid) {
		t.Fatal("unbounded grid accepted", err)
	}
}

func testPhase29Budgets(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	block := phase27Copy(t, f.base)
	block.SQL = "SELECT id, amount FROM analytics.sales WHERE id >= $1 ORDER BY id"
	block.Parameters = []reporting.Parameter{{Name: "minimum", Type: "integer", Required: true, Default: &reporting.Value{Literal: "1"}, Min: "1", Max: "2"}}
	f.block(t, "budget-block", block)
	d := phase29Text("Bounded execution")
	d.PartialFailure = "allow_partial"
	one := phase29BlockWidget("first", "budget-block", 1, "table-main")
	two := phase29BlockWidget("second", "budget-block", 2, "table-second")
	two.Literals = []reporting.Argument{{Name: "minimum", Value: reporting.Value{Literal: "2"}}}
	d.Widgets = append(d.Widgets, one, two)
	state := f.report(t, "bounded-report", d, true)
	limits := f.limits
	limits.Composition.MaxQueries = 1
	bounded := f.withLimits(t, limits)
	before := f.attemptCount(t)
	admitted, err := bounded.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "query-budget"})
	if err != nil || admitted.QueryGroups != 1 || admitted.Pages[0].Widgets[2].Code != "budget_exhausted" {
		t.Fatal("query ceiling did not retain the explicit omission", admitted, err)
	}
	finished, err := bounded.Run(ctx, f.execute, admitted.ID, false)
	if err != nil || finished.State != "partial" || finished.Complete || f.attemptCount(t) != before+1 {
		t.Fatal("query-budget partial report", finished, err)
	}
	limits = f.limits
	limits.Composition.MaxRetainedBytes = 1024
	tooSmall := f.withLimits(t, limits)
	if _, err := tooSmall.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "insufficient-retention"}); !errors.Is(err, reporting.ErrBudget) {
		t.Fatal("insufficient manifest retention budget accepted", err)
	}
	if f.attemptCount(t) != before+1 {
		t.Fatal("retention reservation failure executed a source query")
	}
	limits = f.limits
	limits.Execution.MaxRows, limits.Execution.PageRows = 1, 1
	truncated := f.withLimits(t, limits)
	admitted, err = truncated.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "row-budget"})
	if err != nil {
		t.Fatal(err)
	}
	finished, err = truncated.Run(ctx, f.execute, admitted.ID, false)
	if err != nil || finished.Complete || finished.State != "partial" || finished.Pages[0].Widgets[1].Code != "query_truncated" {
		t.Fatal("truncated values were labeled complete", finished, err)
	}
}
