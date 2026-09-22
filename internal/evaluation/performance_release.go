package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
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
	TargetTenant   string
	WorkloadReport string
	SourceID       string
	ContextID      string
	SourceRevision string
	RuleRevision   string
	TopicRevision  string
	DatasetDigest  string
	DatasetRows    int64
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

type performanceTokenVerifier interface {
	Verify(context.Context, string, auth.Surface) (identity.Envelope, error)
}

// PerformanceReleaseRuntime owns the fail-closed Phase 25 release path. The
// bearer is verified independently of the profile, and no manifest value can
// construct or replace its envelope.
type PerformanceReleaseRuntime struct {
	Verifier  performanceTokenVerifier
	Service   *Service
	Revisions PerformanceRevisionResolver
	Adapters  PerformanceReleaseAdapterFactory
	Clock     Clock
}

// Measure resolves the immutable current evidence and executes a final profile
// only with a verifier-produced envelope and a source/model adapter for the
// declared evidence mode.
func (r *PerformanceReleaseRuntime) Measure(ctx context.Context, bearer string, manifest PerformanceManifest) (PerformanceReport, error) {
	if r == nil || ctx == nil || bearer == "" || r.Verifier == nil || r.Service == nil || r.Revisions == nil || r.Adapters == nil || manifest.Kind != PerformanceFinalStress || manifest.EvidenceMode == PerformanceSynthetic {
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
	scenarios, err := r.resolvePerformanceScenarios(ctx, envelope, manifest, evidence, revisions)
	if err != nil {
		return PerformanceReport{}, err
	}
	if err = validatePerformanceScenarioEvidence(envelope, manifest, evidence, scenarios); err != nil {
		return PerformanceReport{}, err
	}
	adapter, err := r.Adapters.NewPerformanceReleaseAdapter(ctx, envelope, manifest, scenarios)
	if err != nil || adapter == nil || adapter.EvidenceMode() != manifest.EvidenceMode || adapter.SourceMode() != manifest.Environment.SourceMode || adapter.ModelMode() != manifest.Environment.ModelMode {
		return PerformanceReport{}, ErrMode
	}
	bound := &authorityBoundPerformanceRunner{envelope: envelope, manifest: manifest, next: adapter}
	report, err := MeasurePerformance(ctx, manifest, bound, r.Clock)
	if err != nil {
		return report, err
	}
	if report.Validate(manifest) != nil {
		return report, ErrPerformanceEvidence
	}
	return report, nil
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
	if !e.Valid() || evidence.Suite.State != Accepted || evidence.Report.Status != "passed" || revisions.TargetTenant != e.Tenant() || revisions.WorkloadReport == "" || revisions.SourceID != m.Authority.SourceID || revisions.ContextID != m.Authority.ContextID || revisions.WorkloadReport != m.Authority.ReportID || revisions.DatasetDigest != m.Environment.DatasetDigest || revisions.DatasetRows != m.Environment.DatasetRows || !validDigest(revisions.SourceRevision) || !validDigest(revisions.RuleRevision) || !validDigest(revisions.TopicRevision) || !validDigest(evidence.CaseResult.Observation.SemanticDigest) {
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
	for _, step := range m.Steps {
		if !step.Allowed {
			continue
		}
		scenario, ok := scenarios[step.ID]
		if !ok {
			return ErrPerformanceEvidence
		}
		ev, rev := scenario.Evidence, scenario.Revisions
		if ev.Suite.State != Accepted || ev.Suite.Digest != base.Suite.Digest || ev.Suite.Suite.ID != base.Suite.Suite.ID || ev.Suite.Suite.Revision != base.Suite.Suite.Revision || ev.Case.ID != step.Workload || ev.Case.ID != ev.CaseResult.ID || ev.Case.ID == "" || ev.Case.Stage != StageConsumer || ev.Case.Critical || !ev.CaseResult.Passed || !performanceExpected(ev.Case.Expected, ev.CaseResult.Observation) || ev.CaseResult.Observation.SemanticDigest != step.ExpectedDigest {
			return ErrPerformanceEvidence
		}
		if rev.TargetTenant != e.Tenant() || !identifier(rev.WorkloadReport) || !identifier(rev.SourceID) || !identifier(rev.ContextID) || rev.DatasetDigest != m.Environment.DatasetDigest || rev.DatasetRows != m.Environment.DatasetRows || !validDigest(rev.SourceRevision) || !validDigest(rev.RuleRevision) || !validDigest(rev.TopicRevision) {
			return ErrPerformanceEvidence
		}
		request := access.Execution{
			Target:       access.Resource{Tenant: rev.TargetTenant, Kind: "report", Permission: "execute", ID: rev.WorkloadReport},
			Dependencies: []access.Resource{{Tenant: rev.TargetTenant, Kind: "source", Permission: "query", ID: rev.SourceID}},
			Contexts:     []access.Resource{{Tenant: rev.TargetTenant, Kind: "execution_context", Permission: "use", ID: rev.ContextID}},
		}
		if err := access.RequireExecution(e, request); err != nil {
			return ErrPerformanceAuthority
		}
		actual := performanceCurrentBinding(e, rev.ContextID, ev, rev)
		if actual != step.Binding {
			return ErrPerformanceEvidence
		}
	}
	return nil
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
	envelope identity.Envelope
	manifest PerformanceManifest
	next     PerformanceReleaseAdapter
}

func (r *authorityBoundPerformanceRunner) Check(ctx context.Context, step PerformanceStep) (PerformanceAdapterResult, error) {
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

func (r *authorityBoundPerformanceRunner) Run(ctx context.Context, step PerformanceStep, iteration int) (PerformanceAdapterResult, error) {
	denied, err := r.authorize(step)
	if err != nil || denied {
		return blockedPerformanceResult(step), err
	}
	return r.next.Run(ctx, step, iteration)
}

func blockedPerformanceResult(step PerformanceStep) PerformanceAdapterResult {
	return PerformanceAdapterResult{SemanticDigest: step.ExpectedDigest, BindingDigest: step.Binding.digest(), Denied: true}
}

func (r *authorityBoundPerformanceRunner) authorize(step PerformanceStep) (bool, error) {
	if r == nil || !r.envelope.Valid() || r.next == nil {
		return false, ErrPerformanceAuthority
	}
	if step.Kind == "actions_negative" {
		if step.Allowed || step.DeniedAction == "" || r.envelope.Has(step.DeniedAction) {
			return false, ErrPerformanceAuthority
		}
		err := access.Require(r.envelope, step.DeniedAction, access.Tenant(r.envelope, "write"))
		if errors.Is(err, access.ErrForbidden) {
			return true, nil
		}
		return false, ErrPerformanceAuthority
	}
	fixture := r.manifest.Authority
	if step.AuthorityOverride != nil {
		fixture = *step.AuthorityOverride
	}
	request := access.Execution{
		Target:       access.Resource{Tenant: fixture.TargetTenant, Kind: "report", Permission: "execute", ID: fixture.ReportID},
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
