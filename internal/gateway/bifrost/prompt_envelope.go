package bifrost

import (
	"context"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/maximhq/bifrost/core/providers/openai"
	"github.com/maximhq/bifrost/core/schemas"
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
	r, route, err := e.role(name)
	if err != nil {
		return gateway.PromptEnvelope{}, err
	}
	// The pinned OpenAI codec (also used by OpenRouter) raises smaller caps.
	// Reject instead of silently granting a larger output allowance.
	if name == "embedding" || name == "rerank" || r.MaxTokens < openai.MinMaxCompletionTokens {
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
	envelope, err := gateway.NewPromptEnvelope(r.Provider, model, effectiveSystem, configuration, schema, e.cfg.Limits.MaxInputBytes, window, protocol, r.MaxTokens)
	if err == nil && route.provider == schemas.OpenRouter {
		envelope = envelope.RequireStructuredParameters()
	}
	return envelope, err
}

var _ gateway.GenerationEnvelopeProvider = (*Engine)(nil)
