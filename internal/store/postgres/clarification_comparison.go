package postgres

import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
)

func clarificationComparisonJSON(cases []semantics.ClarificationEvaluation, pin rulesets.Evaluation) ([]byte, error) {
	if len(cases) > 16 {
		return nil, store.ErrInvalid
	}
	for _, evaluation := range cases {
		if evaluation.SchemaVersion != semantics.ClarificationSchemaVersion {
			return nil, store.ErrInvalid
		}
		for _, r := range evaluation.Resolutions {
			if r.Topic != pin.Topic || r.TopicVersion != pin.TopicVersion || r.RulesetVersion != pin.RuleVersion || r.RulesetDigest != pin.RuleDigest || r.PackDigest != pin.PackDigest || (r.Sensitivity != semantics.LiteralSensitive && r.Sensitivity != semantics.LiteralNonSensitive) {
				return nil, store.ErrInvalid
			}
		}
	}
	raw, err := json.Marshal(cases)
	if err != nil || len(raw) > 1<<20 {
		return nil, store.ErrInvalid
	}
	return raw, nil
}
