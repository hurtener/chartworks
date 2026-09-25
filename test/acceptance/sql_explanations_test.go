package acceptance

import (
	"context"
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func explanationRawResponse(t *testing.T, sql string, assumptions, ambiguities []string) string {
	t.Helper()
	content, err := json.Marshal(map[string]any{"decision": "ready", "questions": []string{}, "sql": sql, "parameters": []any{}, "assumptions": assumptions, "ambiguities": ambiguities})
	if err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(map[string]any{
		"id": "explanation-fixture", "object": "chat.completion", "model": "model-sqlgen",
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	return "chat_raw:" + string(response)
}

func requireExplanations(t *testing.T, gotA, gotB, wantA, wantB []string) {
	t.Helper()
	if len(gotA) != len(wantA) || len(gotB) != len(wantB) || strings.Join(gotA, "\x00") != strings.Join(wantA, "\x00") || strings.Join(gotB, "\x00") != strings.Join(wantB, "\x00") {
		t.Fatal("accepted generation explanations differ across lifecycle surfaces")
	}
}

func requireExplanationSum(t *testing.T, result nlqexec.RunResult, expected string) {
	t.Helper()
	if result.Execution.Result == nil || len(result.Execution.Result.Rows) != 1 || len(result.Execution.Result.Rows[0]) != 1 {
		t.Fatal("unexpected aggregate result shape")
	}
	var text string
	if json.Unmarshal(result.Execution.Result.Rows[0][0], &text) != nil {
		t.Fatal("lost exact decimal encoding")
	}
	got, ok := new(big.Rat).SetString(text)
	want, _ := new(big.Rat).SetString(expected)
	if !ok || got.Cmp(want) != 0 {
		t.Fatal("explanation lifecycle changed the exact source result")
	}
}

func TestSQLRecoveryExplanationLifecycleAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := newCW01FixtureWithPack(t, func(pack *semantics.TopicPack) {
				for i := range pack.Measures {
					if pack.Measures[i].ID == "revenue" {
						pack.Measures[i].Aliases = append(pack.Measures[i].Aliases, "Ingresos")
					}
				}
			})
			ctx := context.Background()
			assumptions := []string{"Amounts use the reviewed source unit.", "NULL amounts do not contribute to SUM."}
			ambiguities := []string{"No presentation ordering was specified."}
			question := "Revenue"
			if locale == nlq.LanguageSpanish {
				assumptions = []string{"Los importes usan la unidad revisada.", "Los importes nulos no contribuyen a la suma."}
				ambiguities = []string{"No se indicó un orden de presentación."}
				question = "Ingresos"
			}
			f.model.mode.Store(explanationRawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`, assumptions, ambiguities))
			p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question(question, locale)})
			if err != nil || p.QueryID == "" || p.Analytical == nil {
				t.Fatal("explanation plan", err)
			}
			requireExplanations(t, p.Assumptions, p.Ambiguities, assumptions, ambiguities)
			sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
			q, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
			if err != nil {
				t.Fatal(err)
			}
			requireExplanations(t, q.Assumptions, q.Ambiguities, assumptions, ambiguities)
			// A fresh service must use the durable record, not a captured Plan result.
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			calls := f.model.requests.Load()
			r, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-notes-run"})
			if err != nil || r.Execution.Result == nil || len(r.Execution.Result.Rows) != 1 || f.model.requests.Load() != calls {
				t.Fatal("restarted run", err)
			}
			requireExplanationSum(t, r, "9007199254740998.625")
			requireExplanations(t, r.Assumptions, r.Ambiguities, assumptions, ambiguities)
			saved, err := f.f.db.ReadSavedQuery(ctx, f.e, p.QueryID, false)
			if err != nil || saved.Result != nil {
				t.Fatal("saved explanation projection", err)
			}
			requireExplanations(t, saved.Assumptions, saved.Ambiguities, assumptions, ambiguities)
			metadata := support.Raw(t, f.f.dsn)
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			r, err = f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-notes-run"})
			if err != nil || f.model.requests.Load() != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("terminal explanation replay did work", err)
			}
			requireExplanationSum(t, r, "9007199254740998.625")
			requireExplanations(t, r.Assumptions, r.Ambiguities, assumptions, ambiguities)
			f.model.mode.Store(explanationRawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`, []string{"Child interpretation only."}, []string{}))
			child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID})
			if err != nil || child.QueryID == p.QueryID {
				t.Fatal("explanation refinement", err)
			}
			requireExplanations(t, child.Assumptions, child.Ambiguities, []string{"Child interpretation only."}, nil)
			unchanged, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
			if err != nil {
				t.Fatal(err)
			}
			requireExplanations(t, unchanged.Assumptions, unchanged.Ambiguities, assumptions, ambiguities)
		})
	}
}

func TestSQLRecoveryExplanationCorrectionAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.chatSequence = []string{
		explanationRawResponse(t, `SELECT avg(amount) AS revenue FROM analytics.sales`, []string{"rejected-average-canary"}, []string{"rejected-caveat-canary"}),
		explanationRawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`, []string{"accepted-sum-canary"}, []string{"accepted-caveat-canary"}),
	}
	f.model.mu.Unlock()
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue", nlq.LanguageEnglish)})
	if err != nil || p.ValidationFixes != 1 || p.Analytical == nil {
		t.Fatal("explanation validation correction", err)
	}
	requireExplanations(t, p.Assumptions, p.Ambiguities, []string{"accepted-sum-canary"}, []string{"accepted-caveat-canary"})
	r := f.run(t, p, 1, false)
	requireExplanationSum(t, r, "9007199254740998.625")
	requireExplanations(t, r.Assumptions, r.Ambiguities, p.Assumptions, p.Ambiguities)
	f.model.mu.Lock()
	wire := strings.Join(f.model.requestBodies[start:], "\n")
	f.model.mu.Unlock()
	if strings.Contains(wire, "rejected-average-canary") || strings.Contains(wire, "rejected-caveat-canary") {
		t.Fatal("rejected descriptive notes became repair instructions")
	}
	f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`))
	empty, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue", nlq.LanguageEnglish)})
	if err != nil {
		t.Fatal(err)
	}
	requireExplanations(t, empty.Assumptions, empty.Ambiguities, nil, nil)
	r = f.run(t, empty, 1, false)
	requireExplanations(t, r.Assumptions, r.Ambiguities, nil, nil)
}

func TestSQLRecoveryExplanationPrivacyAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	q := populationPrivateQuestion(t, f)
	f.model.mode.Store(explanationRawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`, []string{"Using cw-alpha-731 with alias-secret-731."}, []string{"Review primero."}))
	// Same actor/session and dependency reach, but no raw-SQL inspection grant.
	e := phase18Envelope(t, f.phase17Fixture, f.e.User(), f.e.Session(), false)
	p, err := f.query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil || p.Bindings == nil || p.SQL != "" {
		t.Fatal("private explanation plan", err)
	}
	wantA := []string{"Using [redacted answer] with [redacted answer]."}
	wantB := []string{"Review [redacted answer]."}
	requireExplanations(t, p.Assumptions, p.Ambiguities, wantA, wantB)
	r, err := f.query.Run(ctx, e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-notes-run"})
	if err != nil || r.SQL != "" || r.Execution.Result == nil {
		t.Fatal("private explanation run", err)
	}
	requireExplanationSum(t, r, "9007199254740993.125")
	requireExplanations(t, r.Assumptions, r.Ambiguities, wantA, wantB)
	f.model.mode.Store(explanationRawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`, []string{"Changed from cw-alpha-731 / alias-secret-731 to cw-beta-731."}, []string{}))
	child, err := f.query.Refine(ctx, e, nlqexec.RefineRequest{QueryID: p.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("segundo"))}}})
	if err != nil || child.Bindings == nil || child.SQL != "" {
		t.Fatal("changed private explanation", err)
	}
	if !reflect.DeepEqual(child.Assumptions, []string{"Changed from [redacted answer] / [redacted answer] to [redacted answer]."}) {
		t.Fatal("parent or current private value escaped through child notes")
	}
	requireExplanationSum(t, f.run(t, child, 1, false), "5.5")
}
