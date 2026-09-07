package engineering

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
)

// WorkspaceReceipt identifies an observed committed owned table, never a query
// permission or a claim that unrelated baseline tables may be modified.
type WorkspaceReceipt struct {
	SpecHash     string `json:"spec_hash"`
	Checksum     string `json:"checksum"`
	Schema       string `json:"schema"`
	Table        string `json:"table"`
	TableOID     int64  `json:"table_oid"`
	Rows         int    `json:"rows"`
	DecodedBytes int64  `json:"decoded_bytes"`
}

// UploadRecord is internal metadata only. Customer file bytes live in the
// separately configured managed workspace; no credential is persisted here.
type UploadRecord struct {
	Tenant, Actor, Session string
	SourceRevision         int64
	Spec                   UploadSpec
	SpecHash               string
	State                  string
	Created, Expires       time.Time
	Operation              string
	Receipt                *WorkspaceReceipt
}

// Valid verifies an immutable retained upload manifest before any resource use.
func (r UploadRecord) Valid() bool {
	return identity.Identifier(r.Tenant) && identity.Identifier(r.Actor) && identity.Identifier(r.Session) && r.Spec.Valid(config.DefaultUploads()) && r.SpecHash == readexec.Hash(r.Spec) && !r.Created.IsZero() && r.Expires.After(r.Created)
}

// Require preserves private upload provenance in addition to signed source reach.
func (r UploadRecord) Require(e identity.Envelope, action, permission string) error {
	if !r.Valid() || r.Tenant != e.Tenant() || r.Actor != e.User() || r.Session != e.Session() {
		return store.ErrNotFound
	}
	return access.Require(e, action, access.Resource{Tenant: r.Tenant, Kind: "source", Permission: permission, ID: r.Spec.ID})
}

// UploadStatus excludes warehouse locations, backend identifiers and credentials.
type UploadStatus struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Format    string          `json:"format"`
	State     string          `json:"state"`
	Columns   []UploadColumn  `json:"columns"`
	Rows      int             `json:"rows"`
	Created   time.Time       `json:"created_at"`
	Expires   time.Time       `json:"staging_expires_at"`
	Operation string          `json:"operation"`
	Source    *sources.Source `json:"source"`
}

// Public returns content-free upload status without warehouse coordinates.
func (r UploadRecord) Public() UploadStatus {
	out := UploadStatus{ID: r.Spec.ID, Name: r.Spec.Name, Format: r.Spec.Format, State: r.State, Columns: append([]UploadColumn(nil), r.Spec.Columns...), Created: r.Created, Expires: r.Expires, Operation: r.Operation}
	if r.Receipt != nil {
		out.Rows = r.Receipt.Rows
	}
	if r.State == "active" {
		out.Source = &sources.Source{ID: r.Spec.ID, Name: r.Spec.Name, Dialect: "postgres", Revision: r.SourceRevision, ContextID: r.Spec.ID + ":v" + strconv.FormatInt(r.SourceRevision, 10), Status: "registered"}
	}
	return out
}

// UploadRun is an accepted operation receipt. A retry/cancelled state is not a
// queryable dataset; only Upload.State=active has an activated source pointer.
type UploadRun struct {
	Upload    UploadStatus     `json:"upload"`
	Operation jobs.RequestTask `json:"operation"`
	Code      string           `json:"code"`
}

// Repository adds engineering metadata to the existing operation store. No method
// accepts a caller-selected tenant or bearer to persist for later execution.
type Repository interface {
	jobs.RequestRepository
	ReadCapacity() int
	DatabaseName() string
	ReserveUpload(context.Context, identity.Envelope, UploadSpec, config.Uploads) (UploadRecord, error)
	ReadUpload(context.Context, identity.Envelope, string, string, string) (UploadRecord, error)
	StageUpload(context.Context, identity.Envelope, string, func(context.Context, UploadRecord) error) (UploadRecord, error)
	AttachUpload(context.Context, identity.Envelope, string, jobs.RequestTask, bool) (UploadRecord, error)
	ActivateUpload(context.Context, jobs.Invocation, UploadRecord, WorkspaceReceipt, sources.Record) error
	FinishUploadErasure(context.Context, jobs.Invocation, UploadRecord) error
	ExpiredUploads(context.Context, identity.Envelope, int) ([]UploadRecord, error)
}

