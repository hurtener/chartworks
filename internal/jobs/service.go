package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// Service applies signed authority at every API/in-process call and owns joined worker lifetimes.
type Service struct {
	repo      Repository
	pipeline  PipelineExecutor
	reporting ReportingExecutor
	authority Authority
	limits    Limits
	running   atomic.Bool
	observe   func(string)
}

// New constructs a worker service. An optional concurrent-safe observer receives only fixed error stages.
func New(repo Repository, authority Authority, limits Limits, observer ...func(string)) (*Service, error) {
	if repo == nil || authority == nil || limits.Validate() != nil || len(observer) > 1 {
		return nil, ErrInvalid
	}
	s := &Service{repo: repo, authority: authority, limits: limits}
	if len(observer) == 1 {
		s.observe = observer[0]
	}
	return s, nil
}

// NewMetadata exposes retained metadata, cancellation and pause without model/broker availability.
// It cannot admit work or start workers; no pretend Authority is installed.
func NewMetadata(repo Repository, limits Limits) (*Service, error) {
	if repo == nil || limits.Validate() != nil {
		return nil, ErrInvalid
	}
	return &Service{repo: repo, limits: limits}, nil
}

// DispatchEnabled reports whether the real authority consumer is wired.
func (s *Service) DispatchEnabled() bool { return s != nil && s.authority != nil }
func (s *Service) report(stage string, err error) {
	if err != nil && !errors.Is(err, ErrEmpty) && !errors.Is(err, context.Canceled) && s.observe != nil {
		s.observe(stage)
	}
}
func (s *Service) admission(ctx context.Context, e identity.Envelope, request Submission) (store.Scope, error) {
	if !s.DispatchEnabled() {
		return store.Scope{}, ErrTransient
	}
	if request.Validate() != nil {
		return store.Scope{}, ErrInvalid
	}
	if err := access.Require(e, "scheduling.write", access.Tenant(e, "write"), access.Resource{Tenant: e.Tenant(), Kind: "execution_binding", Permission: "use", ID: request.BindingID}); err != nil {
		return store.Scope{}, err
	}
	if request.Kind == ReportingKind {
		if s.reporting == nil {
			return store.Scope{}, ErrTransient
		}
		if err := request.Reporting.Require(e); err != nil {
			return store.Scope{}, err
		}
		if err := s.reporting.ValidateScheduledReporting(ctx, e, *request.Reporting); err != nil {
			return store.Scope{}, err
		}
		return store.NewScope(e.Tenant(), e.User())
	}
	if request.Kind == PipelineKind {
		if s.pipeline == nil {
			return store.Scope{}, ErrTransient
		}
		if err := s.pipeline.ValidateScheduledPipeline(ctx, e, *request.Pipeline); err != nil {
			return store.Scope{}, err
		}
		return store.NewScope(e.Tenant(), e.User())
	}
	return access.StoreScope(e, "ops.maintain", "erase")
}

// Submit authorizes a fixed target and durably admits or replays its logical request key.
func (s *Service) Submit(ctx context.Context, e identity.Envelope, key string, request Submission) (Job, error) {
	if ctx == nil {
		return Job{}, ErrInvalid
	}
	ctx, stop := context.WithDeadline(ctx, e.Deadline())
	defer stop()
	scope, err := s.admission(ctx, e, request)
	if err != nil {
		return Job{}, err
	}
	if !identity.Identifier(key) {
		return Job{}, ErrInvalid
	}
	return s.repo.AdmitJob(ctx, scope, e.Session(), key, request, s.limits)
}

// Get authorizes a retained job read before accessing the repository.
func (s *Service) Get(ctx context.Context, e identity.Envelope, id string) (Job, error) {
	if err := access.Require(e, "scheduling.read", access.Resource{Tenant: e.Tenant(), Kind: "run", Permission: "read", ID: id}); err != nil {
		return Job{}, err
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return Job{}, err
	}
	return s.repo.ReadJob(ctx, scope, id)
}

// List returns only jobs selected by the caller's signed tenant and addressed read reach.
func (s *Service) List(ctx context.Context, e identity.Envelope, limit int) ([]Job, error) {
	selection, err := access.Constrain(e, "scheduling.read", "run", "read")
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		return nil, ErrInvalid
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return nil, err
	}
	return s.repo.ListJobs(ctx, scope, selection, limit)
}

// Cancel authorizes and persists cancellation without granting any execution authority.
func (s *Service) Cancel(ctx context.Context, e identity.Envelope, id string) (Job, error) {
	if err := access.Require(e, "scheduling.cancel", access.Resource{Tenant: e.Tenant(), Kind: "run", Permission: "write", ID: id}); err != nil {
		return Job{}, err
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return Job{}, err
	}
	return s.repo.CancelJob(ctx, scope, id)
}

// CreateSchedule stores or replays a fixed target and validated recurrence under tenant scope.
func (s *Service) CreateSchedule(ctx context.Context, e identity.Envelope, key string, request ScheduleRequest) (Schedule, error) {
	if ctx == nil {
		return Schedule{}, ErrInvalid
	}
	ctx, stop := context.WithDeadline(ctx, e.Deadline())
	defer stop()
	if request.Validate() != nil || !identity.Identifier(key) {
		return Schedule{}, ErrInvalid
	}
	scope, err := s.admission(ctx, e, request.Target)
	if err != nil {
		return Schedule{}, err
	}
	return s.repo.CreateSchedule(ctx, scope, e.Session(), key, request, s.limits)
}

// GetSchedule requires signed schedule read reach before loading its metadata.
func (s *Service) GetSchedule(ctx context.Context, e identity.Envelope, id string) (Schedule, error) {
	if err := access.Require(e, "scheduling.read", access.Resource{Tenant: e.Tenant(), Kind: "schedule", Permission: "read", ID: id}); err != nil {
		return Schedule{}, err
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return Schedule{}, err
	}
	return s.repo.ReadSchedule(ctx, scope, id)
}

