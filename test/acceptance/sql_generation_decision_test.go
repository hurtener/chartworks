package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/generationdecision"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqexec"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func decisionRawResponse(t *testing.T, kind string, questions ...string) string {
	t.Helper()
	content, err := json.Marshal(map[string]any{"decision": kind, "questions": questions, "sql": "", "parameters": []any{}, "assumptions": []string{}, "ambiguities": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"id": "recorded-decision", "object": "chat.completion", "model": "model-sqlgen", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}})
	if err != nil {
		t.Fatal(err)
	}
	return "chat_raw:" + string(raw)
}
func decisionChatCount(f *cw01Fixture) int {
	f.model.mu.Lock()
	defer f.model.mu.Unlock()
	n := 0
	for _, model := range f.model.models {
		if model == f.model.cfg.Roles["sqlgen"].Model || model == f.model.cfg.Roles["sqlfix"].Model {
			n++
		}
	}
	return n
}
func TestSQLRecoveryDecisionNoExecutablePlanAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	db := support.Raw(t, f.f.dsn)
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		for _, kind := range []string{generationdecision.Clarify, generationdecision.Insufficient} {
			q := f.question("Revenue", locale)
			asked := "Which reviewed metric answers the request?"
			if locale == nlq.LanguageSpanish {
				asked = "¿Qué métrica revisada corresponde?"
			}
			f.model.mode.Store(decisionRawResponse(t, kind, asked))
			beforeCalls := decisionChatCount(f)
			plans := count(t, db, `SELECT count(*) FROM chartworks.nlq_queries`)
			attempts := count(t, db, `SELECT count(*) FROM chartworks.read_attempts`)
			out, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
			problem := nlqexec.GenerationProblem(err)
			if out.QueryID != "" || problem == nil || problem.Outcome != kind || problem.Questions[0] != asked || decisionChatCount(f) != beforeCalls+1 || count(t, db, `SELECT count(*) FROM chartworks.nlq_queries`) != plans || count(t, db, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("blocked generation created/executed a plan or retried", err)
			}
		}
	}
	t.Run("composed PlanAndRun cannot bypass", func(t *testing.T) {
		f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which metric?"))
		attempts := count(t, db, `SELECT count(*) FROM chartworks.read_attempts`)
		p, r, err := f.query.PlanAndRun(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue", nlq.LanguageEnglish), Operation: "decision-composed"}, nlqexec.RunRequest{Operation: "decision-composed"})
		if !errors.Is(err, nlqexec.ErrGenerationClarification) || p.QueryID != "" || r.QueryID != "" || count(t, db, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
			t.Fatal("composition bypass", err)
		}
	})
	t.Run("ready caveat and explicit fresh question", func(t *testing.T) {
		f.model.mode.Store(explanationRawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`, []string{"Reviewed units."}, []string{"No display order requested."}))
		p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue", nlq.LanguageEnglish)})
		if err != nil || p.QueryID == "" || len(p.Ambiguities) != 1 {
			t.Fatal("ready declaration did not plan", err)
		}
		out := f.run(t, p, 1, false)
		requireExplanationSum(t, out, "9007199254740998.625")
		calls := decisionChatCount(f)
		_, err = f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
		if err != nil || decisionChatCount(f) != calls {
			t.Fatal("replay regenerated readiness", err)
		}
		f.model.mode.Store(decisionRawResponse(t, generationdecision.Insufficient, "Which extra reviewed source is needed?"))
		if child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID}); child.QueryID != "" || !errors.Is(err, nlqexec.ErrGenerationContext) {
			t.Fatal("refinement invented a ready child", err)
		}
	})
	t.Run("validation repair may stop for clarification", func(t *testing.T) {
		f.model.mu.Lock()
		f.model.chatSequence = []string{phase18RawResponse(t, `SELECT avg(amount) AS revenue FROM analytics.sales`), decisionRawResponse(t, generationdecision.Clarify, "Which aggregate was intended?")}
		f.model.mu.Unlock()
		calls := decisionChatCount(f)
		attempts := count(t, db, `SELECT count(*) FROM chartworks.read_attempts`)
		p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue", nlq.LanguageEnglish)})
		if !errors.Is(err, nlqexec.ErrGenerationClarification) || p.QueryID != "" || decisionChatCount(f) != calls+2 || count(t, db, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
			t.Fatal("repair fabricated execution", err)
		}
	})
}
func TestSQLRecoveryDecisionTransportsPrivacyAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	q := populationPrivateQuestion(t, f)
	questions := []string{"Clarify cw-alpha-731, alias-secret-731 and primero?"}
	f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, questions...))
	payload, _ := json.Marshal(nlqexec.PlanRequest{QuestionRequest: q})
	handler := nlqapi.ExecutionHandler(f.model.token.verifier, f.query, http.NotFoundHandler())
	token := phase18Token(t, f.phase17Fixture, f.e.User(), f.e.Session(), false)
	request := httptest.NewRequest(http.MethodPost, "/v1/nlq/plans", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	var wire struct {
		Error      string          `json:"error"`
		Generation json.RawMessage `json:"generation"`
	}
	if w.Code != 422 || json.Unmarshal(w.Body.Bytes(), &wire) != nil || wire.Error != "generation_clarification_required" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("HTTP readiness", w.Code, w.Body.String())
	}
	p := sdk.DecodeGenerationProblem(wire.Generation)
	if p == nil || p.Questions[0] != "Clarify [redacted answer], [redacted answer] and [redacted answer]?" {
		t.Fatal("private values in decision response")
	}
	t.Run("same protected MCP path", func(t *testing.T) {
		bindings, err := nlqapi.ExecutionMCPBindings(f.query)
		if err != nil {
			t.Fatal(err)
		}
		registry, err := mcpserver.NewRegistry(bindings)
		if err != nil {
			t.Fatal(err)
		}
		server, err := mcpserver.New(f.model.token.verifier, registry, config.DefaultMCP(), nil)
		if err != nil {
			t.Fatal(err)
		}
		claims := f.model.token.claims(f.e.Tenant(), f.e.User(), append(f.e.Scopes(), "mcp.use"))
		claims["session"] = f.e.Session()
		claims["aud"] = f.model.token.cfg.MCPAudience()
		bearer := f.model.token.sign(t, claims, nil)
		client, err := server.Client(func(context.Context) (string, error) { return bearer, nil })
		if err != nil {
			t.Fatal(err)
		}
		result, err := client.CallTool(ctx, "plan_question", payload)
		if err != nil || result == nil || !result.IsError || len(result.Content) == 0 {
			t.Fatal("MCP decision", err)
		}
		text, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatal("MCP fault format")
		}
		var out struct {
			Error mcpserver.Fault `json:"error"`
		}
		if json.Unmarshal([]byte(text.Text), &out) != nil || out.Error.Generation == nil || out.Error.Code != wire.Error || strings.Contains(text.Text, "cw-alpha-731") || strings.Contains(text.Text, "alias-secret-731") {
			t.Fatal("MCP lost decision or privacy")
		}
	})
	t.Run("no reach no questions or model call", func(t *testing.T) {
		scopes := []string{"query.plan"}
		claims := f.model.token.claims(f.e.Tenant(), f.e.User(), scopes)
		claims["session"] = f.e.Session()
		bearer := f.model.token.sign(t, claims, nil)
		r := httptest.NewRequest(http.MethodPost, "/v1/nlq/plans", bytes.NewReader(payload))
		r.Header.Set("Authorization", "Bearer "+bearer)
		r.Header.Set("Content-Type", "application/json")
		before := f.model.requests.Load()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		if response.Code != 403 && response.Code != 404 {
			t.Fatal("missing reach admitted")
		}
		if strings.Contains(response.Body.String(), "generation-decision-v1") || f.model.requests.Load() != before {
			t.Fatal("unauthorized request acquired context questions")
		}
	})
}
