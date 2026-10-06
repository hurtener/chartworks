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

// ReportDataDependencies discovers publication or original preparation coordinates.
// This metadata-only method does not grant content access or execute source work.
type ReportDataDependencyRequest = reporting.DataDependencyRequest
type ReportDataDependencyManifest = reporting.DataDependencyManifest

func (c *Client) ReportDataDependencies(ctx context.Context, in ReportDataDependencyRequest) (out ReportDataDependencyManifest, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.DataDependencyDiscoveryPath, "", in, &out, 128<<10)
	return
}

// ReportEffectDependencies discovers saved validation, preview and run requirements.
type ReportEffectDependencyRequest = reporting.EffectDependencyRequest
type ReportEffectDependencyManifest = reporting.EffectDependencyManifest

func (c *Client) ReportEffectDependencies(ctx context.Context, in ReportEffectDependencyRequest) (out ReportEffectDependencyManifest, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.EffectDependencyDiscoveryPath, "", in, &out, 128<<10)
	return
}

// ReportRunCandidates returns BFF-only coordinates, never content authority.
type ReportRunCandidateRequest = reporting.RunCandidateRequest
type ReportRunCandidatePage = reporting.RunCandidatePage

func (c *Client) ReportRunCandidates(ctx context.Context, in ReportRunCandidateRequest) (out ReportRunCandidatePage, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.RunCandidateDiscoveryPath, "", in, &out, 128<<10)
	return
}

type ReportOptionDependencyRequest = reporting.OptionDependencyRequest
type ReportOptionDependencyManifest = reporting.OptionDependencyManifest

// ReportOptionDependencies reads target/original custody metadata without source work.
func (c *Client) ReportOptionDependencies(ctx context.Context, in ReportOptionDependencyRequest) (out ReportOptionDependencyManifest, err error) {
	err = c.callLimit(ctx, "POST", reportingapi.OptionDependencyDiscoveryPath, "", in, &out, 128<<10)
	return
}
