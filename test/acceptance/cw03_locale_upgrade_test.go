package acceptance

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

func TestCW03PopulatedV2LocaleUpgrade(t *testing.T) {
	fixture := newPhase29Execution(t, false)
	definition := cw03Definition(t, fixture.base)
	dsn := support.Database(t)
	raw := support.Raw(t, dsn)
	migrations, err := postgres.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sql(t, raw, `CREATE SCHEMA chartworks; CREATE TABLE chartworks.schema_migrations(version integer PRIMARY KEY,name text NOT NULL,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT clock_timestamp())`)
	for _, migration := range migrations {
		if migration.Version > 35 {
			break
		}
		tx, err := raw.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(t.Context(), migration.SQL); err == nil {
			_, err = tx.Exec(t.Context(), `INSERT INTO chartworks.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, migration.Version, migration.Name, migration.Checksum)
		}
		if err != nil {
			_ = tx.Rollback(t.Context())
			t.Fatal(migration.Version, err)
		}
		if err = tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if count(t, raw, `SELECT max(version) FROM chartworks.schema_migrations`) != 35 {
		t.Fatal("fixture did not install original output-intent schema")
	}
	tx, err := raw.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	_, err = tx.Exec(t.Context(), `INSERT INTO chartworks.policy_revisions(tenant_id,revision,audit_days,operation_hours,created_by) VALUES('cw03-locale-upgrade',1,12,48,'actor');
 INSERT INTO chartworks.policies VALUES('cw03-locale-upgrade',1);
 INSERT INTO chartworks.topic_draft_heads(tenant_id,topic_id,actor_id,session_id,current_revision) VALUES('cw03-locale-upgrade','topic','actor','session',1);
 INSERT INTO chartworks.topic_draft_versions(tenant_id,topic_id,revision,version_id,manifest,digest,change_note) VALUES('cw03-locale-upgrade','topic',1,'v1','{}',repeat('a',64),'Synthetic prior metadata');
 INSERT INTO chartworks.topic_publication_heads(tenant_id,topic_id) VALUES('cw03-locale-upgrade','topic');
 INSERT INTO chartworks.block_heads(tenant_id,block_id,topic_id,version,draft_revision,draft_state) VALUES('cw03-locale-upgrade','block','topic',1,1,'draft');`)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO chartworks.block_revisions(tenant_id,block_id,revision,revision_id,definition,digest,execution_digest,actor_id,session_id,provenance,created_at) VALUES('cw03-locale-upgrade','block',$1,$2,$3,$4,$5,'actor','session','{}',clock_timestamp())`
	if _, err = tx.Exec(t.Context(), insert, 1, strings.Repeat("1", 32), encoded, reporting.DefinitionDigest(definition), reporting.ExecutionDigest(definition)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	read := `SELECT definition::text,digest,execution_digest FROM chartworks.block_revisions WHERE tenant_id='cw03-locale-upgrade' AND block_id='block' AND revision=1`
	var before, after [3]string
	if err = raw.QueryRow(t.Context(), read).Scan(&before[0], &before[1], &before[2]); err != nil {
		t.Fatal(err)
	}
	wide := phase27Copy(t, definition)
	wide.Outputs[0].Intent.Metadata[0].Locale = "en-x-aaaaaaa-bbbbbbb-ccccccc-ddddddd-eeeeeee-fffffff"
	encoded, err = json.Marshal(wide)
	if err != nil {
		t.Fatal(err)
	}
	var allowed bool
	if err = raw.QueryRow(t.Context(), `SELECT chartworks.reporting_output_intents_valid($1::jsonb)`, encoded).Scan(&allowed); err != nil || allowed {
		t.Fatal("did not exercise the old narrower storage constraint", allowed, err)
	}
	// Production migration runner verifies every old checksum, preserves the
	// intervening clarification migrations and advances to the locale fix at 38.
	db := support.Open(t, dsn)
	if err = db.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.schema_migrations WHERE version=38`) != 1 {
		t.Fatal("locale migration did not apply exactly once")
	}
	if err = raw.QueryRow(t.Context(), read).Scan(&after[0], &after[1], &after[2]); err != nil || before != after {
		t.Fatal("locale migration rewrote immutable v2 bytes/digests", err)
	}
	if _, err = raw.Exec(t.Context(), insert, 2, strings.Repeat("2", 32), encoded, reporting.DefinitionDigest(wide), reporting.ExecutionDigest(wide)); err != nil {
		t.Fatal("upgraded storage rejected supported locale", err)
	}
	if _, err = raw.Exec(t.Context(), `UPDATE chartworks.block_revisions SET definition=$1 WHERE tenant_id='cw03-locale-upgrade' AND block_id='block' AND revision=1`, encoded); err == nil {
		t.Fatal("locale migration weakened immutable revision protection")
	}
	if err = raw.QueryRow(t.Context(), read).Scan(&after[0], &after[1], &after[2]); err != nil || before != after {
		t.Fatal("failed rewrite changed original definition", err)
	}
}
