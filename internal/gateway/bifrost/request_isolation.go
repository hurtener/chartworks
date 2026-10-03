package bifrost

import (
	"context"
	"github.com/maximhq/bifrost/core/schemas"
)

// requestIsolation gives the SDK worker private routing headers. In the pinned
// SDK, timeout handling reads the original request while workers set the model
// on the queued request. PreLLMHook is the supported ownership boundary: the
// queued copy must not share the mutable model/provider struct with that reader.
// Payloads stay immutable, and cancellation, deadlines and routing are unchanged.
type requestIsolation struct{}

func (requestIsolation) GetName() string { return "chartworks-request-isolation-v1" }
func (requestIsolation) Cleanup() error  { return nil }
func (requestIsolation) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	return nil
}
func (requestIsolation) PostLLMHook(_ *schemas.BifrostContext, r *schemas.BifrostResponse, e *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return r, e, nil
}
func (requestIsolation) PreLLMHook(_ *schemas.BifrostContext, in *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if in == nil {
		return nil, isolationInputError(), nil
	}
	out := *in
	switch in.RequestType {
	case schemas.ChatCompletionRequest:
		if in.ChatRequest == nil {
			return in, isolationInputError(), nil
		}
		copy := *in.ChatRequest
		copy.Fallbacks = append([]schemas.Fallback(nil), copy.Fallbacks...)
		out.ChatRequest = &copy
	case schemas.EmbeddingRequest:
		if in.EmbeddingRequest == nil {
			return in, isolationInputError(), nil
		}
		copy := *in.EmbeddingRequest
		copy.Fallbacks = append([]schemas.Fallback(nil), copy.Fallbacks...)
		out.EmbeddingRequest = &copy
	case schemas.RerankRequest:
		if in.RerankRequest == nil {
			return in, isolationInputError(), nil
		}
		copy := *in.RerankRequest
		copy.Fallbacks = append([]schemas.Fallback(nil), copy.Fallbacks...)
		out.RerankRequest = &copy
	default:
		return in, isolationInputError(), nil
	}
	return &out, nil, nil
}
func isolationInputError() *schemas.LLMPluginShortCircuit {
	return &schemas.LLMPluginShortCircuit{Error: &schemas.BifrostError{IsBifrostError: true, Error: &schemas.ErrorField{Message: "unsupported gateway request type"}}}
}

func isolatedBifrostContext(parent context.Context) (*schemas.BifrostContext, context.CancelFunc) {
	ctx, cancel := schemas.NewBifrostContextWithCancel(parent)
	ctx.SetValue(schemas.BifrostContextKeySkipPluginPipeline, false)
	ctx.SetValue(schemas.BifrostContextKeyPassthroughExtraParams, false)
	ctx.SetValue(schemas.BifrostContextKeyUseRawRequestBody, false)
	ctx.SetValue(schemas.BifrostContextKeyLargePayloadMode, false)
	return ctx, cancel
}
