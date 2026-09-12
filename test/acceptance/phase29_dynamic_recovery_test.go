package acceptance

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

// Inject only a lost response around the real planner. Neither branch fabricates
// a query, authority proof, source receipt, or successful provider response.
type phase29LostPlan struct {
	reporting.DocumentQueries
	persist bool
	lost    atomic.Bool
}

func (q *phase29LostPlan) PrepareDocumentQuery(ctx context.Context, e identity.Envelope, w reporting.QueryWidget, origin reporting.QueryOrigin, operation, locale string) (nlqexec.SavedPlan, error) {
	if !q.lost.CompareAndSwap(false, true) {
		return q.DocumentQueries.PrepareDocumentQuery(ctx, e, w, origin, operation, locale)
	}
	if q.persist {
		if _, err := q.DocumentQueries.PrepareDocumentQuery(ctx, e, w, origin, operation, locale); err != nil {
			return nlqexec.SavedPlan{}, err
		}
	}
	return nlqexec.SavedPlan{}, store.ErrUnavailable
}

func TestReportingCompositionDynamicRecovery(t *testing.T) {
	for _, persisted := range []bool{true, false} {
		name := "missing-plan-remains-uncertain"
		if persisted {
			name = "lost-plan-reply-reuses-evidence"
		}
		t.Run(name, func(t *testing.T) {
			f := newPhase29Execution(t, true)
			ctx := context.Background()
			d := phase29Text("Recover dynamic query evidence")
			d.Widgets = append(d.Widgets, f.queryWidget())
			state := f.report(t, "dynamic-recovery-report", d, true)
			widget := d.Widgets[1].Query
			question := nlqexec.SavedQuestion{Durability: widget.Durability, Context: widget.Context, Question: widget.Question, Selections: widget.Selections}
			for _, pin := range widget.Topics {
				question.Topics = append(question.Topics, nlqexec.SavedTopic{Topic: pin.Topic, Version: pin.Version, Digest: pin.Digest})
			}
			evidence, err := f.query.InspectSaved(ctx, f.execute, question)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.query.RecoverSaved(ctx, f.execute, question, evidence, "../invalid-operation"); !errors.Is(err, nlqexec.ErrInvalid) {
				t.Fatal("invalid recovery coordinates", err)
			}
			changed := evidence
			changed.QueryDigest = "changed-evidence"
			if _, err := f.query.RecoverSaved(ctx, f.execute, question, changed, "missing-plan"); !errors.Is(err, nlqexec.ErrNoPlan) {
				t.Fatal("changed recovery evidence", err)
			}

			queries := &phase29LostPlan{DocumentQueries: reporting.DocumentsFromQueries(f.query), persist: persisted}
			runner, err := jobs.NewRequestRunner(f.f.f.db, jobs.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			crashing, err := reporting.NewCompositions(f.documents, f.f.f.db, f.runs, queries, runner)
			if err != nil {
				t.Fatal(err)
			}
			v, err := crashing.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "dynamic-recovery"})
			if err != nil || v.QueryGroups != 1 {
				t.Fatal("admit real dynamic group", v, err)
			}
			beforeQueries, beforeModels := f.attemptCount(t), f.f.model.requests.Load()
			interrupted, err := crashing.Run(ctx, f.execute, v.ID, false)
			if !errors.Is(err, store.ErrUnavailable) || interrupted.Complete || interrupted.State != "sealed" || !queries.lost.Load() {
				t.Fatal("lost plan response was turned into permanent success/failure", interrupted, err)
			}
			saved, err := f.f.f.db.ReadComposition(ctx, f.execute, v.ID)
			if err != nil || !saved.Started["group-1"] || len(saved.Plans) != 0 || len(saved.Results) != 0 || f.attemptCount(t) != beforeQueries {
				t.Fatal("durable generation marker or no-source guarantee lost", saved, err)
			}
			afterModels := f.f.model.requests.Load()
			if persisted && afterModels <= beforeModels || !persisted && afterModels != beforeModels {
				t.Fatal("fault did not occur at the intended provider boundary", beforeModels, afterModels)
			}
			recovered, err := f.compositions.Run(ctx, f.execute, v.ID, true)
			if f.f.model.requests.Load() != afterModels || recovered.Manifest != v.Manifest {
				t.Fatal("recovery generated replacement SQL or replaced the manifest", recovered, err)
			}
			if !persisted {
				if !errors.Is(err, reporting.ErrIncomplete) || recovered.State != "failed" || recovered.Complete || f.attemptCount(t) != beforeQueries || recovered.Pages[0].Widgets[1].Code != "query_indeterminate" {
					t.Fatal("missing plan evidence was silently regenerated or declared complete", recovered, err)
				}
				return
			}
			if err != nil || !recovered.Complete || recovered.State != "completed" || f.attemptCount(t) != beforeQueries+1 {
				t.Fatal("persisted plan was not recovered once", recovered, err)
			}
			payload, err := f.compositions.Widget(ctx, f.execute, v.ID, "main", "dynamic")
			if err != nil || payload.Query == nil || payload.Query.Execution.Attempt.ID == "" || payload.Query.Execution.Result == nil || len(payload.Query.Execution.Result.Rows) != 2 {
				t.Fatal("recovered dynamic result lost its real source receipt", payload, err)
			}
			if replay, err := f.compositions.Run(ctx, f.execute, v.ID, false); err != nil || !replay.Complete || f.attemptCount(t) != beforeQueries+1 || f.f.model.requests.Load() != afterModels {
				t.Fatal("terminal recovery repeated source/model work", replay, err)
			}
		})
	}
}
