package nlqexec

import (
	"context"

	"github.com/hurtener/chartworks/internal/exec"
)

// These value-free policies distinguish the binder family, not source authority.
// Historical bounds and final group values are never part of a reusable origin.
const (
	ScopedScalarExamplePolicy    = "current-owned-scalar-populations-v1"
	ScopedGroupedExamplePolicy   = "current-owned-grouped-populations-v1"
	ScopedSelectionExamplePolicy = "current-owned-group-selection-v1"
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
	default:
		return ""
	}
}

func scopedLearningPolicy(policy string) bool {
	return policy == ScopedScalarExamplePolicy || policy == ScopedGroupedExamplePolicy || policy == ScopedSelectionExamplePolicy
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
	}
	return ""
}

// Schemas 5 and 6 have no qualified owned-base learning/consumption policy yet.
func groupedFactLearningUnsupported(a admission) bool {
	c, err := compileCurrentAnalytical(context.Background(), a)
	return err == nil && c != nil && (c.Version == exec.AnalyticalGroupedFactsVersion || c.Version == exec.AnalyticalScalarEntailmentVersion)
}
