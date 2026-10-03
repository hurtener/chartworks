package reporting

import "github.com/hurtener/chartworks/internal/exec"

// StatisticalNarrativePolicyVersion is opt-in. Older policies and migration
// defaults retain their original serialized evidence and deterministic wording.
const StatisticalNarrativePolicyVersion = "bounded-narrative-v3"
const StatisticalNarrativeSchemaVersion = "grounded-narrative-v2"

// NarrativeFieldRef binds a reviewed coordinate, including adapter-owned type
// identity. A field name or a display label alone is not a statistical binding.
type NarrativeFieldRef struct {
	Column int        `json:"column"`
	Field  exec.Field `json:"field"`
}

// NarrativeTimeOrder always orders ascending. It deliberately excludes local
// wall timestamps, partial dates, guessed timezones and inferred sampling grain.
type NarrativeTimeOrder struct {
	Field   NarrativeFieldRef `json:"field"`
	Meaning string            `json:"meaning" jsonschema:"enum=date,enum=instant"`
}

// NarrativeStatistic describes one series of already-retained observations.
// No expression, grouping, imputation or source query can be supplied.
type NarrativeStatistic struct {
	ID    string              `json:"id"`
	Kind  string              `json:"kind" jsonschema:"enum=trend,enum=extrema,enum=population_variance"`
	Value NarrativeFieldRef   `json:"value"`
	Time  *NarrativeTimeOrder `json:"time,omitempty"`
}

// NarrativePopulation preserves source-row ordinals after temporal ordering.
// NullRows and SourceRows partition the considered prefix; neither is a claim
// about all rows in a warehouse or an unobserved portion of a result.
type NarrativePopulation struct {
	Scope          string `json:"scope" jsonschema:"enum=retained_row_prefix"`
	RetainedRows   int    `json:"retained_rows"`
	ConsideredRows int    `json:"considered_rows"`
	IncludedRows   int    `json:"included_rows"`
	SourceRows     []int  `json:"source_rows"`
	NullRows       []int  `json:"null_rows"`
	QueryOutcome   string `json:"query_outcome" jsonschema:"enum=succeeded,enum=empty,enum=truncated"`
	Truncation     string `json:"truncation,omitempty"`
	RowsReduced    bool   `json:"rows_reduced"`
}

type NarrativeTrendEvidence struct {
	FirstRow   int    `json:"first_row"`
	LastRow    int    `json:"last_row"`
	FirstTime  string `json:"first_time"`
	LastTime   string `json:"last_time"`
	FirstValue string `json:"first_value"`
	LastValue  string `json:"last_value"`
	Difference string `json:"difference"`
	Sequence   string `json:"sequence" jsonschema:"enum=increasing,enum=decreasing,enum=constant,enum=nondecreasing,enum=nonincreasing,enum=mixed"`
}

type NarrativeExtremaEvidence struct {
	Minimum     string `json:"minimum"`
	Maximum     string `json:"maximum"`
	MinimumRow  int    `json:"minimum_row"`
	MaximumRow  int    `json:"maximum_row"`
	MinimumTies int    `json:"minimum_ties"`
	MaximumTies int    `json:"maximum_ties"`
}

// NarrativeVarianceEvidence is an exact reduced nonnegative fraction. Divisor
// is the included population count, not the sample-variance N-1 convention.
type NarrativeVarianceEvidence struct {
	Numerator   string `json:"numerator"`
	Denominator string `json:"denominator"`
	Divisor     int    `json:"divisor"`
}

// NarrativeAmountEvidence reveals only an egress-approved completeness status.
// It never copies raw count companions or changes their count/value roles.
type NarrativeAmountEvidence struct {
	Declaration string            `json:"declaration"`
	Label       string            `json:"label"`
	Status      string            `json:"status" jsonschema:"enum=complete,enum=incomplete,enum=unknown"`
	Scope       string            `json:"scope" jsonschema:"enum=retained_row_prefix"`
	Companion   NarrativeFieldRef `json:"companion"`
}

// NarrativeStatisticEvidence is derived locally and checked against the
// repository's retained result on write/read. Exactly one calculation is set.
type NarrativeStatisticEvidence struct {
	ID         string                     `json:"id"`
	Output     string                     `json:"output"`
	Kind       string                     `json:"kind" jsonschema:"enum=trend,enum=extrema,enum=population_variance"`
	Value      NarrativeFieldRef          `json:"value"`
	Time       *NarrativeTimeOrder        `json:"time,omitempty"`
	Population NarrativePopulation        `json:"population"`
	Trend      *NarrativeTrendEvidence    `json:"trend,omitempty"`
	Extrema    *NarrativeExtremaEvidence  `json:"extrema,omitempty"`
	Variance   *NarrativeVarianceEvidence `json:"variance,omitempty"`
	Amount     *NarrativeAmountEvidence   `json:"amount,omitempty"`
	Provenance string                     `json:"provenance"`
}
