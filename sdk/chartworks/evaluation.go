package chartworks

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/evaluation"
)

// EvaluationRunRequest selects an exact accepted suite revision.
type EvaluationRunRequest = evaluation.RunRequest

// EvaluationReport is content-free durable run evidence.
type EvaluationReport = evaluation.Report

// EvaluationSuite is an immutable versioned manifest.
type EvaluationSuite = evaluation.Suite

// EvaluationSuiteRecord includes its durable review lifecycle.
type EvaluationSuiteRecord = evaluation.SuiteRecord

// EvaluationSuiteReviewRequest pins a decision to exact suite material.
type EvaluationSuiteReviewRequest = evaluation.SuiteReviewRequest

// EvaluationRuntimePackAuthorRequest proposes server-owned runtime material.
type EvaluationRuntimePackAuthorRequest = evaluation.RuntimePackAuthorRequest

// EvaluationRuntimePackRecord includes its independent review lifecycle.
type EvaluationRuntimePackRecord = evaluation.RuntimePackRecord

// EvaluationRuntimePackReviewRequest makes reviewed routing and cost explicit.
type EvaluationRuntimePackReviewRequest = evaluation.RuntimePackReviewRequest

type evaluationSuiteReviewInput struct {
	SuiteID string                       `json:"suite_id"`
	Request EvaluationSuiteReviewRequest `json:"request"`
}

type evaluationRuntimePackReviewInput struct {
	PackDigest string                             `json:"pack_digest"`
	Request    EvaluationRuntimePackReviewRequest `json:"request"`
}

// EvaluationReadRequest selects one retained run.
type EvaluationReadRequest struct {
	RunID string `json:"run_id"`
}

// EvaluationCancelResult confirms durable cancellation intent.
type EvaluationCancelResult struct {
	Cancelled bool `json:"cancelled"`
}

// EvaluationCandidateExport is an immutable feedback split ledger.
type EvaluationCandidateExport = evaluation.CandidateExport

// EvaluationFeedbackSplit is an independently assigned training/heldout partition.
type EvaluationFeedbackSplit = evaluation.FeedbackSplit

// EvaluationOptimizationProposal is a persisted exact-report comparison.
type EvaluationOptimizationProposal = evaluation.OptimizationProposal

// EvaluationPackSelection is the CAS-controlled active pack pointer.
type EvaluationPackSelection = evaluation.PackSelection

// EvaluationLiveInput is protected material accepted only by registration.
type EvaluationLiveInput = evaluation.LiveInput

type evaluationFeedbackExportInput struct {
	ID    string `json:"id"`
	Topic string `json:"topic"`
	Limit int    `json:"limit"`
}
type evaluationFeedbackSplitInput struct {
	CandidateID     string   `json:"candidate_id"`
	CandidateDigest string   `json:"candidate_digest"`
	TrainingID      string   `json:"training_id"`
	HeldoutID       string   `json:"heldout_id"`
	HeldoutCaseIDs  []string `json:"heldout_case_ids"`
}
type evaluationProposalReviewInput struct {
	ProposalID string                           `json:"proposal_id"`
	Request    evaluation.ProposalReviewRequest `json:"request"`
}
type evaluationPackSelectionInput struct {
	ProposalID       string `json:"proposal_id"`
	ExpectedRevision int64  `json:"expected_revision"`
}
type evaluationProtectedInputRequest struct {
	Retention string `json:"retention"`
	Material  string `json:"material"`
}

