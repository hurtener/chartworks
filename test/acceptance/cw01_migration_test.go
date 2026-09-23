package acceptance

import (
	"context"
	"testing"

	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

// Apply the already shipped migration prefix first, then upgrade through the
// normal runner. A fresh database alone cannot prove retained-prefix fidelity.
func cw01MigrationAcceptance(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	dsn := support.Database(t)
	connection := upgradeFixture(t, dsn)
	manifest, err := postgres.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) < 44 || manifest[34].Name != "migrations/035_reporting_output_intent.sql" || manifest[35].Name != "migrations/036_nlq_clarification.sql" || manifest[36].Name != "migrations/037_clarification_comparison.sql" || manifest[37].Name != "migrations/038_reporting_rule_snapshots.sql" || manifest[38].Name != "migrations/039_template_selection_evidence.sql" || manifest[39].Name != "migrations/040_reporting_template_selections.sql" || manifest[40].Name != "migrations/041_reporting_output_locale_bounds.sql" || manifest[41].Name != "migrations/042_learning_templates.sql" || manifest[42].Name != "migrations/043_reporting_display_intent.sql" || manifest[43].Name != "migrations/044_reporting_question_assessments.sql" {
		t.Fatal("CW-01 did not append to the shipped schema")
	}
	for _, migration := range manifest[1:35] {
		sql(t, connection, migration.SQL)
		sql(t, connection, `INSERT INTO chartworks.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, migration.Version, migration.Name, migration.Checksum)
	}
	sql(t, connection, `INSERT INTO chartworks.policy_revisions(tenant_id,revision,audit_days,operation_hours,created_by) VALUES('cw01-upgrade',1,12,48,'actor'); INSERT INTO chartworks.policies VALUES('cw01-upgrade',1)`)
	database := support.Open(t, dsn)
	if err := database.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, connection, `SELECT count(*) FROM chartworks.schema_migrations`) != int64(len(manifest)) {
		t.Fatal("upgrade did not apply the complete forward migration suffix")
	}
	for _, migration := range manifest[:35] {
		var name, checksum string
		if err := connection.QueryRow(ctx, `SELECT name,checksum FROM chartworks.schema_migrations WHERE version=$1`, migration.Version).Scan(&name, &checksum); err != nil || name != migration.Name || checksum != migration.Checksum {
			t.Fatal("upgrade changed a shipped migration", err)
		}
	}
	policy, err := database.Policy(ctx, support.Scope(t, "cw01-upgrade", "actor"))
	if err != nil || policy.AuditDays != 12 || policy.OperationHours != 48 {
		t.Fatal("upgrade lost retained tenant data", err)
	}
	for _, column := range [][2]string{{"nlq_queries", "clarification"}, {"topic_rule_comparison_evidence", "baseline_clarification_result"}, {"topic_rule_comparison_evidence", "candidate_clarification_result"}} {
		if count(t, connection, `SELECT count(*) FROM information_schema.columns WHERE table_schema='chartworks' AND table_name=$1 AND column_name=$2`, column[0], column[1]) != 1 {
			t.Fatal("upgrade omitted typed clarification persistence")
		}
	}
	options := postgres.Defaults()
	options.MigrationPolicy = "check"
	checked, err := postgres.Open(ctx, dsn, options)
	if err != nil {
		t.Fatal(err)
	}
	checked.Close()
}
