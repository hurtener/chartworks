// Package support contains real, disposable PostgreSQL fixtures, never an in-memory store.
package support

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/jackc/pgx/v5"
)

// Database requires an explicit test server and creates a uniquely named disposable database.
func Database(t testing.TB) string {
	t.Helper()
	base := os.Getenv("CHARTWORKS_TEST_STORE_URL")
	if base == "" {
		t.Fatal("CHARTWORKS_TEST_STORE_URL required; real database tests never skip")
	}
	u, e := url.Parse(base)
	if e != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatal("test store URI invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, e := pgx.Connect(ctx, base)
	if e != nil {
		t.Fatal("cannot connect to explicit test database")
	}
	var b [10]byte
	if _, e = rand.Read(b[:]); e != nil {
		t.Fatal("random fixture ID failed")
	}
	name := "cw_test_" + hex.EncodeToString(b[:])
	if _, e = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); e != nil {
		_ = admin.Close(ctx)
		t.Fatal("cannot create isolated test database")
	}
	u.Path = "/" + name
	dsn := u.String()
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 15*time.Second)
		defer c()
		_, err := admin.Exec(cleanup, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		if err != nil {
			t.Error("test database cleanup failed")
		}
		_ = admin.Close(cleanup)
	})
	return dsn
}

// Open constructs the actual production driver and arranges its bounded cleanup.
func Open(t testing.TB, dsn string) *postgres.DB {
	t.Helper()
	db, e := postgres.Open(context.Background(), dsn, postgres.Defaults())
	if e != nil {
		t.Fatalf("store open: %v", e)
	}
	t.Cleanup(db.Close)
	return db
}

// Raw is a fixture-only administrative connection. Production repositories expose no raw SQL port.
func Raw(t testing.TB, dsn string) *pgx.Conn {
	t.Helper()
	c, e := pgx.Connect(context.Background(), dsn)
	if e != nil {
		t.Fatal("raw fixture connection failed")
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

// Scope identifies synthetic test tenants; it does not masquerade as a verified JWT.
func Scope(t testing.TB, tenant, actor string) store.Scope {
	t.Helper()
	s, e := store.NewScope(tenant, actor)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
