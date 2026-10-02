package acceptance

import (
	"context"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func scopedProjectionPack(pack *semantics.TopicPack) {
	pack.Dimensions = []semantics.Dimension{{ID: "record", Name: "Record", Aliases: []string{"registro"}, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: "id"}, Role: semantics.DimensionCategorical}}
}

func requireScopedProjectionIDs(t *testing.T, result nlqexec.RunResult, columns int, ids ...string) {
	t.Helper()
	if result.Execution.Result == nil || len(result.Execution.Result.Rows) != len(ids) {
		t.Fatal("wrong dimension result row count")
	}
	for i, row := range result.Execution.Result.Rows {
		if len(row) != columns {
			t.Fatal("wrong dimension result column count")
		}
		// Exact integer values may use the adapter's lossless JSON strings.
		// Do not round-trip through floating point to compare their identity.
		raw := string(row[0])
		if raw != ids[i] && raw != `"`+ids[i]+`"` {
			t.Fatal("dimension result contains the wrong ordered source record")
		}
	}
}

func TestSQLRecoveryScopedProjectionAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := newCW01FixtureWithPack(t, scopedProjectionPack)
			ctx := context.Background()
			question := "Record"
			if locale == nlq.LanguageSpanish {
				question = "Registro"
			}
			q := f.question(question, locale)
			q.Kinds = []string{"dimension"}
			f.model.mode.Store(phase18RawResponse(t, "SELECT id FROM analytics.sales ORDER BY id"))
			f.model.mu.Lock()
			start := len(f.model.requestBodies)
			f.model.mu.Unlock()
			p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
			if err != nil || p.QueryID == "" || p.Analytical != nil || p.Route.Context == nil || !strings.Contains(p.Route.Context.Prompt, "physical_projection:topic-selected-closure-v2") || !strings.Contains(p.Route.Context.Prompt, "analytics.sales columns:id\n") {
				t.Fatal("dimension-only planning", err)
			}
			f.model.mu.Lock()
			wire := strings.Join(f.model.requestBodies[start:], "\n")
			f.model.mu.Unlock()
			if !strings.Contains(wire, "topic-selected-closure-v2") {
				t.Fatal("scoped physical context never reached the provider")
			}
			sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
			stored, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
			if err != nil || len(stored.RelationScope) != 1 || len(stored.RelationScope[0].Columns) != 5 || len(stored.Generation.Context.Relations) != 1 || len(stored.Generation.Context.Relations[0].Columns) != 5 || readexec.Hash(stored.Route.Context.Relations) != readexec.Hash(p.Route.Context.Relations) {
				t.Fatal("prompt projection narrowed durable validator scope", err)
			}
			r := f.run(t, p, 2, false)
			requireScopedProjectionIDs(t, r, 1, "1", "2")
			if r.Analytical != nil {
				t.Fatal("dimension projection acquired an aggregate proof")
			}
			saved, err := f.f.db.ReadSavedQuery(ctx, f.e, p.QueryID, false)
			if err != nil || saved.Result != nil || readexec.Hash(saved.RelationScope) != readexec.Hash(stored.RelationScope) {
				t.Fatal("saved projection leaked results or changed reviewed scope", err)
			}
			metadata := support.Raw(t, f.f.dsn)
			calls, attempts := f.model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			replayed := f.run(t, p, 2, false)
			requireScopedProjectionIDs(t, replayed, 1, "1", "2")
			if f.model.requests.Load() != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("terminal replay repeated source or model work")
			}
		})
	}
}

func TestSQLRecoveryScopedProjectionPrivateAcceptance(t *testing.T) {
	t.Run("dimension plus private filter", func(t *testing.T) {
		f := newCW01FixtureWithPack(t, scopedProjectionPack)
		q := f.question("Record named sales", nlq.LanguageEnglish)
		q.Kinds = []string{"dimension"}
		f.model.mode.Store(phase18RawResponse(t, "SELECT id FROM analytics.sales ORDER BY id"))
		f.model.mu.Lock()
		start := len(f.model.requestBodies)
		f.model.mu.Unlock()
		p := f.plan(t, q, "customer", cw01Text("primero"))
		if p.Analytical != nil || p.Bindings == nil || p.Route.Context == nil || !strings.Contains(p.Route.Context.Prompt, "topic-selected-closure-v2") {
			t.Fatal("private dimension predicate lost projection or binding")
		}
		// ID answers the dimension request; name is still required by the private
		// server-owned predicate, even though it is not an output dimension.
		var physical string
		for _, line := range strings.Split(p.Route.Context.Prompt, "\n") {
			if strings.HasPrefix(line, "relation[") {
				physical += line
			}
		}
		if !strings.Contains(physical, "id") || !strings.Contains(physical, "name") || strings.Contains(physical, "amount") || strings.Contains(physical, "created_at") {
			t.Fatal("dimension projection lost its private-filter dependency")
		}
		r := f.run(t, p, 1, false)
		requireScopedProjectionIDs(t, r, 1, "1")
		f.model.mu.Lock()
		wire := strings.Join(f.model.requestBodies[start:], "\n")
		f.model.mu.Unlock()
		if !strings.Contains(wire, "topic-selected-closure-v2") || strings.Contains(wire, "cw-alpha-731") || strings.Contains(wire, "alias-secret-731") {
			t.Fatal("provider projection omitted its marker or exposed private values")
		}
	})
	t.Run("private filter alone keeps detail context", func(t *testing.T) {
		f := newCW01FixtureWithPack(t, scopedProjectionPack)
		q := f.question("named sales", nlq.LanguageEnglish)
		q.Kinds = []string{"dimension"}
		f.model.mode.Store(phase18RawResponse(t, "SELECT id, amount FROM analytics.sales ORDER BY id"))
		p := f.plan(t, q, "customer", cw01Text("primero"))
		if p.Route.Context == nil || p.Bindings == nil || p.Analytical != nil || strings.Contains(p.Route.Context.Prompt, "physical_projection:") {
			t.Fatal("private filter became an output selection")
		}
		var physical string
		for _, line := range strings.Split(p.Route.Context.Prompt, "\n") {
			if strings.HasPrefix(line, "relation[") {
				physical += line
			}
		}
		if !strings.Contains(physical, "id") || !strings.Contains(physical, "amount") || !strings.Contains(physical, "created_at") || !strings.Contains(physical, "name") {
			t.Fatal("filter-only request lost unselected detail fields")
		}
		r := f.run(t, p, 1, true)
		requireScopedProjectionIDs(t, r, 2, "1")
	})
}
