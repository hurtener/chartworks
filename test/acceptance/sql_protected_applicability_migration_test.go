package acceptance

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

// Structural SQL guard fixtures do not manufacture production writer custody.
// The SDK suite separately crosses the private seal and real query INSERT seam.
func TestProtectedApplicabilityStorageMigrationAcceptance(t *testing.T) {
	ctx := t.Context()
	dsn := support.Database(t)
	raw := upgradeFixture(t, dsn)
	migrations, err := postgres.Migrations()
	if err != nil || len(migrations) < 82 || migrations[81].Name != "migrations/082_protected_clarification_origin.sql" {
		t.Fatal("migration identity", err)
	}
	for _, m := range migrations[1:81] {
		sql(t, raw, m.SQL)
		sql(t, raw, `INSERT INTO chartworks.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, m.Version, m.Name, m.Checksum)
	}
	sql(t, raw, `INSERT INTO chartworks.nlq_sessions(tenant_id,actor_id,session_id,context_id,topics,locale) VALUES('protected-upgrade','actor','session','context','["topic"]','en')`)
	parentID := fmt.Sprintf("%032x", 1)
	legacyID := fmt.Sprintf("%032x", 2)
	// Marshal the actual production type so SQL paths cannot drift from wire tags.
	encoded, err := json.Marshal(nlqroute.RouteResult{Request: nlqroute.RouteRequest{Question: "Visible question"}, Applicability: &nlqroute.ApplicabilityEvidence{Version: 1, Query: parentID, PriorQuery: "pending", PriorDigest: readexec.Hash("pending"), OriginalQuestion: readexec.Hash("original"), Topics: []nlqroute.ApplicabilityTopic{{Matches: []semantics.ClarificationTermMatch{{Pattern: "customer", Term: 0, Specificity: 1}}}}}})
	var parent map[string]any
	if err != nil || json.Unmarshal(encoded, &parent) != nil {
		t.Fatal("typed route fixture", err)
	}

	legacy := map[string]any{"request": map[string]any{"question": "Legacy question"}}
	insert := func(id string, route map[string]any, ancestor string) error {
		b, _ := json.Marshal(route)
		var parentID, revision, digest, copyOrigin any
		if ancestor != "" {
			parentID, revision, digest, copyOrigin = ancestor, 1, readexec.Hash("retained-parent"), ancestor
		}
		_, err := raw.Exec(ctx, `INSERT INTO chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id,topic_id,topics,topic_versions,rule_versions,template_selections,example_selection,context_id,locale,question,route,generation,sql_text,parameters,receipt,status,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision,parent_id,parent_revision,parent_digest,saved_copy_parent)
 VALUES('protected-upgrade','actor','session',$1,'topic','["topic"]','["v1"]','[]','[]','{}','context','en','Retained question',$2::jsonb,'{}','SELECT 1','[]','{}','planned','[]','[]','[]',0,0,1,$3,$4,$5,$6)`, id, b, parentID, revision, digest, copyOrigin)
		return err
	}
	if err := insert(parentID, parent, ""); err != nil {
		t.Fatal("protected parent", err)
	}
	if err := insert(legacyID, legacy, ""); err != nil {
		t.Fatal("legacy parent", err)
	}
	var before string
	if err := raw.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(q) ORDER BY query_id)::text FROM chartworks.nlq_queries q`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	upgraded := support.Open(t, dsn)
	if err := upgraded.Check(ctx); err != nil {
		t.Fatal("forward upgrade", err)
	}
	var after string
	if err := raw.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(q) ORDER BY query_id)::text FROM chartworks.nlq_queries q`).Scan(&after); err != nil || before != after {
		t.Fatal("upgrade changed retained records", err)
	}
	for _, m := range migrations[:81] {
		var name, digest string
		if err := raw.QueryRow(ctx, `SELECT name,checksum FROM chartworks.schema_migrations WHERE version=$1`, m.Version).Scan(&name, &digest); err != nil || name != m.Name || digest != m.Checksum {
			t.Fatal("old migration identity", m.Version, err)
		}
	}
	copyRoute := func(id string) map[string]any {
		b, _ := json.Marshal(parent)
		var c map[string]any
		_ = json.Unmarshal(b, &c)
		p := c["clarification_applicability"].(map[string]any)
		p["query"], p["prior_query"], p["prior_digest"] = id, parentID, readexec.Hash(parent["clarification_applicability"])
		return c
	}
	childID := fmt.Sprintf("%032x", 3)
	if err := insert(childID, copyRoute(childID), parentID); err != nil {
		t.Fatal("protected saved copy shape", err)
	}
	if err := insert(fmt.Sprintf("%032x", 4), legacy, legacyID); err != nil {
		t.Fatal("legacy exact saved copy", err)
	}
	for i, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"question", func(c map[string]any) { c["request"].(map[string]any)["question"] = "Changed question" }},
		{"fingerprint", func(c map[string]any) {
			c["clarification_applicability"].(map[string]any)["original_question_digest"] = readexec.Hash("changed")
		}},
		{"matches", func(c map[string]any) { c["clarification_applicability"].(map[string]any)["topics"] = []any{} }},
		{"query", func(c map[string]any) { c["clarification_applicability"].(map[string]any)["query"] = "wrong" }},
		{"parent", func(c map[string]any) { c["clarification_applicability"].(map[string]any)["prior_query"] = "wrong" }},
		{"missing_digest", func(c map[string]any) { delete(c["clarification_applicability"].(map[string]any), "prior_digest") }},
		{"digest_shape", func(c map[string]any) {
			c["clarification_applicability"].(map[string]any)["prior_digest"] = strings.Repeat("g", 64)
		}},
		{"missing_witness", func(c map[string]any) { delete(c, "clarification_applicability") }},
		{"unknown_route_field", func(c map[string]any) { c["unexpected"] = "changed" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := fmt.Sprintf("%032x", 20+i)
			c := copyRoute(id)
			tc.change(c)
			if err := insert(id, c, parentID); err == nil {
				t.Fatal("changed protected route crossed SQL saved-copy guards")
			}
		})
	}
	for i, changedLegacy := range []map[string]any{
		{"request": map[string]any{"question": "Changed legacy question"}},
		{"request": map[string]any{"question": "Legacy question"}, "clarification_applicability": nil},
	} {
		if err := insert(fmt.Sprintf("%032x", 100+i), changedLegacy, legacyID); err == nil {
			t.Fatal("legacy saved copy lost exact route equality")
		}
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.nlq_queries`) != 4 {
		t.Fatal("rejected copy persisted")
	}
}
