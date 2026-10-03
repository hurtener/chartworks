package nlqroute

import (
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
)

const MetricPeriodApplicationPolicy = "current-reviewed-metric-period-v1"

// MetricPeriodApplication is released only by authenticated routing or replay.
// Its serialized shape alone is not a seal or a source capability. Bindings
// contain private current intervals and must never enter model guidance.
type MetricPeriodApplication struct {
	Policy              string                `json:"policy"`
	Topic               string                `json:"topic"`
	KPI                 string                `json:"kpi"`
	TopicVersion        string                `json:"topic_version"`
	PackDigest          string                `json:"pack_digest"`
	MappingDigest       string                `json:"mapping_digest"`
	SourceBindingDigest string                `json:"source_binding_digest"`
	Bindings            []MetricPeriodBinding `json:"bindings"`
}

type MetricPeriodBinding struct {
	Measure    semantics.Reference     `json:"measure"`
	Dimension  semantics.Reference     `json:"dimension"`
	Population string                  `json:"population"`
	Constraint exec.BusinessConstraint `json:"constraint"`
}

func (MetricPeriodApplication) String() string     { return "metric-period-application(redacted)" }
func (p MetricPeriodApplication) GoString() string { return p.String() }
