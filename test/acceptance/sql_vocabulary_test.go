package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func requireVocabularyWire(t *testing.T, f *cw01Fixture, start int, roles ...string) {
	t.Helper()
	guidance, err := sqlpolicy.Guidance("postgres")
	if err != nil {
		t.Fatal(err)
	}
	f.model.mu.Lock()
	bodies := append([]string(nil), f.model.requestBodies[start:]...)
	f.model.mu.Unlock()
	var seen []string
	for _, body := range bodies {
		var packet struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				Type       string `json:"type"`
				JSONSchema struct {
					Name   string          `json:"name"`
					Strict bool            `json:"strict"`
					Schema json.RawMessage `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if json.Unmarshal([]byte(body), &packet) != nil {
			t.Fatal("bad provider JSON")
		}
		role := ""
		for _, candidate := range []string{"sqlgen", "sqlfix"} {
			if packet.Model == f.model.cfg.Roles[candidate].Model {
				role = candidate
			}
		}
		if role == "" {
			continue
		}
		if len(packet.Messages) != 2 || packet.Messages[0].Role != "system" || strings.Count(packet.Messages[0].Content, guidance) != 1 || packet.Messages[1].Role != "user" {
			t.Fatal("wire omitted/replaced server-owned vocabulary")
		}
		var schema struct {
			Type                 string                     `json:"type"`
			AdditionalProperties *bool                      `json:"additionalProperties"`
			Properties           map[string]json.RawMessage `json:"properties"`
			Required             []string                   `json:"required"`
		}
		response := packet.ResponseFormat
		if response.Type != "json_schema" || response.JSONSchema.Name != "nlq_sql_candidate" || !response.JSONSchema.Strict || json.Unmarshal(response.JSONSchema.Schema, &schema) != nil || schema.Type != "object" || schema.AdditionalProperties == nil || *schema.AdditionalProperties {
			t.Fatal("schema boundary lost")
		}
		for _, field := range []string{"decision", "questions", "sql", "parameters", "assumptions", "ambiguities"} {
			required := false
			for _, key := range schema.Required {
				required = required || key == field
			}
			if len(schema.Properties[field]) == 0 || !required {
				t.Fatal("generation schema lost a required field", field)
			}
		}
		seen = append(seen, role)
	}
	if strings.Join(seen, ",") != strings.Join(roles, ",") {
		t.Fatal("unexpected provider roles/calls", seen)
	}
}

func TestSQLRecoveryVocabularyProviderAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := newCW01Fixture(t)
			ctx := context.Background()
			question := "Inspect records"
			if locale == nlq.LanguageSpanish {
				question = "Consultar registros"
			}
			bad := `SELECT md5(name) AS item FROM analytics.sales`
			good := `SELECT id,abs(amount) AS amount FROM analytics.sales ORDER BY id`
			f.model.mu.Lock()
			start := len(f.model.requestBodies)
			f.model.chatSequence = []string{phase18RawResponse(t, bad), phase18RawResponse(t, good)}
			f.model.mu.Unlock()
			p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question(question, locale)})
			if err != nil || p.QueryID == "" || p.ValidationFixes != 1 || p.SQL != good {
				t.Fatal("vocabulary-aware correction", err)
			}
			requireVocabularyWire(t, f, start, "sqlgen", "sqlfix")
			calls := f.model.requests.Load()
			out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
			if err != nil || out.Execution.Result == nil || len(out.Execution.Result.Rows) != 2 || f.model.requests.Load() != calls {
				t.Fatal("native execution", err)
			}
			for i, row := range out.Execution.Result.Rows {
				if len(row) != 2 {
					t.Fatal("wrong source projection")
				}
				var id, value string
				if json.Unmarshal(row[0], &id) != nil || id != []string{"1", "2"}[i] || json.Unmarshal(row[1], &value) != nil {
					t.Fatal("wrong source row")
				}
				got, ok := new(big.Rat).SetString(value)
				want, _ := new(big.Rat).SetString([]string{"9007199254740993.125", "5.5"}[i])
				if !ok || got.Cmp(want) != 0 {
					t.Fatal("name policy changed numeric results")
				}
			}
			metadata := support.Raw(t, f.f.dsn)
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			replay, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
			if err != nil || f.model.requests.Load() != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts || readexec.Hash(replay.Execution.Result.Rows) != readexec.Hash(out.Execution.Result.Rows) {
				t.Fatal("retained query reinterpreted by vocabulary", err)
			}
			sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
			record, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
			if err != nil || record.SQL != good {
				t.Fatal("durable SQL changed", err)
			}
		})
	}
}

func TestSQLRecoveryVocabularyKeepsStrongerProofs(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	metadata := support.Raw(t, f.f.dsn)
	t.Run("allowed function is not correct metric", func(t *testing.T) {
		f.model.mode.Store(phase18RawResponse(t, `SELECT avg(amount) AS revenue FROM analytics.sales`))
		before := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
		p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue", nlq.LanguageEnglish)})
		if p.QueryID != "" || !errors.Is(err, readexec.ErrAnalyticalMismatch) || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before {
			t.Fatal("vocabulary replaced analytical conformance", err)
		}
	})
	t.Run("private reviewed predicate remains server-owned", func(t *testing.T) {
		q := populationPrivateQuestion(t, f)
		f.model.mu.Lock()
		start := len(f.model.requestBodies)
		f.model.mu.Unlock()
		f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`))
		p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
		if err != nil || p.Bindings == nil || p.Analytical == nil {
			t.Fatal("owned predicate plan", err)
		}
		requireVocabularyWire(t, f, start, "sqlgen")
		out := f.run(t, p, 1, true)
		if out.Analytical == nil {
			t.Fatal("lost active analytical proof")
		}
		f.model.mu.Lock()
		wire := strings.Join(f.model.requestBodies[start:], "\n")
		f.model.mu.Unlock()
		for _, private := range []string{"cw-alpha-731", "alias-secret-731"} {
			if strings.Contains(wire, private) {
				t.Fatal("private binding entered capability guidance/request")
			}
		}
		// A zero-permission envelope cannot gain query action from the vocabulary.
		revoked := f.f.token.envelope(t, f.e.Tenant(), f.e.User())
		calls := f.model.requests.Load()
		if denied, err := f.query.Plan(ctx, revoked, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue", nlq.LanguageEnglish)}); err == nil || denied.QueryID != "" || calls != f.model.requests.Load() {
			t.Fatal("profile substituted for action authority")
		}
	})
}
