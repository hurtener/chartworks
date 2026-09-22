// Package evaluationapi exposes the reviewed evaluation lifecycle over shared transports.
package evaluationapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/store"
)

const maxBody = 1 << 20

// ReadRequest selects one retained run.
type ReadRequest struct {
	RunID string `json:"run_id"`
}

// SuiteReviewInput binds a review request to a suite identifier.
type SuiteReviewInput struct {
	SuiteID string                        `json:"suite_id"`
	Request evaluation.SuiteReviewRequest `json:"request"`
}

// CancelResult confirms durable cancellation intent.
type CancelResult struct {
	Cancelled bool `json:"cancelled"`
}

// FeedbackExportInput selects reviewed feedback for an immutable unassigned candidate ledger.
type FeedbackExportInput struct {
	ID    string `json:"id"`
	Topic string `json:"topic"`
	Limit int    `json:"limit"`
}

// FeedbackSplitInput binds independent partitioning to an exact candidate digest.
type FeedbackSplitInput struct {
	CandidateID     string   `json:"candidate_id"`
	CandidateDigest string   `json:"candidate_digest"`
	TrainingID      string   `json:"training_id"`
	HeldoutID       string   `json:"heldout_id"`
	HeldoutCaseIDs  []string `json:"heldout_case_ids"`
}

// ProposalInput wraps an optimization proposal request.
type ProposalInput = evaluation.ProposalRequest

// ProposalReviewInput binds review to one proposal.
type ProposalReviewInput struct {
	ProposalID string                           `json:"proposal_id"`
	Request    evaluation.ProposalReviewRequest `json:"request"`
}

// PackSelectionInput performs a CAS selection or rollback.
type PackSelectionInput struct {
	ProposalID       string `json:"proposal_id"`
	ExpectedRevision int64  `json:"expected_revision"`
}

// ProtectedInputRequest stores live material behind an actor-scoped digest reference.
type ProtectedInputRequest struct {
	Retention string `json:"retention"`
	Material  string `json:"material"`
}

