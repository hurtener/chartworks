package reporting

import (
	"context"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/identity"
)

// AuthoringBlockPresentationRequest admits only display overrides for one exact
// selected output. Canonical columns, bindings and execution remain server-owned.
type AuthoringBlockPresentationRequest struct {
	Block           string                   `json:"block"`
	ExpectedVersion int64                    `json:"expected_version" jsonschema:"minimum=1"`
	Revision        int64                    `json:"revision" jsonschema:"minimum=1,maximum=256"`
	Digest          string                   `json:"digest"`
	Output          string                   `json:"output"`
	Presentation    charts.PresentationPatch `json:"presentation"`
}

// AuthoringBlockPresentationCopyRequest requires an independently authorized
// target, exactly like mapping-copy. Display edits never imply source-write reach.
type AuthoringBlockPresentationCopyRequest struct {
	Block           string                   `json:"block"`
	ExpectedVersion int64                    `json:"expected_version" jsonschema:"minimum=1"`
	Revision        int64                    `json:"revision" jsonschema:"minimum=1,maximum=256"`
	Digest          string                   `json:"digest"`
	Output          string                   `json:"output"`
	NewBlock        string                   `json:"new_block"`
	Presentation    charts.PresentationPatch `json:"presentation"`
}

func amendAuthoringPresentation(ctx context.Context, base Definition, output string, patch charts.PresentationPatch, blocks *Service) (Definition, error) {
	d := clone(base)
	for i := range d.Outputs {
		o := &d.Outputs[i]
		if o.ID != output {
			continue
		}
		if o.Mapping == nil || o.Narrative != nil || o.Kind == "narrative" {
			return Definition{}, ErrInvalid
		}
		mapping, err := charts.ApplyPresentationPatch(ctx, *o.Mapping, patch, chartLimits(blocks.limits))
		if err != nil {
			if ctx.Err() != nil {
				return Definition{}, ctx.Err()
			}
			// A no-op is rejected before creating a revision; it never transfers
			// old validation to a purported new display amendment.
			return Definition{}, ErrInvalid
		}
		o.Mapping = &mapping
		if err := validateDefinition(ctx, d, blocks.limits, true); err != nil {
			return Definition{}, err
		}
		return d, nil
	}
	return Definition{}, access.ErrNotFound
}

func (s *Authoring) PatchBlockPresentation(ctx context.Context, e identity.Envelope, in AuthoringBlockPresentationRequest) (AuthoringBlockView, error) {
	coordinates := AuthoringBlockMappingRequest{Block: in.Block, ExpectedVersion: in.ExpectedVersion, Revision: in.Revision, Digest: in.Digest, Output: in.Output}
	return s.patchBlockDefinition(ctx, e, coordinates, func(ctx context.Context, d Definition, blocks *Service) (Definition, error) {
		return amendAuthoringPresentation(ctx, d, in.Output, in.Presentation, blocks)
	})
}

func (s *Authoring) CopyBlockPresentation(ctx context.Context, e identity.Envelope, in AuthoringBlockPresentationCopyRequest) (AuthoringBlockView, error) {
	coordinates := AuthoringBlockCopyRequest{Block: in.Block, ExpectedVersion: in.ExpectedVersion, Revision: in.Revision, Digest: in.Digest, Output: in.Output, NewBlock: in.NewBlock}
	return s.copyBlockDefinition(ctx, e, coordinates, func(ctx context.Context, d Definition, blocks *Service) (Definition, error) {
		return amendAuthoringPresentation(ctx, d, in.Output, in.Presentation, blocks)
	})
}
