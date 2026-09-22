package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/chartservice"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

// LiveInput is protected execution material. It is never retained in reports or logs.
type LiveInput struct {
	Pack      PackRevision                `json:"pack"`
	Route     *nlqroute.RouteRequest      `json:"route,omitempty"`
	Question  *nlqexec.QuestionRequest    `json:"question,omitempty"`
	Run       *nlqexec.RunRequest         `json:"run,omitempty"`
	Replay    *nlqexec.SavedQuestion      `json:"replay,omitempty"`
	Shadow    *ShadowInput                `json:"shadow,omitempty"`
	BYO       *nlqbyo.SubmitRequest       `json:"byo,omitempty"`
	Chart     *chartservice.SelectRequest `json:"chart,omitempty"`
	ReportID  string                      `json:"report_id,omitempty"`
	ReportRef reporting.Reference         `json:"report_ref,omitempty"`
}

// ShadowInput compares two retained definitions without fresh planning or provider work.
type ShadowInput struct {
	Baseline  nlqexec.SavedQuestion `json:"baseline"`
	Candidate nlqexec.SavedQuestion `json:"candidate"`
}

// LiveInputResolver must enforce exact tenant/resource/context reach before returning material.
type LiveInputResolver interface {
	ResolveEvaluationInput(context.Context, identity.Envelope, ProtectedRef) (LiveInput, error)
}

// SavedInspector reads retained query evidence without model or warehouse work.
type SavedInspector interface {
	InspectSaved(context.Context, identity.Envelope, nlqexec.SavedQuestion) (nlqexec.SavedEvidence, error)
}

// GovernedRunner invokes the existing governed services; it does not introduce a second gateway, validator, or executor.
type GovernedRunner struct {
	Inputs  LiveInputResolver
	Routing *nlqroute.Service
	Query   *nlqexec.Service
	Saved   SavedInspector
	Charts  *chartservice.Service
	Reports *reporting.Service
	BYO     *nlqbyo.Service
	Clock   Clock
}

