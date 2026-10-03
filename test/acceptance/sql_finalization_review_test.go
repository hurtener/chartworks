package acceptance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// Inject only the journal failure/cancel edge. Native source execution, attempt
// admission, observer, PostgreSQL metadata, authorization and replay stay real.
type adversarialFinalizationStore struct {
	readexec.AttemptStore
	failure  bool
	cancel   context.CancelFunc
	finishes int
}

func (s *adversarialFinalizationStore) FinishRead(ctx context.Context, sc store.Scope, a readexec.Attempt, reconcile bool) error {
	s.finishes++
	if s.failure {
		return store.ErrUnavailable
	}
	err := s.AttemptStore.FinishRead(ctx, sc, a, reconcile)
	if s.cancel != nil {
		s.cancel()
	}
	return err
}

func TestSQLRecoveryAdversarialUnknownJournalAcceptance(t *testing.T) {
	for _, fail := range []bool{true, false} {
		name := "client_cancel_after_known_result"
		if fail {
			name = "journal_failure_without_receipt"
		}
		t.Run(name, func(t *testing.T) {
			f := newCW01Fixture(t)
			f.model.mode.Store(phase18RawResponse(t, `SELECT id,amount FROM analytics.sales ORDER BY id`))
			p, err := f.query.Plan(context.Background(), f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Show reviewed records", nlq.LanguageEnglish)})
			if err != nil {
				t.Fatal("native fixture plan", err)
			}
			runCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			repo := &adversarialFinalizationStore{AttemptStore: f.f.db, failure: fail}
			if !fail {
				repo.cancel = cancel
			}
			x, err := readexec.NewExecutor(f.f.s, repo, config.DefaultReadValidation())
			if err != nil {
				t.Fatal(err)
			}
			_, published := newPhase18Service(t, f.phase17Fixture)
			svc, err := nlqexec.New(f.service, published, f.f.s, f.f.validator, x, f.model.engine, f.f.db)
			if err != nil {
				t.Fatal(err)
			}
			metadata := support.Raw(t, f.f.dsn)
			before := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			calls := adversarialChatCount(f)
			out, err := svc.Run(runCtx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-read"})
			want := "succeeded"
			if fail {
				want = "uncertain"
			}
			if out.Status != want || repo.finishes != 1 || adversarialChatCount(f) != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before+1 {
				t.Fatal("logical operation did not finish after real source work", err)
			}
			if fail {
				if !errors.Is(err, readexec.ErrUncertain) || out.Execution.Result != nil || out.Execution.Attempt.Status != "" {
					t.Fatal("unknown journal outcome acquired fabricated proof/rows", err)
				}
			} else if err != nil || runCtx.Err() != context.Canceled || out.Execution.Result == nil || len(out.Execution.Result.Rows) != 2 {
				t.Fatal("cancelled response lost known completed result", err)
			}
			sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
			saved, err := f.f.db.ReadQuery(context.Background(), sc, p.QueryID)
			if err != nil || saved.Status != want || fail && saved.Result != nil {
				t.Fatal("terminal query metadata missing", err)
			}
			replayCtx, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			replay, err := svc.Run(replayCtx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-read"})
			if replay.Status != want || errors.Is(err, context.DeadlineExceeded) || fail && !errors.Is(err, readexec.ErrUncertain) || !fail && err != nil || repo.finishes != 1 || adversarialChatCount(f) != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before+1 {
				t.Fatal("terminal replay waited or repeated source/model work", err)
			}
		})
	}
}

func TestSQLRecoveryAdversarialFailedRerunClearsRowsAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	f.model.mode.Store(phase18RawResponse(t, `SELECT id,1/amount AS amount FROM analytics.sales ORDER BY id`))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Show reviewed records", nlq.LanguageEnglish)})
	if err != nil {
		t.Fatal("initial plan", err)
	}
	first, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-first"})
	if err != nil || first.Execution.Result == nil || len(first.Execution.Result.Rows) != 2 {
		t.Fatal("initial positive result", err)
	}
	if _, err := f.f.admin.Exec(ctx, `UPDATE analytics.sales SET amount=0`); err != nil {
		t.Fatal(err)
	}
	calls := adversarialChatCount(f)
	failed, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-failed"})
	if !errors.Is(err, nlqexec.ErrExecutionBudget) || failed.Status != "failed" || failed.Execution.Result != nil || adversarialChatCount(f) != calls+1 {
		t.Fatal("failed source read carried rows from old operation", err)
	}
	sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
	saved, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
	if err != nil || saved.Result != nil || saved.Status != "failed" {
		t.Fatal("stale result still durable", err)
	}
	metadata := support.Raw(t, f.f.dsn)
	attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	calls = adversarialChatCount(f)
	replay, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-failed"})
	if err == nil || replay.Status != "failed" || replay.Execution.Result != nil || adversarialChatCount(f) != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
		t.Fatal("failed replay returned stale rows or ran again", err)
	}
}
