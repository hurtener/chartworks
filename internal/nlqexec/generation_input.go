package nlqexec

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

const generationInputPolicy = "protected-generation-input-v1"

// GenerationInputCommitment keeps exact input compatibility without retaining
// private original wording. Omission preserves historical pending digests.
type GenerationInputCommitment struct {
	Policy     string `json:"policy"`
	Submission string `json:"submission"`
	Fixed      string `json:"fixed"`
	Refinement string `json:"refinement,omitempty"`
}

func generationInputHash(e identity.Envelope, id, kind string, value any) string {
	return exec.Hash([]any{generationInputPolicy, e.Tenant(), e.User(), e.Session(), id, kind, value})
}
func fixedGenerationQuestion(in QuestionRequest) QuestionRequest {
	in.References = nil
	in.Answers = nil
	in.Choices = nil
	in.Grouping = nil
	in.InterpretationEdits = nil
	in.GenerationQuery = ""
	in.GenerationContext = ""
	return in
}
func generationInputValid(p *GenerationPending) bool {
	if p.Input == nil {
		return true
	}
	digest := func(s string) bool {
		b, err := hex.DecodeString(s)
		return err == nil && len(b) == 32 && strings.ToLower(s) == s
	}
	return p.Input.Policy == generationInputPolicy && digest(p.Input.Submission) && digest(p.Input.Fixed) && (p.Refinement == nil && p.Input.Refinement == "" || p.Refinement != nil && digest(p.Input.Refinement))
}
func sanitizeGenerationInput(in QuestionRequest, route nlqroute.RouteResult) QuestionRequest {
	in = redactClarificationInstructions(in, route)
	if len(in.Answers) > 0 {
		in.Answers = semantics.CanonicalClarificationAnswers(route.Resolutions)
	}
	return in
}
func sanitizeGenerationRefinement(in *RefineRequest, route nlqroute.RouteResult) *RefineRequest {
	if in == nil {
		return nil
	}
	out := *in
	out.QuestionRequest = sanitizeGenerationInput(in.QuestionRequest, route)
	if in.Question == "" {
		out.Question = ""
	}
	if in.IntentReview != nil {
		review := *in.IntentReview
		review.Answers = semantics.CanonicalClarificationAnswers(route.Resolutions)
		out.IntentReview = &review
	}
	return &out
}
func generationSubmissionEqual(e identity.Envelope, q QueryRecord, in QuestionRequest) bool {
	p := q.GenerationPending
	return exec.Hash(in) == exec.Hash(p.Request) || p.Input != nil && p.Input.Submission == generationInputHash(e, q.ID, "submission", in)
}
func generationFixedQuestionEqual(e identity.Envelope, q QueryRecord, in QuestionRequest) bool {
	p := q.GenerationPending
	fixed := fixedGenerationQuestion(in)
	return exec.Hash(fixed) == exec.Hash(fixedGenerationQuestion(p.Request)) || p.Input != nil && p.Input.Fixed == generationInputHash(e, q.ID, "fixed", fixed)
}
func generationFixedRefinementEqual(e identity.Envelope, q QueryRecord, in RefineRequest) bool {
	p := q.GenerationPending
	fixed := fixedGenerationRefinement(in)
	return exec.Hash(fixed) == exec.Hash(fixedGenerationRefinement(*p.Refinement)) || p.Input != nil && p.Input.Refinement == generationInputHash(e, q.ID, "refinement", fixed)
}

// The digest alone is not original-question authority. Exact original wording
// must also be admitted from this immutable pending row's protected fingerprint.
func (s *Service) authenticateGenerationQuestion(ctx context.Context, e identity.Envelope, q QueryRecord, in QuestionRequest, action string) error {
	if in.Question == q.Route.Request.Question {
		return nil
	}
	if q.Route.Applicability == nil {
		return exec.ErrBinding
	}
	_, err := s.withQueryApplicability(ctx, e, q, in.routeRequest(), action, "refine")
	return err
}
