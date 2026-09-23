package evaluation

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
)

func TestPerformanceReleaseRuntimeUsesVerifiedAuthorityAndExactPhase24Evidence(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	scopes := []string{
		"ops.write", "ops.read", "cw.tenant.write:tenant", "cw.tenant.read:tenant",
		"reporting.execute", "cw.report.execute:workload-report", "cw.source.query:source-a",
		"cw.source.query:source-b", "cw.execution_context.use:context-a", "cw.execution_context.use:context-b", "query.plan", "query.execute",
	}
	envelope, err := identity.FromVerified("tenant", "actor", "session", scopes, now.Add(time.Hour), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	actionScopes := withoutPerformanceScope(scopes, "query.execute")
	actionEnvelope, err := identity.FromVerified("tenant", "actor", "session", actionScopes, now.Add(time.Hour), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	repo, manifest, revisions := performanceReleaseFixture(t, envelope, now)
	service, err := New(repo, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	verifier := &releaseTestVerifier{envelope: envelope, wantBearer: "verified-current-bearer", actionEnvelope: actionEnvelope, actionBearer: "verified-action-bearer"}
	adapter := &releaseTestAdapter{mode: PerformanceIntegration, cache: map[string]bool{}}
	factory := &releaseTestAdapterFactory{adapter: adapter}
	revisionCalls := map[string]int{}
	runtime := &PerformanceReleaseRuntime{Verifier: verifier, Service: service, Revisions: releaseTestRevisionResolver{evidence: revisions, byCase: performanceReleaseRevisionVariants(revisions), calls: revisionCalls}, Adapters: factory}

	report, err := runtime.Measure(context.Background(), "verified-current-bearer", "verified-action-bearer", manifest)
	if err != nil || !report.CorrectnessPassed || report.Validate(manifest) != nil {
		t.Fatal("release run failed", err, report.EvidenceHash)
	}
	if report.CurrentBinding != coldPerformanceBinding(manifest) {
		t.Fatal("sealed report did not retain the current revision binding")
	}
	if verifier.lastSurface != auth.HTTP || verifier.calls != 2 || factory.calls != 1 || adapter.mode != manifest.EvidenceMode {
		t.Fatal("runtime did not use the configured verifier and matching adapter", verifier, factory.calls, adapter.mode)
	}
	for _, caseID := range []string{"case-base", "case-source", "case-rule", "case-context", "case-topic", "case-runtime"} {
		if revisionCalls[caseID] == 0 {
			t.Fatalf("release scenario %s did not resolve its own current revisions", caseID)
		}
	}
	wrongRevisionRuntime := *runtime
	wrongRevisionRuntime.Revisions = releaseTestRevisionResolver{evidence: revisions}
	if _, err = wrongRevisionRuntime.Measure(context.Background(), "verified-current-bearer", "verified-action-bearer", manifest); !errors.Is(err, ErrPerformanceEvidence) || factory.calls != 1 {
		t.Fatal("changed-binding scenario passed with the cold scenario's revisions", err, factory.calls)
	}
	if adapter.measuredPhysical != 7 {
		t.Fatalf("source/model executions=%d, want cold + five invalidations + concurrent cold", adapter.measuredPhysical)
	}
	missingScenarioReport := manifest
	missingScenarioReport.Steps = append([]PerformanceStep(nil), manifest.Steps...)
	for i := range missingScenarioReport.Steps {
		if missingScenarioReport.Steps[i].Kind == "source_changed" {
			missingScenarioReport.Steps[i].EvidenceReportID = ""
			missingScenarioReport.Steps[i].EvidenceReport = ""
		}
	}
	if missingScenarioReport.Validate() == nil {
		t.Fatal("final stress accepted an invalidation without its exact accepted report reference")
	}

	probeOnly := &releaseTestAdapter{mode: PerformanceIntegration, cache: map[string]bool{}, skipProbeReceipt: true}
	factory.adapter = probeOnly
	probeReport, probeErr := runtime.Measure(context.Background(), "verified-current-bearer", "verified-action-bearer", manifest)
	if !errors.Is(probeErr, ErrGate) || probeReport.CorrectnessPassed || len(probeReport.Samples) != 0 || probeOnly.runCalls != 0 {
		t.Fatal("timing began without a physical correctness probe", probeErr, len(probeReport.Samples), probeOnly.runCalls)
	}

	wrongBoundary := &releaseTestAdapter{mode: PerformanceIntegration, sourceMode: "synthetic", cache: map[string]bool{}}
	factory.adapter = wrongBoundary
	if _, err = runtime.Measure(context.Background(), "verified-current-bearer", "verified-action-bearer", manifest); !errors.Is(err, ErrMode) || wrongBoundary.runCalls != 0 {
		t.Fatal("adapter with non-PostgreSQL boundary was accepted", err, wrongBoundary.runCalls)
	}

	// Profile claims cannot add reach absent from the verifier-produced envelope.
	tampered := manifest
	tampered.Authority.Scopes = append(append([]string(nil), manifest.Authority.Scopes...), "report.publish")
	if _, err = runtime.Measure(context.Background(), "verified-current-bearer", "verified-action-bearer", tampered); !errors.Is(err, ErrPerformanceAuthority) || factory.calls != 3 {
		t.Fatal("manifest widened verified authority or reached the adapter", err, factory.calls)
	}
	if _, err = runtime.Measure(context.Background(), "manifest-bearer", "verified-action-bearer", manifest); !errors.Is(err, ErrPerformanceAuthority) || factory.calls != 3 {
		t.Fatal("manifest-selected bearer was accepted", err, factory.calls)
	}
	if _, err = runtime.Measure(context.Background(), "verified-current-bearer", "verified-current-bearer", manifest); !errors.Is(err, ErrPerformanceAuthority) || factory.calls != 3 {
		t.Fatal("full-action bearer was accepted for the denial", err, factory.calls)
	}
}

func TestPerformanceReleaseRunnerBlocksBeforeAdapterWork(t *testing.T) {
	now := time.Now()
	e, err := identity.FromVerified("tenant", "actor", "session", []string{
		"ops.write", "cw.tenant.write:tenant", "reporting.execute", "cw.report.execute:workload-report",
		"cw.source.query:source-a", "cw.execution_context.use:context-a", "query.plan", "query.execute",
	}, now.Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	_, m, _ := performanceReleaseFixture(t, e, now)
	_, err = performanceTestEvidence(t, e, m)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &releaseTestAdapter{mode: PerformanceIntegration, cache: map[string]bool{}}
	actionEnvelope, err := identity.FromVerified(e.Tenant(), e.User(), e.Session(), withoutPerformanceScope(e.Scopes(), "query.execute"), now.Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	bound := &authorityBoundPerformanceRunner{envelope: e, actionEnvelope: actionEnvelope, manifest: m, next: adapter}
	for _, index := range []int{9, 10, 11} {
		result, runErr := bound.Run(context.Background(), m.Steps[index], 0)
		if runErr != nil || !result.Denied || result.Receipt.Usage.SourceCalls != 0 || result.Receipt.Usage.ModelCalls != 0 {
			t.Fatalf("denial %s reached physical work: %+v %v", m.Steps[index].Kind, result, runErr)
		}
	}
	if adapter.measuredPhysical != 0 || adapter.checkCalls != 0 {
		t.Fatal("denial delegated to the source/model adapter", adapter)
	}
}

func TestPerformanceReportPersistenceIsCreateOnly(t *testing.T) {
	m := performanceTestManifest()
	report, err := MeasurePerformance(context.Background(), m, newTestSyntheticPerformanceRunner(), nil)
	if err != nil {
		t.Fatal(err)
	}
	mutated := report
	mutated.CurrentBinding.SourceRevision = strings.Repeat("f", 64)
	mutated = sealPerformanceReport(mutated)
	if mutated.Validate(m) == nil {
		t.Fatal("report accepted revision evidence different from its immutable profile")
	}
	path := t.TempDir() + "/report.json"
	if err := PersistPerformanceReport(path, m, report); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("performance report permissions are not private", err, info.Mode().Perm())
	}
	if err = PersistPerformanceReport(path, m, report); err == nil {
		t.Fatal("existing performance evidence was replaced")
	}
	second, readErr := os.ReadFile(path)
	if readErr != nil || string(first) != string(second) {
		t.Fatal("original performance report changed", readErr)
	}
}

type releaseTestVerifier struct {
	envelope       identity.Envelope
	actionEnvelope identity.Envelope
	wantBearer     string
	actionBearer   string
	calls          int
	lastSurface    auth.Surface
}

func (v *releaseTestVerifier) Verify(_ context.Context, bearer string, surface auth.Surface) (identity.Envelope, error) {
	v.calls++
	v.lastSurface = surface
	if bearer != v.wantBearer {
		if bearer == v.actionBearer {
			return v.actionEnvelope, nil
		}
		return identity.Envelope{}, ErrPerformanceAuthority
	}
	return v.envelope, nil
}

func withoutPerformanceScope(scopes []string, removed string) []string {
	out := make([]string, 0, len(scopes)-1)
	for _, scope := range scopes {
		if scope != removed {
			out = append(out, scope)
		}
	}
	return out
}

type releaseTestRevisionResolver struct {
	evidence PerformanceRevisionEvidence
	byCase   map[string]PerformanceRevisionEvidence
	calls    map[string]int
}

func (r releaseTestRevisionResolver) ResolvePerformanceRevisions(_ context.Context, _ identity.Envelope, evidence PerformanceReleaseEvidence) (PerformanceRevisionEvidence, error) {
	if r.calls != nil {
		r.calls[evidence.Case.ID]++
	}
	if revisions, ok := r.byCase[evidence.Case.ID]; ok {
		return revisions, nil
	}
	return r.evidence, nil
}

type releaseTestAdapterFactory struct {
	adapter PerformanceReleaseAdapter
	calls   int
}

func (f *releaseTestAdapterFactory) NewPerformanceReleaseAdapter(_ context.Context, _ identity.Envelope, _ PerformanceManifest, _ map[string]PerformanceScenarioEvidence) (PerformanceReleaseAdapter, error) {
	f.calls++
	return f.adapter, nil
}

type releaseTestAdapter struct {
	mode             PerformanceEvidenceMode
	sourceMode       string
	modelMode        string
	skipProbeReceipt bool
	mu               sync.Mutex
	cache            map[string]bool
	checkCalls       int
	runCalls         int
	measuredPhysical int
}

func (a *releaseTestAdapter) EvidenceMode() PerformanceEvidenceMode { return a.mode }
func (a *releaseTestAdapter) SourceMode() string {
	if a.sourceMode != "" {
		return a.sourceMode
	}
	return "real_postgres"
}
func (a *releaseTestAdapter) ModelMode() string {
	if a.modelMode != "" {
		return a.modelMode
	}
	if a.mode == PerformanceLive {
		return "live"
	}
	return "recorded"
}
func (a *releaseTestAdapter) Check(_ context.Context, step PerformanceStep) (PerformanceAdapterResult, error) {
	a.mu.Lock()
	a.checkCalls++
	a.mu.Unlock()
	if a.skipProbeReceipt {
		return PerformanceAdapterResult{SemanticDigest: step.ExpectedDigest, BindingDigest: step.Binding.digest()}, nil
	}
	return releaseTestPhysicalResult(step), nil
}
func (a *releaseTestAdapter) ProbeDeniedAction(_ context.Context, step PerformanceStep, e identity.Envelope) (PerformanceAdapterResult, error) {
	if !e.Valid() || e.Has(step.DeniedAction) {
		return PerformanceAdapterResult{}, ErrPerformanceAuthority
	}
	return blockedPerformanceResult(step), nil
}
func (a *releaseTestAdapter) Reset(context.Context) error {
	a.mu.Lock()
	a.cache = map[string]bool{}
	a.mu.Unlock()
	return nil
}
func (a *releaseTestAdapter) Run(_ context.Context, step PerformanceStep, _ int) (PerformanceAdapterResult, error) {
	a.mu.Lock()
	a.runCalls++
	key := step.Binding.digest()
	physical := !a.cache[key]
	if physical {
		a.cache[key] = true
		a.measuredPhysical++
	}
	a.mu.Unlock()
	if !physical {
		return PerformanceAdapterResult{
			SemanticDigest: step.ExpectedDigest,
			BindingDigest:  step.Binding.digest(),
			Receipt:        PerformanceReceipt{Usage: PerformanceUsage{ServiceNS: 1}},
		}, nil
	}
	return releaseTestPhysicalResult(step), nil
}

func releaseTestPhysicalResult(step PerformanceStep) PerformanceAdapterResult {
	sourceNS, modelNS := int64(1), int64(1)
	return PerformanceAdapterResult{
		SemanticDigest: step.ExpectedDigest,
		BindingDigest:  step.Binding.digest(),
		Receipt: PerformanceReceipt{Usage: PerformanceUsage{
			ServiceNS: 1, SourceNS: &sourceNS, ModelNS: &modelNS, SourceCalls: 1, ModelCalls: 1,
		}},
	}
}

func performanceReleaseFixture(t *testing.T, e identity.Envelope, now time.Time) (*testRepo, PerformanceManifest, PerformanceRevisionEvidence) {
	t.Helper()
	repo, manifest, revisions := performanceReleaseFixtureWithoutService(t, e, now)
	return repo, manifest, revisions
}

func performanceReleaseFixtureWithoutService(t *testing.T, e identity.Envelope, now time.Time) (*testRepo, PerformanceManifest, PerformanceRevisionEvidence) {
	t.Helper()
	suite := testSuite()
	suite.ID = "phase24-performance"
	suite.Mode = Live
	caseIDs := []string{"case-base", "case-source", "case-rule", "case-context", "case-topic", "case-runtime"}
	suite.Cases = make([]Case, 0, len(caseIDs))
	for i, caseID := range caseIDs {
		suite.Cases = append(suite.Cases, Case{
			ID: caseID, Stage: StageConsumer, Locale: "en",
			Input:    ProtectedRef{Digest: strings.Repeat(string(rune('a'+i)), 64), Retention: "protected"},
			Expected: []Expected{{Decision: "completed", SemanticDigest: testDigest}},
		})
	}
	suite.Provenance.DialectMatrix[0].Mode = Live
	suite.HeldoutLineageDigest = strings.Repeat("d", 64)
	suiteDigest, err := suite.Digest()
	if err != nil {
		t.Fatal(err)
	}
	suiteReview := SuiteReview{SuiteID: suite.ID, Revision: suite.Revision, Digest: suiteDigest, Decision: Accepted, Reviewer: "reviewer", ReviewedAt: now}
	suiteRecord := SuiteRecord{Suite: suite, Digest: suiteDigest, State: Accepted, Author: "author", Review: &suiteReview, CreatedAt: now}
	makeReport := func(runID string, pack PackRevision) Report {
		report, err := EvaluateWithPack(context.Background(), runID, suite, pack, RunnerFunc(func(_ context.Context, x Execution) (Observation, error) {
			want := x.Case.Expected[0]
			return Observation{Decision: want.Decision, SemanticDigest: want.SemanticDigest, ErrorClass: want.ErrorClass, Usage: Usage{ServiceMS: 1}}, nil
		}), func() time.Time { return now })
		if err != nil || report.Status != "passed" || !report.GatePassed {
			t.Fatal("phase 24 fixture report failed", err, report.Status)
		}
		return report
	}
	baseRunID, alternateRunID := "phase24-performance-report", "phase24-performance-alt-report"
	phase24Report := makeReport(baseRunID, suite.Packs[0])
	alternateReport := makeReport(alternateRunID, suite.Packs[1])
	makeRuntimePack := func(pack PackRevision) RuntimePackRecord {
		cfg := testRuntimeConfig(pack.Model)
		runtimeDigest := runtimePackDigest(pack, cfg)
		runtimeReview := RuntimePackReview{PackID: pack.ID, PackRevision: pack.Revision, PackDigest: pack.Digest, RuntimeDigest: runtimeDigest, ConfigurationDigest: cfg.Digest, Model: cfg.Model, SystemInstruction: cfg.SystemInstruction, MaxAttemptCostUSD: cfg.AttemptCostUSD, Decision: Accepted, Reviewer: "reviewer", ReviewedAt: now}
		return RuntimePackRecord{Pack: pack, Config: cfg, Digest: runtimeDigest, State: Accepted, Author: "author", CreatedAt: now, Review: &runtimeReview}
	}
	baseRuntimePack, alternateRuntimePack := makeRuntimePack(suite.Packs[0]), makeRuntimePack(suite.Packs[1])
	repo := &testRepo{
		reports:      map[string]Report{baseRunID: phase24Report, alternateRunID: alternateReport},
		suites:       map[string]SuiteRecord{suiteDigest: suiteRecord},
		exports:      map[string]CandidateExport{},
		proposals:    map[string]OptimizationProposal{},
		runtimePacks: map[string]RuntimePackRecord{suite.Packs[0].Digest: baseRuntimePack, suite.Packs[1].Digest: alternateRuntimePack},
	}
	authority := PerformanceAuthorityFixture{Tenant: e.Tenant(), User: e.User(), Session: e.Session(), Scopes: e.Scopes(), TargetTenant: e.Tenant(), ReportID: "workload-report", SourceID: "source-a", ContextID: "context-a"}
	datasetDigest := strings.Repeat("a", 64)
	revisions := PerformanceRevisionEvidence{TargetTenant: e.Tenant(), WorkloadReport: authority.ReportID, SourceID: authority.SourceID, ContextID: authority.ContextID, SourceRevision: strings.Repeat("b", 64), RuleRevision: strings.Repeat("c", 64), TopicRevision: strings.Repeat("e", 64), DatasetDigest: datasetDigest, DatasetRows: 128}
	manifest := PerformanceManifest{
		SchemaVersion: 1, ID: "phase25-final-stress-test", Kind: PerformanceFinalStress, EvidenceMode: PerformanceIntegration,
		Environment: PerformanceEnvironment{
			RunnerLabel: "release-test", OS: runtime.GOOS, Architecture: runtime.GOARCH, CPUs: runtime.NumCPU(), GoVersion: runtime.Version(),
			DatasetDigest: datasetDigest, DatasetRows: revisions.DatasetRows, SourceMode: "real_postgres", ModelMode: "recorded",
			EvaluationSuiteID: suite.ID, EvaluationSuiteRevision: suite.Revision, EvaluationSuite: suiteDigest,
			EvaluationReportID: baseRunID, EvaluationReport: phase24Report.EvidenceHash, WorkloadCaseID: suite.Cases[0].ID,
		},
		Authority: authority, MaxDurationMS: int64(time.Hour / time.Millisecond),
	}
	current := performanceCurrentBinding(e, authority.ContextID, PerformanceReleaseEvidence{RuntimePack: baseRuntimePack}, revisions)
	kinds := []struct {
		kind        string
		iterations  int
		concurrency int
	}{
		{"cold", 20, 1}, {"warm", 1000, 16}, {"repeat", 10000, 64}, {"concurrent", 2000, 128},
		{"source_changed", 500, 32}, {"rule_changed", 500, 32}, {"context_changed", 500, 32}, {"topic_changed", 500, 32}, {"runtime_pack_changed", 500, 32},
		{"tenant_negative", 1000, 64}, {"context_negative", 1000, 64}, {"actions_negative", 1000, 64},
	}
	for _, shape := range kinds {
		binding := current
		allowed, executions, blocks := true, 0, 0
		reset := shape.kind == "cold" || shape.kind == "concurrent"
		if shape.kind == "cold" || shape.kind == "concurrent" || strings.HasSuffix(shape.kind, "_changed") {
			executions = 1
		}
		switch shape.kind {
		case "source_changed":
			binding.SourceRevision = strings.Repeat("f", 64)
		case "rule_changed":
			binding.RuleRevision = strings.Repeat("1", 64)
		case "context_changed", "context_negative":
			binding.ContextDigest = strings.Repeat("2", 64)
		case "topic_changed":
			binding.TopicRevision = strings.Repeat("3", 64)
		case "runtime_pack_changed":
			binding.RuntimePackDigest = strings.Repeat("4", 64)
		case "tenant_negative":
			binding.TenantDigest = strings.Repeat("5", 64)
		case "actions_negative":
			binding.ActionsDigest = performanceValueDigest(canonicalStrings(withoutPerformanceScope(e.Scopes(), "query.execute")))
		}
		scenarioRevisions := revisions
		caseID := manifest.Environment.WorkloadCaseID
		var evidenceCase, evidenceReport, evidenceReportHash string
		switch shape.kind {
		case "source_changed":
			caseID = "case-source"
			scenarioRevisions.SourceID = "source-b"
			scenarioRevisions.SourceRevision = strings.Repeat("f", 64)
		case "rule_changed":
			caseID = "case-rule"
			scenarioRevisions.RuleRevision = strings.Repeat("1", 64)
		case "context_changed":
			caseID = "case-context"
			scenarioRevisions.ContextID = "context-b"
		case "topic_changed":
			caseID = "case-topic"
			scenarioRevisions.TopicRevision = strings.Repeat("3", 64)
		case "runtime_pack_changed":
			caseID = "case-runtime"
			evidenceReport = alternateRunID
			evidenceReportHash = alternateReport.EvidenceHash
		}
		if strings.HasSuffix(shape.kind, "_changed") {
			evidenceCase = caseID
			if evidenceReport == "" {
				evidenceReport, evidenceReportHash = baseRunID, phase24Report.EvidenceHash
			}
		}
		if shape.kind == "source_changed" {
			binding = PerformanceBinding{TenantDigest: current.TenantDigest, ContextDigest: current.ContextDigest, ActionsDigest: current.ActionsDigest, SourceRevision: scenarioRevisions.SourceRevision, RuleRevision: current.RuleRevision, TopicRevision: current.TopicRevision, RuntimePackDigest: current.RuntimePackDigest}
		} else if shape.kind == "rule_changed" {
			binding = PerformanceBinding{TenantDigest: current.TenantDigest, ContextDigest: current.ContextDigest, ActionsDigest: current.ActionsDigest, SourceRevision: current.SourceRevision, RuleRevision: scenarioRevisions.RuleRevision, TopicRevision: current.TopicRevision, RuntimePackDigest: current.RuntimePackDigest}
		} else if shape.kind == "context_changed" {
			binding = PerformanceBinding{TenantDigest: current.TenantDigest, ContextDigest: performanceValueDigest(struct{ Tenant, Context string }{e.Tenant(), scenarioRevisions.ContextID}), ActionsDigest: current.ActionsDigest, SourceRevision: current.SourceRevision, RuleRevision: current.RuleRevision, TopicRevision: current.TopicRevision, RuntimePackDigest: current.RuntimePackDigest}
		} else if shape.kind == "topic_changed" {
			binding = PerformanceBinding{TenantDigest: current.TenantDigest, ContextDigest: current.ContextDigest, ActionsDigest: current.ActionsDigest, SourceRevision: current.SourceRevision, RuleRevision: current.RuleRevision, TopicRevision: scenarioRevisions.TopicRevision, RuntimePackDigest: current.RuntimePackDigest}
		} else if shape.kind == "runtime_pack_changed" {
			binding.RuntimePackDigest = alternateRuntimePack.Digest
		}
		if strings.HasSuffix(shape.kind, "_negative") {
			allowed, executions, blocks = false, 0, shape.iterations
		}
		step := PerformanceStep{ID: "step-" + shape.kind, Kind: shape.kind, Binding: binding, EvidenceCaseID: evidenceCase, EvidenceReportID: evidenceReport, EvidenceReport: evidenceReportHash, Allowed: allowed, ResetBefore: reset, Iterations: shape.iterations, Concurrency: shape.concurrency, Workload: caseID, ExpectedDigest: suite.Cases[0].Expected[0].SemanticDigest, ExpectedExecutions: executions, ExpectedBlocks: blocks}
		if shape.kind == "tenant_negative" {
			override := authority
			override.TargetTenant = "tenant-b"
			step.AuthorityOverride = &override
		} else if shape.kind == "context_negative" {
			override := authority
			override.ContextID = "context-c"
			step.AuthorityOverride = &override
		} else if shape.kind == "actions_negative" {
			step.DeniedAction = "query.execute"
			override := authority
			override.Scopes = withoutPerformanceScope(e.Scopes(), "query.execute")
			step.AuthorityOverride = &override
		}
		manifest.Steps = append(manifest.Steps, step)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal("final performance fixture invalid", err)
	}
	return repo, manifest, revisions
}

func performanceReleaseRevisionVariants(base PerformanceRevisionEvidence) map[string]PerformanceRevisionEvidence {
	source := base
	source.SourceID, source.SourceRevision = "source-b", strings.Repeat("f", 64)
	rule := base
	rule.RuleRevision = strings.Repeat("1", 64)
	contextRevision := base
	contextRevision.ContextID = "context-b"
	topic := base
	topic.TopicRevision = strings.Repeat("3", 64)
	return map[string]PerformanceRevisionEvidence{
		"case-base": base, "case-source": source, "case-rule": rule,
		"case-context": contextRevision, "case-topic": topic, "case-runtime": base,
	}
}

func performanceTestEvidence(t *testing.T, e identity.Envelope, m PerformanceManifest) (PerformanceReleaseEvidence, error) {
	t.Helper()
	repo, _, _ := performanceReleaseFixtureWithoutService(t, e, time.Now().UTC())
	svc, err := New(repo, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return svc.ResolvePerformanceEvidence(context.Background(), e, m.Environment)
}
