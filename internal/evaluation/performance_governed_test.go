package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
)

func TestGovernedPerformanceAdapterMeasuresCompositePlanAndRun(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	envelope, err := identity.FromVerified("tenant", "actor", "session", []string{"query.plan", "query.execute"}, now.Add(time.Hour), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	suite := testSuite()
	pack := suite.Packs[0]
	config := testRuntimeConfig(pack.Model)
	query := &performanceCompositeQuery{plans: map[string]string{}, executed: map[string]bool{}, receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: "sqlgen", Provider: "recorded", RequestedModel: pack.Model, ConfigurationDigest: pack.ConfigurationDigest, Attempts: 1, DurationMS: 3, InputTokens: ptr(2), OutputTokens: ptr(4), CostUSD: ptr(0.01)}}}}
	input := LiveInput{Pack: pack, Question: &nlqexec.QuestionRequest{Context: "context", Locale: nlq.LanguageEnglish, Question: "count records"}, Run: &nlqexec.RunRequest{QueryID: "protected-query", Operation: "protected-operation", Rows: 20, Bytes: 4096}}
	runner := GovernedRunner{Inputs: staticInputResolver{in: input}, Query: query}
	caseDef := Case{ID: "workload", Stage: StageConsumer, Input: ProtectedRef{Digest: testDigest, Retention: "protected"}, Expected: []Expected{{Decision: "completed", SemanticDigest: testDigest}}}
	scenario := governedPerformanceTestScenario(envelope, suite, caseDef, pack, config, "context")
	adapter := &governedPerformanceReleaseAdapter{runner: runner, envelope: envelope, scenarios: map[string]governedPerformanceScenario{"cold": scenario, "warm": scenario}, mode: PerformanceIntegration, sourceMode: "real_postgres", modelMode: "recorded", runID: "release-run"}
	binding := performanceCurrentBinding(envelope, "context", scenario.evidence, scenario.revisions)
	cold := PerformanceStep{ID: "cold", Kind: "cold", Binding: binding, Allowed: true, Workload: caseDef.ID, ExpectedDigest: testDigest}
	first, err := adapter.Run(context.Background(), cold, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Receipt.Usage.SourceCalls != 1 || first.Receipt.Usage.ModelCalls != 1 || first.Receipt.Usage.SourceNS != nil || first.Receipt.Usage.ModelNS == nil || *first.Receipt.Usage.ModelNS != int64(3*time.Millisecond) {
		t.Fatalf("composite receipts were not collected from both services: %+v", first.Receipt.Usage)
	}
	measured, err := derivePerformanceObservation(PerformanceEnvironment{SourceMode: "real_postgres", ModelMode: "recorded"}, cold, first, true)
	if err != nil || !errors.Is(validatePerformanceObservation(PerformanceEnvironment{SourceMode: "real_postgres", ModelMode: "recorded"}, cold, measured, true), ErrInvalid) {
		t.Fatal("attempt wall time or unknown source time passed the physical timing gate", measured, err)
	}
	warm := cold
	warm.ID, warm.Kind = "warm", "warm"
	second, err := adapter.Run(context.Background(), warm, 1)
	if err != nil {
		t.Fatal(err)
	}
	if second.Receipt.Usage.SourceCalls != 0 || second.Receipt.Usage.ModelCalls != 0 || second.SemanticDigest != first.SemanticDigest || second.BindingDigest != first.BindingDigest {
		t.Fatalf("durable operation replay did not preserve semantics without claiming work: first=%+v second=%+v", first, second)
	}
	if query.planCalls != 1 || query.runCalls != 2 {
		t.Fatalf("expected one real plan and two service run requests, got plans=%d runs=%d", query.planCalls, query.runCalls)
	}
	if err := adapter.Reset(context.Background()); err != nil {
		t.Fatal(err)
	}
	third, err := adapter.Run(context.Background(), cold, 2)
	if err != nil || third.Receipt.Usage.SourceCalls != 1 || third.Receipt.Usage.ModelCalls != 1 {
		t.Fatal("reset did not create a fresh durable Plan→Run operation", third, err)
	}
	if query.planCalls != 2 || query.runCalls != 3 {
		t.Fatalf("fresh operation should plan and run once, got plans=%d runs=%d", query.planCalls, query.runCalls)
	}
	if _, err := json.Marshal(nlqexec.RunResult{Receipt: query.receipt}); err != nil {
		t.Fatal(err)
	} else {
		var public map[string]any
		_ = json.Unmarshal(mustJSON(t, nlqexec.RunResult{Receipt: query.receipt}), &public)
		if _, ok := public["receipt"]; ok {
			t.Fatal("internal model receipt escaped into public RunResult JSON")
		}
	}
}

