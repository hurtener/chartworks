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
	if err != nil || SchemaVersion() != "12" || len(manifest) != 12 {
		t.Fatal("schema version", err, SchemaVersion(), len(manifest))
	}
	latest := manifest[len(manifest)-1]
	if latest.Version != 12 || latest.Name != "migrations/012_topic_drafts.sql" || len(latest.Checksum) != 64 {
		t.Fatal("latest embedded migration identity", latest.Version, latest.Name, latest.Checksum)
	}
	for _, dialect := range []string{"postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		if !strings.Contains(manifest[9].SQL, "'"+dialect+"'") {
			t.Fatal("warehouse dialect migration missing closed variant", dialect)
		}
	}
}
