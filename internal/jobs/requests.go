package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// RequestInput is a content-free immutable manifest for a real request-driven
// target. No bearer, SQL, file bytes, sample values or invented broker binding is
// persisted. These tasks share the operation ledger, attempts and lease engine.
type RequestInput struct {
	Kind      string `json:"kind"`
	Target    string `json:"target"`
	Context   string `json:"context"`
	InputHash string `json:"input_hash"`
}

// Valid restricts request work to the concrete engineering consumers.
func (r RequestInput) Valid() bool {
	b, e := hex.DecodeString(r.InputHash)
	return identity.Identifier(r.Target) && (r.Context == "" || identity.Identifier(r.Context)) &&
		(r.Kind == "report.run" || r.Kind == "dashboard.run" || r.Kind == "reporting.run" || r.Kind == "upload.load" || r.Kind == "upload.erase" || r.Kind == "profile.build" || r.Kind == "pipeline.run") &&
		e == nil && len(b) == 32 && hex.EncodeToString(b) == r.InputHash && (r.Kind != "profile.build" || r.Context != "")
}

// Require checks fresh signed authority, never an actor name or stored token.
func (r RequestInput) Require(e identity.Envelope) error {
	if !r.Valid() {
		return ErrInvalid
	}
	if r.Kind == "report.run" || r.Kind == "dashboard.run" {
		kind := r.Kind[:len(r.Kind)-len(".run")]
		return access.Require(e, "reporting.execute", access.Resource{Tenant: e.Tenant(), Kind: kind, Permission: "execute", ID: r.Target})
	}
	if r.Kind == "reporting.run" {
		return access.Require(e, "reporting.execute", access.Resource{Tenant: e.Tenant(), Kind: "block", Permission: "execute", ID: r.Target})
	}
	action, permission := "sources.upload", "write"
	if r.Kind == "upload.erase" {
		action, permission = "sources.erase", "erase"
	}
	if r.Kind == "pipeline.run" {
		action, permission = "engineering.pipeline.run", "write"
	}
	if r.Kind == "profile.build" {
		action, permission = "engineering.profile", "write"
	}
	refs := []access.Resource{{Tenant: e.Tenant(), Kind: "source", Permission: permission, ID: r.Target}}
	if r.Context != "" {
		refs = append(refs, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: r.Context})
	}
	return access.Require(e, action, refs...)
}

// RequestTask is retained metadata, not execution authority. User and session
// binding survives a process crash; every explicit resume supplies a fresh JWT.
type RequestTask struct {
	ID           string       `json:"id"`
	Tenant       string       `json:"tenant"`
	Actor        string       `json:"actor"`
	Session      string       `json:"session"`
	Input        RequestInput `json:"input"`
	State        string       `json:"state"`
	Code         string       `json:"code"`
	Attempts     int          `json:"attempts"`
	MaxAttempts  int          `json:"max_attempts"`
	Created      time.Time    `json:"created_at"`
	Expires      time.Time    `json:"expires_at"`
	ManifestHash string       `json:"manifest_hash"`
	Dispatch     *Job         `json:"dispatch,omitempty"`
}

// Digest excludes mutable lifecycle state and includes exact admitted identity.
func (t RequestTask) Digest() string {
	if t.Dispatch != nil {
		return t.Dispatch.Digest()
	}
	return requestDigest([]any{"chartworks-request-operation-v1", t.ID, t.Tenant, t.Actor, t.Session, t.Input, t.MaxAttempts, t.Created.UTC(), t.Expires.UTC()})
}

// Valid rejects incomplete or tampered retained manifests.
func (t RequestTask) Valid() bool {
	if t.Dispatch != nil && !validDispatchedRequest(t) {
		return false
	}
	return identity.Identifier(t.Tenant) && identity.Identifier(t.ID) && identity.Identifier(t.Actor) && identity.Identifier(t.Session) && t.Input.Valid() &&
		t.MaxAttempts >= 1 && t.MaxAttempts <= 8 && t.Attempts >= 0 && t.Attempts <= t.MaxAttempts && !t.Created.IsZero() && t.Expires.After(t.Created) && t.ManifestHash == t.Digest()
}

// Require keeps request operations actor/session private, independent of reach.
func (t RequestTask) Require(e identity.Envelope) error {
	if !t.Valid() || t.Tenant != e.Tenant() || t.Actor != e.User() || t.Session != e.Session() {
		return store.ErrNotFound
	}
	return t.Input.Require(e)
}

// RequestLease uses the same persistent owner/fence/attempt coordinates as broker
// work. The lease itself never replaces the caller's current signed authority.
type RequestLease struct {
	Task    RequestTask
	Owner   string
	Fence   int64
	Attempt int
	Until   time.Time
}

