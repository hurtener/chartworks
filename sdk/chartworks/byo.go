package chartworks

import (
	"context"
	"errors"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqbyo"
)

// QueryContextRequest asks for the closed version-1 external context contract.
type QueryContextRequest = nlqbyo.CreateRequest

// QueryContextResult includes either an exact bundle or explicit routing clarification.
type QueryContextResult = nlqbyo.CreateResult

// QueryContextReference is a lookup coordinate, not an authority token.
type QueryContextReference = nlqbyo.Reference

// QueryContextView includes the context and content-free explicit step history.
type QueryContextView = nlqbyo.View

// SQLSubmission asks for one validator-checked external SQL step.
type SQLSubmission = nlqbyo.SubmitRequest

// SQLSubmissionResult reports actual step status. A receipt is not necessarily a
// successful query; replays never pretend that transient values were retained.
type SQLSubmissionResult = nlqbyo.SubmitResult

// GetQueryContext retrieves and stores one semantic context without executing SQL.
func (c *Client) GetQueryContext(ctx context.Context, in QueryContextRequest) (out QueryContextResult, err error) {
	err = c.callLimit(ctx, "POST", "/v1/nlq/contexts", "", in, &out, 2<<20)
	return
}

// ReadQueryContext sends current Pengui authority for every opaque reference read.
func (c *Client) ReadQueryContext(ctx context.Context, in QueryContextReference) (out QueryContextView, err error) {
	if !nlqbyo.ReferenceValid(in) {
		return out, errors.New("chartworks: invalid context reference")
	}
	err = c.callLimit(ctx, "POST", "/v1/nlq/contexts/read", "", in, &out, 2<<20)
	return
}

// SubmitSQL performs one explicit budgeted step, never an internal agent loop.
func (c *Client) SubmitSQL(ctx context.Context, in SQLSubmission) (out SQLSubmissionResult, err error) {
	if !nlqbyo.ReferenceValid(in.Reference) || !wireID(in.Operation) {
		return out, errors.New("chartworks: invalid SQL submission reference")
	}
	in.Parameters = append([]exec.Parameter{}, in.Parameters...)
	err = c.callLimit(ctx, "POST", "/v1/nlq/sql", "", in, &out, nlqRunResponseLimit)
	return
}
