package config

import "time"

// Onboarding bounds the durable coordinator. It stores no credential or token.
type Onboarding struct {
	Enabled       bool     `json:"enabled"`
	MaxStages     int      `json:"max_stages"`
	MaxModelCalls int      `json:"max_model_calls"`
	MaxTokens     int      `json:"max_tokens"`
	MaxEntities   int      `json:"max_entities"`
	MaxDuration   Duration `json:"max_duration"`
}

func DefaultOnboarding() Onboarding {
	return Onboarding{Enabled: true, MaxStages: 12, MaxModelCalls: 4, MaxTokens: 24000, MaxEntities: 256, MaxDuration: Duration(20 * time.Minute)}
}
func ValidateOnboarding(v Onboarding) error {
	if v.MaxStages < 6 || v.MaxStages > 32 || v.MaxModelCalls < 0 || v.MaxModelCalls > 16 || v.MaxTokens < 0 || v.MaxTokens > 200000 || v.MaxEntities < 1 || v.MaxEntities > 2048 || v.MaxDuration < Duration(time.Minute) || v.MaxDuration > Duration(2*time.Hour) {
		return invalid("onboarding", "bounded stage, model, token, entity and duration limits required")
	}
	return nil
}
