package chartworks

import (
	"context"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

// Lifecycle DTOs share the HTTP/MCP domain contracts and carry no grants.
type ReportAppLifecycleRequest = reporting.AuthoringLifecycleRequest
type ReportAppLifecycleView = reporting.AuthoringLifecycleView
type ReportAppBlockLifecycle = reporting.AuthoringBlockLifecycle
type ReportAppBlockPublishRequest = reporting.AuthoringBlockPublishRequest
type ReportAppPublishedWidget = reporting.AuthoringPublishedWidget
type ReportAppRebindPublishedRequest = reporting.AuthoringRebindPublishedRequest
type ReportAppReportTransitionRequest = reporting.AuthoringReportTransitionRequest

// InspectManualLifecycle reads exact retained metadata and discloses all outputs
// of each entire block revision. Eligibility hints are not grants or audience
// counts. Recover unknown mutations with their original exact revision.
func (c *Client) InspectManualLifecycle(ctx context.Context, in ReportAppLifecycleRequest) (out ReportAppLifecycleView, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"lifecycle", "", in, &out, 4<<20)
	return
}

// PublishManualChart publishes the entire exact validated revision after explicit
// user confirmation, with no automatic retry, rebind, report publication, source
// or model work. It only changes eligibility for already-authorized readers.
func (c *Client) PublishManualChart(ctx context.Context, in ReportAppBlockPublishRequest) (out BlockState, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"block_publish", "", in, &out, 4<<20)
	return
}

// RebindPublishedManualCharts is a separately confirmed report CAS amendment of
// selected widget policies. Failure cannot undo already-completed publication.
// Retained private previews remain private; unknown outcomes require inspection.
func (c *Client) RebindPublishedManualCharts(ctx context.Context, in ReportAppRebindPublishedRequest) (out DocumentState, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"rebind_published", "", in, &out, 4<<20)
	return
}

// TransitionManualReport performs one review, publish or reject transition. Keep
// review separate from publication and confirm publication explicitly. Native
// publish/reject authority and rejection notes remain required in addition to the
// editor-entry ceiling. No automatic retry, source work or model work occurs.
func (c *Client) TransitionManualReport(ctx context.Context, in ReportAppReportTransitionRequest) (out DocumentState, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.AuthoringPath+"report_transition", "", in, &out, 4<<20)
	return
}
