package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

func TestMissingRelationFailsReadiness(t *testing.T) {
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	raw := support.Raw(t, dsn)
	for _, relation := range []string{"operations", "topic_rule_comparison_evidence", "topic_rule_evidence_invalidations"} {
		missing := relation + "_missing"
		sql(t, raw, "ALTER TABLE chartworks."+pgx.Identifier{relation}.Sanitize()+" RENAME TO "+pgx.Identifier{missing}.Sanitize())
		if e := db.Check(context.Background()); !errors.Is(e, store.ErrMigration) {
			t.Fatal("history alone masked missing relation", relation)
		}
		if _, e := postgres.Open(context.Background(), dsn, postgres.Defaults()); !errors.Is(e, store.ErrMigration) {
			t.Fatal("startup accepted missing relation", relation)
		}
		sql(t, raw, "ALTER TABLE chartworks."+pgx.Identifier{missing}.Sanitize()+" RENAME TO "+pgx.Identifier{relation}.Sanitize())
		if e := db.Check(context.Background()); e != nil {
			t.Fatal("restored relation remained unready", relation, e)
		}
	}
	for _, dsn := range []string{"", "   "} {
		if _, e := postgres.Open(context.Background(), dsn, postgres.Defaults()); !errors.Is(e, store.ErrInvalid) {
			t.Fatal("ambient DSN accepted")
		}
	}
}
