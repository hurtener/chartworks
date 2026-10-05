package chartworks

import (
	"context"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

type ReportDependencyRequest = reporting.DependencyRequest
type ReportDependencyManifest = reporting.DependencyManifest
type ReportWriteDependencyRequest = reporting.WriteDependencyRequest
type ReportWriteDependencyManifest = reporting.WriteDependencyManifest

// ReportDependencies reads metadata for the Pengui BFF before its independent
// policy decision. The response is neither authority nor executable content.
func (c *Client) ReportDependencies(ctx context.Context, in ReportDependencyRequest) (out ReportDependencyManifest, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.DependencyDiscoveryPath, "", in, &out, 128<<10)
	return
}

// ReportWriteDependencies discovers native baseline and proposed requirements
// before the BFF independently authorizes a create or save operation.
func (c *Client) ReportWriteDependencies(ctx context.Context, in ReportWriteDependencyRequest) (out ReportWriteDependencyManifest, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.WriteDependencyDiscoveryPath, "", in, &out, 128<<10)
	return
}
