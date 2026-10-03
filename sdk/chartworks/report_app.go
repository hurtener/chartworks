package chartworks

import (
	"context"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

// Report app requests share the HTTP/MCP schemas; none carries identity or grants.
type ReportAppCapabilitiesRequest = reporting.AuthoringCapabilitiesRequest
type ReportAppCapabilities = reporting.AuthoringCapabilities
type ReportAppDraftListRequest = reporting.DraftListRequest
type ReportAppDraftList = reporting.DraftList
type ReportAppReadRequest = reporting.AuthoringReadRequest
type ReportAppCreateRequest = reporting.AuthoringCreateRequest
type ReportAppSaveRequest = reporting.AuthoringSaveRequest
type ReportAppPreviewRequest = reporting.AuthoringPreviewRequest
type ReportAppExecuteRequest = reporting.AuthoringExecuteRequest
type ReportAppBootstrapRequest = reportingapi.ReportAppBootstrapRequest
type ReportAppBootstrap = reportingapi.ReportAppBootstrap

// ReportCapabilities returns current hints; callers must not treat hints as grants.
func (c *Client) ReportCapabilities(ctx context.Context, in ReportAppCapabilitiesRequest) (out ReportAppCapabilities, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"capabilities", "", in, &out, 4<<20)
	return
}

// ReportDrafts lists currently authorized private draft metadata.
func (c *Client) ReportDrafts(ctx context.Context, in ReportAppDraftListRequest) (out ReportAppDraftList, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"drafts", "", in, &out, 4<<20)
	return
}

// OpenReportDraft reads an exact private revision under separate read/preview reach.
func (c *Client) OpenReportDraft(ctx context.Context, in ReportAppReadRequest) (out DocumentView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"read", "", in, &out, 4<<20)
	return
}

// CreateManualReport creates a bounded manual private draft without automatic retry.
func (c *Client) CreateManualReport(ctx context.Context, in ReportAppCreateRequest) (out DocumentState, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"create", "", in, &out, 4<<20)
	return
}

// SaveManualReport appends an explicit whole-report revision under expected CAS.
func (c *Client) SaveManualReport(ctx context.Context, in ReportAppSaveRequest) (out DocumentState, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"save", "", in, &out, 4<<20)
	return
}

// PreviewManualReport reserves an exact private preview, without dispatching it.
func (c *Client) PreviewManualReport(ctx context.Context, in ReportAppPreviewRequest) (out CompositionView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"preview", "", in, &out, 4<<20)
	return
}

// ExecuteManualPreview explicitly dispatches one private frozen-only preview.
func (c *Client) ExecuteManualPreview(ctx context.Context, in ReportAppExecuteRequest) (out CompositionView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"execute", "", in, &out, 4<<20)
	return
}

// BootstrapReportApp reads typed intent guidance and exact target capability hints.
func (c *Client) BootstrapReportApp(ctx context.Context, in ReportAppBootstrapRequest) (out ReportAppBootstrap, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"bootstrap", "", in, &out, 4<<20)
	return
}

// ReportWidgetPatch permits only selected-widget text and presentation edits.
type ReportWidgetPatch = reporting.WidgetPatch
type ReportWidgetPatchRequest = reporting.AuthoringWidgetRequest

// PatchReportWidget preserves every unselected field server-side under draft CAS.
func (c *Client) PatchReportWidget(ctx context.Context, in ReportWidgetPatchRequest) (out DocumentState, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"widget", "", in, &out, 4<<20)
	return
}

// ReportAppGuide is versioned public workflow guidance, never authority.
type ReportAppGuide = reportingapi.ReportAppGuide

// ReadReportAppGuide reads the same compiled guidance exposed as an MCP resource.
func (c *Client) ReadReportAppGuide(ctx context.Context) (out ReportAppGuide, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"guide", "", struct{}{}, &out, 4<<20)
	return
}
