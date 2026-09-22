package evaluation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
)

// Reservation is granted before any delegated call. A runner must remain within it.
type Reservation struct {
	Calls    int       `json:"calls"`
	Tokens   int       `json:"tokens"`
	Retries  int       `json:"retries"`
	CostUSD  *float64  `json:"cost_usd,omitempty"`
	Deadline time.Time `json:"deadline"`
}

// Execution is sealed case material plus its pre-admitted budget.
type Execution struct {
	Suite         Suite
	Case          Case
	Pack          PackRevision
	Reservation   Reservation
	Envelope      identity.Envelope
	RuntimeConfig gateway.RuntimeConfig `json:"-"`
	RuntimeDigest string                `json:"-"`
}

// Runner observes a pre-admitted live case.
type Runner interface {
	Observe(context.Context, Execution) (Observation, error)
}

// RunnerFunc adapts a function to Runner.
type RunnerFunc func(context.Context, Execution) (Observation, error)

// Observe invokes the adapted function.
func (f RunnerFunc) Observe(ctx context.Context, x Execution) (Observation, error) { return f(ctx, x) }

// Clock supplies deterministic timestamps.
type Clock func() time.Time

// FailureReport builds reproducible terminal evidence for an admitted run recovered after a crash.
func FailureReport(runID string, suite Suite, status, failure string, started, completed time.Time) (Report, error) {
	if len(suite.Packs) == 0 {
		return Report{}, ErrInvalid
	}
	return FailureReportWithPack(runID, suite, suite.Packs[0], status, failure, started, completed)
}

// FailureReportWithPack binds crash evidence to the exact admitted pack.
func FailureReportWithPack(runID string, suite Suite, pack PackRevision, status, failure string, started, completed time.Time) (Report, error) {
	if !identifier(runID) || suite.Validate() != nil || !validPack(pack) || !validReportStatus(status) || status == "passed" || failure == "" {
		return Report{}, ErrInvalid
	}
	sd, _ := suite.Digest()
	r := Report{SchemaVersion: SchemaVersion, RunID: runID, SuiteID: suite.ID, SuiteRevision: suite.Revision, Mode: suite.Mode, Seed: suite.Seed, SuiteDigest: sd, Pack: pack, Status: status, FailureClass: failure, StartedAt: started.UTC(), CompletedAt: completed.UTC(), Cases: []CaseResult{}}
	if r.CompletedAt.Before(r.StartedAt) {
		return Report{}, ErrInvalid
	}
	e := struct {
		Suite   string
		Pack    PackRevision
		Seed    int64
		Cases   []CaseResult
		Mode    Mode
		Status  string
		Failure string
	}{sd, pack, suite.Seed, r.Cases, suite.Mode, status, failure}
	r.EvidenceHash, _ = digest(e)
	return r, nil
}

// Evaluate always returns hashable terminal evidence after admission, including failures.
func Evaluate(ctx context.Context, runID string, suite Suite, runner Runner, clock Clock) (Report, error) {
	if len(suite.Packs) == 0 {
		return Report{}, ErrInvalid
	}
	return EvaluateWithPack(ctx, runID, suite, suite.Packs[0], runner, clock)
}

