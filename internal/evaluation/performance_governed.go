package evaluation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

// GovernedPerformanceReleaseAdapterFactory composes the existing protected
// evaluation input resolver and governed query service into a real Plan→Run
// request. Source and model modes describe the composition root's concrete
// services; each request still has to produce matching runtime receipts.
type GovernedPerformanceReleaseAdapterFactory struct {
	Runner       *GovernedRunner
	EvidenceMode PerformanceEvidenceMode
	SourceMode   string
	ModelMode    string
	Clock        Clock
}

// ErrPerformanceReuseUnproven means Plan→Run's operation ledger cannot prove
// invalidation of the product's frozen-run reuse key. Final stress stays open.
var ErrPerformanceReuseUnproven = errors.New("evaluation: product reuse invalidation unproven")

// NewGovernedPerformanceReleaseAdapterFactory binds an existing governed
// runner to one explicitly selected PostgreSQL/model evidence mode. It does
// not construct a gateway, source connection, or identity envelope.
func NewGovernedPerformanceReleaseAdapterFactory(runner *GovernedRunner, mode PerformanceEvidenceMode, sourceMode, modelMode string, clock Clock) (*GovernedPerformanceReleaseAdapterFactory, error) {
	if runner == nil || runner.Inputs == nil || runner.Query == nil || (mode != PerformanceIntegration && mode != PerformanceLive) || sourceMode != "real_postgres" || (mode == PerformanceIntegration && modelMode != "recorded") || (mode == PerformanceLive && modelMode != "live") {
		return nil, ErrMode
	}
	query, ok := runner.Query.(interface{ PerformanceModelMode() string })
	if !ok || query.PerformanceModelMode() != modelMode {
		return nil, ErrMode
	}
	isolation, ok := runner.Query.(interface{ PerformancePlanRunIsolated() bool })
	if !ok || !isolation.PerformancePlanRunIsolated() {
		return nil, ErrMode
	}
	if _, ok := runner.Query.(interface {
		PlanAndRun(context.Context, identity.Envelope, nlqexec.PlanRequest, nlqexec.RunRequest) (nlqexec.PlanResult, nlqexec.RunResult, error)
	}); !ok {
		return nil, ErrMode
	}
	return &GovernedPerformanceReleaseAdapterFactory{Runner: runner, EvidenceMode: mode, SourceMode: sourceMode, ModelMode: modelMode, Clock: clock}, nil
}

