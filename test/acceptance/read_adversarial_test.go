package acceptance

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

type heldReadObserver struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (h *heldReadObserver) Check(ctx context.Context) error { return ctx.Err() }
func (h *heldReadObserver) Dispatch(ctx context.Context, _ readexec.RemoteQuery, accepted bool) error {
	if accepted {
		return nil
	}
	h.once.Do(func() { close(h.entered) })
	select {
	case <-h.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestReadRevisionFenceExceedsMetadataTimeout(t *testing.T) {
	f := newSourceFixture(t, nil)
	source := f.create(t, "sales")
	p := f.plan(t, source, `SELECT id FROM analytics.sales ORDER BY id`)
	h := &heldReadObserver{entered: make(chan struct{}), release: make(chan struct{})}
	limits := readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 12 * time.Second, CancelGrace: time.Second, PlannerCost: 1e7}
	type completion struct {
		result readexec.NativeResult
		err    error
	}
	done := make(chan completion, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		r, e := f.s.ExecuteRead(ctx, f.e, p, limits, strings.Repeat("a", 32), h)
		done <- completion{r, e}
	}()
	select {
	case <-h.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("source execution never acquired revision fence")
	}
	// Exercise the actual WithSource transaction past the old five-second timeout.
	time.Sleep(5200 * time.Millisecond)
	meta := support.Raw(t, f.dsn)
	bounded, stop := context.WithTimeout(ctx, 100*time.Millisecond)
	_, err := meta.Exec(bounded, `UPDATE chartworks.sources SET current_revision=current_revision WHERE tenant_id=$1 AND source_id=$2`, f.e.Tenant(), source.ID)
	stop()
	if err == nil {
		t.Fatal("concurrent revision update bypassed held source fence")
	}
	close(h.release)
	select {
	case c := <-done:
		if c.err != nil || c.result.Result.Outcome != "succeeded" {
			t.Fatal("ordinary metadata timeout cut off valid read", c.err, c.result.RemoteState)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("read did not join")
	}
}

type cancelAtFinish struct{ *postgres.DB }

func (r cancelAtFinish) FinishRead(ctx context.Context, s store.Scope, a readexec.Attempt, reconcile bool) error {
	if err := r.DB.CancelRead(ctx, s, a.ID); err != nil {
		return err
	}
	return r.DB.FinishRead(ctx, s, a, reconcile)
}
func TestReadLateCancellationCannotPublishRows(t *testing.T) {
	f := newSourceFixture(t, nil)
	source := f.create(t, "sales")
	p := f.plan(t, source, `SELECT amount FROM analytics.sales`)
	x, err := readexec.NewExecutor(f.s, cancelAtFinish{f.db}, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	r := executeFixture(t, x, f.e, p, readOptions("late-cancel"))
	if r.Result != nil || r.Attempt.Status != "cancelled" || r.Attempt.Rows != 0 || r.Attempt.Bytes != 0 || !r.Attempt.CancelRequested {
		t.Fatal("cancellation race leaked completed rows", r.Attempt)
	}
	meta := support.Raw(t, f.dsn)
	var success, cancelled int
	if err := meta.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE action='read.succeeded'),count(*) FILTER(WHERE action='read.cancelled') FROM chartworks.audit_events WHERE resource_id=$1`, r.Attempt.ID).Scan(&success, &cancelled); err != nil || success != 0 || cancelled != 1 {
		t.Fatal("audit contradicted committed cancellation", err, success, cancelled)
	}

}

type lostReadReply struct {
	readexec.ExecutionAdapter
	mu   sync.Mutex
	lose bool
}

func (a *lostReadReply) ExecuteRead(ctx context.Context, e identity.Envelope, p readexec.Plan, l readexec.Limits, id string, o readexec.Observer) (readexec.NativeResult, error) {
	out, err := a.ExecutionAdapter.ExecuteRead(ctx, e, p, l, id, o)
	a.mu.Lock()
	lose := a.lose
	a.lose = false
	a.mu.Unlock()
	if err == nil && lose {
		return readexec.NativeResult{RemoteState: "unknown"}, readexec.ErrUncertain
	}
	return out, err
}
func TestReadUncertaintyRequiresObservedReconciliation(t *testing.T) {
	f := newSourceFixture(t, nil)
	source := f.create(t, "sales")
	p := f.plan(t, source, `SELECT id FROM analytics.sales ORDER BY id`)
	x, err := readexec.NewExecutor(&lostReadReply{ExecutionAdapter: f.s, lose: true}, f.db, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	o := readOptions("lost-reply")
	r := executeFixture(t, x, f.e, p, o)
	if r.Result != nil || r.Attempt.Status != "uncertain" {
		t.Fatal("lost result fabricated success")
	}
	recovered, err := x.ByOperation(context.Background(), f.e, o.Operation)
	if err != nil || recovered.ID != r.Attempt.ID {
		t.Fatal("lost HTTP response cannot be recovered", err)
	}
	o.Number = 2
	if _, err = x.Execute(context.Background(), f.e, p, o); !errors.Is(err, store.ErrConflict) {
		t.Fatal("uncertain work automatically retried", err)
	}
	control, err := x.Control(context.Background(), f.e, r.Attempt.ID, false)
	if err != nil || control.RemoteState != "stopped" || control.Attempt.Status != "interrupted" || control.Attempt.Code != "result_not_retained" {
		t.Fatal("reconciliation fabricated result or missed stopped backend", err, control)
	}
	r = executeFixture(t, x, f.e, p, o)
	if r.Result == nil || r.Attempt.Number != 2 {
		t.Fatal("explicit reconciled retry did not execute")
	}
}

func TestReadJournalFailureDropsValues(t *testing.T) {
	f := newSourceFixture(t, nil)
	source := f.create(t, "sales")
	p := f.plan(t, source, `SELECT id FROM analytics.sales`)
	x := readExecutor(t, f, func(c *config.ReadValidation) {
		c.Timeout = config.Duration(time.Second)
		c.CancelGrace = config.Duration(250 * time.Millisecond)
	})
	meta := support.Raw(t, f.dsn)
	ctx := context.Background()
	sql(t, meta, `CREATE FUNCTION chartworks.fail_read_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='read.succeeded' THEN RAISE EXCEPTION 'synthetic receipt failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_read_receipt BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.fail_read_receipt()`)
	r, err := x.Execute(ctx, f.e, p, readOptions("receipt-failure"))
	if !errors.Is(err, readexec.ErrUncertain) || r.Result != nil {
		t.Fatal("unjournaled success leaked", err)
	}
	sql(t, meta, `DROP TRIGGER fail_read_receipt ON chartworks.audit_events`)
	time.Sleep(1300 * time.Millisecond)
	a, err := x.ByOperation(ctx, f.e, "receipt-failure")
	if err != nil || a.Status != "uncertain" || a.Remote == nil {
		t.Fatal("crash uncertainty missing", err, a)
	}
	c, err := x.Control(ctx, f.e, a.ID, false)
	if err != nil || c.Attempt.Status != "interrupted" {
		t.Fatal("failed receipt could not be reconciled", err, c)
	}
}

func TestReadDuplicateAdmissionAndJournalPrivacy(t *testing.T) {
	f := newSourceFixture(t, nil)
	source := f.create(t, "sales")
	x := readExecutor(t, f, nil)
	p := f.plan(t, source, `SELECT $1::text AS value`, readexec.Parameter{Kind: "text", Value: "PRIVATE_EXECUTION_VALUE_CANARY"})
	before := f.lookups.Load()
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := x.Execute(context.Background(), f.e, p, readOptions("same-attempt"))
			if err == nil && r.Result != nil {
				mu.Lock()
				accepted++
				mu.Unlock()
			} else if !errors.Is(err, readexec.ErrReplay) {
				t.Errorf("unexpected duplicate outcome %v", err)
			}
		}()
	}
	wg.Wait()
	if accepted != 1 || f.lookups.Load() != before+1 {
		t.Fatal("duplicate physical queries", accepted)
	}
	meta := support.Raw(t, f.dsn)
	var contents string
	if err := meta.QueryRow(context.Background(), `SELECT manifest::text FROM chartworks.read_attempts WHERE operation_id='same-attempt'`).Scan(&contents); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"PRIVATE_EXECUTION_VALUE_CANARY", "SELECT $1", sourcePassword, "CHARTWORKS_SOURCE_READ"} {
		if strings.Contains(contents, s) {
			t.Fatal("sensitive execution data retained")
		}
	}
	if _, err := meta.Exec(context.Background(), `UPDATE chartworks.read_attempts SET operation_id='changed' WHERE operation_id='same-attempt'`); err == nil {
		t.Fatal("operation manifest mutable")
	}
	if _, err := meta.Exec(context.Background(), `UPDATE chartworks.read_attempts SET status='accepted' WHERE operation_id='same-attempt'`); err == nil {
		t.Fatal("terminal receipt reopened")
	}
	if _, err := f.s.ControlRead(context.Background(), f.e, readexec.Control{}, true); err == nil {
		t.Fatal("forged control reached backend")
	}
}

func TestReadSameConnectionAlternatesActualResultSchema(t *testing.T) {
	f := newSourceFixture(t, func(c *config.Sources) { c.MaxConns = 1 })
	source := f.create(t, "sales")
	x := readExecutor(t, f, nil)
	for i, query := range []string{`SELECT id FROM analytics.sales`, `SELECT amount,cash FROM analytics.sales`, `SELECT active,name FROM analytics.sales`, `SELECT id FROM analytics.sales`} {
		p := f.plan(t, source, query)
		r := executeFixture(t, x, f.e, p, readOptions("schema-"+string(rune('a'+i))))
		if r.Result == nil || len(r.Result.Schema) != len(p.Receipt().Columns) {
			t.Fatal("stale prepared FETCH schema", r.Attempt)
		}
	}
	// The native proof rejects a read-write transaction independently of SQL parsing.
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, f.readDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var readonly string
	if err = tx.QueryRow(ctx, `SHOW transaction_read_only`).Scan(&readonly); err != nil || readonly != "off" {
		t.Fatal("fixture must distinguish native read-only enforcement", err)
	}
}
