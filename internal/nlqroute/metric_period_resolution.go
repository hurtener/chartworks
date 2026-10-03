package nlqroute

import (
	"context"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"sort"
)

func bindMetricPeriodApplications(r *RouteResult, admitted []admittedTopic) error {
	if r.Selection == nil {
		return nil
	}
	constraints := map[string]readexec.BusinessConstraint{}
	for _, c := range r.business {
		constraints[c.Resolution] = c
	}
	consumed := map[string]bool{}
	var apps []MetricPeriodApplication
	for _, pick := range r.Selection.Topics {
		var item *admittedTopic
		for i := range admitted {
			if admitted[i].id == pick.Topic {
				item = &admitted[i]
				break
			}
		}
		if item == nil {
			return readexec.ErrBinding
		}
		for _, root := range pick.Roots {
			if root.Reference.Kind != semantics.KindKPI {
				continue
			}
			for _, k := range item.publication.Definition.KPIs {
				if k.ID != root.Reference.ID || k.Periods == nil {
					continue
				}
				if r.Interpretation == nil || r.SourceBindingDigest == "" {
					return metricPeriodClarification()
				}
				app := MetricPeriodApplication{Policy: MetricPeriodApplicationPolicy, Topic: item.id, KPI: k.ID, TopicVersion: item.publication.State.Version, PackDigest: item.publication.Digest, MappingDigest: readexec.Hash(k.Periods), SourceBindingDigest: r.SourceBindingDigest}
				for _, mapping := range k.Periods.Bindings {
					fact := ""
					for _, m := range item.publication.Definition.Measures {
						if m.ID == mapping.Measure.ID {
							fact = m.Field.Dataset
							break
						}
					}
					if fact == "" {
						return readexec.ErrBinding
					}
					found := false
					for _, t := range r.Interpretation.Temporal {
						if t.Topic != item.id || t.Dimension != mapping.Dimension.ID {
							continue
						}
						c, ok := constraints[t.ID]
						if !ok || found {
							return readexec.ErrBinding
						}
						app.Bindings = append(app.Bindings, MetricPeriodBinding{Measure: mapping.Measure, Dimension: mapping.Dimension, Population: fact, Constraint: c})
						consumed[t.ID] = true
						found = true
					}
					if !found {
						return metricPeriodClarification()
					}
				}
				apps = append(apps, app)
			}
		}
	}
	if len(apps) == 0 {
		return nil
	}
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].Topic != apps[j].Topic {
			return apps[i].Topic < apps[j].Topic
		}
		return apps[i].KPI < apps[j].KPI
	})
	r.metricPeriods = apps
	remaining := make([]readexec.BusinessConstraint, 0, len(r.business))
	for _, c := range r.business {
		if !consumed[c.Resolution] {
			remaining = append(remaining, c)
		}
	}
	r.business = remaining
	return nil
}
func metricPeriodClarification() error {
	return &Clarification{Reason: "metric_period_required", Outcome: semantics.ClarificationMissing, Prompt: "Choose a current period for the reviewed metric time basis."}
}

func (s *Service) ReplayMetricPeriodApplications(ctx context.Context, e identity.Envelope, previous RouteResult) ([]MetricPeriodApplication, string, error) {
	var applications []MetricPeriodApplication
	_, digest, err := s.replayClarifications(ctx, e, previous, &applications)
	return applications, digest, err
}
