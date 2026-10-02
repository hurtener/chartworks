package nlqexec

import (
	"context"
	"reflect"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
)

// A pending preflight has no validated SQL/RelationScope to replay. Resolve its
// exact current topic versions and source projection before Refine replays the
// original form and plans a new child. This is NOT a fallback for an executable
// record whose scope is missing, nor permission to modify the parent's receipt.
func (s *Service) pendingRefinementAdmission(ctx context.Context, e identity.Envelope, q QueryRecord) (admission, error) {
	if ctx == nil || !e.Valid() {
		return admission{}, access.ErrUnauthenticated
	}
	if err := ctx.Err(); err != nil {
		return admission{}, err
	}
	if q.Session != e.Session() {
		return admission{}, ErrForeignSession
	}
	if !identity.Identifier(q.ID) || q.Status != "preflight" || q.SQL != "" || len(q.Parameters) != 0 || q.Operation != "" || q.Result != nil || q.EvidenceStale || q.AnalyticalVersion != 0 || q.Analytical != nil || q.Clarification != nil || q.Generation.Prompt != "" || q.Generation.Strategy != "" || q.ValidationFixes != 0 || q.ExecutionFixes != 0 || q.Route.Outcome != nlq.StrategyClarify || q.Route.Clarification == nil || len(q.Topics) == 0 || len(q.Topics) != len(q.TopicVersions) {
		return admission{}, exec.ErrBinding
	}
	// Action and named-resource authority are still required even when no SQL
	// exists. The source/topic readers enforce the exact current dependencies.
	if err := requireQuestionAction(e, "query.execute", QuestionRequest{Topics: q.Topics, Context: q.Context}); err != nil {
		return admission{}, err
	}
	a, err := s.resolveCurrentAdmission(ctx, e, q, s.topics.Contract, s.sources.Binding)
	if err != nil {
		return admission{}, err
	}
	if q.Route.SourceBindingDigest != "" && q.Route.SourceBindingDigest != exec.Hash(a.binding) || len(q.RelationScope) != 0 && !reflect.DeepEqual(q.RelationScope, a.relationScope) {
		return admission{}, exec.ErrBinding
	}
	// Refine still authenticates the original pending answer context and exact
	// semantic selections after this read. Fresh routing and native/analytical
	// validation own the child's plan; no in-process seal is synthesized here.
	return a, nil
}
