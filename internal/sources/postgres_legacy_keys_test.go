package sources

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/jackc/pgx/v5"
)

// Metadata persistence is an in-memory fixture; catalog discovery, role checks,
// native EXPLAIN and opaque-plan execution use the real disposable PostgreSQL.
func TestSQLRecoveryLegacyPostgresKeyPolicyAcceptance(t *testing.T) {
	dsn := os.Getenv("CHARTWORKS_TEST_STORE_URL")
	if dsn == "" {
		t.Skip("CHARTWORKS_TEST_STORE_URL is not set")
	}
	ctx := t.Context()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	var nonce [8]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	name := "cw_legacy_" + hex.EncodeToString(nonce[:])
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE ROLE "+quoted+" LOGIN PASSWORD 'synthetic-legacy-reader' NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA IF EXISTS "+quoted+" CASCADE; DROP ROLE "+quoted); err != nil {
			t.Error("synthetic cleanup", err)
		}
	})
	for _, sql := range []string{
		"CREATE SCHEMA " + quoted,
		"CREATE TABLE " + quoted + ".sales(id integer PRIMARY KEY,amount numeric(12,2))",
		"INSERT INTO " + quoted + ".sales VALUES(1,3.25),(2,7.75)",
		"GRANT USAGE ON SCHEMA " + quoted + " TO " + quoted,
		"GRANT SELECT ON " + quoted + ".sales TO " + quoted,
	} {
		if _, err = admin.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	readURL, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	readURL.User = url.UserPassword(name, "synthetic-legacy-reader")
	settings := config.DefaultSources()
	settings.Enabled = true
	settings.Connections = []config.SourceConnection{{Dialect: "postgres", Tenant: "tenant", ID: "warehouse", Version: "v1", ReadDSN: "env:LEGACY_TEST_DSN", Relations: []config.SourceRelation{{Schema: name, Name: "sales", Columns: []string{"id", "amount"}}}}}
	repo := &cloudMemoryRepository{records: map[string]Record{}}
	service, err := New(repo, settings, func(key string) (string, bool) { return readURL.String(), key == "LEGACY_TEST_DSN" })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	dataset := "ds:" + readexec.Hash([]string{"source", name, "sales"})[:32]
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.write", "sources.read", "sources.query", "cw.tenant.write:tenant", "cw.source.read:source", "cw.source.write:source", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:" + dataset}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := service.Create(ctx, e, CreateRequest{ID: "source", Name: "Synthetic legacy source", Connection: "warehouse"})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := service.Binding(ctx, e, source.ID, source.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.Relations) != 1 || !fresh.Relations[0].HasUniqueKey([]string{"id"}) {
		t.Fatal("fresh source lost physical key")
	}
	legacy, err := service.probe(withStoredKeyPolicy(ctx, readexec.Binding{}), settings.Connections[0], source.ID, source.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.Relations[0].UniqueKeys) != 0 || legacy.Fingerprint == fresh.Fingerprint {
		t.Fatal("legacy source borrowed new metadata")
	}
	record := repo.records["tenant/source"]
	record.Binding = legacy
	repo.records["tenant/source"] = record
	validator, err := readexec.NewValidator(service, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	statement := "SELECT sum(amount) FROM " + name + ".sales"
	plan, err := validator.Validate(ctx, e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: statement})
	if err != nil {
		t.Fatal("legacy native validation", err)
	}
	result, err := service.ExecuteRead(ctx, e, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 5 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, "11111111111111111111111111111111", &cloudObserverCapture{})
	if err != nil {
		t.Fatal("legacy execution", err)
	}
	var total string
	if result.RemoteState != "stopped" || len(result.Result.Rows) != 1 || json.Unmarshal(result.Result.Rows[0][0], &total) != nil || total != "11.00" {
		t.Fatal("legacy exact result", total)
	}
	// A fresh key-bearing registration must never inherit the legacy downgrade.
	record.Binding = fresh
	repo.records["tenant/source"] = record
	if _, err = admin.Exec(ctx, "ALTER TABLE "+quoted+".sales DROP CONSTRAINT sales_pkey"); err != nil {
		t.Fatal(err)
	}
	if _, err = validator.Validate(ctx, e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: statement}); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("removed proved key accepted", err)
	}
}
