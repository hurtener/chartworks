package acceptance

import (
	"context"
	"testing"

	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

// TestCW06PopulatedQueryUpgrade proves migrations 039-040 preserve old immutable
// query evidence with an explicit empty template selection. New writers must
// supply the field after the compatibility backfill; the database does not keep
// a default that could hide an incomplete consumer.
func TestCW06PopulatedQueryUpgrade(t *testing.T) {
	ctx := context.Background()
	dsn := support.Database(t)
	raw := upgradeFixture(t, dsn)
	manifest, err := postgres.Migrations()
	if err != nil || len(manifest) != 41 || manifest[38].Name != "migrations/039_template_selection_evidence.sql" || manifest[39].Name != "migrations/040_reporting_template_selections.sql" || manifest[40].Name != "migrations/041_reporting_output_locale_bounds.sql" {
		t.Fatal("CW-06 migration was not appended to the shipped schema", err)
	}
	for _, migration := range manifest[1:38] {
		sql(t, raw, migration.SQL)
		sql(t, raw, `INSERT INTO chartworks.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, migration.Version, migration.Name, migration.Checksum)
	}
	sql(t, raw, `INSERT INTO chartworks.nlq_sessions(tenant_id,actor_id,session_id,context_id,topics,locale) VALUES('cw06-upgrade','actor','session','context','["topic"]','en')`)
	sql(t, raw, `INSERT INTO chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id,topic_id,topics,topic_versions,rule_versions,context_id,locale,question,route,generation,parameters,receipt,status,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision)
 VALUES('cw06-upgrade','actor','session',repeat('1',32),'topic','["topic"]','["v1"]','["rules-v1"]','context','en','What is revenue?','{}','{}','[]','{}','planned','[]','[]','[]',0,0,1)`)
	database := support.Open(t, dsn)
	if err = database.Check(ctx); err != nil {
		t.Fatal(err)
	}
	var templates string
	if err = raw.QueryRow(ctx, `SELECT template_selections::text FROM chartworks.nlq_queries WHERE tenant_id='cw06-upgrade'`).Scan(&templates); err != nil || templates != "[]" {
		t.Fatal("old query did not receive explicit empty selection evidence", templates, err)
	}
	if _, err = raw.Exec(ctx, `UPDATE chartworks.nlq_queries SET template_selections='[{"id":"invented"}]' WHERE tenant_id='cw06-upgrade'`); err == nil {
		t.Fatal("migration made retained query evidence mutable")
	}
	if _, err = raw.Exec(ctx, `INSERT INTO chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id,topic_id,topics,topic_versions,rule_versions,context_id,locale,question,route,generation,parameters,receipt,status,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision)
 VALUES('cw06-upgrade','actor','session',repeat('2',32),'topic','["topic"]','["v1"]','["rules-v1"]','context','en','What is margin?','{}','{}','[]','{}','planned','[]','[]','[]',0,0,1)`); err == nil {
		t.Fatal("post-upgrade writer silently omitted template selection evidence")
	}
}
