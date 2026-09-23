package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

var (
	// ErrPerformanceEvidence rejects a stale or mismatched Phase 24 proof.
	ErrPerformanceEvidence = errors.New("evaluation: performance evidence unavailable")
	// ErrPerformanceAuthority rejects a profile that does not match current verified reach.
	ErrPerformanceAuthority = errors.New("evaluation: performance authority mismatch")
)

// PerformanceReleaseEvidence is the exact accepted Phase 24 suite, its passing
// report, the selected case and the independently reviewed runtime pack. It is
// loaded from protected storage for each release run; a profile cannot supply it.
type PerformanceReleaseEvidence struct {
	Suite       SuiteRecord
	Report      Report
	Case        Case
	CaseResult  CaseResult
	RuntimePack RuntimePackRecord
}

// PerformanceScenarioEvidence binds one allowed measurement step to its own
// accepted workload, report-selected runtime pack and current revision set.
type PerformanceScenarioEvidence struct {
	Evidence  PerformanceReleaseEvidence
	Revisions PerformanceRevisionEvidence
}

// PerformanceRevisionEvidence is the database/source-bound portion of the
// current release snapshot. The phase-34 composition supplies this from the
// selected migration head and actual source/topic/context stores.
type PerformanceRevisionEvidence struct {
	TargetTenant string
	// WorkloadReport is the protected execution target; frozen consumers use
	// their published block ID and signed block.execute reach.
	WorkloadReport string
	SourceID       string
	ContextID      string
	SourceRevision string
	RuleRevision   string
	TopicRevision  string
	DatasetDigest  string
	DatasetRows    int64
	// The following raw pins are protected current-store evidence for the
	// frozen-run adapter. Profile bindings retain only their digests.
	BlockID       string
	BlockRevision int64
	BlockDigest   string
	SourceHead    int64
	TopicPins     []reporting.TopicPin
	RulePins      []reporting.RulePin
}

// PerformanceRevisionResolver reads current source and semantic revisions
// through their owning services. Implementations must fail closed when they
// cannot prove the selected Phase-34 revision set.
type PerformanceRevisionResolver interface {
	ResolvePerformanceRevisions(context.Context, identity.Envelope, PerformanceReleaseEvidence) (PerformanceRevisionEvidence, error)
}

// PerformanceReleaseAdapter is a concrete execution adapter around the
// existing PostgreSQL/source and configured Bifrost boundaries. Integration
// adapters identify recorded-model evidence; live adapters identify live
// provider evidence. Synthetic runners cannot satisfy this interface.
type PerformanceReleaseAdapter interface {
	PerformanceRunner
	EvidenceMode() PerformanceEvidenceMode
	SourceMode() string
	ModelMode() string
}

// PerformanceReleaseAdapterFactory composes a release adapter after authority,
// current Phase 24 evidence and current revisions have all been resolved.
type PerformanceReleaseAdapterFactory interface {
	NewPerformanceReleaseAdapter(context.Context, identity.Envelope, PerformanceManifest, map[string]PerformanceScenarioEvidence) (PerformanceReleaseAdapter, error)
}

// PerformanceRevisionTransition is an operator-composed, authorized product
// transition. It must make the selected accepted consumer's owner pins current;
// the resolver checks those pins after the transition and after measurement.
// An absent transition cannot stand in for an owner publication or cutover.
type PerformanceRevisionTransition interface {
	PreparePerformanceRevision(context.Context, identity.Envelope, PerformanceStep, PerformanceReleaseEvidence) error
}

type orderedPerformanceAdapterFactory interface {
	NewOrderedPerformanceReleaseAdapter(context.Context, identity.Envelope, PerformanceManifest, map[string]PerformanceReleaseEvidence) (PerformanceReleaseAdapter, error)
}

type performanceScenarioBinder interface {
	BindPerformanceScenario(context.Context, PerformanceStep, PerformanceScenarioEvidence) error
}

type performanceTokenVerifier interface {
	Verify(context.Context, string, auth.Surface) (identity.Envelope, error)
}

// PerformanceReleaseRuntime owns the fail-closed Phase 25 release path. The
// bearer is verified independently of the profile, and no manifest value can
// construct or replace its envelope.
type PerformanceReleaseRuntime struct {
	Verifier   performanceTokenVerifier
	Service    *Service
	Revisions  PerformanceRevisionResolver
	Transition PerformanceRevisionTransition
	Adapters   PerformanceReleaseAdapterFactory
	Clock      Clock
}

