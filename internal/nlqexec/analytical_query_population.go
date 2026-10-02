package nlqexec

import (
	"context"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

// New plans consume only the in-process router's protected predicate seal.
func compileCurrentAnalytical(ctx context.Context, a admission) (contract *exec.AnalyticalContract, err error) {
	defer func() {
		if isGroupedDomainReview(err) {
			err = ordinaryGroupDomainReview(a.route.Request.Locale)
		}
	}()
	version := analyticalRecordVersion
	if selectedKnownAmountCompleteness(a) {
		version = analyticalScopedRecordVersion
	}
	if selectedMetricPeriods(a) {
		applications, readErr := a.route.ResolvedMetricPeriodApplications()
		if readErr != nil {
			return nil, readErr
		}
		if len(applications) == 0 {
			return nil, analyticalUnsupported(AnalyticalMetricPeriodReviewCode)
		}
		a.metricPeriods = applications
		version = analyticalScopedRecordVersion
	}
	if !hasActiveBusinessEvidence(a.route) {
		return compileAnalyticalVersion(ctx, a, version)
	}
	constraints, err := a.route.ResolvedBusinessConstraints()
	if err != nil {
		return nil, err
	}
	return compileAnalyticalVersion(ctx, a, version, constraints)
}

func compileQueryPopulation(ctx context.Context, a admission, contract *exec.AnalyticalContract, supplied [][]exec.BusinessConstraint) error {
	if len(supplied) > 1 {
		return exec.ErrBinding
	}
	if !hasActiveBusinessEvidence(a.route) {
		if len(supplied) == 1 && len(supplied[0]) != 0 {
			return exec.ErrBinding
		}
		return nil
	}
	if len(supplied) != 1 {
		return exec.ErrBinding
	}
	if len(supplied[0]) == 0 {
		// Authenticated reference-only answers are selections, not predicates.
		return nil
	}
	if a.route.SourceBindingDigest != exec.Hash(a.binding) {
		return exec.ErrBinding
	}
	var err error
	if contract.Version == exec.AnalyticalGroupedPopulationsVersion || contract.Version == exec.AnalyticalGroupedProgramsVersion {
		ids := make([]string, 0, len(a.relationScope))
		for _, scope := range a.relationScope {
			ids = append(ids, scope.Dataset)
		}
		contract.QueryPopulation, err = exec.NewAnalyticalQueryPopulationWithin(ctx, a.binding, ids, supplied[0])
	} else {
		contract.QueryPopulation, err = exec.NewAnalyticalQueryPopulation(ctx, a.binding, contract.Dataset, supplied[0])
	}
	return err
}

// Persisted route JSON has no in-process predicate seal. Reconstruct through the
// existing authenticated router replay, never by trusting saved resolutions.
func (s *Service) expectedAnalytical(ctx context.Context, e identity.Envelope, q QueryRecord, a admission) (*exec.AnalyticalContract, error) {
	if isScalarPeriodRecord(q) {
		a, constraints, err := s.scalarPeriodAdmission(ctx, e, q, a)
		if err != nil {
			return nil, err
		}
		return expectedAnalytical(ctx, q, a, constraints)
	}
	if q.AnalyticalVersion < 4 || !hasActiveBusinessEvidence(q.Route) {
		return expectedAnalytical(ctx, q, a)
	}
	constraints, err := s.replayQueryClarifications(ctx, e, q)
	if err != nil {
		return nil, err
	}
	return expectedAnalytical(ctx, q, a, constraints)
}

func analyticalPopulationGuidance(contract *exec.AnalyticalContract) string {
	if contract != nil && contract.ScalarPopulations != nil {
		return ""
	}
	if contract == nil || contract.QueryPopulation == nil {
		return ""
	}
	if contract.GroupDomain != nil {
		return analyticalGroupDomainGuidance(contract)
	}
	if contract.Version == exec.AnalyticalGroupedProgramsVersion && contract.GroupedPopulations != nil {
		return " Query-owned predicates are supplied and verified by the service from typed reviewed constraints; do not invent WHERE/HAVING restrictions or inline private values. Each lane has a separately reviewed group-domain policy: follow its exact WHERE placement and retain every aggregate's own FILTER/CASE population. Raw-source groups and qualifying-population groups are not interchangeable."
	}
	return " Query-wide predicates are supplied and verified by the service from typed reviewed constraints. Do not invent extra WHERE or HAVING restrictions or inline private values. Keep metric-specific filters on their own aggregates; only a reviewed filter shared by every selected metric may be moved to WHERE. The service binds the query-wide scalar/time/aggregate predicates after generation."
}

func analyticalGrainGuidance(contract *exec.AnalyticalContract) string {
	return analyticalGrainGuidanceOnly(contract) + analyticalPopulationGuidance(contract) + analyticalIntentGuidance(contract) + analyticalJoinGuidance(contract)
}
