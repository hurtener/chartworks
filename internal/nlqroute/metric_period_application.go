package nlqroute

import readexec "github.com/hurtener/chartworks/internal/exec"

// ResolvedMetricPeriodApplications exposes only authenticated in-process
// placement. JSON decoding cannot reconstruct this private seal or grant scope.
func (r RouteResult) ResolvedMetricPeriodApplications() ([]MetricPeriodApplication, error) {
	if _, err := r.ResolvedBusinessConstraints(); err != nil {
		return nil, err
	}
	if len(r.metricPeriods) == 0 {
		return nil, nil
	}
	if r.Interpretation == nil || r.SourceBindingDigest == "" {
		return nil, readexec.ErrBinding
	}
	out := make([]MetricPeriodApplication, len(r.metricPeriods))
	for i, p := range r.metricPeriods {
		if p.Policy != MetricPeriodApplicationPolicy || p.SourceBindingDigest != r.SourceBindingDigest || len(p.Bindings) == 0 || len(p.Bindings) > 4 {
			return nil, readexec.ErrBinding
		}
		out[i] = p
		out[i].Bindings = append([]MetricPeriodBinding(nil), p.Bindings...)
		for _, b := range p.Bindings {
			found := false
			for _, t := range r.Interpretation.Temporal {
				c := b.Constraint
				if t.ID == c.Resolution && t.Topic == p.Topic && t.Dimension == b.Dimension.ID && t.Dataset == c.Dataset && t.Column == c.Column && t.Start == c.Value && t.End == c.Upper && t.Grain == c.Grain && t.TimeZone == c.TimeZone && t.TemporalType == c.TemporalType && t.Calendar == c.Calendar && c.Kind == "time_window" && c.Operator == "range" && c.Bounds == "[)" && c.Nulls == "exclude" {
					found = true
					break
				}
			}
			if !found {
				return nil, readexec.ErrBinding
			}
		}
	}
	return out, nil
}
