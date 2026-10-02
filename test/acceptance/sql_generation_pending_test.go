package acceptance

import (
	"context"
	"errors"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/generationdecision"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestSQLRecoveryDurableGenerationQuestionsAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	raw := support.Raw(t, f.f.dsn)
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			q := f.question("Revenue", locale)
			operation := "pending-generation-" + string(locale)
			request := nlqexec.PlanRequest{QuestionRequest: q, Operation: operation}
			f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which reviewed metric?"))
			before := decisionChatCount(f)
			_, err := f.query.Plan(ctx, f.e, request)
			p := nlqexec.GenerationProblem(err)
			if p == nil || p.QueryID == "" || p.AnswerContext == "" || len(p.Choices) == 0 || decisionChatCount(f) != before+1 {
				t.Fatal("missing durable origin", err)
			}
			scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
			retained, err := f.f.db.ReadQuery(ctx, scope, p.QueryID)
			if err != nil || !nlqexec.GenerationPendingValid(retained) || retained.SQL != "" || retained.Status != "preflight" {
				t.Fatal("pending storage", err)
			}
			restarted, _ := newPhase18Service(t, f.phase17Fixture)
			_, err = restarted.Plan(ctx, f.e, request)
			if got := nlqexec.GenerationProblem(err); !reflect.DeepEqual(got, p) || decisionChatCount(f) != before+1 {
				t.Fatal("restart regenerated question", err)
			}
			for _, coordinates := range [][2]string{{"different-tenant", f.e.User()}, {f.e.Tenant(), "different-actor"}} {
				foreign, _ := store.NewScope(coordinates[0], coordinates[1])
				if _, err := f.f.db.ReadQuery(ctx, foreign, p.QueryID); !errors.Is(err, store.ErrNotFound) {
					t.Fatal("pending metadata crossed owner boundary", err)
				}
			}
			if _, err := raw.Exec(ctx, `UPDATE chartworks.nlq_queries SET generation_pending=NULL WHERE query_id=$1`, p.QueryID); err == nil {
				t.Fatal("pending evidence mutable")
			}
			if _, err := restarted.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: "forbidden-pending-run-" + string(locale)}); !errors.Is(err, nlqexec.ErrNoPlan) {
				t.Fatal("pending executed", err)
			}
			q.GenerationQuery = p.QueryID
			q.GenerationContext = p.AnswerContext
			q.References = []semantics.Reference{{Kind: semantics.KindMeasure, ID: f.published.Definition.Measures[0].ID}}
			answer := nlqexec.PlanRequest{QuestionRequest: q, Operation: "resume:" + p.QueryID}
			denied := answer
			denied.GenerationContext = "substituted"
			if _, err = restarted.Plan(ctx, f.e, denied); err == nil || decisionChatCount(f) != before+1 {
				t.Fatal("substituted origin reached provider", err)
			}
			f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`))
			var wg sync.WaitGroup
			results := make([]nlqexec.PlanResult, 4)
			failures := make([]error, 4)
			initialCalls := decisionChatCount(f)
			for i := range results {
				wg.Add(1)
				go func(i int) { defer wg.Done(); results[i], failures[i] = restarted.Plan(ctx, f.e, answer) }(i)
			}
			wg.Wait()
			child := results[0]
			for i := range results {
				if failures[i] != nil || results[i].QueryID == "" || results[i].QueryID != child.QueryID {
					t.Fatal("concurrent reviewed resumption diverged", failures[i])
				}
			}
			if decisionChatCount(f) != initialCalls+1 {
				t.Fatal("concurrent resumption repeated generation")
			}

			accepted, err := f.f.db.ReadQuery(ctx, scope, child.QueryID)
			if err != nil || accepted.Parent != p.QueryID || accepted.ParentDigest != nlqexec.QueryLineageDigest(retained) {
				t.Fatal("answer origin lost", err)
			}
			if _, err := restarted.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: "replace-resume-" + string(locale)}); !errors.Is(err, store.ErrConflict) {
				t.Fatal("run erased resume operation", err)
			}
			result, err := restarted.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: answer.Operation})
			if err != nil {
				t.Fatal("resumed plan failed execution", err)
			}
			requireExplanationSum(t, result, "9007199254740998.625")
			calls := decisionChatCount(f)
			replay, err := restarted.Plan(ctx, f.e, answer)
			if err != nil || replay.QueryID != child.QueryID || decisionChatCount(f) != calls {
				t.Fatal("resumption repeated model work", err)
			}
			unchanged, err := f.f.db.ReadQuery(ctx, scope, p.QueryID)
			if err != nil || nlqexec.QueryLineageDigest(unchanged) != nlqexec.QueryLineageDigest(retained) {
				t.Fatal("resumption mutated origin", err)
			}
		})
	}
}

func TestSQLRecoveryDurablePrivateRefinementAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := newCW01Fixture(t)
			ctx := context.Background()
			original := []readexec.Parameter{{Kind: "number", Value: "5.25"}}
			sql := `SELECT id,amount FROM analytics.sales WHERE amount > $1 ORDER BY id`
			f.model.mode.Store(parameterResponse(t, sql, original))
			parent, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("List sales records", locale)})
			if err != nil {
				t.Fatal(err)
			}
			delta := nlqexec.RefineRequest{QueryID: parent.QueryID, QuestionRequest: nlqexec.QuestionRequest{EditBase: []nlq.Instruction{{Key: "output_edit", Text: "Return only IDs in descending order; preserve current filters and bindings."}}}}
			f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which private cutoff should be retained?"))
			_, err = f.query.Refine(ctx, f.e, delta)
			p := nlqexec.GenerationProblem(err)
			if p == nil || p.QueryID == "" {
				t.Fatal("refinement lost durable question", err)
			}
			scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
			retained, err := f.f.db.ReadQuery(ctx, scope, p.QueryID)
			if err != nil || retained.GenerationPending.Refinement == nil || retained.Parent != parent.QueryID {
				t.Fatal("refinement origin missing", err)
			}
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			answer := delta
			answer.QueryID = p.QueryID
			answer.GenerationQuery = p.QueryID
			answer.GenerationContext = p.AnswerContext
			answer.ParameterEdits = []nlqexec.ParameterEdit{{Position: 1, Replacement: readexec.Parameter{Kind: "number", Value: "20"}}}
			childSQL := `SELECT id FROM analytics.sales WHERE amount > $1 ORDER BY id DESC`
			f.model.mode.Store(parameterResponse(t, childSQL, []readexec.Parameter{{Kind: "number", Value: "99999999"}}))
			f.model.mu.Lock()
			start := len(f.model.requestBodies)
			f.model.mu.Unlock()
			child, err := f.query.Refine(ctx, f.e, answer)
			if err != nil {
				t.Fatal("private question resumption", err)
			}
			persisted, err := f.f.db.ReadQuery(ctx, scope, child.QueryID)
			if err != nil || len(persisted.Parameters) != 1 || persisted.Parameters[0].Value != "20" || persisted.Parent != p.QueryID || persisted.GenerationResolution == nil {
				t.Fatal("private answer custody/origin lost", err)
			}
			calls := decisionChatCount(f)
			replay, err := f.query.Refine(ctx, f.e, answer)
			if err != nil || replay.QueryID != child.QueryID || decisionChatCount(f) != calls {
				t.Fatal("private answer replay regenerated", err)
			}
			altered := answer
			altered.ParameterEdits = []nlqexec.ParameterEdit{{Position: 1, Replacement: readexec.Parameter{Kind: "number", Value: "30"}}}
			if _, err = f.query.Refine(ctx, f.e, altered); !errors.Is(err, store.ErrConflict) || decisionChatCount(f) != calls {
				t.Fatal("different answer borrowed resume operation", err)
			}
			result, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: "resume:" + p.QueryID})
			if err != nil {
				t.Fatal(err)
			}
			requireParameterIDs(t, result, "1")
			f.model.mu.Lock()
			wire := strings.Join(f.model.requestBodies[start:], "\n")
			f.model.mu.Unlock()
			if strings.Contains(wire, "5.25") || !strings.Contains(wire, "retained_parameter_slots") {
				t.Fatal("private model bindings leaked or custody absent")
			}
		})
	}
}

func TestSQLRecoveryDurableCorrectionQuestionsAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	raw := support.Raw(t, f.f.dsn)
	statement := `SELECT id,amount/(id-id) AS amount FROM analytics.sales`
	f.model.mode.Store(phase18RawResponse(t, statement))
	initial, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Show reviewed records", nlq.LanguageEnglish)})
	if err != nil {
		t.Fatal(err)
	}
	f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which reviewed result should be used?"))
	runRequest := nlqexec.RunRequest{QueryID: initial.QueryID, Operation: initial.QueryID + "-run"}
	result, err := f.query.Run(ctx, f.e, runRequest)
	p := nlqexec.GenerationProblem(err)
	if p == nil || p.QueryID == "" || result.Status != "failed" || result.Execution.Attempt.Code != "query_division_by_zero" {
		t.Fatal("correction question not durably finalized", err)
	}
	scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
	parent, err := f.f.db.ReadQuery(ctx, scope, initial.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := f.f.db.ReadQuery(ctx, scope, p.QueryID)
	if err != nil || pending.ParentDigest != nlqexec.QueryLineageDigest(parent) || pending.ParentRevision != parent.Revision {
		t.Fatal("correction parent not atomic", err)
	}
	// A conflicting question insert must roll back the terminal-parent update.
	attempted := parent
	attempted.Revision++
	duplicate := pending
	duplicate.ParentRevision = attempted.Revision
	duplicate.ParentDigest = nlqexec.QueryLineageDigest(attempted)
	if err = f.f.db.FinalizeGenerationPending(ctx, scope, attempted, parent.Revision, duplicate); err == nil {
		t.Fatal("duplicate pending insert succeeded")
	}
	unchanged, err := f.f.db.ReadQuery(ctx, scope, parent.ID)
	if err != nil || unchanged.Revision != parent.Revision || nlqexec.QueryLineageDigest(unchanged) != nlqexec.QueryLineageDigest(parent) {
		t.Fatal("failed atomic finalization changed parent", err)
	}
	calls := decisionChatCount(f)
	reads := count(t, raw, `SELECT count(*) FROM chartworks.read_attempts`)
	f.query, _ = newPhase18Service(t, f.phase17Fixture)
	_, err = f.query.Run(ctx, f.e, runRequest)
	if !reflect.DeepEqual(nlqexec.GenerationProblem(err), p) || decisionChatCount(f) != calls || count(t, raw, `SELECT count(*) FROM chartworks.read_attempts`) != reads {
		t.Fatal("terminal replay lost correction question", err)
	}
	answer := nlqexec.RefineRequest{QueryID: p.QueryID, QuestionRequest: nlqexec.QuestionRequest{GenerationQuery: p.QueryID, GenerationContext: p.AnswerContext, References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: f.published.Definition.Measures[0].ID}}}}
	f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`))
	child, err := f.query.Refine(ctx, f.e, answer)
	if err != nil {
		t.Fatal("explicit correction answer did not plan", err)
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.read_attempts`) != reads {
		t.Fatal("answer implicitly executed source")
	}
	out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: "resume:" + p.QueryID})
	if err != nil {
		t.Fatal(err)
	}
	requireExplanationSum(t, out, "9007199254740998.625")
}

func TestSQLRecoveryDurableGovernedAnswerAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	q := populationPrivateQuestion(t, f)
	f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which reviewed customer should be used?"))
	_, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q, Operation: "private-pending"})
	p := nlqexec.GenerationProblem(err)
	if p == nil || p.Resume != "plan" {
		t.Fatal("private Plan origin", err)
	}
	q.GenerationQuery = p.QueryID
	q.GenerationContext = p.AnswerContext
	q.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("second"))}
	f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`))
	f.query, _ = newPhase18Service(t, f.phase17Fixture)
	answer := nlqexec.PlanRequest{QuestionRequest: q, Operation: "resume:" + p.QueryID}
	child, err := f.query.Plan(ctx, f.e, answer)
	if err != nil {
		t.Fatal("governed value resumption", err)
	}
	result, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: answer.Operation})
	if err != nil {
		t.Fatal(err)
	}
	requireExplanationSum(t, result, "5.5")
	calls := decisionChatCount(f)
	replay, err := f.query.Plan(ctx, f.e, answer)
	if err != nil || replay.QueryID != child.QueryID || decisionChatCount(f) != calls {
		t.Fatal("governed value replay", err)
	}
}

