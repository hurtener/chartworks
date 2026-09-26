package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func groupingContinuationFixture(t *testing.T) *cw01Fixture {
	t.Helper()
	f := newCW01FixtureWithPack(t, func(pack *semantics.TopicPack) {
		ds := pack.Datasets[0].ID
		pack.Dimensions = []semantics.Dimension{
			{ID: "record", Name: "Record", Aliases: []string{"registro"}, Description: "Synthetic record grouping", Role: semantics.DimensionIdentifier, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: ds, ID: "id"}},
			{ID: "status", Name: "Status", Aliases: []string{"estado"}, Description: "Synthetic status grouping", Role: semantics.DimensionCategorical, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: ds, ID: "active"}},
			{ID: "event_date", Name: "Order date", Aliases: []string{"fecha de pedido"}, Description: "Synthetic calendar grouping", Role: semantics.DimensionTemporal, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: ds, ID: "created_at"}, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{semantics.GrainDay, semantics.GrainMonth, semantics.GrainQuarter, semantics.GrainYear}}},
		}
	})
	if _, err := f.f.admin.Exec(context.Background(), `UPDATE analytics.sales SET amount=CASE id WHEN 1 THEN 10 ELSE 20 END,active=true,created_at=CASE id WHEN 1 THEN '2025-03-10T00:00:00Z'::timestamptz ELSE '2025-04-10T00:00:00Z'::timestamptz END; INSERT INTO analytics.sales(id,amount,active,name,created_at) VALUES (3,5,false,'cw-alpha-731','2025-03-20T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	return f
}
func groupingState(f *cw01Fixture, keys ...string) *nlqroute.GroupingSelection {
	out := &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy, Keys: []nlqroute.GroupingKey{}}
	for _, key := range keys {
		out.Keys = append(out.Keys, nlqroute.GroupingKey{Topic: f.pack.Topic, Dimension: key})
	}
	return out
}
func groupingSums(t *testing.T, out nlqexec.RunResult, wants ...string) {
	t.Helper()
	if out.Execution.Result == nil || len(out.Execution.Result.Rows) != len(wants) {
		t.Fatal("wrong grouping result count")
	}
	for i, row := range out.Execution.Result.Rows {
		var raw string
		if len(row) == 0 || json.Unmarshal(row[len(row)-1], &raw) != nil {
			t.Fatal("decimal result not exact string")
		}
		got, ok := new(big.Rat).SetString(raw)
		want, _ := new(big.Rat).SetString(wants[i])
		if !ok || got.Cmp(want) != 0 {
			t.Fatal("grouping changed analytical value", raw, wants[i])
		}
	}
}
func TestSQLRecoveryGroupingContinuationLifecycleAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := groupingContinuationFixture(t)
			ctx := context.Background()
			first, next := "Revenue by Record", "Now by Status"
			if locale == nlq.LanguageSpanish {
				first, next = "Revenue por registro", "Ahora por estado"
			}
			recordSQL := `SELECT id,sum(amount) AS revenue FROM analytics.sales GROUP BY id ORDER BY id`
			statusSQL := `SELECT active,sum(amount) AS revenue FROM analytics.sales GROUP BY active ORDER BY active`
			totalSQL := `SELECT sum(amount) AS revenue FROM analytics.sales`
			f.model.mode.Store(phase18RawResponse(t, recordSQL))
			p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question(first, locale)})
			if err != nil || p.Analytical == nil {
				t.Fatal("parent", err)
			}
			run := func(p nlqexec.PlanResult) nlqexec.RunResult {
				t.Helper()
				out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
				if err != nil {
					t.Fatal("run", err)
				}
				return out
			}
			groupingSums(t, run(p), "10", "20", "5")
			sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
			parent, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
			if err != nil {
				t.Fatal(err)
			}
			digest := nlqexec.QueryLineageDigest(parent)
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			f.model.mode.Store(phase18RawResponse(t, statusSQL))
			child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: next}})
			if err != nil {
				t.Fatal("language group replacement", err)
			}
			groupingSums(t, run(child), "5", "30")
			if child.Route.Request.Grouping == nil || child.Route.Request.Grouping.Keys[0].Dimension != "status" {
				t.Fatal("logical group not durable")
			}
			retained, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: child.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Same results"}})
			if err != nil {
				t.Fatal("group inheritance", err)
			}
			groupingSums(t, run(retained), "5", "30")
			f.model.mode.Store(phase18RawResponse(t, totalSQL))
			total, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: retained.QueryID, QuestionRequest: nlqexec.QuestionRequest{Grouping: groupingState(f)}})
			if err != nil || total.Analytical == nil || total.Analytical.Scope != readexec.AnalyticalTotalScope {
				t.Fatal("explicit total", err)
			}
			groupingSums(t, run(total), "35")
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			f.model.mode.Store(phase18RawResponse(t, `SELECT date_trunc('month',created_at,'UTC'),sum(amount) AS revenue FROM analytics.sales GROUP BY 1 ORDER BY 1`))
			calendar, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: total.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Now by month of Order date"}})
			if err != nil || calendar.Analytical.Scope != readexec.AnalyticalCalendarScope {
				t.Fatal("calendar after empty grouping", err)
			}
			groupingSums(t, run(calendar), "15", "20")
			calls := f.model.requests.Load()
			metadata := support.Raw(t, f.f.dsn)
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			groupingSums(t, run(calendar), "15", "20")
			if calls != f.model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
				t.Fatal("replay made model/source call")
			}
			saved, err := f.f.db.ReadSavedQuery(ctx, f.e, calendar.QueryID, false)
			if err != nil || saved.Result != nil || saved.AnalyticalVersion != 5 || !reflect.DeepEqual(saved.Route.Request.Grouping, calendar.Route.Request.Grouping) {
				t.Fatal("saved grouping projection", err)
			}
			for _, mutate := range []string{`analytical_version=4`, `analytical=NULL`, `analytical=jsonb_set(analytical,'{contract}',to_jsonb(repeat('0',64)))`} {
				if _, err := metadata.Exec(ctx, `UPDATE chartworks.nlq_queries SET `+mutate+` WHERE query_id=$1`, calendar.QueryID); err == nil {
					t.Fatal("grouping proof could be removed or downgraded")
				}
			}
			unchanged, err := f.f.db.ReadQuery(ctx, sc, parent.ID)
			if err != nil || nlqexec.QueryLineageDigest(unchanged) != digest {
				t.Fatal("parent mutated", err)
			}
			// Same words now resolve an explicit total, so old grouped SQL must fail.
			f.model.mode.Store(phase18RawResponse(t, statusSQL))
			attempts = count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			bad, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: calendar.QueryID, QuestionRequest: nlqexec.QuestionRequest{Grouping: groupingState(f)}})
			if !errors.Is(err, readexec.ErrAnalyticalMismatch) || bad.QueryID != "" || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
				t.Fatal("stale grouping executed", err)
			}
		})
	}
}

func TestSQLRecoveryGroupingPrivateBindingsAcceptance(t *testing.T) {
	f := groupingContinuationFixture(t)
	ctx := context.Background()
	base := `SELECT id,sum(amount) AS revenue FROM analytics.sales WHERE amount > $1 GROUP BY id ORDER BY id`
	private := []readexec.Parameter{{Kind: "number", Value: "12.3456"}}
	f.model.mode.Store(parameterResponse(t, base, private))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue by Record", nlq.LanguageEnglish)})
	if err != nil {
		t.Fatal(err)
	}
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.mu.Unlock()
	grouped := `SELECT active,sum(amount) AS revenue FROM analytics.sales WHERE amount > $1 GROUP BY active ORDER BY active`
	f.model.mode.Store(parameterResponse(t, grouped, []readexec.Parameter{{Kind: "number", Value: "0"}}))
	f.query, _ = newPhase18Service(t, f.phase17Fixture)
	child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, QuestionRequest: nlqexec.QuestionRequest{Grouping: groupingState(f, "status")}})
	if err != nil {
		t.Fatal("group edit changed protected slots", err)
	}
	out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
	if err != nil {
		t.Fatal(err)
	}
	groupingSums(t, out, "20")
	sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
	saved, err := f.f.db.ReadQuery(ctx, sc, child.QueryID)
	if err != nil || !reflect.DeepEqual(saved.Parameters, private) {
		t.Fatal("lost private bindings", err)
	}
	f.model.mu.Lock()
	wire := strings.Join(f.model.requestBodies[start:], "\n")
	f.model.mu.Unlock()
	if strings.Contains(wire, private[0].Value) || !strings.Contains(wire, "retained_parameter_slots") {
		t.Fatal("private value leaked or slot metadata lost")
	}
	// Reviewed customer bindings are re-applied separately from grouping/SQL.
	q := f.question("Revenue named sales by Record", nlq.LanguageEnglish)
	pending := f.preflight(t, q)
	q.ClarificationQuery = pending.QueryID
	q.AnswerContext = pending.Route.AnswerContext
	q.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("primero"))}
	f.model.mode.Store(phase18RawResponse(t, `SELECT id,sum(amount) AS revenue FROM analytics.sales GROUP BY id ORDER BY id`))
	parent, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil {
		t.Fatal("owned parent", err)
	}
	f.model.mode.Store(phase18RawResponse(t, `SELECT active,sum(amount) AS revenue FROM analytics.sales GROUP BY active ORDER BY active`))
	owned, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: parent.QueryID, QuestionRequest: nlqexec.QuestionRequest{Grouping: groupingState(f, "status")}})
	if err != nil || owned.Bindings == nil {
		t.Fatal("owned filters/grouping", err)
	}
	out, err = f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: owned.QueryID, Operation: owned.QueryID + "-run"})
	if err != nil {
		t.Fatal(err)
	}
	groupingSums(t, out, "5", "10")
}

func TestSQLRecoveryGroupingComposesWithInferredFiltersAcceptance(t *testing.T) {
	f, query := interpretationContinuationFixture(t)
	ctx := context.Background()
	regionSQL := `SELECT name,sum(amount) AS revenue FROM analytics.sales GROUP BY name ORDER BY name`
	monthSQL := `SELECT date_trunc('month',created_at,'UTC'),sum(amount) AS revenue FROM analytics.sales GROUP BY 1 ORDER BY 1`
	totalSQL := `SELECT sum(amount) AS revenue FROM analytics.sales`
	f.model.mode.Store(phase18RawResponse(t, regionSQL))
	p, err := query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: f.pack.Topic, Context: f.context, Locale: nlq.LanguageEnglish, Question: "Revenue north in March 2025 by Region", InterpretationAnchor: "2026-09-22", Kinds: []string{"measure"}, LimitPerKind: 1}})
	if err != nil {
		t.Fatal("grouped inferred parent", err)
	}
	run := func(p nlqexec.PlanResult, wants ...string) {
		t.Helper()
		out, err := query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
		if err != nil {
			t.Fatal("grouped inferred run", err)
		}
		groupingSums(t, out, wants...)
	}
	run(p, "9007199254741053.125")
	child, err := query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Now south"}})
	if err != nil || child.Route.Request.Grouping == nil || child.Route.Request.Grouping.Keys[0].Dimension != "region" || child.Bindings == nil {
		t.Fatal("value change lost grouping/period", err)
	}
	run(child, "5.5")
	f.model.mode.Store(phase18RawResponse(t, monthSQL))
	monthly := &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy, Keys: []nlqroute.GroupingKey{{Topic: f.pack.Topic, Dimension: "event_date", Grain: semantics.GrainMonth}}}
	child, err = query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: child.QueryID, QuestionRequest: nlqexec.QuestionRequest{Grouping: monthly}})
	if err != nil || len(child.Route.Interpretation.Values) != 1 || len(child.Route.Interpretation.Temporal) != 1 {
		t.Fatal("group replacement lost independent predicates", err)
	}
	run(child, "5.5")
	child, err = query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: child.QueryID, QuestionRequest: nlqexec.QuestionRequest{InterpretationEdits: []nlqroute.InterpretationEdit{{Target: f.pack.Topic + ":event_date:time", Action: "remove"}}}})
	if err != nil || len(child.Route.Interpretation.Temporal) != 0 || child.Route.Request.Grouping == nil {
		t.Fatal("period removal changed grouping", err)
	}
	run(child, "5.5", "50")
	f.model.mode.Store(phase18RawResponse(t, totalSQL))
	child, err = query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: child.QueryID, QuestionRequest: nlqexec.QuestionRequest{Grouping: &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy, Keys: []nlqroute.GroupingKey{}}}})
	if err != nil || child.Analytical == nil || child.Analytical.Scope != readexec.AnalyticalTotalScope || child.Analytical.QueryPopulation == "" {
		t.Fatal("total dropped governed population", err)
	}
	run(child, "55.5")
}

func TestSQLRecoverySavedGroupingContinuationAcceptance(t *testing.T) {
	f := groupingContinuationFixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name, sql string
		group     *nlqroute.GroupingSelection
		want      []string
	}{
		{"status", `SELECT active,sum(amount) AS revenue FROM analytics.sales GROUP BY active ORDER BY active`, groupingState(f, "status"), []string{"5", "30"}},
		{"total", `SELECT sum(amount) AS revenue FROM analytics.sales`, groupingState(f), []string{"35"}},
		{"calendar", `SELECT date_trunc('month',created_at,'UTC'),sum(amount) AS revenue FROM analytics.sales GROUP BY 1 ORDER BY 1`, &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy, Keys: []nlqroute.GroupingKey{{Topic: f.pack.Topic, Dimension: "event_date", Grain: semantics.GrainMonth}}}, []string{"15", "20"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.model.mode.Store(phase18RawResponse(t, tc.sql))
			in := nlqexec.SavedQuestion{Durability: "replayable", Context: f.context, Question: "Revenue", Topics: []nlqexec.SavedTopic{{Topic: f.pack.Topic, Version: f.pack.Version, Digest: f.published.Digest}}, Selections: &nlqexec.SavedSelections{Grouping: nlqroute.CloneGrouping(tc.group), Kinds: []string{"measure"}, LimitPerKind: 1}}
			evidence, err := f.query.InspectSaved(ctx, f.e, in)
			if err != nil {
				t.Fatal("saved group inspect", err)
			}
			operation := "saved-group-" + tc.name
			planned, err := f.query.PrepareSaved(ctx, f.e, in, evidence, operation, "en")
			if err != nil {
				t.Fatal("saved group prepare", err)
			}
			out, err := f.query.RunSaved(ctx, f.e, in, evidence, planned, 100, 1<<20, false)
			if err != nil {
				t.Fatal("saved group execute", err)
			}
			groupingSums(t, nlqexec.RunResult{Execution: out.Execution}, tc.want...)
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			calls := f.model.requests.Load()
			recovered, err := f.query.RecoverSaved(ctx, f.e, in, evidence, operation)
			if err != nil || recovered.Query != planned.Query || f.model.requests.Load() != calls {
				t.Fatal("saved group replay changed interpretation", err)
			}
			wrong := in
			wrong.Selections = &nlqexec.SavedSelections{Grouping: groupingState(f, "record"), Kinds: []string{"measure"}, LimitPerKind: 1}
			if _, err := f.query.RecoverSaved(ctx, f.e, wrong, evidence, operation); err == nil || f.model.requests.Load() != calls {
				t.Fatal("another grouping borrowed saved evidence", err)
			}
		})
	}
}
