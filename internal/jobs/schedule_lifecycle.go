package jobs

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// ScheduleHistoryRequest pages one immutable history stream. The two cursors
// are mutually exclusive, and neither can affect accepted execution windows.
type ScheduleHistoryRequest struct {
	Kind           string     `json:"kind" jsonschema:"enum=revisions,enum=occurrences"`
	Limit          int        `json:"limit"`
	BeforeRevision int64      `json:"before_revision,omitempty"`
	BeforeDue      *time.Time `json:"before_due,omitempty"`
}

// Valid bounds both the query and the cursor representation.
func (r ScheduleHistoryRequest) Valid() bool {
	if r.Limit < 1 || r.Limit > 100 {
		return false
	}
	if r.Kind == "revisions" {
		return r.BeforeDue == nil && r.BeforeRevision >= 0 && r.BeforeRevision < 1<<62
	}
	if r.Kind != "occurrences" || r.BeforeRevision != 0 {
		return false
	}
	return r.BeforeDue == nil || (r.BeforeDue.Year() >= 2000 && r.BeforeDue.Year() <= 2101 && r.BeforeDue.Nanosecond()%1000 == 0)
}

// ScheduleRevision is a retained definition/state revision, not a permission
// grant. Existing installations retain their first recorded baseline honestly.
type ScheduleRevision struct {
	Revision   int64           `json:"revision"`
	Enabled    bool            `json:"enabled"`
	Retired    bool            `json:"retired"`
	Request    ScheduleRequest `json:"request"`
	RecordedAt time.Time       `json:"recorded_at"`
}

// ScheduleOccurrence reports the accepted half-open window or an explicit
// skipped interval. Job IDs are references; reading a job needs run authority.
type ScheduleOccurrence struct {
	DueAt          time.Time  `json:"due_at"`
	WindowStart    time.Time  `json:"window_start"`
	WindowEnd      time.Time  `json:"window_end"`
	SkippedThrough *time.Time `json:"skipped_through,omitempty"`
	Disposition    string     `json:"disposition"`
	JobID          string     `json:"job_id,omitempty"`
}

// ScheduleHistory contains exactly the requested bounded stream. A continuation
// starts strictly before the last returned coordinate, without unstable offsets.
type ScheduleHistory struct {
	ScheduleID         string               `json:"schedule_id"`
	Kind               string               `json:"kind"`
	Revisions          []ScheduleRevision   `json:"revisions"`
	Occurrences        []ScheduleOccurrence `json:"occurrences"`
	NextBeforeRevision int64                `json:"next_before_revision,omitempty"`
	NextBeforeDue      *time.Time           `json:"next_before_due,omitempty"`
}

// ScheduleLifecycleRepository extends the same shared persistent schedule
// store. Metadata management remains available while dispatch is disabled.
type ScheduleLifecycleRepository interface {
	RetireSchedule(context.Context, store.Scope, string, int64) (Schedule, error)
	ScheduleHistory(context.Context, store.Scope, string, ScheduleHistoryRequest) (ScheduleHistory, error)
}

// RetireSchedule permanently disables future admissions. Already accepted jobs
// retain their manifest and can be cancelled separately through ordinary rules.
func (s *Service) RetireSchedule(ctx context.Context, e identity.Envelope, id string, expected int64) (Schedule, error) {
	if s == nil || ctx == nil || !identity.Identifier(id) || expected < 1 || expected >= 1<<62 {
		return Schedule{}, ErrInvalid
	}
	if err := access.Require(e, "scheduling.write", access.Resource{Tenant: e.Tenant(), Kind: "schedule", Permission: "write", ID: id}); err != nil {
		return Schedule{}, err
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return Schedule{}, err
	}
	repo, ok := s.repo.(ScheduleLifecycleRepository)
	if !ok {
		return Schedule{}, ErrTransient
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	return repo.RetireSchedule(ctx, scope, id, expected)
}

// History checks signed schedule reach before reading any retained definition.
// Recipients, creators, and a previously issued execution token confer no reach.
func (s *Service) History(ctx context.Context, e identity.Envelope, id string, request ScheduleHistoryRequest) (ScheduleHistory, error) {
	if s == nil || ctx == nil || !identity.Identifier(id) || !request.Valid() {
		return ScheduleHistory{}, ErrInvalid
	}
	if err := access.Require(e, "scheduling.read", access.Resource{Tenant: e.Tenant(), Kind: "schedule", Permission: "read", ID: id}); err != nil {
		return ScheduleHistory{}, err
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return ScheduleHistory{}, err
	}
	repo, ok := s.repo.(ScheduleLifecycleRepository)
	if !ok {
		return ScheduleHistory{}, ErrTransient
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	return repo.ScheduleHistory(ctx, scope, id, request)
}

// TestSchedule admits an actual manual test occurrence through the same queue,
// overlap policy, domain consumer and fresh Pengui exchange as an ordinary run.
// It is not a dry-run claim and never bypasses pause/retirement or signed reach.
// The caller supplies an expected revision so a concurrent edit cannot change
// the definition whose execution they explicitly requested.
func (s *Service) TestSchedule(ctx context.Context, e identity.Envelope, id, key string, expected int64) (Job, error) {
	if s == nil || ctx == nil || !identity.Identifier(id) || !identity.Identifier(key) || expected < 1 || expected >= 1<<62 {
		return Job{}, ErrInvalid
	}
	if err := access.Require(e, "scheduling.execute", access.Resource{Tenant: e.Tenant(), Kind: "schedule", Permission: "execute", ID: id}); err != nil {
		return Job{}, err
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return Job{}, err
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	current, err := s.repo.ReadSchedule(ctx, scope, id)
	if err != nil {
		return Job{}, err
	}
	if current.Revision != expected || current.Retired || !current.Enabled {
		return Job{}, store.ErrConflict
	}
	if _, err := s.admission(ctx, e, current.Request.Target); err != nil {
		return Job{}, err
	}
	return s.repo.FireSchedule(ctx, scope, e.Session(), id, key, expected, s.limits)
}
