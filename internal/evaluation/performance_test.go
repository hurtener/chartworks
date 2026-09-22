package evaluation

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
)

func performanceTestManifest() PerformanceManifest {
	a := strings.Repeat("a", 64)
	base := PerformanceBinding{TenantDigest: a, ContextDigest: a, ActionsDigest: a, SourceRevision: a, RuleRevision: a, TopicRevision: a, RuntimePackDigest: a}
	digest := syntheticWorkloadDigest("bounded synthetic query")
	step := func(id, kind string, binding PerformanceBinding, allowed bool, reset bool, iterations, concurrency, executions, blocks int) PerformanceStep {
		return PerformanceStep{ID: id, Kind: kind, Binding: binding, Allowed: allowed, ResetBefore: reset, Iterations: iterations, Concurrency: concurrency, Workload: "bounded synthetic query", ExpectedDigest: digest, ExpectedExecutions: executions, ExpectedBlocks: blocks}
	}
	source, rule, contextChanged, topic, runtimePack, tenant, contextNegative, actionsNegative := base, base, base, base, base, base, base, base
	source.SourceRevision = strings.Repeat("b", 64)
	rule.RuleRevision = strings.Repeat("c", 64)
	contextChanged.ContextDigest = strings.Repeat("d", 64)
	topic.TopicRevision = strings.Repeat("e", 64)
	runtimePack.RuntimePackDigest = strings.Repeat("f", 64)
	tenant.TenantDigest = strings.Repeat("1", 64)
	contextNegative.ContextDigest = strings.Repeat("2", 64)
	actionsNegative.ActionsDigest = strings.Repeat("3", 64)
	authority := PerformanceAuthorityFixture{Tenant: "tenant-a", User: "user-a", Session: "session-a", Scopes: []string{"reporting.execute", "cw.report.execute:report-a", "cw.source.query:source-a", "cw.execution_context.use:context-a"}, TargetTenant: "tenant-a", ReportID: "report-a", SourceID: "source-a", ContextID: "context-a"}
	tenantDenied := authority
	tenantDenied.Tenant = "tenant-b"
	contextDenied := authority
	contextDenied.ContextID = "context-b"
	actionsDenied := authority
	actionsDenied.Scopes = []string{"cw.report.execute:report-a", "cw.source.query:source-a", "cw.execution_context.use:context-a"}
	m := PerformanceManifest{
		SchemaVersion: 1,
		ID:            "performance-test",
		Kind:          PerformanceSmoke,
		EvidenceMode:  PerformanceSynthetic,
		Environment:   PerformanceEnvironment{RunnerLabel: "test", OS: "test-os", Architecture: "test-arch", CPUs: 2, GoVersion: "go-test", DatasetDigest: a, DatasetRows: 100, SourceMode: "synthetic", ModelMode: "none", EvaluationSuite: a, EvaluationReport: a},
		Authority:     authority,
		MaxDurationMS: 5000,
		Steps: []PerformanceStep{
			step("cold", "cold", base, true, true, 1, 1, 1, 0),
			step("warm", "warm", base, true, false, 3, 1, 0, 0),
			step("repeat", "repeat", base, true, false, 4, 2, 0, 0),
			step("concurrent", "concurrent", base, true, true, 8, 8, 1, 0),
			step("source", "source_changed", source, true, false, 1, 1, 1, 0),
			step("rule", "rule_changed", rule, true, false, 1, 1, 1, 0),
			step("context", "context_changed", contextChanged, true, false, 1, 1, 1, 0),
			step("topic", "topic_changed", topic, true, false, 1, 1, 1, 0),
			step("runtime", "runtime_pack_changed", runtimePack, true, false, 1, 1, 1, 0),
			step("tenant-negative", "tenant_negative", tenant, false, false, 2, 1, 0, 2),
			step("context-negative", "context_negative", contextNegative, false, false, 2, 2, 0, 2),
			step("actions-negative", "actions_negative", actionsNegative, false, false, 2, 2, 0, 2),
		},
	}
	m.Steps[9].AuthorityOverride = &tenantDenied
	m.Steps[10].AuthorityOverride = &contextDenied
	m.Steps[11].AuthorityOverride = &actionsDenied
	return m
}

