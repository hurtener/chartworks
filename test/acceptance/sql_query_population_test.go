package acceptance

import (
	"context"
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

func populationPrivateQuestion(t *testing.T, f *cw01Fixture) nlqexec.QuestionRequest {
	t.Helper()
	q := f.question("Revenue named sales", nlq.LanguageEnglish)
	pending := f.preflight(t, q)
	q.ClarificationQuery = pending.QueryID
	q.AnswerContext = pending.Route.AnswerContext
	q.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("primero"))}
	return q
}

func requirePopulationProof(t *testing.T, p nlqexec.PlanResult, err error, fixes int) {
	t.Helper()
	if err == nil && p.Analytical != nil && p.Analytical.QueryPopulation == readexec.AnalyticalQueryPopulationPolicy && p.Analytical.Version == readexec.AnalyticalQueryPopulationVersion && p.ValidationFixes == fixes && p.Bindings != nil {
		return
	}
	// Only public policy/slot metadata, never canonical answers or parameters.
	for _, group := range p.Route.Clarifications {
		for _, slot := range group.Slots {
			t.Logf("population slot: pattern=%s slot=%s outcome=%s reason=%s", slot.Pattern, slot.Slot, slot.Outcome, slot.Reason)
		}
	}
	t.Fatalf("population proof: err=%v proof=%+v fixes=%d binding=%v resolutions=%d answers=%d planned=%v", err, p.Analytical, p.ValidationFixes, p.Bindings != nil, len(p.Route.Resolutions), len(p.Route.Request.Answers), p.QueryID != "")
}

