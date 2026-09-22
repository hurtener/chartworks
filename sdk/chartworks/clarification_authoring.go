package chartworks

import (
	"context"
	"errors"

	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
)

// ClarificationInput is a bounded synthetic clarification-preview case.
type ClarificationInput = semantics.ClarificationInput

// ClarificationPreviewRequest pairs a draft definition with synthetic cases.
type ClarificationPreviewRequest = rulesets.ClarificationPreviewRequest

// ClarificationPreview contains deterministic effects and case outcomes.
type ClarificationPreview = rulesets.ClarificationPreview

// ClarificationExportRequest selects an exact reviewed ruleset version.
type ClarificationExportRequest = rulesets.ClarificationExportRequest

// PortableClarifications preserves rule digests and migration dispositions.
type PortableClarifications = rulesets.PortableClarifications

// ClarificationImportRequest proposes an exact-topic portable-pack import.
type ClarificationImportRequest = rulesets.ClarificationImportRequest

// ClarificationImportPreview remains subject to ordinary review and publication.
type ClarificationImportPreview = rulesets.ClarificationImportPreview

// PreviewClarifications evaluates bounded synthetic cases through the same
// reviewed compiler as routing. It cannot activate a draft or issue a plan.
func (c *Client) PreviewClarifications(ctx context.Context, topic string, in ClarificationPreviewRequest) (out ClarificationPreview, err error) {
	if !wireID(topic) {
		return out, errors.New("chartworks: invalid topic")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+topic+"/clarifications/preview", "", in, &out, 2<<20)
	return
}

// ExportClarifications reads an exact retained pack under current export reach.
func (c *Client) ExportClarifications(ctx context.Context, topic string, in ClarificationExportRequest) (out PortableClarifications, err error) {
	if !wireID(topic) {
		return out, errors.New("chartworks: invalid topic")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+topic+"/clarifications/export", "", in, &out, 2<<20)
	return
}

// PreviewClarificationImport validates a proposed import without publishing it.
func (c *Client) PreviewClarificationImport(ctx context.Context, topic string, in ClarificationImportRequest) (out ClarificationImportPreview, err error) {
	if !wireID(topic) {
		return out, errors.New("chartworks: invalid topic")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+topic+"/clarifications/import-preview", "", in, &out, 2<<20)
	return
}
