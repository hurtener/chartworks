package exec

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"reflect"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

var (
	// ErrCancelled is a deliberate cancellation, not a valid empty result.
	ErrCancelled = errors.New("exec: cancelled")
	// ErrTimeout is a client/server deadline, never an invitation to broaden SQL.
	ErrTimeout = errors.New("exec: timed out")
	// ErrUncertain means query termination or durable outcome could not be proved.
	ErrUncertain = errors.New("exec: outcome uncertain; inspect or reconcile the attempt")
	// ErrReplay prevents a repeated physical attempt from executing twice.
	ErrReplay = errors.New("exec: attempt already admitted; read its receipt")
)

// Limits is the effective bounded execution contract, shared by interactive and job consumers.
type Limits struct {
	Rows        int           `json:"rows"`
	Bytes       int           `json:"bytes"`
	Timeout     time.Duration `json:"timeout_ns"`
	CancelGrace time.Duration `json:"cancel_grace_ns"`
	PlannerCost float64       `json:"planner_cost_ceiling"`
}

func (l Limits) Valid() bool {
	return l.Rows > 0 && l.Rows <= 100000 && l.Bytes >= 128 && l.Bytes <= 16<<20 && l.Timeout >= time.Millisecond && l.Timeout <= time.Minute && l.CancelGrace >= time.Millisecond && l.CancelGrace <= 3*time.Second && l.PlannerCost > 0 && l.PlannerCost <= 1e12 && !math.IsNaN(l.PlannerCost)
}

// Options explicitly identifies one logical operation and one physical attempt.
// Reusing the same attempt never reruns a query. Retried failures require Number+1;
// uncertain attempts require reconciliation first. Result values are not cached.
type Options struct {
	Operation string `json:"operation"`
	Number    int    `json:"attempt"`
	Preview   bool   `json:"preview"`
	Rows      int    `json:"rows"`
	Bytes     int    `json:"bytes"`
}

// RemoteQuery identifies one tagged PostgreSQL transaction, not an arbitrary PID.
// The backend secret used by PostgreSQL CancelRequest is never retained.
type RemoteQuery struct {
	PID     uint32    `json:"pid"`
	Started time.Time `json:"backend_started"`
	Tag     string    `json:"tag"`
}

func (q RemoteQuery) Valid() bool {
	return q.PID > 0 && !q.Started.IsZero() && len(q.Tag) == 40 && q.Tag[:8] == "cw-read:" && hashID(q.Tag[8:])
}
func hashID(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 16 && hex.EncodeToString(b) == s
}

// Manifest retains content-free operation identity and exact dependency reach.
// It stores neither SQL/parameters, result values nor JWTs.
type Manifest struct {
	Operation string  `json:"operation"`
	Session   string  `json:"session"`
	Receipt   Receipt `json:"validation"`
	Limits    Limits  `json:"limits"`
	Preview   bool    `json:"preview"`
}

func (m Manifest) Valid() bool {
	return identity.Identifier(m.Operation) && identity.Identifier(m.Session) && m.Receipt.Validated && identity.Identifier(m.Receipt.Source) && identity.Identifier(m.Receipt.Context) && len(m.Receipt.Manifest) == 64 && len(m.Receipt.Dependencies) <= 32 && m.Limits.Valid()
}

// Attempt is protected, content-free execution evidence; active receipts expire
// into uncertainty rather than pretending a process crash completed its query.
type Attempt struct {
	ID              string       `json:"id"`
	Number          int          `json:"number"`
	Manifest        Manifest     `json:"manifest"`
	Status          string       `json:"status"`
	Remote          *RemoteQuery `json:"remote"`
	RemoteState     string       `json:"remote_state"`
	CancelRequested bool         `json:"cancel_requested"`
	Created         time.Time    `json:"created_at"`
	Deadline        time.Time    `json:"deadline"`
	Finished        *time.Time   `json:"finished_at"`
	Rows            int          `json:"rows_returned"`
	Bytes           int          `json:"bytes_returned"`
	Code            string       `json:"code"`
}

// AttemptStore journals physical attempts independently from the source-revision
// transaction. Capacity reserves a connection for journal/cancel traffic.
type AttemptStore interface {
	ReadCapacity() int
	BeginRead(context.Context, store.Scope, Attempt, int) error
	DispatchRead(context.Context, store.Scope, string, RemoteQuery, bool) error
	GetRead(context.Context, store.Scope, string) (Attempt, error)
	GetReadOperation(context.Context, store.Scope, string) (Attempt, error)
	CancelRead(context.Context, store.Scope, string) error
	FinishRead(context.Context, store.Scope, Attempt, bool) error
}

