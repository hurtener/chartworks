package chartworks

import (
	"context"
	"errors"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
)

type ClarificationInput = semantics.ClarificationInput
type ClarificationPreviewRequest = rulesets.ClarificationPreviewRequest
type ClarificationPreview = rulesets.ClarificationPreview
type ClarificationExportRequest = rulesets.ClarificationExportRequest
type PortableClarifications = rulesets.PortableClarifications
type ClarificationImportRequest = rulesets.ClarificationImportRequest
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

func (c *Client) ExportClarifications(ctx context.Context, topic string, in ClarificationExportRequest) (out PortableClarifications, err error) {
	if !wireID(topic) {
		return out, errors.New("chartworks: invalid topic")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+topic+"/clarifications/export", "", in, &out, 2<<20)
	return
}

func (c *Client) PreviewClarificationImport(ctx context.Context, topic string, in ClarificationImportRequest) (out ClarificationImportPreview, err error) {
	if !wireID(topic) {
		return out, errors.New("chartworks: invalid topic")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+topic+"/clarifications/import-preview", "", in, &out, 2<<20)
	return
}
