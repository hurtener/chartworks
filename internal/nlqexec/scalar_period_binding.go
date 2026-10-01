package nlqexec

import (
	"context"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

type metricPeriodReplayer interface {
	ReplayMetricPeriodApplications(context.Context, identity.Envelope, nlqroute.RouteResult) ([]nlqroute.MetricPeriodApplication, string, error)
}

func bindScalarPeriodCandidate(ctx context.Context, a admission, candidate generatedCandidate) (generatedCandidate, error) {
	if a.analytical == nil || a.analytical.ScalarPopulations == nil && a.analytical.Version != exec.AnalyticalGroupedOwnedPopulationsVersion {
		return generatedCandidate{}, exec.ErrBinding
	}
	bound, err := bindPeriodProgram(ctx, a.binding, candidate.SQL, candidate.Parameters, *a.analytical)
	if err != nil {
		return generatedCandidate{}, err
	}
	candidate.clarification = &ClarificationEvidence{SchemaVersion: 1, BaseSQL: candidate.SQL, Binding: bound.Receipt}
	candidate.SQL, candidate.Parameters = bound.SQL, bound.Parameters
	return candidate, nil
}

func (s *Service) scalarPeriodAdmission(ctx context.Context, e identity.Envelope, q QueryRecord, a admission) (admission, []exec.BusinessConstraint, error) {
	if q.AnalyticalVersion != analyticalScopedRecordVersion && q.AnalyticalVersion != analyticalGroupedOwnedRecordVersion {
		return admission{}, nil, exec.ErrBinding
	}
	replayer, ok := s.router.(metricPeriodReplayer)
	if !ok {
		return admission{}, nil, exec.ErrBinding
	}
	applications, binding, err := replayer.ReplayMetricPeriodApplications(ctx, e, q.Route)
	if err != nil {
		return admission{}, nil, err
	}
	if len(applications) == 0 || binding != exec.Hash(a.binding) {
		return admission{}, nil, exec.ErrBinding
	}
	a.metricPeriods = applications
	constraints, err := s.replayQueryClarifications(ctx, e, q)
	if err != nil {
		return admission{}, nil, err
	}
	if len(constraints) != 0 {
		return admission{}, nil, analyticalUnsupported("analytical_query_population_unsupported")
	}
	return a, constraints, nil
}

func (s *Service) verifyScalarPeriodBinding(ctx context.Context, e identity.Envelope, q QueryRecord, a admission) error {
	if !clarificationBindingSchemaValid(q) || q.Clarification.SchemaVersion != 1 || q.Clarification.BaseSQL == "" || len(q.Clarification.BaseParameters) != 0 {
		return exec.ErrBinding
	}
	evidence := q.Clarification
	v := evidence.Binding.Validation
	if v == nil || !v.Validated || v.Source != a.binding.Source || v.Context != a.binding.Context || v.Dialect != a.binding.Dialect || v.Contract != a.binding.Contract || evidence.Binding.SourceBinding != exec.Hash(a.binding) {
		return exec.ErrBinding
	}
	a, constraints, err := s.scalarPeriodAdmission(ctx, e, q, a)
	if err != nil {
		return err
	}
	contract, err := expectedAnalytical(ctx, q, a, constraints)
	if err != nil {
		return err
	}
	if contract == nil || contract.ScalarPopulations == nil && contract.Version != exec.AnalyticalGroupedOwnedPopulationsVersion {
		return exec.ErrBinding
	}
	bound, err := bindPeriodProgram(ctx, a.binding, evidence.BaseSQL, evidence.BaseParameters, *contract)
	if err != nil {
		return err
	}
	want := evidence.Binding
	want.Validation = nil
	if bound.SQL != q.SQL || !parametersEqual(bound.Parameters, q.Parameters) || exec.Hash(bound.Receipt) != exec.Hash(want) {
		return exec.ErrBinding
	}
	return nil
}

func clarificationBindingSchemaValid(q QueryRecord) bool {
	if q.Clarification == nil {
		return false
	}
	b := q.Clarification.Binding
	if b.SchemaVersion == 1 {
		return !isScalarPeriodRecord(q) && !isGroupedPeriodRecord(q) && b.PopulationPolicy == ""
	}
	valid := b.SchemaVersion == 2 && isScalarPeriodRecord(q) && b.PopulationPolicy == exec.AnalyticalScalarPopulationPolicy || b.SchemaVersion == 3 && isGroupedPeriodRecord(q) && b.PopulationPolicy == exec.AnalyticalGroupedOwnedPopulationPolicy
	if !valid || len(b.Bindings) < 2 || len(b.Bindings) > 4 {
		return false
	}
	seen := map[string]bool{}
	for _, binding := range b.Bindings {
		if binding.Population == "" || b.SchemaVersion == 3 && seen[binding.Population] {
			return false
		}
		seen[binding.Population] = true
	}
	return true
}

func analyticalVersionForReceipt(r *exec.AnalyticalReceipt) int {
	if r != nil && r.Version == exec.AnalyticalGroupedOwnedPopulationsVersion && analyticalReceiptScopeValid(r) && topics.DigestValid(r.Contract) && topics.DigestValid(r.Query) {
		return analyticalGroupedOwnedRecordVersion
	}
	if r != nil && r.Version == exec.AnalyticalScopedPopulationsVersion && analyticalReceiptScopeValid(r) && topics.DigestValid(r.Contract) && topics.DigestValid(r.Query) {
		return analyticalScopedRecordVersion
	}
	return analyticalRecordVersion
}

func isScalarPeriodRecord(q QueryRecord) bool {
	return q.AnalyticalVersion == analyticalScopedRecordVersion && q.Analytical != nil && strings.HasSuffix(q.Analytical.Scope, ";independent_scoped_singleton_populations")
}

func isGroupedPeriodRecord(q QueryRecord) bool {
	return q.AnalyticalVersion == analyticalGroupedOwnedRecordVersion && q.Analytical != nil && strings.HasSuffix(q.Analytical.Scope, ";independent_owned_grouped_populations")
}

func bindPeriodProgram(ctx context.Context, binding exec.Binding, statement string, parameters []exec.Parameter, c exec.AnalyticalContract) (exec.BusinessBoundQuery, error) {
	if c.Version == exec.AnalyticalGroupedOwnedPopulationsVersion {
		return exec.BindGroupedPopulationConstraints(ctx, binding, statement, parameters, c)
	}
	return exec.BindScalarPopulationConstraints(ctx, binding, statement, parameters, c)
}