// NewPerformanceReleaseAdapter verifies that the selected accepted workload
// is a protected consumer case with both question and run material at runtime.
func (f *GovernedPerformanceReleaseAdapterFactory) NewPerformanceReleaseAdapter(ctx context.Context, envelope identity.Envelope, manifest PerformanceManifest, scenarios map[string]PerformanceScenarioEvidence) (PerformanceReleaseAdapter, error) {
	if f == nil || ctx == nil || !envelope.Valid() || f.Runner == nil || f.Runner.Inputs == nil || f.Runner.Query == nil || manifest.Validate() != nil || manifest.Kind != PerformanceFinalStress || manifest.EvidenceMode != f.EvidenceMode || manifest.Environment.SourceMode != f.SourceMode || manifest.Environment.ModelMode != f.ModelMode || len(scenarios) == 0 {
		return nil, ErrMode
	}
	prepared := make(map[string]governedPerformanceScenario, len(scenarios))
	requiresProductReuse := false
	for _, step := range manifest.Steps {
		if !step.Allowed {
			continue
		}
		if strings.HasSuffix(step.Kind, "_changed") {
			requiresProductReuse = true
		}
		scenario, ok := scenarios[step.ID]
		if !ok {
			return nil, ErrPerformanceEvidence
		}
		evidence, revisions := scenario.Evidence, scenario.Revisions
		if evidence.Case.ID == "" || evidence.Case.ID != step.Workload || evidence.Case.ID != evidence.CaseResult.ID || evidence.Case.Stage != StageConsumer || evidence.Case.Critical || !evidence.CaseResult.Passed || evidence.CaseResult.Observation.SemanticDigest != step.ExpectedDigest || evidence.RuntimePack.State != Accepted {
			return nil, ErrPerformanceEvidence
		}
		if _, ok := evidence.Suite.Suite.pack(evidence.RuntimePack.Pack.Digest); !ok || evidence.Report.Pack.Digest != evidence.RuntimePack.Pack.Digest || evidence.RuntimePack.Pack.ConfigurationDigest != evidence.RuntimePack.Config.Digest {
			return nil, ErrPerformanceEvidence
		}
		if performanceCurrentBinding(envelope, revisions.ContextID, evidence, revisions) != step.Binding {
			return nil, ErrPerformanceEvidence
		}
		input, err := f.Runner.Inputs.ResolveEvaluationInput(ctx, envelope, evidence.Case.Input)
		if err != nil || input.Question == nil || input.Run == nil || input.Question.Context != revisions.ContextID || !validPack(input.Pack) {
			return nil, ErrPerformanceEvidence
		}
		wantPack, _ := digest(evidence.RuntimePack.Pack)
		gotPack, _ := digest(input.Pack)
		if wantPack != gotPack {
			return nil, ErrPerformanceEvidence
		}
		prepared[step.ID] = governedPerformanceScenario{evidence: evidence, revisions: revisions}
	}
	if requiresProductReuse {
		// A distinct operation would force a new Plan→Run even if product
		// reuse were stale. This adapter does not enter ReuseFrozenRun.
		return nil, ErrPerformanceReuseUnproven
	}
	var runID [16]byte
	if _, err := rand.Read(runID[:]); err != nil {
		return nil, ErrPerformanceEvidence
	}
	copy := *f.Runner
	copy.Clock = f.Clock
	return &governedPerformanceReleaseAdapter{runner: copy, envelope: envelope, scenarios: prepared, mode: f.EvidenceMode, sourceMode: f.SourceMode, modelMode: f.ModelMode, runID: hex.EncodeToString(runID[:])}, nil
}

type governedPerformanceScenario struct {
	evidence  PerformanceReleaseEvidence
	revisions PerformanceRevisionEvidence
}

type governedPerformanceReleaseAdapter struct {
	runner     GovernedRunner
	envelope   identity.Envelope
	scenarios  map[string]governedPerformanceScenario
	mode       PerformanceEvidenceMode
	sourceMode string
	modelMode  string
	epoch      atomic.Uint64
	runID      string
}

func (a *governedPerformanceReleaseAdapter) EvidenceMode() PerformanceEvidenceMode { return a.mode }
func (a *governedPerformanceReleaseAdapter) SourceMode() string                    { return a.sourceMode }
func (a *governedPerformanceReleaseAdapter) ModelMode() string                     { return a.modelMode }

