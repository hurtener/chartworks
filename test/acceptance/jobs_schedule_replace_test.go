package acceptance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
)

func TestScheduleRevisionPreservesAcceptedOccurrence(t *testing.T) {
	q := newQueueFixture(t, nil)
	ctx := context.Background()
	request := jobs.ScheduleRequest{Target: jobs.Submission{Kind: jobs.MaintenanceKind, BindingID: "maintenance"}, Spec: jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}}
	original, err := q.service.CreateSchedule(ctx, q.actor, "reviewed-schedule", request)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := q.service.Fire(ctx, q.actor, original.ID, "accepted-before-edit")
	if err != nil {
		t.Fatal(err)
	}
	request.Spec = jobs.Spec{Type: "interval", Timezone: "UTC", IntervalSeconds: 60, Anchor: time.Now().UTC().Truncate(time.Second), Missed: "skip", Overlap: "queue"}
	updated, err := q.service.ReplaceSchedule(ctx, q.actor, original.ID, original.Revision, "reviewed-change", request)
	if err != nil || updated.Revision != 2 || updated.NextDue == nil {
		t.Fatal("replacement", updated, err)
	}
	replay, err := q.service.ReplaceSchedule(ctx, q.actor, original.ID, original.Revision, "reviewed-change", request)
	if err != nil || replay.Revision != updated.Revision {
		t.Fatal("lost reply reconciliation", replay, err)
	}
	retained, err := q.service.Get(ctx, q.actor, accepted.ID)
	if err != nil || retained.ManifestHash != accepted.ManifestHash || retained.ScheduleRevision != 1 || !retained.DueAt.Equal(accepted.DueAt) || !retained.WindowStart.Equal(accepted.WindowStart) {
		t.Fatal("accepted occurrence changed", retained, err)
	}
	if _, err = q.service.SetSchedule(ctx, q.actor, original.ID, updated.Revision, false); err != nil {
		t.Fatal(err)
	}
	if _, err = q.service.ReplaceSchedule(ctx, q.actor, original.ID, updated.Revision, "reviewed-change", request); !errors.Is(err, store.ErrConflict) {
		t.Fatal("unrelated pause adopted as replacement", err)
	}
}
