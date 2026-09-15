package acceptance

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

type replaceBeforeFire struct {
	jobs.Repository
	replace func(jobs.Schedule) error
	called  atomic.Bool
}

func (r *replaceBeforeFire) ReadSchedule(ctx context.Context, scope store.Scope, id string) (jobs.Schedule, error) {
	s, err := r.Repository.ReadSchedule(ctx, scope, id)
	if err == nil && r.called.CompareAndSwap(false, true) {
		err = r.replace(s)
	}
	return s, err
}

func TestManualFirePinsAuthorizedScheduleRevision(t *testing.T) {
	q := newQueueFixture(t, nil)
	ctx := context.Background()
	request := jobs.ScheduleRequest{Target: jobs.Submission{Kind: jobs.MaintenanceKind, BindingID: "maintenance"}, Spec: jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}}
	schedule, err := q.service.CreateSchedule(ctx, q.actor, "fire-revision", request)
	if err != nil {
		t.Fatal(err)
	}
	limited := q.token.envelope(t, q.actor.Tenant(), q.actor.User(), slices.DeleteFunc(q.actor.Scopes(), func(scope string) bool { return scope == "cw.execution_binding.use:other" })...)
	interposed := &replaceBeforeFire{Repository: q.db, replace: func(old jobs.Schedule) error {
		changed := old.Request
		changed.Target.BindingID = "other"
		_, changeErr := q.service.ReplaceSchedule(ctx, q.actor, old.ID, old.Revision, "replace-before-fire", changed)
		return changeErr
	}}
	service, err := jobs.New(interposed, q.provider, q.limits)
	if err != nil {
		t.Fatal(err)
	}
	// Fire reads and authorizes the old target. Another authorized request then
	// commits a new target before admission acquires the schedule row lock.
	if _, err = service.Fire(ctx, limited, schedule.ID, "fire-old-authority"); !errors.Is(err, store.ErrConflict) || !interposed.called.Load() {
		t.Fatal("manual fire admitted a different target than it authorized", err)
	}
	raw := support.Raw(t, q.dsn)
	var count int
	if err = raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.operations`).Scan(&count); err != nil || count != 0 {
		t.Fatal("changed target left an unauthorized occurrence", count, err)
	}
	if _, err = q.service.Fire(ctx, q.actor, schedule.ID, "fire-current-authority"); err != nil {
		t.Fatal("current authorized revision could not execute", err)
	}
}
