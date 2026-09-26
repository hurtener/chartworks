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
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// Real source validation, PostgreSQL storage/review and recorded provider wire.
// Historical bindings must not become the next question's model context/values.
func TestSQLRecoveryParameterizedExampleLifecycleAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := newCW01Fixture(t)
			ctx := context.Background()
			private := "9007199254740993.12"
			question := "List record IDs with amount above " + private
			nextQuestion := "List record IDs with amount above 5.125"
			if locale == nlq.LanguageSpanish {
				question = "Listar identificadores con importe superior a " + private
				nextQuestion = "Listar identificadores con importe superior a 5.125"
			}
			sql := `SELECT id FROM analytics.sales WHERE amount>$1 ORDER BY id`
			f.model.mode.Store(parameterResponse(t, sql, []readexec.Parameter{{Kind: "number", Value: private}}))
			parent, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question(question, locale)})
			if err != nil || parent.QueryID == "" {
				t.Fatal("source query", err)
			}
			result, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: parent.QueryID, Operation: parent.QueryID + "-run"})
			if err != nil {
				t.Fatal(err)
			}
			requireParameterIDs(t, result, "1")
			if err := f.query.Feedback(ctx, f.e, nlqexec.FeedbackRequest{QueryID: parent.QueryID, Verdict: "positive"}); err != nil {
				t.Fatal("parameterized feedback", err)
			}
			// Keep the established bounded review contract: the newly typed
			// payload does not expand pagination or bypass argument validation.
			if _, err := f.query.Examples(ctx, f.e, f.pack.Topic, 16); !errors.Is(err, nlqexec.ErrInvalid) {
				t.Fatal("typed learning bypassed list limit", err)
			}
			examples, err := f.query.Examples(ctx, f.e, f.pack.Topic, nlq.MaxExamples+1)
			if err != nil || len(examples) != 1 || examples[0].ParameterSchema == nil || examples[0].ParameterSchema.Slots[0].Kind != "number" || examples[0].ParameterSchema.Slots[0].Position != 1 || strings.Contains(examples[0].Question, private) {
				t.Fatal("template lost schema or retained value", err)
			}
			candidate := examples[0]
			metadata := support.Raw(t, f.f.dsn)
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			calls := f.model.requests.Load()
			active, err := f.query.ExampleState(ctx, f.e, nlqexec.ExampleStateRequest{ExampleID: candidate.ID, State: "active", ExpectedVersion: candidate.Version, ReviewNote: "Reviewed abstract amount predicate"})
			if err != nil || active.State != "active" || active.ParameterSchema == nil || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts || f.model.requests.Load() != calls {
				t.Fatal("typed native review executed a query/model or failed", err)
			}
			if _, err := f.query.ExportExamples(ctx, f.e, nlqexec.ExampleExportRequest{Topic: f.pack.Topic, Limit: 16}); !errors.Is(err, nlqexec.ErrInvalid) {
				t.Fatal("typed learning bypassed export limit", err)
			}
			bundle, err := f.query.ExportExamples(ctx, f.e, nlqexec.ExampleExportRequest{Topic: f.pack.Topic, Limit: nlq.MaxExamples + 1})
			if err != nil || bundle.SchemaVersion != 2 || len(bundle.Examples) != 1 || bundle.Examples[0].SchemaVersion != 2 || bundle.Examples[0].ParameterSchema == nil {
				t.Fatal("portable typed example", err)
			}
			raw, _ := json.Marshal(bundle)
			if strings.Contains(string(raw), private) || strings.Contains(string(raw), `"value":`) {
				t.Fatal("binding value exported")
			}
			// Service reconstruction exercises the persisted immutable schema, not an
			// in-memory slot cache. The subsequent SQL is bound to the NEW question.
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			f.model.mu.Lock()
			start := len(f.model.requestBodies)
			f.model.mu.Unlock()
			f.model.mode.Store(parameterResponse(t, sql, []readexec.Parameter{{Kind: "number", Value: "5.125"}}))
			child, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question(nextQuestion, locale)})
			if err != nil || child.QueryID == "" {
				t.Fatal("template reuse", err)
			}
			sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
			stored, err := f.f.db.ReadQuery(ctx, sc, child.QueryID)
			if err != nil || !reflect.DeepEqual(stored.Parameters, []readexec.Parameter{{Kind: "number", Value: "5.125"}}) || stored.ExampleSelection.Usage == nil || len(stored.ExampleSelection.Usage.Used) != 1 || stored.ExampleSelection.Usage.Used[0].ExampleID != active.ID {
				t.Fatal("actual typed example use/current binding", err)
			}
			result, err = f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
			if err != nil {
				t.Fatal(err)
			}
			requireParameterIDs(t, result, "1", "2")
			f.model.mu.Lock()
			wire := strings.Join(f.model.requestBodies[start:], "\n")
			f.model.mu.Unlock()
			if strings.Contains(wire, private) || !strings.Contains(wire, "parameter_schema") || !strings.Contains(wire, "example-parameters-v1") {
				t.Fatal("provider demonstration missing types or contains old bindings")
			}
			// Import remains a separately reviewed candidate. Whitespace is part of the
			// exact SQL template digest, so this creates a distinct portable example.
			row := bundle.Examples[0]
			row.SQL += " "
			row.Digest = readexec.Hash([]any{"parameterized-example-v1", f.pack.Topic, row.Question, row.SQL, row.ParameterSchema})
			imported, err := f.query.ImportExample(ctx, f.e, nlqexec.ExampleImportRequest{Anchor: f.question(nextQuestion, locale), Example: row})
			if err != nil || imported.State != "candidate" || imported.ParameterSchema == nil {
				t.Fatal("typed import", err)
			}
			missing := row
			missing.ParameterSchema = nil
			if _, err := f.query.ImportExample(ctx, f.e, nlqexec.ExampleImportRequest{Anchor: f.question(nextQuestion, locale), Example: missing}); !errors.Is(err, nlqexec.ErrInvalid) {
				t.Fatal("portable schema downgrade", err)
			}
			for _, mutation := range []string{`parameter_schema=NULL`, `parameter_schema=jsonb_set(parameter_schema,'{slots,0,kind}','"text"')`, `parameter_schema=jsonb_set(parameter_schema,'{slots,0,value}','"private-default"')`} {
				if _, err := metadata.Exec(ctx, `UPDATE chartworks.nlq_examples SET `+mutation+` WHERE example_id=$1`, active.ID); err == nil {
					t.Fatal("reviewed parameter shape mutable")
				}
			}
			for _, invalid := range []string{`{"version":"example-parameters-v1","slots":[]}`, `{"version":"example-parameters-v1","slots":[{"position":1,"kind":"number","value":"3"}]}`, `{"version":"example-parameters-v1","slots":[{"position":2,"kind":"text"}]}`, `null`} {
				var valid bool
				if err := metadata.QueryRow(ctx, `SELECT chartworks.valid_example_parameters($1::jsonb)`, invalid).Scan(&valid); err != nil || valid {
					t.Fatal("invalid database slot schema", err)
				}
			}
		})
	}
}

