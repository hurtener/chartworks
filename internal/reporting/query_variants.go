package reporting

import (
	"context"
	"github.com/hurtener/chartworks/internal/identity"
)

// QueryVariantReference selects an explicitly published captured-query revision.
// CaptureDigest is content-only provenance, never source-query read authority.
type QueryVariantReference struct {
	Block         string   `json:"block"`
	Revision      int64    `json:"revision"`
	Digest        string   `json:"digest"`
	CaptureDigest string   `json:"capture_digest"`
	Outputs       []string `json:"outputs" wire:"optional"`
}

// QueryVariantRequest prepares an exact published revision, without mutation.
type QueryVariantRequest struct {
	Block    string   `json:"block"`
	Revision int64    `json:"revision"`
	Outputs  []string `json:"outputs" wire:"optional"`
}

// QueryVariantDescriptor is report-ready metadata; original SQL, query IDs and
// historical parameters remain in the protected captured-block provenance.
type QueryVariantDescriptor struct {
	Query      QueryWidget `json:"query"`
	Parameters []Parameter `json:"parameters"`
}

// IsCapturedQueryVariant distinguishes frozen captured intent from dynamic questions.
func IsCapturedQueryVariant(w Widget) bool {
	return w.Kind == "query" && w.Query != nil && w.Query.Durability == "captured_variant"
}
func validVariantReference(p *QueryVariantReference) bool {
	return p != nil && identity.Identifier(p.Block) && p.Revision > 0 && p.Revision <= 256 && hashValid(p.Digest) && hashValid(p.CaptureDigest) && len(p.Outputs) <= 64 && (p.Outputs == nil || len(p.Outputs) > 0)
}
func validVariantQuery(q QueryWidget) bool {
	return q.Durability == "captured_variant" && validVariantReference(q.Variant) && q.Context == "" && len(q.Topics) == 0 && q.Question == "" && q.Query == "" && q.Selections == nil
}

// CapturedVariantBlock lowers only a closed immutable reference. It grants no
// authority: CheckCapturedQueryVariant must be called against admitted storage.
func CapturedVariantBlock(w Widget) (Widget, error) {
	if !IsCapturedQueryVariant(w) || !validVariantQuery(*w.Query) || w.Block != nil || w.Text != nil {
		return Widget{}, ErrInvalid
	}
	p := w.Query.Variant
	out := clone(w)
	out.Kind = "block"
	out.Query = nil
	out.Block = &BlockWidget{Block: p.Block, Revision: p.Revision, Outputs: clone(p.Outputs), Policy: "published"}
	return out, nil
}

// CheckCapturedQueryVariant is repeated within document and run admission.
// Only independently published captured content can become a reusable variant.
func CheckCapturedQueryVariant(p *QueryVariantReference, s Snapshot) error {
	if !validVariantReference(p) || s.State.ID != p.Block || s.Revision.Number != p.Revision || s.Revision.Digest != p.Digest || s.PublishedAt == nil || s.State.Archived || !s.Current || s.Revision.Provenance.CaptureDigest != p.CaptureDigest {
		return ErrStale
	}
	if _, _, err := ResolveOutputSelection(s.Revision.Definition, p.Outputs); err != nil {
		return err
	}
	return nil
}

// PrepareQueryVariant returns the closed report-widget contract after the normal
// capture, optional parameterization, validation and publication workflow.
func (s *Service) PrepareQueryVariant(ctx context.Context, e identity.Envelope, id string, in QueryVariantRequest) (QueryVariantDescriptor, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Read)
	if err != nil {
		return QueryVariantDescriptor{}, err
	}
	defer cancel()
	if in.Revision < 1 || in.Revision > 256 || len(in.Outputs) > 64 {
		return QueryVariantDescriptor{}, ErrInvalid
	}
	snapshot, err := s.repo.ReadBlock(ctx, e, id, Reference{Revision: in.Revision}, Read)
	if err != nil {
		return QueryVariantDescriptor{}, err
	}
	p := &QueryVariantReference{Block: id, Revision: in.Revision, Digest: snapshot.Revision.Digest, CaptureDigest: snapshot.Revision.Provenance.CaptureDigest, Outputs: clone(in.Outputs)}
	if err = CheckCapturedQueryVariant(p, snapshot); err != nil {
		return QueryVariantDescriptor{}, err
	}
	return QueryVariantDescriptor{Query: QueryWidget{Durability: "captured_variant", Variant: p}, Parameters: clone(snapshot.Revision.Definition.Parameters)}, nil
}

// PrepareQueryVariant uses the existing block lifecycle through delivery surfaces.
func (s *Delivery) PrepareQueryVariant(ctx context.Context, e identity.Envelope, in QueryVariantRequest) (QueryVariantDescriptor, error) {
	if s == nil || s.blocks == nil {
		return QueryVariantDescriptor{}, ErrUnavailable
	}
	return s.blocks.PrepareQueryVariant(ctx, e, in.Block, in)
}
