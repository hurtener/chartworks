package chartworks

import (
	"context"
	"errors"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/rendering"
	"github.com/hurtener/chartworks/internal/reporting"
)

// ErrReportingRequest rejects invalid resource coordinates before network I/O.
var ErrReportingRequest = errors.New("chartworks: invalid reporting request")

// ReportingVersion identifies the shared HTTP/MCP/Apps selected-result contract.
const ReportingVersion = reporting.DeliveryVersion

// ReportingTarget addresses an immutable published resource revision.
type ReportingTarget = reporting.DeliveryTarget

// ReportingSearchRequest searches one bounded published metadata page.
type ReportingSearchRequest = reporting.DeliverySearchRequest

// ReportingResource omits SQL, credentials and retained values.
type ReportingResource = reporting.DeliveryResource

// ReportingSearchResult is a permission-filtered metadata page.
type ReportingSearchResult = reporting.DeliverySearchResult

// ReportingDescribeRequest selects published outputs and business filters.
type ReportingDescribeRequest = reporting.DeliveryDescribeRequest

// ReportingDescription never includes executable SQL or narrative instructions.
type ReportingDescription = reporting.DeliveryDescription

// ReportingOutputChoice preserves a stable output identifier and order.
type ReportingOutputChoice = reporting.ViewerOutputChoice

// ReportingFilter describes a business input, never an authorization predicate.
type ReportingFilter = reporting.ViewerFilter

// ReportingFilterOptionsRequest selects choices for one exact published report filter.
type ReportingFilterOptionsRequest = reporting.DeliveryFilterOptionsRequest

// ReportingFilterOptionsPage preserves typed values and its authority-bound continuation.
type ReportingFilterOptionsPage = reporting.FilterOptionsPage

// ReportingDeliveryRunRequest explicitly requests a new authorized execution.
type ReportingDeliveryRunRequest = reporting.DeliveryRunRequest

// ReportingRunResult points to an ordinary retained artifact catalog entry.
type ReportingRunResult = reporting.DeliveryRunResult

// ReportingRunsRequest selects a bounded retained metadata page.
type ReportingRunsRequest = reporting.DeliveryRunsRequest

// ReportingRunsResult distinguishes artifact availability from execution status.
type ReportingRunsResult = reporting.DeliveryRunsResult

// ReportingRunSummary contains no result values or credentials.
type ReportingRunSummary = reporting.DeliveryRunSummary

// ReportingScheduledProvenance describes accepted windows and independent delivery stages.
type ReportingScheduledProvenance = reporting.ScheduledProvenance

// ReportingViewRequest selects one output or exact retained table page.
type ReportingViewRequest = reporting.DeliveryViewRequest

// ReportingViewResult is identical to the MCP Apps provider payload.
type ReportingViewResult = reporting.DeliveryViewResult

// ReportingViewerOutput identifies an exact projection separately from its full retained digest.
type ReportingViewerOutput = reporting.ViewerOutput

// ReportingViewerTable contains exact retained strings, units and scoped totals.
type ReportingViewerTable = reporting.ViewerTable

// ReportingViewerPage describes an exact row continuation, not a new query.
type ReportingViewerPage = reporting.ViewerPage

// ReportingExportRequest selects a sealed retained output and static format.
type ReportingExportRequest = rendering.Request

// ReportingRendition contains bounded static bytes and immutable provenance.
type ReportingRendition = rendering.Rendition

// ReportingRenditionReadRequest selects one durable rendition.
type ReportingRenditionReadRequest = rendering.ReadRequest

// ReportingRenditionListRequest selects one durable rendition page.
type ReportingRenditionListRequest = rendering.ListRequest

// ReportingRenditionListResult contains currently authorized renditions.
type ReportingRenditionListResult = rendering.ListResult

// ReportingRenditionExpireRequest bounds a rendition retention pass.
type ReportingRenditionExpireRequest = rendering.ExpireRequest

// ReportingRenditionExpireResult reports erased rendition bytes.
type ReportingRenditionExpireResult = rendering.ExpireResult

func validReportingKind(kind string) bool {
	return kind == "block" || kind == "report" || kind == "dashboard"
}

func validReportingTarget(target ReportingTarget) bool {
	return validReportingKind(target.Kind) && identity.Identifier(target.ID) && target.Revision >= 0
}

