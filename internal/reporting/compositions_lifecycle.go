package reporting

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
)

// Cancel records durable cancellation without requiring a model or source service.
func (s *Compositions) Cancel(ctx context.Context, e identity.Envelope, id string) (CompositionView, error) {
	if s == nil || ctx == nil || !identity.Identifier(id) {
		return CompositionView{}, ErrInvalid
	}
	repo, ok := s.repo.(CompositionLifecycleRepository)
	if !ok {
		return CompositionView{}, ErrUnavailable
	}
	return repo.CancelComposition(ctx, e, id)
}

// Expire erases bounded expired composition values; it never regenerates them.
func (s *Compositions) Expire(ctx context.Context, e identity.Envelope, limit int) (int64, error) {
	if s == nil || ctx == nil {
		return 0, ErrInvalid
	}
	repo, ok := s.repo.(CompositionLifecycleRepository)
	if !ok {
		return 0, ErrUnavailable
	}
	return repo.ExpireCompositions(ctx, e, limit)
}
