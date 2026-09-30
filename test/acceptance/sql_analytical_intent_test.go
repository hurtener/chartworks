package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/hurtener/chartworks/test/support"
	"testing"
)

func TestSQLRecoveryAnalyticalIntentAcceptance(t *testing.T) {
	f, draftsService, topicService, model, pack := publicationFixture(t)
	ctx := context.Background()
	dataset := pack.Datasets[0].ID
	pack.Dimensions = []semantics.Dimension{{ID: "record", Name: "Record", Aliases: []string{"registro"}, Description: "Reviewed synthetic direct grouping", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset, ID: "id"}, Role: semantics.DimensionIdentifier}}
	pubEnvelope := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	phase17PublishTopic(t, draftsService, topicService, pubEnvelope, pack)
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := rulesets.New(f.db, f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	router, err := nlqroute.New(topicService, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	pf := &phase17Fixture{f: f, pack: pack, model: model, service: router, context: pack.Datasets[0].Source.Context}
	pf.e = phase18Envelope(t, pf, f.e.User(), "grain-session", true)
	query, _ := newPhase18Service(t, pf)
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	metadata := support.Raw(t, f.dsn)
	question := func(text string) nlqexec.QuestionRequest {
		return nlqexec.QuestionRequest{Topic: pack.Topic, Context: pf.context, Locale: nlq.LanguageEnglish, Question: text, Kinds: []string{"measure"}, LimitPerKind: 1}
	}

	if _, err := f.admin.Exec(ctx, `UPDATE analytics.sales SET amount=CASE id WHEN 1 THEN 10 ELSE 20 END; INSERT INTO analytics.sales(id,amount) VALUES (3,NULL),(4,20)`); err != nil {
		t.Fatal(err)
	}
	good := `SELECT id AS record,sum(amount) AS revenue FROM analytics.sales GROUP BY id ORDER BY revenue DESC NULLS LAST,record ASC LIMIT 2`
	request := question("Revenue by Record ordered by Revenue descending nulls last, Record ascending limit 2")
	t.Run("exact rows durable receipt and terminal replay", func(t *testing.T) {
		model.mode.Store(phase18RawResponse(t, good))
		p, err := query.Plan(ctx, pf.e, nlqexec.PlanRequest{QuestionRequest: request})
		if err != nil || p.Analytical == nil || p.Analytical.Intent != readexec.AnalyticalIntentPolicy {
			t.Fatal("intent plan", err)
		}
		out, err := query.Run(ctx, pf.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
		if err != nil || out.Execution.Result == nil || len(out.Execution.Result.Rows) != 2 {
			t.Fatal("intent result", err)
		}
		for i, row := range out.Execution.Result.Rows {
			var id string
			if json.Unmarshal(row[0], &id) != nil || id != []string{"2", "4"}[i] {
				t.Fatal("incorrect order, null, limit or tie-break semantics")
			}
		}
		sc, _ := store.NewScope(pf.e.Tenant(), pf.e.User())
		saved, err := f.db.ReadQuery(ctx, sc, p.QueryID)
		if err != nil || saved.AnalyticalVersion != 6 || saved.Analytical.Intent != readexec.AnalyticalIntentPolicy {
			t.Fatal("lost durable intent", err)
		}
		restarted, _ := newPhase18Service(t, pf)
		calls := model.requests.Load()
		attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
		replay, err := restarted.Run(ctx, pf.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
		if err != nil || readexec.Hash(replay.Execution.Result.Rows) != readexec.Hash(out.Execution.Result.Rows) || model.requests.Load() != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
			t.Fatal("replay changed result or did work", err)
		}
		if _, err := metadata.Exec(ctx, `UPDATE chartworks.nlq_queries SET analytical=analytical-'intent' WHERE query_id=$1`, p.QueryID); err == nil {
			t.Fatal("mutable proof marker")
		}
	})
	t.Run("bounded correction of wrong ordering", func(t *testing.T) {
		t.Cleanup(func() { model.mu.Lock(); model.chatSequence = nil; model.mu.Unlock() })
		model.mu.Lock()
		model.chatSequence = []string{phase18RawResponse(t, `SELECT id AS record,sum(amount) AS revenue FROM analytics.sales GROUP BY id ORDER BY revenue ASC NULLS LAST,record ASC LIMIT 2`), phase18RawResponse(t, good)}
		model.mu.Unlock()
		p, err := query.Plan(ctx, pf.e, nlqexec.PlanRequest{QuestionRequest: request})
		if err != nil || p.ValidationFixes != 1 || p.Analytical == nil || p.Analytical.Intent != readexec.AnalyticalIntentPolicy {
			t.Fatal("order correction", err)
		}
	})
	t.Run("unrequested filtering and limit changes never execute", func(t *testing.T) {
		for _, sql := range []string{
			`SELECT id AS record,sum(amount) AS revenue FROM analytics.sales GROUP BY id ORDER BY revenue DESC NULLS LAST,record ASC LIMIT 1`,
			`SELECT id AS record,sum(amount) AS revenue FROM analytics.sales WHERE id<4 GROUP BY id ORDER BY revenue DESC NULLS LAST,record ASC LIMIT 2`,
			`SELECT id AS record,sum(amount) AS revenue FROM analytics.sales GROUP BY id ORDER BY revenue DESC NULLS LAST,record ASC LIMIT 2 OFFSET 1`,
			`SELECT id AS record,sum(amount) AS revenue FROM analytics.sales GROUP BY id ORDER BY revenue DESC NULLS LAST,record ASC FETCH FIRST 2 ROWS WITH TIES`,
		} {
			model.mode.Store(phase18RawResponse(t, sql))
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			p, err := query.Plan(ctx, pf.e, nlqexec.PlanRequest{QuestionRequest: request})
			if p.QueryID != "" || !errors.Is(err, readexec.ErrAnalyticalMismatch) || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("incorrect query admitted", err)
			}
		}
	})
}
