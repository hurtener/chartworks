package config

import "time"

// Uploads bounds customer-file parsing and managed workspace operations. The
// workspace itself is an explicitly approved source alias, never the metadata DB.
type Uploads struct {
	Enabled           bool     `json:"enabled"`
	Formats           []string `json:"formats"`
	MaxBytes          int64    `json:"max_bytes"`
	MaxRows           int      `json:"max_rows"`
	MaxColumns        int      `json:"max_columns"`
	MaxCells          int64    `json:"max_cells"`
	MaxCellBytes      int      `json:"max_cell_bytes"`
	MaxExpandedBytes  int64    `json:"max_expanded_bytes"`
	MaxArchiveEntries int      `json:"max_archive_entries"`
	MaxSheets         int      `json:"max_sheets"`
	MaxExpansionRatio int64    `json:"max_expansion_ratio"`
	MaxPageBytes      int64    `json:"max_page_bytes"`
	MaxRowGroupBytes  int64    `json:"max_row_group_bytes"`
	MaxPerTenant      int      `json:"max_per_tenant"`
	MaxTenantBytes    int64    `json:"max_tenant_bytes"`
	Concurrency       int      `json:"concurrency"`
	Timeout           Duration `json:"timeout"`
	StagingTTL        Duration `json:"staging_ttl"`
}

// ProfilePolicy is a data-minimization policy, not an identity grant. Only named
// numeric/temporal fields may retain ranges. No sample values enter model input.
type ProfilePolicy struct {
	ID           string   `json:"id"`
	Tenant       string   `json:"tenant"`
	Source       string   `json:"source"`
	RangeColumns []string `json:"range_columns"`
}

// Profiling controls a bounded, explicitly described sample through the common
// validated-read executor. Returned rows never imply a bounded physical scan.
type Profiling struct {
	Enabled            bool            `json:"enabled"`
	SampleRows         int             `json:"sample_rows"`
	SampleBytes        int             `json:"sample_bytes"`
	PlannerCostCeiling float64         `json:"planner_cost_ceiling"`
	Timeout            Duration        `json:"timeout"`
	FreshFor           Duration        `json:"fresh_for"`
	StaleAfter         Duration        `json:"stale_after"`
	Summaries          bool            `json:"summaries"`
	MaxVersions        int             `json:"max_versions"`
	Policies           []ProfilePolicy `json:"policies"`
}

// DefaultUploads keeps parsing opt-in with the phase-11 file and row ceilings.
func DefaultUploads() Uploads {
	return Uploads{Formats: []string{"csv", "xlsx", "parquet"}, MaxBytes: 100 << 20,
		MaxRows: 1000000, MaxColumns: 256, MaxCells: 4000000, MaxCellBytes: 65536,
		MaxExpandedBytes: 256 << 20, MaxArchiveEntries: 1024, MaxSheets: 32,
		MaxExpansionRatio: 100, MaxPageBytes: 8 << 20, MaxRowGroupBytes: 64 << 20,
		MaxPerTenant: 32, MaxTenantBytes: 1 << 30, Concurrency: 2,
		Timeout: Duration(time.Minute), StagingTTL: Duration(24 * time.Hour)}
}

// DefaultProfiling redacts ranges unless an explicit operational policy permits them.
func DefaultProfiling() Profiling {
	return Profiling{SampleRows: 1000, SampleBytes: 1 << 20, PlannerCostCeiling: 1e7,
		Timeout: Duration(30 * time.Second), FreshFor: Duration(24 * time.Hour),
		StaleAfter: Duration(7 * 24 * time.Hour), MaxVersions: 32, Policies: []ProfilePolicy{}}
}

// Clone detaches upload format configuration from its caller.
func (u Uploads) Clone() Uploads { u.Formats = append([]string(nil), u.Formats...); return u }

// Clone detaches every profile policy and its column list.
func (p Profiling) Clone() Profiling {
	p.Policies = append([]ProfilePolicy{}, p.Policies...)
	for i := range p.Policies {
		p.Policies[i].RangeColumns = append([]string{}, p.Policies[i].RangeColumns...)
	}
	return p
}

// ValidateUploads rejects unbounded allocation, parsing and retention settings.
func ValidateUploads(u Uploads) error {
	if u.MaxBytes < 1 || u.MaxBytes > 100<<20 || u.MaxRows < 1 || u.MaxRows > 1000000 ||
		u.MaxColumns < 1 || u.MaxColumns > 256 || u.MaxCells < 1 || u.MaxCells > 4000000 ||
		u.MaxCellBytes < 1 || u.MaxCellBytes > 65536 || u.MaxExpandedBytes < u.MaxBytes || u.MaxExpandedBytes > 256<<20 ||
		u.MaxArchiveEntries < 1 || u.MaxArchiveEntries > 1024 || u.MaxSheets < 1 || u.MaxSheets > 32 ||
		u.MaxExpansionRatio < 1 || u.MaxExpansionRatio > 100 || u.MaxPageBytes < 1024 || u.MaxPageBytes > 8<<20 ||
		u.MaxRowGroupBytes < u.MaxPageBytes || u.MaxRowGroupBytes > 64<<20 ||
		u.MaxPerTenant < 1 || u.MaxPerTenant > 1000 || u.MaxTenantBytes < u.MaxBytes || u.MaxTenantBytes > 10<<30 ||
		u.Concurrency < 1 || u.Concurrency > 8 || u.Timeout < Duration(time.Millisecond) || u.Timeout > Duration(time.Minute) ||
		u.StagingTTL < Duration(time.Minute) || u.StagingTTL > Duration(7*24*time.Hour) || len(u.Formats) < 1 || len(u.Formats) > 3 {
		return invalid("uploads", "bounds exceeded")
	}
	seen := map[string]bool{}
	for _, f := range u.Formats {
		if (f != "csv" && f != "xlsx" && f != "parquet") || seen[f] {
			return invalid("uploads.formats", "unique qualified formats required")
		}
		seen[f] = true
	}
	return nil
}

// ValidateProfiling is separate from the common executor's immutable ceilings.
func ValidateProfiling(p Profiling) error {
	if p.SampleRows < 1 || p.SampleRows > 100000 || p.SampleBytes < 1024 || p.SampleBytes > 16<<20 ||
		!(p.PlannerCostCeiling > 0 && p.PlannerCostCeiling <= 1e12) || p.Timeout < Duration(time.Millisecond) || p.Timeout > Duration(time.Minute) ||
		p.FreshFor < 0 || p.FreshFor > Duration(365*24*time.Hour) || p.StaleAfter <= p.FreshFor || p.StaleAfter > Duration(10*365*24*time.Hour) ||
		p.MaxVersions < 2 || p.MaxVersions > 1000 || len(p.Policies) > 128 {
		return invalid("profiling", "bounds exceeded")
	}
	seen := map[string]bool{}
	for _, policy := range p.Policies {
		key := policy.Tenant + "/" + policy.ID
		if !sourceCoordinate(policy.ID) || !sourceCoordinate(policy.Tenant) || !sourceCoordinate(policy.Source) || seen[key] || len(policy.RangeColumns) > 256 {
			return invalid("profiling.policies", "bounded unique policy references required")
		}
		seen[key] = true
		columns := map[string]bool{}
		for _, c := range policy.RangeColumns {
			if !sourceSQLName(c) || columns[c] {
				return invalid("profiling.policies", "unique declared columns required")
			}
			columns[c] = true
		}
	}
	return nil
}
