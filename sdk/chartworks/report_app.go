package chartworks

import (
	"context"

	"github.com/hurtener/chartworks/internal/charts"
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

// Manual chart requests carry neither SQL, rows, column definitions nor grants.
type ReportAppBlockReadRequest = reporting.AuthoringBlockReadRequest
type ReportAppBlockMappingRequest = reporting.AuthoringBlockMappingRequest
type ReportAppBlockCopyRequest = reporting.AuthoringBlockCopyRequest
type ReportAppChartMapping = reporting.AuthoringChartMapping
type ReportAppBlockView = reporting.AuthoringBlockView
type ReportAppBlockValidateRequest = reporting.AuthoringBlockValidateRequest
type ReportAppBlockValidationResult = reporting.AuthoringBlockValidationResult

// ReadManualChart returns an exact SQL-free revision and server-owned candidates.
func (c *Client) ReadManualChart(ctx context.Context, in ReportAppBlockReadRequest) (out ReportAppBlockView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"block_read", "", in, &out, 4<<20)
	return
}

// AmendManualChart changes one private output under exact baseline CAS.
func (c *Client) AmendManualChart(ctx context.Context, in ReportAppBlockMappingRequest) (out ReportAppBlockView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"block_mapping", "", in, &out, 4<<20)
	return
}

// CopyManualChart creates an independently authorized private copy, not approval.
func (c *Client) CopyManualChart(ctx context.Context, in ReportAppBlockCopyRequest) (out ReportAppBlockView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"block_copy", "", in, &out, 4<<20)
	return
}

// Presentation requests intentionally cannot carry a mapping or column metadata.
// Optional set pointers preserve explicit zero digits and an empty display label;
// reset names restore reviewed defaults. Capability metadata gates supported fields.
type ReportAppBlockPresentationRequest = reporting.AuthoringBlockPresentationRequest
type ReportAppBlockPresentationCopyRequest = reporting.AuthoringBlockPresentationCopyRequest
type ReportAppPresentationPatch = charts.PresentationPatch
type ReportAppColumnPresentationEdit = charts.ColumnPresentationEdit
type ReportAppColumnPresentationSet = charts.ColumnPresentationSet
type ReportAppPresentationField = charts.PresentationField
type ReportAppPresentationCapabilities = charts.PresentationCapabilities
type ReportAppColumnPresentationCapability = charts.ColumnPresentationCapability

const (
	ReportAppPresentationVersion        = charts.PresentationVersion
	ReportAppPresentationDisplayLabel   = charts.PresentationDisplayLabel
	ReportAppPresentationFractionDigits = charts.PresentationFractionDigits
)

// AmendManualChartPresentation changes only selected display overrides under CAS.
// It uses the existing block_mapping route and never retries an unknown outcome.
func (c *Client) AmendManualChartPresentation(ctx context.Context, in ReportAppBlockPresentationRequest) (out ReportAppBlockView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"block_mapping", "", in, &out, 4<<20)
	return
}

// CopyManualChartPresentation creates an authorized private unvalidated copy with
// one output's display overrides changed, using the existing block_copy route.
func (c *Client) CopyManualChartPresentation(ctx context.Context, in ReportAppBlockPresentationCopyRequest) (out ReportAppBlockView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"block_copy", "", in, &out, 4<<20)
	return
}

// ValidateManualChart performs one explicit bounded read without automatic retry.
func (c *Client) ValidateManualChart(ctx context.Context, in ReportAppBlockValidateRequest) (out ReportAppBlockValidationResult, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"block_validate", "", in, &out, 4<<20)
	return
}

// Dataset-first authoring uses reviewed semantics and explicit native reads.
type ReportAppSourceDatasetPin = reporting.SourceDatasetPin

type ReportAppDatasetRequest = reporting.AuthoringDatasetRequest
type ReportAppDatasetView = reporting.AuthoringDatasetView
type ReportAppFieldSelection = reporting.AuthoringFieldSelection
type ReportAppGrouping = reporting.AuthoringGrouping
type ReportAppMeasureSelection = reporting.AuthoringMeasureSelection
type ReportAppFieldCatalog = reporting.AuthoringFieldCatalog
type ReportAppPrepareRequest = reporting.AuthoringPrepareRequest
type ReportAppPreparationRequest = reporting.AuthoringPreparationRequest
type ReportAppPreparationView = reporting.AuthoringPreparationView
type ReportAppCreatePreparedRequest = reporting.AuthoringCreatePreparedRequest
type ReportAppPreparationControlRequest = reporting.AuthoringPreparationControlRequest

func (c *Client) ReadManualDataset(ctx context.Context, in ReportAppDatasetRequest) (out ReportAppDatasetView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"dataset", "", in, &out, 4<<20)
	return
}

func (c *Client) PrepareManualChart(ctx context.Context, in ReportAppPrepareRequest) (out ReportAppPreparationView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"prepare_chart", "", in, &out, 4<<20)
	return
}

func (c *Client) ReadManualPreparation(ctx context.Context, in ReportAppPreparationRequest) (out ReportAppPreparationView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"preparation", "", in, &out, 4<<20)
	return
}

func (c *Client) CreatePreparedManualChart(ctx context.Context, in ReportAppCreatePreparedRequest) (out ReportAppBlockView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"create_prepared", "", in, &out, 4<<20)
	return
}

func (c *Client) ControlManualPreparation(ctx context.Context, in ReportAppPreparationControlRequest) (out ReportAppPreparationView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"preparation_control", "", in, &out, 4<<20)
	return
}

// ReportAppDocumentationRequest selects one advertised exact versioned resource.
type ReportAppDocumentationRequest = reportingapi.ReportAppDocumentationRequest
type ReportAppDocumentation = reportingapi.ReportAppDocumentation

// ReadReportAppDocumentation reads full public contract text and its digest.
// It shares the HTTP/MCP core, makes one request and does not load a local path.
func (c *Client) ReadReportAppDocumentation(ctx context.Context, in ReportAppDocumentationRequest) (out ReportAppDocumentation, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"documentation", "", in, &out, 4<<20)
	return
}
