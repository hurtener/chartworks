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
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

// LiveInput is protected execution material. It is never retained in reports or logs.
// A narrative frozen case may omit Pack so one reviewed suite can evaluate the
// same consumer under distinct accepted runtime packs. Its product-sealed pin
// must then match the selected runtime before source or model execution.
type LiveInput struct {
	Pack      PackRevision                `json:"pack"`
	Frozen    *FrozenRunInput             `json:"frozen,omitempty"`
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

// ProtectedPackMatches checks the protected input's pack shape. A pack-neutral
// narrative frozen input still requires the product-sealed selected pack pin
// before source or model execution; this check grants no authority by itself.
func ProtectedPackMatches(in LiveInput, selected PackRevision) bool {
	if validPack(in.Pack) {
		want, wantErr := digest(selected)
		got, gotErr := digest(in.Pack)
		return wantErr == nil && gotErr == nil && want == got
	}
	return in.Pack.ID == "" && in.Pack.Revision == 0 && in.Pack.Digest == "" && in.Pack.Model == "" &&
		len(in.Pack.Models) == 0 && in.Pack.ConfigurationDigest == "" && in.Frozen != nil && in.Frozen.Request.Narrative &&
		in.Question == nil && in.Run == nil && in.Route == nil && in.Replay == nil && in.Shadow == nil && in.BYO == nil &&
		in.Chart == nil && (in.ReportID == "" || in.ReportID == in.Frozen.BlockID && identifier(in.ReportID)) && in.ReportRef == (reporting.Reference{})
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

type routingRuntime interface {
	Route(context.Context, identity.Envelope, nlqroute.RouteRequest) (nlqroute.RouteResult, error)
}
type queryRuntime interface {
	Preflight(context.Context, identity.Envelope, nlqexec.PreflightRequest) (nlqexec.PreflightResult, error)
	Plan(context.Context, identity.Envelope, nlqexec.PlanRequest) (nlqexec.PlanResult, error)
	Run(context.Context, identity.Envelope, nlqexec.RunRequest) (nlqexec.RunResult, error)
	InspectSaved(context.Context, identity.Envelope, nlqexec.SavedQuestion) (nlqexec.SavedEvidence, error)
}

type compositePlanRunRuntime interface {
	PlanAndRun(context.Context, identity.Envelope, nlqexec.PlanRequest, nlqexec.RunRequest) (nlqexec.PlanResult, nlqexec.RunResult, error)
}
type chartRuntime interface {
	Select(context.Context, identity.Envelope, chartservice.SelectRequest) (chartservice.SelectionResult, error)
}
type reportRuntime interface {
	Read(context.Context, identity.Envelope, string, reporting.Reference) (reporting.View, error)
}
type frozenRunRuntime interface {
	Admit(context.Context, identity.Envelope, string, reporting.RunRequest) (reporting.RunView, error)
	Run(context.Context, identity.Envelope, string, bool) (reporting.RunView, error)
}
type frozenRunReader interface {
	ReadFrozenRun(context.Context, identity.Envelope, string, bool) (reporting.RunRecord, error)
}
type byoRuntime interface {
	Submit(context.Context, identity.Envelope, nlqbyo.SubmitRequest) (nlqbyo.SubmitResult, error)
}

// GovernedRunner invokes the existing governed services; it does not introduce a second gateway, validator, or executor.
type GovernedRunner struct {
	Inputs      LiveInputResolver
	Routing     routingRuntime
	Query       queryRuntime
	Saved       SavedInspector
	Charts      chartRuntime
	Reports     reportRuntime
	Frozen      frozenRunRuntime
	FrozenStore frozenRunReader
	BYO         byoRuntime
	Clock       Clock
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
	if !ProtectedPackMatches(in, x.Pack) {
		return Observation{Usage: reservation.usage()}, ErrReview
	}
	if x.RuntimeConfig.Digest != x.Pack.ConfigurationDigest || !packModelsMatchConfig(x.Pack, x.RuntimeConfig) {
		return Observation{Usage: reservation.usage()}, ErrReview
	}
	ctx, err = gateway.WithRuntimeConfig(ctx, x.RuntimeConfig)
	if err != nil {
		return Observation{Usage: reservation.usage()}, ErrReview
	}
	var result any
	var receipt gateway.Receipt
	blocked := false
	var sourceCalls int
	var usageSourceMS *int64
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
		if in.Frozen != nil {
			if in.Run != nil || g.Frozen == nil || g.FrozenStore == nil {
				return Observation{}, ErrMode
			}
			frozen := *in.Frozen
			if x.ReportRunID != "" {
				if !identifier(x.ReportRunID) {
					return Observation{}, ErrMode
				}
				key := sha256.Sum256([]byte(x.ReportRunID + ":" + x.Case.ID + ":" + x.Pack.Digest + ":" + frozen.Request.Key))
				frozen.Request.Key = "eval:" + hex.EncodeToString(key[:])
			}
			record, runErr := runFrozenInputChecked(ctx, x.Envelope, g.Frozen, g.FrozenStore, frozen, func(m reporting.RunManifest) error {
				if !in.Frozen.Request.Narrative {
					return nil
				}
				want, valid := expectedNarrativePin(x.Pack, x.RuntimeConfig, x.RuntimeDigest)
				if !valid || m.NarrativePackUnavailable || m.NarrativePack == nil || *m.NarrativePack != want || m.ReuseKey != reporting.ReuseIdentity(m) {
					return ErrReview
				}
				return nil
			})
			if runErr != nil {
				return Observation{}, runErr
			}
			semantic, calls, sourceNS, modelReceipt, evidenceErr := frozenEvidence(record)
			if evidenceErr != nil {
				return Observation{}, evidenceErr
			}
			result, receipt, sourceCalls = semantic, modelReceipt, calls
			if sourceNS != nil {
				value := *sourceNS / int64(time.Millisecond)
				usageSourceMS = &value
			}
			break
		}
		if g.Query == nil || in.Run == nil {
			return Observation{}, ErrMode
		}
		runRequest := *in.Run
		if in.Question != nil {
			if !identity.Identifier(runRequest.Operation) {
				return Observation{}, ErrMode
			}
			composite, ok := g.Query.(compositePlanRunRuntime)
			if !ok {
				return Observation{}, ErrMode
			}
			planned, r, runErr := composite.PlanAndRun(ctx, x.Envelope, nlqexec.PlanRequest{QuestionRequest: *in.Question, Operation: runRequest.Operation}, runRequest)
			if runErr != nil {
				return Observation{}, runErr
			}
			if !identity.Identifier(planned.QueryID) || r.QueryID != planned.QueryID {
				return Observation{}, ErrMode
			}
			result, err, receipt = r, nil, planned.Receipt
			if r.ExecutionFixes > 0 {
				// The current run result preserves the final attempt but not every
				// failed source attempt's duration. Do not seal partial timings.
				return Observation{}, ErrMode
			}
			if r.Execution.Attempt.ID != "" && r.Execution.Attempt.Finished != nil {
				// Attempt wall time includes the journal and finalization. It is
				// evidence of a physical call, never a source-only duration.
				if r.Execution.Attempt.Manifest.Receipt.Dialect == "postgres" {
					sourceCalls = 1
				}
			}
			if len(planned.Receipt.Calls) > 0 {
				extra, ok := gatewayReceiptSuffix(planned.Receipt, r.Receipt)
				if !ok {
					return Observation{}, ErrReview
				}
				receipt.Calls = append(receipt.Calls, extra.Calls...)
				if extra.Warning != "" {
					receipt.Warning = extra.Warning
				}
			}
		} else {
			r, runErr := g.Query.Run(ctx, x.Envelope, runRequest)
			result, err = r, runErr
			if r.Execution.Attempt.ID != "" && r.Execution.Attempt.Finished != nil {
				if r.Execution.Attempt.Manifest.Receipt.Dialect == "postgres" {
					sourceCalls = 1
				}
			}
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
	usage.SourceCalls = sourceCalls
	usage.SourceMS = usageSourceMS
	usage.ServiceMS = clock().Sub(started).Milliseconds()
	if !receiptMatchesPack(receipt, x.Pack) {
		return Observation{Usage: usage}, ErrReview
	}
	if err != nil {
		return Observation{Usage: usage}, err
	}
	semantic := result
	if x.Case.Stage == StageConsumer && in.Question != nil {
		run, ok := result.(nlqexec.RunResult)
		if !ok || run.Execution.Result == nil {
			return Observation{Usage: usage}, ErrMode
		}
		semantic = struct {
			Status        string           `json:"status"`
			EvidenceStale bool             `json:"evidence_stale"`
			Result        *readexec.Result `json:"result"`
		}{run.Status, run.EvidenceStale, run.Execution.Result}
	}
	raw, err := json.Marshal(semantic)
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

func gatewayReceiptSuffix(prefix, complete gateway.Receipt) (gateway.Receipt, bool) {
	if len(complete.Calls) < len(prefix.Calls) {
		return gateway.Receipt{}, false
	}
	for i := range prefix.Calls {
		left, leftErr := json.Marshal(prefix.Calls[i])
		right, rightErr := json.Marshal(complete.Calls[i])
		if leftErr != nil || rightErr != nil || string(left) != string(right) {
			return gateway.Receipt{}, false
		}
	}
	out := gateway.Receipt{Calls: append([]gateway.Usage(nil), complete.Calls[len(prefix.Calls):]...)}
	if complete.Warning != prefix.Warning {
		out.Warning = complete.Warning
	}
	return out, true
}

func receiptMatchesPack(receipt gateway.Receipt, pack PackRevision) bool {
	for _, call := range receipt.Calls {
		model := pack.Model
		for _, binding := range pack.Models {
			if binding.Role == call.Role {
				model = binding.Model
				break
			}
		}
		if call.RequestedModel == "" || call.RequestedModel != model || call.ConfigurationDigest != pack.ConfigurationDigest {
			return false
		}
	}
	return true
}

func packModelsMatchConfig(pack PackRevision, cfg gateway.RuntimeConfig) bool {
	if cfg.Digest != gateway.ConfigurationDigest(cfg) {
		return false
	}
	if cfg.Model != pack.Model || len(cfg.Models) != len(pack.Models) {
		return false
	}
	for _, binding := range pack.Models {
		found := false
		for _, configured := range cfg.Models {
			if binding.Role == configured.Role && binding.Model == configured.Model {
				found = true
				break
			}
		}
		if !found {
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
	cost   float64
}

func (r *gatewayReservation) reserve(ctx context.Context, call gateway.Call, tokens int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	attemptCost, costKnown := gateway.RuntimeAttemptCost(ctx)
	if !call.Valid() || ctx.Err() != nil || r.calls+1 > r.limit.Calls || r.tokens+tokens > r.limit.Tokens || max(0, r.calls) > r.limit.Retries || r.limit.CostUSD != nil && (!costKnown || r.cost+attemptCost > *r.limit.CostUSD) {
		return gateway.ErrBudget
	}
	r.calls++
	r.tokens += tokens
	if r.limit.CostUSD != nil {
		r.cost += attemptCost
	}
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
