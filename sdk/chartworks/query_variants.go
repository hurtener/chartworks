package chartworks

import (
	"context"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

// ReportingQueryVariantRequest selects exact reviewed captured content.
type ReportingQueryVariantRequest = reporting.QueryVariantRequest

// ReportingQueryVariantReference pins the immutable captured revision.
type ReportingQueryVariantReference = reporting.QueryVariantReference

// ReportingQueryVariantDescriptor contains report-ready metadata only.
type ReportingQueryVariantDescriptor = reporting.QueryVariantDescriptor

// PrepareCapturedQueryVariant returns a report-ready query widget after explicit
// captured-block validation and publication. It never publishes or executes SQL.
func (c *Client) PrepareCapturedQueryVariant(ctx context.Context, in ReportingQueryVariantRequest) (out ReportingQueryVariantDescriptor, err error) {
	if !identity.Identifier(in.Block) || in.Revision < 1 || in.Revision > 256 || len(in.Outputs) > 64 {
		return out, ErrBlockRequest
	}
	err = c.call(ctx, "POST", "/v1/reporting/query-variant", "", in, &out)
	return
}