func TestSQLRecoveryQueryPopulationAcceptance(t *testing.T) {
	good := `SELECT sum(amount) AS revenue FROM analytics.sales`
	t.Run("private predicate extra narrowing corrected once", func(t *testing.T) {
		f := newCW01Fixture(t)
		q := populationPrivateQuestion(t, f)
		f.model.mu.Lock()
		start := len(f.model.requestBodies)
		f.model.chatSequence = []string{phase18RawResponse(t, good+" WHERE amount < 10"), phase18RawResponse(t, good)}
		f.model.mu.Unlock()
		p, err := f.query.Plan(context.Background(), f.e, nlqexec.PlanRequest{QuestionRequest: q})
		requirePopulationProof(t, p, err, 1)
		f.run(t, p, 1, true)
		f.model.mu.Lock()
		wire := strings.Join(f.model.requestBodies[start:], "\n")
		f.model.mu.Unlock()
		if !strings.Contains(wire, "analytical_query_population_mismatch") || !strings.Contains(wire, "rejected_sql") || strings.Contains(wire, "cw-alpha-731") || strings.Contains(wire, "alias-secret-731") {
			t.Fatal("repair missing provenance or exposing private values")
		}
	})
	t.Run("persisted proof and zero-work terminal replay", func(t *testing.T) {
		f := newCW01Fixture(t)
		ctx := context.Background()
		f.model.mode.Store(phase18RawResponse(t, good))
		p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: populationPrivateQuestion(t, f)})
		requirePopulationProof(t, p, err, 0)
		f.run(t, p, 1, true)
		metadata := support.Raw(t, f.f.dsn)
		sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
		q, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
		if err != nil || q.AnalyticalVersion != 4 || q.Analytical == nil || q.Analytical.QueryPopulation != readexec.AnalyticalQueryPopulationPolicy {
			t.Fatal("query population persistence", err)
		}
		projected, err := f.f.db.ReadSavedQuery(ctx, f.e, q.ID, false)
		if err != nil || projected.Result != nil || readexec.Hash(projected.Analytical) != readexec.Hash(q.Analytical) {
			t.Fatal("saved proof or result privacy", err)
		}
		for _, mutation := range []string{`analytical_version=3`, `analytical=analytical-'query_population'`, `analytical=jsonb_set(analytical,'{query_population}','"unknown"')`, `analytical=NULL`, `analytical=jsonb_set(analytical,'{contract}','"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"')`} {
			if _, err := metadata.Exec(ctx, `UPDATE chartworks.nlq_queries SET `+mutation+` WHERE query_id=$1`, q.ID); err == nil {
				t.Fatal("immutable population evidence changed")
			}
		}
		calls, attempts := f.model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
		r, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: q.ID, Operation: q.ID + "-run"})
		if err != nil || r.Analytical == nil || readexec.Hash(r.Analytical) != readexec.Hash(q.Analytical) || r.Execution.Result == nil || calls != f.model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
			t.Fatal("terminal replay changed proof or performed work", err)
		}
	})
	t.Run("persistent extra WHERE and HAVING never execute", func(t *testing.T) {
		f := newCW01Fixture(t)
		ctx := context.Background()
		metadata := support.Raw(t, f.f.dsn)
		for _, suffix := range []string{" WHERE active=true", " WHERE 1=0", " HAVING sum(amount)>0"} {
			q := populationPrivateQuestion(t, f)
			f.model.mode.Store(phase18RawResponse(t, good+suffix))
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
			if p.QueryID != "" || !errors.Is(err, readexec.ErrAnalyticalMismatch) || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
				t.Fatalf("extra condition acquired executable result: err=%v proof=%+v binding=%v resolutions=%d planned=%v", err, p.Analytical, p.Bindings != nil, len(p.Route.Resolutions), p.QueryID != "")
			}
		}
	})
	t.Run("retained v3 keeps its original narrower claim", func(t *testing.T) {
		f := newCW01Fixture(t)
		ctx := context.Background()
		f.model.mode.Store(phase18RawResponse(t, good))
		p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: populationPrivateQuestion(t, f)})
		requirePopulationProof(t, p, err, 0)
		sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
		q, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
		if err != nil {
			t.Fatal(err)
		}
		binding, err := f.f.s.Binding(ctx, f.e, f.pack.Datasets[0].Source.Source, f.context)
		if err != nil {
			t.Fatal(err)
		}
		old := readexec.AnalyticalContract{Version: readexec.AnalyticalCalendarVersion, Binding: readexec.Hash(binding), Semantics: q.Route.Selection.Digest, Dataset: f.pack.Datasets[0].ID, Metrics: []readexec.AnalyticalMetric{{ID: f.pack.Topic + ":measure:revenue", Expression: readexec.AnalyticalExpression{Op: "sum", Column: "amount"}}}}
		q.ID, q.Operation = readexec.Hash([]string{"retained-v3-population", p.QueryID})[:32], ""
		q.AnalyticalVersion = 3
		q.Analytical = &readexec.AnalyticalReceipt{Version: old.Version, Scope: readexec.AnalyticalMetricScope, Contract: readexec.Hash(old), Query: readexec.AnalyticalQueryDigest(q.SQL, q.Parameters), Metrics: []string{old.Metrics[0].ID}}
		if err := f.f.db.CreateQuery(ctx, sc, q); err != nil {
			t.Fatal("retained v3 fixture", err)
		}
		r, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: q.ID, Operation: q.ID + "-run"})
		if err != nil || r.Analytical == nil || r.Analytical.Version != readexec.AnalyticalCalendarVersion || r.Analytical.QueryPopulation != "" || r.Execution.Result == nil {
			t.Fatal("retained policy reinterpreted", err)
		}
	})
}

func TestSQLRecoveryQueryPopulationTemporalAndNumeric(t *testing.T) {
	f := newCW01Fixture(t)
	good := `SELECT sum(amount) AS revenue FROM analytics.sales`
	for _, tc := range []struct {
		name, question, pattern string
		value                   semantics.ClarificationValue
	}{
		{"exact threshold", "Revenue large sales", "amount-required", cw01Number("10")},
		{"instant interval", "Revenue dated sales", "period", cw01Time("2026-01-02", "2026-01-03", "day")},
		{"boolean filter", "Revenue active sales", "state", cw01Bool("true")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.model.mode.Store(phase18RawResponse(t, good))
			p := f.plan(t, f.question(tc.question, nlq.LanguageEnglish), tc.pattern, tc.value)
			if p.Analytical == nil || p.Analytical.QueryPopulation != readexec.AnalyticalQueryPopulationPolicy {
				t.Fatal("typed query predicate has no proof")
			}
			if tc.pattern == "period" {
				window := p.Route.Resolutions[0].Time
				if window == nil || window.StartUTC != "2026-01-02T03:00:00Z" || window.EndUTC != "2026-01-03T03:00:00Z" || window.Bounds != "[)" {
					t.Fatal("reviewed local dates lost their exact instant bounds")
				}
			}
			r := f.run(t, p, 1, true)
			if readexec.Hash(r.Analytical) != readexec.Hash(p.Analytical) {
				t.Fatal("typed population proof changed at execution")
			}
		})
	}
}
