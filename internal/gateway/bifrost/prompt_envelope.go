package bifrost

import (
	"context"

	"github.com/hurtener/chartworks/internal/gateway"
)

// GenerationEnvelope resolves the effective model/system before local packet
// fitting. It never invokes Bifrost, reserves budget or grants source authority.
func (e *Engine) GenerationEnvelope(ctx context.Context, name, system string, schema *gateway.Schema) (gateway.PromptEnvelope, error) {
	if ctx == nil {
		return gateway.PromptEnvelope{}, gateway.ErrInput
	}
	if err := ctx.Err(); err != nil {
		return gateway.PromptEnvelope{}, err
	}
	r, _, err := e.role(name)
	if err != nil {
		return gateway.PromptEnvelope{}, err
	}
	if name == "embedding" || name == "rerank" {
		return gateway.PromptEnvelope{}, gateway.ErrInput
	}
	model, effectiveSystem, configuration := gateway.ApplyRuntimeConfig(ctx, name, r.Model, system)
	window, protocol := 0, 1024
	if len(e.cfg.ModelWindows) != 0 {
		found := false
		for _, policy := range e.cfg.ModelWindows {
			if policy.Provider == r.Provider && policy.Model == model {
				window, protocol, found = policy.ContextTokens, policy.ProtocolReserveTokens, true
				break
			}
		}
		// Never apply the old model's larger window to a runtime override.
		if !found {
			return gateway.PromptEnvelope{}, gateway.ErrInput
		}
	}
	return gateway.NewPromptEnvelope(r.Provider, model, effectiveSystem, configuration, schema, e.cfg.Limits.MaxInputBytes, window, protocol, r.MaxTokens)
}

var _ gateway.GenerationEnvelopeProvider = (*Engine)(nil)