// Observer journals intent BEFORE issuing SQL and acknowledgment AFTER DECLARE.
// Check is also called between cursor fetches, so cancellation is not a transient signal.
type Observer interface {
	Dispatch(context.Context, RemoteQuery, bool) error
	Check(context.Context) error
}

// NativeResult reports termination separately from the logical result.
type NativeResult struct {
	Result      Result
	RemoteState string
}

// ExecutionAdapter is the plan-only concrete source boundary. It contains no raw SQL API.
type ExecutionAdapter interface {
	ReadCapabilities() Capabilities
	ReadAdapter
	ExecuteRead(context.Context, identity.Envelope, Plan, Limits, string, Observer) (NativeResult, error)
	ControlRead(context.Context, identity.Envelope, Control, bool) (string, error)
}

// Control can only be created from an authorized retained attempt by Executor.
// An opaque query ID, PID or serialized receipt alone cannot cancel a query.
type Control struct {
	attempt Attempt
	actor   string
	tenant  string
}

// Target checks the current signed reach and exact source context before control I/O.
func (c Control) Target(e identity.Envelope, b Binding) (RemoteQuery, error) {
	a := c.attempt
	if c.actor != e.User() || c.tenant != e.Tenant() || a.Manifest.Session != e.Session() || a.Remote == nil || !a.Remote.Valid() || a.Manifest.Receipt.Source != b.Source || a.Manifest.Receipt.Context != b.Context {
		return RemoteQuery{}, ErrBinding
	}
	if err := Require(e, b, a.Manifest.Receipt.Dependencies); err != nil {
		return RemoteQuery{}, err
	}
	return *a.Remote, nil
}

// Coordinates permits a control adapter to load its current protected source binding.
func (c Control) Coordinates() (string, string) {
	return c.attempt.Manifest.Receipt.Source, c.attempt.Manifest.Receipt.Context
}

// ExecutionReport distinguishes a terminal query failure from transport/admission failure.
// HTTP 200 means an accepted attempt has a durable receipt; inspect Attempt.Status.
type ExecutionReport struct {
	Capabilities Capabilities `json:"capabilities"`
	Attempt      Attempt      `json:"attempt"`
	Result       *Result      `json:"result"`
}

// Executor is one model-free read core; it never rewrites SQL or retries automatically.
type Executor struct {
	adapter  ExecutionAdapter
	repo     AttemptStore
	settings config.ReadValidation
	slots    chan struct{}
}

func nilDependency(v any) bool {
	return v == nil || reflect.ValueOf(v).Kind() == reflect.Pointer && reflect.ValueOf(v).IsNil()
}

// NewExecutor reserves bounded concurrent reads and leaves journal/control capacity.
func NewExecutor(adapter ExecutionAdapter, repo AttemptStore, settings config.ReadValidation) (*Executor, error) {
	if nilDependency(adapter) || nilDependency(repo) || config.ValidateReadValidation(settings) != nil || repo.ReadCapacity() < settings.ExecutionConcurrency+2 {
		return nil, ErrBinding
	}
	return &Executor{adapter: adapter, repo: repo, settings: settings, slots: make(chan struct{}, settings.ExecutionConcurrency)}, nil
}
func (x *Executor) limits(o Options) (Limits, error) {
	r := o.Rows
	if r == 0 {
		r = x.settings.RowsDefault
	}
	b := o.Bytes
	if b == 0 {
		b = x.settings.BytesDefault
	}
	if !identity.Identifier(o.Operation) || o.Number < 1 || o.Number > x.settings.MaxReadAttempts || r < 1 || r > x.settings.RowsCeiling || b < 128 || b > x.settings.BytesCeiling {
		return Limits{}, ErrLimit
	}
	if o.Preview && r > x.settings.PreviewRows {
		r = x.settings.PreviewRows
	}
	return Limits{Rows: r, Bytes: b, Timeout: time.Duration(x.settings.Timeout), CancelGrace: time.Duration(x.settings.CancelGrace), PlannerCost: x.settings.PlannerCostCeiling}, nil
}

