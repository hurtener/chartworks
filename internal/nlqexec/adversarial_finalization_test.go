package nlqexec

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

func adversarialFinalizationService(t *testing.T, q QueryRecord, executor *unitExecutor, repo *unitRepository) *Service {
	t.Helper()
	reader := &unitTopicReader{current: map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)}, retained: map[string]topics.Contract{"topic/v1": unitContract("topic", "v1", "source", "context", "dataset", true, true)}}
	repo.queries[q.ID] = q
	return &Service{topics: reader, sources: retainedSourceReader{}, validator: &unitValidator{}, executor: executor, engine: &sequenceGateway{}, repo: repo}
}

func TestSQLRecoveryAdversarialMissingReceiptFinalizesUncertain(t *testing.T) {
	e := unitEnvelope(t)
	q := unitQuery(e, "missing-physical-receipt", "topic", "v1", "context", false)
	repo := newUnitRepository()
	x := &unitExecutor{errors: []error{exec.ErrUncertain, exec.ErrReplay}}
	svc := adversarialFinalizationService(t, q, x, repo)
	out, err := svc.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "lost-receipt-run"})
	if !errors.Is(err, exec.ErrUncertain) || out.Status != "uncertain" || out.Execution.Result != nil || out.Execution.Attempt.Status != "" || repo.queries[q.ID].Status != "uncertain" {
		t.Fatal("missing physical receipt left an unfinished query or fabricated success", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	out, err = svc.Run(ctx, e, RunRequest{QueryID: q.ID, Operation: "lost-receipt-run"})
	if !errors.Is(err, exec.ErrUncertain) || errors.Is(err, context.DeadlineExceeded) || out.Status != "uncertain" || x.calls != 1 {
		t.Fatal("uncertain operation waited or reran", err)
	}
}

func TestSQLRecoveryAdversarialFailedRerunDiscardsPreviousRows(t *testing.T) {
	e := unitEnvelope(t)
	q := unitQuery(e, "repeat-query", "topic", "v1", "context", false)
	repo := newUnitRepository()
	x := &unitExecutor{reports: []exec.ExecutionReport{unitResult("succeeded"), {Attempt: exec.Attempt{Status: "failed", Code: "source_unavailable"}}}}
	svc := adversarialFinalizationService(t, q, x, repo)
	out, err := svc.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "first-success"})
	if err != nil || out.Execution.Result == nil {
		t.Fatal("positive source result", err)
	}
	out, err = svc.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "later-failure"})
	if err == nil || out.Status != "failed" || out.Execution.Result != nil || repo.queries[q.ID].Result != nil {
		t.Fatal("failed operation returned stale successful rows", err)
	}
	calls := x.calls
	out, err = svc.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "later-failure"})
	if err == nil || out.Execution.Result != nil || x.calls != calls {
		t.Fatal("failure replay recovered old rows or executed again", err)
	}
}

type finalizationScopeKey struct{}
type deadlineAwareQueryRepository struct {
	*unitRepository
	checked bool
}

func (r *deadlineAwareQueryRepository) UpdateQuery(ctx context.Context, scope store.Scope, q QueryRecord, expected int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 3*time.Second || time.Until(deadline) <= 0 || ctx.Value(finalizationScopeKey{}) != "same-authorized-operation" {
		return store.ErrInvalid
	}
	r.checked = true
	return r.unitRepository.UpdateQuery(ctx, scope, q, expected)
}
func TestSQLRecoveryAdversarialCancelledCallerCanFinalize(t *testing.T) {
	e := unitEnvelope(t)
	q := unitQuery(e, "cancelled-response", "topic", "v1", "context", false)
	repo := &deadlineAwareQueryRepository{unitRepository: newUnitRepository()}
	repo.queries[q.ID] = q
	svc := &Service{repo: repo}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), finalizationScopeKey{}, "same-authorized-operation"))
	cancel()
	out, err := svc.finishRun(ctx, e, q, unitResult("succeeded"), 0, nil)
	if err != nil || out.Status != "succeeded" || !repo.checked || repo.queries[q.ID].Status != "succeeded" {
		t.Fatal("caller cancellation abandoned known finalization", err)
	}
}

func TestSQLRecoveryAdversarialReceiptErrorsNeverExposeRows(t *testing.T) {
	e := unitEnvelope(t)
	for _, tc := range []struct {
		report exec.ExecutionReport
		err    error
		status string
	}{
		{exec.ExecutionReport{}, nil, "uncertain"},
		{exec.ExecutionReport{}, context.Canceled, "cancelled"},
		{exec.ExecutionReport{}, context.DeadlineExceeded, "timed_out"},
		{exec.ExecutionReport{}, exec.ErrLimit, "failed"},
		{exec.ExecutionReport{Attempt: exec.Attempt{Status: "accepted"}}, nil, "uncertain"},
		{exec.ExecutionReport{Attempt: exec.Attempt{Status: "succeeded"}}, nil, "uncertain"},
		{unitResult("succeeded"), exec.ErrUncertain, "uncertain"},
		{unitResult("interrupted"), nil, "interrupted"},
	} {
		q := unitQuery(e, "incomplete", "topic", "v1", "context", false)
		q.Result = unitResult("succeeded").Result
		repo := newUnitRepository()
		repo.queries[q.ID] = q
		out, err := (&Service{repo: repo}).finishRun(context.Background(), e, q, tc.report, 0, tc.err)
		if err == nil || out.Status != tc.status || out.Execution.Result != nil || repo.queries[q.ID].Result != nil {
			t.Fatal("invalid/missing receipt acquired a result or success", err)
		}
	}
}
