package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

func TestReportingCompositionOutputSubsets(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	definition := phase27Definition(t, f.f, f.blockAuthor, f.base.SQL)
	f.block(t, "subset-error-block", definition)
	for _, policy := range []string{"fail_closed", "allow_partial"} {
		d := phase29Text("Output-local failure versus shared query")
		d.PartialFailure = policy
		table := phase29BlockWidget("table-only", "subset-error-block", 1, "table-main")
		narrative := phase29BlockWidget("narrative-only", "subset-error-block", 2, "narrative-main")
		narrative.Block.Narrative = true
		d.Widgets = append(d.Widgets, table, narrative)
		state := f.report(t, "subset-error-"+policy, d, true)
		admitted, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "subset-error-" + policy})
		if err != nil || admitted.QueryGroups != 1 {
			t.Fatal("output selection or narrative opt-in duplicated the query", admitted, err)
		}
		beforeQueries, beforeModels := f.attemptCount(t), f.f.model.requests.Load()
		finished, err := f.compositions.Run(ctx, f.execute, admitted.ID, false)
		if finished.Complete || f.attemptCount(t) != beforeQueries+1 || f.f.model.requests.Load() != beforeModels {
			t.Fatal("output failure fabricated completeness or repeated model/source work", finished, err)
		}
		if policy == "fail_closed" {
			if !errors.Is(err, reporting.ErrIncomplete) || finished.State != "failed" {
				t.Fatal("strict policy ignored a failed selected narrative", finished, err)
			}
			if _, err := f.compositions.Widget(ctx, f.execute, admitted.ID, "main", table.ID); !errors.Is(err, reporting.ErrIncomplete) {
				t.Fatal("strict failed report exposed a completed-looking payload", err)
			}
			continue
		}
		if err != nil || finished.State != "partial" || finished.Pages[0].Widgets[1].State != "completed" || finished.Pages[0].Widgets[2].State != "partial" {
			t.Fatal("metadata lost per-widget output status", finished, err)
		}
		good, err := f.compositions.Widget(ctx, f.execute, admitted.ID, "main", table.ID)
		if err != nil || good.State != "completed" || good.Code != "" || len(good.Outputs) != 1 || good.Outputs[0].ID != "table-main" {
			t.Fatal("unselected narrative failure contaminated the successful table payload", good, err)
		}
		bad, err := f.compositions.Widget(ctx, f.execute, admitted.ID, "main", narrative.ID)
		if err != nil || bad.State != "partial" || bad.Code != "output_failed" || len(bad.Outputs) != 1 || bad.Outputs[0].Code != "narrative_unavailable" {
			t.Fatal("failed selected output lost its exact receipt", bad, err)
		}
	}
}

func TestSavedQuestionSessionIsolation(t *testing.T) {
	f := newPhase29Execution(t, true)
	ctx := context.Background()
	queries := reporting.DocumentsFromQueries(f.query)
	widget := f.queryWidget()
	origin, err := queries.InspectDocumentQuery(ctx, f.execute, *widget.Query)
	if err != nil {
		t.Fatal(err)
	}
	origin.Widget = widget.ID
	plan, err := queries.PrepareDocumentQuery(ctx, f.execute, *widget.Query, origin, "origin-session-query", "en")
	if err != nil {
		t.Fatal(err)
	}
	widget.Query.Durability, widget.Query.Question, widget.Query.Query = "session_bound", "", plan.Query
	widget.Query.Selections = nil
	d := phase29Text("Session-bound authoring")
	d.Widgets = append(d.Widgets, widget)
	state := f.report(t, "session-isolation-report", d, false)
	token := phase27Token(t, f.f, f.execute.User(), "different-signed-session", phase29RuntimeScopes(f.execute.Tenant()))
	otherSession, err := f.f.f.token.verifier.Verify(ctx, token, auth.HTTP)
	if err != nil || otherSession.User() != f.execute.User() || otherSession.Session() == f.execute.Session() {
		t.Fatal("signed same-actor/different-session fixture is invalid", err)
	}
	beforeQueries, beforeModels := f.attemptCount(t), f.f.model.requests.Load()
	if _, err := queries.InspectDocumentQuery(ctx, otherSession, *widget.Query); !errors.Is(err, store.ErrNotFound) && !errors.Is(err, access.ErrNotFound) {
		t.Fatal("session-bound source definition escaped its original session", err)
	}
	admitted, err := f.compositions.Admit(ctx, otherSession, "report", state.ID, reporting.CompositionRequest{Key: "other-session-preview", Preview: true, Reference: reporting.DocumentReference{Revision: 1}})
	if err != nil {
		if !errors.Is(err, access.ErrNotFound) && !errors.Is(err, store.ErrNotFound) && !errors.Is(err, access.ErrForbidden) {
			t.Fatal("foreign session rejected outside the protected boundary", err)
		}
	} else {
		finished, err := f.compositions.Run(ctx, otherSession, admitted.ID, false)
		if !errors.Is(err, reporting.ErrIncomplete) || finished.Complete || finished.State != "failed" {
			t.Fatal("report preview authority impersonated the originating query session", finished, err)
		}
	}
	if f.attemptCount(t) != beforeQueries || f.f.model.requests.Load() != beforeModels {
		t.Fatal("foreign session denial reconstructed missing query work")
	}
}
