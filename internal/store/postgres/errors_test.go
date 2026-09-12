package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSafeErrors(t *testing.T) {
	for _, tc := range []struct{ in, want error }{{nlqbyo.ErrBudget, nlqbyo.ErrBudget}, {nil, nil}, {context.Canceled, context.Canceled}, {context.DeadlineExceeded, context.DeadlineExceeded}, {pgx.ErrNoRows, store.ErrNotFound}, {store.ErrExpired, store.ErrExpired}, {errors.New("credential-canary"), store.ErrUnavailable}} {
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
	if err != nil || SchemaVersion() != "30" || len(manifest) != 30 {
		t.Fatal("schema version", err, SchemaVersion(), len(manifest))
	}
	reviewedSchedule := manifest[27]
	if reviewedSchedule.Version != 28 || reviewedSchedule.Name != "migrations/028_reviewed_schedule_changes.sql" || len(reviewedSchedule.Checksum) != 64 || !strings.Contains(reviewedSchedule.SQL, "schedule_change_receipt_shape") || !strings.Contains(reviewedSchedule.SQL, "change_revision") {
		t.Fatal("preserved reviewed-schedule migration identity", reviewedSchedule.Version, reviewedSchedule.Name, reviewedSchedule.Checksum)
	}
	for _, m := range []struct {
		index  int
		name   string
		marker string
	}{
		{28, "migrations/029_reports_dashboards.sql", "document_revisions"},
		{29, "migrations/030_report_composition_runs.sql", "composition_group_guard"},
	} {
		added := manifest[m.index]
		if added.Version != m.index+1 || added.Name != m.name || len(added.Checksum) != 64 || !strings.Contains(added.SQL, m.marker) {
			t.Fatal("reporting migration identity", added.Version, added.Name, added.Checksum)
		}
	}
	for _, dialect := range []string{"postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		if !strings.Contains(manifest[9].SQL, "'"+dialect+"'") {
			t.Fatal("warehouse dialect migration missing closed variant", dialect)
		}
	}
}
