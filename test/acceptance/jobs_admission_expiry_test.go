package acceptance

import (
	"context"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/test/support"
)

func TestQueueAdmissionCannotOutliveBearer(t *testing.T) {
	for _, schedule := range []bool{false, true} {
		name := "submit"
		if schedule {
			name = "schedule"
		}
		t.Run(name, func(t *testing.T) {
			q := newQueueFixture(t, nil)
			cfg := q.token.cfg
			cfg.ClockSkew = 0
			verifier, err := auth.New(cfg, q.token.server.Client(), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(verifier.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			raw := support.Raw(t, q.dsn)
			tx, err := raw.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(context.Background()) }()
			if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7214060601)`); err != nil {
				t.Fatal(err)
			}
			claims := q.token.claims(q.actor.Tenant(), q.actor.User(), q.actor.Scopes())
			claims["exp"] = time.Now().Unix() + 2
			e, err := verifier.Verify(ctx, q.token.sign(t, claims, nil), auth.HTTP)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				target := jobs.Submission{Kind: jobs.MaintenanceKind, BindingID: "maintenance"}
				var callErr error
				if schedule {
					_, callErr = q.service.CreateSchedule(ctx, e, "expires-while-waiting", jobs.ScheduleRequest{Target: target, Spec: jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}})
				} else {
					_, callErr = q.service.Submit(ctx, e, "expires-while-waiting", target)
				}
				done <- callErr
			}()
			waiting := false
			for until := time.Now().Add(750 * time.Millisecond); time.Now().Before(until); {
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND wait_event='advisory')`).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !waiting {
				t.Fatal("admission did not reach the held metadata lock")
			}
			// The real metadata lock remains held beyond the signed expiry. A
			// caller timeout is deliberately later and cannot enforce this check.
			timer := time.NewTimer(time.Until(e.Deadline()) + 100*time.Millisecond)
			defer timer.Stop()
			<-timer.C
			if err = tx.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err == nil {
				t.Fatal("expired authority admitted durable work after lock release")
			}
			var count int
			if err = raw.QueryRow(ctx, `SELECT (SELECT count(*) FROM chartworks.operations)+(SELECT count(*) FROM chartworks.job_schedules)`).Scan(&count); err != nil || count != 0 {
				t.Fatal("expired admission left durable work", count, err)
			}
		})
	}
}
