package jobs

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
)

// NestedRequestRepository delegates one existing report slot to a sequential
// frozen-block child. The same operation ledger, attempts and fences are used;
// this is not a second queue or a way to acquire stronger identity authority.
type NestedRequestRepository interface {
	AdmitNestedRequest(context.Context, Invocation, string, RequestInput, Limits) (RequestTask, error)
	ClaimNestedRequest(context.Context, Invocation, string, string, Limits) (RequestLease, error)
}

// Parent returns the owned parent claim, if any. The returned value cannot
// manufacture an Invocation; its current authority and persistent live fence
// must both be revalidated by the consumer and repository.
func (i Invocation) Parent() (Invocation, bool) {
	if i.parent == nil { return Invocation{}, false }
	return *i.parent, true
}

func nestedAuthority(parent Invocation, input RequestInput) (identity.Envelope, error) {
	p := parent.Lease().Task
	if parent.parent != nil || (p.Input.Kind != "report.run" && p.Input.Kind != "dashboard.run") || input.Kind != "reporting.run" || input.Context != "" {
		return identity.Envelope{}, ErrAuthority
	}
	e, err := parent.Current(p.Input.Kind, p.Input.Target, p.Input.InputHash)
	if err != nil { return identity.Envelope{}, err }
	if err := input.Require(e); err != nil { return identity.Envelope{}, err }
	return e, nil
}

// AdmitNested reserves a child under a currently owned parent, with unchanged
// signed target/dependency authority. PostgreSQL permits only one unfinished
// child per parent and keeps the parent relationship immutable.
func (r *RequestRunner) AdmitNested(ctx context.Context, parent Invocation, key string, input RequestInput) (RequestTask, error) {
	if r == nil || ctx == nil || !identity.Identifier(key) { return RequestTask{}, ErrInvalid }
	if _, err := nestedAuthority(parent, input); err != nil { return RequestTask{}, err }
	repo, ok := r.repo.(NestedRequestRepository)
	if !ok { return RequestTask{}, ErrInvalid }
	return repo.AdmitNestedRequest(ctx, parent, key, input, r.limits)
}

// RunNested lets one sequential child use the parent's counted execution slot.
// Child publication remains contingent on both the child and parent live fence.
func (r *RequestRunner) RunNested(ctx context.Context, parent Invocation, task RequestTask, timeout time.Duration, handler func(context.Context, Invocation) error) (RequestTask, error) {
	e, err := nestedAuthority(parent, task.Input)
	if err != nil { return RequestTask{}, err }
	if err := task.Require(e); err != nil { return RequestTask{}, err }
	return r.run(ctx, e, task, timeout, handler, &parent)
}
