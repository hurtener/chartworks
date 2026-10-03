package acceptance

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func retainedIntentReviewFixture(t *testing.T, f *cw01Fixture, locale nlq.Language) nlqexec.QueryRecord {
	t.Helper()
	ctx := context.Background()
	f.model.mode.Store(phase18RawResponse(t, `SELECT id,sum(amount) AS revenue FROM analytics.sales GROUP BY id ORDER BY id`))
	q := f.question("Revenue by Record", locale)
	q.Grouping = groupingState(f, "record")
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil {
		t.Fatal(err)
	}
	sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
	old, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := f.f.s.Binding(ctx, f.e, f.pack.Datasets[0].Source.Source, f.context)
	if err != nil {
		t.Fatal(err)
	}
	contract := readexec.AnalyticalContract{Version: readexec.AnalyticalIntentVersion, Binding: readexec.Hash(binding), Semantics: old.Route.Selection.Digest, Dataset: f.pack.Datasets[0].ID, Metrics: []readexec.AnalyticalMetric{{ID: f.pack.Topic + ":measure:revenue", Expression: readexec.AnalyticalExpression{Op: "sum", Column: "amount"}}}, Grain: &readexec.AnalyticalGrain{Policy: readexec.AnalyticalGroupingPolicy, Columns: []string{"id"}, Dimensions: []string{f.pack.Topic + ":dimension:record"}}}
	old.ID = readexec.Hash([]string{"legacy-intent-review", p.QueryID})[:32]
	old.Operation = ""
	old.SQL = `SELECT id,sum(amount) AS revenue FROM analytics.sales WHERE amount > $1 GROUP BY id ORDER BY id`
	old.Parameters = []readexec.Parameter{{Kind: "number", Value: "13.333731"}}
	old.AnalyticalVersion = 6
	old.Analytical = &readexec.AnalyticalReceipt{Version: contract.Version, Scope: readexec.AnalyticalGrainScope, Contract: readexec.Hash(contract), Query: readexec.AnalyticalQueryDigest(old.SQL, old.Parameters), Metrics: []string{contract.Metrics[0].ID}, Grouping: contract.Grain.Dimensions}
	if err := f.f.db.CreateQuery(ctx, sc, old); err != nil {
		t.Fatal(err)
	}
	return old
}

func TestSQLRecoveryLegacyIntentReviewAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := groupingContinuationFixture(t)
			ctx := context.Background()
			old := retainedIntentReviewFixture(t, f, locale)
			sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
			// Existing replay remains measured only by its historical proof.
			out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: old.ID, Operation: old.ID + "-original"})
			if err != nil {
				t.Fatal("legacy replay", err)
			}
			groupingSums(t, out, "20")
			old, err = f.f.db.ReadQuery(ctx, sc, old.ID)
			if err != nil {
				t.Fatal(err)
			}
			before := nlqexec.QueryLineageDigest(old)
			q := f.question("Revenue named sales by Status", locale)
			q.Grouping = groupingState(f, "status")
			pending := f.preflight(t, q)
			request := nlqexec.RefineRequest{QueryID: old.ID, IntentReview: &nlqexec.LegacyIntentReview{QueryID: pending.QueryID, AnswerContext: pending.Route.AnswerContext, Answers: []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("primero"))}}}
			f.model.mu.Lock()
			start := len(f.model.requestBodies)
			f.model.mu.Unlock()
			calls := adversarialChatCount(f)
			for _, change := range []func(*nlqexec.RefineRequest){
				func(r *nlqexec.RefineRequest) { r.IntentReview.AnswerContext = "stale" },
				func(r *nlqexec.RefineRequest) { r.IntentReview.QueryID = old.ID },
				func(r *nlqexec.RefineRequest) { r.IntentReview.Answers = nil },
				func(r *nlqexec.RefineRequest) {
					r.IntentReview.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("foreign-private-value"))}
				},
				func(r *nlqexec.RefineRequest) {
					r.ParameterEdits = []nlqexec.ParameterEdit{{Position: 1, Replacement: readexec.Parameter{Kind: "number", Value: "0"}}}
				},
				func(r *nlqexec.RefineRequest) { r.Question = "silently inherit old filters" },
			} {
				bad := request
				cp := *request.IntentReview
				bad.IntentReview = &cp
				change(&bad)
				if _, err := f.query.Refine(ctx, f.e, bad); err == nil {
					t.Fatal("invalid review admitted")
				}
				if adversarialChatCount(f) != calls {
					t.Fatal("invalid review called provider")
				}
			}

			for _, who := range []struct{ tenant, user, session string }{{f.e.Tenant(), "other-review-user", f.e.Session()}, {f.e.Tenant(), f.e.User(), "other-review-session"}, {"other-review-tenant", f.e.User(), f.e.Session()}} {
				foreign, err := identity.FromVerified(who.tenant, who.user, who.session, f.e.Scopes(), f.e.Deadline(), nil)
				if err != nil {
					t.Fatal(err)
				}
				if out, err := f.query.Refine(ctx, foreign, request); err == nil || out.QueryID != "" {
					t.Fatal("foreign authority admitted review", err)
				}
			}
			foreign := phase18Envelope(t, f.phase17Fixture, f.e.User(), "pending-review-session", false)
			foreignPending, err := f.query.Preflight(ctx, foreign, nlqexec.PreflightRequest{QuestionRequest: q})
			if err != nil {
				t.Fatal(err)
			}
			badOrigin := request
			originCopy := *request.IntentReview
			badOrigin.IntentReview = &originCopy
			badOrigin.IntentReview.QueryID = foreignPending.QueryID
			badOrigin.IntentReview.AnswerContext = foreignPending.Route.AnswerContext
			if _, err := f.query.Refine(ctx, f.e, badOrigin); !errors.Is(err, nlqexec.ErrForeignSession) {
				t.Fatal("foreign pending form accepted", err)
			}
			if adversarialChatCount(f) != calls {
				t.Fatal("foreign review called provider")
			}
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			f.model.mode.Store(phase18RawResponse(t, `SELECT active,sum(amount) AS revenue FROM analytics.sales GROUP BY active ORDER BY active`))
			const concurrent = 3
			results := make([]nlqexec.PlanResult, concurrent)
			errs := make([]error, concurrent)
			var wg sync.WaitGroup
			for i := range results {
				wg.Add(1)
				go func(i int) { defer wg.Done(); results[i], errs[i] = f.query.Refine(ctx, f.e, request) }(i)
			}
			wg.Wait()
			for i, err := range errs {
				if err != nil || results[i].QueryID == "" || results[i].QueryID != results[0].QueryID {
					t.Fatalf("review concurrent %d: %v", i, err)
				}
			}
			if adversarialChatCount(f) != calls+1 {
				t.Fatal("duplicate review generated twice", adversarialChatCount(f)-calls)
			}
			child := results[0]
			modern := request
			modern.QueryID = child.QueryID
			if _, err := f.query.Refine(ctx, f.e, modern); err == nil {
				t.Fatal("modern proof used legacy replacement bypass")
			}

			saved, err := f.f.db.ReadQuery(ctx, sc, child.QueryID)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Parent != old.ID || saved.IntentReview == nil || saved.IntentReview.Preflight.QueryID != pending.QueryID || len(saved.Parameters) != 1 || saved.Parameters[0].Value != "cw-alpha-731" || saved.Clarification == nil || len(saved.Clarification.BaseParameters) != 0 {
				t.Fatal("review lost exact origins or copied old parameters")
			}
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			out, err = f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
			if err != nil {
				t.Fatal("review restart/run", err)
			}
			groupingSums(t, out, "5", "10")
			repeated, err := f.query.Refine(ctx, f.e, request)
			if err != nil || repeated.QueryID != child.QueryID || adversarialChatCount(f) != calls+1 {
				t.Fatal("review replay after execution", err)
			}
			changed := request
			cp := *request.IntentReview
			changed.IntentReview = &cp
			changed.IntentReview.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("segundo"))}
			if _, err := f.query.Refine(ctx, f.e, changed); !errors.Is(err, store.ErrConflict) {
				t.Fatal("changed review reused origin", err)
			}
			f.model.mu.Lock()
			wire := strings.Join(f.model.requestBodies[start:], "\n")
			f.model.mu.Unlock()
			if strings.Contains(wire, "13.333731") || strings.Contains(wire, "cw-alpha-731") || strings.Contains(wire, "primero") || strings.Contains(wire, "previous_sql") {
				t.Fatal("private/current or historical intent leaked to provider")
			}

			// Native diagnostics repair the new governed plan, retaining only
			// the newly reviewed private predicate through correction/restart.
			correctionPending := f.preflight(t, q)
			correction := request
			correctionCopy := *request.IntentReview
			correction.IntentReview = &correctionCopy
			correction.IntentReview.QueryID = correctionPending.QueryID
			correction.IntentReview.AnswerContext = correctionPending.Route.AnswerContext
			f.model.mu.Lock()
			f.model.chatSequence = []string{phase18RawResponse(t, `SELECT active,sum(lower(amount)) AS revenue FROM analytics.sales GROUP BY active ORDER BY active`), phase18RawResponse(t, `SELECT active,sum(amount) AS revenue FROM analytics.sales GROUP BY active ORDER BY active`)}
			f.model.mu.Unlock()
			repaired, err := f.query.Refine(ctx, f.e, correction)
			if err != nil || repaired.ValidationFixes != 1 {
				t.Fatal("reviewed native correction", err)
			}
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			repairedOut, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: repaired.QueryID, Operation: repaired.QueryID + "-run"})
			if err != nil {
				t.Fatal(err)
			}
			groupingSums(t, repairedOut, "5", "10")

			// A non-executable model decision survives restart and resumes
			// only the same complete reviewed intent under its protected pin.
			decisionPending := f.preflight(t, q)
			decisionRequest := request
			decisionCopy := *request.IntentReview
			decisionRequest.IntentReview = &decisionCopy
			decisionRequest.IntentReview.QueryID = decisionPending.QueryID
			decisionRequest.IntentReview.AnswerContext = decisionPending.Route.AnswerContext
			f.model.mode.Store(decisionRawResponse(t, "clarify", "Confirm the reviewed intent."))
			_, err = f.query.Refine(ctx, f.e, decisionRequest)
			problem := nlqexec.GenerationProblem(err)
			if problem == nil || problem.QueryID == "" {
				t.Fatal("review decision was not retained", err)
			}
			decisionCalls := adversarialChatCount(f)
			_, err = f.query.Refine(ctx, f.e, decisionRequest)
			repeatedProblem := nlqexec.GenerationProblem(err)
			if repeatedProblem == nil || repeatedProblem.QueryID != problem.QueryID || adversarialChatCount(f) != decisionCalls {
				t.Fatal("pending review repeated model", err)
			}
			resume := decisionRequest
			resume.QueryID = problem.QueryID
			resume.GenerationQuery = problem.QueryID
			resume.GenerationContext = problem.AnswerContext
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			f.model.mode.Store(phase18RawResponse(t, `SELECT active,sum(amount) AS revenue FROM analytics.sales GROUP BY active ORDER BY active`))
			resumed, err := f.query.Refine(ctx, f.e, resume)
			if err != nil {
				t.Fatal("review decision resume", err)
			}
			resumedOut, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: resumed.QueryID, Operation: "resume:" + problem.QueryID})
			if err != nil {
				t.Fatal("resumed review run", err)
			}
			groupingSums(t, resumedOut, "5", "10")
			f.model.mu.Lock()
			allWire := strings.Join(f.model.requestBodies[start:], "\n")
			f.model.mu.Unlock()
			for _, private := range []string{"13.333731", "cw-alpha-731", "primero"} {
				if strings.Contains(allWire, private) {
					t.Fatal("private binding leaked through correction or resume")
				}
			}
			// The persistence boundary checks both exact origins atomically.
			stale := saved
			stale.ID = readexec.Hash([]string{"stale-review-child", saved.ID})[:32]
			stale.Operation = ""
			stale.Revision = 1
			staleProof := *saved.IntentReview
			stale.IntentReview = &staleProof
			stale.IntentReview.Preflight.Revision++
			if err := f.f.db.CreateQuery(ctx, sc, stale); !errors.Is(err, store.ErrConflict) {
				t.Fatal("stale preflight committed", err)
			}
			metadata := support.Raw(t, f.f.dsn)
			if _, err := metadata.Exec(ctx, `UPDATE chartworks.nlq_queries SET intent_review=NULL,revision=revision+1 WHERE tenant_id=$1 AND query_id=$2`, f.e.Tenant(), child.QueryID); err == nil {
				t.Fatal("review provenance mutated")
			}

			beforeReplayCalls := adversarialChatCount(f)
			beforeReads := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			if _, err := metadata.Exec(ctx, `UPDATE chartworks.nlq_queries SET status='failed',revision=revision+1 WHERE tenant_id=$1 AND query_id=$2`, f.e.Tenant(), pending.QueryID); err != nil {
				t.Fatal(err)
			}
			if _, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"}); !errors.Is(err, readexec.ErrBinding) {
				t.Fatal("stale preflight replay accepted", err)
			}
			if adversarialChatCount(f) != beforeReplayCalls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != beforeReads {
				t.Fatal("stale replay did model or row work")
			}
			unchanged, err := f.f.db.ReadQuery(ctx, sc, old.ID)
			if err != nil || nlqexec.QueryLineageDigest(unchanged) != before || !reflect.DeepEqual(unchanged.Parameters, old.Parameters) {
				t.Fatal("legacy query changed", err)
			}
		})
	}
}

