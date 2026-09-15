package reporting

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
)

// Inspect returns an actor/session-private execution receipt. Ordinary artifact
// reads use Get and never need reporting.execute or the originating session.
func (s *Compositions) Inspect(ctx context.Context, e identity.Envelope, id string) (CompositionView, error) {
	if s == nil || ctx == nil || !identity.Identifier(id) {
		return CompositionView{}, ErrInvalid
	}
	record, err := s.repo.ReadComposition(ctx, e, id)
	if err != nil {
		return CompositionView{}, err
	}
	return SummarizeComposition(record), nil
}
