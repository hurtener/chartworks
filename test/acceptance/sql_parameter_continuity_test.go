package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func parameterResponse(t *testing.T, sql string, parameters []readexec.Parameter) string {
	t.Helper()
	content, err := json.Marshal(map[string]any{"sql": sql, "parameters": parameters, "assumptions": []string{}, "ambiguities": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(map[string]any{"id": "parameter-continuity-fixture", "object": "chat.completion", "model": "model-sqlgen", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}})
	if err != nil {
		t.Fatal(err)
	}
	return "chat_raw:" + string(wire)
}
func requireParameterIDs(t *testing.T, r nlqexec.RunResult, expected ...string) {
	t.Helper()
	if r.Execution.Result == nil || len(r.Execution.Result.Rows) != len(expected) {
		t.Fatal("wrong result size")
	}
	for i, row := range r.Execution.Result.Rows {
		var id string
		if len(row) == 0 || json.Unmarshal(row[0], &id) != nil || id != expected[i] {
			t.Fatal("binding changed exact source-record result")
		}
	}
}

func TestSQLRecoveryParameterContinuationAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := newCW01Fixture(t)
			ctx := context.Background()
			question := "List sales records"
			if locale == nlq.LanguageSpanish {
				question = "Listar registros de ventas"
			}
			original := []readexec.Parameter{{Kind: "number", Value: "5.25"}}
			base := `SELECT id,amount FROM analytics.sales WHERE amount > $1 ORDER BY id`
			f.model.mode.Store(parameterResponse(t, base, original))
			p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question(question, locale)})
			if err != nil || p.QueryID == "" {
				t.Fatal("parameterized parent", err)
			}
			sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
			parent, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
			if err != nil || !reflect.DeepEqual(parent.Parameters, original) {
				t.Fatal("parent persistence", err)
			}
			// A restart must recover the values from authorized storage, not the old
			// model request or a transient local cache.
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			childSQL := `SELECT id FROM analytics.sales WHERE amount > $1 ORDER BY id DESC`
			f.model.mode.Store(parameterResponse(t, childSQL, []readexec.Parameter{{Kind: "number", Value: "9999999999999999"}}))
			f.model.mu.Lock()
			start := len(f.model.requestBodies)
			f.model.mu.Unlock()
			callsBefore := f.model.requests.Load()
			changed, changedErr := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Use an amount cutoff of 20"}})
			if !errors.Is(changedErr, readexec.ErrUnsupported) || changed.QueryID != "" || f.model.requests.Load() != callsBefore {
				t.Fatal("new free-text filter silently kept the old private value", changedErr)
			}
			child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, QuestionRequest: nlqexec.QuestionRequest{EditBase: []nlq.Instruction{{Key: "output_edit", Text: "Return only IDs in descending order; preserve the current filters and their bindings."}}}})
			if err != nil || child.QueryID == p.QueryID {
				t.Fatal("parameterized child", err)
			}
			q, err := f.f.db.ReadQuery(ctx, sc, child.QueryID)
			if err != nil || !reflect.DeepEqual(q.Parameters, original) || q.Parent != p.QueryID || q.ParentRevision != parent.Revision || q.ParentDigest != nlqexec.QueryLineageDigest(parent) {
				t.Fatal("binding/lineage lost", err)
			}
			out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
			if err != nil {
				t.Fatal(err)
			}
			requireParameterIDs(t, out, "2", "1")
			f.model.mu.Lock()
			wire := strings.Join(f.model.requestBodies[start:], "\n")
			f.model.mu.Unlock()
			if strings.Contains(wire, original[0].Value) || !strings.Contains(wire, "retained_parameter_slots") {
				t.Fatal("private binding leaked or missing slot metadata")
			}
			saved, err := f.f.db.ReadSavedQuery(ctx, f.e, child.QueryID, false)
			if err != nil || saved.Result != nil || !reflect.DeepEqual(saved.Parameters, original) {
				t.Fatal("saved projection lost parameters", err)
			}
			metadata := support.Raw(t, f.f.dsn)
			calls := f.model.requests.Load()
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			replay, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
			if err != nil || f.model.requests.Load() != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("terminal replay did work", err)
			}
			requireParameterIDs(t, replay, "2", "1")
			unchanged, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
			if err != nil || nlqexec.QueryLineageDigest(unchanged) != nlqexec.QueryLineageDigest(parent) {
				t.Fatal("parent mutated", err)
			}
			// Slot-count, type, column and Boolean changes are not silently blessed as
			// a request to mutate private scalar state. No rejected child can execute.
			for _, bad := range []struct {
				sql    string
				params []readexec.Parameter
			}{
				{`SELECT id FROM analytics.sales WHERE amount>0`, []readexec.Parameter{}},
				{childSQL, []readexec.Parameter{{Kind: "text", Value: "5.25"}}},
				{`SELECT id FROM analytics.sales WHERE id > $1`, []readexec.Parameter{{Kind: "number", Value: "0"}}},
				{`SELECT id FROM analytics.sales WHERE amount > $1 OR true`, []readexec.Parameter{{Kind: "number", Value: "0"}}},
			} {
				f.model.mode.Store(parameterResponse(t, bad.sql, bad.params))
				invalid, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID})
				if err == nil || invalid.QueryID != "" || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
					t.Fatal("unrequested binding change planned/executed", err)
				}
			}
		})
	}
}