// Execute requires a currently authorized nonzero plan, then admits exactly one
// explicit attempt. A failed durable finalization discards rows and leaves recovery evidence.
func (x *Executor) Execute(ctx context.Context, e identity.Envelope, p Plan, o Options) (ExecutionReport, error) {
	if ctx == nil || !e.Valid() {
		return ExecutionReport{}, ErrBinding
	}
	if _, _, err := p.SQL(e, p.candidate.binding); err != nil {
		return ExecutionReport{}, err
	}
	limits, err := x.limits(o)
	if err != nil {
		return ExecutionReport{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, limits.Timeout)
	defer cancel()
	ctx, expiry := context.WithDeadline(ctx, e.Deadline())
	defer expiry()
	select {
	case x.slots <- struct{}{}:
		defer func() { <-x.slots }()
	case <-ctx.Done():
		return ExecutionReport{}, ctx.Err()
	}
	if err = ctx.Err(); err != nil {
		return ExecutionReport{}, err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return ExecutionReport{}, ErrUncertain
	}
	deadline, _ := ctx.Deadline()
	a := Attempt{ID: hex.EncodeToString(random[:]), Number: o.Number, Manifest: Manifest{Operation: o.Operation, Session: e.Session(), Receipt: p.Receipt(), Limits: limits, Preview: o.Preview}, Status: "accepted", RemoteState: "not_issued", Created: time.Now().UTC(), Deadline: deadline.UTC()}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return ExecutionReport{}, err
	}
	if err = x.repo.BeginRead(ctx, scope, a, x.settings.MaxReadAttempts); err != nil {
		return ExecutionReport{}, err
	}
	observer := &readObserver{repo: x.repo, scope: scope, id: a.ID}
	nativeContext, joinWatcher := watchRead(ctx, observer)
	native, runErr := x.adapter.ExecuteRead(nativeContext, e, p, limits, a.ID, observer)
	if watchErr := joinWatcher(); watchErr != nil {
		runErr = watchErr
		native.Result = Result{}
	}
	status, code := "failed", "source_unavailable"
	if runErr == nil {
		status = native.Result.Outcome
		code = ""
		if status != "succeeded" && status != "empty" && status != "truncated" {
			runErr = ErrType
			status = "failed"
			code = "invalid_result"
		}
	} else {
		switch {
		case errors.Is(runErr, ErrCancelled), errors.Is(runErr, context.Canceled):
			status, code = "cancelled", "cancelled"
		case errors.Is(runErr, ErrTimeout), errors.Is(runErr, context.DeadlineExceeded):
			status, code = "timed_out", "timed_out"
		case errors.Is(runErr, ErrUncertain):
			status, code = "uncertain", "remote_outcome_unknown"
		case errors.Is(runErr, ErrType):
			code = "result_type_unsupported"
		case errors.Is(runErr, ErrLimit):
			code = "limit_exceeded"
		case errors.Is(runErr, ErrBinding):
			code = "context_changed"
		case errors.Is(runErr, ErrUnsupported):
			code = "unsupported"
		}
	}
	if native.RemoteState == "unknown" {
		status, code = "uncertain", "remote_outcome_unknown"
	}
	cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), limits.CancelGrace)
	defer stop()
	current, err := x.repo.GetRead(cleanup, scope, a.ID)
	if err != nil {
		return ExecutionReport{}, ErrUncertain
	}
	if current.CancelRequested && status != "uncertain" {
		status, code = "cancelled", "cancelled"
		runErr = ErrCancelled
	}
	current.Status = status
	current.Code = code
	current.RemoteState = native.RemoteState
	if current.RemoteState == "" {
		current.RemoteState = "not_issued"
	}
	if runErr == nil && status != "uncertain" {
		current.Rows = len(native.Result.Rows)
		current.Bytes = native.Result.Bytes
	}
	now := time.Now().UTC()
	current.Finished = &now
	if err = x.repo.FinishRead(cleanup, scope, current, false); err != nil {
		return ExecutionReport{}, ErrUncertain
	}
	current, err = x.repo.GetRead(cleanup, scope, a.ID)
	if err != nil {
		return ExecutionReport{}, ErrUncertain
	}
	status = current.Status
	report := ExecutionReport{Attempt: current, Capabilities: x.adapter.ReadCapabilities()}
	if runErr == nil && (status == "succeeded" || status == "empty" || status == "truncated") {
		report.Result = &native.Result
	}
	return report, nil
}

type readObserver struct {
	repo  AttemptStore
	scope store.Scope
	id    string
}

