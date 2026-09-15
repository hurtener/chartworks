package exec

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
)

// AttemptReservation is an additional domain-owned physical query ceiling.
// It grants no access, accepts no SQL and cannot replace the validator's plan.
// Reporting installs a live, fenced durable reservation for unattended work.
type AttemptReservation func(context.Context, identity.Envelope, Options) error

type attemptReservationKey struct{}

// WithAttemptReservation preserves all existing restrictions and cancellation.
// Composed guards can only narrow execution; unknown previous work stays charged.
func WithAttemptReservation(ctx context.Context, reserve AttemptReservation) (context.Context, error) {
	if ctx == nil || reserve == nil {
		return nil, ErrBinding
	}
	if previous, ok := ctx.Value(attemptReservationKey{}).(AttemptReservation); ok {
		next := reserve
		reserve = func(work context.Context, e identity.Envelope, options Options) error {
			if err := previous(work, e, options); err != nil {
				return err
			}
			return next(work, e, options)
		}
	}
	return context.WithValue(ctx, attemptReservationKey{}, reserve), nil
}

func reserveAttempt(ctx context.Context, e identity.Envelope, options Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if reserve, ok := ctx.Value(attemptReservationKey{}).(AttemptReservation); ok {
		return reserve(ctx, e, options)
	}
	return nil
}
