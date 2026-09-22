package evaluation

import (
	"context"
	"fmt"
	"time"
)

// Runner executes a live case through injected governed services.
type Runner interface {
	Observe(context.Context, Suite, Case) (Observation, error)
}

// RunnerFunc adapts a function to Runner.
type RunnerFunc func(context.Context, Suite, Case) (Observation, error)

// Observe executes one adapted case.
func (f RunnerFunc) Observe(ctx context.Context, s Suite, c Case) (Observation, error) {
	return f(ctx, s, c)
}

// Clock supplies deterministic evaluation timestamps.
type Clock func() time.Time

// Evaluate executes an already reviewed manifest. Live suites require an injected
// runner; fixture observations can never be reported as live evidence.
func Evaluate(ctx context.Context, runID string, suite Suite, runner Runner, clock Clock) (Report, error) {
	if ctx == nil || !identifier(runID) || suite.Validate() != nil {
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
	report := Report{SchemaVersion: SchemaVersion, RunID: runID, SuiteID: suite.ID, SuiteRevision: suite.Revision, Mode: suite.Mode, Seed: suite.Seed, SuiteDigest: suiteDigest, StartedAt: started, Cases: []CaseResult{}}
	deadline := started.Add(time.Duration(suite.Limits.DurationMS) * time.Millisecond)
	for _, c := range stableCases(suite.Cases) {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if !clock().Before(deadline) {
			return report, ErrBudget
		}
		var o Observation
		var err error
		if suite.Mode == Fixture {
			o = *c.Fixture
		} else {
			o, err = runner.Observe(ctx, suite, c)
		}
		if err != nil {
			o.ErrorClass = "dependency_unavailable"
		}
		if validateObservation(o) != nil {
			return report, ErrInvalid
		}
		if o.Usage.Calls > suite.Limits.Calls || o.Usage.Retries > suite.Limits.Retries || o.Usage.Tokens != nil && *o.Usage.Tokens > suite.Limits.Tokens {
			return report, ErrBudget
		}
		passed, reason := match(c, o)
		if c.Critical && !o.Blocked {
			passed, reason = false, "security_not_blocked"
		}
		r := CaseResult{ID: c.ID, Stage: c.Stage, Category: c.Category, Locale: c.Locale, Critical: c.Critical, HeldOut: c.HeldOut, Passed: passed, Reason: reason, Observation: o}
		report.Cases = append(report.Cases, r)
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
		if report.Usage.Calls > suite.Limits.Calls || report.Usage.Retries > suite.Limits.Retries || report.Usage.Tokens != nil && *report.Usage.Tokens > suite.Limits.Tokens {
			return report, ErrBudget
		}
	}
	report.GatePassed = report.SecurityFailures == 0
	if suite.Threshold.QualityMin == nil {
		report.GatePassed = false
	} else if report.QualityTotal > 0 && float64(report.QualityPassed)/float64(report.QualityTotal) < *suite.Threshold.QualityMin {
		report.GatePassed = false
	}
	report.CompletedAt = clock().UTC()
	evidence := struct {
		Suite string
		Seed  int64
		Cases []CaseResult
		Mode  Mode
	}{suiteDigest, suite.Seed, report.Cases, suite.Mode}
	report.EvidenceHash, _ = digest(evidence)
	if !report.GatePassed {
		return report, fmt.Errorf("%w: quality=%d/%d security=%d", ErrGate, report.QualityPassed, report.QualityTotal, report.SecurityFailures)
	}
	return report, nil
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
