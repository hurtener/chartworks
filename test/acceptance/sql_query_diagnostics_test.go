package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/exec/querydiagnostic"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/generationdecision"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func diagnosticRequestText(t *testing.T, body string) string {
	t.Helper()
	var request struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal([]byte(body), &request) != nil {
		t.Fatal("invalid captured provider request")
	}
	var messages []string
	for _, m := range request.Messages {
		messages = append(messages, m.Content)
	}
	return strings.Join(messages, "\n")
}
func TestSQLRecoveryQueryDiagnosticNativeValidationAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := newCW01Fixture(t)
			ctx := context.Background()
			metadata := support.Raw(t, f.f.dsn)
			question := "Show reviewed records"
			if locale == nlq.LanguageSpanish {
				question = "Mostrar registros revisados"
			}
			good := `SELECT id FROM analytics.sales ORDER BY id`
			for _, tc := range []struct{ sql, code string }{{`SELECT lower(id) FROM analytics.sales`, "query_function_signature"}, {`SELECT coalesce(name,id) FROM analytics.sales`, "query_type_mismatch"}, {`SELECT name,sum(amount) FROM analytics.sales`, "query_grouping"}} {
				t.Run(tc.code, func(t *testing.T) {
					_, err := f.f.validator.Validate(ctx, f.e, readexec.Request{Source: f.pack.Datasets[0].Source.Source, Context: f.context, SQL: tc.sql})
					if readexec.QueryRejectionCode(err) != tc.code {
						t.Fatal("real EXPLAIN rejected for unexpected category", tc.code, err)
					}
					before := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
					calls := adversarialChatCount(f)
					f.model.mu.Lock()
					start := len(f.model.requestBodies)
					f.model.chatSequence = []string{phase18RawResponse(t, tc.sql), phase18RawResponse(t, good)}
					f.model.mu.Unlock()
					p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question(question, locale)})
					if err != nil || p.ValidationFixes != 1 || p.SQL != good || p.QueryID == "" || adversarialChatCount(f) != calls+2 || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before {
						t.Fatal("native diagnostic correction/budget", tc.code, err)
					}
					f.model.mu.Lock()
					bodies := append([]string(nil), f.model.requestBodies[start:]...)
					f.model.mu.Unlock()
					found := false
					for _, body := range bodies {
						wire := diagnosticRequestText(t, body)
						if strings.Contains(wire, tc.code) && strings.Contains(wire, "diagnostic_guidance") {
							if !strings.Contains(wire, querydiagnostic.Hint(tc.code)) || !strings.Contains(wire, tc.sql) {
								t.Fatal("specific diagnostic omitted its safe guidance or rejected statement")
							}
							found = true
						}
					}
					if !found {
						t.Fatal("specific diagnostic did not reach actual provider wire")
					}
					r, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
					if err != nil {
						t.Fatal(err)
					}
					requireParameterIDs(t, r, "1", "2")
					calls = adversarialChatCount(f)
					reads := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
					r, err = f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
					if err != nil || adversarialChatCount(f) != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != reads {
						t.Fatal("diagnostic replay did extra work", err)
					}
					requireParameterIDs(t, r, "1", "2")
				})
			}
		})
	}
}
func TestSQLRecoveryQueryDiagnosticPrivateBindingsAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	values := []readexec.Parameter{{Kind: "text", Value: "cw-alpha-731"}}
	bad := `SELECT CAST(CAST($1 AS text) AS integer) FROM analytics.sales`
	good := `SELECT id FROM analytics.sales WHERE name=$1 ORDER BY id`
	_, cause := f.f.validator.Validate(ctx, f.e, readexec.Request{Source: f.pack.Datasets[0].Source.Source, Context: f.context, SQL: bad, Parameters: values})
	if readexec.QueryRejectionCode(cause) != "query_invalid_text" {
		t.Fatal("real invalid-text boundary", cause)
	}
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.chatSequence = []string{parameterResponse(t, bad, values), parameterResponse(t, good, []readexec.Parameter{{Kind: "text", Value: "model-placeholder"}})}
	f.model.mu.Unlock()
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Show reviewed records", nlq.LanguageEnglish)})
	if err != nil || p.ValidationFixes != 1 {
		t.Fatal("private binding correction", err)
	}
	f.model.mu.Lock()
	bodies := append([]string(nil), f.model.requestBodies[start:]...)
	f.model.mu.Unlock()
	found := false
	for _, body := range bodies {
		wire := diagnosticRequestText(t, body)
		if strings.Contains(wire, "cw-alpha-731") || strings.Contains(wire, "alias-secret-731") || strings.Contains(wire, "invalid input syntax for type integer") {
			t.Fatal("native source message/binding reached provider")
		}
		found = found || strings.Contains(wire, "query_invalid_text")
	}
	if !found {
		t.Fatal("specific invalid-text diagnostic missing")
	}
	sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
	q, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
	if err != nil || !reflect.DeepEqual(q.Parameters, values) {
		t.Fatal("private bindings replaced by model placeholders", err)
	}
	r, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
	if err != nil {
		t.Fatal(err)
	}
	requireParameterIDs(t, r, "1")
}
func TestSQLRecoveryQueryDiagnosticPhysicalReceiptAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	metadata := support.Raw(t, f.f.dsn)
	statement := `SELECT id,amount/(id-id) AS amount FROM analytics.sales`
	f.model.mode.Store(phase18RawResponse(t, statement))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Show reviewed records", nlq.LanguageEnglish)})
	if err != nil || p.QueryID == "" {
		t.Fatal("runtime rejection fixture", err)
	}
	f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which reviewed zero-denominator policy applies?"))
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.mu.Unlock()
	before := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	calls := adversarialChatCount(f)
	r, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
	if !errors.Is(err, nlqexec.ErrGenerationClarification) || r.Status != "failed" || r.Execution.Attempt.Code != "query_division_by_zero" || r.Execution.Attempt.RemoteState != "stopped" || r.Execution.Attempt.Finished == nil || r.Execution.Result != nil || adversarialChatCount(f) != calls+1 || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before+1 {
		t.Fatal("confirmed rejection/blocked correction receipt", err)
	}
	var code, status, remote string
	if err := metadata.QueryRow(ctx, `SELECT code,status,remote_state FROM chartworks.read_attempts WHERE attempt_id=$1`, r.Execution.Attempt.ID).Scan(&code, &status, &remote); err != nil || code != "query_division_by_zero" || status != "failed" || remote != "stopped" {
		t.Fatal("specific diagnostic did not survive journal", err)
	}
	f.model.mu.Lock()
	bodies := append([]string(nil), f.model.requestBodies[start:]...)
	f.model.mu.Unlock()
	found := false
	for _, body := range bodies {
		wire := diagnosticRequestText(t, body)
		found = found || strings.Contains(wire, querydiagnostic.Hint("query_division_by_zero"))
		if strings.Contains(wire, "22012") {
			t.Fatal("raw SQLSTATE leaked into provider guidance")
		}
	}
	if !found {
		t.Fatal("runtime correction missing safe guidance")
	}
	sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
	q, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
	if err != nil || q.SQL != statement || q.Result != nil || q.Status != "failed" {
		t.Fatal("blocked correction replaced accepted state", err)
	}
	projected, err := f.f.db.ReadSavedQuery(ctx, f.e, p.QueryID, false)
	if err != nil || projected.Result != nil {
		t.Fatal("saved diagnostic row privacy", err)
	}
	replay, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
	if err == nil || replay.Status != "failed" || replay.Execution.Result != nil || adversarialChatCount(f) != calls+1 || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before+1 {
		t.Fatal("terminal diagnostic replay performed new work", err)
	}
	// Exercise the new closed DB enum on a fresh row, not only through the
	// immutable-finalized-row trigger. Raw text can never occupy this journal.
	if _, err := metadata.Exec(ctx, `INSERT INTO chartworks.read_attempts
 (tenant_id,actor_id,attempt_id,operation_id,attempt_number,source_id,context_id,manifest,manifest_hash,status,remote_state,created_at,deadline,finished_at,rows_returned,bytes_returned,code)
 SELECT tenant_id,actor_id,repeat('f',32),'invalid-diagnostic-fixture',1,source_id,context_id,manifest,manifest_hash,'failed','stopped',created_at,deadline,finished_at,0,0,'query_error PRIVATE_SOURCE_VALUE'
 FROM chartworks.read_attempts WHERE attempt_id=$1`, r.Execution.Attempt.ID); err == nil {
		t.Fatal("raw diagnostic accepted into journal")
	}
}