// ProbeDeniedAction sends the altered verifier-produced authority through the
// same governed Plan→Run entry point. A denial is evidence only when it occurs
// before any query, source or gateway receipt is produced.
func (a *governedPerformanceReleaseAdapter) ProbeDeniedAction(ctx context.Context, step PerformanceStep, denied identity.Envelope) (PerformanceAdapterResult, error) {
	if a == nil || ctx == nil || !a.envelope.Valid() || !denied.Valid() || step.Allowed || (step.DeniedAction != "query.plan" && step.DeniedAction != "query.execute") || denied.Has(step.DeniedAction) || denied.Tenant() != a.envelope.Tenant() || denied.User() != a.envelope.User() || denied.Session() != a.envelope.Session() {
		return PerformanceAdapterResult{}, ErrPerformanceAuthority
	}
	var scenario governedPerformanceScenario
	found := false
	for _, candidate := range a.scenarios {
		if candidate.evidence.Case.ID == step.Workload && performanceCurrentBinding(denied, candidate.revisions.ContextID, candidate.evidence, candidate.revisions) == step.Binding {
			scenario, found = candidate, true
			break
		}
	}
	if !found {
		return PerformanceAdapterResult{}, ErrPerformanceEvidence
	}
	input, err := a.runner.Inputs.ResolveEvaluationInput(ctx, a.envelope, scenario.evidence.Case.Input)
	if err != nil || input.Question == nil || input.Run == nil || input.Question.Context != scenario.revisions.ContextID {
		return PerformanceAdapterResult{}, ErrPerformanceEvidence
	}
	wantPack, wantErr := digest(scenario.evidence.RuntimePack.Pack)
	gotPack, gotErr := digest(input.Pack)
	if wantErr != nil || gotErr != nil || wantPack != gotPack {
		return PerformanceAdapterResult{}, ErrPerformanceEvidence
	}
	composite, ok := a.runner.Query.(compositePlanRunRuntime)
	if !ok {
		return PerformanceAdapterResult{}, ErrMode
	}
	operation := performanceRunOperation(a.runID, "denial", step, a.epoch.Load())
	run := *input.Run
	run.Operation = operation
	planned, result, err := composite.PlanAndRun(ctx, denied, nlqexec.PlanRequest{QuestionRequest: *input.Question, Operation: operation}, run)
	if !errors.Is(err, access.ErrForbidden) || planned.QueryID != "" || len(planned.Receipt.Calls) != 0 || result.QueryID != "" || result.Execution.Attempt.ID != "" || result.Execution.Result != nil || len(result.Receipt.Calls) != 0 {
		return PerformanceAdapterResult{}, ErrGate
	}
	return blockedPerformanceResult(step), nil
}

func (a *governedPerformanceReleaseAdapter) Check(ctx context.Context, step PerformanceStep) (PerformanceAdapterResult, error) {
	if !step.Allowed {
		return PerformanceAdapterResult{}, ErrPerformanceAuthority
	}
	if strings.HasSuffix(step.Kind, "_changed") {
		return PerformanceAdapterResult{}, ErrPerformanceReuseUnproven
	}
	return a.observe(ctx, step, performanceRunOperation(a.runID, "probe", step, a.epoch.Load()))
}

