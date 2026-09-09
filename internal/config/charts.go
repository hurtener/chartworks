package config

import (
	"time"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/chartservice"
)

// Charts contains no secrets, connection settings or renderer-specific options.
type Charts struct {
	Limits        charts.Limits `json:"limits"`
	MaxConcurrent int           `json:"max_concurrent"`
	Timeout       Duration      `json:"timeout"`
	RankEnabled   bool          `json:"rank_enabled"`
	RankCalls     int           `json:"rank_calls"`
	RankTokens    int           `json:"rank_tokens"`
	RankTimeout   Duration      `json:"rank_timeout"`
}

// DefaultCharts works with inference and source connections disabled.
func DefaultCharts() Charts {
	return Charts{Limits: charts.Defaults(), MaxConcurrent: 8, Timeout: Duration(10 * time.Second), RankCalls: 2, RankTokens: 8192, RankTimeout: Duration(5 * time.Second)}
}

// ServiceOptions projects transport-independent bounds into the actual service.
func (c Charts) ServiceOptions() chartservice.Options {
	return chartservice.Options{Limits: c.Limits, MaxConcurrent: c.MaxConcurrent, Timeout: time.Duration(c.Timeout), RankEnabled: c.RankEnabled, RankCalls: c.RankCalls, RankTokens: c.RankTokens, RankTimeout: time.Duration(c.RankTimeout)}
}
func (c Charts) validate() error {
	if c.ServiceOptions().Validate() != nil {
		return invalid("charts", "bounded limits and rank/operation deadlines required")
	}
	return nil
}
