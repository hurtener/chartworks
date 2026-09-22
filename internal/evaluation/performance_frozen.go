package evaluation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

func digestBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// FrozenRunMeasurement is bounded prerequisite evidence from one actual
// reporting Runs operation. It is not a final_stress profile or AC03 result.
type FrozenRunMeasurement struct {
	RunID          string
	ReuseKey       string
	ReusedFrom     string
	SemanticDigest string
	Usage          PerformanceUsage
}

// FrozenPerformanceReleaseAdapterFactory enters the product's frozen-run reuse
// path. Inputs and the selected runtime pack remain protected store material.
type FrozenPerformanceReleaseAdapterFactory struct {
	Inputs    LiveInputResolver
	Runs      frozenRunRuntime
	Store     frozenRunReader
	Clock     Clock
	ModelMode string
}

func NewFrozenPerformanceReleaseAdapterFactory(inputs LiveInputResolver, runs frozenRunRuntime, repo frozenRunReader, modelMode string, clock Clock) (*FrozenPerformanceReleaseAdapterFactory, error) {
	if inputs == nil || runs == nil || repo == nil || modelMode != "recorded" && modelMode != "live" {
		return nil, ErrMode
	}
	return &FrozenPerformanceReleaseAdapterFactory{Inputs: inputs, Runs: runs, Store: repo, ModelMode: modelMode, Clock: clock}, nil
}

// ObserveBounded exercises a protected frozen consumer with a caller-selected
// unique operation key. The caller must separately establish accepted Phase 24
// report and current Phase 34 revision evidence before release use.
func (f *FrozenPerformanceReleaseAdapterFactory) ObserveBounded(ctx context.Context, e identity.Envelope, ref ProtectedRef, pack PackRevision, cfg gateway.RuntimeConfig, operationKey string, reuseAge int) (FrozenRunMeasurement, error) {
	return f.observe(ctx, e, ref, pack, cfg, operationKey, reuseAge, nil)
}

func (f *FrozenPerformanceReleaseAdapterFactory) observe(ctx context.Context, e identity.Envelope, ref ProtectedRef, pack PackRevision, cfg gateway.RuntimeConfig, operationKey string, reuseAge int, revisions *PerformanceRevisionEvidence) (FrozenRunMeasurement, error) {
	if f == nil || ctx == nil || !e.Valid() || !identity.Identifier(operationKey) || reuseAge < 0 || reuseAge > 86400 || !validPack(pack) || cfg.Digest != pack.ConfigurationDigest || !packModelsMatchConfig(pack, cfg) {
		return FrozenRunMeasurement{}, fmt.Errorf("%w: selected pack or configuration", ErrPerformanceEvidence)
	}
	input, err := f.Inputs.ResolveEvaluationInput(ctx, e, ref)
	if err != nil || input.Frozen == nil || input.Question != nil || input.Run != nil || !validPack(input.Pack) {
		return FrozenRunMeasurement{}, fmt.Errorf("%w: protected frozen input", ErrPerformanceEvidence)
	}
	want, wantErr := digest(pack)
	got, gotErr := digest(input.Pack)
	if wantErr != nil || gotErr != nil || want != got {
		return FrozenRunMeasurement{}, fmt.Errorf("%w: protected pack mismatch", ErrPerformanceEvidence)
	}
	runInput := *input.Frozen
	runInput.Request.Key = operationKey
	runInput.Request.ReuseMaxAgeSeconds = reuseAge
	ctx, err = gateway.WithRuntimeConfig(ctx, cfg)
	if err != nil {
		return FrozenRunMeasurement{}, ErrReview
	}
	started := time.Now()
	record, err := runFrozenInput(ctx, e, f.Runs, f.Store, runInput)
	if err != nil {
		return FrozenRunMeasurement{}, err
	}
	semantic, sourceCalls, sourceNS, receipt, err := frozenEvidence(record)
	if err != nil {
		return FrozenRunMeasurement{}, err
	}
	if revisions != nil && !frozenMatchesRevisions(record, *revisions) {
		return FrozenRunMeasurement{}, fmt.Errorf("%w: current frozen pins", ErrPerformanceEvidence)
	}
	if !receiptMatchesPack(receipt, pack) {
		return FrozenRunMeasurement{}, ErrReview
	}
	raw, err := json.Marshal(semantic)
	if err != nil {
		return FrozenRunMeasurement{}, ErrInvalid
	}
	usage := gatewayUsage(receipt)
	physical := PerformanceUsage{ServiceNS: time.Since(started).Nanoseconds(), SourceCalls: sourceCalls, ModelCalls: usage.Calls, Retries: usage.Retries, Tokens: cloneInt(usage.Tokens), CostUSD: cloneFloat(usage.CostUSD)}
	physical.SourceNS = sourceNS
	if usage.ModelMS != nil {
		value, valid := performanceMillisNS(*usage.ModelMS)
		if !valid {
			return FrozenRunMeasurement{}, ErrInvalid
		}
		physical.ModelNS = &value
	}
	if record.View.ReusedFrom != "" && (physical.SourceCalls != 0 || physical.ModelCalls != 0 || physical.SourceNS != nil || physical.ModelNS != nil) {
		return FrozenRunMeasurement{}, ErrPerformanceEvidence
	}
	return FrozenRunMeasurement{RunID: record.View.ID, ReuseKey: record.Manifest.ReuseKey, ReusedFrom: record.View.ReusedFrom, SemanticDigest: digestBytes(raw), Usage: physical}, nil
}