func TestGovernedPerformanceAdapterConcurrentColdKeepsReceiptsTogether(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	envelope, err := identity.FromVerified("tenant", "actor", "session", []string{"query.plan", "query.execute"}, now.Add(time.Hour), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	suite := testSuite()
	pack := suite.Packs[0]
	query := &performanceCompositeQuery{plans: map[string]string{}, executed: map[string]bool{}, receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: "sqlgen", Provider: "recorded", RequestedModel: pack.Model, ConfigurationDigest: pack.ConfigurationDigest, Attempts: 1, DurationMS: 3}}}}
	input := LiveInput{Pack: pack, Question: &nlqexec.QuestionRequest{Context: "context", Locale: nlq.LanguageEnglish, Question: "count records"}, Run: &nlqexec.RunRequest{QueryID: "protected-query", Operation: "protected-operation", Rows: 20, Bytes: 4096}}
	caseDef := Case{ID: "workload", Stage: StageConsumer, Input: ProtectedRef{Digest: testDigest, Retention: "protected"}, Expected: []Expected{{Decision: "completed", SemanticDigest: testDigest}}}
	scenario := governedPerformanceTestScenario(envelope, suite, caseDef, pack, testRuntimeConfig(pack.Model), "context")
	adapter := &governedPerformanceReleaseAdapter{runner: GovernedRunner{Inputs: staticInputResolver{in: input}, Query: query}, envelope: envelope, scenarios: map[string]governedPerformanceScenario{"concurrent": scenario}, mode: PerformanceIntegration, sourceMode: "real_postgres", modelMode: "recorded", runID: "concurrent-release-run"}
	binding := performanceCurrentBinding(envelope, "context", scenario.evidence, scenario.revisions)
	step := PerformanceStep{ID: "concurrent", Kind: "concurrent", Binding: binding, Allowed: true, Workload: caseDef.ID, ExpectedDigest: testDigest}
	const callers = 48
	results := make(chan PerformanceAdapterResult, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(iteration int) {
			defer wg.Done()
			result, runErr := adapter.Run(context.Background(), step, iteration)
			if runErr != nil {
				errs <- runErr
				return
			}
			results <- result
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	for runErr := range errs {
		t.Fatal(runErr)
	}
	var sourceCalls, modelCalls, executions int
	for result := range results {
		sourceCalls += result.Receipt.Usage.SourceCalls
		modelCalls += result.Receipt.Usage.ModelCalls
		if result.Receipt.Usage.SourceCalls > 0 || result.Receipt.Usage.ModelCalls > 0 {
			executions++
		}
	}
	if query.planCalls != 1 || query.runCalls != callers || sourceCalls != 1 || modelCalls != 1 || executions != 1 {
		t.Fatalf("composite operation did not keep the single cold execution and both receipts together: plans=%d runs=%d source=%d model=%d execution samples=%d", query.planCalls, query.runCalls, sourceCalls, modelCalls, executions)
	}
}

func TestGovernedPerformanceAdapterRequiresProtectedPlanAndRunInputs(t *testing.T) {
	now := time.Now().UTC()
	envelope, err := identity.FromVerified("tenant", "actor", "session", []string{"query.plan", "query.execute"}, now.Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	suite := testSuite()
	pack := suite.Packs[0]
	factory, err := NewGovernedPerformanceReleaseAdapterFactory(&GovernedRunner{Inputs: staticInputResolver{in: LiveInput{Pack: pack}}, Query: &performanceCompositeQuery{}}, PerformanceIntegration, "real_postgres", "recorded", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = factory.NewPerformanceReleaseAdapter(context.Background(), envelope, PerformanceManifest{}, nil); !errors.Is(err, ErrMode) {
		t.Fatalf("an incomplete workload was accepted: %v", err)
	}
	if _, err = NewGovernedPerformanceReleaseAdapterFactory(&GovernedRunner{Inputs: staticInputResolver{in: LiveInput{Pack: pack}}, Query: &performanceCompositeQuery{}}, PerformanceIntegration, "synthetic", "recorded", nil); !errors.Is(err, ErrMode) {
		t.Fatalf("synthetic source boundary was accepted for integration mode: %v", err)
	}
	query := &performanceCompositeQuery{}
	if _, err = NewGovernedPerformanceReleaseAdapterFactory(&GovernedRunner{Inputs: staticInputResolver{in: LiveInput{Pack: pack}}, Query: query}, PerformanceLive, "real_postgres", "live", nil); !errors.Is(err, ErrMode) {
		t.Fatalf("recorded gateway was relabeled as live: %v", err)
	}
	if _, err = NewGovernedPerformanceReleaseAdapterFactory(&GovernedRunner{Inputs: staticInputResolver{in: LiveInput{Pack: pack}}, Query: unmarkedPerformanceQuery{queryRuntime: query}}, PerformanceIntegration, "real_postgres", "recorded", nil); !errors.Is(err, ErrMode) {
		t.Fatalf("query runtime without model-mode attestation was accepted: %v", err)
	}
	if _, err = NewGovernedPerformanceReleaseAdapterFactory(&GovernedRunner{Inputs: staticInputResolver{in: LiveInput{Pack: pack}}, Query: nonisolatedPerformanceQuery{query: query}}, PerformanceIntegration, "real_postgres", "recorded", nil); !errors.Is(err, ErrMode) {
		t.Fatalf("query runtime without a durable cross-process operation lock was accepted: %v", err)
	}
}

func TestGovernedPerformanceAdapterRejectsUnsupportedInvalidation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	envelope, err := identity.FromVerified("tenant", "actor", "session", []string{
		"ops.write", "cw.tenant.write:tenant", "reporting.execute", "cw.report.execute:workload-report",
		"cw.source.query:source-a", "cw.source.query:source-b", "cw.execution_context.use:context-a", "cw.execution_context.use:context-b",
		"query.plan", "query.execute",
	}, now.Add(time.Hour), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	repo, manifest, revisions := performanceReleaseFixture(t, envelope, now)
	service, err := New(repo, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	resolver := releaseTestRevisionResolver{evidence: revisions, byCase: performanceReleaseRevisionVariants(revisions)}
	base, err := service.ResolvePerformanceEvidence(context.Background(), envelope, manifest.Environment)
	if err != nil {
		t.Fatal(err)
	}
	release := PerformanceReleaseRuntime{Service: service, Revisions: resolver}
	scenarios, err := release.resolvePerformanceScenarios(context.Background(), envelope, manifest, base, revisions)
	if err != nil {
		t.Fatal("scenario evidence was not resolved", err)
	}
	if err = validatePerformanceScenarioEvidence(envelope, manifest, base, scenarios); err != nil {
		for _, step := range manifest.Steps {
			if !step.Allowed {
				continue
			}
			scenario := scenarios[step.ID]
			actual := performanceCurrentBinding(envelope, scenario.Revisions.ContextID, scenario.Evidence, scenario.Revisions)
			if actual != step.Binding {
				t.Fatalf("scenario %s binding mismatch: got=%+v want=%+v", step.Kind, actual, step.Binding)
			}
		}
		t.Fatal("scenario evidence did not satisfy release validation", err)
	}
	inputs := performanceScenarioInputs(base.Suite.Suite)
	query := &performanceCompositeQuery{plans: map[string]string{}, executed: map[string]bool{}}
	runner := &GovernedRunner{Inputs: inputs, Query: query}
	factory, err := NewGovernedPerformanceReleaseAdapterFactory(runner, PerformanceIntegration, "real_postgres", "recorded", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.NewPerformanceReleaseAdapter(context.Background(), envelope, manifest, scenarios); !errors.Is(err, ErrPerformanceReuseUnproven) {
		t.Fatalf("Plan→Run operation changes were accepted as product reuse invalidation: %v", err)
	}
	var contextStep, runtimeStep PerformanceStep
	for _, step := range manifest.Steps {
		switch step.Kind {
		case "context_changed":
			contextStep = step
		case "runtime_pack_changed":
			runtimeStep = step
		}
	}
	adapter := &governedPerformanceReleaseAdapter{runner: *runner, envelope: envelope, scenarios: map[string]governedPerformanceScenario{}, mode: PerformanceIntegration, sourceMode: "real_postgres", modelMode: "recorded", runID: "invalidation-negative"}
	for _, step := range []PerformanceStep{contextStep, runtimeStep} {
		if _, err = adapter.Run(context.Background(), step, 0); !errors.Is(err, ErrPerformanceReuseUnproven) {
			t.Fatalf("stale-cache test step %s was not rejected: %v", step.Kind, err)
		}
	}
	query.mu.Lock()
	defer query.mu.Unlock()
	if len(query.scenarios) != 0 || query.planCalls != 0 || query.runCalls != 0 {
		t.Fatalf("unsupported invalidation reached Plan→Run: %+v", query.scenarios)
	}
}

func TestGovernedPerformanceActionDenialUsesPlanRun(t *testing.T) {
	now := time.Now().UTC()
	baseScopes := []string{"query.plan", "query.execute"}
	positive, err := identity.FromVerified("tenant", "actor", "session", baseScopes, now.Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	suite := testSuite()
	pack := suite.Packs[0]
	caseDef := Case{ID: "workload", Stage: StageConsumer, Input: ProtectedRef{Digest: testDigest, Retention: "protected"}}
	scenario := governedPerformanceTestScenario(positive, suite, caseDef, pack, testRuntimeConfig(pack.Model), "context")
	query := &performanceCompositeQuery{plans: map[string]string{}, executed: map[string]bool{}}
	input := LiveInput{Pack: pack, Question: &nlqexec.QuestionRequest{Context: "context", Locale: nlq.LanguageEnglish, Question: "count records"}, Run: &nlqexec.RunRequest{QueryID: "protected-query", Rows: 20, Bytes: 4096}}
	adapter := &governedPerformanceReleaseAdapter{runner: GovernedRunner{Inputs: staticInputResolver{in: input}, Query: query}, envelope: positive, scenarios: map[string]governedPerformanceScenario{"cold": scenario}, mode: PerformanceIntegration, sourceMode: "real_postgres", modelMode: "recorded", runID: "denied-release-run"}
	var deniedExecute identity.Envelope
	var executeStep PerformanceStep
	for _, removed := range baseScopes {
		denied, err := identity.FromVerified("tenant", "actor", "session", withoutPerformanceScope(baseScopes, removed), now.Add(time.Hour), time.Now)
		if err != nil {
			t.Fatal(err)
		}
		step := PerformanceStep{ID: "actions-negative", Kind: "actions_negative", Binding: performanceCurrentBinding(denied, "context", scenario.evidence, scenario.revisions), Workload: caseDef.ID, ExpectedDigest: testDigest, DeniedAction: removed}
		fixture := PerformanceAuthorityFixture{Tenant: denied.Tenant(), User: denied.User(), Session: denied.Session(), Scopes: denied.Scopes(), TargetTenant: denied.Tenant(), ReportID: "workload-report", SourceID: "source", ContextID: "context"}
		step.AuthorityOverride = &fixture
		if removed == "query.execute" {
			deniedExecute, executeStep = denied, step
		}
		bound := &authorityBoundPerformanceRunner{envelope: positive, actionEnvelope: denied, next: adapter}
		for _, probe := range []func(context.Context, PerformanceStep) (PerformanceAdapterResult, error){bound.Check, func(ctx context.Context, step PerformanceStep) (PerformanceAdapterResult, error) {
			return bound.Run(ctx, step, 0)
		}} {
			result, err := probe(context.Background(), step)
			if err != nil || !result.Denied || result.Receipt.Usage.SourceCalls != 0 || result.Receipt.Usage.ModelCalls != 0 {
				t.Fatalf("altered %s did not deny before physical work: %+v %v", removed, result, err)
			}
		}
	}
	query.mu.Lock()
	defer query.mu.Unlock()
	if query.denialCalls != 4 || query.planCalls != 0 || query.runCalls != 0 {
		t.Fatalf("action denial bypassed Plan→Run or reached model/source: denials=%d plans=%d runs=%d", query.denialCalls, query.planCalls, query.runCalls)
	}
	query.denyAfterWork = true
	query.mu.Unlock()
	if _, err := adapter.ProbeDeniedAction(context.Background(), executeStep, deniedExecute); !errors.Is(err, ErrGate) {
		t.Fatalf("denial after physical work was accepted: %v", err)
	}
	query.mu.Lock()
}

type performanceScenarioInputResolver map[string]LiveInput

func (r performanceScenarioInputResolver) ResolveEvaluationInput(_ context.Context, _ identity.Envelope, ref ProtectedRef) (LiveInput, error) {
	input, ok := r[ref.Digest]
	if !ok {
		return LiveInput{}, store.ErrNotFound
	}
	return input, nil
}

func performanceScenarioInputs(suite Suite) performanceScenarioInputResolver {
	inputs := performanceScenarioInputResolver{}
	for _, workload := range suite.Cases {
		contextID := "context-a"
		if workload.ID == "case-context" {
			contextID = "context-b"
		}
		pack := suite.Packs[0]
		if workload.ID == "case-runtime" {
			pack = suite.Packs[1]
		}
		inputs[workload.Input.Digest] = LiveInput{
			Pack:     pack,
			Question: &nlqexec.QuestionRequest{Context: contextID, Locale: nlq.LanguageEnglish, Question: "measure " + workload.ID},
			Run:      &nlqexec.RunRequest{QueryID: "protected-query", Operation: "protected-operation", Rows: 20, Bytes: 4096},
		}
	}
	return inputs
}

func governedPerformanceTestScenario(e identity.Envelope, suite Suite, workload Case, pack PackRevision, config gateway.RuntimeConfig, contextID string) governedPerformanceScenario {
	suite.Mode = Live
	suite.Cases = []Case{workload}
	suite.Packs = []PackRevision{pack}
	revisions := PerformanceRevisionEvidence{
		TargetTenant: e.Tenant(), WorkloadReport: "workload-report", SourceID: "source", ContextID: contextID,
		SourceRevision: testDigest, RuleRevision: testDigest, TopicRevision: testDigest,
		DatasetDigest: testDigest, DatasetRows: 1,
	}
	runtimePack := RuntimePackRecord{Pack: pack, Config: config, State: Accepted}
	evidence := PerformanceReleaseEvidence{
		Suite:       SuiteRecord{Suite: suite, Digest: testDigest, State: Accepted},
		Report:      Report{Pack: pack},
		Case:        workload,
		CaseResult:  CaseResult{ID: workload.ID, Passed: true, Observation: Observation{Decision: "completed", SemanticDigest: testDigest}},
		RuntimePack: runtimePack,
	}
	return governedPerformanceScenario{evidence: evidence, revisions: revisions}
}

type performanceCompositeQuery struct {
	mu            sync.Mutex
	composeMu     sync.Mutex
	plans         map[string]string
	executed      map[string]bool
	planCalls     int
	runCalls      int
	denialCalls   int
	denyAfterWork bool
	receipt       gateway.Receipt
	scenarios     []performanceCompositeScenario
}

type performanceCompositeScenario struct {
	context string
	model   string
}

func (q *performanceCompositeQuery) PerformanceModelMode() string     { return "recorded" }
func (q *performanceCompositeQuery) PerformancePlanRunIsolated() bool { return true }

func (q *performanceCompositeQuery) PlanAndRun(ctx context.Context, e identity.Envelope, plan nlqexec.PlanRequest, run nlqexec.RunRequest) (nlqexec.PlanResult, nlqexec.RunResult, error) {
	q.composeMu.Lock()
	defer q.composeMu.Unlock()
	if !e.Has("query.plan") || !e.Has("query.execute") {
		q.mu.Lock()
		q.denialCalls++
		late := q.denyAfterWork
		q.mu.Unlock()
		if late {
			return nlqexec.PlanResult{}, nlqexec.RunResult{Execution: readexec.ExecutionReport{Attempt: readexec.Attempt{ID: "late-physical-attempt"}}}, access.ErrForbidden
		}
		return nlqexec.PlanResult{}, nlqexec.RunResult{}, access.ErrForbidden
	}
	model, _, _ := gateway.ApplyRuntimeConfig(ctx, "sqlgen", "configured", "")
	q.mu.Lock()
	q.scenarios = append(q.scenarios, performanceCompositeScenario{context: plan.Context, model: model})
	q.mu.Unlock()
	planned, err := q.Plan(ctx, e, plan)
	if err != nil {
		return nlqexec.PlanResult{}, nlqexec.RunResult{}, err
	}
	run.QueryID = planned.QueryID
	runResult, err := q.Run(ctx, e, run)
	return planned, runResult, err
}

func (q *performanceCompositeQuery) Preflight(context.Context, identity.Envelope, nlqexec.PreflightRequest) (nlqexec.PreflightResult, error) {
	return nlqexec.PreflightResult{}, nlqexec.ErrInvalid
}

func (q *performanceCompositeQuery) Plan(_ context.Context, _ identity.Envelope, request nlqexec.PlanRequest) (nlqexec.PlanResult, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if queryID := q.plans[request.Operation]; queryID != "" {
		return nlqexec.PlanResult{QueryID: queryID, SessionID: "session", Status: "planned"}, nil
	}
	q.planCalls++
	queryID := "query-" + request.Operation[len(request.Operation)-8:]
	q.plans[request.Operation] = queryID
	return nlqexec.PlanResult{QueryID: queryID, SessionID: "session", Status: "planned", Receipt: q.receipt}, nil
}

func (q *performanceCompositeQuery) Run(_ context.Context, _ identity.Envelope, request nlqexec.RunRequest) (nlqexec.RunResult, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.runCalls++
	if q.plans[request.Operation] != request.QueryID {
		return nlqexec.RunResult{}, store.ErrConflict
	}
	result := &readexec.Result{Schema: []readexec.Field{{Name: "count", Type: "integer", NativeType: "int8", Encoding: "decimal"}}, Rows: [][]json.RawMessage{{json.RawMessage(`1`)}}, Outcome: "complete"}
	out := nlqexec.RunResult{QueryID: request.QueryID, SessionID: "session", Status: "succeeded", Execution: readexec.ExecutionReport{Result: result}, Receipt: q.receipt}
	if q.executed[request.Operation] {
		return out, nil
	}
	q.executed[request.Operation] = true
	created := time.Now().UTC()
	finished := created.Add(7 * time.Millisecond)
	out.Execution.Attempt = readexec.Attempt{ID: "attempt", Status: "succeeded", Created: created, Finished: &finished}
	out.Execution.Attempt.Manifest.Receipt.Dialect = "postgres"
	return out, nil
}

func (q *performanceCompositeQuery) InspectSaved(context.Context, identity.Envelope, nlqexec.SavedQuestion) (nlqexec.SavedEvidence, error) {
	return nlqexec.SavedEvidence{}, nlqexec.ErrInvalid
}

func (q *performanceCompositeQuery) ResolveOperationQueryID(_ context.Context, _ identity.Envelope, operation string) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if id := q.plans[operation]; id != "" {
		return id, nil
	}
	return "", store.ErrNotFound
}

type unmarkedPerformanceQuery struct{ queryRuntime }

type nonisolatedPerformanceQuery struct{ query *performanceCompositeQuery }

func (q nonisolatedPerformanceQuery) Preflight(ctx context.Context, e identity.Envelope, request nlqexec.PreflightRequest) (nlqexec.PreflightResult, error) {
	return q.query.Preflight(ctx, e, request)
}
func (q nonisolatedPerformanceQuery) Plan(ctx context.Context, e identity.Envelope, request nlqexec.PlanRequest) (nlqexec.PlanResult, error) {
	return q.query.Plan(ctx, e, request)
}
func (q nonisolatedPerformanceQuery) Run(ctx context.Context, e identity.Envelope, request nlqexec.RunRequest) (nlqexec.RunResult, error) {
	return q.query.Run(ctx, e, request)
}
func (q nonisolatedPerformanceQuery) InspectSaved(ctx context.Context, e identity.Envelope, request nlqexec.SavedQuestion) (nlqexec.SavedEvidence, error) {
	return q.query.InspectSaved(ctx, e, request)
}
func (q nonisolatedPerformanceQuery) PerformanceModelMode() string     { return "recorded" }
func (q nonisolatedPerformanceQuery) PerformancePlanRunIsolated() bool { return false }
func (q nonisolatedPerformanceQuery) PlanAndRun(ctx context.Context, e identity.Envelope, plan nlqexec.PlanRequest, run nlqexec.RunRequest) (nlqexec.PlanResult, nlqexec.RunResult, error) {
	return q.query.PlanAndRun(ctx, e, plan, run)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
