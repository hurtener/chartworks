package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
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
	if err != nil || len(manifest) != 44 || manifest[38].Name != "migrations/039_template_selection_evidence.sql" || manifest[39].Name != "migrations/040_reporting_template_selections.sql" || manifest[40].Name != "migrations/041_reporting_output_locale_bounds.sql" || manifest[41].Name != "migrations/042_learning_templates.sql" || manifest[42].Name != "migrations/043_reporting_display_intent.sql" || manifest[43].Name != "migrations/044_reporting_question_assessments.sql" {
		t.Fatal("CW-06 migration was not appended to the shipped schema", err)
	}
	for _, migration := range manifest[1:38] {
		sql(t, raw, migration.SQL)
		sql(t, raw, `INSERT INTO chartworks.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, migration.Version, migration.Name, migration.Checksum)
	}
	sql(t, raw, `INSERT INTO chartworks.nlq_sessions(tenant_id,actor_id,session_id,context_id,topics,locale) VALUES('cw06-upgrade','actor','session','context','["topic"]','en')`)
	sql(t, raw, `INSERT INTO chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id,topic_id,topics,topic_versions,rule_versions,context_id,locale,question,route,generation,parameters,receipt,status,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision)
 VALUES('cw06-upgrade','actor','session',repeat('1',32),'topic','["topic"]','["v1"]','["rules-v1"]','context','en','What is revenue?','{}','{}','[]','{}','planned','[]','[]','[]',0,0,1)`)
	sql(t, raw, `INSERT INTO chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id,topic_id,topics,topic_versions,rule_versions,context_id,locale,question,route,generation,parameters,receipt,status,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision)
 VALUES('cw06-upgrade','actor','session',repeat('3',32),'topic','["topic"]','["v1"]','["rules-v1"]','context','en','What is margin?','{"templates":[{"id":"invented"}],"request":{"templates":[{"id":"invented"}]}}','{}','[]','{}','planned','[]','[]','[]',0,0,1)`)
	sql(t, raw, `INSERT INTO chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id,topic_id,topics,topic_versions,rule_versions,context_id,locale,question,route,generation,parameters,receipt,status,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision)
 VALUES('cw06-upgrade','actor','session',repeat('4',32),'topic','["topic"]','["v1"]','["rules-v1"]','context','en','Null evidence','{"templates":null,"request":{"templates":null}}','{}','[]','{}','planned','[]','[]','[]',0,0,1)`)
	sql(t, raw, `INSERT INTO chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id,topic_id,topics,topic_versions,rule_versions,context_id,locale,question,route,generation,parameters,receipt,status,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision)
 VALUES('cw06-upgrade','actor','session',repeat('5',32),'topic','["topic"]','["v1"]','["rules-v1"]','context','en','Explicit empty evidence','{"templates":[],"request":{"templates":[]}}','{}','[]','{}','planned','[]','[]','[]',0,0,1)`)
	sql(t, raw, `INSERT INTO chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id,topic_id,topics,topic_versions,rule_versions,context_id,locale,question,route,generation,parameters,receipt,status,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision)
 VALUES('cw06-upgrade','actor','session',repeat('6',32),'topic','["topic"]','["v1"]','["rules-v1"]','context','en','Partial evidence','{"templates":[],"request":{}}','{}','[]','{}','planned','[]','[]','[]',0,0,1)`)
	database := support.Open(t, dsn)
	if err = database.Check(ctx); err != nil {
		t.Fatal(err)
	}
	var templates string
	if err = raw.QueryRow(ctx, `SELECT template_selections::text FROM chartworks.nlq_queries WHERE tenant_id='cw06-upgrade' AND query_id=repeat('1',32)`).Scan(&templates); err != nil || templates != "[]" {
		t.Fatal("old query did not receive explicit empty selection evidence", templates, err)
	}
	scope := support.Scope(t, "cw06-upgrade", "actor")
	retained, err := database.ReadQuery(ctx, scope, "11111111111111111111111111111111")
	if err != nil || len(retained.Templates) != 0 || len(retained.Route.Templates) != 0 || len(retained.Route.Request.Templates) != 0 {
		t.Fatal("old query with no selection did not remain readable", retained, err)
	}
	if _, err = database.ReadQuery(ctx, scope, "33333333333333333333333333333333"); !errors.Is(err, store.ErrMigration) {
		t.Fatal("backfill accepted conflicting retained selection evidence", err)
	}
	if _, err = database.ReadQuery(ctx, scope, "44444444444444444444444444444444"); !errors.Is(err, store.ErrMigration) {
		t.Fatal("backfill treated explicit null selection evidence as omission", err)
	}
	explicitEmpty, err := database.ReadQuery(ctx, scope, "55555555555555555555555555555555")
	if err != nil || explicitEmpty.Templates == nil || explicitEmpty.Route.Templates == nil || explicitEmpty.Route.Request.Templates == nil {
		t.Fatal("explicit empty selection evidence was not canonicalized", explicitEmpty, err)
	}
	if _, err = database.ReadQuery(ctx, scope, "66666666666666666666666666666666"); !errors.Is(err, store.ErrMigration) {
		t.Fatal("backfill accepted partially present empty selection evidence", err)
	}
	sql(t, raw, `INSERT INTO chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id,topic_id,topics,topic_versions,rule_versions,template_selections,example_selection,context_id,locale,question,route,generation,parameters,receipt,status,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision)
 VALUES('cw06-upgrade','actor','session',repeat('7',32),'topic','["topic"]','["v1"]','["rules-v1"]','[null]','{}','context','en','Malformed evidence','{"templates":[null],"request":{"templates":[null]}}','{}','[]','{}','planned','[]','[]','[]',0,0,1)`)
	if _, err = database.ReadQuery(ctx, scope, "77777777777777777777777777777777"); !errors.Is(err, store.ErrMigration) {
		t.Fatal("new-schema decoder accepted a malformed selection element", err)
	}
	digestA, digestB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	selection := rulesets.TemplateSelection{ID: "reviewed", Topic: "topic", TopicVersion: "v1", PackDigest: digestA, RuleVersion: "rules-v1", RuleDigest: digestB}
	now := time.Now().UTC()
	base := nlqexec.QueryRecord{ID: "88888888888888888888888888888888", Session: "session", Topic: "topic", Topics: []string{"topic"}, TopicVersions: []string{"v1"}, RuleVersions: []string{"rules-v1"}, Templates: []rulesets.TemplateSelection{selection}, Context: "context", Locale: "en", Question: "Valid selection", Route: nlqroute.RouteResult{Templates: []rulesets.TemplateSelection{selection}, Request: nlqroute.RouteRequest{Templates: []rulesets.TemplateSelection{selection}}}, Status: "planned", Revision: 1, Created: now, Updated: now}
	if err = database.CreateQuery(ctx, scope, base); err != nil {
		t.Fatal("valid exact new selection was rejected", err)
	}
	malformed := base
	malformed.ID = "99999999999999999999999999999999"
	malformed.Templates = []rulesets.TemplateSelection{{}}
	malformed.Route.Templates = []rulesets.TemplateSelection{{}}
	malformed.Route.Request.Templates = []rulesets.TemplateSelection{{}}
	if err = database.CreateQuery(ctx, scope, malformed); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("new writer accepted an empty selection object", err)
	}
	mismatched := base
	mismatched.ID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	mismatched.Route.Request.Templates = []rulesets.TemplateSelection{{ID: "other", Topic: "topic", TopicVersion: "v1", PackDigest: digestA, RuleVersion: "rules-v1", RuleDigest: digestB}}
	if err = database.CreateQuery(ctx, scope, mismatched); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("new writer accepted mismatched selection copies", err)
	}
	foreignTopic := base
	foreignTopic.ID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	foreignSelection := selection
	foreignSelection.Topic = "foreign"
	foreignTopic.Templates = []rulesets.TemplateSelection{foreignSelection}
	foreignTopic.Route.Templates = []rulesets.TemplateSelection{foreignSelection}
	foreignTopic.Route.Request.Templates = []rulesets.TemplateSelection{foreignSelection}
	if err = database.CreateQuery(ctx, scope, foreignTopic); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("new writer accepted a selection for a foreign topic", err)
	}
	wrongTopicVersion := base
	wrongTopicVersion.ID = "cccccccccccccccccccccccccccccccc"
	wrongTopicVersionSelection := selection
	wrongTopicVersionSelection.TopicVersion = "v2"
	wrongTopicVersion.Templates = []rulesets.TemplateSelection{wrongTopicVersionSelection}
	wrongTopicVersion.Route.Templates = []rulesets.TemplateSelection{wrongTopicVersionSelection}
	wrongTopicVersion.Route.Request.Templates = []rulesets.TemplateSelection{wrongTopicVersionSelection}
	if err = database.CreateQuery(ctx, scope, wrongTopicVersion); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("new writer accepted a mismatched template topic version", err)
	}
	wrongRuleVersion := base
	wrongRuleVersion.ID = "dddddddddddddddddddddddddddddddd"
	wrongRuleVersionSelection := selection
	wrongRuleVersionSelection.RuleVersion = "rules-v2"
	wrongRuleVersion.Templates = []rulesets.TemplateSelection{wrongRuleVersionSelection}
	wrongRuleVersion.Route.Templates = []rulesets.TemplateSelection{wrongRuleVersionSelection}
	wrongRuleVersion.Route.Request.Templates = []rulesets.TemplateSelection{wrongRuleVersionSelection}
	if err = database.CreateQuery(ctx, scope, wrongRuleVersion); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("new writer accepted a mismatched template rule version", err)
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.nlq_queries WHERE tenant_id='cw06-upgrade' AND query_id IN (repeat('9',32),repeat('a',32),repeat('b',32),repeat('c',32),repeat('d',32))`) != 0 {
		t.Fatal("invalid new selection evidence reached storage")
	}
	if _, err = raw.Exec(ctx, `UPDATE chartworks.nlq_queries SET template_selections='[{"id":"invented"}]' WHERE tenant_id='cw06-upgrade'`); err == nil {
		t.Fatal("migration made retained query evidence mutable")
	}
	if _, err = raw.Exec(ctx, `INSERT INTO chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id,topic_id,topics,topic_versions,rule_versions,context_id,locale,question,route,generation,parameters,receipt,status,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision)
 VALUES('cw06-upgrade','actor','session',repeat('2',32),'topic','["topic"]','["v1"]','["rules-v1"]','context','en','What is margin?','{}','{}','[]','{}','planned','[]','[]','[]',0,0,1)`); err == nil {
		t.Fatal("post-upgrade writer silently omitted template selection evidence")
	}
}
