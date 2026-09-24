package acceptance

import (
	"context"
	"encoding/json"
	"errors"
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

func TestSQLRecoveryAnalyticalGrainAcceptance(t *testing.T) {
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
	good := `SELECT id AS record,sum(amount) AS revenue FROM analytics.sales GROUP BY id ORDER BY id`
	plan := func(t *testing.T, q nlqexec.QuestionRequest) nlqexec.PlanResult {
		t.Helper()
		model.mode.Store(phase18RawResponse(t, good))
		out, err := query.Plan(ctx, pf.e, nlqexec.PlanRequest{QuestionRequest: q})
		if err != nil || out.Analytical == nil || out.Analytical.Version != readexec.AnalyticalCalendarVersion || out.Analytical.Scope != readexec.AnalyticalGrainScope || len(out.Analytical.Grouping) != 1 || out.Analytical.Grouping[0] != pack.Topic+":dimension:record" {
			t.Fatal("grain plan", err)
		}
		return out
	}
	t.Run("free-text grouping results persistence and zero-work replay", func(t *testing.T) {
		p := plan(t, question("Revenue by Record")) // No caller MetricIDs or References.
		out, err := query.Run(ctx, pf.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
		if err != nil || out.Execution.Result == nil || len(out.Execution.Result.Rows) != 2 || out.Analytical == nil || readexec.Hash(out.Analytical) != readexec.Hash(p.Analytical) {
			t.Fatal("native grouped result", err)
		}
		for i, row := range out.Execution.Result.Rows {
			if len(row) != 2 {
				t.Fatal("missing group/output")
			}
			var id string
			if json.Unmarshal(row[0], &id) != nil || id != []string{"1", "2"}[i] {
				t.Fatal("wrong grouping key")
			}
		}
		attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
		calls := model.requests.Load()
		replay, err := query.Run(ctx, pf.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
		if err != nil || readexec.Hash(replay.Execution.Result.Rows) != readexec.Hash(out.Execution.Result.Rows) || model.requests.Load() != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
			t.Fatal("grain replay recomputed or changed evidence", err)
		}
		sc, _ := store.NewScope(pf.e.Tenant(), pf.e.User())
		saved, err := f.db.ReadQuery(ctx, sc, p.QueryID)
		if err != nil || saved.AnalyticalVersion != 3 || readexec.Hash(saved.Analytical) != readexec.Hash(p.Analytical) {
			t.Fatal("grain receipt lost", err)
		}
		projected, err := f.db.ReadSavedQuery(ctx, pf.e, p.QueryID, false)
		if err != nil || projected.Result != nil || readexec.Hash(projected.Analytical) != readexec.Hash(p.Analytical) {
			t.Fatal("saved projection lost proof or leaked result", err)
		}
		for _, mutation := range []string{`analytical_version=1`, `analytical=NULL`, `analytical=analytical-'grouping'`, `analytical=jsonb_set(analytical,'{grouping}','["other:dimension:record"]')`, `analytical=jsonb_set(analytical,'{scope}','"selected_metric_expression_and_population;single_base_relation"')`} {
			if _, err := metadata.Exec(ctx, `UPDATE chartworks.nlq_queries SET `+mutation+` WHERE query_id=$1`, p.QueryID); err == nil {
				t.Fatal("immutable grain proof could be dropped or substituted")
			}
		}
	})
	t.Run("Spanish reviewed alias", func(t *testing.T) { q := question("Revenue por registro"); q.Locale = nlq.LanguageSpanish; plan(t, q) })
	t.Run("wrong grain is corrected with its closed diagnostic", func(t *testing.T) {
		bad := `SELECT sum(amount) AS revenue FROM analytics.sales`
		model.mu.Lock()
		start := len(model.requestBodies)
		model.chatSequence = []string{phase18RawResponse(t, bad), phase18RawResponse(t, good)}
		model.mu.Unlock()
		p, err := query.Plan(ctx, pf.e, nlqexec.PlanRequest{QuestionRequest: question("Revenue by Record")})
		if err != nil || p.QueryID == "" || p.ValidationFixes != 1 || p.Analytical == nil || p.Analytical.Scope != readexec.AnalyticalGrainScope {
			t.Fatal("bounded grain correction", err)
		}
		model.mu.Lock()
		wire := strings.Join(model.requestBodies[start:], "\n")
		model.mu.Unlock()
		if !strings.Contains(wire, "analytical_grain_mismatch") || !strings.Contains(wire, "rejected_sql") || !strings.Contains(wire, "reviewed-calendar-suffix-v1") {
			t.Fatal("grain correction missing failed SQL or exact intent")
		}
	})
	t.Run("safe wrong grouping never executes", func(t *testing.T) {
		for _, sql := range []string{`SELECT sum(amount) AS revenue FROM analytics.sales`, `SELECT amount,sum(amount) AS revenue FROM analytics.sales GROUP BY amount`, `SELECT id,sum(amount) AS revenue FROM analytics.sales GROUP BY id,amount`, `SELECT sum(amount) AS revenue FROM analytics.sales GROUP BY id`} {
			model.mode.Store(phase18RawResponse(t, sql))
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			p, err := query.Plan(ctx, pf.e, nlqexec.PlanRequest{QuestionRequest: question("Revenue by Record")})
			if p.QueryID != "" || !errors.Is(err, readexec.ErrAnalyticalMismatch) || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("wrong grouping executed/planned", err)
			}
		}
	})
	t.Run("retained v1 cannot acquire v2 grain proof", func(t *testing.T) {
		p := plan(t, question("Revenue by Record"))
		sc, _ := store.NewScope(pf.e.Tenant(), pf.e.User())
		q, err := f.db.ReadQuery(ctx, sc, p.QueryID)
		if err != nil {
			t.Fatal(err)
		}
		binding, err := f.s.Binding(ctx, pf.e, pack.Datasets[0].Source.Source, pf.context)
		if err != nil {
			t.Fatal(err)
		}
		legacy := readexec.AnalyticalContract{Version: readexec.AnalyticalVersion, Binding: readexec.Hash(binding), Semantics: q.Route.Selection.Digest, Dataset: dataset, Metrics: []readexec.AnalyticalMetric{{ID: pack.Topic + ":measure:revenue", Expression: readexec.AnalyticalExpression{Op: "sum", Column: "amount"}}}}
		q.ID = readexec.Hash([]string{"retained-v1-grain", p.QueryID})[:32]
		q.Operation = ""
		q.AnalyticalVersion = 1
		q.Analytical = &readexec.AnalyticalReceipt{Version: readexec.AnalyticalVersion, Scope: readexec.AnalyticalMetricScope, Contract: readexec.Hash(legacy), Query: readexec.AnalyticalQueryDigest(q.SQL, q.Parameters), Metrics: []string{legacy.Metrics[0].ID}}
		if err := f.db.CreateQuery(ctx, sc, q); err != nil {
			t.Fatal("v1 persisted fixture", err)
		}
		out, err := query.Run(ctx, pf.e, nlqexec.RunRequest{QueryID: q.ID, Operation: q.ID + "-run"})
		if err != nil || out.Analytical == nil || out.Analytical.Version != readexec.AnalyticalVersion || len(out.Analytical.Grouping) != 0 || out.Execution.Result == nil {
			t.Fatal("retained proof reinterpreted", err)
		}
	})
}

func TestSQLRecoveryAnalyticalGrainOwnedScalar(t *testing.T) {
	f := newCW01FixtureWithPack(t, func(pack *semantics.TopicPack) {
		pack.Dimensions = []semantics.Dimension{{ID: "record", Name: "Record", Description: "Reviewed synthetic grouping", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: "id"}, Role: semantics.DimensionIdentifier}}
	})
	ctx := context.Background()
	q := f.question("Revenue named sales by Record", nlq.LanguageEnglish)
	pending := f.preflight(t, q)
	q.ClarificationQuery = pending.QueryID
	q.AnswerContext = pending.Route.AnswerContext
	q.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("primero"))}
	bad := `SELECT sum(amount) AS revenue FROM analytics.sales`
	good := `SELECT id,sum(amount) AS revenue FROM analytics.sales GROUP BY id`
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.chatSequence = []string{phase18RawResponse(t, bad), phase18RawResponse(t, good)}
	f.model.mu.Unlock()
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil || p.Analytical == nil || p.Analytical.Scope != readexec.AnalyticalGrainScope || p.ValidationFixes != 1 || p.Bindings == nil {
		t.Fatal("owned scalar grain repair", err)
	}
	out := f.run(t, p, 1, true)
	if out.Analytical == nil || readexec.Hash(out.Analytical) != readexec.Hash(p.Analytical) {
		t.Fatal("grain proof changed on execution")
	}
	f.model.mu.Lock()
	wire := strings.Join(f.model.requestBodies[start:], "\n")
	f.model.mu.Unlock()
	if strings.Contains(wire, "cw-alpha-731") || strings.Contains(wire, "alias-secret-731") || !strings.Contains(wire, "analytical_grain_mismatch") {
		t.Fatal("grain repair leaked scalar or lacked diagnostic")
	}
}
