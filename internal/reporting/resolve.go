package reporting

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
)

// ResolveRequest resolves one exact authorized definition without a source call.
// It is not a run, validation receipt or grant to inspect another actor's draft.
type ResolveRequest struct {
	Reference  Reference  `json:"reference"`
	Arguments  []Argument `json:"arguments"`
	Resolution Resolution `json:"resolution"`
}

// ResolutionResult returns typed resolution evidence without executing a source query.
type ResolutionResult struct {
	ID         string   `json:"id"`
	Revision   int64    `json:"revision"`
	RevisionID string   `json:"revision_id"`
	Digest     string   `json:"digest"`
	Resolved   Resolved `json:"resolved"`
}

// Resolve resolves declared parameters for an eligible revision without touching the source.
func (s *Service) Resolve(ctx context.Context, e identity.Envelope, id string, in ResolveRequest) (ResolutionResult, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Read)
	if err != nil {
		return ResolutionResult{}, err
	}
	defer cancel()
	snapshot, err := s.repo.ReadBlock(ctx, e, id, in.Reference, Read)
	if err != nil {
		return ResolutionResult{}, err
	}
	resolved, err := ResolveParameters(snapshot.Revision.Definition.Parameters, in.Arguments, acceptedResolution(in.Resolution))
	if err != nil {
		return ResolutionResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return ResolutionResult{}, err
	}
	return ResolutionResult{ID: id, Revision: snapshot.Revision.Number, RevisionID: snapshot.Revision.ID, Digest: snapshot.Revision.Digest, Resolved: resolved}, nil
}
