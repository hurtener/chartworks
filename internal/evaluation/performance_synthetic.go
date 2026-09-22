package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// syntheticPerformanceRunner is deliberately small and deterministic. It
// measures harness/reuse overhead only; its reports are never live source or
// model evidence.
type syntheticPerformanceRunner struct {
	mu    sync.Mutex
	cache map[string]string
}

func newSyntheticPerformanceRunner() *syntheticPerformanceRunner {
	return &syntheticPerformanceRunner{cache: map[string]string{}}
}

func (r *syntheticPerformanceRunner) Check(_ context.Context, step PerformanceStep) (PerformanceObservation, error) {
	return syntheticPerformanceObservation(step, false, false, PerformanceUsage{}), nil
}

func (r *syntheticPerformanceRunner) Reset(_ context.Context) error {
	r.mu.Lock()
	r.cache = map[string]string{}
	r.mu.Unlock()
	return nil
}

func (r *syntheticPerformanceRunner) Run(ctx context.Context, step PerformanceStep, _ int) (PerformanceObservation, error) {
	if err := ctx.Err(); err != nil {
		return PerformanceObservation{}, err
	}
	if !step.Allowed {
		return syntheticPerformanceObservation(step, false, false, PerformanceUsage{}), nil
	}
	started := time.Now()
	key := step.Binding.digest()
	r.mu.Lock()
	if value, ok := r.cache[key]; ok {
		r.mu.Unlock()
		usage := PerformanceUsage{ServiceNS: time.Since(started).Nanoseconds()}
		return PerformanceObservation{SemanticDigest: value, BindingDigest: key, Reused: true, Usage: usage}, nil
	}
	sourceStarted := time.Now()
	digest := syntheticWorkloadDigest(step.Workload)
	sourceNS := time.Since(sourceStarted).Nanoseconds()
	r.cache[key] = digest
	r.mu.Unlock()
	usage := PerformanceUsage{ServiceNS: time.Since(started).Nanoseconds(), SourceNS: &sourceNS, SourceCalls: 1}
	return PerformanceObservation{SemanticDigest: digest, BindingDigest: key, Executed: true, Usage: usage}, nil
}

func syntheticPerformanceObservation(step PerformanceStep, executed, reused bool, usage PerformanceUsage) PerformanceObservation {
	return PerformanceObservation{SemanticDigest: syntheticWorkloadDigest(step.Workload), BindingDigest: step.Binding.digest(), Blocked: !step.Allowed, Executed: executed, Reused: reused, Usage: usage}
}

func syntheticWorkloadDigest(workload string) string {
	sum := sha256.Sum256([]byte(workload))
	return hex.EncodeToString(sum[:])
}
