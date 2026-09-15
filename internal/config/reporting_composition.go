package config

import "time"

// ReportingComposition bounds reports and dashboards. Enablement is not a grant:
// each dynamic widget still requires the caller's current query authority.
type ReportingComposition struct {
	MaxWidgets         int      `json:"max_widgets"`
	MaxFilters         int      `json:"max_filters"`
	MaxPages           int      `json:"max_pages"`
	MaxTextBytes       int      `json:"max_text_bytes"`
	MaxDefinitionBytes int      `json:"max_definition_bytes"`
	MaxQueries         int      `json:"max_queries"`
	MaxRetainedBytes   int      `json:"max_retained_bytes"`
	LiveQueries        bool     `json:"live_queries"`
	SessionBound       bool     `json:"session_bound"`
	PartialFailure     string   `json:"partial_failure"`
	Timeout            Duration `json:"timeout"`
}

// DefaultReportingComposition leaves both dynamic execution lanes disabled.
func DefaultReportingComposition() ReportingComposition {
	return ReportingComposition{
		MaxWidgets: 100, MaxFilters: 100, MaxPages: 100,
		MaxTextBytes: 32 << 10, MaxDefinitionBytes: 1 << 20,
		MaxQueries: 16, MaxRetainedBytes: 8 << 20,
		PartialFailure: "fail_closed", Timeout: Duration(time.Minute),
	}
}

// Validate applies the same bounds to configuration and in-process consumers.
func (c ReportingComposition) Validate() error {
	if c.MaxWidgets < 1 || c.MaxWidgets > 100 || c.MaxFilters < 1 || c.MaxFilters > 100 ||
		c.MaxPages < 1 || c.MaxPages > 100 || c.MaxTextBytes < 1 || c.MaxTextBytes > 128<<10 ||
		c.MaxDefinitionBytes < c.MaxTextBytes || c.MaxDefinitionBytes > 2<<20 ||
		c.MaxQueries < 1 || c.MaxQueries > 100 || c.MaxRetainedBytes < 1024 || c.MaxRetainedBytes > 16<<20 ||
		(c.PartialFailure != "fail_closed" && c.PartialFailure != "allow_partial") ||
		time.Duration(c.Timeout) < time.Second || time.Duration(c.Timeout) > time.Minute ||
		(c.SessionBound && !c.LiveQueries) {
		return invalid("reporting.composition", "bounded composition limits and explicit dynamic execution policy required")
	}
	return nil
}
