package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// A child can finish before the parent's group checkpoint fails. The parent
// retry is a new consumer of those retained values, not an ordinary artifact
// read: current ceilings still apply, but the child must never run again.
func TestCW03CompletedChildRetryUsesCurrentCaps(t *testing.T) {
	for _, tc := range []struct{ stage, bound string }{
		{"group", "unchanged"}, {"group", "rows"}, {"group", "bytes"},
		{"complete", "unchanged"}, {"complete", "rows"}, {"complete", "bytes"},
	} {
		t.Run(tc.stage+"/"+tc.bound, func(t *testing.T) {
			bound := tc.bound
			f := newPhase29Execution(t, false)
			ctx := t.Context()
			d, err := reporting.MigrateDefinition(f.base)
			if err != nil {
				t.Fatal(err)
			}
			d.QueryLimits = &reporting.QueryLimits{MaxRows: 128, MaxBytes: 65536, TimeoutMillis: 10000, QueryAttempts: 1}
			if bound == "bytes" {
				// Actual normalized result bytes, not an invented usage estimate.
				if _, err = f.f.f.admin.Exec(ctx, `INSERT INTO analytics.sales(id,amount) SELECT n,12345678901234567890.125 FROM generate_series(3,64) AS n`); err != nil {
					t.Fatal(err)
				}
			}
			f.block(t, "cw03-retry-child", d)
			report := phase29Text("Synthetic child checkpoint recovery")
			report.PartialFailure = "fail_closed"
			report.Widgets = []reporting.Widget{phase29BlockWidget("frozen", "cw03-retry-child", 0, "table-second", "table-main")}
			state := f.report(t, "cw03-retry-parent", report, true)
			admitted, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "cw03-child-caps"})
			if err != nil {
				t.Fatal(err)
			}
			raw := support.Raw(t, f.f.f.dsn)
			table, condition := "chartworks.composition_run_groups", "NEW.result IS NOT NULL"
			wantResults := 0
			if tc.stage == "complete" {
				table, condition = "chartworks.composition_runs", "NEW.complete AND NOT OLD.complete"
				wantResults = 1
			}
			// Both identifiers and conditions are fixed synthetic test constants.
			_, err = raw.Exec(ctx, `CREATE FUNCTION chartworks.cw03_parent_checkpoint_outage() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 RAISE EXCEPTION 'synthetic parent checkpoint outage' USING ERRCODE='58000'; END; $$;
 CREATE TRIGGER cw03_parent_checkpoint_outage BEFORE UPDATE ON `+table+`
 FOR EACH ROW WHEN (`+condition+`) EXECUTE FUNCTION chartworks.cw03_parent_checkpoint_outage();`)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := raw.Exec(context.Background(), `DROP TRIGGER IF EXISTS cw03_parent_checkpoint_outage ON `+table+`; DROP FUNCTION IF EXISTS chartworks.cw03_parent_checkpoint_outage();`); err != nil {
					t.Error(err)
				}
			})
			queries, models := f.attemptCount(t), f.f.model.requests.Load()
			interrupted, err := f.compositions.Run(ctx, f.execute, admitted.ID, false)
			if !errors.Is(err, store.ErrUnavailable) || interrupted.Complete || f.attemptCount(t) != queries+1 {
				t.Fatal("did not reach post-child parent checkpoint failure", interrupted, err)
			}
			parentBefore, err := f.f.f.db.ReadComposition(ctx, f.execute, admitted.ID)
			if err != nil || len(parentBefore.Results) != wantResults {
				t.Fatal("parent failed at the wrong persistence boundary", err)
			}
			var childID string
			if err = raw.QueryRow(ctx, `SELECT r.operation_id FROM chartworks.frozen_runs r JOIN chartworks.operations o USING (tenant_id,operation_id) WHERE r.tenant_id=$1 AND o.nested_parent=$2`, f.execute.Tenant(), admitted.ID).Scan(&childID); err != nil {
				t.Fatal(err)
			}
			childBefore, err := f.f.f.db.ReadFrozenRun(ctx, f.execute, childID, true)
			if err != nil || childBefore.View.State != "succeeded" || childBefore.Manifest == nil || childBefore.Result == nil || len(childBefore.View.QueryAttempts) != 1 {
				t.Fatal("child was not durably complete before retry", childBefore.View, err)
			}
			encoded, err := json.Marshal(childBefore.Result)
			if err != nil || bound == "bytes" && len(encoded) <= 1024 || bound == "rows" && len(childBefore.Result.Rows) < 2 {
				t.Fatal("fixture did not exceed the new ceiling", len(encoded), err)
			}
			if _, err = raw.Exec(ctx, `DROP TRIGGER cw03_parent_checkpoint_outage ON `+table); err != nil {
				t.Fatal(err)
			}
			limits := f.limits
			switch bound {
			case "rows":
				limits.Execution.MaxRows, limits.Execution.PageRows = 1, 1
			case "bytes":
				limits.Execution.MaxResultBytes = 1024
			}
			childRuns := phase28RunService(t, f.f, f.blocks, f.f.f.db, nil, limits.Execution)
			documents, err := reporting.NewDocuments(f.f.f.db, f.blocks, reporting.DocumentsFromQueries(f.query), limits)
			if err != nil {
				t.Fatal(err)
			}
			runner, err := jobs.NewRequestRunner(f.f.f.db, jobs.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			bounded, err := reporting.NewCompositions(documents, f.f.f.db, childRuns, reporting.DocumentsFromQueries(f.query), runner)
			if err != nil {
				t.Fatal(err)
			}
			finished, runErr := bounded.Run(ctx, f.execute, admitted.ID, true)
			payload, widgetErr := bounded.Widget(ctx, f.execute, admitted.ID, "main", "frozen")
			// Pending and failed parents are not readable artifacts. A budget
			// refusal must preserve the retained child without exposing it through
			// the incomplete parent, even when its group was checkpointed already.
			if bound != "unchanged" && (!errors.Is(widgetErr, reporting.ErrIncomplete) || !reflect.DeepEqual(payload, reporting.CompositionPayload{})) {
				t.Fatal("incomplete parent exposed retained widget values", payload, widgetErr)
			}
			switch {
			case bound == "unchanged":
				if runErr != nil || widgetErr != nil || finished.State != "completed" || !finished.Complete || len(payload.Outputs) != 2 || payload.Outputs[0].ID != "table-second" || payload.Outputs[1].ID != "table-main" {
					t.Fatal("eligible retained child did not resume exactly", finished, payload, runErr, widgetErr)
				}
			case tc.stage == "complete":
				if !errors.Is(runErr, reporting.ErrBudget) || finished.Complete {
					t.Fatal("pending completion bypassed a lowered runtime ceiling", finished, runErr)
				}
				checkpoint, readErr := f.f.f.db.ReadComposition(ctx, f.execute, admitted.ID)
				if readErr != nil || !reflect.DeepEqual(checkpoint.Results, parentBefore.Results) {
					t.Fatal("budget refusal erased or rewrote checkpointed evidence", readErr)
				}
				// Restoring the accepted cap may complete the unchanged checkpoint;
				// it never grants permission to repeat its physical query.
				finished, runErr = f.compositions.Run(ctx, f.execute, admitted.ID, true)
				payload, widgetErr = f.compositions.Widget(ctx, f.execute, admitted.ID, "main", "frozen")
				if runErr != nil || widgetErr != nil || finished.State != "completed" || !finished.Complete || len(payload.Outputs) != 2 || payload.Outputs[0].ID != "table-second" || payload.Outputs[1].ID != "table-main" {
					t.Fatal("eligible checkpoint could not resume without regeneration", finished, payload, runErr, widgetErr)
				}
			default:
				if !errors.Is(runErr, reporting.ErrIncomplete) || finished.State != "failed" || finished.Complete {
					t.Fatal("completed child bypassed a lowered runtime ceiling", finished, runErr)
				}
			}
			parentAfter, err := f.f.f.db.ReadComposition(ctx, f.execute, admitted.ID)
			if err != nil || !reflect.DeepEqual(parentAfter.Manifest, parentBefore.Manifest) {
				t.Fatal("retry rewrote accepted parent selection/revision/limits", err)
			}
			if tc.stage == "group" && bound != "unchanged" {
				if len(parentAfter.Results) != 1 {
					t.Fatal("budget refusal did not retain its typed group result")
				}
				for _, result := range parentAfter.Results {
					if result.State != "failed" || result.Code != "budget_exhausted" || len(result.Outputs) != 0 || result.Block != nil {
						t.Fatal("budget refusal lost its typed failure or retained blocked values", result)
					}
				}
			}
			childAfter, err := f.f.f.db.ReadFrozenRun(ctx, f.execute, childID, true)
			if err != nil || !reflect.DeepEqual(childAfter.Manifest, childBefore.Manifest) || !reflect.DeepEqual(childAfter.Result, childBefore.Result) || !reflect.DeepEqual(childAfter.Outputs, childBefore.Outputs) || len(childAfter.View.QueryAttempts) != 1 {
				t.Fatal("retry changed the original retained child", err)
			}
			if f.attemptCount(t) != queries+1 || f.f.model.requests.Load() != models {
				t.Fatal("retry regenerated retained source/model work")
			}
		})
	}
}

