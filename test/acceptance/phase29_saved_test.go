package acceptance

import (
	"context"
	"testing"

	"github.com/hurtener/chartworks/internal/reporting"
)

// Exercise the real saved-query adapter separately from composition persistence.
// Each boundary has an assertion so a collapsed widget error cannot hide which
// owner rejected the synthetic request. No production error or authority changes.
func TestSavedQuestionReplayable(t *testing.T) {
	f := newPhase29Execution(t, true)
	ctx := context.Background()
	queries := reporting.DocumentsFromQueries(f.query)
	widget := f.queryWidget()
	beforeQueries := f.attemptCount(t)
	origin, err := queries.InspectDocumentQuery(ctx, f.execute, *widget.Query)
	if err != nil {
		t.Fatalf("saved query inspection: %T %v", err, err)
	}
	origin.Widget = widget.ID
	if f.attemptCount(t) != beforeQueries {
		t.Fatal("saved query inspection executed a source query")
	}
	plan, err := queries.PrepareDocumentQuery(ctx, f.execute, *widget.Query, origin, "saved-replayable-boundary", "en")
	if err != nil {
		t.Fatalf("saved query preparation: %T %v", err, err)
	}
	if plan.Query == "" || plan.BindingDigest == "" || f.attemptCount(t) != beforeQueries {
		t.Fatal("saved query preparation lost its proof or executed source work", plan)
	}
	result, err := queries.RunDocumentQuery(ctx, f.execute, *widget.Query, origin, plan, 100, 1<<20, false)
	if err != nil {
		t.Fatalf("saved query execution: %T %v", err, err)
	}
	if result.Execution.Result == nil || len(result.Execution.Result.Rows) != 2 || result.Execution.Attempt.ID == "" || result.Execution.Attempt.Finished == nil || f.attemptCount(t) != beforeQueries+1 {
		t.Fatal("saved query did not retain the real result and source receipt", result)
	}
	beforeModels := f.f.model.requests.Load()
	replayed, err := queries.RunDocumentQuery(ctx, f.execute, *widget.Query, origin, plan, 100, 1<<20, false)
	if err != nil || replayed.Execution.Attempt.ID != result.Execution.Attempt.ID || f.attemptCount(t) != beforeQueries+1 || f.f.model.requests.Load() != beforeModels {
		t.Fatal("saved query terminal replay repeated work or lost the original receipt", replayed, err)
	}
}
