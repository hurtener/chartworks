package acceptance

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

type phase29LostReply struct {
	reporting.CompositionRepository
	kind string
	lost atomic.Bool
}

func (r *phase29LostReply) CheckpointComposition(ctx context.Context, inv jobs.Invocation, proof reporting.PreparedCompositionWrite) (reporting.CompositionRecord, error) {
	write, err := proof.Checked(inv)
	if err != nil {
		return reporting.CompositionRecord{}, err
	}
	out, err := r.CompositionRepository.CheckpointComposition(ctx, inv, proof)
	if err == nil && write.Kind == r.kind && r.lost.CompareAndSwap(false, true) {
		return reporting.CompositionRecord{}, store.ErrUnavailable
	}
	return out, err
}

func TestReportingCompositionRecovery(t *testing.T) {
	t.Run("lost-child-result-reply", testPhase29ChildRecovery)
	t.Run("lost-parent-group-reply", func(t *testing.T) { testPhase29ParentRecovery(t, "group") })
	t.Run("lost-parent-completion-reply", func(t *testing.T) { testPhase29ParentRecovery(t, "complete") })
}

func testPhase29ChildRecovery(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	f.block(t, "child-recovery-block", f.base)
	definition := phase29Text("Recover child evidence")
	definition.Widgets = append(definition.Widgets, phase29BlockWidget("frozen", "child-recovery-block", 1, "table-main"))
	state := f.report(t, "child-recovery-report", definition, true)
	lost := &phase28LostReply{RunRepository: f.f.f.db}
	childRuns := phase28RunService(t, f.f, f.blocks, lost, nil, f.limits.Execution)
	runner, err := jobs.NewRequestRunner(f.f.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	crashing, err := reporting.NewCompositions(f.documents, f.f.f.db, childRuns, reporting.DocumentsFromQueries(f.query), runner)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := crashing.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "child-recovery"})
	if err != nil {
		t.Fatal(err)
	}
	beforeQueries, beforeModels := f.attemptCount(t), f.f.model.requests.Load()
	interrupted, err := crashing.Run(ctx, f.execute, admitted.ID, false)
	if !errors.Is(err, store.ErrUnavailable) || !lost.lost.Load() || interrupted.State == "failed" || interrupted.Complete || f.attemptCount(t) != beforeQueries+1 {
		t.Fatal("child storage uncertainty was sealed as permanent failure", interrupted, err)
	}
	recovered, err := f.compositions.Run(ctx, f.execute, admitted.ID, true)
	if err != nil || recovered.State != "completed" || !recovered.Complete || recovered.Manifest != admitted.Manifest || f.attemptCount(t) != beforeQueries+1 || f.f.model.requests.Load() != beforeModels {
		t.Fatal("child recovery repeated source work or changed the accepted manifest", recovered, err)
	}
	payload, err := f.compositions.Widget(ctx, f.execute, admitted.ID, "main", "frozen")
	if err != nil || len(payload.Outputs) != 1 || payload.Outputs[0].ID != "table-main" {
		t.Fatal("recovered child output was not retained", payload, err)
	}
}

func testPhase29ParentRecovery(t *testing.T, boundary string) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	f.block(t, "parent-recovery-block", f.base)
	definition := phase29Text("Recover parent checkpoints")
	definition.Widgets = append(definition.Widgets, phase29BlockWidget("frozen", "parent-recovery-block", 1, "table-main"))
	state := f.report(t, "parent-recovery-report", definition, true)
	lost := &phase29LostReply{CompositionRepository: f.f.f.db, kind: boundary}
	runner, err := jobs.NewRequestRunner(f.f.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	crashing, err := reporting.NewCompositions(f.documents, lost, f.runs, reporting.DocumentsFromQueries(f.query), runner)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := crashing.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "parent-recovery-" + boundary})
	if err != nil {
		t.Fatal(err)
	}
	beforeQueries, beforeModels := f.attemptCount(t), f.f.model.requests.Load()
	_, err = crashing.Run(ctx, f.execute, admitted.ID, false)
	if !lost.lost.Load() || boundary == "group" && !errors.Is(err, store.ErrUnavailable) || boundary == "complete" && err != nil {
		t.Fatal("parent reply-loss boundary was not exercised", boundary, err)
	}
	recovered, err := f.compositions.Run(ctx, f.execute, admitted.ID, true)
	if err != nil || !recovered.Complete || recovered.Manifest != admitted.Manifest || f.attemptCount(t) != beforeQueries+1 || f.f.model.requests.Load() != beforeModels {
		t.Fatal("parent replay discarded checkpoints or repeated source/model work", recovered, err)
	}
	if _, err := f.f.f.db.CheckpointComposition(ctx, jobs.Invocation{}, reporting.PreparedCompositionWrite{}); err == nil {
		t.Fatal("forged lease published a composition checkpoint")
	}
}
