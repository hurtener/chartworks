package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/gateway/bifrost"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"strings"
	"sync"
	"testing"
)

// Records bounded gateway usage. Explicit test-only tracing can additionally
// retain this synthetic fixture's rendered prompts and parsed SQL, never private
// parameter values, raw provider errors or HTTP headers. The embedded concrete engine
// preserves the production envelope provider and every other constructor seam.
type liveReceiptEngine struct {
	*bifrost.Engine
	mu           sync.Mutex
	calls        []gateway.Usage
	traceEnabled bool
	traceBytes   int
	traces       []liveGenerationTrace
}

func (e *liveReceiptEngine) record(r gateway.Receipt) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, r.Calls...)
}
func (e *liveReceiptEngine) count() int { e.mu.Lock(); defer e.mu.Unlock(); return len(e.calls) }
func (e *liveReceiptEngine) since(n int) []gateway.Usage {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]gateway.Usage(nil), e.calls[n:]...)
}
func (e *liveReceiptEngine) Generate(ctx context.Context, c gateway.Call, b *gateway.Budget, role, system, prompt string, s *gateway.Schema) (gateway.Generated, error) {
	o, err := e.Engine.Generate(ctx, c, b, role, system, prompt, s)
	e.record(o.Receipt)
	e.recordTrace(role, system, prompt, o, err)
	return o, err
}
func (e *liveReceiptEngine) Embed(ctx context.Context, c gateway.Call, b *gateway.Budget, role string, in []string) (gateway.Embedded, error) {
	o, err := e.Engine.Embed(ctx, c, b, role, in)
	e.record(o.Receipt)
	return o, err
}
func (e *liveReceiptEngine) Rerank(ctx context.Context, c gateway.Call, b *gateway.Budget, q string, in gateway.Candidates) (gateway.Ranked, error) {
	o, err := e.Engine.Rerank(ctx, c, b, q, in)
	e.record(o.Receipt)
	return o, err
}
func (e *liveReceiptEngine) VisualRank(ctx context.Context, c gateway.Call, b *gateway.Budget, q string, in gateway.Candidates) (gateway.Ranked, error) {
	o, err := e.Engine.VisualRank(ctx, c, b, q, in)
	e.record(o.Receipt)
	return o, err
}

var _ gateway.GenerationEnvelopeProvider = (*liveReceiptEngine)(nil)

func TestLiveFailureDiagnosticsStayClosed(t *testing.T) {
	if got := livePlanFailureReason(errors.Join(nlqexec.ErrValidationBudget, &readexec.AnalyticalError{Code: "analytical_grain_mismatch"})); got != "analytical_grain_mismatch" {
		t.Fatal("lost underlying proof code", got)
	}
	if got := livePlanFailureReason(&readexec.AnalyticalError{Code: "PRIVATE_CANARY"}); got != "plan_error" {
		t.Fatal("untrusted code escaped", got)
	}
	if got := livePlanFailureReason(errors.New("PRIVATE_CANARY")); got != "plan_error" {
		t.Fatal("raw error escaped", got)
	}
	e := &liveReceiptEngine{}
	start := e.count()
	e.record(gateway.Receipt{Calls: []gateway.Usage{{Role: "sqlgen", Provider: "synthetic", Attempts: 1}}})
	if len(e.since(start)) != 1 {
		t.Fatal("failed attempt usage lost")
	}
	e.since(start)[0].Role = "changed"
	if e.since(start)[0].Role != "sqlgen" {
		t.Fatal("receipt alias")
	}
}

// This DTO is intentionally not a provider wire/request response dump.
type liveGenerationTrace struct {
	Role           string          `json:"role"`
	System         string          `json:"system"`
	Prompt         string          `json:"prompt"`
	Decision       string          `json:"decision"`
	SQL            string          `json:"sql"`
	ParameterKinds []string        `json:"parameter_kinds"`
	Failure        string          `json:"failure,omitempty"`
	Receipt        gateway.Receipt `json:"receipt"`
}

func (e *liveReceiptEngine) recordTrace(role, system, prompt string, out gateway.Generated, err error) {
	if !e.traceEnabled {
		return
	}
	if len(system) > 128<<10 || len(prompt) > 128<<10 || len(out.JSON) > 64<<10 {
		return
	}
	var candidate struct {
		Decision   string `json:"decision"`
		SQL        string `json:"sql"`
		Parameters []struct {
			Kind string `json:"kind"`
		} `json:"parameters"`
	}
	if json.Unmarshal(out.JSON, &candidate) != nil {
		candidate.Decision, candidate.SQL, candidate.Parameters = "unparsed_output", "", nil
	}
	trace := liveGenerationTrace{Role: role, System: system, Prompt: prompt, SQL: candidate.SQL, Decision: candidate.Decision, Receipt: out.Receipt}
	for _, p := range candidate.Parameters {
		switch p.Kind {
		case "null", "boolean", "number", "text":
			trace.ParameterKinds = append(trace.ParameterKinds, p.Kind)
		default:
			trace.ParameterKinds = append(trace.ParameterKinds, "unknown")
		}
	}
	if err != nil {
		trace.Failure = livePlanFailureReason(err)
	}
	raw, _ := json.Marshal(trace)
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.traces) >= 32 || e.traceBytes+len(raw) > 2<<20 {
		return
	}
	e.traceBytes += len(raw)
	e.traces = append(e.traces, trace)
}
func (e *liveReceiptEngine) traceSnapshot() []liveGenerationTrace {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]liveGenerationTrace(nil), e.traces...)
}
func TestLiveSyntheticTraceOmitsParameterValues(t *testing.T) {
	e := &liveReceiptEngine{traceEnabled: true}
	e.recordTrace("sqlgen", "synthetic system", "synthetic question", gateway.Generated{JSON: json.RawMessage(`{"decision":"ready","sql":"SELECT $1","parameters":[{"kind":"text","value":"PRIVATE_PARAMETER_CANARY"}]}`)}, nil)
	raw, _ := json.Marshal(e.traceSnapshot())
	if len(e.traceSnapshot()) != 1 || strings.Contains(string(raw), "PRIVATE_PARAMETER_CANARY") || !strings.Contains(string(raw), "parameter_kinds") {
		t.Fatal("trace DTO leaked value or lost shape")
	}
	disabled := &liveReceiptEngine{}
	disabled.recordTrace("sqlgen", "system", "prompt", gateway.Generated{JSON: json.RawMessage(`{"sql":"SELECT 1"}`)}, nil)
	if len(disabled.traceSnapshot()) != 0 {
		t.Fatal("trace enabled by default")
	}
}
