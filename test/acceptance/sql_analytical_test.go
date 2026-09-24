package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"

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

// Actual publication, native parser/EXPLAIN, source results and durable replay.
// The provider speaks recorded Bifrost wire responses, not a live quality cohort.
func TestSQLRecoveryAnalyticalAcceptance(t *testing.T) {
	f, draftsService, topicService, model, pack := publicationFixture(t)
	ctx := context.Background()
	if _, err := f.admin.Exec(ctx, `UPDATE analytics.sales SET amount=CASE id WHEN 1 THEN 10 ELSE 5 END`); err != nil {
		t.Fatal(err)
	}
	dataset := pack.Datasets[0].ID
	for i := range pack.Datasets[0].Columns {
		pack.Datasets[0].Columns[i].Sensitivity = semantics.LiteralNonSensitive
	}
	field := semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset, ID: "amount"}
	id := semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset, ID: "id"}
	measure := func(metric, name, value string) semantics.Measure {
		return semantics.Measure{ID: metric, Name: name, Description: "Reviewed synthetic population", Field: field, Aggregation: semantics.AggregationSum, Filters: []semantics.SemanticFilter{{ID: "population", Field: id, Operator: "eq", Values: []string{value}}}}
	}
	pack.Measures = append(pack.Measures, measure("alpha_sales", "Alpha sales", "1"), measure("beta_sales", "Beta sales", "2"))
	pack.KPIs = append(pack.KPIs, semantics.KPI{ID: "population_ratio", Name: "Population ratio", Description: "Ratio of two independently filtered aggregates", Expression: "alpha_sales / beta_sales", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "alpha_sales"}, {Kind: semantics.KindMeasure, ID: "beta_sales"}}})
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
	pf.e = phase18Envelope(t, pf, f.e.User(), "analytical-session", true)
	query, _ := newPhase18Service(t, pf)
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	metadata := support.Raw(t, f.dsn)
	question := func(metrics ...string) nlqexec.QuestionRequest {
		return nlqexec.QuestionRequest{Topic: pack.Topic, Context: pf.context, Locale: nlq.LanguageEnglish, Question: "Compare reviewed results", MetricIDs: metrics, Kinds: []string{"measure"}, LimitPerKind: 1}
	}
	plan := func(t *testing.T, q nlqexec.QuestionRequest, sql string) nlqexec.PlanResult {
		t.Helper()
		model.mode.Store(phase18RawResponse(t, sql))
		p, err := query.Plan(ctx, pf.e, nlqexec.PlanRequest{QuestionRequest: q})
		if err != nil || p.QueryID == "" || p.Analytical == nil {
			t.Fatal("analytical plan", err)
		}
		return p
	}
	run := func(t *testing.T, p nlqexec.PlanResult) nlqexec.RunResult {
		t.Helper()
		out, err := query.Run(ctx, pf.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
		if err != nil || out.Execution.Result == nil || out.Analytical == nil || readexec.Hash(out.Analytical) != readexec.Hash(p.Analytical) {
			t.Fatal("analytical run/replay", err)
		}
		return out
	}
	value := func(t *testing.T, raw json.RawMessage, want string) {
		t.Helper()
		var text string
		if json.Unmarshal(raw, &text) != nil {
			t.Fatal("expected exact numeric string")
		}
		got, ok := new(big.Rat).SetString(text)
		expected, _ := new(big.Rat).SetString(want)
		if !ok || got.Cmp(expected) != 0 {
			t.Fatalf("result %s != %s", text, want)
		}
	}
	chatCount := func() int {
		model.mu.Lock()
		defer model.mu.Unlock()
		n := 0
		for _, name := range model.models {
			if name == model.cfg.Roles["sqlgen"].Model || name == model.cfg.Roles["sqlfix"].Model {
				n++
			}
		}
		return n
	}
	t.Run("distinct metric populations and replay", func(t *testing.T) {
		p := plan(t, question("alpha_sales", "beta_sales"), `SELECT sum(amount) FILTER (WHERE id=1) AS alpha, sum(CASE WHEN id=2 THEN amount ELSE NULL END) AS beta FROM analytics.sales`)
		out := run(t, p)
		if len(out.Execution.Result.Rows) != 1 || len(out.Execution.Result.Rows[0]) != 2 {
			t.Fatal("wrong population result shape")
		}
		value(t, out.Execution.Result.Rows[0][0], "10")
		value(t, out.Execution.Result.Rows[0][1], "5")
		countBefore := chatCount()
		attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
		replay := run(t, p)
		if chatCount() != countBefore || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts || readexec.Hash(replay.Execution.Result.Rows) != readexec.Hash(out.Execution.Result.Rows) {
			t.Fatal("replay recomputed proof with model/source or changed rows")
		}
		scope, _ := store.NewScope(pf.e.Tenant(), pf.e.User())
		saved, err := f.db.ReadQuery(ctx, scope, p.QueryID)
		if err != nil || saved.AnalyticalVersion != 4 || readexec.Hash(saved.Analytical) != readexec.Hash(p.Analytical) {
			t.Fatal("proof persistence", err)
		}
		projected, err := f.db.ReadSavedQuery(ctx, pf.e, p.QueryID, false)
		if err != nil || projected.Result != nil || projected.Analytical == nil {
			t.Fatal("saved projection dropped proof or exposed rows", err)
		}
		for _, mutation := range []string{`analytical_version=0,analytical=NULL`, `analytical=NULL`, `analytical=jsonb_set(analytical,'{contract}',to_jsonb(repeat('0',64)))`} {
			if _, err := metadata.Exec(ctx, `UPDATE chartworks.nlq_queries SET `+mutation+` WHERE query_id=$1`, p.QueryID); err == nil {
				t.Fatal("durable proof could be downgraded/replaced")
			}
		}
	})
	t.Run("arithmetic KPI and zero denominator", func(t *testing.T) {
		statement := `SELECT sum(amount) FILTER (WHERE id=1) / NULLIF(sum(amount) FILTER (WHERE id=2),0) AS result FROM analytics.sales`
		p := plan(t, question("population_ratio"), statement)
		out := run(t, p)
		value(t, out.Execution.Result.Rows[0][0], "2")
		if _, err := f.admin.Exec(ctx, `UPDATE analytics.sales SET amount=0 WHERE id=2`); err != nil {
			t.Fatal(err)
		}
		defer func() { _, _ = f.admin.Exec(ctx, `UPDATE analytics.sales SET amount=5 WHERE id=2`) }()
		p = plan(t, question("population_ratio"), statement)
		out = run(t, p)
		if string(out.Execution.Result.Rows[0][0]) != "null" {
			t.Fatal("zero denominator policy was not NULL")
		}
	})
	t.Run("safe but wrong SQL cannot execute", func(t *testing.T) {
		for _, statement := range []string{`SELECT avg(amount) AS result FROM analytics.sales`, `SELECT sum(amount) AS alpha,sum(amount) AS beta FROM analytics.sales WHERE id=1 AND id=2`, `WITH q AS (SELECT sum(amount) AS result FROM analytics.sales) SELECT result FROM q`} {
			before := chatCount()
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			model.mode.Store(phase18RawResponse(t, statement))
			p, err := query.Plan(ctx, pf.e, nlqexec.PlanRequest{QuestionRequest: question("alpha_sales", "beta_sales")})
			if p.QueryID != "" || err == nil || !errors.Is(err, readexec.ErrAnalyticalMismatch) && !errors.Is(err, readexec.ErrAnalyticalUnsupported) {
				t.Fatal("wrong metric was planned", err)
			}
			if chatCount()-before != 2 || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("semantic failure exceeded bounded correction or executed source")
			}
		}
	})
	t.Run("wrong aggregate is corrected using rejected candidate", func(t *testing.T) {
		bad := `SELECT avg(amount) FILTER (WHERE id=1) AS alpha FROM analytics.sales`
		good := `SELECT sum(amount) FILTER (WHERE id=1) AS alpha FROM analytics.sales`
		model.mu.Lock()
		start := len(model.requestBodies)
		model.chatSequence = []string{phase18RawResponse(t, bad), phase18RawResponse(t, good)}
		model.mu.Unlock()
		p, err := query.Plan(ctx, pf.e, nlqexec.PlanRequest{QuestionRequest: question("alpha_sales")})
		if err != nil || p.Analytical == nil || p.ValidationFixes != 1 || p.SQL != good {
			t.Fatal("targeted analytical correction failed", err)
		}
		out := run(t, p)
		value(t, out.Execution.Result.Rows[0][0], "10")
		model.mu.Lock()
		wire := strings.Join(model.requestBodies[start:], "\n")
		model.mu.Unlock()
		if !strings.Contains(wire, "rejected_sql") || !strings.Contains(wire, "analytical_metric_mismatch") {
			t.Fatal("correction lacked rejected SQL and closed diagnostic")
		}
	})
	t.Run("legacy rows remain unmeasured", func(t *testing.T) {
		// The additive migration's default does not fabricate proofs for prior rows.
		p := plan(t, question("alpha_sales"), `SELECT sum(amount) FILTER (WHERE id=1) AS alpha FROM analytics.sales`)
		scope, _ := store.NewScope(pf.e.Tenant(), pf.e.User())
		q, err := f.db.ReadQuery(ctx, scope, p.QueryID)
		if err != nil {
			t.Fatal(err)
		}
		// Persisted query IDs retain the existing 32-hex schema grammar.
		q.ID = readexec.Hash([]string{"legacy-analytical-fixture", p.QueryID})[:32]
		q.Operation = ""
		q.AnalyticalVersion = 0
		q.Analytical = nil
		if err := f.db.CreateQuery(ctx, scope, q); err != nil {
			t.Fatal("legacy record fixture", err)
		}
		out, err := query.Run(ctx, pf.e, nlqexec.RunRequest{QueryID: q.ID, Operation: q.ID + "-run"})
		if err != nil || out.Analytical != nil || out.Execution.Result == nil {
			t.Fatal("legacy evidence reinterpreted", err)
		}
	})
}

func TestSQLRecoveryAnalyticalOwnedScalarRepair(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	question := f.question("Revenue named sales", nlq.LanguageEnglish)
	pending := f.preflight(t, question)
	question.ClarificationQuery = pending.QueryID
	question.AnswerContext = pending.Route.AnswerContext
	question.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("primero"))}
	bad := `SELECT avg(amount) AS revenue FROM analytics.sales`
	good := `SELECT sum(amount) AS revenue FROM analytics.sales`
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.chatSequence = []string{phase18RawResponse(t, bad), phase18RawResponse(t, good)}
	f.model.mu.Unlock()
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: question})
	if err != nil || p.Analytical == nil || p.Bindings == nil || p.ValidationFixes != 1 {
		t.Fatal("owned scalar repair", err)
	}
	out := f.run(t, p, 1, true)
	if out.Analytical == nil {
		t.Fatal("run lost metric proof")
	}
	f.model.mu.Lock()
	wire := strings.Join(f.model.requestBodies[start:], "\n")
	f.model.mu.Unlock()
	if strings.Contains(wire, "cw-alpha-731") || strings.Contains(wire, "alias-secret-731") {
		t.Fatal("owned scalar was copied into model requests")
	}
}