// Measure resolves the immutable current evidence and executes a final profile
// only with verifier-produced envelopes. The second short-lived bearer carries
// the same subject/reach with one query action removed for the governed denial.
func (r *PerformanceReleaseRuntime) Measure(ctx context.Context, bearer, actionNegativeBearer string, manifest PerformanceManifest) (PerformanceReport, error) {
	if r == nil || ctx == nil || bearer == "" || actionNegativeBearer == "" || r.Verifier == nil || r.Service == nil || r.Revisions == nil || r.Adapters == nil || manifest.Kind != PerformanceFinalStress || manifest.EvidenceMode == PerformanceSynthetic {
		return PerformanceReport{}, ErrMode
	}
	envelope, err := r.Verifier.Verify(ctx, bearer, auth.HTTP)
	if err != nil || !envelope.Valid() {
		return PerformanceReport{}, ErrPerformanceAuthority
	}
	if manifest.Authority.Tenant != envelope.Tenant() || manifest.Authority.User != envelope.User() || manifest.Authority.Session != envelope.Session() || !sameStrings(manifest.Authority.Scopes, envelope.Scopes()) {
		return PerformanceReport{}, ErrPerformanceAuthority
	}
	if manifest.Validate() != nil {
		return PerformanceReport{}, ErrInvalid
	}
	actionStep, ok := performanceStep(manifest.Steps, "actions_negative")
	if !ok || actionStep.AuthorityOverride == nil {
		return PerformanceReport{}, ErrInvalid
	}
	actionEnvelope, err := r.Verifier.Verify(ctx, actionNegativeBearer, auth.HTTP)
	if err != nil || !actionEnvelope.Valid() || actionEnvelope.Tenant() != envelope.Tenant() || actionEnvelope.User() != envelope.User() || actionEnvelope.Session() != envelope.Session() || !sameStrings(actionEnvelope.Scopes(), actionStep.AuthorityOverride.Scopes) || actionEnvelope.Has(actionStep.DeniedAction) {
		return PerformanceReport{}, ErrPerformanceAuthority
	}
	host := RuntimePerformanceEnvironment(manifest.Environment.RunnerLabel)
	if manifest.Environment.OS != host.OS || manifest.Environment.Architecture != host.Architecture || manifest.Environment.CPUs != host.CPUs || manifest.Environment.GoVersion != host.GoVersion {
		return PerformanceReport{}, ErrPerformanceEvidence
	}
	evidence, err := r.Service.ResolvePerformanceEvidence(ctx, envelope, manifest.Environment)
	if err != nil {
		return PerformanceReport{}, err
	}
	revisions, err := r.Revisions.ResolvePerformanceRevisions(ctx, envelope, evidence)
	if err != nil {
		return PerformanceReport{}, ErrPerformanceEvidence
	}
	if err = validatePerformanceReleaseInputs(envelope, manifest, evidence, revisions); err != nil {
		return PerformanceReport{}, err
	}
	if performanceCurrentBinding(actionEnvelope, revisions.ContextID, evidence, revisions) != actionStep.Binding {
		return PerformanceReport{}, ErrPerformanceEvidence
	}
	if r.Transition != nil {
		return r.measureOrdered(ctx, envelope, actionEnvelope, manifest, evidence, revisions)
	}
	scenarios, err := r.resolvePerformanceScenarios(ctx, envelope, manifest, evidence, revisions)
	if err != nil {
		return PerformanceReport{}, err
	}
	if err = validatePerformanceScenarioEvidence(envelope, manifest, evidence, scenarios); err != nil {
		return PerformanceReport{}, err
	}
	adapter, err := r.Adapters.NewPerformanceReleaseAdapter(ctx, envelope, manifest, scenarios)
	if errors.Is(err, ErrPerformanceReuseUnproven) {
		return PerformanceReport{}, err
	}
	if err != nil || adapter == nil || adapter.EvidenceMode() != manifest.EvidenceMode || adapter.SourceMode() != manifest.Environment.SourceMode || adapter.ModelMode() != manifest.Environment.ModelMode {
		return PerformanceReport{}, ErrMode
	}
	bound := &authorityBoundPerformanceRunner{envelope: envelope, actionEnvelope: actionEnvelope, manifest: manifest, targetKind: performanceTargetKind(revisions), scenarios: scenarios, next: adapter}
	report, err := MeasurePerformance(ctx, manifest, bound, r.Clock)
	if restore, ok := adapter.(interface{ RestorePerformanceSelection(context.Context) error }); ok {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		restoreErr := restore.RestorePerformanceSelection(cleanup)
		cancel()
		if restoreErr != nil {
			return report, errors.Join(err, restoreErr)
		}
	}
	if err != nil {
		return report, err
	}
	if report.Validate(manifest) != nil {
		return report, ErrPerformanceEvidence
	}
	return report, nil
}