// newTestSyntheticPerformanceRunner is deliberately test-only. Production
// callers must supply authority that was produced by the configured verifier;
// manifest fixture fields are never an identity source.
func newTestSyntheticPerformanceRunner() *syntheticPerformanceRunner {
	now := time.Unix(1_800_000_000, 0)
	return newSyntheticPerformanceRunner(func(_ context.Context, fixture PerformanceAuthorityFixture) (identity.Envelope, error) {
		return identity.FromVerified(fixture.Tenant, fixture.User, fixture.Session, fixture.Scopes, now.Add(time.Hour), func() time.Time { return now })
	})
}

func TestPerformanceManifestPinsAuthorityAndInvalidation(t *testing.T) {
	m := performanceTestManifest()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	tampered := m
	tampered.Steps = append([]PerformanceStep(nil), m.Steps...)
	tampered.Steps[4].Binding.ContextDigest = strings.Repeat("9", 64)
	if !errors.Is(tampered.Validate(), ErrInvalid) {
		t.Fatal("multi-dimensional invalidation accepted")
	}
	tampered = m
	tampered.Steps = append([]PerformanceStep(nil), m.Steps...)
	tampered.Steps[9].Allowed = true
	if !errors.Is(tampered.Validate(), ErrInvalid) {
		t.Fatal("cross-tenant negative widened")
	}
}

func TestPerformanceSmokeMeasuresRawReuseAndNegatives(t *testing.T) {
	m := performanceTestManifest()
	now := time.Unix(1000, 0)
	report, err := MeasurePerformance(context.Background(), m, newTestSyntheticPerformanceRunner(), func() time.Time {
		now = now.Add(time.Millisecond)
		return now
	})
	if err != nil || !report.CorrectnessPassed || report.EvidenceHash == "" || len(report.Samples) != 27 || len(report.Summaries) != len(m.Steps) {
		t.Fatal(report, err)
	}
	if err = report.Validate(m); err != nil {
		t.Fatal(err)
	}
	for i, summary := range report.Summaries {
		if summary.Executions != m.Steps[i].ExpectedExecutions || summary.Blocks != m.Steps[i].ExpectedBlocks || summary.MaxWallNS < summary.MinWallNS {
			t.Fatal(summary)
		}
	}
	for _, sample := range report.Samples {
		if sample.Blocked && (sample.Executed || sample.Reused || sample.Usage.SourceCalls != 0 || sample.Usage.ModelCalls != 0) {
			t.Fatal("authority denial reached work", sample)
		}
		if sample.Usage.CostUSD != nil || sample.Usage.Tokens != nil || sample.Usage.ModelNS != nil {
			t.Fatal("synthetic run fabricated model usage", sample)
		}
	}
	tampered := report
	tampered.Samples = append([]PerformanceSample(nil), report.Samples...)
	tampered.Samples[0].Usage.SourceCalls++
	if !errors.Is(tampered.Validate(m), ErrInvalid) {
		t.Fatal("tampered raw evidence accepted")
	}
	mutations := []func(*PerformanceReport){
		func(r *PerformanceReport) { r.SchemaVersion++ },
		func(r *PerformanceReport) { r.ManifestID = "other-manifest" },
		func(r *PerformanceReport) { r.ManifestDigest = strings.Repeat("9", 64) },
		func(r *PerformanceReport) { r.StartedAt = r.StartedAt.Add(-time.Millisecond) },
		func(r *PerformanceReport) { r.CompletedAt = r.CompletedAt.Add(time.Millisecond) },
		func(r *PerformanceReport) { r.Environment.RunnerLabel = "other-runner" },
	}
	for i, mutate := range mutations {
		tampered = report
		mutate(&tampered)
		if !errors.Is(tampered.Validate(m), ErrInvalid) {
			t.Fatalf("tampered persisted field %d accepted", i)
		}
	}
}

type failingPerformanceRunner struct {
	runs atomic.Int64
}

func (r *failingPerformanceRunner) Check(_ context.Context, step PerformanceStep) (PerformanceAdapterResult, error) {
	return PerformanceAdapterResult{SemanticDigest: strings.Repeat("9", 64), BindingDigest: step.Binding.digest()}, nil
}
func (*failingPerformanceRunner) Reset(context.Context) error { return nil }
func (r *failingPerformanceRunner) Run(context.Context, PerformanceStep, int) (PerformanceAdapterResult, error) {
	r.runs.Add(1)
	return PerformanceAdapterResult{}, nil
}

