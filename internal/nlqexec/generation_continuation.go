package nlqexec

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

// GenerationResolution binds a child to the exact pending origin and typed
// submission. It is protected immutable evidence, never a bearer capability.
type GenerationResolution struct {
	QueryID      string `json:"query_id"`
	AnswerDigest string `json:"answer_digest"`
}

type generationContinuationKey struct{}
type generationContinuation struct {
	origin       *QueryRecord
	refinement   *RefineRequest
	answerDigest string
}

func generationContinuationState(ctx context.Context) *generationContinuation {
	if ctx == nil {
		return nil
	}
	p, _ := ctx.Value(generationContinuationKey{}).(*generationContinuation)
	return p
}
func withGenerationContinuation(ctx context.Context, p *generationContinuation) context.Context {
	return context.WithValue(ctx, generationContinuationKey{}, p)
}
func (p *generationContinuation) pendingOrigin() *QueryRecord {
	if p == nil {
		return nil
	}
	return p.origin
}
func (p *generationContinuation) refinementRequest() *RefineRequest {
	if p == nil {
		return nil
	}
	return p.refinement
}
func bindGenerationContinuation(ctx context.Context, q *QueryRecord) {
	state := generationContinuationState(ctx)
	if state == nil || state.origin == nil {
		return
	}
	q.Parent = state.origin.ID
	bindParentLineage(q, state.origin)
	q.GenerationResolution = &GenerationResolution{QueryID: state.origin.ID, AnswerDigest: state.answerDigest}
}
func GenerationResolutionValid(q QueryRecord) bool {
	p := q.GenerationResolution
	if p == nil {
		return true
	}
	digest, err := hex.DecodeString(p.AnswerDigest)
	return identity.Identifier(p.QueryID) && p.QueryID == q.Parent && q.ParentRevision > 0 && err == nil && len(digest) == 32 && strings.ToLower(p.AnswerDigest) == p.AnswerDigest
}
func generationResolutionReplay(ctx context.Context, q QueryRecord) (PlanResult, error, bool) {
	state := generationContinuationState(ctx)
	if state == nil || state.origin == nil {
		return PlanResult{}, nil, false
	}
	p := q.GenerationResolution
	if p == nil || !GenerationResolutionValid(q) || p.QueryID != state.origin.ID || p.AnswerDigest != state.answerDigest {
		return PlanResult{}, store.ErrConflict, true
	}
	if q.GenerationPending != nil {
		return PlanResult{}, &generationDecisionError{problem: q.GenerationPending.Problem}, true
	}
	return PlanResult{QueryID: q.ID, SessionID: q.Session, Status: "planned"}, nil, true
}

// The fixed portion binds the user's original request. Only the existing typed
// answer/edit fields may change; all those fields are revalidated by Refine.
func fixedGenerationRefinement(in RefineRequest) RefineRequest {
	in.QueryID = ""
	in.GenerationQuery = ""
	in.GenerationContext = ""
	in.Answers = nil
	in.AnswerContext = ""
	in.Choices = nil
	in.Grouping = nil
	in.InterpretationEdits = nil
	in.ParameterEdits = nil
	in.ReferenceEdits = nil
	in.MetricEdits = nil
	in.References = nil
	in.MetricIDs = nil
	return in
}
func generationReviewedReference(p *GenerationPending, r semantics.Reference) bool {
	if r.Dataset != "" || r.Revision != 0 {
		return false
	}
	for _, c := range p.Problem.Choices {
		if c.Kind == string(r.Kind) && c.ID == r.ID {
			return true
		}
	}
	return false
}
func generationRefinementChoices(p *GenerationPending, in RefineRequest) bool {
	for _, r := range in.References {
		if !generationReviewedReference(p, r) {
			return false
		}
	}
	for _, edit := range in.ReferenceEdits {
		if edit.Action == "add" && !generationReviewedReference(p, edit.Target) {
			return false
		}
		if edit.Replacement != nil && !generationReviewedReference(p, *edit.Replacement) {
			return false
		}
	}
	metric := func(id string) bool {
		return generationReviewedReference(p, semantics.Reference{Kind: semantics.KindMeasure, ID: id}) || generationReviewedReference(p, semantics.Reference{Kind: semantics.KindKPI, ID: id})
	}
	for _, id := range in.MetricIDs {
		if !metric(id) {
			return false
		}
	}
	for _, edit := range in.MetricEdits {
		if edit.Action == "add" && !metric(edit.Target) {
			return false
		}
		if edit.Replacement != "" && !metric(edit.Replacement) {
			return false
		}
	}
	return true
}
func (s *Service) resumeGenerationRefinement(ctx context.Context, e identity.Envelope, in RefineRequest) (PlanResult, error) {
	if in.QueryID != in.GenerationQuery || !identity.Identifier(in.GenerationQuery) || in.GenerationContext == "" {
		return PlanResult{}, ErrInvalid
	}
	q, err := s.repo.ReadQuery(ctx, mustScope(e), in.GenerationQuery)
	if err != nil {
		return PlanResult{}, err
	}
	if q.Session != e.Session() {
		return PlanResult{}, ErrForeignSession
	}
	p := q.GenerationPending
	if p == nil || p.Refinement == nil || p.Round >= maxGenerationRounds || p.Problem.AnswerContext != in.GenerationContext || !generationFixedRefinementEqual(e, q, in) || !generationRefinementChoices(p, in) {
		return PlanResult{}, exec.ErrBinding
	}
	if err = s.authenticateGenerationQuestion(ctx, e, q, refinementQuestion(q, in.QuestionRequest), "query.execute"); err != nil {
		return PlanResult{}, err
	}
	if err = s.checkGenerationPending(ctx, e, q); err != nil {
		return PlanResult{}, err
	}
	request := in
	request.QueryID = p.Refinement.QueryID
	request.GenerationQuery = ""
	request.GenerationContext = ""
	state := &generationContinuation{origin: &q, refinement: &request, answerDigest: exec.Hash(in)}
	return s.Refine(withGenerationContinuation(ctx, state), e, request)
}

func (s *Service) verifyGenerationAncestry(ctx context.Context, e identity.Envelope, q QueryRecord) error {
	seen := map[string]bool{}
	for depth := 0; q.Parent != ""; depth++ {
		if depth >= MaxRefinementDepth || seen[q.ID] {
			return ErrRefinementLimit
		}
		seen[q.ID] = true
		parent, err := s.repo.ReadQuery(ctx, mustScope(e), q.Parent)
		if err != nil {
			return err
		}
		if parent.Session != e.Session() || parent.Context != q.Context || parent.Revision != q.ParentRevision || QueryLineageDigest(parent) != q.ParentDigest {
			return exec.ErrBinding
		}
		q = parent
	}
	return nil
}