func frozenMatchesRevisions(record reporting.RunRecord, revision PerformanceRevisionEvidence) bool {
	m := record.Manifest
	if m == nil || revision.BlockID != m.Block || revision.BlockRevision != m.Revision.Number || revision.BlockDigest != m.Revision.Digest || revision.SourceID != m.Binding.Source || revision.ContextID != m.Binding.Context || revision.SourceHead != m.Binding.Revision || revision.SourceHead < 1 || len(revision.TopicPins) == 0 {
		return false
	}
	if !reflect.DeepEqual(m.Revision.Definition.Topics, revision.TopicPins) || !reflect.DeepEqual(m.Rules, revision.RulePins) || len(m.Definitions) != len(revision.TopicPins) {
		return false
	}
	for i, pin := range revision.TopicPins {
		if m.Definitions[i].Topic != pin.Topic || m.Definitions[i].Version != pin.Version {
			return false
		}
	}
	return true
}

func (f *FrozenPerformanceReleaseAdapterFactory) NewPerformanceReleaseAdapter(ctx context.Context, e identity.Envelope, manifest PerformanceManifest, scenarios map[string]PerformanceScenarioEvidence) (PerformanceReleaseAdapter, error) {
	if f == nil || ctx == nil || !e.Valid() || manifest.Validate() != nil || manifest.Kind != PerformanceFinalStress || manifest.Environment.SourceMode != "real_postgres" || manifest.Environment.ModelMode != f.ModelMode || len(scenarios) == 0 {
		return nil, ErrMode
	}
	prepared := make(map[string]PerformanceScenarioEvidence, len(scenarios))
	for _, step := range manifest.Steps {
		if !step.Allowed {
			continue
		}
		scenario, ok := scenarios[step.ID]
		if !ok || scenario.Evidence.Case.ID != step.Workload || scenario.Evidence.Case.Stage != StageConsumer || scenario.Evidence.CaseResult.Observation.SemanticDigest != step.ExpectedDigest || scenario.Evidence.RuntimePack.State != Accepted || performanceCurrentBinding(e, scenario.Revisions.ContextID, scenario.Evidence, scenario.Revisions) != step.Binding {
			return nil, ErrPerformanceEvidence
		}
		input, err := f.Inputs.ResolveEvaluationInput(ctx, e, scenario.Evidence.Case.Input)
		if err != nil || input.Frozen == nil || input.Question != nil || input.Run != nil || input.Frozen.BlockID != scenario.Revisions.BlockID || !frozenRevisionsComplete(scenario.Revisions) {
			return nil, ErrPerformanceEvidence
		}
		prepared[step.ID] = scenario
	}
	// The current reviewed-pack seam cannot yet prove that a changed accepted
	// pack is actually selected by a frozen narrative consumer. No final
	// profile may enter timing until that product binding lands.
	if _, ok := performanceStep(manifest.Steps, "runtime_pack_changed"); ok {
		return nil, ErrPerformanceReuseUnproven
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, ErrPerformanceEvidence
	}
	return &frozenReleaseAdapter{factory: f, envelope: e, scenarios: prepared, nonce: hex.EncodeToString(nonce[:])}, nil
}

func frozenRevisionsComplete(r PerformanceRevisionEvidence) bool {
	return identity.Identifier(r.BlockID) && r.BlockRevision > 0 && validDigest(r.BlockDigest) && r.SourceHead > 0 && len(r.TopicPins) > 0
}

