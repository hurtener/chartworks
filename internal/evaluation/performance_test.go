package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
	return PerformanceManifest{
		SchemaVersion: 1,
		ID:            "performance-test",
		Kind:          PerformanceSmoke,
		EvidenceMode:  PerformanceSynthetic,
		Environment:   PerformanceEnvironment{RunnerLabel: "test", OS: "test-os", Architecture: "test-arch", CPUs: 2, GoVersion: "go-test", DatasetDigest: a, DatasetRows: 100, SourceMode: "synthetic", ModelMode: "none", EvaluationSuite: a, EvaluationReport: a},
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
	report, err := MeasurePerformance(context.Background(), m, newSyntheticPerformanceRunner(), func() time.Time {
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
}

type failingPerformanceRunner struct {
	runs atomic.Int64
}

func (r *failingPerformanceRunner) Check(_ context.Context, step PerformanceStep) (PerformanceObservation, error) {
	o := syntheticPerformanceObservation(step, false, false, PerformanceUsage{})
	o.SemanticDigest = strings.Repeat("9", 64)
	return o, nil
}
func (*failingPerformanceRunner) Reset(context.Context) error { return nil }
func (r *failingPerformanceRunner) Run(context.Context, PerformanceStep, int) (PerformanceObservation, error) {
	r.runs.Add(1)
	return PerformanceObservation{}, nil
}

func TestPerformanceCorrectnessFailsBeforeTiming(t *testing.T) {
	runner := &failingPerformanceRunner{}
	report, err := MeasurePerformance(context.Background(), performanceTestManifest(), runner, nil)
	if !errors.Is(err, ErrGate) || report.CorrectnessPassed || len(report.Samples) != 0 || runner.runs.Load() != 0 {
		t.Fatal(report, err, runner.runs.Load())
	}
}

func TestPerformanceCommandStoresBoundedReportAndRefusesFinal(t *testing.T) {
	m := performanceTestManifest()
	m.Environment.OS, m.Environment.Architecture, m.Environment.GoVersion, m.Environment.CPUs = "", "", "", 0
	raw, _ := json.Marshal(m)
	dir := t.TempDir()
	profilePath, reportPath := filepath.Join(dir, "profile.json"), filepath.Join(dir, "report.json")
	if err := os.WriteFile(profilePath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := Command(context.Background(), []string{"perf-smoke", "--profile", profilePath, "--report", reportPath}, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	stored, err := os.ReadFile(reportPath)
	if err != nil || !bytes.Contains(stored, []byte(`"correctness_passed": true`)) || !strings.Contains(out.String(), `"evidence_mode":"synthetic"`) {
		t.Fatal(err, string(stored), out.String())
	}
	m.Kind, m.EvidenceMode = PerformanceFinalStress, PerformanceIntegration
	m.MaxDurationMS = int64(time.Hour / time.Millisecond)
	m.Environment.SourceMode, m.Environment.ModelMode = "real_postgres", "recorded"
	for i := range m.Steps {
		switch m.Steps[i].Kind {
		case "cold":
			m.Steps[i].Iterations, m.Steps[i].Concurrency = 20, 1
		case "warm":
			m.Steps[i].Iterations, m.Steps[i].Concurrency = 1000, 16
		case "repeat":
			m.Steps[i].Iterations, m.Steps[i].Concurrency = 10000, 64
		case "concurrent":
			m.Steps[i].Iterations, m.Steps[i].Concurrency = 2000, 128
		case "source_changed", "rule_changed", "context_changed", "topic_changed", "runtime_pack_changed":
			m.Steps[i].Iterations, m.Steps[i].Concurrency = 500, 32
			m.Steps[i].ExpectedExecutions = 1
		default:
			m.Steps[i].Iterations, m.Steps[i].Concurrency = 1000, 64
			m.Steps[i].ExpectedBlocks = 1000
		}
	}
	raw, _ = json.Marshal(m)
	if err = os.WriteFile(profilePath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	if code := Command(context.Background(), []string{"perf-smoke", "--profile", profilePath}, &out, &stderr); code != 2 || !strings.Contains(stderr.String(), "release runtime") {
		t.Fatal(code, stderr.String())
	}
}