func (r *PerformanceReleaseRuntime) measureOrdered(ctx context.Context, envelope, actionEnvelope identity.Envelope, manifest PerformanceManifest, base PerformanceReleaseEvidence, baseRevisions PerformanceRevisionEvidence) (PerformanceReport, error) {
	if !validOrderedPerformanceSteps(manifest.Steps) {
		return PerformanceReport{}, ErrInvalid
	}
	factory, ok := r.Adapters.(orderedPerformanceAdapterFactory)
	if !ok {
		return PerformanceReport{}, ErrMode
	}
	evidence, err := r.resolvePerformanceScenarioReports(ctx, envelope, manifest, base)
	if err != nil {
		return PerformanceReport{}, err
	}
	adapter, err := factory.NewOrderedPerformanceReleaseAdapter(ctx, envelope, manifest, evidence)
	if err != nil || adapter == nil {
		if err != nil {
			return PerformanceReport{}, err
		}
		return PerformanceReport{}, ErrMode
	}
	if adapter.EvidenceMode() != manifest.EvidenceMode || adapter.SourceMode() != manifest.Environment.SourceMode || adapter.ModelMode() != manifest.Environment.ModelMode {
		return PerformanceReport{}, ErrMode
	}
	binder, ok := adapter.(performanceScenarioBinder)
	if !ok {
		return PerformanceReport{}, ErrMode
	}
	baseStep, _ := performanceStep(manifest.Steps, "cold")
	baseScenario := PerformanceScenarioEvidence{Evidence: base, Revisions: baseRevisions}
	if err := validatePerformanceScenarioStep(envelope, manifest, base, baseRevisions, baseStep, baseScenario); err != nil {
		return PerformanceReport{}, err
	}
	runner := &orderedPerformanceRunner{
		authorityBoundPerformanceRunner: authorityBoundPerformanceRunner{envelope: envelope, actionEnvelope: actionEnvelope, manifest: manifest, targetKind: performanceTargetKind(baseRevisions), scenarios: map[string]PerformanceScenarioEvidence{}, next: adapter},
		transition:                      r.Transition, revisions: r.Revisions, evidence: evidence, base: baseScenario, binder: binder,
	}
	report, runErr := MeasurePerformanceOrdered(ctx, manifest, runner, r.Clock)
	if restore, ok := adapter.(interface{ RestorePerformanceSelection(context.Context) error }); ok {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		restoreErr := restore.RestorePerformanceSelection(cleanup)
		cancel()
		if restoreErr != nil {
			return report, errors.Join(runErr, restoreErr)
		}
	}
	if runErr != nil {
		return report, runErr
	}
	if report.Validate(manifest) != nil {
		return report, ErrPerformanceEvidence
	}
	return report, nil
}

func validOrderedPerformanceSteps(steps []PerformanceStep) bool {
	want := []string{"cold", "warm", "repeat", "concurrent", "tenant_negative", "context_negative", "actions_negative", "runtime_pack_changed", "rule_changed", "topic_changed", "source_changed", "context_changed"}
	if len(steps) != len(want) {
		return false
	}
	for i, step := range steps {
		if step.Kind != want[i] {
			return false
		}
	}
	return true
}

type orderedPerformanceRunner struct {
	authorityBoundPerformanceRunner
	transition PerformanceRevisionTransition
	revisions  PerformanceRevisionResolver
	evidence   map[string]PerformanceReleaseEvidence
	base       PerformanceScenarioEvidence
	binder     performanceScenarioBinder
}

func (r *orderedPerformanceRunner) PreparePerformanceStep(ctx context.Context, step PerformanceStep) error {
	if r == nil || ctx == nil || r.transition == nil || r.revisions == nil || r.binder == nil {
		return ErrMode
	}
	if !step.Allowed {
		return r.authorityBoundPerformanceRunner.PreparePerformanceStep(ctx, step)
	}
	evidence, ok := r.evidence[step.ID]
	if !ok {
		return ErrPerformanceEvidence
	}
	if err := r.transition.PreparePerformanceRevision(ctx, r.envelope, step, evidence); err != nil {
		return err
	}
	revisions, err := r.revisions.ResolvePerformanceRevisions(ctx, r.envelope, evidence)
	if err != nil {
		return ErrPerformanceEvidence
	}
	scenario := PerformanceScenarioEvidence{Evidence: evidence, Revisions: revisions}
	if err := validatePerformanceScenarioStep(r.envelope, r.manifest, r.base.Evidence, r.base.Revisions, step, scenario); err != nil {
		return err
	}
	if step.Kind == "cold" && !reflect.DeepEqual(revisions, r.base.Revisions) {
		return ErrPerformanceEvidence
	}
	if err := r.binder.BindPerformanceScenario(ctx, step, scenario); err != nil {
		return err
	}
	r.scenarios[step.ID] = scenario
	return r.authorityBoundPerformanceRunner.PreparePerformanceStep(ctx, step)
}

