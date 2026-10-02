package nlqexec

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"reflect"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

// LegacyIntentReview replaces complete retained intent through typed answers,
// or an explicit current selection for a dedicated v7 domain review. It does not approve,
// copy, or infer any predicate from the historical SQL or its parameter values.
type LegacyIntentReview struct {
	QueryID         string                          `json:"query_id"`
	SelectionDigest string                          `json:"selection_digest,omitempty"`
	AnswerContext   string                          `json:"answer_context"`
	Answers         []semantics.ClarificationAnswer `json:"answers"`
}

func (LegacyIntentReview) String() string         { return "legacy-intent-review(redacted)" }
func (v LegacyIntentReview) GoString() string     { return v.String() }
func (v LegacyIntentReview) LogValue() slog.Value { return slog.StringValue(v.String()) }

// IntentReviewOrigin pins protected metadata, not an executable capability.
type IntentReviewOrigin struct {
	QueryID  string `json:"query_id"`
	Revision int64  `json:"revision"`
	Digest   string `json:"digest"`
}

// IntentReviewEvidence keeps both origins even if a later generation decision
// adds an intervening parent. No private answer value is copied into this seal.
type IntentReviewEvidence struct {
	Legacy          IntentReviewOrigin `json:"legacy"`
	Preflight       IntentReviewOrigin `json:"preflight"`
	AnswerDigest    string             `json:"answer_digest"`
	SelectionDigest string             `json:"selection_digest,omitempty"`
}

func IntentReviewValid(q QueryRecord) bool {
	p := q.IntentReview
	if p == nil {
		return true
	}
	digest := func(s string) bool {
		b, e := hex.DecodeString(s)
		return e == nil && len(b) == 32 && strings.ToLower(s) == s
	}
	origin := func(o IntentReviewOrigin) bool {
		return identity.Identifier(o.QueryID) && o.Revision > 0 && digest(o.Digest)
	}
	return (q.GenerationPending != nil || q.AnalyticalVersion >= 7) && q.Parent != "" && origin(p.Legacy) && origin(p.Preflight) && p.Legacy.QueryID != p.Preflight.QueryID && p.Legacy.QueryID != q.ID && p.Preflight.QueryID != q.ID && digest(p.AnswerDigest) && (p.SelectionDigest == "" || digest(p.SelectionDigest))
}
func reviewOrigin(q QueryRecord) IntentReviewOrigin {
	return IntentReviewOrigin{q.ID, q.Revision, QueryLineageDigest(q)}
}

type intentReviewKey struct{}

// IntentReviewReader locates the initial immutable replacement after execution
// has acquired its own operation identity.
type IntentReviewReader interface {
	ReadIntentReview(context.Context, store.Scope, string, string) (QueryRecord, error)
}

func bindIntentReview(ctx context.Context, q *QueryRecord) {
	if p, ok := ctx.Value(intentReviewKey{}).(*IntentReviewEvidence); ok {
		copy := *p
		q.IntentReview = &copy
	}
}

