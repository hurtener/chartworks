package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

type acknowledgedRead struct{ ready chan readexec.RemoteQuery }

func (a acknowledgedRead) Dispatch(_ context.Context, q readexec.RemoteQuery, accepted bool) error {
	if accepted {
		a.ready <- q
	}
	return nil
}
func (a acknowledgedRead) Check(ctx context.Context) error { return ctx.Err() }

func TestReadOldCancellationCannotSignalReusedBackend(t *testing.T) {
	f := newSourceFixture(t, func(c *config.Sources) { c.MaxConns = 2 })
	ctx := context.Background()
	if _, err := f.admin.Exec(ctx, `INSERT INTO analytics.items SELECT i,1 FROM generate_series(3,15000) i`); err != nil {
		t.Fatal(err)
	}
	source := f.create(t, "sales")
	small := f.plan(t, source, `SELECT id FROM analytics.sales ORDER BY id`)
	heavy := f.plan(t, source, `SELECT sum(i.quantity::bigint*j.quantity) AS total FROM analytics.items i CROSS JOIN analytics.items j`)
	x, err := readexec.NewExecutor(&lostReadReply{ExecutionAdapter: f.s, lose: true}, f.db, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	old := executeFixture(t, x, f.e, small, readOptions("old-backend"))
	if old.Attempt.Status != "uncertain" || old.Attempt.Remote == nil {
		t.Fatal("missing original physical identity")
	}
	observer := acknowledgedRead{ready: make(chan readexec.RemoteQuery, 1)}
	run, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 1)
	go func() {
		_, e := f.s.ExecuteRead(run, f.e, heavy, readexec.Limits{Rows: 1, Bytes: 4096, Timeout: 15 * time.Second, CancelGrace: 2 * time.Second, PlannerCost: 1e9}, strings.Repeat("b", 32), observer)
		done <- e
	}()
	var next readexec.RemoteQuery
	select {
	case next = <-observer.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("new query did not dispatch")
	}
	if next.Driver != "postgres" || next.Postgres.PID != old.Attempt.Remote.Postgres.PID {
		stop()
		<-done
		t.Fatal("fixture did not actually reuse the backend PID")
	}
	receipt, err := x.Control(ctx, f.e, old.Attempt.ID, true)
	if err != nil || receipt.Attempt.Status != "interrupted" || receipt.RemoteState != "stopped" {
		stop()
		<-done
		t.Fatal("old attempt reconciliation", err, receipt)
	}
	select {
	case e := <-done:
		t.Fatal("old cancellation interrupted a different query", e)
	case <-time.After(50 * time.Millisecond):
	}
	var active bool
	if err = f.admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND application_name=$2)`, next.Postgres.PID, next.Tag).Scan(&active); err != nil || !active {
		stop()
		<-done
		t.Fatal("new transaction disappeared", err)
	}
	stop()
	select {
	case e := <-done:
		if !errors.Is(e, readexec.ErrCancelled) {
			t.Fatal("new owner's cancellation", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("new query did not join")
	}
}

func TestReadReconciliationCannotSwitchCredentialContext(t *testing.T) {
	f := newSourceFixture(t, nil)
	source := f.create(t, "sales")
	p := f.plan(t, source, `SELECT id FROM analytics.sales`)
	x, err := readexec.NewExecutor(&lostReadReply{ExecutionAdapter: f.s, lose: true}, f.db, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	r := executeFixture(t, x, f.e, p, readOptions("changed-observation-context"))
	original := f.readDSN()
	f.setReadDSN(f.dsn)
	control, err := x.Control(context.Background(), f.e, r.Attempt.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if control.RemoteState == "stopped" || control.Attempt.Status != "uncertain" {
		t.Fatal("a different database/role falsely proved old termination", control)
	}
	f.setReadDSN(original)
	until := time.Now().Add(2 * time.Second)
	for time.Now().Before(until) {
		control, err = x.Control(context.Background(), f.e, r.Attempt.ID, false)
		if err != nil {
			t.Fatal(err)
		}
		if control.RemoteState == "stopped" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if control.RemoteState != "stopped" || control.Attempt.Status != "interrupted" {
		t.Fatal("original context did not reconcile", control)
	}
}

type readFailureAdapter struct {
	readexec.ExecutionAdapter
	failure error
	outcome string
}

func (a readFailureAdapter) ExecuteRead(context.Context, identity.Envelope, readexec.Plan, readexec.Limits, string, readexec.Observer) (readexec.NativeResult, error) {
	return readexec.NativeResult{RemoteState: "not_issued", Result: readexec.Result{Outcome: a.outcome}}, a.failure
}
func TestReadTypedFailureContractAndInvalidAdmission(t *testing.T) {
	f := newSourceFixture(t, nil)
	source := f.create(t, "sales")
	p := f.plan(t, source, `SELECT id FROM analytics.sales`)
	settings := config.DefaultReadValidation()
	before := f.lookups.Load()
	for i, c := range []struct {
		err                   error
		outcome, status, code string
	}{{readexec.ErrUnsupported, "", "failed", "unsupported"}, {readexec.ErrUncertain, "", "uncertain", "remote_outcome_unknown"}, {nil, "invented", "failed", "invalid_result"}, {store.ErrUnavailable, "", "failed", "source_unavailable"}} {
		x, err := readexec.NewExecutor(readFailureAdapter{ExecutionAdapter: f.s, failure: c.err, outcome: c.outcome}, f.db, settings)
		if err != nil {
			t.Fatal(err)
		}
		r := executeFixture(t, x, f.e, p, readOptions("typed-"+string(rune('a'+i))))
		if r.Result != nil || r.Attempt.Status != c.status || r.Attempt.Code != c.code {
			t.Fatal("failure contract", r.Attempt)
		}
	}
	if f.lookups.Load() != before {
		t.Fatal("failure fixtures unexpectedly queried a source")
	}
	x := readExecutor(t, f, nil)
	for _, o := range []readexec.Options{{Operation: "../bad", Number: 1}, {Operation: "number", Number: 0}, {Operation: "number", Number: 4}, {Operation: "rows", Number: 1, Rows: -1}, {Operation: "bytes", Number: 1, Bytes: 127}, {Operation: "bytes", Number: 1, Bytes: 17 << 20}} {
		if _, err := x.Execute(context.Background(), f.e, p, o); !errors.Is(err, readexec.ErrLimit) {
			t.Fatal("invalid execution bounds", err)
		}
	}
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	for _, invalid := range []struct {
		name string
		ctx  context.Context
	}{{"nil", nil}, {"expired", expired}} {
		if _, err := x.Execute(invalid.ctx, f.e, p, readOptions("invalid-context")); err == nil {
			t.Fatal("invalid execution context accepted", invalid.name)
		}
		if _, err := x.ByOperation(invalid.ctx, f.e, "valid"); err == nil {
			t.Fatal("invalid operation context accepted", invalid.name)
		}
	}
	if _, err := x.Execute(context.Background(), identity.Envelope{}, p, readOptions("zero-authority")); err == nil {
		t.Fatal("zero authority")
	}
	if _, err := x.Inspect(context.Background(), f.e, "../bad"); err == nil {
		t.Fatal("invalid attempt")
	}
	if _, err := x.ByOperation(context.Background(), f.e, "../bad"); err == nil {
		t.Fatal("invalid operation")
	}
	if _, err := x.ByOperation(context.Background(), f.e, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("missing operation", err)
	}
	if _, err := readexec.NewExecutor(nil, f.db, settings); err == nil {
		t.Fatal("nil adapter")
	}
	if _, err := readexec.NewExecutor(f.s, nil, settings); err == nil {
		t.Fatal("nil journal")
	}
	settings.ExecutionConcurrency = 16
	if _, err := readexec.NewExecutor(f.s, f.db, settings); err == nil {
		t.Fatal("journal capacity not reserved")
	}
}
