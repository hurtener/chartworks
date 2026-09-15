package gateway

import "context"

// AttemptReservation adds a durable ceiling at a real SDK attempt boundary.
// It is installed by a domain owner with a live operation claim, never decoded
// from tool arguments. The ordinary role budget and authority remain required.
type AttemptReservation func(context.Context, Call, int) error

type attemptReservationKey struct{}

// WithAttemptReservation composes, rather than replaces, existing ceilings.
// Neither cancellation nor the outer invocation's reservation can be stripped
// by adding another budget. This context contains no token or model input.
func WithAttemptReservation(ctx context.Context, reserve AttemptReservation) (context.Context, error) {
	if ctx == nil || reserve == nil {
		return nil, ErrInput
	}
	if previous, ok := ctx.Value(attemptReservationKey{}).(AttemptReservation); ok {
		next := reserve
		reserve = func(work context.Context, call Call, tokens int) error {
			if err := previous(work, call, tokens); err != nil {
				return err
			}
			return next(work, call, tokens)
		}
	}
	return context.WithValue(ctx, attemptReservationKey{}, reserve), nil
}

// ReserveAttempt charges both the existing call-bound budget and any durable
// occurrence budget before Bifrost can dispatch. Failed/unknown attempts are
// intentionally not refunded. Every SDK retry must call this again.
func ReserveAttempt(ctx context.Context, budget *Budget, call Call, tokens int) error {
	if ctx == nil {
		return ErrInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := budget.Reserve(call, tokens); err != nil {
		return err
	}
	if reserve, ok := ctx.Value(attemptReservationKey{}).(AttemptReservation); ok {
		return reserve(ctx, call, tokens)
	}
	return nil
}