func (r *orderedPerformanceRunner) CurrentPerformanceTransition(step PerformanceStep) (PerformanceTransitionEvidence, error) {
	if r == nil || !step.Allowed {
		return PerformanceTransitionEvidence{}, ErrPerformanceEvidence
	}
	scenario, ok := r.scenarios[step.ID]
	if !ok || !frozenRevisionsComplete(scenario.Revisions) {
		return PerformanceTransitionEvidence{}, ErrPerformanceEvidence
	}
	rev := scenario.Revisions
	return PerformanceTransitionEvidence{
		StepID: step.ID, BindingDigest: step.Binding.digest(), SourceID: rev.SourceID, ContextID: rev.ContextID,
		SourceHead: rev.SourceHead, BlockID: rev.BlockID, BlockRevision: rev.BlockRevision, BlockDigest: rev.BlockDigest,
		TopicPinsDigest: performanceValueDigest(rev.TopicPins), RulePinsDigest: performanceValueDigest(rev.RulePins),
	}, nil
}

func (r *orderedPerformanceRunner) PostPerformanceStep(ctx context.Context, step PerformanceStep) error {
	if r == nil || ctx == nil {
		return ErrPerformanceEvidence
	}
	if !step.Allowed {
		return nil
	}
	scenario, ok := r.scenarios[step.ID]
	if !ok {
		return ErrPerformanceEvidence
	}
	current, err := r.revisions.ResolvePerformanceRevisions(ctx, r.envelope, scenario.Evidence)
	if err != nil || !reflect.DeepEqual(current, scenario.Revisions) {
		return ErrPerformanceEvidence
	}
	return nil
}

func (r *PerformanceReleaseRuntime) resolvePerformanceScenarioReports(ctx context.Context, e identity.Envelope, manifest PerformanceManifest, base PerformanceReleaseEvidence) (map[string]PerformanceReleaseEvidence, error) {
	out := make(map[string]PerformanceReleaseEvidence, len(manifest.Steps))
	type evidenceKey struct{ caseID, reportID, reportHash string }
	baseKey := evidenceKey{manifest.Environment.WorkloadCaseID, manifest.Environment.EvaluationReportID, manifest.Environment.EvaluationReport}
	cache := map[evidenceKey]PerformanceReleaseEvidence{baseKey: base}
	for _, step := range manifest.Steps {
		if !step.Allowed {
			continue
		}
		key := baseKey
		if step.EvidenceCaseID != "" {
			key.caseID = step.EvidenceCaseID
		}
		if step.EvidenceReportID != "" {
			key.reportID, key.reportHash = step.EvidenceReportID, step.EvidenceReport
		}
		selected, ok := cache[key]
		if !ok {
			environment := manifest.Environment
			environment.WorkloadCaseID, environment.EvaluationReportID, environment.EvaluationReport = key.caseID, key.reportID, key.reportHash
			var err error
			selected, err = r.Service.ResolvePerformanceEvidence(ctx, e, environment)
			if err != nil {
				return nil, err
			}
			cache[key] = selected
		}
		if !validPerformanceScenarioRecord(base, step, selected) {
			return nil, ErrPerformanceEvidence
		}
		out[step.ID] = selected
	}
	return out, nil
}

func (r *PerformanceReleaseRuntime) resolvePerformanceScenarios(ctx context.Context, e identity.Envelope, manifest PerformanceManifest, base PerformanceReleaseEvidence, baseRevisions PerformanceRevisionEvidence) (map[string]PerformanceScenarioEvidence, error) {
	out := make(map[string]PerformanceScenarioEvidence, len(manifest.Steps))
	type evidenceKey struct{ caseID, reportID, reportHash string }
	cache := map[evidenceKey]PerformanceScenarioEvidence{}
	baseKey := evidenceKey{manifest.Environment.WorkloadCaseID, manifest.Environment.EvaluationReportID, manifest.Environment.EvaluationReport}
	cache[baseKey] = PerformanceScenarioEvidence{Evidence: base, Revisions: baseRevisions}
	for _, step := range manifest.Steps {
		if !step.Allowed {
			continue
		}
		key := baseKey
		if step.EvidenceCaseID != "" {
			key.caseID = step.EvidenceCaseID
		}
		if step.EvidenceReportID != "" {
			key.reportID, key.reportHash = step.EvidenceReportID, step.EvidenceReport
		}
		scenario, ok := cache[key]
		if !ok {
			environment := manifest.Environment
			environment.WorkloadCaseID = key.caseID
			environment.EvaluationReportID = key.reportID
			environment.EvaluationReport = key.reportHash
			evidence, err := r.Service.ResolvePerformanceEvidence(ctx, e, environment)
			if err != nil {
				return nil, err
			}
			revisions, err := r.Revisions.ResolvePerformanceRevisions(ctx, e, evidence)
			if err != nil {
				return nil, ErrPerformanceEvidence
			}
			scenario = PerformanceScenarioEvidence{Evidence: evidence, Revisions: revisions}
			cache[key] = scenario
		}
		out[step.ID] = scenario
	}
	return out, nil
}

