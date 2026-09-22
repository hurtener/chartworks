package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

const testDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func ptr[T any](v T) *T { return &v }
func testRuntimeConfig(model string) gateway.RuntimeConfig {
	cfg := gateway.RuntimeConfig{Model: model, SystemInstruction: "reviewed instruction", AttemptCostUSD: 0.01}
	cfg.Digest = gateway.ConfigurationDigest(cfg)
	return cfg
}
func testSuite() Suite {
	q := 1.0
	obs := Observation{Decision: "route", SemanticDigest: testDigest, Usage: Usage{SourceMS: ptr(int64(2)), ModelMS: ptr(int64(3)), Tokens: ptr(5), CostUSD: ptr(0.01), Calls: 1, ServiceMS: 6}}
	critical := Observation{Decision: "blocked", SemanticDigest: testDigest, ErrorClass: "unsafe", Blocked: true, Usage: Usage{ServiceMS: 1, SourceMS: ptr(int64(0)), ModelMS: ptr(int64(0)), Tokens: ptr(0), CostUSD: ptr(0.0)}}
	baseCfg, candCfg := testRuntimeConfig("model-v1"), testRuntimeConfig("model-v2")
	packs := []PackRevision{{ID: "baseline", Revision: 1, Model: "model-v1", ConfigurationDigest: baseCfg.Digest}, {ID: "candidate", Revision: 1, Model: "model-v2", ConfigurationDigest: candCfg.Digest}}
	for i := range packs {
		packs[i].Digest = packs[i].CanonicalDigest()
	}
	return Suite{SchemaVersion: 1, ID: "suite", Revision: 1, Mode: Fixture, Seed: 42, Calibration: "reviewed", Threshold: Threshold{QualityMin: &q}, Limits: Limits{Cases: 10, Calls: 10, Tokens: 100, Retries: 2, DurationMS: 10000}, Provenance: Provenance{Implementation: "sha", EnvironmentDigest: testDigest, ConfigurationDigest: testDigest, SemanticVersion: "semantic-v1", RuleVersion: "rule-v1", SourceSnapshot: testDigest, DialectMatrix: []DialectEvidence{{Engine: "postgres", Dialect: "postgres", Mode: Fixture, EvidenceDigest: testDigest, Status: "measured"}}}, Packs: packs, Frontiers: []string{"EVAL-01", "EXP-01", "EXP-03", "EXP-05", "EXP-09", "EXP-10", "EXP-11"}, Cases: []Case{{ID: "quality", Stage: StageRouting, Locale: "en", HeldOut: true, Input: ProtectedRef{testDigest, "protected"}, Expected: []Expected{{"route", testDigest, ""}}, Fixture: &obs}, {ID: "security", Stage: StageAdversarial, Category: "injection", Locale: "es", Critical: true, Input: ProtectedRef{testDigest, "protected"}, Expected: []Expected{{"blocked", testDigest, "unsafe"}}, Fixture: &critical}}}
}

func optimizationSuite() Suite {
	s := testSuite()
	s.Mode = Live
	s.HeldoutLineageDigest = strings.Repeat("d", 64)
	q := 0.5
	s.Threshold.QualityMin = &q
	second := s.Cases[0]
	second.ID = "quality-control"
	second.Input.Digest = strings.Repeat("e", 64)
	s.Cases = append([]Case{s.Cases[0], second}, s.Cases[1])
	for i := range s.Cases {
		s.Cases[i].Fixture = nil
	}
	s.Provenance.DialectMatrix[0].Mode = Live
	return s
}

func TestEvaluateGateAndReproducibility(t *testing.T) {
	s := testSuite()
	now := time.Unix(100, 0)
	clock := func() time.Time { return now }
	a, err := Evaluate(context.Background(), "run-a", s, nil, clock)
	if err != nil || !a.GatePassed || a.QualityPassed != 1 || a.SecurityFailures != 0 {
		t.Fatal(a, err)
	}
	b, err := Evaluate(context.Background(), "run-b", s, nil, clock)
	if err != nil || a.EvidenceHash != b.EvidenceHash {
		t.Fatal("non-reproducible evidence", err)
	}
	if a.Usage.SourceMS == nil || a.Usage.ModelMS == nil || a.Usage.CostUSD == nil {
		t.Fatal("measurement separation lost")
	}
	if a.Validate() != nil {
		t.Fatal(a.Validate())
	}
	tampered := a
	tampered.Usage.Calls++
	if !errors.Is(tampered.Validate(), ErrInvalid) {
		t.Fatal("tampered aggregate accepted")
	}
	if got, err := s.Digest(); err != nil || got != a.SuiteDigest {
		t.Fatal(got, err)
	}
}

