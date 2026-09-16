package reporting

import (
	"encoding/json"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
)

// QueryLimits are authored ceilings, not scan/cost guarantees. Zero members
// inherit the accepted deployment ceiling; positive members can only narrow it.
// Attempts counts physical reads, independently of queue/authority retries.
type QueryLimits struct {
	MaxRows       int `json:"max_rows"`
	MaxBytes      int `json:"max_bytes"`
	TimeoutMillis int `json:"timeout_ms"`
	QueryAttempts int `json:"query_attempts"`
}

func (q QueryLimits) valid() bool {
	return q.MaxRows >= 0 && q.MaxRows <= 10000 && (q.MaxBytes == 0 || q.MaxBytes >= 1024 && q.MaxBytes <= 4<<20) &&
		(q.TimeoutMillis == 0 || q.TimeoutMillis >= 1000 && q.TimeoutMillis <= 60000) && q.QueryAttempts >= 0 && q.QueryAttempts <= 3
}

func lower(current, requested int) int {
	if requested > 0 {
		return min(current, requested)
	}
	return current
}

func resolveQueryLimits(deployment config.ReportingExecution, attempts int, authored ...*QueryLimits) (QueryLimits, error) {
	if deployment.Validate() != nil || attempts < 1 || attempts > 3 {
		return QueryLimits{}, ErrInvalid
	}
	out := QueryLimits{MaxRows: deployment.MaxRows, MaxBytes: deployment.MaxResultBytes, TimeoutMillis: int(time.Duration(deployment.Timeout) / time.Millisecond), QueryAttempts: attempts}
	for _, q := range authored {
		if q == nil {
			continue
		}
		if !q.valid() {
			return QueryLimits{}, ErrInvalid
		}
		out.MaxRows = lower(out.MaxRows, q.MaxRows)
		out.MaxBytes = lower(out.MaxBytes, q.MaxBytes)
		out.TimeoutMillis = lower(out.TimeoutMillis, q.TimeoutMillis)
		out.QueryAttempts = lower(out.QueryAttempts, q.QueryAttempts)
	}
	return out, nil
}

func limitsForQuery(limits config.ReportingExecution, q QueryLimits) config.ReportingExecution {
	limits.MaxRows = min(limits.MaxRows, q.MaxRows)
	limits.MaxResultBytes = min(limits.MaxResultBytes, q.MaxBytes)
	limits.Timeout = config.Duration(min(time.Duration(limits.Timeout), time.Duration(q.TimeoutMillis)*time.Millisecond))
	limits.PageRows = min(limits.PageRows, limits.MaxRows)
	limits.NarrativeTimeout = min(limits.NarrativeTimeout, limits.Timeout)
	return limits
}

// RuntimeQueryLimits intersects the original accepted caps with the current
// deployment. It never alters the accepted manifest, selection or reuse key.
func RuntimeQueryLimits(m RunManifest, current config.ReportingExecution) (QueryLimits, error) {
	saved := QueryLimits{MaxRows: m.Limits.MaxRows, MaxBytes: m.Limits.MaxResultBytes, TimeoutMillis: int(time.Duration(m.Limits.Timeout) / time.Millisecond), QueryAttempts: 3}
	return resolveQueryLimits(current, 3, &saved, m.QueryLimits)
}

// CheckRuntimeResult is used before consuming an intermediate result or
// committing reuse. A lower runtime ceiling cannot be bypassed by old evidence.
// Rejection does not authorize regenerating or re-querying that evidence.
func CheckRuntimeResult(m RunManifest, current config.ReportingExecution, result exec.Result) error {
	q, err := RuntimeQueryLimits(m, current)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(result)
	if err != nil || len(result.Rows) > q.MaxRows || result.Bytes > q.MaxBytes || len(raw) > q.MaxBytes {
		return ErrBudget
	}
	return nil
}

func maxClaims(n Narrative) int {
	if n.MaxClaims == 0 {
		return 32
	}
	return n.MaxClaims
}

// intersectExecutionLimits keeps the accepted parent ceilings through delayed
// child admission and retries. Current deployment can narrow, never widen them.
func intersectExecutionLimits(accepted, current config.ReportingExecution) config.ReportingExecution {
	out := accepted
	out.Retention = min(accepted.Retention, current.Retention)
	out.PreviewRetention = min(accepted.PreviewRetention, current.PreviewRetention, out.Retention)
	out.Timeout = min(accepted.Timeout, current.Timeout)
	out.MaxRows = min(accepted.MaxRows, current.MaxRows)
	out.MaxResultBytes = min(accepted.MaxResultBytes, current.MaxResultBytes)
	out.MaxArtifactBytes = min(accepted.MaxArtifactBytes, current.MaxArtifactBytes)
	out.MaxTenantBytes = min(accepted.MaxTenantBytes, current.MaxTenantBytes)
	out.MaxRequests = min(accepted.MaxRequests, current.MaxRequests)
	out.PageRows = min(accepted.PageRows, current.PageRows, out.MaxRows)
	out.MaxReuseAge = min(accepted.MaxReuseAge, current.MaxReuseAge, out.Retention)
	out.NarrativeCalls = min(accepted.NarrativeCalls, current.NarrativeCalls)
	out.NarrativeTokens = min(accepted.NarrativeTokens, current.NarrativeTokens)
	out.NarrativeTimeout = min(accepted.NarrativeTimeout, current.NarrativeTimeout, out.Timeout)
	// The accepted model identity is not silently replaced by a later deployment.
	return out
}
