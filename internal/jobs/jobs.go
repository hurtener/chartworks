// Package jobs owns the single durable operation queue and bounded maintenance scheduling.
// Admission uses supplied Pengui authority; every attempt obtains new Pengui-issued authority.
package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

var (
	ErrInvalid   = errors.New("jobs: invalid request")
	ErrBusy      = errors.New("jobs: queue capacity reached")
	ErrEmpty     = errors.New("jobs: no eligible operation")
	ErrAuthority = errors.New("jobs: fresh execution authority unavailable")
	ErrTransient = errors.New("jobs: execution authority temporarily unavailable")
	ErrRunning   = errors.New("jobs: worker already running")
)

// MaintenanceKind is the only executable target implemented in this phase.
const MaintenanceKind = "retention.sweep"

// Limits apply at admission and across all replicas sharing the same database.
// Database configuration is pinned by the first enabled worker and checked by other replicas.
type Limits struct {
	Workers             int           `json:"workers"`
	GlobalConcurrency   int           `json:"global_concurrency"`
	TenantConcurrency   int           `json:"tenant_concurrency"`
	MaxPending          int           `json:"max_pending"`
	MaxPendingPerTenant int           `json:"max_pending_per_tenant"`
	MaxAttempts         int           `json:"max_attempts"`
	Batch               int           `json:"batch"`
	Lease               time.Duration `json:"-"`
	Heartbeat           time.Duration `json:"-"`
	Poll                time.Duration `json:"-"`
	AttemptTimeout      time.Duration `json:"-"`
	Backoff             time.Duration `json:"-"`
}

// Defaults returns the bounded reference worker and admission limits.
func Defaults() Limits {
	return Limits{Workers: 4, GlobalConcurrency: 16, TenantConcurrency: 2, MaxPending: 10000, MaxPendingPerTenant: 1000, MaxAttempts: 3, Batch: 100, Lease: 15 * time.Second, Heartbeat: 5 * time.Second, Poll: 500 * time.Millisecond, AttemptTimeout: 10 * time.Second, Backoff: time.Second}
}

// Validate rejects malformed or unbounded values before use.
func (l Limits) Validate() error {
	if l.Workers < 1 || l.Workers > 32 || l.GlobalConcurrency < l.Workers || l.GlobalConcurrency > 128 || l.TenantConcurrency < 1 || l.TenantConcurrency > l.GlobalConcurrency || l.MaxPending < 1 || l.MaxPending > 100000 || l.MaxPendingPerTenant < 1 || l.MaxPendingPerTenant > l.MaxPending || l.MaxAttempts < 1 || l.MaxAttempts > 8 || l.Batch < 1 || l.Batch > 1000 || l.Lease < time.Second || l.Lease > time.Minute || l.Heartbeat < 10*time.Millisecond || l.Heartbeat >= l.Lease/2 || l.Poll < 10*time.Millisecond || l.Poll > 5*time.Second || l.AttemptTimeout < 100*time.Millisecond || l.AttemptTimeout > time.Minute || l.Backoff < 10*time.Millisecond || l.Backoff > 30*time.Second {
		return ErrInvalid
	}
	return nil
}