func TestPerformanceCorrectnessFailsBeforeTiming(t *testing.T) {
	runner := &failingPerformanceRunner{}
	report, err := MeasurePerformance(context.Background(), performanceTestManifest(), runner, nil)
	if !errors.Is(err, ErrGate) || report.CorrectnessPassed || len(report.Samples) != 0 || runner.runs.Load() != 0 {
		t.Fatal(report, err, runner.runs.Load())
	}
}

type receiptTamperRunner struct {
	inner *syntheticPerformanceRunner
	mode  string
}

func (r *receiptTamperRunner) Check(ctx context.Context, step PerformanceStep) (PerformanceAdapterResult, error) {
	return r.inner.Check(ctx, step)
}
func (r *receiptTamperRunner) Reset(ctx context.Context) error { return r.inner.Reset(ctx) }
func (r *receiptTamperRunner) Run(ctx context.Context, step PerformanceStep, iteration int) (PerformanceAdapterResult, error) {
	if r.mode == "noop" {
		return r.inner.authorize(ctx, step)
	}
	result, err := r.inner.Run(ctx, step, iteration)
	if err == nil && step.Kind == "warm" {
		ns := int64(1)
		result.Receipt.Usage.SourceCalls = 1
		result.Receipt.Usage.SourceNS = &ns
	}
	return result, err
}

func TestPerformanceDerivesOutcomeFromIndependentReceipts(t *testing.T) {
	for _, mode := range []string{"noop", "reuse-with-work"} {
		t.Run(mode, func(t *testing.T) {
			runner := &receiptTamperRunner{inner: newTestSyntheticPerformanceRunner(), mode: mode}
			report, err := MeasurePerformance(context.Background(), performanceTestManifest(), runner, nil)
			if !errors.Is(err, ErrGate) || !report.CorrectnessPassed {
				t.Fatal(report, err)
			}
		})
	}
}

func TestPerformanceAuthorityExpectationCannotControlDenial(t *testing.T) {
	m := performanceTestManifest()
	m.Steps = append([]PerformanceStep(nil), m.Steps...)
	wrong := m.Authority
	m.Steps[9].AuthorityOverride = &wrong
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	report, err := MeasurePerformance(context.Background(), m, newTestSyntheticPerformanceRunner(), nil)
	if !errors.Is(err, ErrGate) || report.CorrectnessPassed || len(report.Samples) != 0 {
		t.Fatal(report, err)
	}
}

