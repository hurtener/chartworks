// Package workapi exposes only implemented gateway probes and durable work operations.
// It is a thin authenticated surface; domain services independently enforce authority.
package workapi

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
)

// ProbeRequest performs a paid, fixed-input remote role check. It accepts no provider/key,
// prompt, source, actor or arbitrary schema override and never performs analytics SQL.
type ProbeRequest struct {
	Role string `json:"role"`
}
type ProbeResult struct {
	Role       string          `json:"role"`
	OK         bool            `json:"ok"`
	Space      string          `json:"space,omitempty"`
	Dimensions int             `json:"dimensions,omitempty"`
	Receipt    gateway.Receipt `json:"receipt"`
}

// Probe is the first real gateway consumer. Later domain services reuse Engine directly
// with their validated data contexts and operation budgets; this is not a second NLQ API.
func Probe(ctx context.Context, e identity.Envelope, engine gateway.Engine, request ProbeRequest) (ProbeResult, error) {
	out := ProbeResult{Role: request.Role}
	if engine == nil {
		return out, gateway.ErrDisabled
	}
	call, err := gateway.Authorize(e, "ops.model", "operator-role-probe-v1", access.Tenant(e, "use"))
	if err != nil {
		return out, err
	}
	budget, err := gateway.NewBudget(call, gateway.Limits{Calls: 2, Tokens: 131072, Duration: 30 * time.Second})
	if err != nil {
		return out, err
	}
	const sample = "Chartworks remote model connectivity verification."
	switch request.Role {
	case "embedding":
		result, err := engine.Embed(ctx, call, budget, engine.Space(), []string{sample})
		out.Receipt = result.Receipt
		if err != nil {
			return out, err
		}
		out.Space = result.Space
		out.Dimensions = len(result.Vectors[0])
	case "rerank", "visual_rank":
		candidates, err := gateway.AdmitCandidates(call, "ops.model", []gateway.Candidate{{ID: "first", Text: "Remote connectivity verification", Resource: access.Tenant(e, "use")}, {ID: "second", Text: "Unrelated synthetic text", Resource: access.Tenant(e, "use")}})
		if err != nil {
			return out, err
		}
		var result gateway.Ranked
		if request.Role == "rerank" {
			result, err = engine.Rerank(ctx, call, budget, sample, candidates)
		} else {
			result, err = engine.VisualRank(ctx, call, budget, sample, candidates)
		}
		out.Receipt = result.Receipt
		if err != nil {
			return out, err
		}
		if result.Receipt.Warning == "rerank_disabled" || result.Receipt.Warning == "visual_rank_disabled" {
			return out, gateway.ErrDisabled
		}
		if result.Receipt.Warning != "" {
			return out, gateway.ErrUnavailable
		}
	case "enhance", "sqlgen", "sqlfix", "clarify", "pipeline_draft", "profile_summary", "narrative":
		schema, err := gateway.NewSchema("connectivity_check", []byte(`{"type":"object","additionalProperties":false,"required":["summary"],"properties":{"summary":{"type":"string","maxLength":256}}}`))
		if err != nil {
			return out, err
		}
		result, err := engine.Generate(ctx, call, budget, request.Role, "Return a short summary of the supplied synthetic connectivity text in the required JSON. Do not call tools.", sample, schema)
		out.Receipt = result.Receipt
		if err != nil {
			return out, err
		}
	default:
		return out, gateway.ErrInput
	}
	out.OK = true
	return out, nil
}
