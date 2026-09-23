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
	"sync"
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
	Inputs              LiveInputResolver
	Runs                frozenRunRuntime
	Store               frozenRunReader
	Clock               Clock
	ModelMode           string
	Selection           PerformancePackSelection
	ChangedPackProposal string
}

// PerformancePackSelection is the existing reviewed-proposal CAS service,
// called only with the freshly verified release envelope. Proposal identifiers
// are operator composition, never profile-selected authority.
type PerformancePackSelection interface {
	SelectedPack(context.Context, identity.Envelope) (PackSelection, error)
	ProposedPackDigest(context.Context, identity.Envelope, string) (string, error)
	SelectPack(context.Context, identity.Envelope, string, int64) (PackSelection, error)
}

func NewFrozenPerformanceReleaseAdapterFactory(inputs LiveInputResolver, runs frozenRunRuntime, repo frozenRunReader, modelMode string, clock Clock) (*FrozenPerformanceReleaseAdapterFactory, error) {
	if inputs == nil || runs == nil || repo == nil || modelMode != "recorded" && modelMode != "live" {
		return nil, ErrMode
	}
	return &FrozenPerformanceReleaseAdapterFactory{Inputs: inputs, Runs: runs, Store: repo, ModelMode: modelMode, Clock: clock}, nil
}

func (f *FrozenPerformanceReleaseAdapterFactory) WithPackTransition(selection PerformancePackSelection, changedProposal string) (*FrozenPerformanceReleaseAdapterFactory, error) {
	if f == nil || selection == nil || !identity.Identifier(changedProposal) {
		return nil, ErrMode
	}
	configured := *f
	configured.Selection = selection
	configured.ChangedPackProposal = changedProposal
	return &configured, nil
}

// ObserveBounded exercises a protected frozen consumer with a caller-selected
// unique operation key. The caller must separately establish accepted Phase 24
// report and current Phase 34 revision evidence before release use.
func (f *FrozenPerformanceReleaseAdapterFactory) ObserveBounded(ctx context.Context, e identity.Envelope, ref ProtectedRef, runtime RuntimePackRecord, operationKey string, reuseAge int) (FrozenRunMeasurement, error) {
	return f.observe(ctx, e, ref, runtime, operationKey, reuseAge, nil)
}

func (f *FrozenPerformanceReleaseAdapterFactory) observe(ctx context.Context, e identity.Envelope, ref ProtectedRef, runtime RuntimePackRecord, operationKey string, reuseAge int, revisions *PerformanceRevisionEvidence) (FrozenRunMeasurement, error) {
	if f == nil || ctx == nil || !e.Valid() || !identity.Identifier(operationKey) || reuseAge < 0 || reuseAge > 86400 || runtime.Validate() != nil || runtime.State != Accepted || runtime.Review == nil || runtime.Review.Reviewer == runtime.Author {
		return FrozenRunMeasurement{}, fmt.Errorf("%w: accepted runtime pack", ErrPerformanceEvidence)
	}
	pack, cfg := runtime.Pack, runtime.Config
	input, err := f.Inputs.ResolveEvaluationInput(ctx, e, ref)
	if err != nil || input.Frozen == nil || input.Question != nil || input.Run != nil || !ProtectedPackMatches(input, pack) {
		return FrozenRunMeasurement{}, fmt.Errorf("%w: protected frozen input", ErrPerformanceEvidence)
	}
	runInput := *input.Frozen
	runInput.Request.Key = operationKey
	runInput.Request.ReuseMaxAgeSeconds = reuseAge
	ctx, err = gateway.WithRuntimeConfig(ctx, cfg)
	if err != nil {
		return FrozenRunMeasurement{}, ErrReview
	}
	started := time.Now()
	record, err := runFrozenInputChecked(ctx, e, f.Runs, f.Store, runInput, func(m reporting.RunManifest) error {
		want, valid := expectedNarrativePin(pack, cfg, runtime.Digest)
		if !valid || m.NarrativePackUnavailable || m.NarrativePack == nil || *m.NarrativePack != want || m.ReuseKey != reporting.ReuseIdentity(m) {
			return fmt.Errorf("%w: selected frozen narrative pack", ErrPerformanceEvidence)
		}
		if revisions != nil && !frozenMatchesRevisions(reporting.RunRecord{Manifest: &m}, *revisions) {
			return fmt.Errorf("%w: current frozen pins", ErrPerformanceEvidence)
		}
		return nil
	})
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
	baseStep, baseOK := performanceStep(manifest.Steps, "cold")
	changedStep, changedOK := performanceStep(manifest.Steps, "runtime_pack_changed")
	if !baseOK || !changedOK || f.Selection == nil || !identity.Identifier(f.ChangedPackProposal) {
		return nil, ErrPerformanceReuseUnproven
	}
	base, changed := prepared[baseStep.ID], prepared[changedStep.ID]
	if base.Evidence.RuntimePack.Pack.Digest == changed.Evidence.RuntimePack.Pack.Digest || base.Evidence.RuntimePack.Digest == changed.Evidence.RuntimePack.Digest {
		return nil, ErrPerformanceReuseUnproven
	}
	selected, err := f.Selection.SelectedPack(ctx, e)
	if err != nil || selected.Revision < 1 || !identity.Identifier(selected.ProposalID) || selected.PackDigest != base.Evidence.RuntimePack.Pack.Digest {
		return nil, ErrPerformanceReuseUnproven
	}
	proposed, err := f.Selection.ProposedPackDigest(ctx, e, f.ChangedPackProposal)
	if err != nil || proposed != changed.Evidence.RuntimePack.Pack.Digest {
		return nil, ErrPerformanceReuseUnproven
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, ErrPerformanceEvidence
	}
	return &frozenReleaseAdapter{factory: f, envelope: e, scenarios: prepared, nonce: hex.EncodeToString(nonce[:]), baseSelection: selected, changedPack: changed.Evidence.RuntimePack.Pack.Digest, selectionRevision: selected.Revision}, nil
}

