package config

import "time"

// ReportingExecution bounds retained frozen runs. None of these operational
// settings grants source, preview, publication or artifact-reading authority.
type ReportingExecution struct {
	Retention         Duration `json:"retention"`
	PreviewRetention  Duration `json:"preview_retention"`
	Timeout           Duration `json:"timeout"`
	MaxRows           int      `json:"max_rows"`
	MaxResultBytes    int      `json:"max_result_bytes"`
	MaxArtifactBytes  int      `json:"max_artifact_bytes"`
	MaxTenantBytes    int64    `json:"max_tenant_bytes"`
	MaxRequests       int      `json:"max_requests"`
	PageRows          int      `json:"page_rows"`
	MaxReuseAge       Duration `json:"max_reuse_age"`
	NarrativeCalls    int      `json:"narrative_calls"`
	NarrativeTokens   int      `json:"narrative_tokens"`
	NarrativeTimeout  Duration `json:"narrative_timeout"`
}

// DefaultReportingExecution retains public values for seven days and private
// previews for one day. Reuse is still explicit in each admitted request.
func DefaultReportingExecution() ReportingExecution {
	return ReportingExecution{
		Retention: Duration(7 * 24 * time.Hour), PreviewRetention: Duration(24 * time.Hour),
		Timeout: Duration(time.Minute), MaxRows: 1000, MaxResultBytes: 1 << 20,
		MaxArtifactBytes: 16 << 20, MaxTenantBytes: 256 << 20, MaxRequests: 20000,
		PageRows: 200, MaxReuseAge: Duration(time.Hour), NarrativeCalls: 4,
		NarrativeTokens: 32768, NarrativeTimeout: Duration(15 * time.Second),
	}
}

// Validate also protects direct in-process construction from unbounded work.
func (c ReportingExecution) Validate() error {
	if time.Duration(c.Retention) < time.Minute || time.Duration(c.Retention) > 90*24*time.Hour ||
		time.Duration(c.PreviewRetention) < time.Minute || c.PreviewRetention > c.Retention || time.Duration(c.PreviewRetention) > 7*24*time.Hour ||
		time.Duration(c.Timeout) < time.Second || time.Duration(c.Timeout) > time.Minute ||
		c.MaxRows < 1 || c.MaxRows > 10000 || c.MaxResultBytes < 1024 || c.MaxResultBytes > 4<<20 ||
		c.MaxArtifactBytes < c.MaxResultBytes || c.MaxArtifactBytes > 64<<20 ||
		c.MaxTenantBytes < int64(c.MaxArtifactBytes) || c.MaxTenantBytes > 1<<40 ||
		c.MaxRequests < 1 || c.MaxRequests > 100000 || c.PageRows < 1 || c.PageRows > 1000 || c.PageRows > c.MaxRows ||
		time.Duration(c.MaxReuseAge) < time.Second || time.Duration(c.MaxReuseAge) > 24*time.Hour || c.MaxReuseAge > c.Retention ||
		c.NarrativeCalls < 1 || c.NarrativeCalls > 8 || c.NarrativeTokens < 64 || c.NarrativeTokens > 128<<10 ||
		time.Duration(c.NarrativeTimeout) < 100*time.Millisecond || c.NarrativeTimeout > c.Timeout {
		return invalid("reporting.execution", "bounded retention, result, paging, reuse and narrative limits required")
	}
	return nil
}
