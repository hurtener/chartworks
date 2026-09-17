package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

// This corpus executes Chartworks services with PostgreSQL and recorded model
// responses. It is not a live provider quality or representative-user study.
func TestCW01(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	t.Run("AC01", func(t *testing.T) {
		before := f.model.requests.Load()
		missing := f.preflight(t, f.question("Show large sales", nlq.LanguageEnglish))
		if missing.Route.Outcome != nlq.StrategyClarify || missing.Route.Clarification == nil || len(missing.Route.Clarification.Questions) != 1 || f.model.requests.Load() != before {
			t.Fatal("matching blocker did not stop before provider")
		}
		unrelated := f.preflight(t, f.question("List all sales", nlq.LanguageEnglish))
		if unrelated.Route.Clarification != nil || unrelated.Route.Context == nil || f.model.requests.Load() <= before {
			t.Fatal("unrelated complete question interrupted")
		}
		for _, evaluation := range unrelated.Route.Clarifications {
			for _, slot := range evaluation.Slots {
				if slot.Outcome != semantics.ClarificationNotApplicable {
					t.Fatal("unrelated policy applied")
				}
			}
		}
	})
	t.Run("AC02", func(t *testing.T) {
		en := f.plan(t, f.question("Show dated sales", nlq.LanguageEnglish), "period", cw01Time("2026-01-02", "2026-01-03", "day"))
		es := f.plan(t, f.question("Mostrá ventas fechadas", nlq.LanguageSpanish), "period", cw01Time("2026-01-02", "2026-01-03", "day"))
		a, b := en.Route.Resolutions[0].Time, es.Route.Resolutions[0].Time
		if a == nil || !reflect.DeepEqual(a, b) || a.StartUTC != "2026-01-02T03:00:00Z" || a.EndUTC != "2026-01-03T03:00:00Z" || a.Bounds != "[)" {
			t.Fatal("bilingual time bounds lost calendar/zone/boundaries")
		}
		f.run(t, en, 1, true)
		f.run(t, es, 1, true)
		corrected := f.answer(t, "period", cw01Time("2026-01-03", "2026-01-04", "day"))
		child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: en.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{corrected}}})
		if err != nil {
			t.Fatal("date correction", err)
		}
		f.run(t, child, 1, false)
		t.Run("BilingualNamedPeriod", func(t *testing.T) {
			period := func(label string) semantics.ClarificationValue {
				return semantics.ClarificationValue{Time: &semantics.ClarificationTimeInput{Period: label, Calendar: "gregorian", TimeZone: "America/Argentina/Buenos_Aires", Grain: "month"}}
			}
			en := f.plan(t, f.question("Show dated sales", nlq.LanguageEnglish), "period", period("January 2026"))
			es := f.plan(t, f.question("Mostrá ventas fechadas", nlq.LanguageSpanish), "period", period("enero de 2026"))
			if !reflect.DeepEqual(en.Route.Resolutions[0].Time, es.Route.Resolutions[0].Time) {
				t.Fatal("different language month inputs changed canonical business window")
			}
			f.run(t, en, 2, true)
			f.run(t, es, 2, true)
		})
	})
	t.Run("AC03", func(t *testing.T) {
		cases := []struct {
			name, question, pattern string
			value                   semantics.ClarificationValue
		}{
			{"date", "Show dated sales", "period", cw01Time("2026-02-30", "2026-03-02", "day")},
			{"grain", "Show dated sales", "period", cw01Time("2026-01-02", "2026-01-03", "hour")},
			{"number", "Show large sales", "amount-required", cw01Number("NaN")},
			{"precision", "Show large sales", "amount-required", cw01Number("1.0001")},
			{"boolean", "Show active sales", "state", cw01Bool("perhaps")},
			{"choice", "Choose sales", "metric", semantics.ClarificationValue{OptionID: "foreign"}},
			{"nonreference-option", "Show large sales", "amount-required", semantics.ClarificationValue{OptionID: "10"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				q := f.question(tc.question, nlq.LanguageEnglish)
				p := f.preflight(t, q)
				q.AnswerContext = p.Route.AnswerContext
				q.ClarificationQuery = p.QueryID
				q.Answers = []semantics.ClarificationAnswer{f.answer(t, tc.pattern, tc.value)}
				before := f.model.requests.Load()
				out, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
				var failure *nlqroute.Clarification
				if !errors.As(err, &failure) || failure.Outcome != semantics.ClarificationInvalid || len(failure.Errors) == 0 || out.QueryID != "" || out.Bindings != nil || f.model.requests.Load() != before {
					t.Fatalf("invalid accepted or provider called: %v", err)
				}
			})
		}
	})
	t.Run("AC04", func(t *testing.T) {
		boolean := f.plan(t, f.question("Show active sales", nlq.LanguageSpanish), "state", cw01Bool("falso"))
		if boolean.Route.Resolutions[0].Value != "false" {
			t.Fatal("boolean not canonical")
		}
		f.run(t, boolean, 1, false)
		entity := f.plan(t, f.question("Show named sales", nlq.LanguageSpanish), "customer", cw01Text("primero"))
		r := entity.Route.Resolutions[0]
		if r.Value != "cw-alpha-731" || r.Effect == nil || r.Effect.Target.ID != "name" || r.Effect.Operator != "eq" || r.Effect.Nulls != "exclude" {
			t.Fatal("governed entity effect lost")
		}
		f.run(t, entity, 1, true)
		choice := f.plan(t, f.question("Choose sales", nlq.LanguageEnglish), "metric", semantics.ClarificationValue{OptionID: "revenue-option"})
		if choice.Route.Resolutions[0].Reference == nil || choice.Route.Resolutions[0].Reference.ID != "revenue" || len(choice.Route.Context.Metrics) != 1 {
			t.Fatal("reference choice not propagated")
		}
		q := f.question("Show named sales", nlq.LanguageEnglish)
		pending := f.preflight(t, q)
		q.AnswerContext = pending.Route.AnswerContext
		q.ClarificationQuery = pending.QueryID
		q.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("not-governed"))}
		before := f.model.requests.Load()
		out, err := f.query.Preflight(ctx, f.e, nlqexec.PreflightRequest{QuestionRequest: q})
		if err != nil || out.Route.Clarification == nil || f.model.requests.Load() != before {
			t.Fatal("unresolved text treated as a filter", err)
		}
	})
	t.Run("AC05", func(t *testing.T) {
		initial := f.plan(t, f.question("Show optional sales", nlq.LanguageEnglish), "amount-optional", cw01Number("10"))
		f.run(t, initial, 1, true)
		if len(initial.Route.Resolutions) != 1 || initial.Bindings.Validation == nil || initial.Route.Context.Constraints == nil {
			t.Fatal("resolution did not reach validated plan receipt")
		}
		scope, err := store.NewScope(f.e.Tenant(), f.e.User())
		if err != nil {
			t.Fatal(err)
		}
		stored, err := f.f.db.ReadQuery(ctx, scope, initial.QueryID)
		if err != nil || len(stored.Parameters) != 1 || stored.Clarification == nil || stored.Clarification.BaseSQL == stored.SQL {
			t.Fatal("canonical binding not persisted", err)
		}
		changed := f.answer(t, "amount-optional", cw01Number("1"))
		child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: initial.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{changed}}})
		if err != nil || len(child.AnswerChanges) != 1 || child.AnswerChanges[0].Action != "superseded" || child.AnswerChanges[0].Previous != initial.Route.Resolutions[0].ID {
			t.Fatal("replacement lineage lost", err)
		}
		f.run(t, child, 2, true)
		removed := changed
		removed.Value = nil
		removed.Remove = true
		clean, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: child.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{removed}}})
		if err != nil || len(clean.Route.Resolutions) != 0 || len(clean.AnswerChanges) != 1 || clean.AnswerChanges[0].Action != "removed" {
			t.Fatal("explicit removal lost", err)
		}
		f.run(t, clean, 2, true)
		replay, err := f.f.db.ReadQuery(ctx, scope, initial.QueryID)
		if err != nil || !reflect.DeepEqual(stored.Parameters, replay.Parameters) {
			t.Fatal("replacement mutated retained parent", err)
		}
		latest, err := f.f.db.ReadQuery(ctx, scope, clean.QueryID)
		if err != nil || len(latest.Parameters) != 0 || strings.Contains(latest.SQL, "cw_filter_") {
			t.Fatal("removed answer retained a stale bound filter", err)
		}
		raw, _ := json.Marshal(latest.Route.Request)
		if strings.Contains(string(raw), "Bearer") {
			t.Fatal("authority retained in replay request")
		}
		cw01BindingAcceptance(t, f)
	})
	t.Run("AC06", func(t *testing.T) { cw01OrderingAcceptance(t, f) })
	t.Run("AC07", func(t *testing.T) { cw01DefaultsAcceptance(t, f) })
	t.Run("AC08", func(t *testing.T) { cw01IsolationAcceptance(t) })
	t.Run("AC09", func(t *testing.T) { cw01BudgetPrivacyAcceptance(t, f) })
	t.Run("AC10", func(t *testing.T) {
		t.Run("migration", cw01MigrationAcceptance)
		t.Run("authoring", cw01AuthoringAcceptance)
		t.Run("consumers", cw01ConsumerAcceptance)
		t.Run("saved-query", TestSavedQuestionReplayable)
		t.Run("saved-selection-identity", TestSavedQuestionClarificationAndSelectionIdentity)
		t.Run("typed-saved-query", cw01TypedSavedQueryAcceptance)
	})

}