// EvaluateWithPack runs a suite with one exact immutable admitted pack revision.
func EvaluateWithPack(ctx context.Context, runID string, suite Suite, pack PackRevision, runner Runner, clock Clock) (report Report, retErr error) {
	if ctx == nil || !identifier(runID) || suite.Validate() != nil || !validPack(pack) {
		return Report{}, ErrInvalid
	}
	if clock == nil {
		clock = time.Now
	}
	if suite.Mode == Live && runner == nil {
		return Report{}, ErrMode
	}
	started := clock().UTC()
	suiteDigest, _ := digest(suite)
	report = Report{SchemaVersion: SchemaVersion, RunID: runID, SuiteID: suite.ID, SuiteRevision: suite.Revision, Mode: suite.Mode, Seed: suite.Seed, SuiteDigest: suiteDigest, Pack: pack, StartedAt: started, Cases: []CaseResult{}, Status: "failed"}
	finish := func(status, failure string, err error) (Report, error) {
		report.Status, report.FailureClass, report.CompletedAt = status, failure, clock().UTC()
		if report.CompletedAt.Before(report.StartedAt) {
			report.CompletedAt = report.StartedAt
		}
		evidence := struct {
			Suite   string
			Pack    PackRevision
			Seed    int64
			Cases   []CaseResult
			Mode    Mode
			Status  string
			Failure string
		}{suiteDigest, pack, suite.Seed, report.Cases, suite.Mode, status, failure}
		report.EvidenceHash, _ = digest(evidence)
		return report, err
	}
	deadline := started.Add(time.Duration(suite.Limits.DurationMS) * time.Millisecond)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	cases := stableCases(suite.Cases)
	remainingCalls, remainingTokens, remainingRetries := suite.Limits.Calls, suite.Limits.Tokens, suite.Limits.Retries
	remainingCost := suite.Limits.CostUSD
	for i, c := range cases {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return finish("timed_out", "deadline_exceeded", err)
			}
			return finish("cancelled", "cancelled", err)
		}
		if !clock().Before(deadline) {
			return finish("timed_out", "deadline_exceeded", ErrBudget)
		}
		left := len(cases) - i
		reserve := Reservation{Calls: remainingCalls / left, Tokens: remainingTokens / left, Retries: remainingRetries / left, Deadline: deadline}
		if remainingCost != nil {
			v := *remainingCost / float64(left)
			reserve.CostUSD = &v
		}
		if suite.Mode == Live && reserve.Calls < 1 {
			return finish("budget_exhausted", "calls_exhausted", ErrBudget)
		}
		var o Observation
		var err error
		if suite.Mode == Fixture {
			o = *c.Fixture
		} else {
			caseCtx, cancel := context.WithDeadline(ctx, reserve.Deadline)
			o, err = runner.Observe(caseCtx, Execution{Suite: suite, Case: c, Pack: pack, Reservation: reserve})
			cancel()
		}
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return finish("timed_out", "deadline_exceeded", err)
			}
			if errors.Is(err, context.Canceled) {
				return finish("cancelled", "cancelled", err)
			}
			if errors.Is(err, gateway.ErrBudget) || errors.Is(err, ErrBudget) {
				return finish("budget_exhausted", "reservation_exceeded", ErrBudget)
			}
			o = Observation{ErrorClass: "dependency_unavailable", Usage: o.Usage}
		}
		if validateObservation(o) != nil {
			return finish("dependency_failed", "invalid_observation", ErrInvalid)
		}
		if reserve.CostUSD != nil && o.Usage.Calls > 0 && o.Usage.CostUSD == nil {
			return finish("dependency_failed", "unknown_cost", ErrBudget)
		}
		if o.Usage.Calls > reserve.Calls || o.Usage.Retries > reserve.Retries || o.Usage.Tokens != nil && *o.Usage.Tokens > reserve.Tokens || reserve.CostUSD != nil && o.Usage.CostUSD != nil && *o.Usage.CostUSD > *reserve.CostUSD {
			return finish("budget_exhausted", "reservation_exceeded", ErrBudget)
		}
		remainingCalls -= o.Usage.Calls
		remainingRetries -= o.Usage.Retries
		if o.Usage.Tokens != nil {
			remainingTokens -= *o.Usage.Tokens
		}
		if remainingCost != nil && o.Usage.CostUSD != nil {
			v := *remainingCost - *o.Usage.CostUSD
			remainingCost = &v
		}
		passed, reason := match(c, o)
		if c.Critical && !o.Blocked {
			passed, reason = false, "security_not_blocked"
		}
		report.Cases = append(report.Cases, CaseResult{ID: c.ID, Stage: c.Stage, Category: c.Category, Locale: c.Locale, Critical: c.Critical, HeldOut: c.HeldOut, Passed: passed, Reason: reason, Observation: o})
		if c.Critical {
			if !passed {
				report.SecurityFailures++
			}
		} else {
			report.QualityTotal++
			if passed {
				report.QualityPassed++
			}
		}
		if len(report.Cases) == 1 {
			report.Usage = o.Usage
		} else {
			report.Usage = addUsage(report.Usage, o.Usage)
		}
	}
	report.GatePassed = report.SecurityFailures == 0 && suite.Threshold.QualityMin != nil && report.QualityTotal > 0 && float64(report.QualityPassed)/float64(report.QualityTotal) >= *suite.Threshold.QualityMin
	if !report.GatePassed {
		return finish("failed", "gate_failed", fmt.Errorf("%w: quality=%d/%d security=%d", ErrGate, report.QualityPassed, report.QualityTotal, report.SecurityFailures))
	}
	return finish("passed", "", nil)
}
func match(c Case, o Observation) (bool, string) {
	for _, e := range c.Expected {
		if e.Decision == o.Decision && e.SemanticDigest == o.SemanticDigest && e.ErrorClass == o.ErrorClass {
			return true, "equivalent"
		}
	}
	if o.ErrorClass != "" {
		return false, "typed_failure"
	}
	return false, "semantic_drift"
}
func addUsage(a, b Usage) Usage {
	a.ServiceMS += b.ServiceMS
	a.Calls += b.Calls
	a.Retries += b.Retries
	a.SourceMS = addOptional(a.SourceMS, b.SourceMS)
	a.ModelMS = addOptional(a.ModelMS, b.ModelMS)
	a.Tokens = addOptionalInt(a.Tokens, b.Tokens)
	a.CostUSD = addOptionalFloat(a.CostUSD, b.CostUSD)
	return a
}
func addOptional(a, b *int64) *int64 {
	if a == nil || b == nil {
		return nil
	}
	v := *a + *b
	return &v
}
func addOptionalInt(a, b *int) *int {
	if a == nil || b == nil {
		return nil
	}
	v := *a + *b
	return &v
}
func addOptionalFloat(a, b *float64) *float64 {
	if a == nil || b == nil {
		return nil
	}
	v := *a + *b
	return &v
}