func frozenRevisionsComplete(r PerformanceRevisionEvidence) bool {
	return identity.Identifier(r.BlockID) && r.BlockRevision > 0 && validDigest(r.BlockDigest) && r.SourceHead > 0 && len(r.TopicPins) > 0 && len(r.RulePins) == len(r.TopicPins)
}

type frozenReleaseAdapter struct {
	factory           *FrozenPerformanceReleaseAdapterFactory
	envelope          identity.Envelope
	scenarios         map[string]PerformanceScenarioEvidence
	nonce             string
	baseSelection     PackSelection
	changedPack       string
	selectionMu       sync.Mutex
	selectionRevision int64
	keysMu            sync.Mutex
	productKeys       map[string]map[string]string
	epoch             atomic.Uint64
}

func (a *frozenReleaseAdapter) EvidenceMode() PerformanceEvidenceMode {
	if a.factory.ModelMode == "live" {
		return PerformanceLive
	}
	return PerformanceIntegration
}
func (a *frozenReleaseAdapter) SourceMode() string { return "real_postgres" }
func (a *frozenReleaseAdapter) ModelMode() string  { return a.factory.ModelMode }

// Selection is changed serially outside the measured worker pool. A foreign
// pointer is never overwritten, and the database validates the approved
// proposal and CAS revision under the verified Pengui envelope.
func (a *frozenReleaseAdapter) PreparePerformanceStep(ctx context.Context, step PerformanceStep) error {
	if a == nil || ctx == nil || !a.envelope.Valid() || a.factory.Selection == nil || !step.Allowed {
		return ErrPerformanceAuthority
	}
	if step.Kind == "runtime_pack_changed" {
		return a.selectPack(ctx, a.changedPack, a.factory.ChangedPackProposal)
	}
	return a.selectPack(ctx, a.baseSelection.PackDigest, a.baseSelection.ProposalID)
}

func (a *frozenReleaseAdapter) RestorePerformanceSelection(ctx context.Context) error {
	if a == nil || ctx == nil || !a.envelope.Valid() || a.factory.Selection == nil {
		return ErrPerformanceAuthority
	}
	return a.selectPack(ctx, a.baseSelection.PackDigest, a.baseSelection.ProposalID)
}

