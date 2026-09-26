package acceptance

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/hurtener/chartworks/test/support"
)

func confirmedProjectionFixture(t *testing.T) *cw01Fixture {
	t.Helper()
	f, draftsService, topicService, model, pack := publicationFixture(t)
	ctx := context.Background()
	if _, err := f.admin.Exec(ctx, `UPDATE analytics.sales SET name=CASE id WHEN 1 THEN 'cw-alpha-731' ELSE 'cw-beta-731' END`); err != nil {
		t.Fatal(err)
	}
	old := pack.Datasets[0]
	profile := f.profile(t, engineering.ProfileSpec{ID: "join-projection-profile", Previous: old.Source.ProfileVersion, Source: old.Source.Source, Context: old.Source.Context, Dataset: old.ID, Columns: []string{"id", "amount", "created_at", "active", "name"}, SkipLLM: true}).Profile.Profile
	d := semantics.Dataset{ID: profile.Dataset, Name: "Sales", Source: semantics.SourceReference{Source: old.Source.Source, Context: old.Source.Context, Dataset: profile.Dataset, SourceRevision: profile.SourceRevision, ProfileVersion: profile.Version, ProfileDigest: profile.DeterministicHash()}}
	for _, c := range profile.Schema {
		switch c.Name {
		case "id", "amount", "created_at", "active", "name":
			d.Columns = append(d.Columns, semantics.Column{ID: c.Name, SourceName: c.Name, Name: c.Name, NativeType: c.NativeType, Category: c.Category, Nullable: c.Nullable})
		}
	}
	pack.Datasets[0] = d
	pack = phase17EnrichPack(t, f, pack)
	pack.Dimensions = []semantics.Dimension{{ID: "record", Name: "Record", Aliases: []string{"registro"}, Role: semantics.DimensionIdentifier, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: "id"}}}
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	published := phase17PublishTopic(t, draftsService, topicService, e, pack)
	related := cloneTopic(t, pack)
	related.Topic = "join-projection-related"
	related.Name = "Related records"
	phase17PublishTopic(t, draftsService, topicService, e, related)
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := rulesets.New(f.db, f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	route, err := nlqroute.New(topicService, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	pf := &phase17Fixture{f: f, pack: pack, related: related, model: model, service: route, context: d.Source.Context}
	pf.e = phase18Envelope(t, pf, f.e.User(), "confirmed-projection", true)
	out := &cw01Fixture{phase17Fixture: pf, published: published, rules: rules, definition: cw01Definition(published)}
	out.query, _ = newPhase18Service(t, pf)
	out.publishRules(t, out.definition, 0)
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	model.mode.Store(phase18RawResponse(t, `SELECT s.id FROM analytics.sales s JOIN analytics.items i ON s.id=i.sale_id ORDER BY s.id`))
	return out
}

func confirmedProjectionQuestion(f *cw01Fixture, locale nlq.Language, private bool) nlqexec.QuestionRequest {
	text := "Record"
	if locale == nlq.LanguageSpanish {
		text = "Registro"
	}
	if private {
		text += " named sales"
	}
	return nlqexec.QuestionRequest{Topic: f.pack.Topic, Topics: []string{f.pack.Topic, f.related.Topic}, Context: f.context, Question: text, Locale: locale, Kinds: []string{"dimension"}, LimitPerKind: 1, Rerank: true, Joins: []nlqroute.JoinChoice{{Topic: f.pack.Topic, JoinID: "sales-items"}, {Topic: f.related.Topic, JoinID: "sales-items"}}}
}
func confirmedPhysicalLines(prompt string) string {
	var out []string
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, "relation[") {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func TestSQLRecoveryConfirmedJoinProjectionAcceptance(t *testing.T) {
	f := confirmedProjectionFixture(t)
	ctx := context.Background()
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			q := confirmedProjectionQuestion(f, locale, false)
			f.model.mu.Lock()
			start := len(f.model.requestBodies)
			f.model.mu.Unlock()
			p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
			if err != nil || p.QueryID == "" || p.Route.Context == nil || p.Analytical != nil || !strings.Contains(p.Route.Context.Prompt, "confirmed-join-closure-v3") {
				t.Fatal("confirmed projection Plan", err)
			}
			physical := confirmedPhysicalLines(p.Route.Context.Prompt)
			if strings.Count(physical, "analytics.sales columns:id") != 2 || strings.Count(physical, "analytics.items columns:sale_id") != 2 || strings.Contains(physical, "amount") || strings.Contains(physical, "quantity") || strings.Contains(physical, "name") {
				t.Fatal("shared schema not minimal or key missing", physical)
			}
			f.model.mu.Lock()
			wire := strings.Join(f.model.requestBodies[start:], "\n")
			f.model.mu.Unlock()
			if !strings.Contains(wire, "confirmed-join-closure-v3") || !strings.Contains(wire, "confirmed-joins-v1") {
				t.Fatal("projection did not reach actual provider")
			}
			sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
			saved, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
			if err != nil || len(saved.RelationScope) != 2 || len(saved.Generation.Context.Relations) != 4 || readexec.Hash(saved.Generation.Context.Relations) != readexec.Hash(p.Route.Context.Relations) {
				t.Fatal("durable full schema lost", err)
			}
			expectedScope := map[string][]string{
				f.pack.Datasets[0].ID: {"active", "amount", "created_at", "id", "name"},
				f.pack.Datasets[1].ID: {"quantity", "sale_id"},
			}
			for _, r := range saved.RelationScope {
				if !reflect.DeepEqual(r.Columns, expectedScope[r.Dataset]) {
					t.Fatal("validator source columns were pruned or their dataset identity changed")
				}
				delete(expectedScope, r.Dataset)
			}
			if len(expectedScope) != 0 {
				t.Fatal("validator relation disappeared from durable scope")
			}
			// Recreate the service so saved context, not in-process helper state, owns execution.
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			r := f.run(t, p, 2, false)
			requireScopedProjectionIDs(t, r, 1, "1", "2")
			projection, err := f.f.db.ReadSavedQuery(ctx, f.e, p.QueryID, false)
			if err != nil || projection.Result != nil || readexec.Hash(projection.RelationScope) != readexec.Hash(saved.RelationScope) {
				t.Fatal("saved projection changed authority/privacy", err)
			}
			metadata := support.Raw(t, f.f.dsn)
			calls, attempts := f.model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			requireScopedProjectionIDs(t, f.run(t, p, 2, false), 1, "1", "2")
			if calls != f.model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
				t.Fatal("replay repeated work")
			}
		})
	}
	t.Run("unconfirmed relationship stops before inference", func(t *testing.T) {
		q := confirmedProjectionQuestion(f, nlq.LanguageEnglish, false)
		q.Joins = q.Joins[:1]
		before := f.model.requests.Load()
		out, err := f.query.Preflight(ctx, f.e, nlqexec.PreflightRequest{QuestionRequest: q})
		if err != nil || out.Route.Clarification == nil || out.Route.Outcome != nlq.StrategyClarify || before != f.model.requests.Load() {
			t.Fatal("unconfirmed join narrowed source or reached provider", err)
		}
	})
	t.Run("unselected source remains required authority", func(t *testing.T) {
		var scopes []string
		for _, scope := range phase18Scopes(f.e.Tenant(), true) {
			if scope != "cw.dataset.query:*" {
				scopes = append(scopes, scope)
			}
		}
		scopes = append(scopes, "cw.dataset.query:"+f.pack.Datasets[0].ID)
		restricted := f.model.token.envelope(t, f.e.Tenant(), f.e.User(), scopes...)
		before := f.model.requests.Load()
		if _, err := f.query.Plan(ctx, restricted, nlqexec.PlanRequest{QuestionRequest: confirmedProjectionQuestion(f, nlq.LanguageEnglish, false)}); err == nil || before != f.model.requests.Load() {
			t.Fatal("projection granted missing dataset reach")
		}
	})
}