// Invocation is an opaque in-process claim issued only by RequestRunner. It can
// seal cleanup evidence after expiry, but any effect/publication calls Current.
type Invocation struct {
	lease     RequestLease
	authority identity.Envelope
	owned     bool
	parent    *Invocation
}

// Lease returns content-free coordinates; a serialized copy cannot forge Invocation.
func (i Invocation) Lease() RequestLease { return i.lease }

// Valid identifies the originally owned attempt, not permission to perform effects.
func (i Invocation) Valid() bool {
	if i.parent != nil {
		p := i.parent
		if p.parent != nil || !p.Valid() || (p.lease.Task.Input.Kind != "report.run" && p.lease.Task.Input.Kind != "dashboard.run") ||
			i.lease.Task.Input.Kind != "reporting.run" || i.lease.Task.Tenant != p.lease.Task.Tenant ||
			i.lease.Task.Actor != p.lease.Task.Actor || i.lease.Task.Session != p.lease.Task.Session || i.lease.Task.ID == p.lease.Task.ID {
			return false
		}
	}
	return i.owned && i.lease.Task.Valid() && identity.Identifier(i.lease.Owner) && i.lease.Fence > 0 && i.lease.Attempt > 0 && i.lease.Task.Tenant == i.authority.Tenant() && i.lease.Task.Actor == i.authority.User() && i.lease.Task.Session == i.authority.Session()
}

// Current repeats expiry, target/context reach and immutable effect binding.
func (i Invocation) Current(kind, target, hash string) (identity.Envelope, error) {
	if !i.Valid() || i.lease.Task.Input.Kind != kind || i.lease.Task.Input.Target != target || i.lease.Task.Input.InputHash != hash {
		return identity.Envelope{}, ErrAuthority
	}
	if i.parent != nil {
		p := i.parent.Lease().Task
		if _, err := i.parent.Current(p.Input.Kind, p.Input.Target, p.Input.InputHash); err != nil {
			return identity.Envelope{}, err
		}
	}
	if err := i.lease.Task.Require(i.authority); err != nil {
		return identity.Envelope{}, err
	}
	return i.authority, nil
}

// RequestRepository adds real consumers to the existing operation ledger. Its
// PostgreSQL implementation shares queue admission and claim/renew/failure helpers.
type RequestRepository interface {
	AdmitRequest(context.Context, identity.Envelope, string, RequestInput, Limits) (RequestTask, error)
	ReadRequest(context.Context, identity.Envelope, string) (RequestTask, error)
	ResumeRequest(context.Context, identity.Envelope, string, Limits) (RequestTask, error)
	CancelRequest(context.Context, identity.Envelope, string) (RequestTask, error)
	ClaimRequest(context.Context, identity.Envelope, string, string, Limits) (RequestLease, error)
	PulseRequest(context.Context, Invocation, bool, time.Duration) (string, error)
	FailRequest(context.Context, Invocation, string, bool, time.Duration) error
}

// RequestRunner executes one explicit, bounded request attempt. It is not another
// daemon or scheduler and cannot obtain new tokens. The current Pengui broker's
// retention-only contract remains unchanged rather than being spoofed locally.
type RequestRunner struct {
	repo   RequestRepository
	limits Limits
}

// NewRequestRunner performs no database or source work.
func NewRequestRunner(repo RequestRepository, limits Limits) (*RequestRunner, error) {
	if repo == nil || limits.Validate() != nil {
		return nil, ErrInvalid
	}
	return &RequestRunner{repo: repo, limits: limits}, nil
}

// Admit binds an explicit idempotency key to one immutable real target.
func (r *RequestRunner) Admit(ctx context.Context, e identity.Envelope, key string, input RequestInput) (RequestTask, error) {
	if ctx == nil || !identity.Identifier(key) {
		return RequestTask{}, ErrInvalid
	}
	if err := input.Require(e); err != nil {
		return RequestTask{}, err
	}
	return r.repo.AdmitRequest(ctx, e, key, input, r.limits)
}

// Inspect authorizes before returning any retained operation details.
func (r *RequestRunner) Inspect(ctx context.Context, e identity.Envelope, id string) (RequestTask, error) {
	if ctx == nil || !e.Valid() || !identity.Identifier(id) {
		return RequestTask{}, ErrInvalid
	}
	return r.repo.ReadRequest(ctx, e, id)
}