func (a *frozenReleaseAdapter) selectPack(ctx context.Context, digest, proposal string) error {
	a.selectionMu.Lock()
	defer a.selectionMu.Unlock()
	selected, err := a.factory.Selection.SelectedPack(ctx, a.envelope)
	if err != nil || selected.Revision != a.selectionRevision || selected.PackDigest != a.baseSelection.PackDigest && selected.PackDigest != a.changedPack ||
		selected.PackDigest == a.baseSelection.PackDigest && selected.ProposalID != a.baseSelection.ProposalID ||
		selected.PackDigest == a.changedPack && selected.ProposalID != a.factory.ChangedPackProposal {
		return ErrPerformanceReuseUnproven
	}
	if selected.PackDigest == digest {
		return nil
	}
	advanced, err := a.factory.Selection.SelectPack(ctx, a.envelope, proposal, selected.Revision)
	if err != nil || advanced.Revision != selected.Revision+1 || advanced.PackDigest != digest || advanced.ProposalID != proposal || advanced.Actor != a.envelope.User() {
		return ErrPerformanceReuseUnproven
	}
	a.selectionRevision = advanced.Revision
	current, err := a.factory.Selection.SelectedPack(ctx, a.envelope)
	if err != nil || current.Revision != advanced.Revision || current.PackDigest != digest || current.ProposalID != proposal {
		return ErrPerformanceReuseUnproven
	}
	return nil
}

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
	if a == nil || ctx == nil || !step.Allowed {
		return PerformanceAdapterResult{}, ErrPerformanceAuthority
	}
	scenario, ok := a.scenarios[step.ID]
	if !ok {
		return PerformanceAdapterResult{}, ErrPerformanceEvidence
	}
	key := "p25:" + digestBytes([]byte(a.nonce + ":" + stage + ":" + step.ID + ":" + strconv.FormatUint(a.epoch.Load(), 10) + ":" + strconv.Itoa(iteration)))[:56]
	result, err := a.factory.observe(ctx, a.envelope, scenario.Evidence.Case.Input, scenario.Evidence.RuntimePack, key, age, &scenario.Revisions)
	if err != nil {
		return PerformanceAdapterResult{}, err
	}
	if result.SemanticDigest != step.ExpectedDigest || result.ReuseKey == "" {
		return PerformanceAdapterResult{}, ErrGate
	}
	if err := a.verifyProductReuseKey(stage, step, result.ReuseKey); err != nil {
		return PerformanceAdapterResult{}, err
	}
	if stage == "probe" && (result.ReusedFrom != "" || result.Usage.SourceNS == nil || result.Usage.ModelNS == nil) {
		return PerformanceAdapterResult{}, ErrGate
	}
	return PerformanceAdapterResult{SemanticDigest: result.SemanticDigest, BindingDigest: step.Binding.digest(), Receipt: PerformanceReceipt{
		Usage: result.Usage, RunID: result.RunID, ReuseKey: result.ReuseKey, ReusedFrom: result.ReusedFrom,
	}}, nil
}

// A one-field profile binding is insufficient when the product's actual
// frozen-run key did not change. This guard runs before any timing is accepted.
func (a *frozenReleaseAdapter) verifyProductReuseKey(stage string, step PerformanceStep, key string) error {
	if a == nil || (stage != "probe" && stage != "measure") || !validDigest(key) {
		return ErrPerformanceReuseUnproven
	}
	a.keysMu.Lock()
	defer a.keysMu.Unlock()
	if a.productKeys == nil {
		a.productKeys = map[string]map[string]string{}
	}
	if a.productKeys[stage] == nil {
		a.productKeys[stage] = map[string]string{}
	}
	seen := a.productKeys[stage]
	if prior := seen[step.Kind]; prior != "" && prior != key {
		return ErrPerformanceReuseUnproven
	}
	if step.Kind == "cold" {
		seen[step.Kind] = key
		return nil
	}
	cold := seen["cold"]
	if cold == "" || strings.HasSuffix(step.Kind, "_changed") && key == cold || !strings.HasSuffix(step.Kind, "_changed") && key != cold {
		return ErrPerformanceReuseUnproven
	}
	seen[step.Kind] = key
	return nil
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