// QueueFingerprint excludes per-process worker/poll settings, but pins global admission bounds.
func (l Limits) QueueFingerprint() string {
	b, _ := json.Marshal([]int{l.GlobalConcurrency, l.TenantConcurrency, l.MaxPending, l.MaxPendingPerTenant})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Submission is closed: there is no caller-selected actor, tenant, credential, SQL or timestamp.
type Submission struct {
	Kind      string `json:"kind"`
	BindingID string `json:"binding_id"`
}

// Validate rejects malformed or unbounded values before use.
func (s Submission) Validate() error {
	if s.Kind != MaintenanceKind || !BindingID(s.BindingID) {
		return ErrInvalid
	}
	return nil
}

// BindingID validates the opaque binding grammar shared with Pengui execution authority v1.
func BindingID(s string) bool {
	return identity.Identifier(s) && len(s) <= 64 && !strings.Contains(s, ":")
}

// Executor derives the expected Pengui service attribution; the name alone grants no authority.
func Executor(binding string) string { return "svc:chartworks:" + binding }

// Job is retained execution metadata, not authority. All temporal values are UTC microseconds.
type Job struct {
	ID                string    `json:"id"`
	Tenant            string    `json:"tenant"`
	Kind              string    `json:"kind"`
	BindingID         string    `json:"binding_id"`
	Executor          string    `json:"executor"`
	Initiator         string    `json:"initiator"`
	InitiatorSession  string    `json:"initiator_session"`
	State             string    `json:"state"`
	ErrorCode         string    `json:"error_code,omitempty"`
	PolicyRevision    int64     `json:"policy_revision"`
	DueAt             time.Time `json:"due_at"`
	WindowStart       time.Time `json:"window_start"`
	WindowEnd         time.Time `json:"window_end"`
	Cutoff            time.Time `json:"cutoff"`
	Batch             int       `json:"batch"`
	Attempts          int       `json:"attempts"`
	MaxAttempts       int       `json:"max_attempts"`
	ScheduleID        string    `json:"schedule_id,omitempty"`
	ScheduleRevision  int64     `json:"schedule_revision,omitempty"`
	ManifestHash      string    `json:"manifest_hash"`
	DeletedEvents     int64     `json:"deleted_events"`
	DeletedOperations int64     `json:"deleted_operations"`
}

// Digest binds only the immutable accepted manifest; attempt/result state cannot change its meaning.
func (j Job) Digest() string {
	b, _ := json.Marshal([]any{"chartworks-operation-v1", j.Tenant, j.ID, j.Kind, j.BindingID, j.Executor, j.Initiator, j.InitiatorSession, j.PolicyRevision, j.DueAt.UTC(), j.WindowStart.UTC(), j.WindowEnd.UTC(), j.Cutoff.UTC(), j.Batch, j.ScheduleID, j.ScheduleRevision})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Valid checks the value's invariants and any attached authority expiry.
func (j Job) Valid() bool {
	return identity.Identifier(j.ID) && identity.Identifier(j.Tenant) && BindingID(j.BindingID) && j.Kind == MaintenanceKind && j.Executor == Executor(j.BindingID) && identity.Identifier(j.Initiator) && identity.Identifier(j.InitiatorSession) && j.PolicyRevision > 0 && j.Batch >= 1 && j.Batch <= 1000 && !j.DueAt.IsZero() && !j.WindowStart.After(j.WindowEnd) && j.WindowEnd.Equal(j.DueAt) && j.ManifestHash == j.Digest()
}

// Lease fences bookkeeping and completion; it is never a replacement for fresh signed authority.
type Lease struct {
	Job     Job
	Owner   string
	Fence   int64
	Attempt int
	Until   time.Time
}

// Authority is implemented by the concrete Pengui HTTP adapter, not a local signer.
type Authority interface {
	Acquire(context.Context, Job) (auth.Execution, error)
}

// AssertExecution is repeated at the effect boundary, after broker verification and before writes.
func AssertExecution(proof auth.Execution, j Job) error {
	e := proof.Envelope()
	if !proof.Matches(j.BindingID, j.ID, j.ManifestHash) {
		return ErrAuthority
	}
	if !j.Valid() || !e.Valid() || e.Tenant() != j.Tenant || e.User() != j.Executor || e.Session() != j.ID {
		return ErrAuthority
	}
	if err := access.Require(e, "ops.maintain", access.Resource{Tenant: j.Tenant, Kind: "tenant", Permission: "erase", ID: j.Tenant}, access.Resource{Tenant: j.Tenant, Kind: "execution_binding", Permission: "use", ID: j.BindingID}, access.Resource{Tenant: j.Tenant, Kind: "run", Permission: "execute", ID: j.ID}); err != nil {
		return ErrAuthority
	}
	return nil
}

// Repository extends the existing operation ledger; there is no parallel queue transport.
type Repository interface {
	ConfigureQueue(context.Context, Limits) error
	AdmitJob(context.Context, store.Scope, string, string, Submission, Limits) (Job, error)
	ReadJob(context.Context, store.Scope, string) (Job, error)
	ListJobs(context.Context, store.Scope, access.Selection, int) ([]Job, error)
	CancelJob(context.Context, store.Scope, string) (Job, error)
	ClaimJob(context.Context, string, Limits) (Lease, error)
	HeartbeatJob(context.Context, Lease, time.Duration) error
	CompleteJob(context.Context, Lease, auth.Execution) (Job, error)
	FinishAttempt(context.Context, Lease, string, bool, time.Duration) error
	CreateSchedule(context.Context, store.Scope, string, string, ScheduleRequest, Limits) (Schedule, error)
	ReadSchedule(context.Context, store.Scope, string) (Schedule, error)
	SetSchedule(context.Context, store.Scope, string, int64, bool) (Schedule, error)
	FireSchedule(context.Context, store.Scope, string, string, string, Limits) (Job, error)
	TickSchedules(context.Context, Limits) (int, error)
}