// Observe resolves protected input and calls the owning governed service.
func (g *GovernedRunner) Observe(ctx context.Context, x Execution) (Observation, error) {
	if g == nil || g.Inputs == nil || !x.Envelope.Valid() {
		return Observation{}, ErrMode
	}
	clock := g.Clock
	if clock == nil {
		clock = time.Now
	}
	started := clock()
	reservation := &gatewayReservation{limit: x.Reservation}
	ctx, err := gateway.WithAttemptReservation(ctx, reservation.reserve)
	if err != nil {
		return Observation{}, err
	}
	in, err := g.Inputs.ResolveEvaluationInput(ctx, x.Envelope, x.Case.Input)
	if err != nil {
		return Observation{Usage: reservation.usage()}, err
	}
	wantPack, _ := digest(x.Pack)
	gotPack, _ := digest(in.Pack)
	if !validPack(in.Pack) || wantPack != gotPack {
		return Observation{Usage: reservation.usage()}, ErrReview
	}
	var result any
	var receipt gateway.Receipt
	var sourceMS *int64
	blocked := false
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
	case StageSQL, StageValidation:
		if g.Query == nil || in.Question == nil {
			return Observation{}, ErrMode
		}
		r, e := g.Query.Plan(ctx, x.Envelope, nlqexec.PlanRequest{QuestionRequest: *in.Question})
		result, err = r, e
		receipt = r.Receipt
	case StageReplay:
		saved := g.Saved
		if saved == nil {
			saved = g.Query
		}
		if saved == nil || in.Replay == nil {
			return Observation{}, ErrMode
		}
		result, err = saved.InspectSaved(ctx, x.Envelope, *in.Replay)
	case StageShadow:
		saved := g.Saved
		if saved == nil {
			saved = g.Query
		}
		if saved == nil || in.Shadow == nil {
			return Observation{}, ErrMode
		}
		baseline, e := saved.InspectSaved(ctx, x.Envelope, in.Shadow.Baseline)
		if e != nil {
			err = e
			break
		}
		candidate, e := saved.InspectSaved(ctx, x.Envelope, in.Shadow.Candidate)
		result, err = struct{ Baseline, Candidate nlqexec.SavedEvidence }{baseline, candidate}, e
	case StageAdversarial:
		switch x.Case.Category {
		case "identity_scope":
			if g.Reports == nil || !identifier(in.ReportID) {
				return Observation{}, ErrMode
			}
			result, err = g.Reports.Read(ctx, x.Envelope, in.ReportID, in.ReportRef)
			blocked = adversarialDenied(err)
		case "injection", "dialect_escape", "resource_exhaustion":
			if g.Query == nil || in.Question == nil {
				return Observation{}, ErrMode
			}
			result, err = g.Query.Plan(ctx, x.Envelope, nlqexec.PlanRequest{QuestionRequest: *in.Question})
			blocked = adversarialDenied(err)
		case "byo":
			if g.BYO == nil || in.BYO == nil {
				return Observation{}, ErrMode
			}
			result, err = g.BYO.Submit(ctx, x.Envelope, *in.BYO)
			blocked = adversarialDenied(err)
		case "frozen_report":
			if g.Reports == nil || !identifier(in.ReportID) {
				return Observation{}, ErrMode
			}
			result, err = g.Reports.Read(ctx, x.Envelope, in.ReportID, in.ReportRef)
			blocked = err == nil
		default:
			return Observation{}, ErrMode
		}
		if blocked && err != nil {
			result, err = struct {
				Denied string `json:"denied"`
			}{x.Case.Category}, nil
		}
	case StageChart:
		if g.Charts == nil || in.Chart == nil {
			return Observation{}, ErrMode
		}
		r, e := g.Charts.Select(ctx, x.Envelope, *in.Chart)
		result, err = r, e
		receipt = r.Provenance.Receipt
	case StageConsumer:
		if g.Query == nil || in.Run == nil {
			return Observation{}, ErrMode
		}
		r, e := g.Query.Run(ctx, x.Envelope, *in.Run)
		result, err = r, e
		if r.Execution.Attempt.Finished != nil {
			v := r.Execution.Attempt.Finished.Sub(r.Execution.Attempt.Created).Milliseconds()
			if v < 0 {
				v = 0
			}
			sourceMS = &v
		}
	case StageReport:
		if g.Reports == nil || !identifier(in.ReportID) {
			return Observation{}, ErrMode
		}
		r, e := g.Reports.Read(ctx, x.Envelope, in.ReportID, in.ReportRef)
		result, err = r, e
	default:
		return Observation{}, ErrMode
	}
	usage := gatewayUsage(receipt)
	reserved := reservation.usage()
	if reserved.Calls > usage.Calls {
		usage.Calls, usage.Retries, usage.Tokens = reserved.Calls, reserved.Retries, reserved.Tokens
	}
	usage.SourceMS = sourceMS
	usage.ServiceMS = clock().Sub(started).Milliseconds()
	if !receiptMatchesPack(receipt, x.Pack) {
		return Observation{Usage: usage}, ErrReview
	}
	if err != nil {
		return Observation{Usage: usage}, err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return Observation{}, ErrInvalid
	}
	sum := sha256.Sum256(raw)
	if x.Case.Stage == StageAdversarial {
		if blocked {
			return Observation{Decision: "blocked", SemanticDigest: hex.EncodeToString(sum[:]), ErrorClass: x.Case.Category, Blocked: true, Usage: usage}, nil
		}
		return Observation{Decision: "completed", SemanticDigest: hex.EncodeToString(sum[:]), Usage: usage}, nil
	}
	return Observation{Decision: "completed", SemanticDigest: hex.EncodeToString(sum[:]), Usage: usage}, nil
}

func receiptMatchesPack(receipt gateway.Receipt, pack PackRevision) bool {
	for _, call := range receipt.Calls {
		if call.RequestedModel == "" || call.RequestedModel != pack.Model {
			return false
		}
	}
	return true
}

func adversarialDenied(err error) bool {
	return errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) || errors.Is(err, store.ErrNotFound) ||
		errors.Is(err, nlqexec.ErrInvalid) || errors.Is(err, nlqexec.ErrNoPlan) || errors.Is(err, nlqexec.ErrUnsafeCorrection) || errors.Is(err, nlqexec.ErrInspectionRequired) || errors.Is(err, nlqexec.ErrValidationBudget) || errors.Is(err, nlqexec.ErrExecutionBudget) ||
		errors.Is(err, nlqbyo.ErrInvalid) || errors.Is(err, nlqbyo.ErrReplan) || errors.Is(err, nlqbyo.ErrBudget) || errors.Is(err, reporting.ErrInvalid) || errors.Is(err, reporting.ErrStale) || errors.Is(err, reporting.ErrBudget)
}

type gatewayReservation struct {
	mu     sync.Mutex
	limit  Reservation
	calls  int
	tokens int
}

func (r *gatewayReservation) reserve(ctx context.Context, call gateway.Call, tokens int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !call.Valid() || ctx.Err() != nil || r.calls+1 > r.limit.Calls || r.tokens+tokens > r.limit.Tokens || max(0, r.calls) > r.limit.Retries {
		return gateway.ErrBudget
	}
	r.calls++
	r.tokens += tokens
	return nil
}

func (r *gatewayReservation) usage() Usage {
	r.mu.Lock()
	defer r.mu.Unlock()
	tokens := r.tokens
	return Usage{Calls: r.calls, Retries: max(0, r.calls-1), Tokens: &tokens}
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
