package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func planLockTestDatabaseURL(t *testing.T) string {
	t.Helper()
	base := os.Getenv("CHARTWORKS_TEST_STORE_URL")
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatal("real PostgreSQL test store is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal("cannot connect to PostgreSQL test store")
	}
	var random [10]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	name := "cw_lock_" + hex.EncodeToString(random[:])
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal("cannot create lock test database", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error("lock test database cleanup failed", err)
		}
		_ = admin.Close(cleanup)
	})
	u.Path = "/" + name
	return u.String()
}

func planLockTestDB(t *testing.T) *DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	opts := Defaults()
	opts.MaxConns = 2
	db, err := Open(ctx, planLockTestDatabaseURL(t), opts)
	if err != nil {
		t.Fatal("cannot open two-connection store", err)
	}
	t.Cleanup(db.Close)
	return db
}

func TestPlanOperationLockKeepsPoolCapacityForDistinctKeys(t *testing.T) {
	db := planLockTestDB(t)
	scope, err := store.NewScope("tenant", "actor")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	read := func() error {
		_, err := db.ReadOperation(ctx, scope, "absent-operation")
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	results := make(chan error, 2)
	go func() {
		results <- db.WithPlanOperationLock(ctx, scope, "first-operation", func() error {
			close(entered)
			<-release
			return read()
		})
	}()
	select {
	case <-entered:
	case err := <-results:
		t.Fatalf("first operation failed before its callback: %v", err)
	case <-ctx.Done():
		t.Fatal("first operation did not acquire its lock")
	}
	started := make(chan struct{})
	go func() {
		close(started)
		results <- db.WithPlanOperationLock(ctx, scope, "second-operation", read)
	}()
	<-started
	// The old implementation allowed both distinct lock keys to consume the
	// entire pool; neither callback could then enter ReadOperation.
	time.Sleep(75 * time.Millisecond)
	held := db.pool.Stat().AcquiredConns()
	close(release)
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("distinct operation could not read through the pool: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("distinct operation locks exhausted the pool")
		}
	}
	if held != 1 || db.pool.Stat().AcquiredConns() != 0 {
		t.Fatalf("advisory locks consumed callback capacity or leaked connections: held=%d remaining=%d", held, db.pool.Stat().AcquiredConns())
	}
}

func TestPlanOperationLockCancellationReleasesWaiterAndSession(t *testing.T) {
	db := planLockTestDB(t)
	scope, err := store.NewScope("tenant", "actor")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- db.WithPlanOperationLock(ctx, scope, "shared-operation", func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case <-entered:
	case err := <-firstDone:
		t.Fatalf("first lock failed before its callback: %v", err)
	case <-ctx.Done():
		t.Fatal("first lock was not acquired")
	}
	waitCtx, cancelWait := context.WithCancel(ctx)
	var called atomic.Bool
	waitDone := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		waitDone <- db.WithPlanOperationLock(waitCtx, scope, "shared-operation", func() error {
			called.Store(true)
			return nil
		})
	}()
	<-started
	time.Sleep(75 * time.Millisecond)
	cancelWait()
	select {
	case err := <-waitDone:
		if !errors.Is(err, context.Canceled) || called.Load() {
			t.Fatalf("canceled waiter ran the callback: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("canceled waiter did not return")
	}
	close(release)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal("holder failed to release its lock", err)
		}
	case <-ctx.Done():
		t.Fatal("holder did not release its lock")
	}
	if err := db.WithPlanOperationLock(ctx, scope, "shared-operation", func() error { return nil }); err != nil {
		t.Fatal("new request could not reacquire released lock", err)
	}
	if db.pool.Stat().AcquiredConns() != 0 {
		t.Fatal("canceled lock path leaked a pool connection")
	}
}

func TestPlanOperationUnlockErrorClosesLockedSession(t *testing.T) {
	testURL := planLockTestDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, testURL)
	if err != nil {
		t.Fatal("cannot connect to lock test database", err)
	}
	// The shadow function fails only the unlock statement. PostgreSQL keeps
	// this session healthy after the server error and leaves its advisory lock
	// held; a pool Release would therefore strand the lock on an idle session.
	_, err = admin.Exec(ctx, `CREATE FUNCTION public.pg_advisory_unlock(bigint) RETURNS boolean LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected unlock failure'; END $$`)
	_ = admin.Close(ctx)
	if err != nil {
		t.Fatal("cannot install test-only unlock failure", err)
	}
	cfg, err := pgxpool.ParseConfig(testURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 2
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = "public,pg_catalog"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := &DB{pool: pool, timeout: 5 * time.Second, planOperationSlots: make(chan struct{}, 1)}
	t.Cleanup(db.Close)
	scope, err := store.NewScope("tenant", "actor")
	if err != nil {
		t.Fatal(err)
	}
	called := false
	err = db.WithPlanOperationLock(ctx, scope, "unlock-failure", func() error {
		called = true
		return nil
	})
	if !called || !errors.Is(err, store.ErrUnavailable) {
		t.Fatalf("unlock server error did not fail closed after callback: called=%t err=%v", called, err)
	}
	if db.pool.Stat().AcquiredConns() != 0 {
		t.Fatal("failed unlock left an acquired pool connection")
	}
	// Use an independent session: the same pooled PostgreSQL session could
	// reenter its own leaked advisory lock and conceal the failure.
	probe, err := pgx.Connect(ctx, testURL)
	if err != nil {
		t.Fatal("cannot connect cross-session lock probe", err)
	}
	defer func() { _ = probe.Close(context.Background()) }()
	const lockKey = `["tenant","actor","unlock-failure"]`
	var locked bool
	if err := probe.QueryRow(ctx, `SELECT pg_catalog.pg_try_advisory_lock(pg_catalog.hashtextextended($1,7214061010))`, lockKey).Scan(&locked); err != nil || !locked {
		t.Fatalf("failed unlock stranded the advisory lock for another session: locked=%t err=%v", locked, err)
	}
	var unlocked bool
	if err := probe.QueryRow(ctx, `SELECT pg_catalog.pg_advisory_unlock(pg_catalog.hashtextextended($1,7214061010))`, lockKey).Scan(&unlocked); err != nil || !unlocked {
		t.Fatalf("cross-session probe could not release its lock: unlocked=%t err=%v", unlocked, err)
	}
}