// ResolvePerformanceEvidence loads and verifies an exact accepted Phase 24
// suite/report pair and its accepted runtime pack under current signed scope.
func (s *Service) ResolvePerformanceEvidence(ctx context.Context, e identity.Envelope, environment PerformanceEnvironment) (PerformanceReleaseEvidence, error) {
	if s == nil || ctx == nil || !e.Valid() || environment.EvaluationSuiteRevision < 1 || !identifier(environment.EvaluationSuiteID) || !validDigest(environment.EvaluationSuite) || !identifier(environment.EvaluationReportID) || !validDigest(environment.EvaluationReport) || !identifier(environment.WorkloadCaseID) {
		return PerformanceReleaseEvidence{}, ErrPerformanceEvidence
	}
	scope, err := access.StoreScope(e, "ops.write", "write")
	if err != nil {
		return PerformanceReleaseEvidence{}, ErrPerformanceAuthority
	}
	suite, err := s.repo.AcceptedSuite(ctx, scope, environment.EvaluationSuiteID, environment.EvaluationSuiteRevision, environment.EvaluationSuite)
	if err != nil || suite.State != Accepted || suite.Review == nil || suite.Review.Decision != Accepted || suite.Review.Digest != suite.Digest || suite.Review.Reviewer == suite.Author || suite.Digest != environment.EvaluationSuite {
		return PerformanceReleaseEvidence{}, ErrPerformanceEvidence
	}
	suiteDigest, digestErr := suite.Suite.Digest()
	if digestErr != nil || suiteDigest != suite.Digest || suite.Suite.Mode != Live {
		return PerformanceReleaseEvidence{}, ErrPerformanceEvidence
	}
	report, err := s.repo.ReadReport(ctx, scope, environment.EvaluationReportID)
	if err != nil || report.Validate() != nil || report.RunID != environment.EvaluationReportID || report.EvidenceHash != environment.EvaluationReport || report.SuiteID != suite.Suite.ID || report.SuiteRevision != suite.Suite.Revision || report.SuiteDigest != suite.Digest || report.Mode != Live || report.Status != "passed" || !report.GatePassed || report.SecurityFailures != 0 {
		return PerformanceReleaseEvidence{}, ErrPerformanceEvidence
	}
	selectedCase, found := performanceCase(suite.Suite.Cases, environment.WorkloadCaseID)
	selectedResult, resultFound := performanceCaseResult(report.Cases, environment.WorkloadCaseID)
	if !found || !resultFound || selectedCase.Critical || !selectedResult.Passed || !performanceExpected(selectedCase.Expected, selectedResult.Observation) {
		return PerformanceReleaseEvidence{}, ErrPerformanceEvidence
	}
	pack, ok := suite.Suite.pack(report.Pack.Digest)
	if !ok || pack.Digest != report.Pack.Digest || pack.CanonicalDigest() != report.Pack.CanonicalDigest() {
		return PerformanceReleaseEvidence{}, ErrPerformanceEvidence
	}
	runtime, err := s.repo.AcceptedRuntimePack(ctx, scope, pack.Digest, pack.ConfigurationDigest)
	if err != nil || !validRuntimePack(runtime) || runtime.State != Accepted || runtime.Review == nil || runtime.Review.Decision != Accepted || runtime.Review.Reviewer == runtime.Author || runtime.Pack.Digest != pack.Digest || runtime.Pack.ID != pack.ID || runtime.Pack.Revision != pack.Revision || runtime.Pack.ConfigurationDigest != pack.ConfigurationDigest || runtime.Review.PackID != pack.ID || runtime.Review.PackRevision != pack.Revision || runtime.Review.RuntimeDigest != runtime.Digest || runtime.Review.PackDigest != pack.Digest || runtime.Review.ConfigurationDigest != pack.ConfigurationDigest || runtime.Review.MaxAttemptCostUSD != runtime.Config.AttemptCostUSD {
		return PerformanceReleaseEvidence{}, ErrPerformanceEvidence
	}
	return PerformanceReleaseEvidence{Suite: suite, Report: report, Case: selectedCase, CaseResult: selectedResult, RuntimePack: runtime}, nil
}

