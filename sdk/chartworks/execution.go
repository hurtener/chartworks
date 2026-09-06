package chartworks

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ReadExecutionOptions identifies a logical operation and an explicit physical attempt.
// Zero rows/bytes use server defaults. An attempt number is never retried automatically.
type ReadExecutionOptions struct {
	Operation string `json:"operation"`
	Number int `json:"attempt"`
	Preview bool `json:"preview"`
	Rows int `json:"rows"`
	Bytes int `json:"bytes"`
}

// ReadExecutionRequest is validated again by the server, never trusted as a plan.
type ReadExecutionRequest struct {
	Context string `json:"context"`
	SQL string `json:"sql"`
	Parameters []ReadParameter `json:"parameters"`
	Execution ReadExecutionOptions `json:"execution"`
}

// ReadField preserves ordered type/encoding metadata. Exact values use strings.
type ReadField struct { Name string `json:"name"`;Type string `json:"type"`;Encoding string `json:"encoding"`;NativeType string `json:"native_type"` }

// ReadCost never turns unavailable actual scanned bytes into fabricated zero.
type ReadCost struct { PlannerUnits *float64 `json:"planner_units"`;ScannedBytes *int64 `json:"scanned_bytes"` }

// ReadResult retains JSON strings for exact numerics and JSON text, booleans and NULL.
// Outcome is succeeded, empty or truncated; truncated prefixes are not complete totals.
type ReadResult struct { Schema []ReadField `json:"schema"`;Rows [][]json.RawMessage `json:"rows"`;Outcome string `json:"outcome"`;Truncation string `json:"truncation"`;Bytes int `json:"bytes"`;Cost ReadCost `json:"cost"` }

// ReadLimits records effective server limits with explicit duration units.
type ReadLimits struct { Rows int `json:"rows"`;Bytes int `json:"bytes"`;Timeout int64 `json:"timeout_ns"`;CancelGrace int64 `json:"cancel_grace_ns"`;PlannerCost float64 `json:"planner_cost_ceiling"` }

// ReadManifest contains content-free request identity, not SQL, values or credentials.
type ReadManifest struct { Operation string `json:"operation"`;Session string `json:"session"`;Receipt ReadValidationReceipt `json:"validation"`;Limits ReadLimits `json:"limits"`;Preview bool `json:"preview"` }

// ReadRemoteQuery identifies one tagged backend transaction, never a cancellation credential.
type ReadRemoteQuery struct { PID uint32 `json:"pid"`;Started time.Time `json:"backend_started"`;Tag string `json:"tag"` }

// ReadAttempt records admission, dispatch uncertainty and actual terminal outcome.
type ReadAttempt struct {
	ID string `json:"id"`
	Number int `json:"number"`
	Manifest ReadManifest `json:"manifest"`
	Status string `json:"status"`
	Remote *ReadRemoteQuery `json:"remote"`
	RemoteState string `json:"remote_state"`
	CancelRequested bool `json:"cancel_requested"`
	Created time.Time `json:"created_at"`
	Deadline time.Time `json:"deadline"`
	Finished *time.Time `json:"finished_at"`
	Rows int `json:"rows_returned"`
	Bytes int `json:"bytes_returned"`
	Code string `json:"code"`
}

// ReadExecutionReport may describe a failed accepted attempt. Check Attempt.Status;
// an HTTP 200 receipt does not assert that warehouse execution succeeded.
type ReadExecutionReport struct { Attempt ReadAttempt `json:"attempt"`;Result *ReadResult `json:"result"` }

// ReadControlReceipt distinguishes cancellation intent from observed termination.
type ReadControlReceipt struct { Attempt ReadAttempt `json:"attempt"`;RemoteState string `json:"remote_state"` }

// ExecuteRead performs one submission and never retries on transport uncertainty.
func (c *Client) ExecuteRead(ctx context.Context,id string,input ReadExecutionRequest) (out ReadExecutionReport,err error) {
	if !wireID(id) || !wireID(input.Context) || !wireID(input.Execution.Operation) || input.Execution.Number<1 || input.Execution.Number>3 { return out,errors.New("chartworks: invalid execution coordinates") }
	if input.Parameters==nil { input.Parameters=[]ReadParameter{} }
	err=c.callLimit(ctx,"POST","/v1/sources/"+id+"/execute","",input,&out,(16<<20)+(128<<10))
	return out,err
}

// ReadExecution reads a receipt; it cannot recreate or silently rerun result values.
func (c *Client) ReadExecution(ctx context.Context,id string) (out ReadAttempt,err error) {
	if !wireID(id) { return out,errors.New("chartworks: invalid attempt identifier") }
	err=c.call(ctx,"GET","/v1/read-executions/"+id,"",nil,&out);return out,err
}

// CancelRead records intent and requests cancellation using the actual source adapter.
func (c *Client) CancelRead(ctx context.Context,id string) (out ReadControlReceipt,err error) {
	if !wireID(id) { return out,errors.New("chartworks: invalid attempt identifier") }
	err=c.call(ctx,"POST","/v1/read-executions/"+id+"/cancel","",struct{}{},&out);return out,err
}

// ReconcileRead observes an uncertain attempt without reconstructing successful rows.
func (c *Client) ReconcileRead(ctx context.Context,id string) (out ReadControlReceipt,err error) {
	if !wireID(id) { return out,errors.New("chartworks: invalid attempt identifier") }
	err=c.call(ctx,"POST","/v1/read-executions/"+id+"/reconcile","",struct{}{},&out);return out,err
}
