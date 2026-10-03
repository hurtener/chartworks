package nlqexec

import "github.com/hurtener/chartworks/internal/exec"

// Validate the explicit retained render version only after ordinary resource
// admission. An absent generation packet still permits the historical route
// fallback. Neither prompt text nor a version supplies execution authority.
func validateRetainedMetricFormat(q QueryRecord) error {
	generation := q.Generation.Context
	if !generation.MetricFormat.Valid() {
		return exec.ErrBinding
	}
	if q.Route.Context == nil {
		return nil
	}
	if !q.Route.Context.MetricFormat.Valid() {
		return exec.ErrBinding
	}
	if generation.Tier != "" && generation.Question != "" && generation.MetricFormat != q.Route.Context.MetricFormat {
		return exec.ErrBinding
	}
	return nil
}
