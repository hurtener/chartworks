package config

// ReportingViewer bounds one selected, authorized retained-result projection.
// It grants no access and never increases a source or model execution budget.
type ReportingViewer struct {
	MaxMessageBytes int `json:"max_message_bytes"`
	MaxRows         int `json:"max_rows"`
	PageRows        int `json:"page_rows"`
	MaxOutputs      int `json:"max_outputs"`
	MaxPoints       int `json:"max_points"`
}

// DefaultReportingViewer keeps pages small while preserving complete charts.
func DefaultReportingViewer() ReportingViewer {
	return ReportingViewer{MaxMessageBytes: 2 << 20, MaxRows: 500, PageRows: 100, MaxOutputs: 32, MaxPoints: 5000}
}

// Validate rejects zero-as-unlimited and bounds both provider and component work.
func (c ReportingViewer) Validate() error {
	if c.MaxMessageBytes < 16384 || c.MaxMessageBytes > 4<<20 || c.MaxRows < 1 || c.MaxRows > 1000 || c.PageRows < 1 || c.PageRows > c.MaxRows || c.MaxOutputs < 1 || c.MaxOutputs > 64 || c.MaxPoints < 1 || c.MaxPoints > 10000 {
		return invalid("reporting.viewer", "bounded message, output, point and page limits required")
	}
	return nil
}