func TestPerformanceAuthorityFixturesUseVerifiedBearerAndProtectedResources(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := json.Marshal(map[string]any{"keys": []any{map[string]any{"kty": "EC", "use": "sig", "alg": "ES256", "kid": "performance-key", "crv": "P-256", "x": base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))), "y": base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32)))}}})
	jwks := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(doc)
	}))
	t.Cleanup(jwks.Close)
	now := time.Now().Truncate(time.Second)
	cfg := config.Defaults().Auth
	cfg.Issuer = jwks.URL + "/issuer"
	cfg.JWKSURL = jwks.URL
	cfg.Audience = ""
	cfg.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp"}
	cfg.RefreshInterval = config.Duration(time.Second)
	cfg.JWKSMaxStale = config.Duration(10 * time.Second)
	verifier, err := auth.New(cfg, jwks.Client(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(verifier.Close)
	runner := newSyntheticPerformanceRunner(func(ctx context.Context, fixture PerformanceAuthorityFixture) (identity.Envelope, error) {
		claims := jwt.MapClaims{"iss": cfg.Issuer, "aud": "chartworks:http", "sub": fixture.User, "tenant": fixture.Tenant, "user": fixture.User, "session": fixture.Session, "iat": now.Unix(), "nbf": now.Add(-time.Minute).Unix(), "exp": now.Add(5 * time.Minute).Unix(), "scopes": fixture.Scopes}
		token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
		token.Header["kid"] = "performance-key"
		signed, signErr := token.SignedString(key)
		if signErr != nil {
			return identity.Envelope{}, signErr
		}
		return verifier.Verify(ctx, signed, auth.HTTP)
	})
	report, err := MeasurePerformance(context.Background(), performanceTestManifest(), runner, nil)
	if err != nil || !report.CorrectnessPassed {
		t.Fatal(report, err)
	}
	for _, sample := range report.Samples {
		if sample.Blocked && (sample.Usage.SourceCalls != 0 || sample.Usage.ModelCalls != 0) {
			t.Fatal("verified denial reached physical work", sample)
		}
	}
}

func TestPerformanceNearestRankP95(t *testing.T) {
	for _, tc := range []struct {
		n, want int
	}{{1, 1}, {2, 2}, {4, 4}, {20, 19}, {100, 95}} {
		samples := make([]PerformanceSample, tc.n)
		for i := range samples {
			samples[i] = PerformanceSample{WallNS: int64(i + 1)}
		}
		if got := summarizePerformance("p95", samples).P95WallNS; got != int64(tc.want) {
			t.Fatalf("n=%d got=%d want=%d", tc.n, got, tc.want)
		}
	}
}

type hangingPerformanceRunner struct{ inner *syntheticPerformanceRunner }

func (r *hangingPerformanceRunner) Check(ctx context.Context, step PerformanceStep) (PerformanceAdapterResult, error) {
	return r.inner.Check(ctx, step)
}
func (r *hangingPerformanceRunner) Reset(ctx context.Context) error { return r.inner.Reset(ctx) }
func (*hangingPerformanceRunner) Run(ctx context.Context, _ PerformanceStep, _ int) (PerformanceAdapterResult, error) {
	<-ctx.Done()
	return PerformanceAdapterResult{}, ctx.Err()
}

func TestPerformanceWorkerPoolJoinsOnCancellation(t *testing.T) {
	m := performanceTestManifest()
	m.MaxDurationMS = 20
	started := time.Now()
	_, err := MeasurePerformance(context.Background(), m, &hangingPerformanceRunner{inner: newTestSyntheticPerformanceRunner()}, nil)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Fatal(err, time.Since(started))
	}
}

func TestPerformanceManifestCannotMintAuthority(t *testing.T) {
	m := performanceTestManifest()
	step := m.Steps[0]
	step.AuthorityOverride = &m.Authority
	if _, err := newSyntheticPerformanceRunner(nil).authorize(context.Background(), step); !errors.Is(err, ErrMode) {
		t.Fatal("raw manifest created or bypassed verified authority", err)
	}
	raw, _ := json.Marshal(m)
	profilePath := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(profilePath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := Command(context.Background(), []string{"perf-smoke", "--profile", profilePath}, &out, &stderr); code != 2 || !strings.Contains(stderr.String(), "cannot create verified authority") || out.Len() != 0 {
		t.Fatal(code, out.String(), stderr.String())
	}
}

func TestPerformanceCommandInspectsAndStoresBoundedReport(t *testing.T) {
	m := performanceTestManifest()
	m.Environment.OS, m.Environment.Architecture, m.Environment.GoVersion, m.Environment.CPUs = "", "", "", 0
	raw, _ := json.Marshal(m)
	dir := t.TempDir()
	profilePath, reportPath := filepath.Join(dir, "profile.json"), filepath.Join(dir, "report.json")
	if err := os.WriteFile(profilePath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := Command(context.Background(), []string{"perf-inspect", "--profile", profilePath}, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	if !strings.Contains(out.String(), `"evidence_mode":"synthetic"`) {
		t.Fatal(out.String())
	}
	runtimeEnvironment := RuntimePerformanceEnvironment(m.Environment.RunnerLabel)
	m.Environment.OS = runtimeEnvironment.OS
	m.Environment.Architecture = runtimeEnvironment.Architecture
	m.Environment.CPUs = runtimeEnvironment.CPUs
	m.Environment.GoVersion = runtimeEnvironment.GoVersion
	report, err := MeasurePerformance(context.Background(), m, newTestSyntheticPerformanceRunner(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = writePerformanceReport(reportPath, report); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- reportPath is beneath this test's private temporary directory.
	stored, err := os.ReadFile(reportPath)
	info, statErr := os.Stat(reportPath)
	if err != nil || statErr != nil || !bytes.Contains(stored, []byte(`"correctness_passed": true`)) || info.Mode().Perm() != 0600 {
		t.Fatal(err, statErr, string(stored))
	}
}