func (s *Service) refineReviewedLegacyIntent(ctx context.Context, e identity.Envelope, in RefineRequest, old QueryRecord, parent admission, domainReview bool) (PlanResult, error) {
	review := in.IntentReview
	// The complete replacement is selected in Preflight, never merged with the
	// old question, old SQL, additive edits, or caller-supplied prompt fragments.
	if review == nil || old.AnalyticalVersion < 1 || (old.AnalyticalVersion > 6 && !domainReview) || old.Analytical == nil || old.SQL == "" || old.EvidenceStale || !reflect.DeepEqual(in.QuestionRequest, QuestionRequest{}) || len(in.ParameterEdits)+len(in.ReferenceEdits)+len(in.MetricEdits) != 0 || !identity.Identifier(review.QueryID) || review.QueryID == old.ID || len(review.Answers) > 64 {
		return PlanResult{}, ErrInvalid
	}
	if domainReview {
		if old.AnalyticalVersion != 7 || review.SelectionDigest == "" || review.AnswerContext != "" || len(review.Answers) != 0 {
			return PlanResult{}, ErrInvalid
		}
	} else if review.SelectionDigest != "" || review.AnswerContext == "" || len(review.Answers) == 0 {
		return PlanResult{}, ErrInvalid
	}
	pending, err := s.repo.ReadQuery(ctx, mustScope(e), review.QueryID)
	if err != nil {
		return PlanResult{}, err
	}
	if pending.Session != e.Session() || pending.Context != old.Context {
		return PlanResult{}, ErrForeignSession
	}
	current, err := s.intentReviewPreflightAdmission(ctx, e, pending, review.SelectionDigest)
	if err != nil {
		return PlanResult{}, err
	}
	if exec.Hash(current.binding) != exec.Hash(parent.binding) || !reflect.DeepEqual(pending.Topics, old.Topics) || !domainReview && (!reflect.DeepEqual(pending.TopicVersions, old.TopicVersions) || !reflect.DeepEqual(pending.RuleVersions, old.RuleVersions)) {
		return PlanResult{}, exec.ErrBinding
	}
	question := refinementQuestion(pending, QuestionRequest{})
	if !domainReview {
		question.ClarificationQuery, question.AnswerContext = pending.ID, review.AnswerContext
	}
	question.Answers = semantics.CloneClarificationAnswers(review.Answers)
	if err := s.validateClarificationOrigin(ctx, e, question, "query.execute"); err != nil {
		return PlanResult{}, err
	}
	proof := &IntentReviewEvidence{Legacy: reviewOrigin(old), Preflight: reviewOrigin(pending), AnswerDigest: exec.Hash(review), SelectionDigest: review.SelectionDigest}
	ctx = context.WithValue(ctx, intentReviewKey{}, proof)
	operation := "intent-review:" + exec.Hash([]string{old.ID, pending.ID})
	if origin := generationContinuationState(ctx).pendingOrigin(); origin != nil {
		operation = "resume:" + origin.ID
	}
	var out PlanResult
	compose := func() error {
		var retained QueryRecord
		var err error
		if generationContinuationState(ctx).pendingOrigin() != nil {
			retained, err = s.repo.ReadOperation(ctx, mustScope(e), operation)
		} else {
			reader, ok := s.repo.(IntentReviewReader)
			if !ok {
				return exec.ErrBinding
			}
			retained, err = reader.ReadIntentReview(ctx, mustScope(e), old.ID, pending.ID)
		}
		if err == nil {
			if retained.Session != e.Session() || exec.Hash(retained.IntentReview) != exec.Hash(proof) {
				return store.ErrConflict
			}
			if err := s.verifyIntentReview(ctx, e, retained); err != nil {
				return err
			}
			if retained.GenerationPending != nil {
				if err := s.checkGenerationPending(ctx, e, retained); err != nil {
					return err
				}
				return &generationDecisionError{problem: retained.GenerationPending.Problem}
			}
			out = PlanResult{QueryID: retained.ID, SessionID: retained.Session, Status: "planned", Analytical: cloneAnalyticalReceipt(retained.Analytical), Bindings: publicClarificationBinding(retained.Clarification)}
			return nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		out, err = s.planFresh(ctx, e, question, operation, old.ID, &old, "query.execute")
		return err
	}
	locker, ok := s.repo.(PlanOperationLocker)
	if !ok {
		return PlanResult{}, exec.ErrBinding
	}
	err = locker.WithPlanOperationLock(ctx, mustScope(e), operation, compose)
	return out, err
}

func (s *Service) verifyIntentReview(ctx context.Context, e identity.Envelope, q QueryRecord) error {
	if q.IntentReview == nil {
		return nil
	}
	if !IntentReviewValid(q) {
		return exec.ErrBinding
	}
	p := q.IntentReview
	var old, pending QueryRecord
	for i, origin := range []IntentReviewOrigin{p.Legacy, p.Preflight} {
		saved, err := s.repo.ReadQuery(ctx, mustScope(e), origin.QueryID)
		if err != nil {
			return err
		}
		if saved.Session != e.Session() || saved.Context != q.Context || saved.Revision != origin.Revision || QueryLineageDigest(saved) != origin.Digest || saved.EvidenceStale {
			return exec.ErrBinding
		}
		if i == 0 {
			old = saved
		} else {
			pending = saved
		}
	}
	domainReview := p.SelectionDigest != ""
	if old.AnalyticalVersion < 1 || old.AnalyticalVersion > 6 && (!domainReview || old.AnalyticalVersion != 7) || old.IntentReview != nil {
		return exec.ErrBinding
	}
	var a admission
	var err error
	if domainReview {
		a, err = s.retainedAdmission(ctx, e, old)
	} else {
		a, err = s.currentAdmission(ctx, e, old)
	}
	if err != nil {
		return err
	}
	if domainReview {
		constraints, err := s.replayIntentReviewParent(ctx, e, old, true)
		if err != nil {
			return err
		}
		if err := s.verifyQueryClarificationBinding(ctx, e, old, a); err != nil {
			return err
		}
		if !isGroupedDomainReview(s.reviewLegacyAnalyticalContinuation(ctx, e, old, a, constraints)) {
			return exec.ErrBinding
		}
	}
	b, err := s.intentReviewPreflightAdmission(ctx, e, pending, p.SelectionDigest)
	if err != nil {
		return err
	}
	if exec.Hash(a.binding) != exec.Hash(b.binding) || !reflect.DeepEqual(old.Topics, pending.Topics) || !domainReview && (!reflect.DeepEqual(old.TopicVersions, pending.TopicVersions) || !reflect.DeepEqual(old.RuleVersions, pending.RuleVersions)) {
		return exec.ErrBinding
	}
	return nil
}

func isGroupedDomainReview(err error) bool {
	var failure *exec.AnalyticalError
	return errors.As(err, &failure) && failure.Code == exec.AnalyticalGroupDomainReviewCode
}

// A ready domain-review preflight is still service-owned SQL-empty metadata.
// Its current typed catalog selection, not an arbitrary caller definition, is
// the explicit confirmation target.
func (s *Service) intentReviewPreflightAdmission(ctx context.Context, e identity.Envelope, q QueryRecord, selection string) (admission, error) {
	if selection == "" {
		return s.pendingRefinementAdmission(ctx, e, q)
	}
	if q.Session != e.Session() {
		return admission{}, ErrForeignSession
	}
	if q.Status != "preflight" || q.SQL != "" || len(q.Parameters) != 0 || q.Operation != "" || q.Result != nil || q.EvidenceStale || q.AnalyticalVersion != 0 || q.Analytical != nil || q.Clarification != nil || q.GenerationPending != nil || q.IntentReview != nil || q.Generation.Prompt != "" || q.Generation.Strategy != "" || q.Route.Clarification != nil || q.Route.Selection == nil || q.Route.Selection.Digest != selection || len(q.Route.Request.Answers) != 0 || len(q.Route.Resolutions) != 0 {
		return admission{}, exec.ErrBinding
	}
	question := refinementQuestion(q, QuestionRequest{})
	if err := requireQuestionAction(e, "query.execute", question); err != nil {
		return admission{}, err
	}
	a, err := s.resolveCurrentAdmission(ctx, e, q, s.topics.Contract, s.sources.Binding)
	if err != nil {
		return admission{}, err
	}
	if q.Route.SourceBindingDigest != "" && q.Route.SourceBindingDigest != exec.Hash(a.binding) {
		return admission{}, exec.ErrBinding
	}
	if err := reviewedGroupDomains(ctx, a); err != nil {
		return admission{}, err
	}
	return a, nil
}
func reviewedGroupDomains(ctx context.Context, a admission) error {
	c, err := compileAnalytical(ctx, a)
	if err != nil {
		return err
	}
	if c == nil || c.GroupedPopulations == nil {
		return exec.ErrBinding
	}
	for _, lane := range c.GroupedPopulations.Lanes {
		found := false
		for _, pub := range a.publications {
			if p := pub.Definition.GroupedPopulation; p != nil {
				for _, d := range p.GroupDomains {
					if d.Dataset == lane.Dataset && d.Domain == lane.Domain {
						found = true
					}
				}
			}
		}
		if !found {
			return exec.ErrBinding
		}
	}
	return nil
}

// Domain-only retained v7 has no service-owned scalar predicates. Reinterpreting
// its empty answer context against a different current publication would turn
// historical selection into new intent. Its exact old catalog and native proof
// are checked separately; active business evidence still uses normal replay.
func (s *Service) replayIntentReviewParent(ctx context.Context, e identity.Envelope, q QueryRecord, review bool) ([]exec.BusinessConstraint, error) {
	if review && q.AnalyticalVersion == 7 && !hasActiveBusinessEvidence(q.Route) && !usesGroundedConcepts(q.Route) && len(q.Route.Request.Answers) == 0 {
		return nil, nil
	}
	return s.replayQueryClarifications(ctx, e, q)
}
