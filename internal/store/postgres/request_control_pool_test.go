package postgres

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestRequestControlPoolBoundsAndClose(t *testing.T) {
	for _, maximum := range []int32{1, 100} {
		t.Run(map[int32]string{1: "minimum", 100: "maximum"}[maximum], func(t *testing.T) {
			u, err := url.Parse(planLockTestDatabaseURL(t))
			if err != nil {
				t.Fatal(err)
			}
			q := u.Query()
			q.Set("pool_max_conns", "200")
			q.Set("pool_min_conns", "150")
			q.Set("pool_min_idle_conns", "150")
			u.RawQuery = q.Encode()
			opts := Defaults()
			opts.MaxConns = maximum
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			db, err := Open(ctx, u.String(), opts)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			ordinary, control := db.pool.Config(), db.requestControlPool.Config()
			if ordinary.MaxConns != maximum || control.MaxConns != 1 || ordinary.MinConns != 0 || ordinary.MinIdleConns != 0 || control.MinConns != 0 || control.MinIdleConns != 0 || db.ReadCapacity() != int(maximum) {
				t.Fatal("DSN widened bounded ordinary/control capacity")
			}
			if ordinary.ConnConfig.RuntimeParams["search_path"] != "pg_catalog" || control.ConnConfig.RuntimeParams["search_path"] != "pg_catalog" {
				t.Fatal("control lost trusted session settings")
			}
			if ordinary.ConnConfig.RuntimeParams["application_name"] != "chartworks" || control.ConnConfig.RuntimeParams["application_name"] != "chartworks-request-control" {
				t.Fatal("control configuration mutated ordinary connection settings")
			}
			db.Close()
			db.Close()
			if db.pool.Ping(ctx) == nil || db.requestControlPool.Ping(ctx) == nil {
				t.Fatal("Close left a pool usable")
			}
			if err := db.requestControlTransaction(ctx, func(context.Context, pgx.Tx) error { t.Error("closed control callback entered"); return nil }); !errors.Is(err, store.ErrUnavailable) {
				t.Fatalf("closed control did not fail closed: %v", err)
			}
		})
	}
}

func TestRequestControlPoolOpenFailureClosesBothPools(t *testing.T) {
	dsn := planLockTestDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	opts := Defaults()
	opts.MaxConns = 1
	opts.MigrationPolicy = "check" // Empty database is an actual migration-check failure.
	db, err := Open(ctx, dsn, opts)
	if db != nil || !errors.Is(err, store.ErrMigration) {
		t.Fatalf("unexpected open result: db=%v err=%v", db != nil, err)
	}
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	// Observe server-side session removal, bounded by context, rather than assume
	// that client-side Close or elapsed time proves both sessions were released.
	for {
		var count int
		err := admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND application_name IN ('chartworks','chartworks-request-control')`).Scan(&count)
		if err != nil {
			t.Fatal("failed Open left a database session", err)
		}
		if count == 0 {
			break
		}
	}
}

func TestRequestControlCloseJoinsBoundedTransactions(t *testing.T) {
	db := requestControlTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entered, done, closed := make(chan struct{}), make(chan error, 1), make(chan struct{})
	go func() {
		done <- db.requestControlTransaction(ctx, func(txCtx context.Context, _ pgx.Tx) error { close(entered); <-txCtx.Done(); return txCtx.Err() })
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("control transaction did not enter")
	}
	go func() { db.Close(); close(closed) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("active transaction lost bound: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("active control transaction did not stop")
	}
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("Close did not join bounded control transaction")
	}
	if db.requestControlPool.Stat().AcquiredConns() != 0 || db.pool.Stat().AcquiredConns() != 0 {
		t.Fatal("Close leaked acquired connections")
	}
}
