package acceptance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestManagedUploadReadGrantAndPublicationFence(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	raw, columns := engineeringCSV()
	spec := engineeringSpec("publication-fence", "csv", raw, columns)
	loaded := f.load(t, spec, raw)
	f.readUpload(t, *loaded.Upload.Source)
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	schema, table, err := sources.ManagedLocation(f.cfg.Connections[1], spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	qualified := pgx.Identifier{schema, table}.Sanitize()
	var oid int64
	var selectable, writable, registryReadable bool
	if err = f.admin.QueryRow(ctx, `SELECT c.oid::bigint,has_table_privilege($1,c.oid,'SELECT'),has_table_privilege($1,c.oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER') OR has_any_column_privilege($1,c.oid,'INSERT,UPDATE,REFERENCES') FROM pg_catalog.pg_class c WHERE c.oid=to_regclass($2)`, f.role, qualified).Scan(&oid, &selectable, &writable); err != nil || !selectable || writable {
		t.Fatal("managed reader grant does not match the common adapter", err, selectable, writable)
	}
	if err = f.admin.QueryRow(ctx, `SELECT has_table_privilege($1,to_regclass($2),'SELECT')`, f.role, pgx.Identifier{schema, "_uploads"}.Sanitize()).Scan(&registryReadable); err != nil || registryReadable {
		t.Fatal("managed reader can read private staging bytes", err)
	}
	names := make([]string, len(columns))
	for i, column := range columns {
		names[i] = column.Name
	}
	called := false
	callback := func(context.Context, sources.Record) error { called = true; return nil }
	if err = f.s.WithManagedRecord(ctx, f.e, spec.ID, spec.Name, spec.Connection, names, oid+1, callback); !errors.Is(err, readexec.ErrBinding) || called {
		t.Fatal("publication adopted a table other than the loaded receipt", err, called)
	}
	before := f.lookups.Load()
	if err = f.s.WithManagedRecord(ctx, identity.Envelope{}, spec.ID, spec.Name, spec.Connection, names, oid, callback); err == nil || called || f.lookups.Load() != before {
		t.Fatal("unverified publication reached source credentials", err, called)
	}
	err = f.s.WithManagedRecord(ctx, f.e, spec.ID, spec.Name, spec.Connection, names, oid, func(ctx context.Context, record sources.Record) error {
		called = true
		if !record.Valid() || record.Source.ID != spec.ID {
			t.Error("publication supplied an invalid or substituted record")
		}
		other, beginErr := f.admin.Begin(ctx)
		if beginErr != nil {
			return beginErr
		}
		defer func() { _ = other.Rollback(context.Background()) }()
		_, lockErr := other.Exec(ctx, "LOCK TABLE "+qualified+" IN ACCESS EXCLUSIVE MODE NOWAIT")
		var pg *pgconn.PgError
		if !errors.As(lockErr, &pg) || pg.Code != "55P03" {
			t.Errorf("publication did not retain the table lock: %v", lockErr)
		}
		return nil
	})
	if err != nil || !called {
		t.Fatal("valid publication callback failed", err, called)
	}
	other, err := f.admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Rollback(context.Background()) }()
	if _, err = other.Exec(ctx, "LOCK TABLE "+qualified+" IN ACCESS EXCLUSIVE MODE NOWAIT"); err != nil {
		t.Fatal("publication leaked its table lock", err)
	}
}

func TestManagedUploadErasureRejectsReplacementWhileWaitingForLock(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	raw, columns := engineeringCSV()
	spec := engineeringSpec("replacement-race", "csv", raw, columns)
	f.load(t, spec, raw)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	schema, table, err := sources.ManagedLocation(f.cfg.Connections[1], spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	qualified := pgx.Identifier{schema, table}.Sanitize()
	preserved := table + "_preserved"
	observer := support.Raw(t, f.warehouse)
	tx, err := f.admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, "LOCK TABLE "+qualified+" IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		run engineering.UploadRun
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		run, err := f.service.EraseUpload(ctx, f.e, spec.ID, "replacement-erase", false)
		done <- outcome{run: run, err: err}
	}()
	joined := false
	defer func() {
		cancel()
		_ = tx.Rollback(context.Background())
		if !joined {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("erasure did not join after cancellation")
			}
		}
	}()
	// Observe the real writer waiting after its first ownership probe; elapsed
	// sleep alone is not evidence that the race window has been reached.
	deadline := time.Now().Add(3 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		if err = observer.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_stat_activity WHERE datname=current_database() AND application_name='chartworks-managed-writer' AND wait_event_type='Lock' AND position($1 in query)>0)`, "LOCK TABLE "+qualified).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case result := <-done:
			joined = true
			t.Fatal("erasure completed before the held object lock", result.err, result.run.Code)
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if !waiting {
		t.Fatal("the real eraser never reached the ownership lock")
	}
	if _, err = tx.Exec(ctx, "ALTER TABLE "+qualified+" RENAME TO "+pgx.Identifier{preserved}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE "+qualified+" (note text NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "INSERT INTO "+qualified+" VALUES($1)", "UNRELATED_REPLACEMENT_CANARY"); err != nil {
		t.Fatal(err)
	}
	// Match the writer owner deliberately: OID identity, not merely ownership
	// or the generated name, must be what prevents this replacement's deletion.
	if _, err = tx.Exec(ctx, "ALTER TABLE "+qualified+" OWNER TO "+pgx.Identifier{f.writer}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		joined = true
		if result.err != nil || result.run.Upload.State == "erased" || result.run.Code != "workspace_ownership_unproven" {
			t.Fatal("object substitution was not rejected truthfully", result.err, result.run)
		}
	case <-ctx.Done():
		t.Fatal("erasure did not observe the replacement", ctx.Err())
	}
	var note string
	if err = observer.QueryRow(ctx, "SELECT note FROM "+qualified).Scan(&note); err != nil || note != "UNRELATED_REPLACEMENT_CANARY" {
		t.Fatal("unrelated replacement was deleted or changed", err)
	}
	var rows int
	if err = observer.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{schema, preserved}.Sanitize()).Scan(&rows); err != nil || rows != 2 {
		t.Fatal("original renamed table was deleted or changed", err, rows)
	}
}
