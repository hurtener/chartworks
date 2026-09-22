package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

// syntheticPerformanceRunner is deliberately small and deterministic. It
// measures harness/reuse overhead only; its reports are never live source or
// model evidence.
type syntheticPerformanceRunner struct {
	mu        sync.Mutex
	cache     map[string]string
	authority func(context.Context, PerformanceAuthorityFixture) (identity.Envelope, error)
}

func newSyntheticPerformanceRunner(authority func(context.Context, PerformanceAuthorityFixture) (identity.Envelope, error)) *syntheticPerformanceRunner {
	return &syntheticPerformanceRunner{cache: map[string]string{}, authority: authority}
}

func (r *syntheticPerformanceRunner) Check(ctx context.Context, step PerformanceStep) (PerformanceAdapterResult, error) {
	return r.authorize(ctx, step)
}

func (r *syntheticPerformanceRunner) Reset(_ context.Context) error {
	r.mu.Lock()
	r.cache = map[string]string{}
	r.mu.Unlock()
	return nil
}

func (r *syntheticPerformanceRunner) Run(ctx context.Context, step PerformanceStep, _ int) (PerformanceAdapterResult, error) {
	if err := ctx.Err(); err != nil {
		return PerformanceAdapterResult{}, err
	}
	result, err := r.authorize(ctx, step)
	if err != nil || result.Denied {
		return result, err
	}
	started := time.Now()
	key := step.Binding.digest()
	r.mu.Lock()
	if value, ok := r.cache[key]; ok {
		r.mu.Unlock()
		usage := PerformanceUsage{ServiceNS: time.Since(started).Nanoseconds()}
		result.SemanticDigest = value
		result.Receipt.Usage = usage
		return result, nil
	}
	sourceStarted := time.Now()
	digest := syntheticWorkloadDigest(step.Workload)
	sourceNS := time.Since(sourceStarted).Nanoseconds()
	r.cache[key] = digest
	r.mu.Unlock()
	usage := PerformanceUsage{ServiceNS: time.Since(started).Nanoseconds(), SourceNS: &sourceNS, SourceCalls: 1}
	result.SemanticDigest = digest
	result.Receipt.Usage = usage
	return result, nil
}

func (r *syntheticPerformanceRunner) authorize(ctx context.Context, step PerformanceStep) (PerformanceAdapterResult, error) {
	result := PerformanceAdapterResult{SemanticDigest: syntheticWorkloadDigest(step.Workload), BindingDigest: step.Binding.digest()}
	if step.AuthorityOverride == nil {
		return PerformanceAdapterResult{}, ErrInvalid
	}
	if r.authority == nil {
		return PerformanceAdapterResult{}, ErrMode
	}
	e, err := r.authority(ctx, *step.AuthorityOverride)
	if err != nil {
		return PerformanceAdapterResult{}, err
	}
	a := *step.AuthorityOverride
	err = access.RequireExecution(e, access.Execution{
		Target:       access.Resource{Tenant: a.TargetTenant, Kind: "report", Permission: "execute", ID: a.ReportID},
		Dependencies: []access.Resource{{Tenant: a.TargetTenant, Kind: "source", Permission: "query", ID: a.SourceID}},
		Contexts:     []access.Resource{{Tenant: a.TargetTenant, Kind: "execution_context", Permission: "use", ID: a.ContextID}},
	})
	result.Denied = err != nil
	return result, nil
}

func syntheticWorkloadDigest(workload string) string {
	sum := sha256.Sum256([]byte(workload))
	return hex.EncodeToString(sum[:])
}
