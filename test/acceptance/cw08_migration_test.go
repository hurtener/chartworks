package acceptance

import (
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

// TestCW08PopulatedExampleUpgrade proves migration 042 can normalize both
// active and candidate rows while migration 017's old active-to-candidate
// immutability trigger is installed. Migration 041 is applied and retained
// unchanged before this forward upgrade.
func TestCW08PopulatedExampleUpgrade(t *testing.T) {
	ctx := t.Context()
	dsn := support.Database(t)
	raw := upgradeFixture(t, dsn)
	manifest, err := postgres.Migrations()
	if err != nil || len(manifest) != 44 || manifest[40].Name != "migrations/041_reporting_output_locale_bounds.sql" || manifest[41].Name != "migrations/042_learning_templates.sql" || manifest[42].Name != "migrations/043_reporting_display_intent.sql" || manifest[43].Name != "migrations/044_reporting_question_assessments.sql" {
		t.Fatal("CW-08 migration inventory", err)
	}
	for _, migration := range manifest[1:41] {
		sql(t, raw, migration.SQL)
		sql(t, raw, `INSERT INTO chartworks.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, migration.Version, migration.Name, migration.Checksum)
	}
	sql(t, raw, `INSERT INTO chartworks.nlq_examples(tenant_id,example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,provenance)
 VALUES('cw08-upgrade',repeat('1',32),'topic','Active question','SELECT 1',repeat('a',64),'active',0.8,4,'synthetic-active'),
       ('cw08-upgrade',repeat('2',32),'topic','Candidate question','SELECT 2',repeat('b',64),'candidate',0.5,2,'synthetic-candidate')`)
	db := support.Open(t, dsn)
	if err = db.Check(ctx); err != nil {
		t.Fatal("apply migration 042", err)
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.schema_migrations WHERE version=41 AND name='migrations/041_reporting_output_locale_bounds.sql'`) != 1 || count(t, raw, `SELECT count(*) FROM chartworks.schema_migrations WHERE version=42 AND name='migrations/042_learning_templates.sql'`) != 1 || count(t, raw, `SELECT count(*) FROM chartworks.schema_migrations WHERE version=43 AND name='migrations/043_reporting_display_intent.sql'`) != 1 || count(t, raw, `SELECT count(*) FROM chartworks.schema_migrations WHERE version=44 AND name='migrations/044_reporting_question_assessments.sql'`) != 1 {
		t.Fatal("migration identities changed")
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.nlq_examples WHERE tenant_id='cw08-upgrade' AND state='candidate' AND positive_evidence=evidence_count AND negative_evidence=0 AND origin->>'topic_version'='legacy'`) != 2 {
		t.Fatal("populated examples were not safely normalized")
	}
	if _, err = raw.Exec(ctx, `UPDATE chartworks.nlq_examples SET state='active',reviewed_by='reviewer',review_note='reviewed',reviewed_at=clock_timestamp(),version=version+1 WHERE tenant_id='cw08-upgrade' AND example_id=repeat('1',32)`); err != nil {
		t.Fatal("final lifecycle guard rejected reviewed activation", err)
	}
	if _, err = raw.Exec(ctx, `UPDATE chartworks.nlq_examples SET state='candidate',version=version+1 WHERE tenant_id='cw08-upgrade' AND example_id=repeat('1',32)`); err == nil {
		t.Fatal("final lifecycle guard allowed active rollback to candidate")
	}
	// A negative-feedback transaction that wins after the reviewer read must
	// invalidate both the observed version and the atomic activation predicate.
	if _, err = raw.Exec(ctx, `UPDATE chartworks.nlq_examples SET negative_evidence=positive_evidence,evidence_count=positive_evidence*2,weight=0.5,uncertainty=1/sqrt((positive_evidence*2+2)::double precision),version=version+1 WHERE tenant_id='cw08-upgrade' AND example_id=repeat('2',32)`); err != nil {
		t.Fatal("commit negative evidence interleaving", err)
	}
	scope, err := store.NewScope("cw08-upgrade", "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []int64{1, 0} {
		if _, err = db.SetExampleState(ctx, scope, nlqexec.ExampleStateRequest{ExampleID: "22222222222222222222222222222222", State: "active", ExpectedVersion: expected, ReviewNote: "stale review"}, "reviewer"); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("ineligible activation expected_version=%d returned %v", expected, err)
		}
	}
}
