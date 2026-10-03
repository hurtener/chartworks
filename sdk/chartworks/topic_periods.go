package chartworks

import "github.com/hurtener/chartworks/internal/semantics"

// TopicMetricPeriodBindings pins exact reviewed leaf-measure period meanings.
// It supplies no literal time range, SQL or execution authority.
type TopicMetricPeriodBindings = semantics.MetricPeriodBindings

// TopicMetricPeriodBinding maps one leaf measure to a reviewed temporal dimension.
type TopicMetricPeriodBinding = semantics.MetricPeriodBinding

const TopicMetricPeriodBindingsPolicy = semantics.MetricPeriodBindingsPolicy
