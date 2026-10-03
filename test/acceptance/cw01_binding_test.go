package acceptance

import (
	"context"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

func cw01BindingAcceptance(t *testing.T, f *cw01Fixture) {
	t.Helper()
	ctx := context.Background()
	defer f.model.mode.Store(phase18RawResponse(t, "SELECT id, amount FROM analytics.sales ORDER BY id"))
	for _, tc := range []struct{ name, sql string }{
		{"disjunction", "SELECT id,amount FROM analytics.sales WHERE id=1 OR id=2 ORDER BY id"},
		{"reviewed-alias", "SELECT s.id,s.amount FROM analytics.sales AS s ORDER BY s.id"},
		{"existing-filter", "SELECT id,amount FROM analytics.sales WHERE amount>=0 ORDER BY id"},
		{"flat-cte", "WITH values AS (SELECT id,amount FROM analytics.sales) SELECT id,amount FROM values ORDER BY id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.model.mode.Store(phase18RawResponse(t, tc.sql))
			plan := f.plan(t, f.question("Show large sales", nlq.LanguageEnglish), "amount-required", cw01Number("10"))
			f.run(t, plan, 1, true)
		})
	}
	t.Run("unsupported-shape-no-read", func(t *testing.T) {
		f.model.mode.Store(phase18RawResponse(t, "WITH RECURSIVE values AS (SELECT id,amount FROM analytics.sales) SELECT id,amount FROM values"))
		q := f.question("Show large sales", nlq.LanguageEnglish)
		pending := f.preflight(t, q)
		q.AnswerContext = pending.Route.AnswerContext
		q.ClarificationQuery = pending.QueryID
		q.Answers = append(q.Answers, f.answer(t, "amount-required", cw01Number("10")))
		out, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
		if err == nil || out.QueryID != "" || out.Bindings != nil {
			t.Fatal("unsupported shape silently dropped its typed constraint")
		}
	})
}
