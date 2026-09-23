package evaluation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

type testPackSelection struct {
	mu        sync.Mutex
	current   PackSelection
	proposals map[string]string
}

func (s *testPackSelection) SelectedPack(ctx context.Context, e identity.Envelope) (PackSelection, error) {
	if ctx == nil || ctx.Err() != nil || !e.Valid() || !e.Has("ops.write") {
		return PackSelection{}, ErrPerformanceAuthority
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current, nil
}

func (s *testPackSelection) ProposedPackDigest(ctx context.Context, e identity.Envelope, proposal string) (string, error) {
	if ctx == nil || ctx.Err() != nil || !e.Valid() || !e.Has("ops.write") {
		return "", ErrPerformanceAuthority
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if digest := s.proposals[proposal]; digest != "" {
		return digest, nil
	}
	return "", ErrPerformanceReuseUnproven
}

func (s *testPackSelection) SelectPack(ctx context.Context, e identity.Envelope, proposal string, expected int64) (PackSelection, error) {
	if ctx == nil || ctx.Err() != nil || !e.Valid() || !e.Has("ops.write") {
		return PackSelection{}, ErrPerformanceAuthority
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	digest := s.proposals[proposal]
	if digest == "" || s.current.Revision != expected {
		return PackSelection{}, ErrPerformanceReuseUnproven
	}
	s.current = PackSelection{Revision: expected + 1, PackDigest: digest, ProposalID: proposal, Actor: e.User(), SelectedAt: time.Now().UTC()}
	return s.current, nil
}

func TestFrozenReleasePackTransitionUsesOrderedCASAndRestores(t *testing.T) {
	ctx := t.Context()
	e, err := identity.FromVerified("tenant", "operator", "session", []string{"ops.write"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	base, changed := strings.Repeat("a", 64), strings.Repeat("b", 64)
	initial := PackSelection{Revision: 1, PackDigest: base, ProposalID: "base-proposal", Actor: "reviewer", SelectedAt: time.Now().UTC()}
	selection := &testPackSelection{current: initial, proposals: map[string]string{"base-proposal": base, "changed-proposal": changed}}
	adapter := &frozenReleaseAdapter{
		factory:  &FrozenPerformanceReleaseAdapterFactory{Selection: selection, ChangedPackProposal: "changed-proposal"},
		envelope: e, baseSelection: initial, changedPack: changed, selectionRevision: initial.Revision,
	}
	baseline := PerformanceStep{Kind: "cold", Allowed: true}
	changedStep := PerformanceStep{Kind: "runtime_pack_changed", Allowed: true}
	if err := adapter.PreparePerformanceStep(ctx, baseline); err != nil {
		t.Fatal("baseline selection", err)
	}
	if err := adapter.PreparePerformanceStep(ctx, changedStep); err != nil {
		t.Fatal("changed-pack selection", err)
	}
	if got := selection.current; got.Revision != 2 || got.PackDigest != changed || got.ProposalID != "changed-proposal" || got.Actor != e.User() {
		t.Fatal("changed pack did not advance reviewed selection", got)
	}
	if err := adapter.PreparePerformanceStep(ctx, changedStep); err != nil || selection.current.Revision != 2 {
		t.Fatal("repeat correctness probe changed the selected revision", err)
	}
	if err := adapter.PreparePerformanceStep(ctx, baseline); err != nil {
		t.Fatal("measurement baseline restore", err)
	}
	if err := adapter.PreparePerformanceStep(ctx, changedStep); err != nil {
		t.Fatal("measurement changed-pack selection", err)
	}
	if err := adapter.RestorePerformanceSelection(ctx); err != nil {
		t.Fatal("release cleanup restore", err)
	}
	if got := selection.current; got.Revision != 5 || got.PackDigest != base || got.ProposalID != initial.ProposalID {
		t.Fatal("ordered release transitions did not restore baseline", got)
	}
	if _, err := selection.SelectPack(ctx, e, "changed-proposal", 5); err != nil {
		t.Fatal(err)
	}
	if err := adapter.RestorePerformanceSelection(ctx); !errors.Is(err, ErrPerformanceReuseUnproven) || selection.current.PackDigest != changed {
		t.Fatal("foreign selection revision was overwritten", err)
	}
}

func TestFrozenReleaseFactoryRequiresAcceptedChangedReportAndSelection(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	e, err := identity.FromVerified("tenant", "actor", "session", []string{
		"ops.write", "cw.tenant.write:tenant", "reporting.execute", "cw.report.execute:workload-report",
		"cw.source.query:source-a", "cw.source.query:source-b", "cw.execution_context.use:context-a", "cw.execution_context.use:context-b", "query.plan", "query.execute",
	}, now.Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	repo, manifest, baseRevisions := performanceReleaseFixture(t, e, now)
	service, err := New(repo, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	variants := performanceReleaseRevisionVariants(baseRevisions)
	scenarios := map[string]PerformanceScenarioEvidence{}
	for _, step := range manifest.Steps {
		if !step.Allowed {
			continue
		}
		environment := manifest.Environment
		if step.EvidenceCaseID != "" {
			environment.WorkloadCaseID = step.EvidenceCaseID
		}
		if step.EvidenceReportID != "" {
			environment.EvaluationReportID, environment.EvaluationReport = step.EvidenceReportID, step.EvidenceReport
		}
		evidence, resolveErr := service.ResolvePerformanceEvidence(t.Context(), e, environment)
		if resolveErr != nil {
			t.Fatal("accepted report fixture", step.Kind, resolveErr)
		}
		revisions := variants[evidence.Case.ID]
		revisions.BlockID, revisions.BlockRevision, revisions.BlockDigest, revisions.SourceHead = "block", 1, strings.Repeat("9", 64), 1
		revisions.TopicPins = []reporting.TopicPin{{Topic: "topic", Version: "v1"}}
		scenarios[step.ID] = PerformanceScenarioEvidence{Evidence: evidence, Revisions: revisions}
	}
	base := scenarios[manifest.Steps[0].ID].Evidence.RuntimePack.Pack.Digest
	changedStep, _ := performanceStep(manifest.Steps, "runtime_pack_changed")
	changed := scenarios[changedStep.ID].Evidence.RuntimePack.Pack.Digest
	selected := PackSelection{Revision: 1, PackDigest: base, ProposalID: "base-proposal", Actor: "reviewer", SelectedAt: now}
	selection := &testPackSelection{current: selected, proposals: map[string]string{"base-proposal": base, "changed-proposal": changed}}
	factory := &FrozenPerformanceReleaseAdapterFactory{
		Inputs:    staticInputResolver{in: LiveInput{Frozen: &FrozenRunInput{BlockID: "block", Request: reporting.RunRequest{Key: "template", Narrative: true}}}},
		ModelMode: "recorded", Selection: selection, ChangedPackProposal: "changed-proposal",
	}
	adapter, err := factory.NewPerformanceReleaseAdapter(t.Context(), e, manifest, scenarios)
	if err != nil || adapter == nil {
		t.Fatal("complete accepted changed report did not enter strict adapter", err)
	}
	transition := adapter.(interface {
		PreparePerformanceStep(context.Context, PerformanceStep) error
		RestorePerformanceSelection(context.Context) error
	})
	if err := transition.PreparePerformanceStep(t.Context(), changedStep); err != nil || selection.current.PackDigest != changed {
		t.Fatal("strict adapter did not select the report-bound changed pack", err)
	}
	if err := transition.RestorePerformanceSelection(t.Context()); err != nil || selection.current.PackDigest != base {
		t.Fatal("strict adapter did not restore the base selection", err)
	}
	missing := *factory
	missing.Selection = nil
	if _, err := missing.NewPerformanceReleaseAdapter(t.Context(), e, manifest, scenarios); !errors.Is(err, ErrPerformanceReuseUnproven) {
		t.Fatal("strict adapter accepted a profile without authorized pack selection", err)
	}
	wrong := *factory
	wrong.ChangedPackProposal = "unapproved-proposal"
	if _, err := wrong.NewPerformanceReleaseAdapter(t.Context(), e, manifest, scenarios); !errors.Is(err, ErrPerformanceReuseUnproven) || selection.current.PackDigest != base {
		t.Fatal("unapproved proposal entered the strict adapter", err)
	}
	selection.proposals["wrong-pack-proposal"] = strings.Repeat("c", 64)
	wrong.ChangedPackProposal = "wrong-pack-proposal"
	if _, err := wrong.NewPerformanceReleaseAdapter(t.Context(), e, manifest, scenarios); !errors.Is(err, ErrPerformanceReuseUnproven) || selection.current.PackDigest != base {
		t.Fatal("wrong-pack proposal entered the strict adapter", err)
	}
}

type preparingPerformanceRunner struct {
	PerformanceRunner
	prepared []string
}

func (r *preparingPerformanceRunner) PreparePerformanceStep(_ context.Context, step PerformanceStep) error {
	r.prepared = append(r.prepared, step.ID)
	return nil
}

func TestPerformancePreparesEachCorrectnessAndMeasuredStepSerially(t *testing.T) {
	manifest := performanceTestManifest()
	runner := &preparingPerformanceRunner{PerformanceRunner: newTestSyntheticPerformanceRunner()}
	report, err := MeasurePerformance(t.Context(), manifest, runner, nil)
	if err != nil || !report.CorrectnessPassed || len(runner.prepared) != 2*len(manifest.Steps) {
		t.Fatal("serial step preparation was skipped", err, len(runner.prepared))
	}
	for i, step := range manifest.Steps {
		if runner.prepared[i] != step.ID || runner.prepared[len(manifest.Steps)+i] != step.ID {
			t.Fatal("step preparation changed correctness/measurement order", i, runner.prepared)
		}
	}
}

type failingChangedPackAdapter struct {
	*releaseTestAdapter
	changed      bool
	restoreCalls int
}

func (a *failingChangedPackAdapter) PreparePerformanceStep(_ context.Context, step PerformanceStep) error {
	a.changed = step.Kind == "runtime_pack_changed"
	return nil
}

func (a *failingChangedPackAdapter) Check(ctx context.Context, step PerformanceStep) (PerformanceAdapterResult, error) {
	if a.changed {
		return PerformanceAdapterResult{}, ErrGate
	}
	return a.releaseTestAdapter.Check(ctx, step)
}

func (a *failingChangedPackAdapter) RestorePerformanceSelection(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	a.restoreCalls++
	a.changed = false
	return nil
}

func TestPerformanceReleaseRestoresSelectionAfterChangedPackProbeFailure(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	scopes := []string{"ops.write", "ops.read", "cw.tenant.write:tenant", "cw.tenant.read:tenant", "reporting.execute", "cw.report.execute:workload-report", "cw.source.query:source-a", "cw.source.query:source-b", "cw.execution_context.use:context-a", "cw.execution_context.use:context-b", "query.plan", "query.execute"}
	e, err := identity.FromVerified("tenant", "actor", "session", scopes, now.Add(time.Hour), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	action, err := identity.FromVerified("tenant", "actor", "session", withoutPerformanceScope(scopes, "query.execute"), now.Add(time.Hour), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	repo, manifest, revisions := performanceReleaseFixture(t, e, now)
	service, err := New(repo, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &failingChangedPackAdapter{releaseTestAdapter: &releaseTestAdapter{mode: PerformanceIntegration, cache: map[string]bool{}}}
	runtime := &PerformanceReleaseRuntime{
		Verifier:  &releaseTestVerifier{envelope: e, wantBearer: "current", actionEnvelope: action, actionBearer: "negative"},
		Service:   service,
		Revisions: releaseTestRevisionResolver{evidence: revisions, byCase: performanceReleaseRevisionVariants(revisions)},
		Adapters:  &releaseTestAdapterFactory{adapter: adapter},
	}
	report, err := runtime.Measure(context.Background(), "current", "negative", manifest)
	if !errors.Is(err, ErrGate) || report.CorrectnessPassed || len(report.Samples) != 0 || adapter.restoreCalls != 1 || adapter.changed || adapter.runCalls != 0 {
		t.Fatal("failed changed-pack correctness did not restore selection before timing", err, report.CorrectnessPassed, adapter.restoreCalls, adapter.changed, adapter.runCalls)
	}
}

func TestPerformanceLineageRejectsStaleAndInconsistentReuse(t *testing.T) {
	env := PerformanceEnvironment{SourceMode: "real_postgres", ModelMode: "recorded"}
	key := strings.Repeat("a", 64)
	physical := PerformanceObservation{Executed: true}
	reused := PerformanceObservation{Reused: true}
	if err := validatePerformanceLineage(env, physical, "run-a", key, ""); err != nil {
		t.Fatal(err)
	}
	if err := validatePerformanceLineage(env, reused, "run-b", key, "run-a"); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []struct {
		outcome            PerformanceObservation
		runID, key, origin string
	}{
		{physical, "run-b", key, "run-a"},
		{reused, "run-b", key, ""},
		{reused, "run-b", strings.Repeat("b", 63), "run-a"},
		{reused, "run-b", key, "run-b"},
	} {
		if err := validatePerformanceLineage(env, stale.outcome, stale.runID, stale.key, stale.origin); err == nil {
			t.Fatal("inconsistent product reuse lineage passed", stale)
		}
	}
}