// Service owns bounded upload work and deterministic profiles. Actual querying
// always goes through the injected validator/executor; managed writes are separate.
type Service struct {
	summarySchema *gateway.Schema
	repo          Repository
	sources       *sources.Service
	validator     *readexec.Validator
	executor      *readexec.Executor
	gateway       gateway.Engine
	runner        *jobs.RequestRunner
	values        config.Values
	lookup        func(string) (string, bool)
	slots         chan struct{}
	lifecycle     sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	closed        bool
}

// New does not open warehouse connections or call a model. Retained metadata is
// available independently from source credentials and optional summary services.
func New(repo Repository, source *sources.Service, validator *readexec.Validator, executor *readexec.Executor, model gateway.Engine, values config.Values, lookup func(string) (string, bool)) (*Service, error) {
	if repo == nil || source == nil || lookup == nil || config.ValidateUploads(values.Uploads) != nil || config.ValidateProfiling(values.Profiling) != nil {
		return nil, ErrInvalid
	}
	if (values.Uploads.Enabled || values.Profiling.Enabled) && (validator == nil || executor == nil || !values.Sources.Enabled || repo.ReadCapacity() < values.Uploads.Concurrency+values.Exec.ExecutionConcurrency+2) {
		return nil, ErrInvalid
	}
	limits := jobs.Defaults()
	limits.GlobalConcurrency = values.Jobs.GlobalConcurrency
	limits.TenantConcurrency = values.Jobs.TenantConcurrency
	limits.MaxPending = values.Jobs.MaxPending
	limits.MaxPendingPerTenant = values.Jobs.MaxPendingPerTenant
	limits.MaxAttempts = values.Jobs.MaxAttempts
	limits.Workers = values.Jobs.Workers
	limits.Lease = time.Duration(values.Jobs.Lease)
	limits.Heartbeat = time.Duration(values.Jobs.Heartbeat)
	limits.Poll = time.Duration(values.Jobs.Poll)
	limits.Backoff = time.Duration(values.Jobs.Backoff)
	limits.Batch = values.Jobs.Batch
	limits.AttemptTimeout = time.Duration(values.Jobs.AttemptTimeout)
	runner, err := jobs.NewRequestRunner(repo, limits)
	if err != nil {
		return nil, err
	}
	values.Sources = values.Sources.Clone()
	values.Uploads = values.Uploads.Clone()
	values.Profiling = values.Profiling.Clone()
	schema, err := profileSummarySchema()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{summarySchema: schema, repo: repo, sources: source, validator: validator, executor: executor, gateway: model, runner: runner, values: values, lookup: lookup, slots: make(chan struct{}, values.Uploads.Concurrency), ctx: ctx, cancel: cancel}, nil
}

// Close cancels and joins owned work before releasing service lifecycle state.
func (s *Service) Close() {
	s.cancel()
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.closed = true
}
func (s *Service) call(ctx context.Context, e identity.Envelope, work bool, fn func(context.Context) error) error {
	if ctx == nil || !e.Valid() {
		return access.ErrUnauthenticated
	}
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	if s.closed {
		return store.ErrUnavailable
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	if err := ctx.Err(); err != nil {
		return err
	}
	if work {
		ctx, finish := context.WithTimeout(ctx, time.Duration(s.values.Uploads.Timeout))
		defer finish()
		select {
		case s.slots <- struct{}{}:
			defer func() { <-s.slots }()
		case <-ctx.Done():
			return ctx.Err()
		}
		return fn(ctx)
	}
	return fn(ctx)
}
func (s *Service) workspace(e identity.Envelope, alias string) (config.SourceConnection, error) {
	for _, c := range s.values.Sources.Connections {
		if c.Tenant == e.Tenant() && c.ID == alias && c.ManagedSchema != "" && c.WriteDSN != "" {
			return c, nil
		}
	}
	return config.SourceConnection{}, store.ErrNotFound
}

// ReserveUpload authorizes a deterministic target before quota or source access.
func (s *Service) ReserveUpload(ctx context.Context, e identity.Envelope, spec UploadSpec) (out UploadStatus, err error) {
	if err = access.Require(e, "sources.upload", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "write", ID: spec.ID}, access.Tenant(e, "write")); err != nil {
		return out, err
	}
	if !s.values.Uploads.Enabled {
		return out, store.ErrUnavailable
	}
	if !spec.Valid(s.values.Uploads) {
		return out, ErrInvalid
	}
	if _, err = s.workspace(e, spec.Connection); err != nil {
		return out, err
	}
	err = s.call(ctx, e, false, func(ctx context.Context) error {
		record, e2 := s.repo.ReserveUpload(ctx, e, spec, s.values.Uploads)
		if e2 == nil {
			out = record.Public()
		}
		return e2
	})
	return out, err
}

