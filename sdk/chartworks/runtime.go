package chartworks

import (
	"context"
	"net/url"
	"strconv"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

type ReportingRunRequest = reporting.RunRequest
type ReportingRun = reporting.RunView
type ReportingRunDispatch struct {
	Resume bool `json:"resume"`
}
type ReportingResultPage = reporting.ResultPage
type ReportingRetainedOutput = reporting.RetainedOutput
type ReportingArtifactList = reporting.ArtifactList
type EngineeringGoal = engineering.AutopilotGoal
type EngineeringProposal = engineering.AutopilotProposal
type EngineeringProposalEdit = engineering.AutopilotEditRequest
type EngineeringProposalReview = engineering.AutopilotReviewRequest
type EngineeringProposalApply = engineering.AutopilotApplyRequest
type EngineeringDrift = engineering.AutopilotDrift

// AdmitReportingRun calls the common registered domain operation with fresh caller authority.
func (c *Client) AdmitReportingRun(ctx context.Context, id string, in ReportingRunRequest) (out ReportingRun, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/blocks/"+id+"/runs", "", in, &out, 64<<20)
	return
}

// ExecuteReportingRun calls the common registered domain operation with fresh caller authority.
func (c *Client) ExecuteReportingRun(ctx context.Context, id string, in ReportingRunDispatch) (out ReportingRun, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting-runs/"+id+"/execute", "", in, &out, 64<<20)
	return
}

// ReadReportingRun calls the common registered domain operation with fresh caller authority.
func (c *Client) ReadReportingRun(ctx context.Context, id string) (out ReportingRun, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	err = c.callLimit(ctx, "GET", "/v1/reporting-runs/"+id+"", "", nil, &out, 64<<20)
	return
}

// InspectReportingRun calls the common registered domain operation with fresh caller authority.
func (c *Client) InspectReportingRun(ctx context.Context, id string) (out ReportingRun, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	err = c.callLimit(ctx, "GET", "/v1/reporting-runs/"+id+"/receipt", "", nil, &out, 64<<20)
	return
}

// CancelReportingRun calls the common registered domain operation with fresh caller authority.
func (c *Client) CancelReportingRun(ctx context.Context, id string) (out ReportingRun, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting-runs/"+id+"/cancel", "", struct{}{}, &out, 64<<20)
	return
}

// ProposeEngineering calls the common registered domain operation with fresh caller authority.
func (c *Client) ProposeEngineering(ctx context.Context, in EngineeringGoal) (out EngineeringProposal, err error) {
	err = c.callLimit(ctx, "POST", "/v1/engineering-proposals", "", in, &out, 64<<20)
	return
}

// ReadEngineeringProposal calls the common registered domain operation with fresh caller authority.
func (c *Client) ReadEngineeringProposal(ctx context.Context, id string) (out EngineeringProposal, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	err = c.callLimit(ctx, "GET", "/v1/engineering-proposals/"+id+"", "", nil, &out, 64<<20)
	return
}

// EditEngineeringProposal calls the common registered domain operation with fresh caller authority.
func (c *Client) EditEngineeringProposal(ctx context.Context, id string, in EngineeringProposalEdit) (out EngineeringProposal, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	err = c.callLimit(ctx, "PUT", "/v1/engineering-proposals/"+id+"", "", in, &out, 64<<20)
	return
}

// ReviewEngineeringProposal calls the common registered domain operation with fresh caller authority.
func (c *Client) ReviewEngineeringProposal(ctx context.Context, id string, in EngineeringProposalReview) (out EngineeringProposal, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/engineering-proposals/"+id+"/review", "", in, &out, 64<<20)
	return
}

// ApplyEngineeringProposal calls the common registered domain operation with fresh caller authority.
func (c *Client) ApplyEngineeringProposal(ctx context.Context, id string, in EngineeringProposalApply) (out EngineeringProposal, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/engineering-proposals/"+id+"/apply", "", in, &out, 64<<20)
	return
}

// CompensateEngineeringProposal calls the common registered domain operation with fresh caller authority.
func (c *Client) CompensateEngineeringProposal(ctx context.Context, id string, in EngineeringProposalApply) (out EngineeringProposal, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/engineering-proposals/"+id+"/compensate", "", in, &out, 64<<20)
	return
}

// DetectEngineeringDrift calls the common registered domain operation with fresh caller authority.
func (c *Client) DetectEngineeringDrift(ctx context.Context, id string) (out EngineeringDrift, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/engineering-proposals/"+id+"/drift", "", struct{}{}, &out, 64<<20)
	return
}

func (c *Client) ListReportingRuns(ctx context.Context, after string, limit int) (out ReportingArtifactList, err error) {
	if limit < 1 || limit > 100 || after != "" && !identity.Identifier(after) {
		return out, ErrBlockRequest
	}
	q := url.Values{"after": {after}, "limit": {strconv.Itoa(limit)}}
	err = c.callLimit(ctx, "GET", "/v1/reporting-runs?"+q.Encode(), "", nil, &out, 64<<20)
	return
}
func (c *Client) ReportingRunRows(ctx context.Context, id string, offset, limit int) (out ReportingResultPage, err error) {
	if !identity.Identifier(id) || offset < 0 || offset > 10000 || limit < 1 || limit > 1000 {
		return out, ErrBlockRequest
	}
	q := url.Values{"offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(limit)}}
	err = c.callLimit(ctx, "GET", "/v1/reporting-runs/"+id+"/rows?"+q.Encode(), "", nil, &out, 64<<20)
	return
}
func (c *Client) ReportingRunOutput(ctx context.Context, id, output string) (out ReportingRetainedOutput, err error) {
	if !identity.Identifier(id) || !identity.Identifier(output) {
		return out, ErrBlockRequest
	}
	q := url.Values{"output": {output}}
	err = c.callLimit(ctx, "GET", "/v1/reporting-runs/"+id+"/output?"+q.Encode(), "", nil, &out, 64<<20)
	return
}
func (c *Client) ExpireReportingArtifacts(ctx context.Context, limit int) (removed int64, err error) {
	if limit < 1 || limit > 1000 {
		return 0, ErrBlockRequest
	}
	in := struct {
		Limit int `json:"limit"`
	}{limit}
	var out struct {
		Removed int64 `json:"removed"`
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting-retention", "", in, &out, 64<<20)
	return out.Removed, err
}
