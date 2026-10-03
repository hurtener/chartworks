package nlqexec

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/generationdecision"
	"github.com/hurtener/chartworks/internal/semantics"
)

const maxGenerationRounds = 3

// GenerationPending is protected immutable evidence. Model prose never defines
// the choice catalog: every choice is derived from admitted reviewed definitions.
type GenerationPending struct {
	Input      *GenerationInputCommitment `json:"input,omitempty"`
	Refinement *RefineRequest             `json:"refinement,omitempty"`
	Problem    generationdecision.Problem `json:"problem"`
	Request    QuestionRequest            `json:"request"`
	Binding    string                     `json:"binding"`
	Round      int                        `json:"round"`
}

func generationChoices(a admission) []generationdecision.Choice {
	counts := map[generationdecision.Choice]int{}
	for _, p := range a.publications {
		for _, m := range p.Definition.Measures {
			counts[generationdecision.Choice{Kind: "measure", ID: m.ID}]++
		}
		for _, m := range p.Definition.KPIs {
			counts[generationdecision.Choice{Kind: "kpi", ID: m.ID}]++
		}
		for _, m := range p.Definition.Dimensions {
			counts[generationdecision.Choice{Kind: "dimension", ID: m.ID}]++
		}
	}
	var out []generationdecision.Choice
	for c, n := range counts {
		if n == 1 {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > 128 {
		return nil
	} // No silently truncated catalog.
	return out
}

func (s *Service) persistGenerationPending(ctx context.Context, e identity.Envelope, in QuestionRequest, operation, parent string, previous *QueryRecord, a admission, generation nlq.GenerationContext, receipt gateway.Receipt, fixes int, cause error) error {
	q, err := s.prepareGenerationPending(ctx, e, in, operation, parent, previous, a, generation, receipt, fixes, cause)
	if err != nil {
		return err
	}
	if err = s.repo.CreateQuery(ctx, mustScope(e), q); err != nil {
		return err
	}
	return &generationDecisionError{problem: q.GenerationPending.Problem}
}
func (s *Service) prepareGenerationPending(ctx context.Context, e identity.Envelope, in QuestionRequest, operation, parent string, previous *QueryRecord, a admission, generation nlq.GenerationContext, receipt gateway.Receipt, fixes int, cause error) (QueryRecord, error) {
	problem := GenerationProblem(cause)
	if problem == nil {
		return QueryRecord{}, cause
	}
	id, err := newID()
	if err != nil {
		return QueryRecord{}, err
	}
	if origin := generationContinuationState(ctx).pendingOrigin(); origin != nil {
		previous = origin
		parent = origin.ID
	}
	round := 1
	if previous != nil {
		ancestor := *previous
		for depth := 0; depth < MaxRefinementDepth; depth++ {
			if ancestor.GenerationPending != nil {
				round = ancestor.GenerationPending.Round + 1
				break
			}
			if ancestor.Parent == "" {
				break
			}
			next, readErr := s.repo.ReadQuery(ctx, mustScope(e), ancestor.Parent)
			if readErr != nil {
				return QueryRecord{}, readErr
			}
			if next.Session != e.Session() || next.Context != in.Context || next.Revision != ancestor.ParentRevision || QueryLineageDigest(next) != ancestor.ParentDigest {
				return QueryRecord{}, exec.ErrBinding
			}
			ancestor = next
		}
	}
	if round > maxGenerationRounds {
		return QueryRecord{}, ErrRefinementLimit
	}
	for i := range problem.Questions {
		problem.Questions[i] = a.route.RedactProtectedText(problem.Questions[i], in.Answers)
		if len(problem.Questions[i]) > generationdecision.MaxQuestionBytes {
			problem.Questions[i] = "[question withheld: redaction exceeds limit]"
		}
	}
	problem.QueryID = id
	problem.Resume = "plan"
	if generationContinuationState(ctx).refinementRequest() != nil {
		problem.Resume = "refine"
	}
	problem.ExpiresAt = time.Now().UTC().Add(15 * time.Minute)
	problem.Choices = generationChoices(a)
	refinement := generationContinuationState(ctx).refinementRequest()
	commitment := &GenerationInputCommitment{Policy: generationInputPolicy, Submission: generationInputHash(e, id, "submission", in), Fixed: generationInputHash(e, id, "fixed", fixedGenerationQuestion(in))}
	if refinement != nil {
		commitment.Refinement = generationInputHash(e, id, "refinement", fixedGenerationRefinement(*refinement))
	}
	in = sanitizeGenerationInput(in, a.route)
	pending := &GenerationPending{Input: commitment, Problem: *problem, Request: in, Binding: exec.Hash(a.binding), Round: round, Refinement: sanitizeGenerationRefinement(refinement, a.route)}
	problem.AnswerContext = generationPendingDigest(pending)

	q, err := queryRecord(e, id, "preflight", parent, in, a)
	if err != nil {
		return QueryRecord{}, err
	}
	bindParentLineage(&q, previous)
	pending.Problem = *problem
	q.GenerationPending = pending
	bindGenerationContinuation(ctx, &q)
	bindIntentReview(ctx, &q)
	q.Operation, q.Generation, q.Receipt, q.ValidationFixes = operation, generation, receipt, fixes
	return q, nil
}

// GenerationPendingValid is used at the durable store boundary as well as at
// resumption. Existing ordinary records do not acquire a pending state.
func GenerationPendingValid(q QueryRecord) bool {
	p := q.GenerationPending
	if p == nil {
		return GenerationResolutionValid(q)
	}
	if !generationInputValid(p) || q.Status != "preflight" || q.SQL != "" || len(q.Parameters) != 0 || q.Result != nil || q.Analytical != nil || q.AnalyticalVersion != 0 || p.Round < 1 || p.Round > maxGenerationRounds || p.Binding == "" || p.Problem.QueryID != q.ID || p.Problem.AnswerContext != generationPendingDigest(p) || p.Problem.ExpiresAt.IsZero() || generationdecision.Public(p.Problem) == nil {
		return false
	}
	for _, c := range p.Problem.Choices {
		if !identity.Identifier(c.ID) || c.Kind != "measure" && c.Kind != "kpi" && c.Kind != "dimension" {
			return false
		}
	}
	return GenerationResolutionValid(q)
}

func generationResumeRequest(old QuestionRequest, in QuestionRequest, p *GenerationPending) bool {
	return generationResumeChoices(in, p) && exec.Hash(fixedGenerationQuestion(old)) == exec.Hash(fixedGenerationQuestion(in))
}
func generationResumeChoices(in QuestionRequest, p *GenerationPending) bool {
	// Existing typed rule/value/grouping answers are revalidated by the same
	// router. Free text, instructions and the original form pin cannot change.
	if len(in.References) > 128 || len(in.References) == 0 && len(in.Answers) == 0 && len(in.Choices) == 0 && in.Grouping == nil && len(in.InterpretationEdits) == 0 {
		return false
	}
	allowed := map[generationdecision.Choice]bool{}
	for _, c := range p.Problem.Choices {
		allowed[c] = true
	}
	seen := map[semantics.Reference]bool{}
	for _, r := range in.References {
		if r.Dataset != "" || r.Revision != 0 || !allowed[generationdecision.Choice{Kind: string(r.Kind), ID: r.ID}] || seen[r] {
			return false
		}
		seen[r] = true
	}
	return true
}

func (s *Service) resumeGeneration(ctx context.Context, e identity.Envelope, in PlanRequest) (PlanResult, error) {
	if !identity.Identifier(in.GenerationQuery) || in.GenerationContext == "" || in.Operation != "resume:"+in.GenerationQuery {
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
	if p == nil || p.Refinement != nil || !GenerationPendingValid(q) || q.EvidenceStale || p.Problem.AnswerContext != in.GenerationContext || !time.Now().Before(p.Problem.ExpiresAt) || p.Round >= maxGenerationRounds || !generationResumeChoices(in.QuestionRequest, p) || !generationFixedQuestionEqual(e, q, in.QuestionRequest) {
		return PlanResult{}, exec.ErrBinding
	}
	if err = s.authenticateGenerationQuestion(ctx, e, q, in.QuestionRequest, "query.plan"); err != nil {
		return PlanResult{}, err
	}
	if err = requireQuestionAction(e, "query.plan", p.Request); err != nil {
		return PlanResult{}, err
	}
	if err := s.checkGenerationPending(ctx, e, q); err != nil {
		return PlanResult{}, err
	}
	question := in.QuestionRequest
	question.GenerationQuery = ""
	question.GenerationContext = ""
	if err := validateQuestion(question); err != nil {
		return PlanResult{}, err
	}
	ctx = withGenerationContinuation(ctx, &generationContinuation{origin: &q, answerDigest: exec.Hash(in)})
	return s.plan(ctx, e, question, in.Operation, q.ID, &q, "query.plan")
}

func (s *Service) checkGenerationPending(ctx context.Context, e identity.Envelope, q QueryRecord) error {
	if err := s.verifyIntentReview(ctx, e, q); err != nil {
		return err
	}
	p := q.GenerationPending
	if p == nil || !GenerationPendingValid(q) || q.EvidenceStale || q.Session != e.Session() || !time.Now().Before(p.Problem.ExpiresAt) {
		return exec.ErrBinding
	}
	action := "query.plan"
	if p.Refinement != nil {
		action = "query.execute"
	}
	if err := requireQuestionAction(e, action, p.Request); err != nil {
		return err
	}
	if err := s.verifyGenerationAncestry(ctx, e, q); err != nil {
		return err
	}
	current, err := s.resolveCurrentAdmission(ctx, e, q, s.topics.Contract, s.sources.Binding)
	if err != nil {
		return err
	}
	if err = gatewayRequirement(e, action, current.resources); err != nil {
		return err
	}
	if exec.Hash(current.binding) != p.Binding || !reflect.DeepEqual(current.relationScope, q.RelationScope) || !reflect.DeepEqual(generationChoices(current), p.Problem.Choices) {
		return exec.ErrBinding
	}
	return nil
}

func generationPendingDigest(p *GenerationPending) string {
	return exec.Hash(struct {
		ID         string
		Request    QuestionRequest
		Binding    string
		Choices    []generationdecision.Choice
		Questions  []string
		Outcome    string
		Expires    time.Time
		Round      int
		Refinement *RefineRequest
		Mode       string
		Input      *GenerationInputCommitment `json:"Input,omitempty"`
	}{p.Problem.QueryID, p.Request, p.Binding, p.Problem.Choices, p.Problem.Questions, p.Problem.Outcome, p.Problem.ExpiresAt, p.Round, p.Refinement, p.Problem.Resume, p.Input})
}