func TestSQLRecoveryConfirmedJoinProjectionPrivateAcceptance(t *testing.T) {
	f := confirmedProjectionFixture(t)
	q := confirmedProjectionQuestion(f, nlq.LanguageEnglish, true)
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.mu.Unlock()
	p := f.plan(t, q, "customer", cw01Text("primero"))
	if p.Bindings == nil || p.Route.Context == nil || p.Analytical != nil || !strings.Contains(p.Route.Context.Prompt, "confirmed-join-closure-v3") {
		t.Fatal("private join projection lost its consumer")
	}
	lines := confirmedPhysicalLines(p.Route.Context.Prompt)
	if !strings.Contains(lines, "name") || !strings.Contains(lines, "sale_id") || strings.Contains(lines, "quantity") || strings.Contains(lines, "amount") {
		t.Fatal("private dependency or join key missing", lines)
	}
	r := f.run(t, p, 1, false)
	requireScopedProjectionIDs(t, r, 1, "1")
	f.model.mu.Lock()
	wire := strings.Join(f.model.requestBodies[start:], "\n")
	f.model.mu.Unlock()
	if strings.Contains(wire, "cw-alpha-731") || strings.Contains(wire, "alias-secret-731") {
		t.Fatal("private scalar was sent as a join hint")
	}
}
