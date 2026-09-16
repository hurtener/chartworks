package reporting

import (
	"context"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"math"
	"time"
)

// QueryLimits only narrows the existing opaque-plan reader. Planner cost is a
// native optimizer estimate, not measured scan bytes, spend, or a guarantee.
type QueryLimits struct {
	MaxRows       int     `json:"max_rows"`
	MaxBytes      int     `json:"max_bytes"`
	TimeoutMillis int     `json:"timeout_ms"`
	PlannerCost   float64 `json:"planner_cost_ceiling"`
	MaxAttempts   int     `json:"max_attempts"`
}

// Valid checks all authored bounds without granting source authority.
func (l QueryLimits) Valid() bool {
	return l.MaxRows >= 1 && l.MaxRows <= 100000 && l.MaxBytes >= 128 && l.MaxBytes <= 16<<20 && l.TimeoutMillis >= 1 && l.TimeoutMillis <= 60000 && l.PlannerCost > 0 && l.PlannerCost <= 1e12 && !math.IsNaN(l.PlannerCost) && !math.IsInf(l.PlannerCost, 0) && l.MaxAttempts >= 1 && l.MaxAttempts <= 3
}

// AcceptedLimits is immutable. Current execution may lower it, never raise it.
type AcceptedLimits struct {
	Query                  QueryLimits `json:"query"`
	NarrativeCalls         int         `json:"narrative_calls"`
	NarrativeTokens        int         `json:"narrative_tokens"`
	NarrativeTimeoutMillis int         `json:"narrative_timeout_ms"`
}

func (l AcceptedLimits) valid() bool {
	return l.Query.Valid() && l.NarrativeCalls >= 0 && l.NarrativeCalls <= 8 && l.NarrativeTokens >= 0 && l.NarrativeTokens <= 131072 && (l.NarrativeCalls == 0 && l.NarrativeTokens == 0 || l.NarrativeCalls > 0 && l.NarrativeTokens >= 64) && l.NarrativeTimeoutMillis >= 1 && l.NarrativeTimeoutMillis <= 60000
}
func clampQuery(a, b QueryLimits) QueryLimits {
	return QueryLimits{MaxRows: min(a.MaxRows, b.MaxRows), MaxBytes: min(a.MaxBytes, b.MaxBytes), TimeoutMillis: min(a.TimeoutMillis, b.TimeoutMillis), PlannerCost: min(a.PlannerCost, b.PlannerCost), MaxAttempts: min(a.MaxAttempts, b.MaxAttempts)}
}
func deploymentQuery(l config.ReportingExecution) QueryLimits {
	return QueryLimits{MaxRows: l.MaxRows, MaxBytes: l.MaxResultBytes, TimeoutMillis: int(time.Duration(l.Timeout) / time.Millisecond), PlannerCost: 1e12, MaxAttempts: 3}
}

type cappedExecutor interface {
	ExecuteCapped(context.Context, identity.Envelope, exec.Plan, exec.Options, exec.Caps) (exec.ExecutionReport, error)
	ConsumerCaps() (exec.Caps, int)
}

func executorQuery(executor Executor) (QueryLimits, bool) {
	capped, ok := executor.(cappedExecutor)
	if !ok {
		return QueryLimits{}, false
	}
	c, attempts := capped.ConsumerCaps()
	q := QueryLimits{MaxRows: c.Rows, MaxBytes: c.Bytes, TimeoutMillis: int(c.Timeout / time.Millisecond), PlannerCost: c.PlannerCost, MaxAttempts: min(attempts, 3)}
	return q, q.Valid()
}
func (s *Runs) acceptLimits(d Definition, requested *QueryLimits, selected []Output) (AcceptedLimits, error) {
	q := deploymentQuery(s.limits)
	if native, ok := executorQuery(s.blocks.executor); ok {
		q = clampQuery(q, native)
	} else if d.QueryLimits != nil || requested != nil {
		return AcceptedLimits{}, ErrUnavailable
	}
	for _, policy := range []*QueryLimits{d.QueryLimits, requested} {
		if policy != nil {
			if !policy.Valid() {
				return AcceptedLimits{}, ErrInvalid
			}
			q = clampQuery(q, *policy)
		}
	}
	out := AcceptedLimits{Query: q, NarrativeTimeoutMillis: min(q.TimeoutMillis, int(time.Duration(s.limits.NarrativeTimeout)/time.Millisecond))}
	for _, o := range selected {
		if o.Kind == "narrative" {
			out.NarrativeCalls = s.limits.NarrativeCalls
			out.NarrativeTokens = s.limits.NarrativeTokens
			break
		}
	}
	if !out.valid() {
		return AcceptedLimits{}, ErrBudget
	}
	return out, nil
}
func acceptedQuery(m RunManifest) QueryLimits {
	if m.AcceptedLimits != nil {
		return m.AcceptedLimits.Query
	}
	return deploymentQuery(m.Limits)
}
func (s *Runs) currentQuery(m RunManifest) QueryLimits {
	q := clampQuery(acceptedQuery(m), deploymentQuery(s.limits))
	if native, ok := executorQuery(s.blocks.executor); ok {
		q = clampQuery(q, native)
	}
	return q
}
func executeWithCaps(ctx context.Context, e identity.Envelope, executor Executor, plan exec.Plan, options exec.Options, q QueryLimits, require bool) (exec.ExecutionReport, error) {
	if !q.Valid() {
		return exec.ExecutionReport{}, ErrBudget
	}
	options.Rows = min(options.Rows, q.MaxRows)
	options.Bytes = min(options.Bytes, q.MaxBytes)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(q.TimeoutMillis)*time.Millisecond)
	defer cancel()
	if capped, ok := executor.(cappedExecutor); ok {
		return capped.ExecuteCapped(ctx, e, plan, options, exec.Caps{Rows: q.MaxRows, Bytes: q.MaxBytes, Timeout: time.Duration(q.TimeoutMillis) * time.Millisecond, PlannerCost: q.PlannerCost})
	}
	if require {
		return exec.ExecutionReport{}, ErrUnavailable
	}
	return executor.Execute(ctx, e, plan, options)
}
func (s *Runs) narrativeBudget(m RunManifest, n Narrative) error {
	calls, tokens, timeout := min(m.Limits.NarrativeCalls, s.limits.NarrativeCalls), min(m.Limits.NarrativeTokens, s.limits.NarrativeTokens), min(time.Duration(m.Limits.NarrativeTimeout), time.Duration(s.limits.NarrativeTimeout))
	if m.AcceptedLimits != nil {
		calls = min(calls, m.AcceptedLimits.NarrativeCalls)
		tokens = min(tokens, m.AcceptedLimits.NarrativeTokens)
		timeout = min(timeout, time.Duration(m.AcceptedLimits.NarrativeTimeoutMillis)*time.Millisecond)
	}
	if n.MaxCalls < 1 || n.MaxCalls > calls || n.MaxTokens < 64 || n.MaxTokens > tokens || timeout < time.Millisecond {
		return ErrBudget
	}
	totalCalls, totalTokens := 0, 0
	for _, o := range m.Outputs {
		if o.Narrative != nil {
			totalCalls += o.Narrative.MaxCalls
			totalTokens += o.Narrative.MaxTokens
		}
	}
	if totalCalls > calls || totalTokens > tokens {
		return ErrBudget
	}
	return nil
}