func TestSQLRecoveryQueryDiagnosticDataAndWindowMatrix(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	for _, tc := range []struct{ sql, code string }{
		{`SELECT CAST(9223372036854775807 AS bigint)+1 FROM analytics.sales`, "query_numeric_range"},
		{`SELECT CAST('not_a_civil_date' AS date) FROM analytics.sales`, "query_invalid_datetime"},
		{`SELECT CAST('2026-02-30' AS date) FROM analytics.sales`, "query_datetime_range"},
		{`SELECT id FROM analytics.sales WHERE row_number() OVER ()=1`, "query_windowing"},
	} {
		_, err := f.f.validator.Validate(ctx, f.e, readexec.Request{Source: f.pack.Datasets[0].Source.Source, Context: f.context, SQL: tc.sql})
		if readexec.QueryRejectionCode(err) != tc.code {
			t.Fatal("native diagnostic matrix", tc.code, err)
		}
		if strings.Contains(err.Error(), "not_a_civil_date") || strings.Contains(err.Error(), "2026-02-30") {
			t.Fatal("native conversion detail leaked")
		}
	}
	// Cardinality failure is row-dependent, so EXPLAIN succeeds while execution
	// fails. Stopping for clarification must not mask it with an arbitrary LIMIT.
	statement := `SELECT (SELECT id FROM analytics.sales) AS value FROM analytics.sales`
	f.model.mode.Store(phase18RawResponse(t, statement))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Show reviewed records", nlq.LanguageEnglish)})
	if err != nil {
		t.Fatal("cardinality plan", err)
	}
	f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which reviewed scalar selection is intended?"))
	r, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
	if !errors.Is(err, nlqexec.ErrGenerationClarification) || r.Execution.Attempt.Code != "query_cardinality" || r.Execution.Attempt.RemoteState != "stopped" || r.Execution.Result != nil {
		t.Fatal("physical cardinality diagnostic", err)
	}
}