// InspectUpload returns private, content-free status without source/model calls.
func (s *Service) InspectUpload(ctx context.Context, e identity.Envelope, id string) (out UploadStatus, err error) {
	if err = access.Require(e, "sources.read", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: id}); err != nil {
		return out, err
	}
	err = s.call(ctx, e, false, func(ctx context.Context) error {
		record, e2 := s.repo.ReadUpload(ctx, e, id, "sources.read", "read")
		if e2 == nil {
			out = record.Public()
		}
		return e2
	})
	return out, err
}

// StageUpload authorizes before reading an untrusted body and preserves complete
// bytes atomically in the workspace. A failed transfer leaves a resumable reserve.
func (s *Service) StageUpload(ctx context.Context, e identity.Envelope, id string, body io.Reader) (out UploadStatus, err error) {
	if err = access.Require(e, "sources.upload", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "write", ID: id}); err != nil {
		return out, err
	}
	if body == nil || !s.values.Uploads.Enabled {
		return out, ErrInvalid
	}
	err = s.call(ctx, e, true, func(ctx context.Context) error {
		r, e2 := s.repo.StageUpload(ctx, e, id, func(ctx context.Context, r UploadRecord) error {
			c, e2 := s.workspace(e, r.Spec.Connection)
			if e2 != nil {
				return e2
			}
			raw, e2 := io.ReadAll(io.LimitReader(body, r.Spec.Bytes+1))
			if e2 != nil {
				return ErrFormat
			}
			if int64(len(raw)) != r.Spec.Bytes || contentHash(raw) != r.Spec.SHA256 {
				return ErrChecksum
			}
			if e2 = ctx.Err(); e2 != nil {
				return e2
			}
			return s.stageWorkspace(ctx, e, c, r, raw)
		})
		if e2 == nil {
			out = r.Public()
		}
		return e2
	})
	return out, err
}

// LoadUpload executes one explicit shared-ledger attempt. A lost external commit
// is recovered from the exact owned workspace receipt, never a duplicate table.
func (s *Service) LoadUpload(ctx context.Context, e identity.Envelope, id, key string, resume bool) (out UploadRun, err error) {
	return s.runUpload(ctx, e, id, key, resume, false)
}

// EraseUpload fences new reads before dropping exactly the owned workspace table.
func (s *Service) EraseUpload(ctx context.Context, e identity.Envelope, id, key string, resume bool) (out UploadRun, err error) {
	return s.runUpload(ctx, e, id, key, resume, true)
}
func (s *Service) runUpload(ctx context.Context, e identity.Envelope, id, key string, resume, erase bool) (out UploadRun, err error) {
	action, permission, kind := "sources.upload", "write", "upload.load"
	if erase {
		action, permission, kind = "sources.erase", "erase", "upload.erase"
	}
	if err = access.Require(e, action, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: permission, ID: id}); err != nil {
		return out, err
	}
	if !s.values.Uploads.Enabled || !identity.Identifier(key) {
		return out, ErrInvalid
	}
	err = s.call(ctx, e, true, func(ctx context.Context) error {
		r, err := s.repo.ReadUpload(ctx, e, id, action, permission)
		if err != nil {
			return err
		}
		if !erase && r.State == "active" || erase && r.State == "erased" {
			out.Upload = r.Public()
			out.Code = "already_complete"
			return nil
		}
		if !erase && r.State != "staged" {
			return ErrState
		}
		if !erase && time.Now().After(r.Expires) {
			return store.ErrExpired
		}
		c, err := s.workspace(e, r.Spec.Connection)
		if err != nil {
			return err
		}
		partition := ""
		if r.SourceRevision > 0 {
			partition = id + ":v" + strconv.FormatInt(r.SourceRevision, 10)
		}
		input := jobs.RequestInput{Kind: kind, Target: id, InputHash: r.SpecHash, Context: partition}
		task, err := s.runner.Admit(ctx, e, key, input)
		if err != nil {
			return err
		}
		if resume {
			task, err = s.runner.Resume(ctx, e, task.ID)
			if err != nil {
				return err
			}
		}
		r, err = s.repo.AttachUpload(ctx, e, id, task, erase)
		if err != nil {
			return err
		}
		task, runErr := s.runner.Run(ctx, e, task, time.Duration(s.values.Uploads.Timeout), func(ctx context.Context, i jobs.Invocation) error {
			if _, err := i.Current(kind, id, r.SpecHash); err != nil {
				return err
			}
			if erase {
				if err := s.eraseWorkspace(ctx, e, c, r); err != nil {
					return err
				}
				return s.repo.FinishUploadErasure(ctx, i, r)
			}
			receipt, err := s.loadWorkspace(ctx, e, c, r)
			if err != nil {
				return err
			}
			columns := make([]string, len(r.Spec.Columns))
			for i, c := range r.Spec.Columns {
				columns[i] = c.Name
			}
			return s.sources.WithManagedRecord(ctx, e, id, r.Spec.Name, r.Spec.Connection, columns, receipt.TableOID, func(ctx context.Context, source sources.Record) error {
				return s.repo.ActivateUpload(ctx, i, r, receipt, source)
			})
		})
		if task.ID == "" {
			return runErr
		}
		current, readErr := s.repo.ReadUpload(ctx, e, id, action, permission)
		if readErr != nil {
			return readErr
		}
		out = UploadRun{Upload: current.Public(), Operation: task}
		if runErr != nil {
			out.Code = engineeringCode(runErr)
		}
		return nil
	})
	return out, err
}