func (o *readObserver) Dispatch(ctx context.Context, q RemoteQuery, accepted bool) error {
	return o.repo.DispatchRead(ctx, o.scope, o.id, q, accepted)
}
func (o *readObserver) Check(ctx context.Context) error {
	a, err := o.repo.GetRead(ctx, o.scope, o.id)
	if err != nil {
		return err
	}
	if a.CancelRequested {
		return ErrCancelled
	}
	if a.Status == "uncertain" || a.Finished != nil {
		return ErrUncertain
	}
	return nil
}

func (x *Executor) authorized(ctx context.Context, e identity.Envelope, id string) (Attempt, store.Scope, error) {
	if ctx == nil || !e.Valid() || !hashID(id) {
		return Attempt{}, store.Scope{}, ErrBinding
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return Attempt{}, scope, err
	}
	a, err := x.repo.GetRead(ctx, scope, id)
	if err != nil {
		return Attempt{}, scope, err
	}
	if a.Manifest.Session != e.Session() {
		return Attempt{}, scope, store.ErrNotFound
	}
	b, err := x.adapter.Binding(ctx, e, a.Manifest.Receipt.Source, a.Manifest.Receipt.Context)
	if err != nil {
		return Attempt{}, scope, err
	}
	if err = Require(e, b, a.Manifest.Receipt.Dependencies); err != nil {
		return Attempt{}, scope, err
	}
	return a, scope, nil
}

// Inspect reads content-free evidence with fresh actor/session/source/context reach.
func (x *Executor) Inspect(ctx context.Context, e identity.Envelope, id string) (Attempt, error) {
	a, _, err := x.authorized(ctx, e, id)
	return a, err
}

// ControlReceipt never equates delivery of a cancel request with remote termination.
type ControlReceipt struct {
	Attempt     Attempt `json:"attempt"`
	RemoteState string  `json:"remote_state"`
}

// Control requests cancellation or reconciles a stopped uncertain attempt. It does
// not reconstruct rows, rerun queries or infer successful execution after a crash.
func (x *Executor) Control(ctx context.Context, e identity.Envelope, id string, cancel bool) (ControlReceipt, error) {
	a, scope, err := x.authorized(ctx, e, id)
	if err != nil {
		return ControlReceipt{}, err
	}
	if a.Finished != nil && a.Status != "uncertain" {
		return ControlReceipt{Attempt: a, RemoteState: a.RemoteState}, nil
	}
	if cancel {
		if err = x.repo.CancelRead(ctx, scope, id); err != nil {
			return ControlReceipt{}, err
		}
		a.CancelRequested = true
	}
	state := "not_issued"
	if a.Remote != nil {
		state, err = x.adapter.ControlRead(ctx, e, Control{attempt: a, tenant: e.Tenant(), actor: e.User()}, cancel)
		if err != nil {
			state = "unknown"
			if errors.Is(err, ErrUnsupported) {
				state = "unsupported"
			}
		}
	}
	// Only a crashed/expired attempt is reconciled here. A live owner remains
	// responsible for its result and observes the durable cancel flag at fetch boundaries.
	if a.Status == "uncertain" && (state == "stopped" || state == "not_issued") {
		a.Status = "interrupted"
		a.Code = "result_not_retained"
		a.RemoteState = state
		now := time.Now().UTC()
		a.Finished = &now
		if err = x.repo.FinishRead(ctx, scope, a, true); err != nil {
			return ControlReceipt{}, err
		}
	}
	return ControlReceipt{Attempt: a, RemoteState: state}, nil
}

// watchRead owns one bounded, joined cancellation observer per active read.
// Metadata failure cancels work instead of silently losing the cancellation fence.
func watchRead(ctx context.Context, observer Observer) (context.Context, func() error) {
	child, cancel := context.WithCancel(ctx)
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				done <- nil
				return
			case <-ctx.Done():
				done <- nil
				return
			case <-ticker.C:
				probe, end := context.WithTimeout(child, time.Second)
				err := observer.Check(probe)
				end()
				if err != nil {
					cancel()
					done <- err
					return
				}
			}
		}
	}()
	return child, func() error { close(stop); err := <-done; cancel(); return err }
}

// ByOperation recovers a content-free attempt after a lost response. It never reruns values.
func (x *Executor) ByOperation(ctx context.Context, e identity.Envelope, operation string) (Attempt, error) {
	if ctx == nil || !e.Valid() || !identity.Identifier(operation) {
		return Attempt{}, ErrBinding
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return Attempt{}, err
	}
	a, err := x.repo.GetReadOperation(ctx, scope, operation)
	if err != nil {
		return Attempt{}, err
	}
	return x.Inspect(ctx, e, a.ID)
}
