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

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

const testDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func ptr[T any](v T) *T { return &v }
func testSuite() Suite {
	q := 1.0
	obs := Observation{Decision: "route", SemanticDigest: testDigest, Usage: Usage{SourceMS: ptr(int64(2)), ModelMS: ptr(int64(3)), Tokens: ptr(5), CostUSD: ptr(0.01), Calls: 1, ServiceMS: 6}}
	critical := Observation{Decision: "blocked", SemanticDigest: testDigest, ErrorClass: "unsafe", Blocked: true, Usage: Usage{ServiceMS: 1, SourceMS: ptr(int64(0)), ModelMS: ptr(int64(0)), Tokens: ptr(0), CostUSD: ptr(0.0)}}
	return Suite{SchemaVersion: 1, ID: "suite", Revision: 1, Mode: Fixture, Seed: 42, Calibration: "reviewed", Threshold: Threshold{QualityMin: &q, Reviewed: true}, Limits: Limits{Cases: 10, Calls: 10, Tokens: 100, Retries: 2, DurationMS: 10000}, Provenance: Provenance{Implementation: "sha", EnvironmentDigest: testDigest, ConfigurationDigest: testDigest, SemanticVersion: "semantic-v1", RuleVersion: "rule-v1", SourceSnapshot: testDigest, DialectMatrix: []DialectEvidence{{Engine: "postgres", Dialect: "postgres", Mode: Fixture, EvidenceDigest: testDigest, Status: "measured"}}}, Frontiers: []string{"EVAL-01", "EXP-01", "EXP-03", "EXP-05", "EXP-09", "EXP-10", "EXP-11"}, Cases: []Case{{ID: "quality", Stage: StageRouting, Locale: "en", HeldOut: true, Input: ProtectedRef{testDigest, "protected"}, Expected: []Expected{{"route", testDigest, ""}}, Fixture: &obs}, {ID: "security", Stage: StageAdversarial, Category: "injection", Locale: "es", Critical: true, Input: ProtectedRef{testDigest, "protected"}, Expected: []Expected{{"blocked", testDigest, "unsafe"}}, Fixture: &critical}}}
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
	bad.Threshold.Reviewed = false
	if !errors.Is(bad.Validate(), ErrInvalid) {
		t.Fatal("unreviewed threshold")
	}
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
	runner := RunnerFunc(func(context.Context, Suite, Case) (Observation, error) {
		return Observation{Decision: "route", SemanticDigest: testDigest, Usage: Usage{Calls: 11}}, nil
	})
	if _, err := Evaluate(context.Background(), "run", bad, runner, nil); !errors.Is(err, ErrBudget) {
		t.Fatal(err)
	}
}

func TestOptimizationNeedsHeldOutImprovementAndReview(t *testing.T) {
	s := testSuite()
	base, _ := Evaluate(context.Background(), "base", s, nil, func() time.Time { return time.Unix(1, 0) })
	cand := base
	cand.Cases = append([]CaseResult(nil), base.Cases...)
	base.Cases[0].Passed = false
	base.QualityPassed = 0
	base.EvidenceHash = strings.Repeat("b", 64)
	p, err := ProposeOptimization(s, base, cand, testDigest, strings.Repeat("c", 64), time.Unix(2, 0))
	if err != nil || p.State != "candidate" {
		t.Fatal(p, err)
	}
	if _, err = ReviewPromotion(p, "reviewer", "approve", time.Unix(3, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err = ReviewPromotion(p, "reviewer", "publish", time.Now()); !errors.Is(err, ErrReview) {
		t.Fatal("automatic promotion possible")
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

type testRepo struct{ reports map[string]Report }

func (r *testRepo) SaveSuite(context.Context, store.Scope, Suite, string) error { return nil }
func (r *testRepo) SaveReport(_ context.Context, _ store.Scope, v Report) error {
	r.reports[v.RunID] = v
	return nil
}
func (r *testRepo) ReadReport(_ context.Context, _ store.Scope, id string) (Report, error) {
	v, ok := r.reports[id]
	if !ok {
		return v, store.ErrNotFound
	}
	return v, nil
}

type testFeedback []FeedbackEvidence

func (f testFeedback) ReviewedFeedback(context.Context, store.Scope, int) ([]FeedbackEvidence, error) {
	return append([]FeedbackEvidence(nil), f...), nil
}

func testAuthority(t *testing.T, export bool) identity.Envelope {
	t.Helper()
	scopes := []string{"ops.write", "ops.read", "cw.tenant.write:tenant", "cw.tenant.read:tenant"}
	if export {
		scopes = append(scopes, "cw.tenant.export:tenant")
	}
	e, err := identity.FromVerified("tenant", "actor", "session", scopes, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestServicePersistenceAndProtectedFeedback(t *testing.T) {
	repo := &testRepo{reports: map[string]Report{}}
	svc, err := New(repo, testFeedback{{ID: "feedback", Locale: "en", InputDigest: testDigest, ExpectedDigest: testDigest, Decision: "route", SourceBindingDigest: testDigest}}, func() time.Time { return time.Unix(5, 0) })
	if err != nil {
		t.Fatal(err)
	}
	r, err := svc.Run(context.Background(), testAuthority(t, true), "run", testSuite(), nil)
	if err != nil || repo.reports["run"].EvidenceHash != r.EvidenceHash {
		t.Fatal(r, err)
	}
	if got, err := svc.Read(context.Background(), testAuthority(t, true), "run"); err != nil || got.RunID != "run" {
		t.Fatal(got, err)
	}
	out, err := svc.ExportFeedback(context.Background(), testAuthority(t, true), 10)
	if err != nil || out.Status != "candidate" || len(out.Cases) != 1 || out.Cases[0].Fixture != nil {
		t.Fatal(out, err)
	}
	if _, err = svc.ExportFeedback(context.Background(), testAuthority(t, false), 10); err == nil {
		t.Fatal("unscoped export")
	}
	if _, err = New(nil, nil, nil); !errors.Is(err, ErrInvalid) {
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