// AuthorEvaluationSuite stores an immutable draft.
func (c *Client) AuthorEvaluationSuite(ctx context.Context, in EvaluationSuite) (EvaluationSuiteRecord, error) {
	var out EvaluationSuiteRecord
	raw, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/suites", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}

// AuthorEvaluationRuntimePack stores one immutable runtime configuration draft.
func (c *Client) AuthorEvaluationRuntimePack(ctx context.Context, in EvaluationRuntimePackAuthorRequest) (EvaluationRuntimePackRecord, error) {
	var out EvaluationRuntimePackRecord
	raw, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/runtime-packs", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}

// ReviewEvaluationRuntimePack accepts exact visible routing/configuration/cost material.
func (c *Client) ReviewEvaluationRuntimePack(ctx context.Context, packDigest string, in EvaluationRuntimePackReviewRequest) (EvaluationRuntimePackRecord, error) {
	var out EvaluationRuntimePackRecord
	raw, err := json.Marshal(evaluationRuntimePackReviewInput{PackDigest: packDigest, Request: in})
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/runtime-packs/review", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}

// ReviewEvaluationSuite records an authenticated exact-revision decision.
func (c *Client) ReviewEvaluationSuite(ctx context.Context, id string, in EvaluationSuiteReviewRequest) (EvaluationSuiteRecord, error) {
	var out EvaluationSuiteRecord
	raw, err := json.Marshal(evaluationSuiteReviewInput{SuiteID: id, Request: in})
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/suites/review", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}

// RunEvaluation executes one accepted exact suite revision.
func (c *Client) RunEvaluation(ctx context.Context, in EvaluationRunRequest) (EvaluationReport, error) {
	var out EvaluationReport
	raw, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/runs", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}

// ReadEvaluation reads retained evidence without executing work.
func (c *Client) ReadEvaluation(ctx context.Context, runID string) (EvaluationReport, error) {
	var out EvaluationReport
	raw, err := json.Marshal(EvaluationReadRequest{RunID: runID})
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/runs/read", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}

// CancelEvaluation persists intent and signals locally owned work.
func (c *Client) CancelEvaluation(ctx context.Context, runID string) (EvaluationCancelResult, error) {
	var out EvaluationCancelResult
	raw, err := json.Marshal(EvaluationReadRequest{RunID: runID})
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/runs/cancel", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}

// RecoverEvaluation finalizes an abandoned actor-owned run.
func (c *Client) RecoverEvaluation(ctx context.Context, runID string) (EvaluationReport, error) {
	var out EvaluationReport
	raw, err := json.Marshal(EvaluationReadRequest{RunID: runID})
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/runs/recover", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}

// ExportEvaluationFeedback creates immutable pending split evidence.
func (c *Client) ExportEvaluationFeedback(ctx context.Context, id, topic string, limit int) (EvaluationCandidateExport, error) {
	var out EvaluationCandidateExport
	raw, err := json.Marshal(evaluationFeedbackExportInput{id, topic, limit})
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/feedback/export", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}

// ReviewEvaluationSplit creates independently reviewed disjoint training and heldout ledgers.
func (c *Client) ReviewEvaluationSplit(ctx context.Context, candidateID, digest, trainingID, heldoutID string, heldoutCaseIDs []string) (EvaluationFeedbackSplit, error) {
	var out EvaluationFeedbackSplit
	raw, err := json.Marshal(evaluationFeedbackSplitInput{candidateID, digest, trainingID, heldoutID, heldoutCaseIDs})
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/feedback/review", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}

// ProposeEvaluationOptimization compares persisted measured pack reports.
func (c *Client) ProposeEvaluationOptimization(ctx context.Context, in evaluation.ProposalRequest) (EvaluationOptimizationProposal, error) {
	var out EvaluationOptimizationProposal
	raw, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/optimizations", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}

// ReviewEvaluationOptimization records a human decision over an exact proposal.
func (c *Client) ReviewEvaluationOptimization(ctx context.Context, id string, in evaluation.ProposalReviewRequest) (evaluation.ReviewReceipt, error) {
	var out evaluation.ReviewReceipt
	raw, err := json.Marshal(evaluationProposalReviewInput{id, in})
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/optimizations/review", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}

// SelectEvaluationPack performs an approved CAS selection or rollback.
func (c *Client) SelectEvaluationPack(ctx context.Context, id string, expected int64) (EvaluationPackSelection, error) {
	var out EvaluationPackSelection
	raw, err := json.Marshal(evaluationPackSelectionInput{id, expected})
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/packs/select", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}

// RegisterEvaluationInput stores actor-scoped protected material and returns its digest reference.
func (c *Client) RegisterEvaluationInput(ctx context.Context, retention string, in EvaluationLiveInput) (evaluation.ProtectedRef, error) {
	var out evaluation.ProtectedRef
	material, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	raw, err := json.Marshal(evaluationProtectedInputRequest{Retention: retention, Material: string(material)})
	if err != nil {
		return out, err
	}
	err = c.exchange(ctx, "POST", "/v1/evaluations/inputs", "", "application/json", bytes.NewReader(raw), &out, 1<<20, wireOptions{})
	return out, err
}
