package config

import "time"

// QueryBundles bounds opaque BYO context storage and explicit external steps.
// None of these settings grants authority or configures a signing key.
type QueryBundles struct {
	TTL         Duration `json:"ttl"`
	Retention   Duration `json:"retention"`
	MaxBytes    int      `json:"max_bytes"`
	PerSession  int      `json:"per_session"`
	PerTenant   int      `json:"per_tenant"`
	MaxSteps    int      `json:"max_steps"`
	StepTimeout Duration `json:"step_timeout"`
}

// DefaultQueryBundles retains bounded context evidence, not bearer tokens or results.
func DefaultQueryBundles() QueryBundles {
	return QueryBundles{TTL: Duration(15 * time.Minute), Retention: Duration(24 * time.Hour), MaxBytes: 256 << 10, PerSession: 16, PerTenant: 1024, MaxSteps: 8, StepTimeout: Duration(time.Minute)}
}

// ValidateQueryBundles rejects zero, contradictory and unbounded storage/work limits.
func ValidateQueryBundles(v QueryBundles) error {
	if v.TTL < Duration(time.Second) || v.TTL > Duration(time.Hour) || v.Retention < v.TTL || v.Retention > Duration(7*24*time.Hour) || v.MaxBytes < 4096 || v.MaxBytes > 1<<20 || v.PerSession < 1 || v.PerSession > 64 || v.PerTenant < v.PerSession || v.PerTenant > 4096 || v.MaxSteps < 1 || v.MaxSteps > 32 || v.StepTimeout < Duration(time.Second) || v.StepTimeout > Duration(time.Minute) {
		return invalid("query_bundles", "bounded TTL, retention, bytes, counts and step budget required")
	}
	return nil
}
