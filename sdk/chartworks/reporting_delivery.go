package chartworks

import (
	"context"
	"errors"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

// ErrReportingRequest rejects invalid resource coordinates before network I/O.
var ErrReportingRequest = errors.New("chartworks: invalid reporting request")

// ReportingVersion identifies the shared HTTP/MCP/Apps selected-result contract.
const ReportingVersion = reporting.DeliveryVersion

// ReportingTarget addresses an immutable published resource revision.
type ReportingTarget = reporting.DeliveryTarget

// ReportingSearchRequest searches one bounded published metadata page.
type ReportingSearchRequest = reporting.ReportingSearchRequest

// ReportingResource omits SQL, credentials and retained values.
type ReportingResource = reporting.ReportingResource

// ReportingSearchResult is a permission-filtered metadata page.
type ReportingSearchResult = reporting.ReportingSearchResult

// ReportingDescribeRequest selects published outputs and business filters.
type ReportingDescribeRequest = reporting.ReportingDescribeRequest

// ReportingDescription never includes executable SQL or narrative instructions.
type ReportingDescription = reporting.ReportingDescription

// ReportingOutputChoice preserves a stable output identifier and order.
type ReportingOutputChoice = reporting.ViewerOutputChoice

// ReportingFilter describes a business input, never an authorization predicate.
type ReportingFilter = reporting.ViewerFilter

// ReportingRunRequest explicitly requests a new authorized execution.
type ReportingRunRequest = reporting.ReportingRunRequest

// ReportingRunResult points to an ordinary retained artifact catalog entry.
type ReportingRunResult = reporting.ReportingRunResult

// ReportingRunsRequest selects a bounded retained metadata page.
type ReportingRunsRequest = reporting.ReportingRunsRequest

// ReportingRunsResult distinguishes artifact availability from execution status.
type ReportingRunsResult = reporting.ReportingRunsResult

// ReportingRunSummary contains no result values or credentials.
type ReportingRunSummary = reporting.ReportingRunSummary

// ReportingViewRequest selects one output or exact retained table page.
type ReportingViewRequest = reporting.ReportingViewRequest

// ReportingViewResult is identical to the MCP Apps provider payload.
type ReportingViewResult = reporting.ReportingViewResult

// ReportingViewerOutput identifies an exact projection separately from its full retained digest.
type ReportingViewerOutput = reporting.ViewerOutput

// ReportingViewerTable contains exact retained strings, units and scoped totals.
type ReportingViewerTable = reporting.ViewerTable

// ReportingViewerPage describes an exact row continuation, not a new query.
type ReportingViewerPage = reporting.ViewerPage

func validReportingKind(kind string) bool {
	return kind == "block" || kind == "report" || kind == "dashboard"
}

func validReportingTarget(target ReportingTarget) bool {
	return validReportingKind(target.Kind) && identity.Identifier(target.ID) && target.Revision >= 0
}

// SearchReporting reads authorized published metadata. It does not run data or models.
func (c *Client) SearchReporting(ctx context.Context, in ReportingSearchRequest) (out ReportingSearchResult, err error) {
	if in.Limit == 0 { in.Limit = 20 }
	if !validReportingKind(in.Kind) || in.Limit < 1 || in.Limit > 100 || in.After != "" && !identity.Identifier(in.After) || len(in.Query) > 256 {
		return out, ErrReportingRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting/search", "", in, &out, 4<<20)
	return
}

// DescribeReporting reads public presentation metadata and typed filters only.
func (c *Client) DescribeReporting(ctx context.Context, in ReportingDescribeRequest) (out ReportingDescription, err error) {
	if !validReportingTarget(in.Target) || len(in.Outputs) > 64 { return out, ErrReportingRequest }
	err = c.callLimit(ctx, "POST", "/v1/reporting/describe", "", in, &out, 4<<20)
	return
}

// RunReporting may query sources, spend model tokens and persist an artifact.
// The exact published revision and fresh signed authority are required. This
// POST is never automatically retried; inspect the catalog after an unknown
// outcome rather than submitting another key blindly.
func (c *Client) RunReporting(ctx context.Context, in ReportingRunRequest) (out ReportingRunResult, err error) {
	if !validReportingTarget(in.Target) || in.Target.Revision < 1 || !identity.Identifier(in.Key) {
		return out, ErrReportingRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting/run", "", in, &out, 4<<20)
	return
}

// ListReportingRuns reads currently authorized retained artifact metadata only.
func (c *Client) ListReportingRuns(ctx context.Context, in ReportingRunsRequest) (out ReportingRunsResult, err error) {
	if in.Limit == 0 { in.Limit = 20 }
	if !validReportingKind(in.Kind) || in.Limit < 1 || in.Limit > 100 || in.After != "" && !identity.Identifier(in.After) || in.Resource != "" && !identity.Identifier(in.Resource) {
		return out, ErrReportingRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting/runs", "", in, &out, 4<<20)
	return
}

// ViewReporting reads one exact retained output. Pagination and redraw do not
// change filters or invoke source/model execution. Every page rechecks authority.
func (c *Client) ViewReporting(ctx context.Context, in ReportingViewRequest) (out ReportingViewResult, err error) {
	if !validReportingKind(in.Kind) || !identity.Identifier(in.Run) || in.Page != "" && !identity.Identifier(in.Page) || in.Widget != "" && !identity.Identifier(in.Widget) || in.Output != "" && !identity.Identifier(in.Output) || in.Offset < 0 || in.Offset > 100000 || in.Limit < 0 || in.Limit > 1000 {
		return out, ErrReportingRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting/view", "", in, &out, 4<<20)
	return
}