func TestSQLRecoveryParameterizedExampleNativeReviewFailure(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	sql := `SELECT id FROM analytics.sales WHERE id=$1::integer`
	f.model.mode.Store(parameterResponse(t, sql, []readexec.Parameter{{Kind: "text", Value: "2"}}))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Find one record", nlq.LanguageEnglish)})
	if err != nil {
		t.Fatal("original valid cast", err)
	}
	if err := f.query.Feedback(ctx, f.e, nlqexec.FeedbackRequest{QueryID: p.QueryID, Verdict: "positive"}); err != nil {
		t.Fatal(err)
	}
	rows, err := f.query.Examples(ctx, f.e, f.pack.Topic, 8)
	if err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	before := f.model.requests.Load()
	if _, err := f.query.ExampleState(ctx, f.e, nlqexec.ExampleStateRequest{ExampleID: rows[0].ID, State: "active", ReviewNote: "Do not bypass dry validation"}); err == nil {
		t.Fatal("invalid public text probe bypassed native cast validation")
	}
	sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
	stored, err := f.f.db.ReadExample(ctx, sc, rows[0].ID)
	if err != nil || stored.State != "candidate" || f.model.requests.Load() != before {
		t.Fatal("failed review promoted example or used model", err)
	}
}

func TestSQLRecoveryLearningDoesNotCopyOwnedOrAnnotatedPredicates(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	q := populationPrivateQuestion(t, f)
	f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) FROM analytics.sales`))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.query.Feedback(ctx, f.e, nlqexec.FeedbackRequest{QueryID: p.QueryID, Verdict: "positive"}); err != nil {
		t.Fatal(err)
	}
	examples, err := f.query.Examples(ctx, f.e, f.pack.Topic, 8)
	if err != nil || len(examples) != 0 {
		t.Fatal("service-owned private predicate became demonstration", err)
	}
	// Valid SQL can carry an annotation with a private value. Record feedback,
	// but refuse to publish that text as an automatically proposed example.
	f.model.mode.Store(parameterResponse(t, `SELECT id FROM analytics.sales WHERE amount>$1 /* private-annotation */`, []readexec.Parameter{{Kind: "number", Value: "5.25"}}))
	p, err = f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("List sales records", nlq.LanguageEnglish)})
	if err != nil {
		t.Fatal("native-valid annotation", err)
	}
	if err := f.query.Feedback(ctx, f.e, nlqexec.FeedbackRequest{QueryID: p.QueryID, Verdict: "positive"}); err != nil {
		t.Fatal("feedback unnecessarily blocked", err)
	}
	examples, err = f.query.Examples(ctx, f.e, f.pack.Topic, 8)
	if err != nil || len(examples) != 0 {
		t.Fatal("annotated bound SQL was proposed for reuse", err)
	}
}