// RequestOperation inspects only an actor/session-bound task with current reach.
func (s *Service) RequestOperation(ctx context.Context, e identity.Envelope, id string) (out jobs.RequestTask, err error) {
	if !e.Valid() {
		return out, access.ErrUnauthenticated
	}
	if !e.Has("jobs.read") {
		return out, access.ErrForbidden
	}
	err = s.call(ctx, e, false, func(ctx context.Context) error {
		// The shared runner independently checks the actual task's original
		// domain reach and actor/session ownership, not merely its identifier.
		out, err = s.runner.Inspect(ctx, e, id)
		return err
	})
	if err != nil {
		return jobs.RequestTask{}, err
	}
	return out, nil
}

// CancelOperation persists intent; stopping the actual owner is separately observed.
func (s *Service) CancelOperation(ctx context.Context, e identity.Envelope, id string) (out jobs.RequestTask, err error) {
	if !e.Valid() {
		return out, access.ErrUnauthenticated
	}
	if !e.Has("jobs.cancel") {
		return out, access.ErrForbidden
	}
	err = s.call(ctx, e, false, func(ctx context.Context) error {
		// The shared runner independently checks the actual task's original
		// domain reach and actor/session ownership, not merely its identifier.
		out, err = s.runner.Cancel(ctx, e, id)
		return err
	})
	if err != nil {
		return jobs.RequestTask{}, err
	}
	return out, nil
}

// SweepUploads explicitly erases a bounded set of expired private staging records.
// It does not enlist retention-only broker credentials or erase active datasets.
func (s *Service) SweepUploads(ctx context.Context, e identity.Envelope, limit int) (out []UploadRun, err error) {
	if err = access.Require(e, "sources.erase", access.Tenant(e, "erase")); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 32 {
		return nil, ErrInvalid
	}
	records, err := s.repo.ExpiredUploads(ctx, e, limit)
	if err != nil {
		return nil, err
	}
	out = []UploadRun{}
	for _, r := range records {
		result, e2 := s.EraseUpload(ctx, e, r.Spec.ID, "staging-expiry-"+r.SpecHash[:32], true)
		if e2 != nil {
			return out, e2
		}
		out = append(out, result)
	}
	return out, nil
}
func engineeringCode(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, readexec.ErrTimeout):
		return "timed_out"
	case errors.Is(err, ErrLimit), errors.Is(err, readexec.ErrLimit):
		return "limit_exceeded"
	case errors.Is(err, ErrFormat), errors.Is(err, ErrChecksum), errors.Is(err, ErrInvalid):
		return "invalid_file"
	case errors.Is(err, ErrOwnership):
		return "workspace_ownership_unproven"
	case errors.Is(err, readexec.ErrBinding):
		return "context_changed"
	case errors.Is(err, access.ErrUnauthenticated), errors.Is(err, access.ErrForbidden), errors.Is(err, jobs.ErrAuthority):
		return "authority_blocked"
	case errors.Is(err, ErrState):
		return "reconciliation_required"
	default:
		return "dependency_unavailable"
	}
}

// ProtectedString rejects accidental control characters in retained display text.
func ProtectedString(s string) bool {
	return len(s) > 0 && len(s) <= 128 && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\x00\r\n\t")
}