// Cancel records durable intent/state. A live owner observes it across replicas;
// a cancellation receipt is not a claim that an external transaction has stopped.
func (r *RequestRunner) Cancel(ctx context.Context, e identity.Envelope, id string) (RequestTask, error) {
	if _, err := r.Inspect(ctx, e, id); err != nil {
		return RequestTask{}, err
	}
	return r.repo.CancelRequest(ctx, e, id)
}

// Resume is explicit. It preserves the accepted manifest and never refreshes or
// replaces stored authority; only the current supplied bearer can start work.
func (r *RequestRunner) Resume(ctx context.Context, e identity.Envelope, id string) (RequestTask, error) {
	if _, err := r.Inspect(ctx, e, id); err != nil {
		return RequestTask{}, err
	}
	return r.repo.ResumeRequest(ctx, e, id, r.limits)
}

// Run executes exactly one physical attempt and joins its cancellation/heartbeat
// observer. Domain completion must atomically publish its pointer and finish the
// same live fence; returning nil from a handler alone does not manufacture success.
func (r *RequestRunner) Run(ctx context.Context, e identity.Envelope, task RequestTask, timeout time.Duration, handler func(context.Context, Invocation) error) (RequestTask, error) {
	return r.run(ctx, e, task, timeout, handler, nil)
}

func (r *RequestRunner) run(ctx context.Context, e identity.Envelope, task RequestTask, timeout time.Duration, handler func(context.Context, Invocation) error, parent *Invocation) (RequestTask, error) {
	if ctx == nil || handler == nil || timeout < time.Millisecond || timeout > time.Minute {
		return RequestTask{}, ErrInvalid
	}
	if err := task.Require(e); err != nil {
		return RequestTask{}, err
	}
	ctx, expire := context.WithDeadline(ctx, e.Deadline())
	defer expire()
	ctx, deadline := context.WithTimeout(ctx, timeout)
	defer deadline()
	if err := ctx.Err(); err != nil {
		return RequestTask{}, err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return RequestTask{}, store.ErrUnavailable
	}
	var lease RequestLease
	var err error
	if parent == nil {
		lease, err = r.repo.ClaimRequest(ctx, e, task.ID, hex.EncodeToString(id[:]), r.limits)
	} else {
		if _, err = nestedAuthority(*parent, task.Input); err != nil {
			return RequestTask{}, err
		}
		repo, ok := r.repo.(NestedRequestRepository)
		if !ok {
			return RequestTask{}, ErrInvalid
		}
		lease, err = repo.ClaimNestedRequest(ctx, *parent, task.ID, hex.EncodeToString(id[:]), r.limits)
	}
	if err != nil {
		return RequestTask{}, err
	}
	invocation := Invocation{lease: lease, authority: e, owned: true, parent: parent}
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	stop, done := make(chan struct{}), make(chan error, 1)
	go func() {
		ticker := time.NewTicker(min(r.limits.Heartbeat, 100*time.Millisecond))
		defer ticker.Stop()
		last := time.Now()
		for {
			select {
			case <-stop:
				done <- nil
				return
			case <-ctx.Done():
				done <- nil
				return
			case <-ticker.C:
				pulse, end := context.WithTimeout(work, time.Second)
				renew := time.Since(last) >= r.limits.Heartbeat
				state, e := r.repo.PulseRequest(pulse, invocation, renew, r.limits.Lease)
				end()
				if e != nil {
					cancel()
					done <- e
					return
				}
				if state == "succeeded" {
					done <- nil
					return
				}
				if state != "running" {
					cancel()
					done <- context.Canceled
					return
				}
				if renew {
					last = time.Now()
				}
			}
		}
	}()
	var once sync.Once
	var watchErr error
	join := func() { once.Do(func() { close(stop); watchErr = <-done }) }
	defer join()
	runErr := handler(work, invocation)
	join()
	if watchErr != nil {
		runErr = watchErr
	}
	cleanup, finish := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer finish()
	if runErr != nil {
		code := "attempt_failed"
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			code = "attempt_timeout"
		}
		if err = r.repo.FailRequest(cleanup, invocation, code, false, r.limits.Backoff); err != nil && !errors.Is(err, store.ErrConflict) {
			return RequestTask{}, err
		}
	}
	// Metadata recovery still requires current authority. An expired caller cannot
	// publish or retrieve a fresh result by using the cleanup allowance.
	out, err := r.repo.ReadRequest(cleanup, e, task.ID)
	if err != nil {
		return RequestTask{}, err
	}
	if out.State == "succeeded" {
		// A lost commit reply or late observer failure cannot overturn the
		// domain's atomic publication. Only this authorized durable receipt
		// resolves success; absent or unreadable evidence remains an error.
		return out, nil
	}
	if runErr == nil {
		return out, store.ErrConflict
	}
	return out, runErr
}
