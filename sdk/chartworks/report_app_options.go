package chartworks

import (
	"context"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

// Exact logical option coordinates never carry SQL, identity or physical names.
type ReportAppDatasetOptionTarget = reporting.AuthoringDatasetOptionTarget
type ReportAppReportOptionTarget = reporting.AuthoringReportOptionTarget
type ReportAppOptionTarget = reporting.AuthoringOptionTarget
type ReportAppOptionRequest = reporting.AuthoringOptionRequest
type ReportAppOptionReference = reporting.AuthoringOptionReference
type ReportAppOptionControlRequest = reporting.AuthoringOptionControlRequest
type ReportAppOptionView = reporting.AuthoringOptionView

// SearchManualDatasetOptions performs one explicit bounded source read. Generate
// a fresh option:<UnixSeconds>:<32 lowercase hex> key only for deliberate Search.
// Preserve that key after an unknown result; never automatically requery.
func (c *Client) SearchManualDatasetOptions(ctx context.Context, in ReportAppOptionRequest) (out ReportAppOptionView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"dataset_options", "", in, &out, 4<<20)
	return
}

// SearchReportFilterOptions searches one exact saved page/filter under explicit
// private_preview or published policy. Changing text or a selection is not Search.
func (c *Client) SearchReportFilterOptions(ctx context.Context, in ReportAppOptionRequest) (out ReportAppOptionView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"report_options", "", in, &out, 4<<20)
	return
}

// ReadReportAppOptionStatus inspects original custody without source work.
// ValuesAvailable=false is not an empty page. NewOperationAllowed only permits a
// separately explicit search and never authorizes an automatic retry.
func (c *Client) ReadReportAppOptionStatus(ctx context.Context, in ReportAppOptionReference) (out ReportAppOptionView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"option_status", "", in, &out, 4<<20)
	return
}

// ControlReportAppOptions explicitly cancels or reconciles one original source
// attempt. It mutates custody and may contact source control, but never requeries.
func (c *Client) ControlReportAppOptions(ctx context.Context, in ReportAppOptionControlRequest) (out ReportAppOptionView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"option_control", "", in, &out, 4<<20)
	return
}
