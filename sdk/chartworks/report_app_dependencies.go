package chartworks

import (
	"context"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

type ReportDependencyRequest = reporting.DependencyRequest
type ReportDependencyManifest = reporting.DependencyManifest

// ReportDependencies reads metadata for the Pengui BFF before its independent
// policy decision. The response is neither authority nor executable content.
func (c *Client) ReportDependencies(ctx context.Context, in ReportDependencyRequest) (out ReportDependencyManifest, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.DependencyDiscoveryPath, "", in, &out, 128<<10)
	return
}
