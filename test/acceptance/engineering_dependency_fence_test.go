package acceptance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestProfileDependencyRegistrationFencesSourceDeletion(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, columns := engineeringCSV()
	loaded := f.load(t, engineeringSpec("dependency-source-fence", "csv", raw, columns), raw)
	spec := f.profileSpec(t, *loaded.Upload.Source, "dependency-fence-profile", []string{"id"}, "")
	f.profile(t, spec)
	dep := engineering.Dependency{Kind: "report", ID: "fenced-dependent-report", Version: readexec.Hash("approved-fence-fixture"), Source: spec.Source, Context: spec.Context, Dataset: spec.Dataset, Columns: []string{"id"}}
	metadata := support.Raw(t, f.dsn)
	head, err := metadata.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = head.Rollback(context.Background()) }()
	key := readexec.Hash([]string{f.e.Tenant(), f.e.User(), f.e.Session(), spec.Source, spec.Dataset})
	if _, err = head.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061210))`, key); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- f.service.RegisterDependency(ctx, f.e, spec.ID, dep) }()
	joined := false
	defer func() {
		cancel()
		_ = head.Rollback(context.Background())
		if !joined {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("dependency registration failed to join")
			}
		}
	}()
	observer := support.Raw(t, f.dsn)
	waiting := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err = observer.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_stat_activity WHERE datname=current_database() AND application_name='chartworks' AND wait_event_type='Lock' AND position('7214061210' in query)>0)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err = <-done:
			joined = true
			t.Fatal("registration did not wait for the held head lock", err)
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if !waiting {
		t.Fatal("did not observe registration waiting after source proof")
	}
	// A source tombstone is an UPDATE. While registration is blocked on the
	// head, it must already hold the source's row lock, not just an advisory lock.
	probe, err := observer.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, lockErr := probe.Exec(ctx, `SELECT source_id FROM chartworks.sources WHERE tenant_id=$1 AND source_id=$2 FOR UPDATE NOWAIT`, f.e.Tenant(), spec.Source)
	_ = probe.Rollback(context.Background())
	var pg *pgconn.PgError
	if !errors.As(lockErr, &pg) || pg.Code != "55P03" {
		t.Fatal("source deletion can overtake dependency insertion", lockErr)
	}
	if err = head.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		joined = true
		if err != nil {
			t.Fatal("valid registration did not complete", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	erased, err := f.service.EraseUpload(ctx, f.e, spec.Source, "erase-fenced-dependency", false)
	if err != nil || erased.Upload.State != "erased" {
		t.Fatal("registration leaked its source lock or blocked erasure", err, erased)
	}
	if _, err = f.service.Health(ctx, f.e, dep); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("erased dependency remained visible", err)
	}
	if err = f.service.RegisterDependency(ctx, f.e, spec.ID, dep); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("registration recreated a reference to an erased source", err)
	}
	if count(t, observer, `SELECT count(*) FROM chartworks.profile_dependencies`) != 0 {
		t.Fatal("a derived dependency survived source erasure")
	}
}