func TestCW03OutputLocaleIndependentOfBlockLocale(t *testing.T) {
	f := newPhase29Execution(t, false)
	d, err := reporting.MigrateDefinition(f.base)
	if err != nil {
		t.Fatal(err)
	}
	d.Metadata = []reporting.Localized{{Locale: "en-US", Title: "Synthetic result", Description: "Block metadata in English", Question: "What are the retained values?"}}
	for i := range d.Outputs {
		d.Outputs[i].Intent.Metadata = []reporting.OutputMetadata{
			{Locale: "en-US", DisplayName: "Retained values", Description: "Synthetic output"},
			{Locale: "es-AR", DisplayName: "Valores retenidos", Description: "Salida sintética"},
		}
	}
	f.block(t, "cw03-output-locale", d)
	delivery, err := reporting.NewDelivery(f.blocks, f.runs, f.documents, f.compositions, f.f.f.db, config.DefaultReportingViewer())
	if err != nil {
		t.Fatal(err)
	}
	queries, models := f.attemptCount(t), f.f.model.requests.Load()
	for _, tc := range []struct{ request, locale, title string }{
		{"es-AR", "es-AR", "Valores retenidos"},
		{"es-UY", "es-AR", "Valores retenidos"},
		{"fr-FR", "en-US", "Retained values"},
		{"", "en-US", "Retained values"},
	} {
		view, err := delivery.Describe(t.Context(), f.execute, reporting.DeliveryDescribeRequest{
			Target: reporting.DeliveryTarget{Kind: "block", ID: "cw03-output-locale", Revision: 1}, Locale: tc.request,
		})
		if err != nil || view.Resource.Locale != "en-US" || len(view.Outputs) != 2 {
			t.Fatal("localized published description unavailable", view, err)
		}
		for _, output := range view.Outputs {
			if output.Locale != tc.locale || output.Title != tc.title {
				t.Fatal("block locale overrode independently authored output locale", tc, output)
			}
		}
	}
	if f.attemptCount(t) != queries || f.f.model.requests.Load() != models {
		t.Fatal("metadata localization performed source/model work")
	}
}

