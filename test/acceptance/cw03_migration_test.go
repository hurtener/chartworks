package acceptance

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

// Install the real prior migration chain, populate an immutable v1 definition,
// then use the production migration runner. Domain publication and in-flight
// revision checks are separate real execution cases in cw03_reporting_test.go.
func TestCW03PopulatedLegacyDatabaseUpgrade(t *testing.T) {
	ctx := context.Background()
	fixture := newPhase29Execution(t, false)
	legacy := phase27Copy(t, fixture.base)
	dsn := support.Database(t)
	raw := support.Raw(t, dsn)
	migrations, err := postgres.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sql(t, raw, `CREATE SCHEMA chartworks; CREATE TABLE chartworks.schema_migrations(version integer PRIMARY KEY,name text NOT NULL,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT clock_timestamp())`)
	for _, migration := range migrations {
		if migration.Version > 34 {
			break
		}
		tx, err := raw.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, migration.SQL); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO chartworks.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, migration.Version, migration.Name, migration.Checksum)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(migration.Version, err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if count(t, raw, `SELECT max(version) FROM chartworks.schema_migrations`) != 34 {
		t.Fatal("not the prior schema")
	}
	tx, err := raw.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO chartworks.policy_revisions(tenant_id,revision,audit_days,operation_hours,created_by) VALUES('cw03-migration',1,12,48,'actor');
 INSERT INTO chartworks.policies VALUES('cw03-migration',1);
 INSERT INTO chartworks.topic_draft_heads(tenant_id,topic_id,actor_id,session_id,current_revision) VALUES('cw03-migration','topic','actor','session',1);
 INSERT INTO chartworks.topic_draft_versions(tenant_id,topic_id,revision,version_id,manifest,digest,change_note) VALUES('cw03-migration','topic',1,'v1','{}',repeat('a',64),'Synthetic prior metadata');
 INSERT INTO chartworks.topic_publication_heads(tenant_id,topic_id) VALUES('cw03-migration','topic');
 INSERT INTO chartworks.block_heads(tenant_id,block_id,topic_id,version,draft_revision,draft_state) VALUES('cw03-migration','legacy','topic',1,1,'draft');`)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO chartworks.block_revisions(tenant_id,block_id,revision,revision_id,definition,digest,execution_digest,actor_id,session_id,provenance,created_at) VALUES('cw03-migration','legacy',$1,$2,$3,$4,$5,'actor','session','{}',clock_timestamp())`
	if _, err = tx.Exec(ctx, insert, 1, strings.Repeat("1", 32), encoded, reporting.DefinitionDigest(legacy), reporting.ExecutionDigest(legacy)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var before, beforeDigest, beforeExecution string
	read := `SELECT definition::text,digest,execution_digest FROM chartworks.block_revisions WHERE tenant_id='cw03-migration' AND block_id='legacy' AND revision=1`
	if err = raw.QueryRow(ctx, read).Scan(&before, &beforeDigest, &beforeExecution); err != nil {
		t.Fatal(err)
	}
	db := support.Open(t, dsn)
	if err = db.Check(ctx); err != nil {
		t.Fatal(err)
	}
	var after, afterDigest, afterExecution string
	if err = raw.QueryRow(ctx, read).Scan(&after, &afterDigest, &afterExecution); err != nil {
		t.Fatal(err)
	}
	if before != after || beforeDigest != afterDigest || beforeExecution != afterExecution || count(t, raw, `SELECT definition_version FROM chartworks.block_revisions WHERE tenant_id='cw03-migration' AND block_id='legacy' AND revision=1`) != 1 {
		t.Fatal("migration rewrote immutable legacy definition or hashes")
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.schema_migrations WHERE version=35`) != 1 {
		t.Fatal("new migration not applied exactly once")
	}
	migrated, err := reporting.MigrateDefinition(legacy)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(migrated)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(ctx, insert, 2, strings.Repeat("2", 32), encoded, reporting.DefinitionDigest(migrated), reporting.ExecutionDigest(migrated)); err != nil {
		t.Fatal("v2 did not persist after real upgrade", err)
	}
	for _, shape := range []string{"missing-intent", "duplicate-order", "hidden-legacy-intent"} {
		invalid := phase27Copy(t, migrated)
		switch shape {
		case "missing-intent":
			invalid.Outputs[0].Intent = nil
		case "duplicate-order":
			invalid.Outputs[1].Intent.DisplayOrder = invalid.Outputs[0].Intent.DisplayOrder
		case "hidden-legacy-intent":
			invalid.SchemaVersion = 1
		}
		encoded, _ = json.Marshal(invalid)
		if _, err = raw.Exec(ctx, insert, 3, strings.Repeat("3", 32), encoded, reporting.DefinitionDigest(invalid), reporting.ExecutionDigest(invalid)); err == nil {
			t.Fatal("database accepted invalid versioned intent", shape)
		}
	}
	if _, err = raw.Exec(ctx, `UPDATE chartworks.block_revisions SET definition=jsonb_set(definition,'{schema_version}','2') WHERE tenant_id='cw03-migration' AND block_id='legacy' AND revision=1`); err == nil {
		t.Fatal("old immutable revision became editable")
	}
	if _, err = raw.Exec(ctx, `DELETE FROM chartworks.block_revisions WHERE tenant_id='cw03-migration' AND block_id='legacy' AND revision=1`); err == nil {
		t.Fatal("old immutable revision became deletable")
	}
	if err = raw.QueryRow(ctx, read).Scan(&after, &afterDigest, &afterExecution); err != nil || before != after || beforeDigest != afterDigest || beforeExecution != afterExecution {
		t.Fatal("rejected changes damaged original evidence", err)
	}
}
