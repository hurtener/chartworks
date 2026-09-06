package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func readExecutor(t *testing.T, f *sourceFixture, change func(*config.ReadValidation)) *readexec.Executor {
	t.Helper()
	settings := config.DefaultReadValidation()
	if change != nil {
		change(&settings)
	}
	x, err := readexec.NewExecutor(f.s, f.db, settings)
	if err != nil {
		t.Fatal("executor construction", err)
	}
	return x
}
func executeFixture(t *testing.T, x *readexec.Executor, e identity.Envelope, p readexec.Plan, o readexec.Options) readexec.ExecutionReport {
	t.Helper()
	out, err := x.Execute(context.Background(), e, p, o)
	if err != nil {
		t.Fatal("execution admission", err)
	}
	return out
}
func readOptions(id string) readexec.Options { return readexec.Options{Operation: id, Number: 1} }

func TestPhase10(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		f := newSourceFixture(t, nil)
		source := f.create(t, "sales")
		x := readExecutor(t, f, nil)
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, f.readDSN())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close(ctx) }()
		tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO analytics.items VALUES(9,9)`)
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "25006" {
			t.Fatal("real read-only transaction did not reject write", err)
		}
		_ = tx.Rollback(ctx)
		before := f.lookups.Load()
		for _, sql := range []string{`UPDATE analytics.sales SET amount=0`, `SELECT pg_read_file('/etc/passwd')`, `SELECT pg_sleep(1)`, `SELECT public.unproven_function(1)`} {
			if _, err = f.validator.Validate(ctx, f.e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: sql}); err == nil {
				t.Fatal("unsafe execution accepted")
			}
		}
		if f.lookups.Load() != before {
			t.Fatal("rejection reached warehouse")
		}
		p := f.plan(t, source, `SELECT id FROM analytics.sales ORDER BY id`)
		r := executeFixture(t, x, f.e, p, readOptions("readonly"))
		if r.Attempt.Status != "succeeded" || r.Result == nil || len(r.Result.Rows) != 2 || r.Attempt.RemoteState != "stopped" || r.Attempt.Remote == nil || f.writeLookups.Load() != 0 {
			t.Fatal("read-only native execution evidence", r.Attempt)
		}
		var count int
		if err = f.admin.QueryRow(ctx, `SELECT count(*) FROM analytics.items`).Scan(&count); err != nil || count != 2 {
			t.Fatal("baseline was written", err, count)
		}
	})
	t.Run("AC02", func(t *testing.T) {
		f := newSourceFixture(t, nil)
		ctx := context.Background()
		if _, err := f.admin.Exec(ctx, `INSERT INTO analytics.items SELECT i,1 FROM generate_series(3,15000) i`); err != nil {
			t.Fatal(err)
		}
		source := f.create(t, "sales")
		p := f.plan(t, source, `SELECT sum(i.quantity::bigint*j.quantity) AS total FROM analytics.items i CROSS JOIN analytics.items j`)
		x := readExecutor(t, f, nil)
		type completion struct {
			report readexec.ExecutionReport
			err    error
		}
		done := make(chan completion, 1)
		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() { r, e := x.Execute(runCtx, f.e, p, readOptions("cancel-native")); done <- completion{r, e} }()
		meta := support.Raw(t, f.dsn)
		id := ""
		until := time.Now().Add(5 * time.Second)
		for time.Now().Before(until) {
			_ = meta.QueryRow(ctx, `SELECT attempt_id FROM chartworks.read_attempts WHERE operation_id='cancel-native' AND status='running'`).Scan(&id)
			if id != "" {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if id == "" {
			cancel()
			t.Fatal("native attempt never acknowledged")
		}
		control, err := x.Control(ctx, f.e, id, true)
		if err != nil || !control.Attempt.CancelRequested {
			t.Fatal("cancel intent missing", err)
		}
		select {
		case c := <-done:
			if c.err != nil || c.report.Result != nil || c.report.Attempt.Status != "cancelled" || c.report.Attempt.RemoteState != "stopped" {
				t.Fatal("cancel did not terminate/acknowledge native work", c.err, c.report.Attempt)
			}
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatal("cancelled query remained active")
		}
		var active bool
		if err = f.admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1)`, "cw-read:"+id).Scan(&active); err != nil || active {
			t.Fatal("tagged remote transaction survived cancellation", err)
		}
		short := readExecutor(t, f, func(c *config.ReadValidation) { c.Timeout = config.Duration(250 * time.Millisecond) })
		r := executeFixture(t, short, f.e, p, readOptions("native-timeout"))
		if r.Result != nil || r.Attempt.Status != "timed_out" || r.Attempt.RemoteState != "stopped" {
			t.Fatal("timeout was disguised or not reconciled", r.Attempt)
		}
		cancelled, stop := context.WithCancel(ctx)
		stop()
		before := f.lookups.Load()
		if _, err = x.Execute(cancelled, f.e, p, readOptions("cancel-before-admission")); !errors.Is(err, context.Canceled) || f.lookups.Load() != before {
			t.Fatal("pre-cancelled work accessed source", err)
		}
	})
	t.Run("AC03", func(t *testing.T) {
		f := newSourceFixture(t, nil)
		source := f.create(t, "sales")
		x := readExecutor(t, f, nil)
		p := f.plan(t, source, `SELECT amount,9007199254740993::bigint AS big,cash,active,NULL::text AS absent,document,payload FROM analytics.sales ORDER BY id`)
		r := executeFixture(t, x, f.e, p, readOptions("exact-values"))
		if r.Result == nil || r.Attempt.Status != "succeeded" {
			t.Fatal("native exact result", r.Attempt)
		}
		want := []string{`"9007199254740993.125"`, `"9007199254740993"`, `"12.25"`, `true`, `null`, `"{\"a\": 1}"`, `"cafe"`}
		for i, value := range want {
			if string(r.Result.Rows[0][i]) != value {
				t.Fatalf("column %d lost exact value/type: %s", i, r.Result.Rows[0][i])
			}
		}
		encoded, err := json.Marshal(r)
		if err != nil || !strings.Contains(string(encoded), `"9007199254740993"`) {
			t.Fatal("transport rounded exact integer", err)
		}
		if r.Result.Schema[0].Encoding != "string" || r.Result.Schema[3].Encoding != "boolean" || r.Result.Cost.ScannedBytes != nil || r.Result.Cost.PlannerUnits == nil {
			t.Fatal("type/cost evidence invented")
		}
		if _, err = f.admin.Exec(context.Background(), `UPDATE analytics.sales SET amount='NaN' WHERE id=2`); err != nil {
			t.Fatal(err)
		}
		nan := f.plan(t, source, `SELECT amount FROM analytics.sales ORDER BY id`)
		bad := executeFixture(t, x, f.e, nan, readOptions("nonfinite"))
		if bad.Result != nil || bad.Attempt.Code != "result_type_unsupported" {
			t.Fatal("nonfinite was coerced or partial values escaped", bad.Attempt)
		}
		array := f.plan(t, source, `SELECT ARRAY[id] AS unsupported FROM analytics.sales`)
		bad = executeFixture(t, x, f.e, array, readOptions("unqualified-output"))
		if bad.Result != nil || bad.Attempt.Code != "result_type_unsupported" {
			t.Fatal("unqualified result type silently converted", bad.Attempt)
		}
	})
	t.Run("AC04", func(t *testing.T) {
		f := newSourceFixture(t, nil)
		ctx := context.Background()
		if _, err := f.admin.Exec(ctx, `INSERT INTO analytics.items SELECT i,i FROM generate_series(3,10005) i`); err != nil {
			t.Fatal(err)
		}
		source := f.create(t, "sales")
		x := readExecutor(t, f, nil)
		p := f.plan(t, source, `WITH q AS (SELECT sale_id,quantity FROM analytics.items) SELECT sale_id FROM q ORDER BY quantity DESC,sale_id DESC`)
		r := executeFixture(t, x, f.e, p, readOptions("default-cap"))
		if r.Result == nil || r.Result.Outcome != "truncated" || r.Result.Truncation != "rows" || len(r.Result.Rows) != 10000 || string(r.Result.Rows[0][0]) != `"10005"` {
			t.Fatal("default cap/order changed", r.Attempt)
		}
		o := readOptions("preview-cap")
		o.Preview = true
		r = executeFixture(t, x, f.e, p, o)
		if r.Result == nil || len(r.Result.Rows) != 200 || r.Attempt.Manifest.Limits.Rows != 200 {
			t.Fatal("preview cap ignored")
		}
		o = readOptions("ceiling")
		o.Rows = 100001
		before := f.lookups.Load()
		if _, err := x.Execute(ctx, f.e, p, o); !errors.Is(err, readexec.ErrLimit) || f.lookups.Load() != before {
			t.Fatal("ceiling not checked before work", err)
		}
		if _, err := f.admin.Exec(ctx, `UPDATE analytics.sales SET name=repeat('x',600)`); err != nil {
			t.Fatal(err)
		}
		p = f.plan(t, source, `SELECT name FROM analytics.sales ORDER BY id`)
		o = readOptions("byte-cap")
		o.Bytes = 1024
		r = executeFixture(t, x, f.e, p, o)
		if r.Result == nil || r.Result.Truncation != "bytes" || len(r.Result.Rows) != 1 || r.Result.Bytes > 1024 {
			t.Fatal("byte cap disguised", r.Attempt)
		}
		schema, _ := json.Marshal(r.Result.Schema)
		rows, _ := json.Marshal(r.Result.Rows)
		if r.Result.Bytes != len(schema)+len(rows) {
			t.Fatal("bytes do not describe actual serialized data")
		}
		// Both HTTP and future frozen/job callers use the same plan-only core: no
		// invocation-mode flag can raise ceilings or bypass this admission path.
		limited := readExecutor(t, f, func(c *config.ReadValidation) { c.PlannerCostCeiling = 0.0001 })
		r = executeFixture(t, limited, f.e, p, readOptions("native-cost"))
		if r.Result != nil || r.Attempt.Code != "limit_exceeded" || r.Attempt.Remote != nil {
			t.Fatal("optimizer cost gate ran result work", r.Attempt)
		}
	})
	t.Run("AC05", func(t *testing.T) {
		f := newSourceFixture(t, nil)
		source := f.create(t, "sales")
		x := readExecutor(t, f, nil)
		ctx := context.Background()
		p := f.plan(t, source, `SELECT id FROM analytics.sales`)
		before := f.lookups.Load()
		if _, err := x.Execute(ctx, f.e, readexec.Plan{}, readOptions("zero-plan")); !errors.Is(err, readexec.ErrBinding) {
			t.Fatal("zero plan reached executor")
		}
		contextOnly := f.token.envelope(t, "source-a", "operator", "cw.execution_context.use:*")
		if _, err := x.Execute(ctx, contextOnly, p, readOptions("context-only")); err == nil {
			t.Fatal("context label was authority")
		}
		foreign := f.actor(t, "source-b", "operator")
		if _, err := x.Execute(ctx, foreign, p, readOptions("foreign")); err == nil {
			t.Fatal("foreign tenant reused plan")
		}
		if f.lookups.Load() != before {
			t.Fatal("denied plan did warehouse work")
		}
		r := executeFixture(t, x, f.e, p, readOptions("owned-attempt"))
		if _, err := x.Inspect(ctx, foreign, r.Attempt.ID); err == nil {
			t.Fatal("foreign attempt visible")
		}
		other := f.actor(t, "source-a", "other")
		if _, err := x.Control(ctx, other, r.Attempt.ID, true); err == nil {
			t.Fatal("another actor controlled remote query")
		}
		if _, err := f.s.Rotate(ctx, f.e, source.ID, source.Revision); err != nil {
			t.Fatal(err)
		}
		before = f.lookups.Load()
		stale := executeFixture(t, x, f.e, p, readOptions("stale-context"))
		if stale.Result != nil || stale.Attempt.Code != "context_changed" || f.lookups.Load() != before {
			t.Fatal("rotation did not fence old plan", stale.Attempt)
		}
	})
	t.Run("AC06", func(t *testing.T) {
		f := newSourceFixture(t, nil)
		source := f.create(t, "sales")
		x := readExecutor(t, f, nil)
		ctx := context.Background()
		p := f.plan(t, source, `SELECT 100/amount AS value FROM analytics.sales ORDER BY id`)
		if _, err := f.admin.Exec(ctx, `UPDATE analytics.sales SET amount=0 WHERE id=2`); err != nil {
			t.Fatal(err)
		}
		before := f.lookups.Load()
		o := readOptions("explicit-retry")
		first := executeFixture(t, x, f.e, p, o)
		if first.Result != nil || first.Attempt.Status != "failed" || first.Attempt.RemoteState != "stopped" || f.lookups.Load() != before+1 {
			t.Fatal("failure was retried/coerced", first.Attempt)
		}
		if _, err := x.Execute(ctx, f.e, p, o); !errors.Is(err, readexec.ErrReplay) {
			t.Fatal("same attempt ran twice", err)
		}
		if _, err := f.admin.Exec(ctx, `UPDATE analytics.sales SET amount=1 WHERE id=2`); err != nil {
			t.Fatal(err)
		}
		o.Number = 2
		second := executeFixture(t, x, f.e, p, o)
		if second.Result == nil || second.Attempt.ID == first.Attempt.ID || second.Attempt.Number != 2 || second.Attempt.Manifest.Operation != first.Attempt.Manifest.Operation {
			t.Fatal("logical/physical attempt identity lost")
		}
		changed := f.plan(t, source, `SELECT id FROM analytics.sales`)
		o.Number = 3
		if _, err := x.Execute(ctx, f.e, changed, o); !errors.Is(err, store.ErrConflict) {
			t.Fatal("changed request reused logical operation", err)
		}
		empty := f.plan(t, source, `SELECT id FROM analytics.sales WHERE id<0`)
		r := executeFixture(t, x, f.e, empty, readOptions("valid-empty"))
		if r.Result == nil || r.Result.Outcome != "empty" || len(r.Result.Rows) != 0 || len(r.Result.Schema) != 1 {
			t.Fatal("empty result widened or lost schema")
		}
		var wg sync.WaitGroup
		before = f.lookups.Load()
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				r, err := x.Execute(ctx, f.e, changed, readOptions(fmt.Sprintf("concurrent-%d", i)))
				if err != nil || r.Result == nil || len(r.Result.Rows) != 2 {
					t.Errorf("concurrent result contamination: %v %+v", err, r.Attempt)
				}
			}(i)
		}
		wg.Wait()
		if f.lookups.Load() != before+12 {
			t.Fatal("hidden native retry")
		}
		meta := support.Raw(t, f.dsn)
		var attempts int
		if err := meta.QueryRow(ctx, `SELECT count(*) FROM chartworks.read_attempts WHERE operation_id='explicit-retry'`).Scan(&attempts); err != nil || attempts != 2 {
			t.Fatal("attempt ledger mismatch", err, attempts)
		}
		if f.writeLookups.Load() != 0 {
			t.Fatal("write credential used")
		}
	})
}
