package reporting

import (
	"context"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

// Search may discover a new target; status/control require original custody.
// No search text, cursor, definition, source query or values enter this seam.
type OptionDependencyRequest struct {
	Mode      string                `json:"mode" jsonschema:"enum=search,enum=status,enum=control"`
	Target    AuthoringOptionTarget `json:"target"`
	Operation string                `json:"operation"`
}
type OptionDependencyManifest struct {
	Version    string                `json:"version"`
	Target     AuthoringOptionTarget `json:"target"`
	Operation  string                `json:"operation"`
	Original   bool                  `json:"original"`
	Actions    []string              `json:"actions"`
	References []ResourceReference   `json:"references"`
}
type OptionDependencyRepository interface {
	DiscoverAuthoringOptionDependencies(context.Context, identity.Envelope, OptionDependencyRequest) (OptionDependencyManifest, error)
}

func RequireOptionDependencyDiscovery(e identity.Envelope, in OptionDependencyRequest) error {
	if err := requireAuthoringEnvelope(e); err != nil {
		return err
	}
	if !slices.Contains([]string{"search", "status", "control"}, in.Mode) || !AuthoringOptionOperationValid(in.Operation, time.Now(), false) || (in.Target.Dataset == nil) == (in.Target.Report == nil) {
		return ErrInvalid
	}
	refs := []ResourceReference{}
	previewKind, previewID := "", ""
	if d := in.Target.Dataset; d != nil {
		if !identity.Identifier(d.NewBlock) || !identity.Identifier(d.Dataset) || !identity.Identifier(d.Dimension) || !identity.Identifier(d.Topic.Topic) || !identity.Identifier(d.Topic.Version) || !hashValid(d.Topic.Digest) {
			return ErrInvalid
		}
		refs = append(refs, ResourceReference{Kind: "topic", Permission: "read", ID: d.Topic.Topic}, ResourceReference{Kind: "block", Permission: "read", ID: d.NewBlock}, ResourceReference{Kind: "block", Permission: "write", ID: d.NewBlock})
		previewKind, previewID = "block", d.NewBlock
	} else {
		r := in.Target.Report
		if !slices.Contains([]string{"private_preview", "published"}, r.Policy) || !identity.Identifier(r.Report) || r.Revision < 1 || r.Revision > 256 || !hashValid(r.Digest) || !identity.Identifier(r.Page) || !identity.Identifier(r.Filter) {
			return ErrInvalid
		}
		refs = append(refs, ResourceReference{Kind: "report", Permission: "read", ID: r.Report})
		if r.Policy == "private_preview" {
			previewKind, previewID = "report", r.Report
		}
	}
	for _, ref := range refs {
		if err := access.Require(e, DependencyDiscoveryAction, access.Resource{Tenant: e.Tenant(), Kind: ref.Kind, Permission: ref.Permission, ID: ref.ID}); err != nil {
			return err
		}
	}
	if previewKind != "" {
		return access.Require(e, "reporting.preview", access.Resource{Tenant: e.Tenant(), Kind: previewKind, Permission: "preview", ID: previewID})
	}
	return nil
}

func (s *Authoring) OptionDependencies(ctx context.Context, e identity.Envelope, in OptionDependencyRequest) (OptionDependencyManifest, error) {
	if s == nil || ctx == nil {
		return OptionDependencyManifest{}, ErrInvalid
	}
	if err := RequireOptionDependencyDiscovery(e, in); err != nil {
		return OptionDependencyManifest{}, err
	}
	repo, ok := s.documents.repo.(OptionDependencyRepository)
	if !ok {
		return OptionDependencyManifest{}, ErrUnavailable
	}
	return repo.DiscoverAuthoringOptionDependencies(ctx, e, in)
}
