package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

func TestSavedQuestionClarificationAndSelectionIdentity(t *testing.T) {
	f := newPhase29Execution(t, true)
	ctx := context.Background()
	queries := reporting.DocumentsFromQueries(f.query)
	widget := f.queryWidget()
	ambiguous := phase27Copy(t, *widget.Query)
	ambiguous.Selections = nil
	origin, err := queries.InspectDocumentQuery(ctx, f.execute, ambiguous)
	if err != nil {
		t.Fatal(err)
	}
	origin.Widget = widget.ID
	beforeQueries := f.attemptCount(t)
	_, err = queries.PrepareDocumentQuery(ctx, f.execute, ambiguous, origin, "saved-ambiguous", "en")
	var clarification *nlqroute.Clarification
	if !errors.As(err, &clarification) || clarification.Reason != "ambiguous_retrieval" || f.attemptCount(t) != beforeQueries {
		t.Fatal("saved consumer bypassed a genuine routing clarification", err)
	}
	selected, err := queries.InspectDocumentQuery(ctx, f.execute, *widget.Query)
	if err != nil || selected.QueryDigest == origin.QueryDigest {
		t.Fatal("explicit routing inputs lost independent identity", selected, err)
	}
	selected.Widget = widget.ID
	plan, err := queries.PrepareDocumentQuery(ctx, f.execute, *widget.Query, selected, "saved-selected", "en")
	if err != nil {
		t.Fatal("explicit reviewed routing inputs did not reach the existing planner", err)
	}
	changed := phase27Copy(t, *widget.Query)
	changed.Selections.LimitPerKind = 2
	changedOrigin, err := queries.InspectDocumentQuery(ctx, f.execute, changed)
	if err != nil {
		t.Fatal(err)
	}
	changedOrigin.Widget = widget.ID
	beforeModels := f.f.model.requests.Load()
	if _, err := queries.PrepareDocumentQuery(ctx, f.execute, changed, changedOrigin, "saved-selected", "en"); !errors.Is(err, store.ErrConflict) {
		t.Fatal("operation replay accepted changed routing inputs", err)
	}
	replayed, err := queries.PrepareDocumentQuery(ctx, f.execute, *widget.Query, selected, "saved-selected", "en")
	if err != nil || replayed != plan || f.f.model.requests.Load() != beforeModels || f.attemptCount(t) != beforeQueries {
		t.Fatal("exact planning replay repeated model/source work", replayed, err)
	}
}
