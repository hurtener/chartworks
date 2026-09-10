package acceptance

import (
	"context"
	"testing"
	"time"

	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

// Hold a real warehouse relation lock until explicit cancellation has stopped
// the request. Observing the lock wait avoids racing a trivially fast SELECT.
func phase28CancelBlockedWarehouse(t *testing.T, dsn string, client *sdk.Client, block string) {
	t.Helper()
	ctx := context.Background()
	run, err := client.AdmitReportingRun(ctx, block, sdk.ReportingRunRequest{Key: "p28-running-cancel"})
	if err != nil {
		t.Fatal(err)
	}
	raw := support.Raw(t, dsn)
	lock, err := raw.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Rollback(context.Background()) }()
	if _, err = lock.Exec(ctx, `LOCK TABLE analytics.sales IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	work, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, executeErr := client.ExecuteReportingRun(work, run.ID, sdk.ReportingRunDispatch{})
		done <- executeErr
	}()
	joined := false
	defer func() {
		cancel()
		_ = lock.Rollback(context.Background())
		if !joined {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("cancelled execution did not join")
			}
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		if err = lock.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE relation='analytics.sales'::regclass AND NOT granted)`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err = <-done:
			joined = true
			t.Fatal("execution ended before warehouse lock wait", err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !blocked {
		t.Fatal("execution never reached the blocked warehouse")
	}
	cancelled, err := client.CancelReportingRun(ctx, run.ID)
	if err != nil || cancelled.State != "cancelled" {
		t.Fatal("running cancellation failed", cancelled, err)
	}
	select {
	case err = <-done:
		joined = true
		if err == nil {
			t.Fatal("cancelled warehouse execution reported success")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("durable cancellation did not stop blocked warehouse work")
	}
	if _, err = client.ReportingRunRows(ctx, run.ID, 0, 1); err == nil {
		t.Fatal("cancelled run exposed result values")
	}
}