// Exercise the actual tenant-scoped keyset pager, including its continuation
// branch. Counting only isolated serializer tests misses this storage behavior.
func TestCW03RetainedCatalogPagination(t *testing.T) {
	f := newPhase29Execution(t, false)
	d, err := reporting.MigrateDefinition(f.base)
	if err != nil {
		t.Fatal(err)
	}
	f.block(t, "cw03-catalog-paging", d)
	ids := make([]string, 0, 3)
	for _, key := range []string{"cw03-page-a", "cw03-page-b", "cw03-page-c"} {
		accepted, err := f.runs.Admit(t.Context(), f.execute, "cw03-catalog-paging", reporting.RunRequest{Key: key, Outputs: []string{"table-main"}})
		if err != nil {
			t.Fatal(err)
		}
		done, err := f.runs.Run(t.Context(), f.execute, accepted.ID, false)
		if err != nil || done.State != "succeeded" {
			t.Fatal("catalog fixture did not retain a real result", done, err)
		}
		ids = append(ids, done.ID)
	}
	slices.Sort(ids)
	reader := phase28Reader(t, f.f, "cw03-page-reader", "cw03-catalog-paging", d.Context)
	queries, models := f.attemptCount(t), f.f.model.requests.Load()
	after := ""
	for i, id := range ids {
		page, err := f.f.f.db.ListFrozenArtifacts(t.Context(), reader, after, 1)
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != id || page.Items[0].Revision != 1 || page.Items[0].State != "succeeded" {
			t.Fatal("retained keyset page changed identity/order", page, err)
		}
		wantNext := ""
		if i < len(ids)-1 {
			wantNext = id
		}
		if page.Next != wantNext {
			t.Fatal("continuation skipped or repeated a retained result", page.Next, wantNext)
		}
		after = id
	}
	empty, err := f.f.f.db.ListFrozenArtifacts(t.Context(), reader, after, 1)
	if err != nil || len(empty.Items) != 0 || empty.Next != "" {
		t.Fatal("last page did not terminate", empty, err)
	}
	otherContext := phase28Reader(t, f.f, "cw03-page-isolated", "cw03-catalog-paging", "other-context")
	isolated, err := f.f.f.db.ListFrozenArtifacts(t.Context(), otherContext, "", 1)
	if err != nil || len(isolated.Items) != 0 || isolated.Next != "" {
		t.Fatal("same-tenant different-context catalog leaked entries", isolated, err)
	}
	for _, tc := range []struct {
		after string
		limit int
	}{{"", 0}, {"", 101}, {"../artifact", 1}} {
		page, err := f.f.f.db.ListFrozenArtifacts(t.Context(), reader, tc.after, tc.limit)
		if !errors.Is(err, store.ErrInvalid) || len(page.Items) != 0 || page.Next != "" {
			t.Fatal("invalid pagination accepted or leaked values", page, err)
		}
	}
	if f.attemptCount(t) != queries || f.f.model.requests.Load() != models {
		t.Fatal("retained catalog performed warehouse/model work")
	}
}