func (a *governedPerformanceReleaseAdapter) Reset(ctx context.Context) error {
	if ctx == nil || !a.envelope.Valid() {
		return ErrPerformanceAuthority
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Advancing the opaque idempotency namespace makes the next request a real
	// fresh Plan→Run against the existing durable query/read operation ledgers.
	a.epoch.Add(1)
	return nil
}

func (a *governedPerformanceReleaseAdapter) Run(ctx context.Context, step PerformanceStep, iteration int) (PerformanceAdapterResult, error) {
	if iteration < 0 || !step.Allowed {
		return PerformanceAdapterResult{}, ErrPerformanceAuthority
	}
	if strings.HasSuffix(step.Kind, "_changed") {
		return PerformanceAdapterResult{}, ErrPerformanceReuseUnproven
	}
	return a.observe(ctx, step, performanceRunOperation(a.runID, "measure", step, a.epoch.Load()))
}

func (a *governedPerformanceReleaseAdapter) observe(ctx context.Context, step PerformanceStep, operation string) (PerformanceAdapterResult, error) {
	scenario, ok := a.scenarios[step.ID]
	if ctx == nil || !a.envelope.Valid() || !identity.Identifier(operation) || !ok || !step.Allowed {
		return PerformanceAdapterResult{}, ErrPerformanceAuthority
	}
	evidence, revisions := scenario.evidence, scenario.revisions
	if evidence.Case.ID != step.Workload || evidence.Case.Stage != StageConsumer || evidence.RuntimePack.State != Accepted || performanceCurrentBinding(a.envelope, revisions.ContextID, evidence, revisions) != step.Binding {
		return PerformanceAdapterResult{}, ErrPerformanceEvidence
	}
	deadline := time.Now().Add(time.Duration(evidence.Suite.Suite.Limits.DurationMS) * time.Millisecond)
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	operationCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	limits := evidence.Suite.Suite.Limits
	reservation := Reservation{Calls: limits.Calls, Tokens: limits.Tokens, Retries: limits.Retries, Deadline: deadline}
	if limits.CostUSD != nil {
		cost := *limits.CostUSD
		reservation.CostUSD = &cost
	}
	input := performanceOperationInputResolver{next: a.runner.Inputs, operation: operation, expectedRef: evidence.Case.Input, expectedContext: revisions.ContextID, expectedPack: evidence.RuntimePack.Pack}
	runner := a.runner
	runner.Inputs = input
	serviceStarted := time.Now()
	observation, err := runner.Observe(operationCtx, Execution{Suite: evidence.Suite.Suite, Case: evidence.Case, Pack: evidence.RuntimePack.Pack, RuntimeConfig: evidence.RuntimePack.Config, Reservation: reservation, Envelope: a.envelope})
	serviceNS := time.Since(serviceStarted).Nanoseconds()
	if err != nil {
		return PerformanceAdapterResult{}, err
	}
	if observation.Decision != "completed" || observation.Blocked || observation.SemanticDigest == "" {
		return PerformanceAdapterResult{}, ErrGate
	}
	if serviceNS < 0 {
		return PerformanceAdapterResult{}, ErrInvalid
	}
	usage := PerformanceUsage{ServiceNS: serviceNS, SourceCalls: observation.Usage.SourceCalls, ModelCalls: observation.Usage.Calls, Retries: observation.Usage.Retries}
	if observation.Usage.SourceMS != nil {
		value, valid := performanceMillisNS(*observation.Usage.SourceMS)
		if !valid {
			return PerformanceAdapterResult{}, ErrInvalid
		}
		usage.SourceNS = &value
	}
	if observation.Usage.ModelMS != nil {
		value, valid := performanceMillisNS(*observation.Usage.ModelMS)
		if !valid {
			return PerformanceAdapterResult{}, ErrInvalid
		}
		usage.ModelNS = &value
	}
	usage.Tokens = cloneInt(observation.Usage.Tokens)
	usage.CostUSD = cloneFloat(observation.Usage.CostUSD)
	return PerformanceAdapterResult{SemanticDigest: observation.SemanticDigest, BindingDigest: step.Binding.digest(), Receipt: PerformanceReceipt{Usage: usage}}, nil
}

type performanceOperationInputResolver struct {
	next            LiveInputResolver
	operation       string
	expectedRef     ProtectedRef
	expectedContext string
	expectedPack    PackRevision
}

func (r performanceOperationInputResolver) ResolveEvaluationInput(ctx context.Context, envelope identity.Envelope, ref ProtectedRef) (LiveInput, error) {
	if r.next == nil || !identity.Identifier(r.operation) || ref != r.expectedRef {
		return LiveInput{}, ErrMode
	}
	in, err := r.next.ResolveEvaluationInput(ctx, envelope, ref)
	if err != nil {
		return LiveInput{}, err
	}
	if in.Question == nil || in.Run == nil || in.Question.Context != r.expectedContext {
		return LiveInput{}, ErrMode
	}
	wantPack, wantErr := digest(r.expectedPack)
	gotPack, gotErr := digest(in.Pack)
	if wantErr != nil || gotErr != nil || wantPack != gotPack {
		return LiveInput{}, ErrPerformanceEvidence
	}
	request := *in.Run
	request.Operation = r.operation
	in.Run = &request
	return in, nil
}

func performanceRunOperation(runID, stage string, step PerformanceStep, epoch uint64) string {
	material := runID + ":" + step.Binding.digest() + ":" + stage + ":" + strconv.FormatUint(epoch, 10)
	if stage == "probe" {
		material += ":" + step.ID
	}
	sum := sha256.Sum256([]byte(material))
	return "p25:" + hex.EncodeToString(sum[:28])
}

func performanceMillisNS(value int64) (int64, bool) {
	if value < 0 || value > math.MaxInt64/int64(time.Millisecond) {
		return 0, false
	}
	return value * int64(time.Millisecond), true
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

var _ PerformanceReleaseAdapterFactory = (*GovernedPerformanceReleaseAdapterFactory)(nil)
var _ PerformanceReleaseAdapter = (*governedPerformanceReleaseAdapter)(nil)
