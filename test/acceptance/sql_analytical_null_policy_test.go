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
	"math/big"
	"testing"
)

func TestSQLRecoveryDefaultIntentNullPolicyAcceptance(t *testing.T) {
	f, draftsService, topicService, model, pack := publicationFixture(t)
	ctx := context.Background()
	pack.KPIs = append(pack.KPIs, semantics.KPI{ID: "reviewed_zero", Name: "Reviewed zero", Description: "Explicit reviewed empty and null fallback", Expression: "coalesce(revenue,0)", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}})
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
	pf.e = phase18Envelope(t, pf, f.e.User(), "null-policy-session", true)
	query, _ := newPhase18Service(t, pf)
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	metadata := support.Raw(t, f.dsn)

	request := nlqexec.QuestionRequest{Topic: pack.Topic, Context: pf.context, Locale: nlq.LanguageEnglish, Question: "Reviewed zero", MetricIDs: []string{"reviewed_zero"}, Kinds: []string{"kpi"}, LimitPerKind: 1}
	good := `SELECT coalesce(sum(amount),0) AS value FROM analytics.sales`
	if _, err := f.admin.Exec(ctx, `UPDATE analytics.sales SET amount=NULL`); err != nil {
		t.Fatal(err)
	}
	plan := func(t *testing.T) nlqexec.PlanResult {
		t.Helper()
		model.mode.Store(phase18RawResponse(t, good))
		p, err := query.Plan(ctx, pf.e, nlqexec.PlanRequest{QuestionRequest: request})
		if err != nil || p.Analytical == nil || p.Analytical.Version != readexec.AnalyticalGroupedPopulationsVersion || p.Analytical.Intent != readexec.AnalyticalIntentPolicy || p.Analytical.QueryPopulation != readexec.AnalyticalQueryPopulationPolicy {
			t.Fatal("mandatory reviewed intent", err)
		}
		return p
	}
	runZero := func(t *testing.T, p nlqexec.PlanResult) nlqexec.RunResult {
		t.Helper()
		out, err := query.Run(ctx, pf.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
		if err != nil || out.Execution.Result == nil || len(out.Execution.Result.Rows) != 1 || len(out.Execution.Result.Rows[0]) != 1 {
			t.Fatal("reviewed zero result", err)
		}
		var value string
		if json.Unmarshal(out.Execution.Result.Rows[0][0], &value) != nil {
			t.Fatal("numeric result type")
		}
		number, ok := new(big.Rat).SetString(value)
		if !ok || number.Sign() != 0 {
			t.Fatal("fallback did not return exact zero")
		}
		return out
	}
	t.Run("all null result and durable marker replay", func(t *testing.T) {
		p := plan(t)
		out := runZero(t, p)
		scope, _ := store.NewScope(pf.e.Tenant(), pf.e.User())
		saved, err := f.db.ReadQuery(ctx, scope, p.QueryID)
		if err != nil || saved.AnalyticalVersion != 7 || !nlqexec.AnalyticalRecordValid(saved) {
			t.Fatal("persisted v7 proof", err)
		}
		for _, marker := range []string{"intent", "query_population"} {
			if _, err := metadata.Exec(ctx, `UPDATE chartworks.nlq_queries SET analytical=analytical-($1::text) WHERE query_id=$2`, marker, p.QueryID); err == nil {
				t.Fatal("mutable mandatory proof", marker)
			}
		}
		restarted, _ := newPhase18Service(t, pf)
		calls := model.requests.Load()
		attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
		replay, err := restarted.Run(ctx, pf.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
		if err != nil || readexec.Hash(replay.Execution.Result.Rows) != readexec.Hash(out.Execution.Result.Rows) || calls != model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
			t.Fatal("terminal replay changed result or repeated work", err)
		}
	})
	t.Run("unrequested narrowing remains rejected", func(t *testing.T) {
		for _, sql := range []string{`SELECT coalesce(sum(amount),0) AS value FROM analytics.sales WHERE id>1`, `SELECT coalesce(sum(amount),0) AS value FROM analytics.sales LIMIT 1`, `SELECT coalesce(sum(amount),0) AS value FROM analytics.sales HAVING sum(amount)>0`} {
			model.mode.Store(phase18RawResponse(t, sql))
			before := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			p, err := query.Plan(ctx, pf.e, nlqexec.PlanRequest{QuestionRequest: request})
			if p.QueryID != "" || !errors.Is(err, readexec.ErrAnalyticalMismatch) || before != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
				t.Fatal("unrequested narrowing acquired executable record", err)
			}
		}
	})
	t.Run("bounded repair retains reviewed fallback", func(t *testing.T) {
		t.Cleanup(func() { model.mu.Lock(); model.chatSequence = nil; model.mu.Unlock() })
		model.mu.Lock()
		model.chatSequence = []string{phase18RawResponse(t, `SELECT sum(amount) AS value FROM analytics.sales`), phase18RawResponse(t, good)}
		model.mu.Unlock()
		p, err := query.Plan(ctx, pf.e, nlqexec.PlanRequest{QuestionRequest: request})
		if err != nil || p.ValidationFixes != 1 {
			t.Fatal("fallback correction", err)
		}
		runZero(t, p)
	})
	t.Run("empty population uses only explicit reviewed fallback", func(t *testing.T) {
		if _, err := f.admin.Exec(ctx, `DELETE FROM analytics.sales`); err != nil {
			t.Fatal(err)
		}
		runZero(t, plan(t))
	})
}