func performanceCase(cases []Case, id string) (Case, bool) {
	for _, candidate := range cases {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return Case{}, false
}

func performanceCaseResult(cases []CaseResult, id string) (CaseResult, bool) {
	for _, candidate := range cases {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return CaseResult{}, false
}

func performanceExpected(expected []Expected, observation Observation) bool {
	for _, candidate := range expected {
		if candidate.Decision == observation.Decision && candidate.SemanticDigest == observation.SemanticDigest && candidate.ErrorClass == observation.ErrorClass {
			return true
		}
	}
	return false
}

func validatePerformanceReleaseInputs(e identity.Envelope, m PerformanceManifest, evidence PerformanceReleaseEvidence, revisions PerformanceRevisionEvidence) error {
	if !e.Valid() || evidence.Suite.State != Accepted || evidence.Report.Status != "passed" || revisions.TargetTenant != e.Tenant() || revisions.WorkloadReport == "" || revisions.BlockID != "" && revisions.WorkloadReport != revisions.BlockID || revisions.SourceID != m.Authority.SourceID || revisions.ContextID != m.Authority.ContextID || revisions.WorkloadReport != m.Authority.ReportID || revisions.DatasetDigest != m.Environment.DatasetDigest || revisions.DatasetRows != m.Environment.DatasetRows || !validDigest(revisions.SourceRevision) || !validDigest(revisions.RuleRevision) || !validDigest(revisions.TopicRevision) || !validDigest(evidence.CaseResult.Observation.SemanticDigest) {
		return ErrPerformanceEvidence
	}
	coldStep, found := performanceStep(m.Steps, "cold")
	if !found || coldStep.Workload != m.Environment.WorkloadCaseID || coldStep.ExpectedDigest != evidence.CaseResult.Observation.SemanticDigest {
		return ErrPerformanceEvidence
	}
	if m.Authority.Tenant != e.Tenant() || m.Authority.User != e.User() || m.Authority.Session != e.Session() || m.Authority.TargetTenant != e.Tenant() || !sameStrings(m.Authority.Scopes, e.Scopes()) {
		return ErrPerformanceAuthority
	}
	current := performanceCurrentBinding(e, m.Authority.ContextID, evidence, revisions)
	var cold *PerformanceStep
	for i := range m.Steps {
		if m.Steps[i].Kind == "cold" {
			cold = &m.Steps[i]
			break
		}
	}
	if cold == nil || cold.Binding != current {
		return ErrPerformanceEvidence
	}
	return nil
}

func validatePerformanceScenarioEvidence(e identity.Envelope, m PerformanceManifest, base PerformanceReleaseEvidence, scenarios map[string]PerformanceScenarioEvidence) error {
	cold, found := performanceStep(m.Steps, "cold")
	if !found {
		return ErrPerformanceEvidence
	}
	baseScenario, ok := scenarios[cold.ID]
	if !ok {
		return ErrPerformanceEvidence
	}
	for _, step := range m.Steps {
		if !step.Allowed {
			continue
		}
		scenario, ok := scenarios[step.ID]
		if !ok {
			return ErrPerformanceEvidence
		}
		if err := validatePerformanceScenarioStep(e, m, base, baseScenario.Revisions, step, scenario); err != nil {
			return err
		}
	}
	return nil
}

func validPerformanceScenarioRecord(base PerformanceReleaseEvidence, step PerformanceStep, ev PerformanceReleaseEvidence) bool {
	return ev.Suite.State == Accepted && ev.Suite.Digest == base.Suite.Digest && ev.Suite.Suite.ID == base.Suite.Suite.ID && ev.Suite.Suite.Revision == base.Suite.Suite.Revision && ev.Case.ID == step.Workload && ev.Case.ID == ev.CaseResult.ID && ev.Case.ID != "" && ev.Case.Stage == StageConsumer && !ev.Case.Critical && ev.CaseResult.Passed && performanceExpected(ev.Case.Expected, ev.CaseResult.Observation) && ev.CaseResult.Observation.SemanticDigest == step.ExpectedDigest
}

func validatePerformanceScenarioStep(e identity.Envelope, m PerformanceManifest, base PerformanceReleaseEvidence, baseRevisions PerformanceRevisionEvidence, step PerformanceStep, scenario PerformanceScenarioEvidence) error {
	ev, rev := scenario.Evidence, scenario.Revisions
	if !validPerformanceScenarioRecord(base, step, ev) {
		return ErrPerformanceEvidence
	}
	targetKind := performanceTargetKind(baseRevisions)
	if rev.TargetTenant != e.Tenant() || !identifier(rev.WorkloadReport) || rev.BlockID != "" && rev.WorkloadReport != rev.BlockID || performanceTargetKind(rev) != targetKind || !identifier(rev.SourceID) || !identifier(rev.ContextID) || rev.DatasetDigest != m.Environment.DatasetDigest || rev.DatasetRows != m.Environment.DatasetRows || !validDigest(rev.SourceRevision) || !validDigest(rev.RuleRevision) || !validDigest(rev.TopicRevision) {
		return ErrPerformanceEvidence
	}
	request := access.Execution{
		Target:       access.Resource{Tenant: rev.TargetTenant, Kind: targetKind, Permission: "execute", ID: rev.WorkloadReport},
		Dependencies: []access.Resource{{Tenant: rev.TargetTenant, Kind: "source", Permission: "query", ID: rev.SourceID}},
		Contexts:     []access.Resource{{Tenant: rev.TargetTenant, Kind: "execution_context", Permission: "use", ID: rev.ContextID}},
	}
	if err := access.RequireExecution(e, request); err != nil {
		return ErrPerformanceAuthority
	}
	if performanceCurrentBinding(e, rev.ContextID, ev, rev) != step.Binding || targetKind == "block" && !validFrozenDependencyClosure(baseRevisions, rev, step.Kind) {
		return ErrPerformanceEvidence
	}
	return nil
}

// The current resolver proves each block pin against its owner. This check
// requires the candidate's raw owner pins to change in the same dependency
// closure as the declared primary axis, so a profile-only cohort hash cannot
// stand in for an actual published block/source transition.
func validFrozenDependencyClosure(base, next PerformanceRevisionEvidence, kind string) bool {
	if !frozenRevisionsComplete(base) || !frozenRevisionsComplete(next) || base.TargetTenant != next.TargetTenant || base.WorkloadReport != next.WorkloadReport || base.BlockID != next.BlockID || len(base.TopicPins) != len(next.TopicPins) || len(base.RulePins) != len(next.RulePins) {
		return false
	}
	for i := range base.TopicPins {
		if base.TopicPins[i].Topic != next.TopicPins[i].Topic || base.RulePins[i].Topic != next.RulePins[i].Topic {
			return false
		}
	}
	context := base.ContextID != next.ContextID
	sourceID := base.SourceID != next.SourceID
	sourceHead := base.SourceHead != next.SourceHead
	topic := !reflect.DeepEqual(base.TopicPins, next.TopicPins)
	rule := !reflect.DeepEqual(base.RulePins, next.RulePins)
	block := base.BlockRevision != next.BlockRevision || base.BlockDigest != next.BlockDigest
	switch kind {
	case "source_changed":
		// Sources derive their context from both ID and revision. Moving to a
		// different current source must carry its context and republish the
		// dependent topic, rules and block. Head numbers may both be one.
		return sourceID && context && topic && rule && block
	case "rule_changed":
		return rule && block && !context && !sourceID && !sourceHead && !topic
	case "context_changed":
		// Rotating one source increments its head and changes its context.
		return !sourceID && context && sourceHead && topic && rule && block
	case "topic_changed":
		return topic && rule && block && !context && !sourceID && !sourceHead
	case "runtime_pack_changed", "cold", "warm", "repeat", "concurrent":
		return !context && !sourceID && !sourceHead && !topic && !rule && !block
	default:
		return false
	}
}

func performanceTargetKind(revisions PerformanceRevisionEvidence) string {
	if revisions.BlockID != "" {
		return "block"
	}
	return "report"
}

func (r *authorityBoundPerformanceRunner) workloadKind() string {
	if r.targetKind == "block" {
		return "block"
	}
	return "report"
}

func performanceStep(steps []PerformanceStep, kind string) (PerformanceStep, bool) {
	for _, step := range steps {
		if step.Kind == kind {
			return step, true
		}
	}
	return PerformanceStep{}, false
}

func performanceCurrentBinding(e identity.Envelope, contextID string, evidence PerformanceReleaseEvidence, revisions PerformanceRevisionEvidence) PerformanceBinding {
	return PerformanceBinding{
		TenantDigest: performanceValueDigest(struct {
			Tenant string `json:"tenant"`
		}{e.Tenant()}),
		ContextDigest:     performanceValueDigest(struct{ Tenant, Context string }{e.Tenant(), contextID}),
		ActionsDigest:     performanceValueDigest(canonicalStrings(e.Scopes())),
		SourceRevision:    revisions.SourceRevision,
		RuleRevision:      revisions.RuleRevision,
		TopicRevision:     revisions.TopicRevision,
		RuntimePackDigest: evidence.RuntimePack.Digest,
	}
}

func performanceValueDigest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func canonicalStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func sameStrings(a, b []string) bool {
	x, y := canonicalStrings(a), canonicalStrings(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

type authorityBoundPerformanceRunner struct {
	envelope       identity.Envelope
	actionEnvelope identity.Envelope
	manifest       PerformanceManifest
	targetKind     string
	scenarios      map[string]PerformanceScenarioEvidence
	next           PerformanceReleaseAdapter
}

func (r *authorityBoundPerformanceRunner) Check(ctx context.Context, step PerformanceStep) (PerformanceAdapterResult, error) {
	if step.Kind == "actions_negative" {
		return r.probeDeniedAction(ctx, step)
	}
	denied, err := r.authorize(step)
	if err != nil || denied {
		return blockedPerformanceResult(step), err
	}
	return r.next.Check(ctx, step)
}

func (r *authorityBoundPerformanceRunner) Reset(ctx context.Context) error {
	if ctx == nil || !r.envelope.Valid() {
		return ErrPerformanceAuthority
	}
	return r.next.Reset(ctx)
}

func (r *authorityBoundPerformanceRunner) PreparePerformanceStep(ctx context.Context, step PerformanceStep) error {
	if r == nil || ctx == nil || !r.envelope.Valid() || r.next == nil {
		return ErrPerformanceAuthority
	}
	if !step.Allowed {
		return nil
	}
	denied, err := r.authorize(step)
	if err != nil || denied {
		return ErrPerformanceAuthority
	}
	if preparer, ok := r.next.(performanceStepPreparer); ok {
		return preparer.PreparePerformanceStep(ctx, step)
	}
	return nil
}

func (r *authorityBoundPerformanceRunner) Run(ctx context.Context, step PerformanceStep, iteration int) (PerformanceAdapterResult, error) {
	if step.Kind == "actions_negative" {
		return r.probeDeniedAction(ctx, step)
	}
	denied, err := r.authorize(step)
	if err != nil || denied {
		return blockedPerformanceResult(step), err
	}
	return r.next.Run(ctx, step, iteration)
}

type performanceDeniedActionProbe interface {
	ProbeDeniedAction(context.Context, PerformanceStep, identity.Envelope) (PerformanceAdapterResult, error)
}

func (r *authorityBoundPerformanceRunner) probeDeniedAction(ctx context.Context, step PerformanceStep) (PerformanceAdapterResult, error) {
	if r == nil || !r.envelope.Valid() || !r.actionEnvelope.Valid() || r.next == nil || step.Allowed || step.AuthorityOverride == nil || r.actionEnvelope.Has(step.DeniedAction) || !sameStrings(r.actionEnvelope.Scopes(), step.AuthorityOverride.Scopes) {
		return PerformanceAdapterResult{}, ErrPerformanceAuthority
	}
	probe, ok := r.next.(performanceDeniedActionProbe)
	if !ok {
		return PerformanceAdapterResult{}, ErrPerformanceAuthority
	}
	return probe.ProbeDeniedAction(ctx, step, r.actionEnvelope)
}

func blockedPerformanceResult(step PerformanceStep) PerformanceAdapterResult {
	return PerformanceAdapterResult{SemanticDigest: step.ExpectedDigest, BindingDigest: step.Binding.digest(), Denied: true}
}

func (r *authorityBoundPerformanceRunner) authorize(step PerformanceStep) (bool, error) {
	if r == nil || !r.envelope.Valid() || r.next == nil {
		return false, ErrPerformanceAuthority
	}
	fixture := r.manifest.Authority
	if step.Allowed {
		scenario, ok := r.scenarios[step.ID]
		if !ok || scenario.Revisions.TargetTenant != r.envelope.Tenant() || performanceTargetKind(scenario.Revisions) != r.workloadKind() {
			return false, ErrPerformanceAuthority
		}
		fixture.TargetTenant = scenario.Revisions.TargetTenant
		fixture.ReportID = scenario.Revisions.WorkloadReport
		fixture.SourceID = scenario.Revisions.SourceID
		fixture.ContextID = scenario.Revisions.ContextID
	} else if step.AuthorityOverride != nil {
		fixture = *step.AuthorityOverride
	}
	request := access.Execution{
		Target:       access.Resource{Tenant: fixture.TargetTenant, Kind: r.workloadKind(), Permission: "execute", ID: fixture.ReportID},
		Dependencies: []access.Resource{{Tenant: fixture.TargetTenant, Kind: "source", Permission: "query", ID: fixture.SourceID}},
		Contexts:     []access.Resource{{Tenant: fixture.TargetTenant, Kind: "execution_context", Permission: "use", ID: fixture.ContextID}},
	}
	err := access.RequireExecution(r.envelope, request)
	denied := err != nil
	if denied != !step.Allowed {
		return false, ErrPerformanceAuthority
	}
	return denied, nil
}
