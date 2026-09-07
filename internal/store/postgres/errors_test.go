package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSafeErrors(t *testing.T) {
	for _, tc := range []struct{ in, want error }{{nil, nil}, {context.Canceled, context.Canceled}, {context.DeadlineExceeded, context.DeadlineExceeded}, {pgx.ErrNoRows, store.ErrNotFound}, {store.ErrExpired, store.ErrExpired}, {errors.New("credential-canary"), store.ErrUnavailable}} {
		if !errors.Is(safe(tc.in), tc.want) {
			t.Errorf("wrong safe category")
		}
	}
	for _, code := range []string{"23505", "40001", "40P01", "23503", "23502", "23514", "22003", "55000", "57014"} {
		e := safe(&pgconn.PgError{Code: code, Message: "credential-canary"})
		if e == nil || e.Error() == "credential-canary" {
			t.Fatal("unsafe database error")
		}
	}
	manifest, err := Migrations()
	if err != nil || SchemaVersion() != "9" || len(manifest) != 9 {
		t.Fatal("schema version", err, SchemaVersion(), len(manifest))
	}
	latest := manifest[len(manifest)-1]
	if latest.Version != 9 || latest.Name != "migrations/009_pipelines.sql" || len(latest.Checksum) != 64 {
		t.Fatal("latest embedded migration identity", latest.Version, latest.Name, latest.Checksum)
	}
	for _, relation := range []string{"pipeline_definitions", "pipeline_runs", "pipeline_stages", "pipeline_outputs"} {
		if !strings.Contains(latest.SQL, "chartworks."+relation) {
			t.Fatal("pipeline migration missing required relation", relation)
		}
	}
}
