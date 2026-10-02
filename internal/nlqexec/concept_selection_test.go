package nlqexec

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

type groundedReplayProbe struct {
	Router
	calls   int
	failure error
}

func (r *groundedReplayProbe) ReplayClarifications(context.Context, identity.Envelope, nlqroute.RouteResult) ([]exec.BusinessConstraint, string, error) {
	r.calls++
	return nil, "", r.failure
}
func TestSQLRecoveryGroundedConceptRunAlwaysReplays(t *testing.T) {
	r := &groundedReplayProbe{failure: exec.ErrBinding}
	s := Service{router: r}
	q := QueryRecord{Route: nlqroute.RouteResult{Concepts: &nlqroute.ConceptEvidence{}, Request: nlqroute.RouteRequest{ConceptPolicy: nlqroute.GroundedConceptPolicy}}}
	if err := s.verifyQueryClarificationBinding(context.Background(), testEnvelope(t), q, admission{}); !errors.Is(err, exec.ErrBinding) || r.calls != 1 {
		t.Fatal("unfiltered model selection bypassed authenticated replay", err)
	}
	q.Route.Concepts = nil
	if err := s.verifyQueryClarificationBinding(context.Background(), testEnvelope(t), q, admission{}); !errors.Is(err, exec.ErrBinding) || r.calls != 2 {
		t.Fatal("missing proof acquired fallback", err)
	}
	q.Route.Request.ConceptPolicy = ""
	q.Route.Selection = &nlqroute.SemanticSelection{Topics: []nlqroute.SelectedTopic{{Roots: []nlqroute.SelectedRoot{{Reason: "grounded_model"}}}}}
	if err := s.verifyQueryClarificationBinding(context.Background(), testEnvelope(t), q, admission{}); !errors.Is(err, exec.ErrBinding) || r.calls != 3 {
		t.Fatal("missing policy and proof downgraded model roots", err)
	}
	q.Route.Selection = nil
	if err := s.verifyQueryClarificationBinding(context.Background(), testEnvelope(t), q, admission{}); err != nil || r.calls != 3 {
		t.Fatal("legacy path changed", err)
	}
}
func TestSQLRecoveryGroundedConceptSavedPolicyAndEdits(t *testing.T) {
	in := SavedQuestion{Context: "context", Question: "income", Durability: "replayable", Topics: []SavedTopic{{Topic: "topic", Version: "v1"}}, Selections: &SavedSelections{ConceptPolicy: nlqroute.GroundedConceptPolicy}}
	q := savedRouting(in, nlq.LanguageEnglish)
	if q.ConceptPolicy != nlqroute.GroundedConceptPolicy || q.routeRequest().ConceptPolicy != q.ConceptPolicy {
		t.Fatal("saved policy lost")
	}
	record := QueryRecord{Route: nlqroute.RouteResult{Request: q.routeRequest(), Selection: &nlqroute.SemanticSelection{Topics: []nlqroute.SelectedTopic{{Topic: "topic", Roots: []nlqroute.SelectedRoot{{Reference: semantics.Reference{Kind: semantics.KindKPI, ID: "margin"}, Reason: "grounded_model"}, {Reference: semantics.Reference{Kind: semantics.KindMeasure, ID: "cost"}, Reason: "required_rule"}}}}}}}
	if !savedSelectionsMatch(record, in) {
		t.Fatal("saved selection mismatch")
	}
	base := refinementQuestion(record, QuestionRequest{})
	retainCatalogSelection(record, &base)
	if base.ConceptPolicy != nlqroute.GroundedConceptPolicy || len(base.References) != 1 || base.References[0].ID != "margin" || len(base.MetricIDs) != 1 {
		t.Fatal("refinement lost independent model roots or promoted ingredients")
	}
	in.Selections.ConceptPolicy = ""
	if savedSelectionsMatch(record, in) {
		t.Fatal("saved policy silently removed")
	}
}
func TestSQLRecoveryGroundedConceptRejectsForeignPolicyBeforeRouting(t *testing.T) {
	q := QuestionRequest{Context: "context", Locale: nlq.LanguageEnglish, Question: "income", ConceptPolicy: "invented"}
	if validateQuestion(q) == nil || nlqroute.ValidateRequest(q.routeRequest()) == nil {
		t.Fatal("unknown policy accepted")
	}
}