func TestSQLRecoveryDurableCalendarGroupingAcceptance(t *testing.T) {
	f := groupingContinuationFixture(t)
	ctx := context.Background()
	q := f.question("Revenue", nlq.LanguageEnglish)
	f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which reviewed calendar grain?"))
	_, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	p := nlqexec.GenerationProblem(err)
	if p == nil {
		t.Fatal(err)
	}
	q.GenerationQuery = p.QueryID
	q.GenerationContext = p.AnswerContext
	q.Grouping = &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy, Keys: []nlqroute.GroupingKey{{Topic: f.pack.Topic, Dimension: "event_date", Grain: semantics.GrainMonth}}}
	f.model.mode.Store(phase18RawResponse(t, `SELECT date_trunc('month',created_at,'UTC'),sum(amount) AS revenue FROM analytics.sales GROUP BY 1 ORDER BY 1`))
	f.query, _ = newPhase18Service(t, f.phase17Fixture)
	answer := nlqexec.PlanRequest{QuestionRequest: q, Operation: "resume:" + p.QueryID}
	child, err := f.query.Plan(ctx, f.e, answer)
	if err != nil {
		t.Fatal("calendar choice resumption", err)
	}
	out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: answer.Operation})
	if err != nil {
		t.Fatal(err)
	}
	groupingSums(t, out, "15", "20")
}
