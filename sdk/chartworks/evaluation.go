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

type evaluationSuiteReviewInput struct {
	SuiteID string                       `json:"suite_id"`
	Request EvaluationSuiteReviewRequest `json:"request"`
}

// EvaluationReadRequest selects one retained run.
type EvaluationReadRequest struct {
	RunID string `json:"run_id"`
}

// EvaluationCancelResult confirms durable cancellation intent.
type EvaluationCancelResult struct {
	Cancelled bool `json:"cancelled"`
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
