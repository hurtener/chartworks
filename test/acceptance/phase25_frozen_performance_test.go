package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/gateway/recorded"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

type staleFrozenReader struct {
	inner interface {
		ReadFrozenRun(context.Context, identity.Envelope, string, bool) (reporting.RunRecord, error)
	}
	key string
}

func (r staleFrozenReader) ReadFrozenRun(ctx context.Context, e identity.Envelope, id string, execution bool) (reporting.RunRecord, error) {
	record, err := r.inner.ReadFrozenRun(ctx, e, id, execution)
	if err == nil && record.Manifest != nil {
		manifest := *record.Manifest
		manifest.ReuseKey = r.key
		record.Manifest = &manifest
	}
	return record, err
}

type frozenNarrativeCapture struct {
	gateway.Engine
	key            string
	call           gateway.Call
	system, prompt string
	schema         *gateway.Schema
}

type frozenRecordedDiagnostic struct {
	gateway.Engine
	mu   sync.Mutex
	keys []string
	errs []error
}

func (d *frozenRecordedDiagnostic) Generate(ctx context.Context, call gateway.Call, budget *gateway.Budget, role, system, prompt string, schema *gateway.Schema) (gateway.Generated, error) {
	model, effectiveSystem, cfg := gateway.ApplyRuntimeConfig(ctx, role, "model-narrative", system)
	key := recorded.InputDigest(call, role, model, cfg, role, effectiveSystem, prompt, schema.Name(), schema.Document())
	out, err := d.Engine.Generate(ctx, call, budget, role, system, prompt, schema)
	d.mu.Lock()
	d.keys = append(d.keys, key)
	d.errs = append(d.errs, err)
	d.mu.Unlock()
	return out, err
}

func (c *frozenNarrativeCapture) Generate(ctx context.Context, call gateway.Call, budget *gateway.Budget, role, system, prompt string, schema *gateway.Schema) (gateway.Generated, error) {
	if role != "narrative" || !call.Valid() || schema == nil {
		return gateway.Generated{}, gateway.ErrInput
	}
	model, effectiveSystem, cfg := gateway.ApplyRuntimeConfig(ctx, role, "model-narrative", system)
	c.key = recorded.InputDigest(call, role, model, cfg, role, effectiveSystem, prompt, schema.Name(), schema.Document())
	c.call, c.system, c.prompt, c.schema = call, system, prompt, schema
	if err := gateway.ReserveAttempt(ctx, budget, call, 128); err != nil {
		return gateway.Generated{}, err
	}
	return gateway.Generated{JSON: json.RawMessage(`{"claims":[{"kind":"value","evidence":["e1"]}]}`), Receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: role, Provider: "capture", RequestedModel: model, ConfigurationDigest: cfg, Attempts: 1}}}}, nil
}