// Registry describes the concrete evaluation operations.
func Registry() (*api.Registry, error) {
	errs := []api.ErrorResponse{{Status: 400, Code: "invalid_request"}, {Status: 401, Code: "unauthenticated"}, {Status: 403, Code: "forbidden"}, {Status: 404, Code: "not_found"}, {Status: 409, Code: "conflict"}, {Status: 422, Code: "gate_failed"}, {Status: 429, Code: "budget_exhausted"}, {Status: 503, Code: "unavailable"}, {Status: 504, Code: "cancelled_or_timed_out"}}
	rows := []struct {
		id, path, action, effect, summary, loader string
		req, resp                                 reflect.Type
	}{
		{"authorEvaluationSuite", "/v1/evaluations/suites", "ops.write", "evaluation_suite_draft_commit", "Store one immutable evaluation suite draft", "evaluation.Service.Author", reflect.TypeFor[evaluation.Suite](), reflect.TypeFor[evaluation.SuiteRecord]()},
		{"reviewEvaluationSuite", "/v1/evaluations/suites/review", "ops.audit", "evaluation_suite_review_commit", "Review one exact evaluation suite revision", "evaluation.Service.Review", reflect.TypeFor[SuiteReviewInput](), reflect.TypeFor[evaluation.SuiteRecord]()},
		{"runEvaluation", "/v1/evaluations/runs", "ops.write", "evaluation_live_or_fixture_run", "Run one exact accepted evaluation suite revision", "evaluation.Service.Run", reflect.TypeFor[evaluation.RunRequest](), reflect.TypeFor[evaluation.Report]()},
		{"readEvaluation", "/v1/evaluations/runs/read", "ops.read", "evaluation_evidence_read", "Read one retained evaluation report", "evaluation.Service.Read", reflect.TypeFor[ReadRequest](), reflect.TypeFor[evaluation.Report]()},
		{"cancelEvaluation", "/v1/evaluations/runs/cancel", "ops.write", "evaluation_cancel_request", "Cancel one in-flight evaluation run", "evaluation.Service.Cancel", reflect.TypeFor[ReadRequest](), reflect.TypeFor[CancelResult]()}}
	rows = append(rows,
		struct {
			id, path, action, effect, summary, loader string
			req, resp                                 reflect.Type
		}{"registerEvaluationInput", "/v1/evaluations/inputs", "ops.write", "evaluation_input_commit", "Register actor-scoped protected evaluation material", "evaluation.Service.RegisterInput", reflect.TypeFor[ProtectedInputRequest](), reflect.TypeFor[evaluation.ProtectedRef]()},
		struct {
			id, path, action, effect, summary, loader string
			req, resp                                 reflect.Type
		}{"recoverEvaluation", "/v1/evaluations/runs/recover", "ops.maintain", "evaluation_recovery_commit", "Recover one abandoned evaluation", "evaluation.Service.Recover", reflect.TypeFor[ReadRequest](), reflect.TypeFor[evaluation.Report]()},
		struct {
			id, path, action, effect, summary, loader string
			req, resp                                 reflect.Type
		}{"exportEvaluationFeedback", "/v1/evaluations/feedback/export", "ops.read", "evaluation_feedback_export", "Export reviewed feedback as pending split evidence", "evaluation.Service.ExportFeedback", reflect.TypeFor[FeedbackExportInput](), reflect.TypeFor[evaluation.CandidateExport]()},
		struct {
			id, path, action, effect, summary, loader string
			req, resp                                 reflect.Type
		}{"reviewEvaluationSplit", "/v1/evaluations/feedback/review", "ops.audit", "evaluation_split_review_commit", "Assign disjoint training and heldout children", "evaluation.Service.ReviewFeedbackSplit", reflect.TypeFor[FeedbackSplitInput](), reflect.TypeFor[evaluation.FeedbackSplit]()},
		struct {
			id, path, action, effect, summary, loader string
			req, resp                                 reflect.Type
		}{"proposeEvaluationOptimization", "/v1/evaluations/optimizations", "ops.write", "evaluation_proposal_commit", "Propose optimization from persisted measured reports", "evaluation.Service.ProposeOptimization", reflect.TypeFor[ProposalInput](), reflect.TypeFor[evaluation.OptimizationProposal]()},
		struct {
			id, path, action, effect, summary, loader string
			req, resp                                 reflect.Type
		}{"reviewEvaluationOptimization", "/v1/evaluations/optimizations/review", "ops.audit", "evaluation_proposal_review_commit", "Review one exact optimization proposal", "evaluation.Service.ReviewOptimization", reflect.TypeFor[ProposalReviewInput](), reflect.TypeFor[evaluation.ReviewReceipt]()},
		struct {
			id, path, action, effect, summary, loader string
			req, resp                                 reflect.Type
		}{"selectEvaluationPack", "/v1/evaluations/packs/select", "ops.write", "evaluation_pack_cas_commit", "CAS select or rollback an approved pack", "evaluation.Service.SelectPack", reflect.TypeFor[PackSelectionInput](), reflect.TypeFor[evaluation.PackSelection]()},
	)
	defs := make([]api.Definition, 0, len(rows))
	for _, x := range rows {
		req, err := api.SchemaFor(x.id+"Request", x.req, false, api.OptionalJSONFields)
		if err != nil {
			return nil, fmt.Errorf("%s request: %w", x.id, err)
		}
		resp, err := api.SchemaFor(x.id+"Response", x.resp, true)
		if err != nil {
			return nil, fmt.Errorf("%s response: %w", x.id, err)
		}
		definition := api.Definition{Operation: api.Operation{Method: http.MethodPost, Path: x.path, Action: x.action, Effect: x.effect}, ID: x.id, Summary: x.summary, ResourceLoader: x.loader, Audit: "evaluation lifecycle audit", MaxBodyBytes: maxBody, Request: req, Response: resp, Errors: errs}
		if _, err := api.New([]api.Definition{definition}); err != nil {
			return nil, fmt.Errorf("%s: %w", x.id, err)
		}
		defs = append(defs, definition)
	}
	for i := range defs {
		for j := i + 1; j < len(defs); j++ {
			if defs[i].ID == defs[j].ID || defs[i].Path == defs[j].Path {
				return nil, fmt.Errorf("evaluation registry collision %s/%s", defs[i].ID, defs[j].ID)
			}
		}
	}
	registry, err := api.New(defs)
	if err != nil {
		return nil, fmt.Errorf("evaluation registry: %w", err)
	}
	return registry, nil
}

