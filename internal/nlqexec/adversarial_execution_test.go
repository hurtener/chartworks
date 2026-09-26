package nlqexec

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func TestSQLRecoveryAdversarialRepairContextFailureFinalizesOperation(t *testing.T) {
	e := unitEnvelope(t)
	repo := newUnitRepository()
	q := unitQuery(e, "broken-retained-context", "topic", "v1", "context", false)
	// Native execution of an old, parameter-free query does not require a new
	// generation packet. If a physical query error occurs, missing/incompatible
	// retained generation context is a terminal correction-preparation failure.
	q.Generation.Context = nlq.AssembledContext{}
	q.Assumptions = []string{"original accepted statement"}
	repo.queries[q.ID] = q
	model := &sequenceGateway{}
	executor := &unitExecutor{reports: []exec.ExecutionReport{{Attempt: exec.Attempt{Status: "failed", Code: "query_error"}}}, errors: []error{exec.ErrQuery}}
	reader := &unitTopicReader{current: map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)}, retained: map[string]topics.Contract{"topic/v1": unitContract("topic", "v1", "source", "context", "dataset", true, true)}}
	svc := &Service{topics: reader, sources: retainedSourceReader{}, validator: &unitValidator{}, executor: executor, engine: model, repo: repo}
	out, err := svc.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "broken-context-run"})
	if err == nil || !errors.Is(err, ErrExecutionBudget) || out.QueryID != q.ID || out.Status != "failed" || executor.calls != 1 || model.calls != 0 {
		t.Fatal("failed physical attempt not finalized before correction", err)
	}
	stored := repo.queries[q.ID]
	if stored.Status != "failed" || stored.ExecutionFixes != 0 || !reflect.DeepEqual(stored.Assumptions, q.Assumptions) || stored.SQL != q.SQL {
		t.Fatal("terminal preparation failure changed accepted metadata")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	replay, err := svc.Run(ctx, e, RunRequest{QueryID: q.ID, Operation: "broken-context-run"})
	if replay.Status != "failed" || err == nil || errors.Is(err, context.DeadlineExceeded) || executor.calls != 1 || model.calls != 0 {
		t.Fatal("idempotent replay waited on an abandoned operation", err)
	}
}

func TestSQLRecoveryAdversarialDurableFailureReceiptGatesCorrection(t *testing.T) {
	now := time.Now().UTC()
	valid := exec.ExecutionReport{Attempt: exec.Attempt{Status: "failed", Code: "query_error", RemoteState: "stopped", Finished: &now}}
	if !executionRepairable(valid, nil) {
		t.Fatal("durable stopped query failure could not reach correction")
	}
	for _, change := range []func(*exec.ExecutionReport){
		func(r *exec.ExecutionReport) { r.Attempt.RemoteState = "unknown" },
		func(r *exec.ExecutionReport) { r.Attempt.RemoteState = "running" },
		func(r *exec.ExecutionReport) { r.Attempt.RemoteState = "not_issued" },
		func(r *exec.ExecutionReport) { r.Attempt.Finished = nil },
		func(r *exec.ExecutionReport) { r.Attempt.Code = "source_unavailable" },
		func(r *exec.ExecutionReport) { r.Attempt.Status = "uncertain" },
		func(r *exec.ExecutionReport) { r.Result = &exec.Result{} },
	} {
		r := valid
		change(&r)
		if executionRepairable(r, nil) {
			t.Fatal("unconfirmed/non-query result authorized correction")
		}
	}
	for _, failure := range []error{errors.New("unclassified failure"), exec.ErrUncertain, exec.ErrBinding, context.Canceled, context.DeadlineExceeded} {
		if executionRepairable(valid, failure) {
			t.Fatal("terminal/unknown error authorized correction")
		}
	}
}
