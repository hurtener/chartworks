package config

import "time"

// Autopilot bounds reviewed L2 proposal work. Enablement and operational budgets
// do not grant propose, review, publication, managed-write or compensation rights.
// L3 policy and arbitrary workflow/custom-code settings are intentionally absent.
type Autopilot struct {
	Enabled                bool     `json:"enabled"`
	MaxSteps               int      `json:"max_steps"`
	MaxCalls               int      `json:"max_calls"`
	MaxTokens              int      `json:"max_tokens"`
	Timeout                Duration `json:"timeout"`
	MaxProposalBytes       int      `json:"max_proposal_bytes"`
	MaxEvidenceBytes       int      `json:"max_evidence_bytes"`
	MaxProposals           int      `json:"max_proposals"`
	AmendmentDedupInterval Duration `json:"amendment_dedup_interval"`
	ModelVersion           string   `json:"model_version"`
}

// DefaultAutopilot is disabled until an operator configures the existing model
// gateway and managed pipeline runner. Review/read remain explicit signed actions.
func DefaultAutopilot() Autopilot {
	return Autopilot{MaxSteps: 8, MaxCalls: 4, MaxTokens: 65536, Timeout: Duration(time.Minute), MaxProposalBytes: 512 << 10, MaxEvidenceBytes: 64 << 10, MaxProposals: 10000, AmendmentDedupInterval: Duration(time.Hour)}
}

// Validate applies the same limits to configuration decoding and constructors.
func (c Autopilot) Validate() error {
	if c.MaxSteps < 1 || c.MaxSteps > 16 || c.MaxCalls < 2 || c.MaxCalls > 8 || c.MaxTokens < 256 || c.MaxTokens > 128<<10 ||
		time.Duration(c.Timeout) < time.Second || time.Duration(c.Timeout) > time.Minute || c.MaxProposalBytes < 4096 || c.MaxProposalBytes > 1<<20 ||
		c.MaxEvidenceBytes < 1024 || c.MaxEvidenceBytes > 128<<10 || c.MaxEvidenceBytes > c.MaxProposalBytes || c.MaxProposals < 1 || c.MaxProposals > 100000 ||
		time.Duration(c.AmendmentDedupInterval) < time.Minute || time.Duration(c.AmendmentDedupInterval) > 7*24*time.Hour || len(c.ModelVersion) > 256 || c.Enabled && c.ModelVersion == "" {
		return invalid("autopilot", "bounded reviewed proposal budgets and a configured model version required")
	}
	for _, r := range c.ModelVersion {
		if r < 33 || r > 126 {
			return invalid("autopilot.model_version", "printable version without whitespace required")
		}
	}
	return nil
}