func TestSQLRecoveryParameterContinuationOwnedAnswerAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	question := f.question("List named sales", nlq.LanguageEnglish)
	pending := f.preflight(t, question)
	if pending.Route.Clarification == nil || pending.Route.Clarification.Reason != "required_answers" {
		t.Fatal("owned-answer fixture must activate the reviewed named-sales policy")
	}
	question.ClarificationQuery = pending.QueryID
	question.AnswerContext = pending.Route.AnswerContext
	question.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("primero"))}
	base := `SELECT id,amount FROM analytics.sales WHERE name <> $1 ORDER BY id`
	modelParameters := []readexec.Parameter{{Kind: "text", Value: "excluded-private-318"}}
	f.model.mode.Store(parameterResponse(t, base, modelParameters))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: question})
	if err != nil || p.Bindings == nil {
		t.Fatal("mixed-ownership parent", err)
	}
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.mu.Unlock()
	f.model.mode.Store(parameterResponse(t, base, []readexec.Parameter{{Kind: "text", Value: "ignored-model-placeholder"}}))
	child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("segundo"))}}})
	if err != nil || child.Bindings == nil {
		t.Fatal("owned answer replacement lost independent model slot", err)
	}
	sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
	q, err := f.f.db.ReadQuery(ctx, sc, child.QueryID)
	if err != nil || q.Clarification == nil || !reflect.DeepEqual(q.Clarification.BaseParameters, modelParameters) || q.Clarification.BaseSQL != base {
		t.Fatal("unbound slot ownership changed", err)
	}
	out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
	if err != nil {
		t.Fatal(err)
	}
	requireParameterIDs(t, out, "2")
	f.model.mu.Lock()
	wire := strings.Join(f.model.requestBodies[start:], "\n")
	f.model.mu.Unlock()
	for _, value := range []string{"excluded-private-318", "cw-alpha-731", "cw-beta-731", "alias-secret-731"} {
		if strings.Contains(wire, value) {
			t.Fatal("private retained or replaced value entered provider input")
		}
	}
	// Native-invalid projection repair must preserve the same parent slot values.
	f.model.mu.Lock()
	f.model.chatSequence = []string{parameterResponse(t, `SELECT missing FROM analytics.sales WHERE name <> $1 ORDER BY id`, []readexec.Parameter{{Kind: "text", Value: "dummy-one"}}), parameterResponse(t, base, []readexec.Parameter{{Kind: "text", Value: "dummy-two"}})}
	f.model.mu.Unlock()
	fixed, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: child.QueryID})
	if err != nil || fixed.ValidationFixes != 1 {
		t.Fatal("parameterized correction", err)
	}
	result, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: fixed.QueryID, Operation: fixed.QueryID + "-run"})
	if err != nil {
		t.Fatal(err)
	}
	requireParameterIDs(t, result, "2")
	// Forged service-owned base evidence cannot be borrowed as trusted edit SQL.
	metadata := support.Raw(t, f.f.dsn)
	_, err = metadata.Exec(ctx, `UPDATE chartworks.nlq_queries SET clarification=jsonb_set(clarification,'{base_parameters}','[{"kind":"text","value":"forged"}]'::jsonb) WHERE query_id=$1`, p.QueryID)
	if err != nil {
		t.Fatal("tamper fixture", err)
	}
	calls := f.model.requests.Load()
	_, err = f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID})
	if !errors.Is(err, readexec.ErrBinding) || f.model.requests.Load() != calls {
		t.Fatal("unverified owned base reached model", err)
	}
}