func TestSQLRecoveryLegacyIntentReviewVersionMatrixAcceptance(t *testing.T) {
	f := groupingContinuationFixture(t)
	ctx := context.Background()
	f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`))
	base, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue", nlq.LanguageEnglish)})
	if err != nil {
		t.Fatal(err)
	}
	sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
	retained, err := f.f.db.ReadQuery(ctx, sc, base.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := f.f.s.Binding(ctx, f.e, f.pack.Datasets[0].Source.Source, f.context)
	if err != nil {
		t.Fatal(err)
	}
	q := f.question("Revenue named sales", nlq.LanguageEnglish)
	pending := f.preflight(t, q)
	versions := []string{readexec.AnalyticalVersion, readexec.AnalyticalGrainVersion, readexec.AnalyticalCalendarVersion, readexec.AnalyticalQueryPopulationVersion, readexec.AnalyticalGroupingVersion, readexec.AnalyticalIntentVersion}
	for i, version := range versions {
		t.Run(version, func(t *testing.T) {
			old := retained
			old.ID = readexec.Hash([]string{"legacy-version-review", version, base.QueryID})[:32]
			old.Operation = ""
			old.AnalyticalVersion = i + 1
			old.SQL = `SELECT sum(amount) AS revenue FROM analytics.sales WHERE amount > $1`
			old.Parameters = []readexec.Parameter{{Kind: "number", Value: "13.333731"}}
			contract := readexec.AnalyticalContract{Version: version, Binding: readexec.Hash(binding), Semantics: old.Route.Selection.Digest, Dataset: f.pack.Datasets[0].ID, Metrics: []readexec.AnalyticalMetric{{ID: f.pack.Topic + ":measure:revenue", Expression: readexec.AnalyticalExpression{Op: "sum", Column: "amount"}}}}
			native, err := f.f.validator.ValidateWithin(ctx, f.e, readexec.Request{Source: binding.Source, Context: f.context, SQL: old.SQL, Parameters: old.Parameters}, old.RelationScope)
			if err != nil {
				t.Fatal(err)
			}
			old.Analytical, err = readexec.CheckAnalyticalPlan(ctx, native, contract)
			if err != nil {
				t.Fatal("historical native proof", err)
			}
			if err := f.f.db.CreateQuery(ctx, sc, old); err != nil {
				t.Fatal(err)
			}
			original, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: old.ID, Operation: old.ID + "-run"})
			if err != nil {
				t.Fatal("version replay", err)
			}
			groupingSums(t, original, "20")
			f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`))
			child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: old.ID, IntentReview: &nlqexec.LegacyIntentReview{QueryID: pending.QueryID, AnswerContext: pending.Route.AnswerContext, Answers: []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("primero"))}}})
			if err != nil {
				t.Fatal("version reviewed restart", err)
			}
			fresh, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
			if err != nil {
				t.Fatal(err)
			}
			groupingSums(t, fresh, "15")
		})
	}
}