func TestGateFailsSeededRegressionAndCritical(t *testing.T) {
	s := testSuite()
	s.Cases[0].Fixture.SemanticDigest = strings.Repeat("b", 64)
	r, err := Evaluate(context.Background(), "run", s, nil, func() time.Time { return time.Unix(1, 0) })
	if !errors.Is(err, ErrGate) || r.GatePassed || r.Cases[0].Reason != "semantic_drift" {
		t.Fatal(r, err)
	}
	s = testSuite()
	s.Cases[1].Fixture.Blocked = false
	r, err = Evaluate(context.Background(), "run", s, nil, func() time.Time { return time.Unix(1, 0) })
	if !errors.Is(err, ErrGate) || r.SecurityFailures != 1 {
		t.Fatal(r, err)
	}
}

func TestValidationModesAndBudgets(t *testing.T) {
	s := testSuite()
	if s.Validate() != nil {
		t.Fatal(s.Validate())
	}
	bad := s
	bad.Threshold.QualityMin = ptr(0.8)
	bad = s
	bad.Mode = Live
	if !errors.Is(bad.Validate(), ErrInvalid) {
		t.Fatal("live fixture accepted")
	}
	for i := range bad.Cases {
		bad.Cases[i].Fixture = nil
	}
	for i := range bad.Provenance.DialectMatrix {
		bad.Provenance.DialectMatrix[i].Mode = Live
	}
	if bad.Validate() != nil {
		t.Fatal(bad.Validate())
	}
	if _, err := Evaluate(context.Background(), "run", bad, nil, nil); !errors.Is(err, ErrMode) {
		t.Fatal(err)
	}
	runner := RunnerFunc(func(context.Context, Execution) (Observation, error) {
		return Observation{Decision: "route", SemanticDigest: testDigest, Usage: Usage{Calls: 11}}, nil
	})
	if _, err := Evaluate(context.Background(), "run", bad, runner, nil); !errors.Is(err, ErrBudget) {
		t.Fatal(err)
	}
}

