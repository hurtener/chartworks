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
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func ownedExampleQuestion(t *testing.T, f *cw01Fixture, text string, locale nlq.Language, answers ...semantics.ClarificationAnswer) nlqexec.QuestionRequest {
	t.Helper()
	q := f.question(text, locale)
	pending := f.preflight(t, q)
	q.ClarificationQuery, q.AnswerContext, q.Answers = pending.QueryID, pending.Route.AnswerContext, answers
	return q
}
func reviewOwnedExample(t *testing.T, f *cw01Fixture, p nlqexec.PlanResult) nlqexec.ExampleRecord {
	t.Helper()
	ctx := context.Background()
	if err := f.query.Feedback(ctx, f.e, nlqexec.FeedbackRequest{QueryID: p.QueryID, Verdict: "positive"}); err != nil {
		t.Fatal("owned template feedback", err)
	}
	examples, err := f.query.Examples(ctx, f.e, f.pack.Topic, 8)
	if err != nil || len(examples) != 1 || examples[0].Origin.BindingPolicy != nlqexec.OwnedExamplePolicy {
		t.Fatal("missing owned template origin", err)
	}
	metadata := support.Raw(t, f.f.dsn)
	attempts, calls := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`), f.model.requests.Load()
	active, err := f.query.ExampleState(ctx, f.e, nlqexec.ExampleStateRequest{ExampleID: examples[0].ID, State: "active", ExpectedVersion: examples[0].Version, ReviewNote: "Reviewed the value-free base and current-only predicate contract"})
	if err != nil || active.State != "active" || f.model.requests.Load() != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
		t.Fatal("native base review performed generation/execution or failed", err)
	}
	return active
}
func requireOwnedExampleUse(t *testing.T, f *cw01Fixture, p nlqexec.PlanResult, id string) nlqexec.QueryRecord {
	t.Helper()
	sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
	q, err := f.f.db.ReadQuery(context.Background(), sc, p.QueryID)
	if err != nil || q.ExampleSelection.Usage == nil || len(q.ExampleSelection.Usage.Used) != 1 || q.ExampleSelection.Usage.Used[0].ExampleID != id || q.Clarification == nil || q.Clarification.Binding.SchemaVersion != 1 {
		t.Fatal("current request did not use the reviewed base with fresh binding", err)
	}
	return q
}
func TestSQLRecoveryOwnedExamplePrivateLifecycleAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := newCW01Fixture(t)
			ctx := context.Background()
			text := "Revenue named sales"
			if locale == nlq.LanguageSpanish {
				text = "Revenue ventas por nombre"
			}
			base := `SELECT sum(amount) AS revenue FROM analytics.sales`
			f.model.mode.Store(phase18RawResponse(t, base))
			q := ownedExampleQuestion(t, f, text, locale, f.answer(t, "customer", cw01Text("primero")))
			p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
			if err != nil || p.Bindings == nil {
				t.Fatal("first private query", err)
			}
			requireExplanationSum(t, f.run(t, p, 1, true), "9007199254740993.125")
			active := reviewOwnedExample(t, f, p)
			sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
			row, err := f.f.db.ReadExample(ctx, sc, active.ID)
			if err != nil || row.SQL != base || row.ParameterSchema != nil {
				t.Fatal("base acquired historical service slots", err)
			}
			bundle, err := f.query.ExportExamples(ctx, f.e, nlqexec.ExampleExportRequest{Topic: f.pack.Topic, Limit: 8})
			if err != nil || bundle.SchemaVersion != 3 || len(bundle.Examples) != 1 || bundle.Examples[0].SchemaVersion != 3 {
				t.Fatal("policy portability", err)
			}
			raw, _ := json.Marshal(bundle)
			for _, secret := range []string{"cw-alpha-731", "alias-secret-731", "primero", "named sales", "ventas por nombre"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("historical question/predicate exported", secret)
				}
			}
			if err := f.query.Feedback(ctx, f.e, nlqexec.FeedbackRequest{QueryID: p.QueryID, Verdict: "positive"}); err != nil {
				t.Fatal("feedback replay", err)
			}
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			current := ownedExampleQuestion(t, f, text, locale, f.answer(t, "customer", cw01Text("segundo")))
			f.model.mu.Lock()
			start := len(f.model.requestBodies)
			f.model.mu.Unlock()
			child, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: current})
			if err != nil {
				t.Fatal("base reuse", err)
			}
			stored := requireOwnedExampleUse(t, f, child, active.ID)
			if len(stored.Parameters) != 1 || stored.Parameters[0].Value != "cw-beta-731" || stored.Clarification.BaseSQL != base || len(stored.Clarification.BaseParameters) != 0 {
				t.Fatal("historical customer restored or service slots entered base")
			}
			requireExplanationSum(t, f.run(t, child, 1, false), "5.5")
			f.model.mu.Lock()
			wire := strings.Join(f.model.requestBodies[start:], "\n")
			f.model.mu.Unlock()
			if !strings.Contains(wire, nlqexec.OwnedExamplePolicy) || strings.Contains(wire, "cw-alpha-731") || strings.Contains(wire, "alias-secret-731") || strings.Contains(wire, "cw-beta-731") {
				t.Fatal("reuse wire has historical/current private predicates")
			}
			metadata := support.Raw(t, f.f.dsn)
			for _, mutation := range []string{`origin=origin-'binding_policy'`, `origin=jsonb_set(origin,'{binding_policy}','"other-policy"')`, `sql_text=sql_text || ' WHERE false'`} {
				if _, err := metadata.Exec(ctx, `UPDATE chartworks.nlq_examples SET `+mutation+` WHERE example_id=$1`, active.ID); err == nil {
					t.Fatal("reviewed base policy/content mutable")
				}
			}
			// A typed schema cannot silently turn the marker into a source grant. Fresh
			// routing must resolve an actual owned predicate for consumption or import.
			plain, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue", locale)})
			if err != nil {
				t.Fatal(err)
			}
			unbound, err := f.f.db.ReadQuery(ctx, sc, plain.QueryID)
			if err != nil || unbound.ExampleSelection.Usage != nil && len(unbound.ExampleSelection.Usage.Used) != 0 {
				t.Fatal("owned template used without current predicates", err)
			}
			found := false
			for _, excluded := range unbound.ExampleSelection.Excluded {
				if excluded.ExampleID == active.ID && excluded.Reason == "current_owned_predicates_required" {
					found = true
				}
			}
			if !found {
				t.Fatal("missing explicit current-predicate exclusion")
			}
			portable := bundle.Examples[0]
			portable.SQL += " "
			portable.Digest = readexec.Hash([]any{nlqexec.OwnedExamplePolicy, f.pack.Topic, portable.Question, portable.SQL, portable.ParameterSchema})
			if _, err := f.query.ImportExample(ctx, f.e, nlqexec.ExampleImportRequest{Anchor: f.question("Revenue", locale), Example: portable}); !errors.Is(err, readexec.ErrBinding) {
				t.Fatal("import acquired nonexistent predicates", err)
			}
			imported, err := f.query.ImportExample(ctx, f.e, nlqexec.ExampleImportRequest{Anchor: current, Example: portable})
			if err != nil || imported.State != "candidate" || imported.Origin.BindingPolicy != nlqexec.OwnedExamplePolicy {
				t.Fatal("protected value-free import", err)
			}
			portable.Origin.BindingPolicy = ""
			if _, err := f.query.ImportExample(ctx, f.e, nlqexec.ExampleImportRequest{Anchor: current, Example: portable}); !errors.Is(err, nlqexec.ErrInvalid) {
				t.Fatal("portable policy downgrade accepted", err)
			}
			before := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			calls := f.model.requests.Load()
			replay, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
			if err != nil || f.model.requests.Load() != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before {
				t.Fatal("replay relearned/requeried", err)
			}
			requireExplanationSum(t, replay, "5.5")
		})
	}
}
func TestSQLRecoveryOwnedExampleModelSlotsAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	base := `SELECT id FROM analytics.sales WHERE amount>$1 ORDER BY id`
	q := ownedExampleQuestion(t, f, "List IDs for named sales", nlq.LanguageEnglish, f.answer(t, "customer", cw01Text("primero")))
	f.model.mode.Store(parameterResponse(t, base, []readexec.Parameter{{Kind: "number", Value: "5.125"}}))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil {
		t.Fatal(err)
	}
	active := reviewOwnedExample(t, f, p)
	if active.ParameterSchema == nil || len(active.ParameterSchema.Slots) != 1 || active.ParameterSchema.Slots[0].Kind != "number" {
		t.Fatal("base slots lost or owned slot leaked")
	}
	q = ownedExampleQuestion(t, f, "List IDs for named sales", nlq.LanguageEnglish, f.answer(t, "customer", cw01Text("segundo")))
	f.model.mode.Store(parameterResponse(t, base, []readexec.Parameter{{Kind: "number", Value: "1.25"}}))
	current, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil {
		t.Fatal(err)
	}
	record := requireOwnedExampleUse(t, f, current, active.ID)
	if len(record.Parameters) != 2 || record.Parameters[0].Value != "1.25" || record.Parameters[1].Value != "cw-beta-731" || len(record.Clarification.BaseParameters) != 1 {
		t.Fatal("mixed historical/service/model parameter ownership")
	}
	out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: current.QueryID, Operation: current.QueryID + "-run"})
	if err != nil {
		t.Fatal(err)
	}
	requireParameterIDs(t, out, "2")
}
func TestSQLRecoveryOwnedExampleCalendarAndBooleanAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	if _, err := f.f.admin.Exec(ctx, `UPDATE analytics.sales SET created_at=CASE id WHEN 1 THEN '2025-01-15T12:00:00Z'::timestamptz ELSE '2025-02-15T12:00:00Z'::timestamptz END`); err != nil {
		t.Fatal(err)
	}
	base := `SELECT sum(amount) AS revenue FROM analytics.sales`
	f.model.mode.Store(phase18RawResponse(t, base))
	q := ownedExampleQuestion(t, f, "Revenue dated sales", nlq.LanguageEnglish, f.answer(t, "period", cw01Time("2025-01-01", "2025-02-01", "month")))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil {
		t.Fatal("first calendar", err)
	}
	active := reviewOwnedExample(t, f, p)
	if active.ParameterSchema != nil {
		t.Fatal("historical time bounds became template slots")
	}
	f.query, _ = newPhase18Service(t, f.phase17Fixture)
	q = ownedExampleQuestion(t, f, "Revenue dated sales", nlq.LanguageEnglish, f.answer(t, "period", cw01Time("2025-02-01", "2025-03-01", "month")))
	p, err = f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil {
		t.Fatal("changed calendar", err)
	}
	current := requireOwnedExampleUse(t, f, p, active.ID)
	if len(current.Parameters) != 2 || strings.HasPrefix(current.Parameters[0].Value, "2025-01") {
		t.Fatal("inherited historical interval")
	}
	requireExplanationSum(t, f.run(t, p, 1, false), "5.5")
	q = ownedExampleQuestion(t, f, "Revenue active sales", nlq.LanguageEnglish, f.answer(t, "state", cw01Bool("false")))
	p, err = f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil {
		t.Fatal("current boolean", err)
	}
	current = requireOwnedExampleUse(t, f, p, active.ID)
	if len(current.Parameters) != 1 || current.Parameters[0].Kind != "boolean" || current.Parameters[0].Value != "false" {
		t.Fatal("template dictated a predicate's historical type/value")
	}
	requireExplanationSum(t, f.run(t, p, 1, false), "5.5")
	q = ownedExampleQuestion(t, f, "Revenue large sales", nlq.LanguageEnglish, f.answer(t, "amount-required", cw01Number("6")))
	p, err = f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil {
		t.Fatal("current numeric threshold", err)
	}
	current = requireOwnedExampleUse(t, f, p, active.ID)
	if len(current.Parameters) != 1 || current.Parameters[0].Kind != "text" || current.Parameters[0].Value != "6" {
		t.Fatal("template dictated old numeric threshold")
	}
	requireExplanationSum(t, f.run(t, p, 1, true), "9007199254740993.125")
	bundle, err := f.query.ExportExamples(ctx, f.e, nlqexec.ExampleExportRequest{Topic: f.pack.Topic, Limit: 8})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(bundle)
	if strings.Contains(string(raw), "2025-") || strings.Contains(string(raw), "America/Argentina") || strings.Contains(string(raw), `"value":`) {
		t.Fatal("calendar/default values in portable templates")
	}
}
func TestSQLRecoveryOwnedExampleCorrectedBoundFeedbackIsNotTemplate(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	base := `SELECT sum(amount) AS revenue FROM analytics.sales`
	q := populationPrivateQuestion(t, f)
	f.model.mode.Store(phase18RawResponse(t, base))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.query.Feedback(ctx, f.e, nlqexec.FeedbackRequest{QueryID: p.QueryID, Verdict: "positive", Correction: p.SQL}); err != nil {
		t.Fatal("review feedback", err)
	}
	examples, err := f.query.Examples(ctx, f.e, f.pack.Topic, 8)
	if err != nil || len(examples) != 0 {
		t.Fatal("manual bound SQL pretended to have an authenticated base", err)
	}
	metadata := support.Raw(t, f.f.dsn)
	if count(t, metadata, `SELECT count(*) FROM chartworks.nlq_feedback`) != 1 {
		t.Fatal("ineligible base discarded user's feedback")
	}
}
