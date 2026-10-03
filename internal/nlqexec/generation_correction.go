package nlqexec

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/store"
)

type generationCorrectionKey struct{}
type generationCorrection struct {
	admitted   admission
	generation nlq.GenerationContext
	receipt    gateway.Receipt
}
type generationPendingFinalizer interface {
	FinalizeGenerationPending(context.Context, store.Scope, QueryRecord, int64, QueryRecord) error
}

func correctionPendingOperation(q QueryRecord) string {
	return "generation:" + q.ID + ":" + strconv.FormatInt(q.Revision, 10)
}
func (s *Service) finalizeGenerationCorrection(ctx context.Context, e identity.Envelope, q QueryRecord, runErr error) (error, bool) {
	correction, _ := ctx.Value(generationCorrectionKey{}).(*generationCorrection)
	if correction == nil || q.Status != "failed" || GenerationProblem(runErr) == nil {
		return nil, false
	}
	finalizer, ok := s.repo.(generationPendingFinalizer)
	if !ok {
		return nil, false
	} // Test/in-process repositories retain their existing terminal behavior.
	q.Updated = time.Now().UTC().Truncate(time.Microsecond)
	request := RefineRequest{QueryID: q.ID}
	pendingCtx := withGenerationContinuation(ctx, &generationContinuation{refinement: &request})
	question := refinementQuestion(q, QuestionRequest{})
	var custodyErr error
	if correction.admitted.route.Applicability != nil {
		keeper, ok := s.router.(applicabilityKeeper)
		if !ok {
			custodyErr = exec.ErrBinding
		} else {
			correction.admitted.route, custodyErr = keeper.ReissueApplicabilityForPending(ctx, e, q.ID, q.Route)
		}
	}
	if q.IntentReview != nil {
		pendingCtx = context.WithValue(pendingCtx, intentReviewKey{}, q.IntentReview)
	}
	var pending QueryRecord
	err := custodyErr
	if err == nil {
		pending, err = s.prepareGenerationPending(pendingCtx, e, question, correctionPendingOperation(q), q.ID, &q, correction.admitted, correction.generation, correction.receipt, 0, runErr)
	}
	if err != nil {
		if finalErr := s.repo.UpdateQuery(ctx, mustScope(e), q, q.Revision-1); finalErr != nil {
			return finalErr, true
		}
		return err, true
	}
	if err = finalizer.FinalizeGenerationPending(ctx, mustScope(e), q, q.Revision-1, pending); err != nil {
		return err, true
	}
	return &generationDecisionError{problem: pending.GenerationPending.Problem}, true
}
func (s *Service) replayGenerationCorrection(ctx context.Context, e identity.Envelope, q QueryRecord) (error, bool) {
	if q.Status != "failed" {
		return nil, false
	}
	pending, err := s.repo.ReadOperation(ctx, mustScope(e), correctionPendingOperation(q))
	if errors.Is(err, store.ErrNotFound) {
		return nil, false
	}
	if err != nil {
		return err, true
	}
	if pending.Parent != q.ID || pending.GenerationPending == nil {
		return exec.ErrBinding, true
	}
	if err = s.checkGenerationPending(ctx, e, pending); err != nil {
		return err, true
	}
	return &generationDecisionError{problem: pending.GenerationPending.Problem}, true
}