// This is a bounded AC03 prerequisite over real PG17 and a recorded gateway.
// It does not satisfy the final_stress profile or live-model release criterion.
func TestPhase25FrozenPerformancePrerequisite(t *testing.T) {
	f := newReportingStoreFixture(t)
	f.definition.Outputs = f.definition.Outputs[:2]
	f.definition.Outputs[1].Narrative.SchemaVersion = "grounded-narrative-v1"
	f.definition.Outputs[1].Narrative.MaxTokens = 8192
	block := f.create(t, "p25-frozen-perf", true).State.ID
	scopes := append(phase28Scopes(f.execute.Tenant()), "ops.write")
	operator := phase27Actor(t, f.f, f.execute.User(), scopes)
	ctx := t.Context()
	cfg := gateway.RuntimeConfig{Model: "model-narrative", Models: []gateway.RuntimeModel{{Role: "narrative", Model: "model-narrative"}, {Role: "embedding", Model: "model-embed"}}, AttemptCostUSD: 0.001}
	cfg.Digest = gateway.ConfigurationDigest(cfg)
	pack := evaluation.PackRevision{ID: "p25-recorded", Revision: 1, Model: cfg.Model, Models: []evaluation.PackModel{{Role: "narrative", Model: "model-narrative"}, {Role: "embedding", Model: "model-embed"}}, ConfigurationDigest: cfg.Digest}
	pack.Digest = pack.CanonicalDigest()
	evaluationService, err := evaluation.New(f.f.f.db, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	reviewer := phase27Actor(t, f.f, "p25-reviewer", []string{"ops.audit", "cw.tenant.certify:" + operator.Tenant()})
	reviewPack := func(pack evaluation.PackRevision, cfg gateway.RuntimeConfig) evaluation.RuntimePackRecord {
		t.Helper()
		draft, authorErr := evaluationService.AuthorRuntimePack(ctx, operator, pack, cfg)
		if authorErr != nil {
			t.Fatal("author runtime pack", authorErr)
		}
		accepted, reviewErr := evaluationService.ReviewRuntimePack(ctx, reviewer, pack.Digest, evaluation.RuntimePackReviewRequest{
			PackID: pack.ID, PackRevision: pack.Revision, RuntimeDigest: draft.Digest, ConfigurationDigest: cfg.Digest,
			Model: cfg.Model, Models: cfg.Models, SystemInstruction: cfg.SystemInstruction, MaxAttemptCostUSD: cfg.AttemptCostUSD, Decision: evaluation.Accepted,
		})
		if reviewErr != nil || accepted.State != evaluation.Accepted {
			t.Fatal("review runtime pack", reviewErr)
		}
		return accepted
	}
	acceptedPack := reviewPack(pack, cfg)
	cfgB := cfg
	cfgB.SystemInstruction = "Use only the bounded evidence in the changed reviewed pack."
	cfgB.Digest = gateway.ConfigurationDigest(cfgB)
	packB := pack
	packB.ID, packB.ConfigurationDigest = "p25-recorded-changed", cfgB.Digest
	packB.Digest = packB.CanonicalDigest()
	acceptedPackB := reviewPack(packB, cfgB)
	selectNarrativePack(t, f.f, f.raw, acceptedPack, 1)
	configuredCtx, err := gateway.WithRuntimeConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	request := reporting.RunRequest{Outputs: []string{"table-main", "narrative-main"}, Narrative: true, Resolution: reporting.Resolution{At: time.Now().UTC().Truncate(time.Second), Timezone: "UTC"}}
	// Capture each selected pack's exact authorized narrative input outside measurement.
	capture := &frozenNarrativeCapture{}
	captureRuns := phase28RunService(t, f.f, f.blocks, f.f.f.db, capture, config.DefaultReportingExecution())
	captureRuns, err = captureRuns.WithReviewedNarrativePacks(f.f.f.db)
	if err != nil {
		t.Fatal(err)
	}
	request.Key = "p25-cassette-capture"
	seed, err := captureRuns.Admit(configuredCtx, operator, block, request)
	if err != nil {
		t.Fatal("capture admit", err)
	}
	seed, err = captureRuns.Run(configuredCtx, operator, seed.ID, false)
	if err != nil || seed.State != "succeeded" || len(capture.key) != 64 {
		t.Fatal("capture actual narrative input", err, seed.State)
	}
	captureA := *capture
	selectNarrativePack(t, f.f, f.raw, acceptedPackB, 2)
	request.Key = "p25-cassette-capture-changed"
	seedB, err := captureRuns.Admit(ctx, operator, block, request)
	if err != nil {
		t.Fatal("changed-pack capture admit", err)
	}
	seedB, err = captureRuns.Run(ctx, operator, seedB.ID, false)
	if err != nil || seedB.State != "succeeded" || len(capture.key) != 64 || capture.key == captureA.key {
		t.Fatal("changed reviewed pack did not change the actual narrative input", err, seedB.State)
	}
	captureB := *capture
	selectNarrativePack(t, f.f, f.raw, acceptedPack, 3)
	space := gateway.EmbeddingSpace{Provider: "recorded", Route: "integration", Endpoint: "local", Model: "model-embed", Revision: "fixture-v1", Dimensions: 2, Preprocessing: "utf8-exact;float32-finite", InputType: "text", Normalization: "no-normalization"}
	engine, err := recorded.New(space, map[string]string{"narrative": "model-narrative", "embedding": "model-embed"}, []recorded.Recording{
		{Role: "narrative", InputDigest: captureA.key, JSON: json.RawMessage(`{"claims":[{"kind":"value","evidence":["e1"]}]}`)},
		{Role: "narrative", InputDigest: captureB.key, JSON: json.RawMessage(`{"claims":[{"kind":"value","evidence":["e1"]}]}`)},
	})
	if err != nil {
		t.Fatal("recorded gateway", err)
	}
	budget, err := gateway.NewBudget(captureA.call, gateway.Limits{Calls: 1, Tokens: 4096, Duration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Generate(configuredCtx, captureA.call, budget, "narrative", captureA.system, captureA.prompt, captureA.schema); err != nil {
		t.Fatal("captured cassette did not replay exact input", err)
	}
	t.Cleanup(engine.Close)
	diagnostic := &frozenRecordedDiagnostic{Engine: engine}
	runs := phase28RunService(t, f.f, f.blocks, f.f.f.db, diagnostic, config.DefaultReportingExecution())
	runs, err = runs.WithReviewedNarrativePacks(f.f.f.db)
	if err != nil {
		t.Fatal(err)
	}
	request.Key = "p25-protected-template"
	ref, err := evaluationService.RegisterInput(ctx, operator, "protected", evaluation.LiveInput{Frozen: &evaluation.FrozenRunInput{BlockID: block, Request: request}})
	if err != nil {
		t.Fatal("register protected frozen consumer", err)
	}
	adapter, err := evaluation.NewFrozenPerformanceReleaseAdapterFactory(f.f.f.db, runs, f.f.f.db, "recorded", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	observe := func(key string, age int) evaluation.FrozenRunMeasurement {
		t.Helper()
		result, observeErr := adapter.ObserveBounded(ctx, operator, ref, acceptedPack, key, age)
		if observeErr != nil {
			t.Fatal("actual frozen observation", key, observeErr, diagnostic.keys, diagnostic.errs, "cassette match", len(diagnostic.keys) > 0 && diagnostic.keys[0] == captureA.key)
		}
		return result
	}
	cold := observe("p25-frozen-cold", 0)
	if cold.RunID == seed.ID || cold.ReusedFrom != "" || cold.Usage.SourceCalls != 1 || cold.Usage.SourceNS == nil || *cold.Usage.SourceNS <= 0 || cold.Usage.ModelCalls != 1 || cold.Usage.ModelNS == nil {
		t.Fatal("cold path lacked physical source and recorded-model receipts", cold)
	}
	warm := observe("p25-frozen-warm", 60)
	repeat := observe("p25-frozen-repeat", 60)
	origins := map[string]string{cold.RunID: "", warm.RunID: warm.ReusedFrom, repeat.RunID: repeat.ReusedFrom}
	for _, reused := range []evaluation.FrozenRunMeasurement{warm, repeat} {
		if reused.RunID == cold.RunID || reused.RunID == seed.ID || reused.ReusedFrom == "" || reused.ReuseKey != cold.ReuseKey || reused.SemanticDigest != cold.SemanticDigest || reused.Usage.SourceCalls != 0 || reused.Usage.ModelCalls != 0 || reused.Usage.SourceNS != nil || reused.Usage.ModelNS != nil {
			t.Fatal("distinct run failed real product reuse", reused, cold)
		}
	}
	if warm.ReusedFrom != cold.RunID || repeat.ReusedFrom != cold.RunID && repeat.ReusedFrom != warm.RunID {
		t.Fatal("warm/repeat reuse did not lead to measured physical origin", warm.ReusedFrom, repeat.ReusedFrom)
	}
	// The reference queue permits two simultaneous requests per tenant.
	const peers = 2
	var wg sync.WaitGroup
	results := make([]evaluation.FrozenRunMeasurement, peers)
	errs := make([]error, peers)
	start := make(chan struct{})
	for i := range peers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = adapter.ObserveBounded(ctx, operator, ref, acceptedPack, "p25-frozen-peer-"+string(rune('a'+i)), 60)
		}(i)
	}
	close(start)
	wg.Wait()
	seen := map[string]bool{cold.RunID: true, warm.RunID: true, repeat.RunID: true}
	for i, result := range results {
		if errs[i] != nil || seen[result.RunID] || result.ReusedFrom == "" || result.Usage.SourceCalls != 0 || result.Usage.ModelCalls != 0 || result.SemanticDigest != cold.SemanticDigest {
			t.Fatal("concurrent real reuse", i, errs[i], result)
		}
		seen[result.RunID] = true
		origins[result.RunID] = result.ReusedFrom
	}
	for _, result := range results {
		origin := result.ReusedFrom
		for hops := 0; origin != cold.RunID && hops < len(origins); hops++ {
			origin = origins[origin]
		}
		if origin != cold.RunID {
			t.Fatal("concurrent reuse did not resolve to measured physical origin", result.RunID)
		}
	}
	// Admit the same protected consumer through a real reviewed Phase 24 suite
	// and persist its passing report. The release resolver later binds this
	// exact report hash, never a profile-supplied observation.
	caseProof := evalCase("p25-frozen-consumer", evaluation.StageConsumer, "en", false, "")
	caseProof.Fixture, caseProof.Input = nil, ref
	caseProof.Expected = []evaluation.Expected{{Decision: "completed", SemanticDigest: cold.SemanticDigest}}
	suite := evalSuite(evaluation.Live, []evaluation.Case{caseProof})
	suite.ID, suite.Packs = "p25-frozen-suite", []evaluation.PackRevision{pack, packB}
	suite.Limits.Calls, suite.Limits.Tokens, suite.Limits.Retries = 1, 8192, 0
	draft, err := evaluationService.Author(ctx, operator, suite)
	if err != nil {
		t.Fatal("author selected Phase 24 suite", err)
	}
	acceptedSuite, err := evaluationService.Review(ctx, reviewer, suite.ID, evaluation.SuiteReviewRequest{Revision: suite.Revision, Digest: draft.Digest, Decision: evaluation.Accepted})
	if err != nil || acceptedSuite.State != evaluation.Accepted {
		t.Fatal("review selected Phase 24 suite", err)
	}
	phase24 := &evaluation.GovernedRunner{Inputs: f.f.f.db, Frozen: runs, FrozenStore: f.f.f.db}
	report, err := evaluationService.Run(ctx, operator, evaluation.RunRequest{RunID: "p25-frozen-report", SuiteID: suite.ID, SuiteRevision: suite.Revision, SuiteDigest: draft.Digest, PackDigest: pack.Digest}, phase24)
	if err != nil || !report.GatePassed || report.Status != "passed" || len(report.Cases) != 1 || report.Cases[0].Observation.SemanticDigest != cold.SemanticDigest || report.Cases[0].Observation.Usage.SourceCalls != 1 || report.Cases[0].Observation.Usage.Calls != 1 || report.Cases[0].Observation.Usage.SourceMS == nil {
		t.Fatal("accepted Phase 24 run did not exercise the actual frozen source and recorded gateway", err, report)
	}
	proof, err := evaluationService.ResolvePerformanceEvidence(ctx, operator, evaluation.PerformanceEnvironment{EvaluationSuiteID: suite.ID, EvaluationSuiteRevision: suite.Revision, EvaluationSuite: draft.Digest, EvaluationReportID: report.RunID, EvaluationReport: report.EvidenceHash, WorkloadCaseID: caseProof.ID})
	if err != nil || proof.CaseResult.Observation.SemanticDigest != cold.SemanticDigest || proof.RuntimePack.Digest != acceptedPack.Digest {
		t.Fatal("performance release selected a different Phase 24 report or pack", err)
	}
	stored, err := f.f.f.db.ReadFrozenRun(ctx, operator, cold.RunID, true)
	if err != nil || stored.Manifest == nil || stored.Manifest.ReuseKey != reporting.ReuseIdentity(*stored.Manifest) {
		t.Fatal("canonical persisted identity", err)
	}
	for _, mutate := range []func(*reporting.RunManifest){
		func(m *reporting.RunManifest) { m.Binding.Context = "other-context" },
		func(m *reporting.RunManifest) { m.Binding.Revision++ },
		func(m *reporting.RunManifest) { m.Rules = append(m.Rules, reporting.RulePin{Topic: "other-topic"}) },
		func(m *reporting.RunManifest) { m.Definitions[0].Version = "other-version" },
		func(m *reporting.RunManifest) {
			m.NarrativePack = &reporting.NarrativePackPin{PackDigest: packB.Digest, RuntimeDigest: acceptedPackB.Digest, ConfigurationDigest: cfgB.Digest, Model: "model-narrative"}
		},
	} {
		copy := phase27Copy(t, *stored.Manifest)
		mutate(&copy)
		if copy.ReuseKey == reporting.ReuseIdentity(copy) {
			t.Fatal("changed product dimension kept old canonical reuse key")
		}
	}
	wrong := acceptedPack
	wrong.Pack.Digest = evalDigest
	if _, err := adapter.ObserveBounded(ctx, operator, ref, wrong, "p25-frozen-wrong-pack", 60); !errors.Is(err, evaluation.ErrPerformanceEvidence) {
		t.Fatal("unreviewed pack substituted", err)
	}
	withoutSelection := slices.DeleteFunc(append([]string(nil), scopes...), func(s string) bool { return s == "ops.write" })
	selectionDenied := phase27Actor(t, f.f, operator.User(), withoutSelection)
	proposed, err := evaluationService.ProposedPackDigest(ctx, operator, "reviewed-narrative-"+packB.ID)
	if err != nil || proposed != packB.Digest {
		t.Fatal("reviewed proposal did not bind the expected changed pack", err, proposed)
	}
	if _, err := evaluationService.ProposedPackDigest(ctx, selectionDenied, "reviewed-narrative-"+packB.ID); err == nil {
		t.Fatal("proposal digest read bypassed signed selection authority")
	}
	if _, err := evaluationService.SelectPack(ctx, selectionDenied, "reviewed-narrative-"+packB.ID, 3); err == nil {
		t.Fatal("missing signed ops.write changed the selected pack")
	}
	selected, err := evaluationService.SelectedPack(ctx, operator)
	if err != nil || selected.Revision != 3 || selected.PackDigest != pack.Digest {
		t.Fatal("unauthorized transition changed the production selection", err, selected)
	}
	selected, err = evaluationService.SelectPack(ctx, operator, "reviewed-narrative-"+packB.ID, selected.Revision)
	if err != nil || selected.Revision != 4 || selected.PackDigest != packB.Digest || selected.Actor != operator.User() {
		t.Fatal("authorized changed-pack transition did not advance the reviewed CAS pointer", err, selected)
	}
	beforeChanged := f.f.f.lookups.Load()
	if _, err := adapter.ObserveBounded(ctx, operator, ref, acceptedPack, "p25-frozen-stale-key", 60); !errors.Is(err, evaluation.ErrPerformanceEvidence) || f.f.f.lookups.Load() != beforeChanged {
		t.Fatal("stale reviewed-pack input reached source before correctness gate", err)
	}
	staleAdapter, err := evaluation.NewFrozenPerformanceReleaseAdapterFactory(f.f.f.db, runs, staleFrozenReader{inner: f.f.f.db, key: cold.ReuseKey}, "recorded", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := staleAdapter.ObserveBounded(ctx, operator, ref, acceptedPackB, "p25-frozen-stale-substitution", 60); !errors.Is(err, evaluation.ErrPerformanceEvidence) || f.f.f.lookups.Load() != beforeChanged {
		t.Fatal("stale repository reuse key reached source before correctness gate", err)
	}
	staleRequest := request
	staleRequest.Key = "p25-phase24-stale-pack"
	staleRef, err := evaluationService.RegisterInput(ctx, operator, "protected", evaluation.LiveInput{Pack: pack, Frozen: &evaluation.FrozenRunInput{BlockID: block, Request: staleRequest}})
	if err != nil {
		t.Fatal(err)
	}
	staleCase := caseProof
	staleCase.Input = staleRef
	_, err = phase24.Observe(ctx, evaluation.Execution{Case: staleCase, Pack: pack, RuntimeConfig: cfg, RuntimeDigest: acceptedPack.Digest,
		Envelope: operator, Reservation: evaluation.Reservation{Calls: 1, Tokens: 8192, Deadline: time.Now().Add(time.Minute)}})
	if !errors.Is(err, evaluation.ErrReview) || f.f.f.lookups.Load() != beforeChanged {
		t.Fatal("Phase 24 frozen report could claim a different selected pack", err)
	}
	changedReport, err := evaluationService.Run(ctx, operator, evaluation.RunRequest{RunID: "p25-frozen-changed-report", SuiteID: suite.ID, SuiteRevision: suite.Revision, SuiteDigest: draft.Digest, PackDigest: packB.Digest}, phase24)
	if err != nil || !changedReport.GatePassed || changedReport.Status != "passed" || changedReport.Pack.Digest != packB.Digest || changedReport.EvidenceHash == report.EvidenceHash || changedReport.Cases[0].Observation.SemanticDigest != cold.SemanticDigest || changedReport.Cases[0].Observation.Usage.SourceCalls != 1 || changedReport.Cases[0].Observation.Usage.Calls != 1 || changedReport.Cases[0].Observation.Usage.SourceMS == nil {
		t.Fatal("changed reviewed pack did not produce a distinct physical Phase 24 report", err, changedReport)
	}
	changedProof, err := evaluationService.ResolvePerformanceEvidence(ctx, operator, evaluation.PerformanceEnvironment{EvaluationSuiteID: suite.ID, EvaluationSuiteRevision: suite.Revision, EvaluationSuite: draft.Digest, EvaluationReportID: changedReport.RunID, EvaluationReport: changedReport.EvidenceHash, WorkloadCaseID: caseProof.ID})
	if err != nil || changedProof.RuntimePack.Digest != acceptedPackB.Digest || changedProof.Report.EvidenceHash != changedReport.EvidenceHash {
		t.Fatal("changed-pack release evidence did not bind exact accepted Phase 24 report", err)
	}
	changed, err := adapter.ObserveBounded(ctx, operator, ref, acceptedPackB, "p25-frozen-changed-pack", 0)
	if err != nil || changed.ReuseKey == cold.ReuseKey || changed.ReusedFrom != "" || changed.Usage.SourceCalls != 1 || changed.Usage.SourceNS == nil || changed.Usage.ModelCalls != 1 || changed.SemanticDigest != cold.SemanticDigest {
		t.Fatal("selected reviewed-pack change did not invalidate product reuse", err, changed)
	}
	deniedScopes := slices.DeleteFunc(append([]string(nil), scopes...), func(s string) bool { return s == "reporting.execute" })
	denied := phase27Actor(t, f.f, operator.User(), deniedScopes)
	before := f.f.f.lookups.Load()
	_, err = adapter.ObserveBounded(ctx, denied, ref, acceptedPackB, "p25-frozen-denied", 60)
	if err == nil || f.f.f.lookups.Load() != before {
		t.Fatal("missing signed action reached source", err)
	}
	selected, err = evaluationService.SelectPack(ctx, operator, "reviewed-narrative-"+pack.ID, selected.Revision)
	if err != nil || selected.Revision != 5 || selected.PackDigest != pack.Digest || selected.Actor != operator.User() {
		t.Fatal("authorized release rollback did not restore the reviewed base pack", err, selected)
	}
}