// SetSchedule applies a revision-checked pause/resume while preserving the durable occurrence cursor.
func (s *Service) SetSchedule(ctx context.Context, e identity.Envelope, id string, expected int64, enabled bool) (Schedule, error) {
	if ctx == nil {
		return Schedule{}, ErrInvalid
	}
	ctx, stop := context.WithDeadline(ctx, e.Deadline())
	defer stop()
	if err := access.Require(e, "scheduling.write", access.Resource{Tenant: e.Tenant(), Kind: "schedule", Permission: "write", ID: id}); err != nil {
		return Schedule{}, err
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return Schedule{}, err
	}
	if enabled {
		current, err := s.repo.ReadSchedule(ctx, scope, id)
		if err != nil {
			return Schedule{}, err
		}
		if _, err = s.admission(ctx, e, current.Request.Target); err != nil {
			return Schedule{}, err
		}
	}
	return s.repo.SetSchedule(ctx, scope, id, expected, enabled)
}

// Fire authorizes one idempotent manual occurrence of a stored schedule.
func (s *Service) Fire(ctx context.Context, e identity.Envelope, id, key string) (Job, error) {
	if ctx == nil {
		return Job{}, ErrInvalid
	}
	ctx, stop := context.WithDeadline(ctx, e.Deadline())
	defer stop()
	if err := access.Require(e, "scheduling.execute", access.Resource{Tenant: e.Tenant(), Kind: "schedule", Permission: "execute", ID: id}); err != nil {
		return Job{}, err
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return Job{}, err
	}
	schedule, err := s.repo.ReadSchedule(ctx, scope, id)
	if err != nil {
		return Job{}, err
	}
	if _, err = s.admission(ctx, e, schedule.Request.Target); err != nil {
		return Job{}, err
	}
	if !identity.Identifier(key) {
		return Job{}, ErrInvalid
	}
	return s.repo.FireSchedule(ctx, scope, e.Session(), id, key, schedule.Revision, s.limits)
}

// Run starts a fixed worker set and joins all of it on every exit path. There is no detached job.
func (s *Service) Run(ctx context.Context) error {
	if !s.DispatchEnabled() {
		return ErrTransient
	}
	if !s.running.CompareAndSwap(false, true) {
		return ErrRunning
	}
	defer s.running.Store(false)
	if err := s.repo.ConfigureQueue(ctx, s.limits); err != nil {
		return err
	}
	var wg sync.WaitGroup
	for i := 0; i < s.limits.Workers; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.worker(ctx) }()
	}
	ticker := time.NewTicker(s.limits.Poll)
	defer ticker.Stop()
	defer wg.Wait()
	for {
		if ctx.Err() != nil {
			return nil
		}
		_, err := s.repo.TickSchedules(ctx, s.limits)
		s.report("schedule_tick", err)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func (s *Service) worker(ctx context.Context) {
	ticker := time.NewTicker(s.limits.Poll)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		s.report("worker_attempt", s.RunOnce(ctx))
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce is also the bounded synchronous worker entry used by operational acceptance tests.
func (s *Service) RunOnce(ctx context.Context) error {
	if !s.DispatchEnabled() {
		return ErrTransient
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return ErrTransient
	}
	lease, err := s.repo.ClaimJob(ctx, hex.EncodeToString(random[:]), s.limits)
	if err != nil {
		return err
	}
	work, cancel := context.WithTimeout(ctx, s.limits.AttemptTimeout)
	var heart sync.WaitGroup
	heart.Add(1)
	go func() {
		defer heart.Done()
		tick := time.NewTicker(s.limits.Heartbeat)
		defer tick.Stop()
		for {
			select {
			case <-work.Done():
				return
			case <-tick.C:
				if s.repo.HeartbeatJob(work, lease, s.limits.Lease) != nil {
					cancel()
					return
				}
			}
		}
	}()
	envelope, err := s.authority.Acquire(work, lease.Job)
	if err == nil {
		err = AssertExecution(envelope, lease.Job)
	}
	if err == nil {
		effect, stop := context.WithDeadline(work, envelope.Envelope().Deadline())
		if lease.Job.Kind == PipelineKind {
			if s.pipeline == nil {
				err = ErrAuthority
			} else {
				err = s.pipeline.ExecuteScheduledPipeline(effect, lease, envelope)
			}
		} else if lease.Job.Kind == ReportingKind {
			if s.reporting == nil {
				err = ErrAuthority
			} else {
				err = s.reporting.ExecuteScheduledReporting(effect, lease, envelope)
			}
		} else {
			_, err = s.repo.CompleteJob(effect, lease, envelope)
		}
		stop()
	}
	cancel()
	heart.Wait()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	} // leave a recoverable lease; process shutdown is not user cancellation.
	code, permanent := "attempt_failed", false
	switch {
	case errors.Is(err, ErrReportingBudget):
		code, permanent = "reporting_budget", true
	case errors.Is(err, ErrReportingAttention):
		code, permanent = "reporting_attention", true
	case errors.Is(err, ErrAuthority):
		code, permanent = "authority_blocked", true
	case errors.Is(err, store.ErrConflict):
		code, permanent = "definition_changed", true
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		code = "attempt_timeout"
	}
	delay := s.limits.Backoff * time.Duration(1<<min(lease.Attempt-1, 5))
	if delay > time.Minute {
		delay = time.Minute
	}
	cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stop()
	finishErr := s.repo.FinishAttempt(cleanup, lease, code, permanent, delay)
	if finishErr != nil {
		return finishErr
	}
	return err
}