type frozenReleaseAdapter struct {
	factory   *FrozenPerformanceReleaseAdapterFactory
	envelope  identity.Envelope
	scenarios map[string]PerformanceScenarioEvidence
	nonce     string
	epoch     atomic.Uint64
}

func (a *frozenReleaseAdapter) EvidenceMode() PerformanceEvidenceMode {
	if a.factory.ModelMode == "live" {
		return PerformanceLive
	}
	return PerformanceIntegration
}
func (a *frozenReleaseAdapter) SourceMode() string { return "real_postgres" }
func (a *frozenReleaseAdapter) ModelMode() string  { return a.factory.ModelMode }

func (a *frozenReleaseAdapter) Check(ctx context.Context, step PerformanceStep) (PerformanceAdapterResult, error) {
	return a.run(ctx, step, "probe", 0, 0)
}
func (a *frozenReleaseAdapter) Reset(ctx context.Context) error {
	if ctx == nil || !a.envelope.Valid() || ctx.Err() != nil {
		return ErrPerformanceAuthority
	}
	a.epoch.Add(1)
	return nil
}
func (a *frozenReleaseAdapter) Run(ctx context.Context, step PerformanceStep, iteration int) (PerformanceAdapterResult, error) {
	if iteration < 0 {
		return PerformanceAdapterResult{}, ErrInvalid
	}
	age := 60
	if iteration == 0 && (step.Kind == "cold" || step.Kind == "concurrent" || strings.HasSuffix(step.Kind, "_changed")) {
		age = 0
	}
	return a.run(ctx, step, "measure", iteration, age)
}
func (a *frozenReleaseAdapter) run(ctx context.Context, step PerformanceStep, stage string, iteration, age int) (PerformanceAdapterResult, error) {
	if a == nil || ctx == nil || !step.Allowed || strings.HasSuffix(step.Kind, "_changed") && step.Kind == "runtime_pack_changed" {
		return PerformanceAdapterResult{}, ErrPerformanceReuseUnproven
	}
	scenario, ok := a.scenarios[step.ID]
	if !ok {
		return PerformanceAdapterResult{}, ErrPerformanceEvidence
	}
	key := "p25:" + digestBytes([]byte(a.nonce + ":" + stage + ":" + step.ID + ":" + strconv.FormatUint(a.epoch.Load(), 10) + ":" + strconv.Itoa(iteration)))[:56]
	result, err := a.factory.observe(ctx, a.envelope, scenario.Evidence.Case.Input, scenario.Evidence.RuntimePack.Pack, scenario.Evidence.RuntimePack.Config, key, age, &scenario.Revisions)
	if err != nil {
		return PerformanceAdapterResult{}, err
	}
	if result.SemanticDigest != step.ExpectedDigest || result.ReuseKey == "" {
		return PerformanceAdapterResult{}, ErrGate
	}
	if stage == "probe" && (result.ReusedFrom != "" || result.Usage.SourceNS == nil || result.Usage.ModelNS == nil) {
		return PerformanceAdapterResult{}, ErrGate
	}
	return PerformanceAdapterResult{SemanticDigest: result.SemanticDigest, BindingDigest: step.Binding.digest(), Receipt: PerformanceReceipt{Usage: result.Usage}}, nil
}

func (a *frozenReleaseAdapter) ProbeDeniedAction(ctx context.Context, step PerformanceStep, denied identity.Envelope) (PerformanceAdapterResult, error) {
	if a == nil || ctx == nil || !denied.Valid() || step.DeniedAction != "reporting.execute" || denied.Has(step.DeniedAction) {
		return PerformanceAdapterResult{}, ErrPerformanceAuthority
	}
	for _, scenario := range a.scenarios {
		if scenario.Evidence.Case.ID == step.Workload {
			input, err := a.factory.Inputs.ResolveEvaluationInput(ctx, a.envelope, scenario.Evidence.Case.Input)
			if err != nil || input.Frozen == nil {
				return PerformanceAdapterResult{}, ErrPerformanceEvidence
			}
			request := input.Frozen.Request
			request.Key = "p25:" + digestBytes([]byte(a.nonce + ":denied"))[:56]
			view, runErr := a.factory.Runs.Admit(ctx, denied, input.Frozen.BlockID, request)
			if !errors.Is(runErr, access.ErrForbidden) || view.ID != "" {
				return PerformanceAdapterResult{}, ErrGate
			}
			return blockedPerformanceResult(step), nil
		}
	}
	return PerformanceAdapterResult{}, ErrPerformanceEvidence
}
