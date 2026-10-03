package nlqexec

import (
	"context"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

// These value-free policies distinguish the binder family, not source authority.
// Historical bounds and final group values are never part of a reusable origin.
const (
	ScopedScalarExamplePolicy      = "current-owned-scalar-populations-v1"
	ScopedGroupedExamplePolicy     = "current-owned-grouped-populations-v1"
	ScopedSelectionExamplePolicy   = "current-owned-group-selection-v1"
	ScopedGroupedFactExamplePolicy = "current-owned-grouped-fact-predicates-v1"
)

func learningPolicyForBinding(version int) string {
	switch version {
	case 1:
		return OwnedExamplePolicy
	case 2:
		return ScopedScalarExamplePolicy
	case 3:
		return ScopedGroupedExamplePolicy
	case 4:
		return ScopedSelectionExamplePolicy
	case 5:
		return ScopedGroupedFactExamplePolicy
	default:
		return ""
	}
}

func scopedLearningPolicy(policy string) bool {
	return policy == ScopedScalarExamplePolicy || policy == ScopedGroupedExamplePolicy || policy == ScopedSelectionExamplePolicy || policy == ScopedGroupedFactExamplePolicy
}

func ownedLearningPolicy(policy string) bool {
	return policy == OwnedExamplePolicy || scopedLearningPolicy(policy)
}

// Applicability must compile from the current in-process sealed route. Retained
// JSON and a stored policy marker cannot recreate owned population applications.
// The subsequent native binder and analytical proof still validate the actual SQL.
func scopedExampleApplicable(policy string, a admission) bool {
	return scopedLearningPolicy(policy) && policy == currentScopedLearningPolicy(a)
}

func currentScopedLearningPolicy(a admission) string {
	contract, err := compileCurrentAnalytical(context.Background(), a)
	if err != nil || contract == nil {
		return ""
	}
	switch contract.Version {
	case exec.AnalyticalScopedPopulationsVersion:
		if contract.ScalarPopulations != nil {
			return ScopedScalarExamplePolicy
		}
	case exec.AnalyticalGroupedOwnedPopulationsVersion:
		return ScopedGroupedExamplePolicy
	case exec.AnalyticalGroupedSelectionVersion:
		return ScopedSelectionExamplePolicy
	case exec.AnalyticalGroupedFactsVersion:
		return ScopedGroupedFactExamplePolicy
	}
	return ""
}

// Fact predicates require their distinct scoped family. Scalar entailment remains
// unsupported for owned-base learning; neither may borrow the ordinary policy.
func ordinaryOwnedLearningUnsupported(a admission) bool {
	c, err := compileCurrentAnalytical(context.Background(), a)
	return err == nil && c != nil && (c.Version == exec.AnalyticalGroupedFactsVersion || c.Version == exec.AnalyticalScalarEntailmentVersion)
}

// The schema-5 producer proves the complete retained analytical receipt, including
// output ordinals, before converting its exact owned binding to a reusable base.
// Contract/digest shape checks alone cannot establish output identity. Feedback
// supplies its already native-validated bound plan; standalone callers must obtain
// the same opaque plan rather than trusting SQL or retained JSON.
func (s *Service) verifyGroupedFactLearningReceipt(ctx context.Context, e identity.Envelope, q QueryRecord, a admission, provedBound ...*exec.Plan) error {
	contract, err := s.expectedAnalytical(ctx, e, q, a)
	if err != nil {
		return err
	}
	if contract == nil || contract.Version != exec.AnalyticalGroupedFactsVersion {
		return exec.ErrBinding
	}
	var plan exec.Plan
	if len(provedBound) == 1 && provedBound[0] != nil {
		plan = *provedBound[0]
	} else {
		plan, err = s.validator.ValidateWithin(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: q.SQL, Parameters: q.Parameters}, a.relationScope)
		if err != nil {
			return err
		}
	}
	receipt, err := exec.CheckAnalyticalPlan(ctx, plan, *contract)
	if err != nil {
		return err
	}
	if exec.Hash(receipt) != exec.Hash(q.Analytical) {
		return exec.ErrBinding
	}
	return nil
}