func TestOptimizationNeedsHeldOutImprovementAndReview(t *testing.T) {
	s := optimizationSuite()
	run := RunnerFunc(func(_ context.Context, x Execution) (Observation, error) {
		if x.Case.Critical {
			return Observation{Decision: "blocked", SemanticDigest: testDigest, ErrorClass: "unsafe", Blocked: true}, nil
		}
		d := testDigest
		if x.Pack.ID == "baseline" && x.Case.ID == "quality" {
			d = strings.Repeat("b", 64)
		}
		return Observation{Decision: "route", SemanticDigest: d, Usage: Usage{Calls: 1}}, nil
	})
	cand, _ := EvaluateWithPack(context.Background(), "candidate", s, s.Packs[1], run, func() time.Time { return time.Unix(1, 0) })
	base, _ := EvaluateWithPack(context.Background(), "baseline", s, s.Packs[0], run, func() time.Time { return time.Unix(1, 0) })
	p, err := ProposeOptimization("proposal", s, base, cand, time.Unix(2, 0))
	if err != nil || p.State != "candidate" {
		t.Fatal(p, err)
	}
	if _, err = proposalReceipt(p, "reviewer", "approve", time.Unix(3, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err = proposalReceipt(p, "reviewer", "publish", time.Now()); !errors.Is(err, ErrReview) {
		t.Fatal("automatic promotion possible")
	}
	fixture := testSuite()
	if _, err = ProposeOptimization("fixture", fixture, base, cand, time.Now()); !errors.Is(err, ErrInvalid) {
		t.Fatal("fixture evidence admitted for optimization", err)
	}
	failed := base
	failed.Status, failed.GatePassed, failed.FailureClass = "failed", false, "quality_threshold"
	evidence := struct {
		Suite   string
		Pack    PackRevision
		Seed    int64
		Cases   []CaseResult
		Mode    Mode
		Status  string
		Failure string
	}{failed.SuiteDigest, failed.Pack, failed.Seed, failed.Cases, failed.Mode, failed.Status, failed.FailureClass}
	failed.EvidenceHash, _ = digest(evidence)
	if _, err = ProposeOptimization("failed", s, failed, cand, time.Now()); !errors.Is(err, ErrGate) {
		t.Fatal("failing report admitted for optimization", err)
	}
}

func TestCommandStrictFixtureOnly(t *testing.T) {
	s := testSuite()
	raw, _ := json.Marshal(s)
	path := filepath.Join(t.TempDir(), "suite.json")
	if os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("write")
	}
	var out, stderr bytes.Buffer
	if code := Command(context.Background(), []string{"gate", "--suite", path}, &out, &stderr); code != 0 || !strings.Contains(out.String(), `"gate_passed":true`) {
		t.Fatal(code, out.String(), stderr.String())
	}
	s.Mode = Live
	for i := range s.Cases {
		s.Cases[i].Fixture = nil
	}
	for i := range s.Provenance.DialectMatrix {
		s.Provenance.DialectMatrix[i].Mode = Live
	}
	raw, _ = json.Marshal(s)
	_ = os.WriteFile(path, raw, 0600)
	out.Reset()
	stderr.Reset()
	if code := Command(context.Background(), []string{"gate", "--suite", path}, &out, &stderr); code != 2 || !strings.Contains(stderr.String(), "authority-bound") {
		t.Fatal(code, stderr.String())
	}
}

type testRepo struct {
	reports   map[string]Report
	suites    map[string]SuiteRecord
	exports   map[string]CandidateExport
	proposals map[string]OptimizationProposal
	selection int64
	selected  PackSelection
}

func (r *testRepo) CreateSuite(_ context.Context, _ store.Scope, v SuiteRecord) error {
	r.suites[v.Digest] = v
	return nil
}
func (*testRepo) SaveInput(context.Context, store.Scope, ProtectedRef, LiveInput) error { return nil }
func (r *testRepo) ReviewSuite(_ context.Context, _ store.Scope, v SuiteReview) (SuiteRecord, error) {
	x := r.suites[v.Digest]
	if x.Author == v.Reviewer {
		return x, store.ErrConflict
	}
	x.State = v.Decision
	x.Review = &v
	r.suites[v.Digest] = x
	return x, nil
}
func (r *testRepo) AcceptedSuite(_ context.Context, _ store.Scope, _ string, _ int64, d string) (SuiteRecord, error) {
	x, ok := r.suites[d]
	if !ok || x.State != Accepted {
		return x, store.ErrNotFound
	}
	return x, nil
}
func (r *testRepo) BeginRun(context.Context, store.Scope, RunRequest) error { return nil }
func (r *testRepo) SaveReport(_ context.Context, _ store.Scope, v Report) error {
	r.reports[v.RunID] = v
	return nil
}
func (r *testRepo) RequestCancel(context.Context, store.Scope, string) error { return nil }
func (r *testRepo) RecoverRun(context.Context, store.Scope, string, time.Time) (Report, error) {
	return Report{}, store.ErrNotFound
}
func (r *testRepo) SaveFeedbackExport(_ context.Context, _ store.Scope, x CandidateExport) error {
	r.exports[x.ID] = x
	return nil
}
func (r *testRepo) ReadFeedbackExport(_ context.Context, _ store.Scope, id string) (CandidateExport, error) {
	x, ok := r.exports[id]
	if !ok {
		return x, store.ErrNotFound
	}
	return x, nil
}
func (r *testRepo) ReviewFeedbackSplit(_ context.Context, _ store.Scope, _ string, _ string, x FeedbackSplit) error {
	r.exports[x.Training.ID] = x.Training
	r.exports[x.Heldout.ID] = x.Heldout
	return nil
}
func (*testRepo) ValidateHeldoutCases(context.Context, store.Scope, []Case) error       { return nil }
func (*testRepo) ValidateOptimizationHeldout(context.Context, store.Scope, Suite) error { return nil }
func (r *testRepo) SaveProposal(_ context.Context, _ store.Scope, p OptimizationProposal) error {
	r.proposals[p.ID] = p
	return nil
}
func (r *testRepo) ReadProposal(_ context.Context, _ store.Scope, id string) (OptimizationProposal, error) {
	p, ok := r.proposals[id]
	if !ok {
		return p, store.ErrNotFound
	}
	return p, nil
}
func (r *testRepo) ReviewProposal(context.Context, store.Scope, ReviewReceipt) error { return nil }
func (r *testRepo) SelectPack(_ context.Context, _ store.Scope, x PackSelection, expected int64) (PackSelection, error) {
	if expected != r.selection {
		return x, store.ErrConflict
	}
	x.Revision = expected + 1
	r.selection = x.Revision
	r.selected = x
	return x, nil
}
func (r *testRepo) SelectedPack(context.Context, store.Scope) (PackSelection, error) {
	if r.selected.PackDigest == "" {
		return PackSelection{}, store.ErrNotFound
	}
	return r.selected, nil
}
func (r *testRepo) ReadReport(_ context.Context, _ store.Scope, id string) (Report, error) {
	v, ok := r.reports[id]
	if !ok {
		return v, store.ErrNotFound
	}
	return v, nil
}

type testFeedback []FeedbackEvidence

func (f testFeedback) ReviewedFeedback(context.Context, identity.Envelope, string, int) ([]FeedbackEvidence, error) {
	return append([]FeedbackEvidence(nil), f...), nil
}

func testAuthority(t *testing.T, actor string, export bool) identity.Envelope {
	t.Helper()
	scopes := []string{"ops.write", "ops.read", "cw.tenant.write:tenant", "cw.tenant.read:tenant"}
	if export {
		scopes = append(scopes, "cw.tenant.export:tenant")
	}
	if actor == "reviewer" {
		scopes = append(scopes, "ops.audit", "cw.tenant.certify:tenant")
	}
	e, err := identity.FromVerified("tenant", actor, "session", scopes, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestServicePersistenceAndProtectedFeedback(t *testing.T) {
	repo := &testRepo{reports: map[string]Report{}, suites: map[string]SuiteRecord{}, exports: map[string]CandidateExport{}, proposals: map[string]OptimizationProposal{}}
	svc, err := New(repo, testFeedback{{ID: "training-case", Locale: "en", InputDigest: testDigest, ExpectedDigest: testDigest, Decision: "route", SourceBindingDigest: testDigest}, {ID: "heldout-case", Locale: "es", InputDigest: strings.Repeat("b", 64), ExpectedDigest: testDigest, Decision: "route", SourceBindingDigest: testDigest}}, func() time.Time { return time.Unix(5, 0) })
	if err != nil {
		t.Fatal(err)
	}
	draft, err := svc.Author(context.Background(), testAuthority(t, "actor", true), testSuite())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Review(context.Background(), testAuthority(t, "reviewer", true), draft.Suite.ID, SuiteReviewRequest{Revision: draft.Suite.Revision, Digest: draft.Digest, Decision: Accepted}); err != nil {
		t.Fatal(err)
	}
	r, err := svc.Run(context.Background(), testAuthority(t, "actor", true), RunRequest{RunID: "run", SuiteID: draft.Suite.ID, SuiteRevision: draft.Suite.Revision, SuiteDigest: draft.Digest, PackDigest: draft.Suite.Packs[0].Digest}, nil)
	if err != nil || repo.reports["run"].EvidenceHash != r.EvidenceHash {
		t.Fatal(r, err)
	}
	if got, err := svc.Read(context.Background(), testAuthority(t, "actor", true), "run"); err != nil || got.RunID != "run" {
		t.Fatal(got, err)
	}
	out, err := svc.ExportFeedback(context.Background(), testAuthority(t, "actor", true), "export", "topic", 10)
	if err != nil || out.Status != "pending" || out.Split != "candidate" || len(out.Cases) != 2 || out.Cases[0].HeldOut {
		t.Fatal(out, err)
	}
	if _, err = svc.ReviewFeedbackSplit(context.Background(), testAuthority(t, "actor", true), out.ID, out.EvidenceHash, "training-self", "heldout-self", []string{"heldout-case"}); err == nil {
		t.Fatal("author reviewed own split")
	}
	split, err := svc.ReviewFeedbackSplit(context.Background(), testAuthority(t, "reviewer", true), out.ID, out.EvidenceHash, "training", "heldout", []string{"heldout-case"})
	if err != nil || split.Heldout.Split != "heldout" || split.Heldout.ParentDigest != out.EvidenceHash || !split.Heldout.Cases[0].HeldOut || len(split.Training.Cases) != 1 || split.Training.Cases[0].HeldOut || repo.exports[out.ID].Cases[0].HeldOut {
		t.Fatal(split, err)
	}
	if _, err = svc.ExportFeedback(context.Background(), testAuthority(t, "actor", false), "export2", "topic", 10); err == nil {
		t.Fatal("unscoped export")
	}
	if _, err = New(nil, nil, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestAcceptedLifecycleAndEarlyBudgetEvidence(t *testing.T) {
	repo := &testRepo{reports: map[string]Report{}, suites: map[string]SuiteRecord{}, exports: map[string]CandidateExport{}, proposals: map[string]OptimizationProposal{}}
	svc, _ := New(repo, nil, func() time.Time { return time.Unix(9, 0) })
	suite := testSuite()
	suite.Mode = Live
	for i := range suite.Cases {
		suite.Cases[i].Fixture = nil
	}
	suite.Provenance.DialectMatrix[0].Mode = Live
	suite.Limits.Calls = 1
	draft, err := svc.Author(context.Background(), testAuthority(t, "actor", false), suite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Review(context.Background(), testAuthority(t, "actor", false), suite.ID, SuiteReviewRequest{Revision: 1, Digest: draft.Digest, Decision: Accepted}); err == nil {
		t.Fatal("author self-review accepted")
	}
	if _, err = svc.Review(context.Background(), testAuthority(t, "reviewer", false), suite.ID, SuiteReviewRequest{Revision: 1, Digest: draft.Digest, Decision: Accepted}); err != nil {
		t.Fatal(err)
	}
	r, err := svc.Run(context.Background(), testAuthority(t, "actor", false), RunRequest{RunID: "bounded", SuiteID: suite.ID, SuiteRevision: 1, SuiteDigest: draft.Digest, PackDigest: draft.Suite.Packs[0].Digest}, RunnerFunc(func(context.Context, Execution) (Observation, error) {
		t.Fatal("delegated without reservation")
		return Observation{}, nil
	}))
	if !errors.Is(err, ErrBudget) || r.Status != "budget_exhausted" || r.EvidenceHash == "" || repo.reports["bounded"].EvidenceHash != r.EvidenceHash {
		t.Fatal(r, err)
	}
}

func TestOptimizationServiceAndRecoverySurfaces(t *testing.T) {
	repo := &testRepo{reports: map[string]Report{}, suites: map[string]SuiteRecord{}, exports: map[string]CandidateExport{}, proposals: map[string]OptimizationProposal{}}
	svc, _ := New(repo, nil, func() time.Time { return time.Unix(11, 0) })
	s := optimizationSuite()
	run := RunnerFunc(func(_ context.Context, x Execution) (Observation, error) {
		if x.Case.Critical {
			return Observation{Decision: "blocked", SemanticDigest: testDigest, ErrorClass: "unsafe", Blocked: true}, nil
		}
		d := testDigest
		if x.Pack.ID == "baseline" && x.Case.ID == "quality" {
			d = strings.Repeat("b", 64)
		}
		return Observation{Decision: "route", SemanticDigest: d, Usage: Usage{Calls: 1}}, nil
	})
	base, _ := EvaluateWithPack(context.Background(), "base", s, s.Packs[0], run, func() time.Time { return time.Unix(1, 0) })
	candidate, _ := EvaluateWithPack(context.Background(), "candidate", s, s.Packs[1], run, func() time.Time { return time.Unix(1, 0) })
	dig, _ := s.Digest()
	review := SuiteReview{SuiteID: s.ID, Revision: s.Revision, Digest: dig, Decision: Accepted, Reviewer: "reviewer", ReviewedAt: time.Unix(2, 0)}
	repo.suites[dig] = SuiteRecord{Suite: s, Digest: dig, State: Accepted, Author: "author", Review: &review}
	repo.reports[base.RunID] = base
	repo.reports[candidate.RunID] = candidate
	p, err := svc.ProposeOptimization(context.Background(), testAuthority(t, "actor", false), ProposalRequest{ID: "proposal", SuiteID: s.ID, SuiteRevision: s.Revision, SuiteDigest: dig, BaselineRun: "base", CandidateRun: "candidate"})
	if err != nil {
		t.Fatal(err)
	}
	pd, _ := digest(p)
	if _, err = svc.ReviewOptimization(context.Background(), testAuthority(t, "reviewer", false), p.ID, ProposalReviewRequest{Digest: pd, Decision: "approve"}); err != nil {
		t.Fatal(err)
	}
	if selected, err := svc.SelectPack(context.Background(), testAuthority(t, "actor", false), p.ID, 0); err != nil || selected.Revision != 1 {
		t.Fatal(selected, err)
	}
	if _, err := svc.SelectPack(context.Background(), testAuthority(t, "actor", false), p.ID, 0); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale pack CAS accepted", err)
	}
	if err = svc.Cancel(context.Background(), testAuthority(t, "actor", false), "run"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Recover(context.Background(), testAuthority(t, "actor", false), "run"); err == nil {
		t.Fatal("missing recovery accepted")
	}
}

func TestGatewayReservationRejectsBeforeProviderAttempt(t *testing.T) {
	e := testAuthority(t, "actor", false)
	call, err := gateway.Authorize(e, "ops.write", "evaluation", access.Tenant(e, "write"))
	if err != nil {
		t.Fatal(err)
	}
	budget, err := gateway.NewBudget(call, gateway.Limits{Calls: 4, Tokens: 100, Duration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	r := &gatewayReservation{limit: Reservation{Calls: 2, Tokens: 12, Retries: 1, Deadline: time.Now().Add(time.Minute)}}
	ctx, err := gateway.WithAttemptReservation(context.Background(), r.reserve)
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	invoke := func(tokens int) error {
		if err := gateway.ReserveAttempt(ctx, budget, call, tokens); err != nil {
			return err
		}
		providerCalls++
		return nil
	}
	if err := invoke(5); err != nil {
		t.Fatal("reserved attempt rejected", err)
	}
	if err := invoke(5); err != nil {
		t.Fatal("reserved attempts rejected")
	}
	if err := invoke(1); !errors.Is(err, gateway.ErrBudget) || providerCalls != 2 {
		t.Fatal("over-consumption reached provider", providerCalls, err)
	}
	u := r.usage()
	if u.Calls != 2 || u.Retries != 1 || u.Tokens == nil || *u.Tokens != 10 {
		t.Fatal(u)
	}
}

func TestDollarReservationAndUnknownCostFailClosed(t *testing.T) {
	e := testAuthority(t, "actor", false)
	call, err := gateway.Authorize(e, "ops.write", "evaluation", access.Tenant(e, "write"))
	if err != nil {
		t.Fatal(err)
	}
	cap := 0.015
	r := &gatewayReservation{limit: Reservation{Calls: 3, Tokens: 100, Retries: 2, CostUSD: &cap, Deadline: time.Now().Add(time.Minute)}}
	ctx, err := gateway.WithAttemptReservation(context.Background(), r.reserve)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err = gateway.WithRuntimeConfig(ctx, testRuntimeConfig("model-v1"))
	if err != nil {
		t.Fatal(err)
	}
	budget, _ := gateway.NewBudget(call, gateway.Limits{Calls: 3, Tokens: 100, Duration: time.Minute})
	providerCalls := 0
	invoke := func() error {
		if err := gateway.ReserveAttempt(ctx, budget, call, 1); err != nil {
			return err
		}
		providerCalls++
		return nil
	}
	if err := invoke(); err != nil {
		t.Fatal(err)
	}
	if err := invoke(); !errors.Is(err, gateway.ErrBudget) || providerCalls != 1 {
		t.Fatal(providerCalls, err)
	}

	s := testSuite()
	s.Mode = Live
	s.Limits.CostUSD = &cap
	for i := range s.Cases {
		s.Cases[i].Fixture = nil
	}
	s.Provenance.DialectMatrix[0].Mode = Live
	report, err := EvaluateWithPack(context.Background(), "unknown-cost", s, s.Packs[0], RunnerFunc(func(context.Context, Execution) (Observation, error) {
		return Observation{Decision: "route", SemanticDigest: testDigest, Usage: Usage{Calls: 1}}, nil
	}), func() time.Time { return time.Unix(1, 0) })
	if !errors.Is(err, ErrBudget) || report.Status != "dependency_failed" || report.FailureClass != "unknown_cost" || report.EvidenceHash == "" {
		t.Fatal(report, err)
	}
	s.Limits.CostUSD = nil
	report, err = EvaluateWithPack(context.Background(), "provider-reservation", s, s.Packs[0], RunnerFunc(func(context.Context, Execution) (Observation, error) {
		return Observation{}, gateway.ErrBudget
	}), func() time.Time { return time.Unix(1, 0) })
	if !errors.Is(err, ErrBudget) || report.Status != "budget_exhausted" || report.FailureClass != "reservation_exceeded" || report.EvidenceHash == "" {
		t.Fatal(report, err)
	}
}

func TestSelectedPackChangesDefaultRun(t *testing.T) {
	s := testSuite()
	repo := &testRepo{reports: map[string]Report{}, suites: map[string]SuiteRecord{}, exports: map[string]CandidateExport{}, proposals: map[string]OptimizationProposal{}, selected: PackSelection{Revision: 2, PackDigest: s.Packs[1].Digest, ProposalID: "approved"}}
	svc, _ := New(repo, nil, func() time.Time { return time.Unix(10, 0) })
	d, _ := s.Digest()
	review := SuiteReview{SuiteID: s.ID, Revision: s.Revision, Digest: d, Decision: Accepted, Reviewer: "reviewer", ReviewedAt: time.Unix(9, 0)}
	repo.suites[d] = SuiteRecord{Suite: s, Digest: d, State: Accepted, Author: "author", Review: &review}
	r, err := svc.Run(context.Background(), testAuthority(t, "actor", false), RunRequest{RunID: "selected-default", SuiteID: s.ID, SuiteRevision: s.Revision, SuiteDigest: d}, nil)
	if err != nil || r.Pack.Digest != repo.selected.PackDigest {
		t.Fatal(r.Pack, err)
	}
}

type staticInputResolver struct{ in LiveInput }

func (r staticInputResolver) ResolveEvaluationInput(context.Context, identity.Envelope, ProtectedRef) (LiveInput, error) {
	return r.in, nil
}

type savedInspector struct{ calls int }

func (s *savedInspector) InspectSaved(context.Context, identity.Envelope, nlqexec.SavedQuestion) (nlqexec.SavedEvidence, error) {
	s.calls++
	return nlqexec.SavedEvidence{Source: "source", Context: "context", SemanticDigest: testDigest, QueryDigest: testDigest}, nil
}

type adversarialQuery struct{ calls int }

func (q *adversarialQuery) Preflight(context.Context, identity.Envelope, nlqexec.PreflightRequest) (nlqexec.PreflightResult, error) {
	return nlqexec.PreflightResult{}, nlqexec.ErrInvalid
}
func (q *adversarialQuery) Plan(context.Context, identity.Envelope, nlqexec.PlanRequest) (nlqexec.PlanResult, error) {
	q.calls++
	return nlqexec.PlanResult{}, nlqexec.ErrInvalid
}
func (q *adversarialQuery) Run(context.Context, identity.Envelope, nlqexec.RunRequest) (nlqexec.RunResult, error) {
	return nlqexec.RunResult{}, nlqexec.ErrInvalid
}
func (q *adversarialQuery) InspectSaved(context.Context, identity.Envelope, nlqexec.SavedQuestion) (nlqexec.SavedEvidence, error) {
	return nlqexec.SavedEvidence{}, nlqexec.ErrInvalid
}

type adversarialReports struct{ calls int }

func (r *adversarialReports) Read(_ context.Context, _ identity.Envelope, id string, _ reporting.Reference) (reporting.View, error) {
	r.calls++
	if id == "frozen" {
		return reporting.View{}, nil
	}
	return reporting.View{}, access.ErrForbidden
}

type adversarialBYO struct{ calls int }

func (b *adversarialBYO) Submit(context.Context, identity.Envelope, nlqbyo.SubmitRequest) (nlqbyo.SubmitResult, error) {
	b.calls++
	return nlqbyo.SubmitResult{}, nlqbyo.ErrInvalid
}

func TestGovernedRunnerExercisesSixAdversarialBoundaries(t *testing.T) {
	s := testSuite()
	q, reports, byo := &adversarialQuery{}, &adversarialReports{}, &adversarialBYO{}
	question, submit := &nlqexec.QuestionRequest{}, &nlqbyo.SubmitRequest{}
	in := LiveInput{Pack: s.Packs[0], RuntimeConfig: testRuntimeConfig(s.Packs[0].Model), Question: question, BYO: submit, ReportID: "denied"}
	runner := &GovernedRunner{Inputs: staticInputResolver{in: in}, Query: q, Reports: reports, BYO: byo}
	x := Execution{Suite: s, Pack: s.Packs[0], Reservation: Reservation{Calls: 4, Tokens: 100, Retries: 2, Deadline: time.Now().Add(time.Minute)}, Envelope: testAuthority(t, "actor", false)}
	for _, category := range []string{"identity_scope", "injection", "dialect_escape", "resource_exhaustion", "byo"} {
		x.Case = Case{Stage: StageAdversarial, Category: category, Input: ProtectedRef{Digest: testDigest, Retention: "protected"}}
		o, err := runner.Observe(context.Background(), x)
		if err != nil || !o.Blocked || o.ErrorClass != category {
			t.Fatalf("%s: %#v %v", category, o, err)
		}
	}
	in.ReportID = "frozen"
	runner.Inputs = staticInputResolver{in: in}
	x.Case = Case{Stage: StageAdversarial, Category: "frozen_report", Input: ProtectedRef{Digest: testDigest, Retention: "protected"}}
	if o, err := runner.Observe(context.Background(), x); err != nil || !o.Blocked || o.ErrorClass != "frozen_report" {
		t.Fatal(o, err)
	}
	if q.calls != 3 || reports.calls != 2 || byo.calls != 1 {
		t.Fatal(q.calls, reports.calls, byo.calls)
	}
}
func TestFailureReportAndLiveRunnerBoundaries(t *testing.T) {
	s := testSuite()
	r, err := FailureReport("recovered", s, "dependency_failed", "crash_recovered", time.Unix(1, 0), time.Unix(2, 0))
	if err != nil || r.Validate() != nil {
		t.Fatal(r, err)
	}
	if _, err = FailureReport("", s, "passed", "", time.Time{}, time.Time{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err = (*GovernedRunner)(nil).Observe(context.Background(), Execution{}); !errors.Is(err, ErrMode) {
		t.Fatal(err)
	}
	g := &GovernedRunner{Inputs: staticInputResolver{LiveInput{Pack: s.Packs[0], RuntimeConfig: testRuntimeConfig(s.Packs[0].Model)}}}
	_, err = g.Observe(context.Background(), Execution{Suite: s, Pack: s.Packs[0], Case: s.Cases[0], Envelope: testAuthority(t, "actor", false)})
	if !errors.Is(err, ErrMode) {
		t.Fatal(err)
	}
	u := gatewayUsage(gateway.Receipt{Calls: []gateway.Usage{{Attempts: 2, DurationMS: 3, InputTokens: ptr(2), OutputTokens: ptr(4), CostUSD: ptr(0.1)}}})
	if u.Calls != 2 || u.Retries != 1 || u.Tokens == nil || *u.Tokens != 6 || u.ModelMS == nil || *u.ModelMS != 3 {
		t.Fatal(u)
	}
	unknown := gatewayUsage(gateway.Receipt{Calls: []gateway.Usage{{Attempts: 1}}})
	if unknown.Tokens != nil || unknown.CostUSD != nil {
		t.Fatal(unknown)
	}
	if (OptimizationProposal{}).Validate() == nil {
		t.Fatal("invalid proposal")
	}
	if _, err = (NLQFeedbackSource{}).ReviewedFeedback(context.Background(), testAuthority(t, "actor", false), "topic", 1); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	repo := &testRepo{reports: map[string]Report{}, suites: map[string]SuiteRecord{}, exports: map[string]CandidateExport{}, proposals: map[string]OptimizationProposal{}}
	svc, _ := New(repo, nil, nil)
	ref, err := svc.RegisterInput(context.Background(), testAuthority(t, "actor", false), "protected", LiveInput{Pack: testSuite().Packs[0]})
	if err != nil || !validDigest(ref.Digest) {
		t.Fatal(ref, err)
	}
}

func TestGovernedRunnerUsesRetainedReplayAndShadow(t *testing.T) {
	s := testSuite()
	inspector := &savedInspector{}
	q := nlqexec.SavedQuestion{Durability: "session_bound", Context: "context", Query: "query"}
	g := &GovernedRunner{Inputs: staticInputResolver{LiveInput{Pack: s.Packs[0], RuntimeConfig: testRuntimeConfig(s.Packs[0].Model), Replay: &q, Shadow: &ShadowInput{Baseline: q, Candidate: q}}}, Saved: inspector}
	x := Execution{Suite: s, Pack: s.Packs[0], Reservation: Reservation{Calls: 1, Tokens: 1, Deadline: time.Now().Add(time.Minute)}, Envelope: testAuthority(t, "actor", false)}
	x.Case = Case{Stage: StageReplay, Input: ProtectedRef{Digest: testDigest, Retention: "protected"}}
	if o, err := g.Observe(context.Background(), x); err != nil || !validDigest(o.SemanticDigest) || inspector.calls != 1 {
		t.Fatal(o, inspector.calls, err)
	}
	x.Case.Stage = StageShadow
	if o, err := g.Observe(context.Background(), x); err != nil || !validDigest(o.SemanticDigest) || inspector.calls != 3 {
		t.Fatal(o, inspector.calls, err)
	}
}

func TestPackExportAndAdversarialBoundaryHelpers(t *testing.T) {
	s := testSuite()
	if p, ok := s.PackByDigest(s.Packs[1].Digest); !ok || p.ID != "candidate" {
		t.Fatal(p, ok)
	}
	if _, ok := s.PackByDigest(strings.Repeat("f", 64)); ok {
		t.Fatal("unknown pack accepted")
	}
	now := time.Unix(9, 0)
	x := CandidateExport{SchemaVersion: SchemaVersion, ID: "split", Status: "reviewed", Split: "heldout", Cases: []Case{{ID: "case", HeldOut: true, Input: ProtectedRef{Digest: testDigest}}}, EvidenceHash: testDigest, ParentDigest: strings.Repeat("b", 64), Author: "author", Reviewer: "reviewer", ReviewedAt: &now, CreatedAt: now}
	if err := x.ValidateExport(); err != nil {
		t.Fatal(err)
	}
	x.Cases[0].HeldOut = false
	if !errors.Is(x.ValidateExport(), ErrInvalid) {
		t.Fatal("split relabel accepted")
	}
	for _, err := range []error{access.ErrForbidden, store.ErrNotFound, nlqexec.ErrInvalid, nlqexec.ErrNoPlan, nlqexec.ErrValidationBudget} {
		if !adversarialDenied(err) {
			t.Fatal("denial not classified", err)
		}
	}
	if adversarialDenied(errors.New("transport unavailable")) {
		t.Fatal("dependency failure classified as safety denial")
	}
	if !receiptMatchesPack(gateway.Receipt{}, s.Packs[0]) || receiptMatchesPack(gateway.Receipt{Calls: []gateway.Usage{{Role: "sql_generation", RequestedModel: "other", ConfigurationDigest: s.Packs[0].ConfigurationDigest}}}, s.Packs[0]) || !receiptMatchesPack(gateway.Receipt{Calls: []gateway.Usage{{Role: "sql_generation", RequestedModel: s.Packs[0].Model, ConfigurationDigest: s.Packs[0].ConfigurationDigest}}}, s.Packs[0]) {
		t.Fatal("provider receipt was not bound to requested pack model")
	}
	called := false
	a := authorityRunner{Envelope: testAuthority(t, "actor", false), Next: RunnerFunc(func(_ context.Context, x Execution) (Observation, error) {
		called = x.Envelope.Valid()
		return Observation{Decision: "ok", SemanticDigest: testDigest}, nil
	})}
	if _, err := a.Observe(context.Background(), Execution{}); err != nil || !called {
		t.Fatal(err)
	}
	repo := &testRepo{reports: map[string]Report{}, suites: map[string]SuiteRecord{}, exports: map[string]CandidateExport{}, proposals: map[string]OptimizationProposal{}}
	svc, _ := New(repo, nil, func() time.Time { return now })
	if _, err := svc.RegisterInput(context.Background(), testAuthority(t, "actor", false), "bad/retention", LiveInput{Pack: testSuite().Packs[0]}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := svc.Author(context.Background(), testAuthority(t, "actor", false), Suite{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := svc.Review(context.Background(), testAuthority(t, "reviewer", false), "bad/id", SuiteReviewRequest{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestCommandInspectAndRejectsDuplicateFields(t *testing.T) {
	s := testSuite()
	raw, _ := json.Marshal(s)
	path := filepath.Join(t.TempDir(), "suite.json")
	_ = os.WriteFile(path, raw, 0600)
	var out, stderr bytes.Buffer
	if code := Command(context.Background(), []string{"inspect", "--suite", path}, &out, &stderr); code != 0 || !strings.Contains(out.String(), `"cases":2`) {
		t.Fatal(code, out.String(), stderr.String())
	}
	duplicate := bytes.Replace(raw, []byte(`"schema_version":1`), []byte(`"schema_version":1,"schema_version":1`), 1)
	_ = os.WriteFile(path, duplicate, 0600)
	out.Reset()
	stderr.Reset()
	if code := Command(context.Background(), []string{"inspect", "--suite", path}, &out, &stderr); code != 2 || !strings.Contains(stderr.String(), "invalid") {
		t.Fatal(code, stderr.String())
	}
}
