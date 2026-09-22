package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/hurtener/chartworks/internal/chartservice"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/reporting"
)

// LiveInput is protected execution material. It is never retained in reports or logs.
type LiveInput struct {
	Route     *nlqroute.RouteRequest
	Question  *nlqexec.QuestionRequest
	Chart     *chartservice.SelectRequest
	ReportID  string
	ReportRef reporting.Reference
}

// LiveInputResolver must enforce exact tenant/resource/context reach before returning material.
type LiveInputResolver interface {
	ResolveEvaluationInput(context.Context, identity.Envelope, ProtectedRef) (LiveInput, error)
}

// GovernedRunner invokes the existing governed services; it does not introduce a second gateway, validator, or executor.
type GovernedRunner struct {
	Inputs  LiveInputResolver
	Routing *nlqroute.Service
	Query   *nlqexec.Service
	Charts  *chartservice.Service
	Reports *reporting.Service
	Clock   Clock
}

// Observe resolves protected input and calls the owning governed service.
func (g *GovernedRunner) Observe(ctx context.Context, x Execution) (Observation, error) {
	if g == nil || g.Inputs == nil || !x.Envelope.Valid() {
		return Observation{}, ErrMode
	}
	if g.Clock == nil {
		g.Clock = time.Now
	}
	started := g.Clock()
	in, err := g.Inputs.ResolveEvaluationInput(ctx, x.Envelope, x.Case.Input)
	if err != nil {
		return Observation{}, err
	}
	var result any
	var receipt gateway.Receipt
	switch x.Case.Stage {
	case StageRouting:
		if g.Routing == nil || in.Route == nil {
			return Observation{}, ErrMode
		}
		r, e := g.Routing.Route(ctx, x.Envelope, *in.Route)
		result, err = r, e
		receipt.Calls = append(receipt.Calls, r.RemoteCalls...)
	case StageContext:
		if g.Query == nil || in.Question == nil {
			return Observation{}, ErrMode
		}
		r, e := g.Query.Preflight(ctx, x.Envelope, nlqexec.PreflightRequest{QuestionRequest: *in.Question})
		result, err = r, e
		receipt.Calls = append(receipt.Calls, r.Route.RemoteCalls...)
	case StageSQL, StageValidation, StageReplay, StageShadow:
		if g.Query == nil || in.Question == nil {
			return Observation{}, ErrMode
		}
		r, e := g.Query.Plan(ctx, x.Envelope, nlqexec.PlanRequest{QuestionRequest: *in.Question})
		result, err = r, e
		receipt = r.Receipt
	case StageChart:
		if g.Charts == nil || in.Chart == nil {
			return Observation{}, ErrMode
		}
		r, e := g.Charts.Select(ctx, x.Envelope, *in.Chart)
		result, err = r, e
		receipt = r.Provenance.Receipt
	case StageReport, StageConsumer:
		if g.Reports == nil || !identifier(in.ReportID) {
			return Observation{}, ErrMode
		}
		r, e := g.Reports.Read(ctx, x.Envelope, in.ReportID, in.ReportRef)
		result, err = r, e
	default:
		return Observation{}, ErrMode
	}
	if err != nil {
		return Observation{}, err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return Observation{}, ErrInvalid
	}
	sum := sha256.Sum256(raw)
	usage := gatewayUsage(receipt)
	usage.ServiceMS = g.Clock().Sub(started).Milliseconds()
	usage.Calls++
	return Observation{Decision: "completed", SemanticDigest: hex.EncodeToString(sum[:]), Usage: usage}, nil
}
func gatewayUsage(r gateway.Receipt) Usage {
	u := Usage{}
	tokens, cost := 0, 0.0
	tokensKnown, costKnown := true, true
	for _, c := range r.Calls {
		u.Calls += c.Attempts
		if c.Attempts > 1 {
			u.Retries += c.Attempts - 1
		}
		if c.InputTokens == nil || c.OutputTokens == nil {
			tokensKnown = false
		} else {
			tokens += *c.InputTokens + *c.OutputTokens
		}
		if c.CostUSD == nil {
			costKnown = false
		} else {
			cost += *c.CostUSD
		}
		if u.ModelMS == nil {
			v := c.DurationMS
			u.ModelMS = &v
		} else {
			*u.ModelMS += c.DurationMS
		}
	}
	if tokensKnown {
		u.Tokens = &tokens
	}
	if costKnown {
		u.CostUSD = &cost
	}
	return u
}