// SearchReporting reads authorized published metadata. It does not run data or models.
func (c *Client) SearchReporting(ctx context.Context, in ReportingSearchRequest) (out ReportingSearchResult, err error) {
	if in.Limit == 0 {
		in.Limit = 20
	}
	if !validReportingKind(in.Kind) || in.Limit < 1 || in.Limit > 100 || in.After != "" && !identity.Identifier(in.After) || len(in.Query) > 256 {
		return out, ErrReportingRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting/search", "", in, &out, 4<<20)
	return
}

// DescribeReporting reads public presentation metadata and typed filters only.
func (c *Client) DescribeReporting(ctx context.Context, in ReportingDescribeRequest) (out ReportingDescription, err error) {
	if !validReportingTarget(in.Target) || len(in.Outputs) > 64 {
		return out, ErrReportingRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting/describe", "", in, &out, 4<<20)
	return
}

// ReportingFilterOptions performs one bounded validated source read and never
// invokes a model. A continuation is valid only for the unchanged request and authority.
func (c *Client) ReportingFilterOptions(ctx context.Context, in ReportingFilterOptionsRequest) (out ReportingFilterOptionsPage, err error) {
	if !identity.Identifier(in.Report) || in.Revision < 1 || in.Revision > 256 || !identity.Identifier(in.Filter) || in.Limit < 1 || in.Limit > 200 || len(in.Search) > 256 || len(in.Cursor) > 16<<10 {
		return out, ErrReportingRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting/filter-options", "", in, &out, 2<<20)
	return
}

// RunReporting may query sources, spend model tokens and persist an artifact.
// The exact published revision and fresh signed authority are required. This
// POST is never automatically retried; inspect the catalog after an unknown
// outcome rather than submitting another key blindly.
func (c *Client) RunReporting(ctx context.Context, in ReportingDeliveryRunRequest) (out ReportingRunResult, err error) {
	if !validReportingTarget(in.Target) || in.Target.Revision < 1 || !identity.Identifier(in.Key) {
		return out, ErrReportingRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting/run", "", in, &out, 4<<20)
	return
}

// SearchReportingRuns reads currently authorized retained artifact metadata only.
func (c *Client) SearchReportingRuns(ctx context.Context, in ReportingRunsRequest) (out ReportingRunsResult, err error) {
	if in.Limit == 0 {
		in.Limit = 20
	}
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

// ExportReporting produces JSON, CSV, static HTML or static SVG from retained
// values only. It never runs a query or model and requires explicit export reach.
func (c *Client) ExportReporting(ctx context.Context, in ReportingExportRequest) (out ReportingRendition, err error) {
	if !validReportingKind(in.View.Kind) || !identity.Identifier(in.View.Run) || in.View.Page != "" && !identity.Identifier(in.View.Page) || in.View.Widget != "" && !identity.Identifier(in.View.Widget) || in.View.Output != "" && !identity.Identifier(in.View.Output) || in.View.Offset < 0 || in.View.Offset > 100000 || in.View.Limit < 0 || in.View.Limit > 1000 || !oneOfString(in.Format, "json", "csv", "html", "svg") || !oneOfString(in.Theme, "light", "dark") || in.Width < 320 || in.Width > 4096 || in.Height < 200 || in.Height > 4096 {
		return out, ErrReportingRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting/export", "", in, &out, 20<<20)
	return
}

// CreateReportingRendition idempotently persists one static retained export.
func (c *Client) CreateReportingRendition(ctx context.Context, in ReportingExportRequest) (out ReportingRendition, err error) {
	if !validReportingKind(in.View.Kind) || !identity.Identifier(in.View.Run) || !oneOfString(in.Format, "json", "csv", "html", "svg") {
		return out, ErrReportingRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting/renditions", "", in, &out, 20<<20)
	return
}

// ReadReportingRendition reads bytes after current artifact authority is checked.
func (c *Client) ReadReportingRendition(ctx context.Context, id string) (out ReportingRendition, err error) {
	if !identity.Identifier(id) {
		return out, ErrReportingRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting/renditions/read", "", rendering.ReadRequest{ID: id}, &out, 20<<20)
	return
}

// ListReportingRenditions lists currently authorized durable renditions.
func (c *Client) ListReportingRenditions(ctx context.Context, in ReportingRenditionListRequest) (out ReportingRenditionListResult, err error) {
	if in.Limit < 0 || in.Limit > 100 {
		return out, ErrReportingRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting/renditions/list", "", in, &out, 20<<20)
	return
}

// ExpireReportingRenditions executes one bounded retention deletion pass.
func (c *Client) ExpireReportingRenditions(ctx context.Context, limit int) (out ReportingRenditionExpireResult, err error) {
	if limit < 1 || limit > 1000 {
		return out, ErrReportingRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/reporting/renditions/expire", "", rendering.ExpireRequest{Limit: limit}, &out, 1<<20)
	return
}

func oneOfString(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