// Handler mounts the authority-verified HTTP consumers.
func Handler(verifier *auth.Verifier, svc *evaluation.Service, runner evaluation.Runner, next http.Handler) http.Handler {
	if verifier == nil || svc == nil || next == nil {
		return http.NotFoundHandler()
	}
	reg, err := Registry()
	if err != nil {
		return next
	}
	protected := verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, _, ok := reg.Match(r.Method, r.URL.Path)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		e, err := identity.FromContext(r.Context())
		if err != nil {
			failure(w, access.ErrUnauthenticated)
			return
		}
		if !e.Has(d.Action) {
			failure(w, access.ErrForbidden)
			return
		}
		var out any
		switch d.ID {
		case "authorEvaluationSuite":
			var in evaluation.Suite
			err = decode(w, r, d.Request, &in)
			if err == nil {
				out, err = svc.Author(r.Context(), e, in)
			}
		case "reviewEvaluationSuite":
			var in SuiteReviewInput
			err = decode(w, r, d.Request, &in)
			if err == nil {
				out, err = svc.Review(r.Context(), e, in.SuiteID, in.Request)
			}
		case "runEvaluation":
			var in evaluation.RunRequest
			err = decode(w, r, d.Request, &in)
			if err == nil {
				out, err = svc.Run(r.Context(), e, in, runner)
			}
		case "readEvaluation":
			var in ReadRequest
			err = decode(w, r, d.Request, &in)
			if err == nil {
				out, err = svc.Read(r.Context(), e, in.RunID)
			}
		case "cancelEvaluation":
			var in ReadRequest
			err = decode(w, r, d.Request, &in)
			if err == nil {
				err = svc.Cancel(r.Context(), e, in.RunID)
				out = CancelResult{Cancelled: err == nil}
			}
		case "registerEvaluationInput":
			var in ProtectedInputRequest
			err = decode(w, r, d.Request, &in)
			if err == nil {
				var material evaluation.LiveInput
				if json.Unmarshal([]byte(in.Material), &material) != nil {
					err = evaluation.ErrInvalid
				} else {
					out, err = svc.RegisterInput(r.Context(), e, in.Retention, material)
				}
			}
		case "recoverEvaluation":
			var in ReadRequest
			err = decode(w, r, d.Request, &in)
			if err == nil {
				out, err = svc.Recover(r.Context(), e, in.RunID)
			}
		case "exportEvaluationFeedback":
			var in FeedbackExportInput
			err = decode(w, r, d.Request, &in)
			if err == nil {
				out, err = svc.ExportFeedback(r.Context(), e, in.ID, in.Topic, in.Limit)
			}
		case "reviewEvaluationSplit":
			var in FeedbackSplitInput
			err = decode(w, r, d.Request, &in)
			if err == nil {
				out, err = svc.ReviewFeedbackSplit(r.Context(), e, in.CandidateID, in.CandidateDigest, in.TrainingID, in.HeldoutID, in.HeldoutCaseIDs)
			}
		case "proposeEvaluationOptimization":
			var in ProposalInput
			err = decode(w, r, d.Request, &in)
			if err == nil {
				out, err = svc.ProposeOptimization(r.Context(), e, in)
			}
		case "reviewEvaluationOptimization":
			var in ProposalReviewInput
			err = decode(w, r, d.Request, &in)
			if err == nil {
				out, err = svc.ReviewOptimization(r.Context(), e, in.ProposalID, in.Request)
			}
		case "selectEvaluationPack":
			var in PackSelectionInput
			err = decode(w, r, d.Request, &in)
			if err == nil {
				out, err = svc.SelectPack(r.Context(), e, in.ProposalID, in.ExpectedRevision)
			}
		}
		if err != nil {
			failure(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(out)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := reg.Match(r.Method, r.URL.Path)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}
func decode(w http.ResponseWriter, r *http.Request, s interface{ Validate([]byte, int) error }, out any) error {
	if r.Header.Get("Content-Type") != "application/json" || r.URL.RawQuery != "" {
		return store.ErrInvalid
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil || s.Validate(b, maxBody) != nil || json.Unmarshal(b, out) != nil {
		return store.ErrInvalid
	}
	return nil
}
func failure(w http.ResponseWriter, err error) {
	status, code := 503, "unavailable"
	switch {
	case errors.Is(err, access.ErrUnauthenticated):
		status, code = 401, "unauthenticated"
	case errors.Is(err, access.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, access.ErrNotFound), errors.Is(err, store.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, store.ErrConflict):
		status, code = 409, "conflict"
	case errors.Is(err, evaluation.ErrInvalid):
		status, code = 400, "invalid_request"
	case errors.Is(err, evaluation.ErrGate):
		status, code = 422, "gate_failed"
	case errors.Is(err, evaluation.ErrBudget):
		status, code = 429, "budget_exhausted"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status, code = 504, "cancelled_or_timed_out"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

// MCPBindings mounts the same service operations for the established Apps bridge.
func MCPBindings(svc *evaluation.Service, runner evaluation.Runner) ([]mcpserver.Binding, error) {
	if svc == nil {
		return nil, nil
	}
	reg, err := Registry()
	if err != nil {
		return nil, err
	}
	mapper := func(err error) mcpserver.Fault { _, code := classify(err); return mcpserver.Fault{Code: code} }
	a, err := mcpserver.Bind(reg, "runEvaluation", "run_evaluation", "evaluation", "Run one accepted exact evaluation revision with bounded calls and durable terminal evidence.", func(ctx context.Context, e identity.Envelope, in evaluation.RunRequest) (evaluation.Report, error) {
		return svc.Run(ctx, e, in, runner)
	}, mapper)
	if err != nil {
		return nil, err
	}
	b, err := mcpserver.Bind(reg, "readEvaluation", "read_evaluation", "evaluation", "Read retained content-free evaluation evidence without source or model work.", func(ctx context.Context, e identity.Envelope, in ReadRequest) (evaluation.Report, error) {
		return svc.Read(ctx, e, in.RunID)
	}, mapper)
	if err != nil {
		return nil, err
	}
	c, err := mcpserver.Bind(reg, "cancelEvaluation", "cancel_evaluation", "evaluation", "Persist cancellation intent and signal an owned in-flight evaluation.", func(ctx context.Context, e identity.Envelope, in ReadRequest) (CancelResult, error) {
		err := svc.Cancel(ctx, e, in.RunID)
		return CancelResult{Cancelled: err == nil}, err
	}, mapper)
	if err != nil {
		return nil, err
	}
	d, err := mcpserver.Bind(reg, "authorEvaluationSuite", "author_evaluation_suite", "evaluation", "Store an immutable draft suite; this does not make it executable.", svc.Author, mapper)
	if err != nil {
		return nil, err
	}
	eb, err := mcpserver.Bind(reg, "reviewEvaluationSuite", "review_evaluation_suite", "evaluation", "Record a distinct authenticated decision over one exact suite revision.", func(ctx context.Context, e identity.Envelope, in SuiteReviewInput) (evaluation.SuiteRecord, error) {
		return svc.Review(ctx, e, in.SuiteID, in.Request)
	}, mapper)
	if err != nil {
		return nil, err
	}
	f, err := mcpserver.Bind(reg, "exportEvaluationFeedback", "export_evaluation_feedback", "evaluation", "Export reviewed feedback as immutable pending split evidence.", func(ctx context.Context, e identity.Envelope, in FeedbackExportInput) (evaluation.CandidateExport, error) {
		return svc.ExportFeedback(ctx, e, in.ID, in.Topic, in.Limit)
	}, mapper)
	if err != nil {
		return nil, err
	}
	g, err := mcpserver.Bind(reg, "reviewEvaluationSplit", "review_evaluation_split", "evaluation", "Assign immutable disjoint training and heldout ledgers.", func(ctx context.Context, e identity.Envelope, in FeedbackSplitInput) (evaluation.FeedbackSplit, error) {
		return svc.ReviewFeedbackSplit(ctx, e, in.CandidateID, in.CandidateDigest, in.TrainingID, in.HeldoutID, in.HeldoutCaseIDs)
	}, mapper)
	if err != nil {
		return nil, err
	}
	h, err := mcpserver.Bind(reg, "proposeEvaluationOptimization", "propose_evaluation_optimization", "evaluation", "Compare persisted exact-pack reports without promotion.", svc.ProposeOptimization, mapper)
	if err != nil {
		return nil, err
	}
	i, err := mcpserver.Bind(reg, "reviewEvaluationOptimization", "review_evaluation_optimization", "evaluation", "Review one exact optimization proposal.", func(ctx context.Context, e identity.Envelope, in ProposalReviewInput) (evaluation.ReviewReceipt, error) {
		return svc.ReviewOptimization(ctx, e, in.ProposalID, in.Request)
	}, mapper)
	if err != nil {
		return nil, err
	}
	j, err := mcpserver.Bind(reg, "selectEvaluationPack", "select_evaluation_pack", "evaluation", "CAS select or rollback an approved measured pack.", func(ctx context.Context, e identity.Envelope, in PackSelectionInput) (evaluation.PackSelection, error) {
		return svc.SelectPack(ctx, e, in.ProposalID, in.ExpectedRevision)
	}, mapper)
	if err != nil {
		return nil, err
	}
	k, err := mcpserver.Bind(reg, "recoverEvaluation", "recover_evaluation", "evaluation", "Recover an abandoned actor-owned run into terminal evidence.", func(ctx context.Context, e identity.Envelope, in ReadRequest) (evaluation.Report, error) {
		return svc.Recover(ctx, e, in.RunID)
	}, mapper)
	if err != nil {
		return nil, err
	}
	l, err := mcpserver.Bind(reg, "registerEvaluationInput", "register_evaluation_input", "evaluation", "Store protected actor-scoped live material and return only its digest reference.", func(ctx context.Context, e identity.Envelope, in ProtectedInputRequest) (evaluation.ProtectedRef, error) {
		var material evaluation.LiveInput
		if json.Unmarshal([]byte(in.Material), &material) != nil {
			return evaluation.ProtectedRef{}, evaluation.ErrInvalid
		}
		return svc.RegisterInput(ctx, e, in.Retention, material)
	}, mapper)
	if err != nil {
		return nil, err
	}
	return []mcpserver.Binding{d, eb, a, b, c, f, g, h, i, j, k, l}, nil
}
func classify(err error) (int, string) {
	switch {
	case errors.Is(err, evaluation.ErrInvalid):
		return 400, "invalid_request"
	case errors.Is(err, evaluation.ErrGate):
		return 422, "gate_failed"
	case errors.Is(err, evaluation.ErrBudget):
		return 429, "budget_exhausted"
	case errors.Is(err, access.ErrForbidden):
		return 403, "forbidden"
	case errors.Is(err, access.ErrUnauthenticated):
		return 401, "unauthenticated"
	case errors.Is(err, store.ErrNotFound):
		return 404, "not_found"
	}
	return 503, "unavailable"
}
