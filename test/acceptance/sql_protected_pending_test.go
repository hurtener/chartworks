package acceptance

import (
	"encoding/json"
	"errors"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/generationdecision"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

func protectedPendingQuestion(t *testing.T, f *cw01Fixture) nlqexec.QuestionRequest {
	t.Helper()
	q := f.question("Show named sales for alias-secret-731", nlq.LanguageEnglish)
	p, err := f.query.Preflight(t.Context(), f.e, nlqexec.PreflightRequest{QuestionRequest: q})
	if err != nil || p.Route.Clarification == nil {
		t.Fatal("protected preflight", err)
	}
	q.ClarificationQuery, q.AnswerContext = p.QueryID, p.Route.AnswerContext
	q.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("alias-secret-731"))}
	return q
}
func protectedPendingRecord(t *testing.T, f *cw01Fixture, id string) nlqexec.QueryRecord {
	t.Helper()
	scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
	q, err := f.f.db.ReadQuery(t.Context(), scope, id)
	if err != nil {
		t.Fatal("retained pending", err)
	}
	return q
}
func requireProtectedPendingPrivacy(t *testing.T, q nlqexec.QueryRecord) {
	t.Helper()
	b, err := json.Marshal(q.GenerationPending)
	if err != nil || strings.Contains(string(b), "alias-secret-731") {
		t.Fatal("generation pending retained original private text")
	}
}
func TestProtectedGenerationPendingAcceptance(t *testing.T) {
	t.Run("original_plan_privacy", func(t *testing.T) {
		f := newCW01Fixture(t)
		q := protectedPendingQuestion(t, f)
		q.Hints = []nlq.Instruction{{Key: "private_hint", Text: "Use alias-secret-731"}}
		f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which reviewed customer?"))
		_, err := f.query.Plan(t.Context(), f.e, nlqexec.PlanRequest{QuestionRequest: q, Operation: "protected-pending-plan"})
		p := nlqexec.GenerationProblem(err)
		if p == nil {
			t.Fatal("pending plan", err)
		}
		first := protectedPendingRecord(t, f, p.QueryID)
		requireProtectedPendingPrivacy(t, first)
		f.query, _ = newPhase18Service(t, f.phase17Fixture)
		calls := decisionChatCount(f)
		_, err = f.query.Plan(t.Context(), f.e, nlqexec.PlanRequest{QuestionRequest: q, Operation: "protected-pending-plan"})
		if !reflect.DeepEqual(nlqexec.GenerationProblem(err), p) || decisionChatCount(f) != calls {
			t.Fatal("original pending restart replay", err)
		}
		q.GenerationQuery, q.GenerationContext = p.QueryID, p.AnswerContext
		q.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("second"))}
		answer := nlqexec.PlanRequest{QuestionRequest: q, Operation: "resume:" + p.QueryID}
		for _, mutate := range []func(*nlqexec.PlanRequest){func(r *nlqexec.PlanRequest) { r.Question = "Show named sales for second" }, func(r *nlqexec.PlanRequest) { r.Hints = []nlq.Instruction{{Key: "private_hint", Text: "Use second"}} }} {
			bad := answer
			mutate(&bad)
			if _, err := f.query.Plan(t.Context(), f.e, bad); err == nil || decisionChatCount(f) != calls {
				t.Fatal("same-mask pending settings borrowed original commitment", err)
			}
		}
		_, err = f.query.Plan(t.Context(), f.e, answer)
		second := nlqexec.GenerationProblem(err)
		if second == nil {
			t.Fatal("exact original Plan second pending", err)
		}
		secondRow := protectedPendingRecord(t, f, second.QueryID)
		requireProtectedPendingPrivacy(t, secondRow)
		requireProtectedPendingAncestor(t, secondRow, first)
		answer.GenerationQuery, answer.GenerationContext, answer.Operation = second.QueryID, second.AnswerContext, "resume:"+second.QueryID
		f.model.mode.Store(phase18RawResponse(t, `SELECT id,name FROM analytics.sales ORDER BY id`))
		child, err := f.query.Plan(t.Context(), f.e, answer)
		if err != nil {
			t.Fatal("original Plan final resume", err)
		}
		requireProtectedPendingAncestor(t, protectedPendingRecord(t, f, child.QueryID), secondRow)
		calls = decisionChatCount(f)
		again, err := f.query.Plan(t.Context(), f.e, answer)
		if err != nil || again.QueryID != child.QueryID || decisionChatCount(f) != calls {
			t.Fatal("Plan resume replay", err)
		}
		out, err := f.query.Run(t.Context(), f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: answer.Operation})
		if err != nil {
			t.Fatal(err)
		}
		requireParameterIDs(t, out, "2")
	})
	t.Run("original_refine_privacy", func(t *testing.T) {
		f := newCW01Fixture(t)
		q := protectedPendingQuestion(t, f)
		parent, err := f.query.Plan(t.Context(), f.e, nlqexec.PlanRequest{QuestionRequest: q})
		if err != nil {
			t.Fatal(err)
		}
		f.query, _ = newPhase18Service(t, f.phase17Fixture)
		delta := nlqexec.RefineRequest{QueryID: parent.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: q.Question, Hints: []nlq.Instruction{{Key: "private_hint", Text: "Use alias-secret-731"}}}}
		f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which reviewed customer?"))
		_, err = f.query.Refine(t.Context(), f.e, delta)
		p := nlqexec.GenerationProblem(err)
		if p == nil {
			t.Fatal("pending refinement", err)
		}
		first := protectedPendingRecord(t, f, p.QueryID)
		requireProtectedPendingPrivacy(t, first)
		f.query, _ = newPhase18Service(t, f.phase17Fixture)
		answer := delta
		answer.QueryID = p.QueryID
		answer.GenerationQuery = p.QueryID
		answer.GenerationContext = p.AnswerContext
		answer.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("second"))}
		calls := decisionChatCount(f)
		bad := answer
		bad.Hints = []nlq.Instruction{{Key: "private_hint", Text: "Use second"}}
		if _, err := f.query.Refine(t.Context(), f.e, bad); err == nil || decisionChatCount(f) != calls {
			t.Fatal("same-mask Refine instruction borrowed original commitment", err)
		}
		f.model.mode.Store(phase18RawResponse(t, `SELECT id,name FROM analytics.sales ORDER BY id`))
		child, err := f.query.Refine(t.Context(), f.e, answer)
		if err != nil {
			t.Fatal("explicit original Refine resume", err)
		}
		requireProtectedPendingAncestor(t, protectedPendingRecord(t, f, child.QueryID), first)
		calls = decisionChatCount(f)
		again, err := f.query.Refine(t.Context(), f.e, answer)
		if err != nil || again.QueryID != child.QueryID || decisionChatCount(f) != calls {
			t.Fatal("explicit original Refine replay", err)
		}
		out, err := f.query.Run(t.Context(), f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: "resume:" + p.QueryID})
		if err != nil {
			t.Fatal(err)
		}
		requireParameterIDs(t, out, "2")
	})
	t.Run("refine_pending_ancestry", func(t *testing.T) {
		f := newCW01Fixture(t)
		q := protectedPendingQuestion(t, f)
		parent, err := f.query.Plan(t.Context(), f.e, nlqexec.PlanRequest{QuestionRequest: q})
		if err != nil {
			t.Fatal(err)
		}
		f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which reviewed customer?"))
		delta := nlqexec.RefineRequest{QueryID: parent.QueryID}
		_, err = f.query.Refine(t.Context(), f.e, delta)
		p := nlqexec.GenerationProblem(err)
		if p == nil {
			t.Fatal("first pending refinement", err)
		}
		f.query, _ = newPhase18Service(t, f.phase17Fixture)
		answer := nlqexec.RefineRequest{QueryID: p.QueryID, QuestionRequest: nlqexec.QuestionRequest{GenerationQuery: p.QueryID, GenerationContext: p.AnswerContext, Answers: []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("second"))}}}
		_, err = f.query.Refine(t.Context(), f.e, answer)
		next := nlqexec.GenerationProblem(err)
		if next == nil {
			t.Fatal("second pending refinement lost custody", err)
		}
		retained := protectedPendingRecord(t, f, next.QueryID)
		requireProtectedPendingAncestor(t, retained, protectedPendingRecord(t, f, p.QueryID))
		if retained.GenerationPending.Refinement.QueryID != parent.QueryID {
			t.Fatal("second pending lost SQL ancestor")
		}
		answer.QueryID, answer.GenerationQuery, answer.GenerationContext = next.QueryID, next.QueryID, next.AnswerContext
		f.model.mode.Store(phase18RawResponse(t, `SELECT id,name FROM analytics.sales ORDER BY id`))
		child, err := f.query.Refine(t.Context(), f.e, answer)
		if err != nil {
			t.Fatal("second pending final refinement", err)
		}
		requireProtectedPendingAncestor(t, protectedPendingRecord(t, f, child.QueryID), retained)
	})
	t.Run("reviewed_replacement_origin", func(t *testing.T) {
		f := groupingContinuationFixture(t)
		old := retainedIntentReviewFixture(t, f, nlq.LanguageEnglish)
		q := f.question("Revenue named sales for alias-secret-731 by Status", nlq.LanguageEnglish)
		q.Grouping = groupingState(f, "status")
		pending := f.preflight(t, q)
		if pending.Route.Applicability == nil {
			t.Fatal("protected reviewed origin fixture")
		}
		request := nlqexec.RefineRequest{QueryID: old.ID, IntentReview: &nlqexec.LegacyIntentReview{QueryID: pending.QueryID, AnswerContext: pending.Route.AnswerContext, Answers: []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("primero"))}}}
		f.model.mode.Store(phase18RawResponse(t, `SELECT active,sum(amount) AS revenue FROM analytics.sales GROUP BY active ORDER BY active`))
		child, err := f.query.Refine(t.Context(), f.e, request)
		if err != nil {
			t.Fatal("protected reviewed replacement lost origin", err)
		}
		retained := protectedPendingRecord(t, f, child.QueryID)
		if retained.Parent != old.ID || retained.IntentReview == nil || retained.Route.Applicability.PriorQuery != pending.QueryID {
			t.Fatal("review and applicability origins were conflated")
		}
		scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
		forged := retained
		forged.ID = readexec.Hash("forged-review-origin")[:32]
		forged.Operation = ""
		forged.Revision = 1
		forged.Route, err = child.Route.BindApplicabilityQuery(f.e, forged.ID)
		if err != nil || !forged.Route.ApplicabilityWriteValid(f.e.Tenant(), f.e.User(), f.e.Session(), forged.ID) {
			t.Fatal("private route fixture", err)
		}
		if err := f.f.db.CreateQuery(t.Context(), scope, forged); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("fresh JSON review proof manufactured alternate write origin", err)
		}
		otherQuestion := q
		otherQuestion.Question = "Revenue named sales for second by Status"
		other := f.preflight(t, otherQuestion)
		if other.Route.Request.Question != pending.Route.Request.Question {
			t.Fatal("same-mask review fixture")
		}
		otherRow := protectedPendingRecord(t, f, other.QueryID)
		changed := *forged.IntentReview
		changed.Preflight = nlqexec.IntentReviewOrigin{QueryID: otherRow.ID, Revision: otherRow.Revision, Digest: nlqexec.QueryLineageDigest(otherRow)}
		forged.IntentReview = &changed
		if err := f.f.db.CreateQuery(t.Context(), scope, forged); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("same-mask preflight transplant manufactured review custody", err)
		}
		pendingRow := protectedPendingRecord(t, f, pending.QueryID)
		oldDigest := nlqexec.QueryLineageDigest(protectedPendingRecord(t, f, old.ID))
		calls := decisionChatCount(f)
		pendingRow.Revision++
		if err := f.f.db.UpdateQuery(t.Context(), scope, pendingRow, pendingRow.Revision-1); err != nil {
			t.Fatal("preflight drift fixture", err)
		}
		if _, err := f.query.Refine(t.Context(), f.e, request); err == nil || decisionChatCount(f) != calls {
			t.Fatal("preflight drift borrowed still-valid legacy parent", err)
		}
		if nlqexec.QueryLineageDigest(protectedPendingRecord(t, f, old.ID)) != oldDigest {
			t.Fatal("preflight drift mutated legacy parent")
		}

	})

	t.Run("run_correction_pending", func(t *testing.T) {
		f := newCW01Fixture(t)
		q := protectedPendingQuestion(t, f)
		f.model.mode.Store(phase18RawResponse(t, `SELECT id,amount/(id-id) AS amount FROM analytics.sales`))
		parent, err := f.query.Plan(t.Context(), f.e, nlqexec.PlanRequest{QuestionRequest: q})
		if err != nil {
			t.Fatal(err)
		}
		f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which reviewed customer?"))
		out, err := f.query.Run(t.Context(), f.e, nlqexec.RunRequest{QueryID: parent.QueryID, Operation: "protected-correction-run"})
		p := nlqexec.GenerationProblem(err)
		if p == nil || out.Status != "failed" {
			t.Fatal("protected correction pending was not atomically retained", err)
		}
		pending := protectedPendingRecord(t, f, p.QueryID)
		requireProtectedPendingPrivacy(t, pending)
		actualParent := protectedPendingRecord(t, f, parent.QueryID)
		requireProtectedPendingAncestor(t, pending, actualParent)
		scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
		attempted := actualParent
		attempted.Revision++
		duplicate := pending
		duplicate.ParentRevision = attempted.Revision
		duplicate.ParentDigest = nlqexec.QueryLineageDigest(attempted)
		if err := f.f.db.FinalizeGenerationPending(t.Context(), scope, attempted, actualParent.Revision, duplicate); err == nil {
			t.Fatal("duplicate pending correction was not atomic")
		}
		if nlqexec.QueryLineageDigest(protectedPendingRecord(t, f, parent.QueryID)) != nlqexec.QueryLineageDigest(actualParent) {
			t.Fatal("failed correction insert mutated parent")
		}
		f.query, _ = newPhase18Service(t, f.phase17Fixture)
		calls := decisionChatCount(f)
		_, err = f.query.Run(t.Context(), f.e, nlqexec.RunRequest{QueryID: parent.QueryID, Operation: "protected-correction-run"})
		if !reflect.DeepEqual(nlqexec.GenerationProblem(err), p) || decisionChatCount(f) != calls {
			t.Fatal("correction pending restart replay", err)
		}
		answer := nlqexec.RefineRequest{QueryID: p.QueryID, QuestionRequest: nlqexec.QuestionRequest{GenerationQuery: p.QueryID, GenerationContext: p.AnswerContext, Answers: []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("second"))}}}
		f.model.mode.Store(phase18RawResponse(t, `SELECT id,name FROM analytics.sales ORDER BY id`))
		child, err := f.query.Refine(t.Context(), f.e, answer)
		if err != nil {
			t.Fatal("correction pending resume", err)
		}
		requireProtectedPendingAncestor(t, protectedPendingRecord(t, f, child.QueryID), pending)
		out, err = f.query.Run(t.Context(), f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: "resume:" + p.QueryID})
		if err != nil {
			t.Fatal(err)
		}
		requireParameterIDs(t, out, "2")
	})
}

func requireProtectedPendingAncestor(t *testing.T, child, parent nlqexec.QueryRecord) {
	t.Helper()
	if child.Parent != parent.ID || child.ParentRevision != parent.Revision || child.ParentDigest != nlqexec.QueryLineageDigest(parent) || child.Route.Applicability == nil || parent.Route.Applicability == nil || child.Route.Applicability.PriorQuery != parent.ID || child.Route.Applicability.PriorDigest != readexec.Hash(parent.Route.Applicability) {
		t.Fatal("pending query and witness ancestry disagreed")
	}
}

func TestProtectedLegacyPendingCompatibilityAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	q := f.question("Revenue", nlq.LanguageEnglish)
	f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which reviewed metric?"))
	_, err := f.query.Plan(t.Context(), f.e, nlqexec.PlanRequest{QuestionRequest: q})
	problem := nlqexec.GenerationProblem(err)
	if problem == nil {
		t.Fatal(err)
	}
	legacy := protectedPendingRecord(t, f, problem.QueryID)
	legacy.ID = readexec.Hash("legacy-pending-fixture")[:32]
	legacy.Operation = "legacy-pending-replay"
	p := *legacy.GenerationPending
	p.Input = nil
	p.Problem.QueryID = legacy.ID
	p.Problem.AnswerContext = readexec.Hash(struct {
		ID         string
		Request    nlqexec.QuestionRequest
		Binding    string
		Choices    []generationdecision.Choice
		Questions  []string
		Outcome    string
		Expires    time.Time
		Round      int
		Refinement *nlqexec.RefineRequest
		Mode       string
	}{p.Problem.QueryID, p.Request, p.Binding, p.Problem.Choices, p.Problem.Questions, p.Problem.Outcome, p.Problem.ExpiresAt, p.Round, p.Refinement, p.Problem.Resume})
	legacy.GenerationPending = &p
	scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
	if err := f.f.db.CreateQuery(t.Context(), scope, legacy); err != nil {
		t.Fatal("legacy omitted commitment", err)
	}
	f.query, _ = newPhase18Service(t, f.phase17Fixture)
	calls := decisionChatCount(f)
	_, err = f.query.Plan(t.Context(), f.e, nlqexec.PlanRequest{QuestionRequest: q, Operation: legacy.Operation})
	if !reflect.DeepEqual(nlqexec.GenerationProblem(err), &p.Problem) || decisionChatCount(f) != calls {
		t.Fatal("legacy pending replay", err)
	}
	q.GenerationQuery, q.GenerationContext = legacy.ID, p.Problem.AnswerContext
	q.References = []semantics.Reference{{Kind: semantics.KindMeasure, ID: f.published.Definition.Measures[0].ID}}
	f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`))
	child, err := f.query.Plan(t.Context(), f.e, nlqexec.PlanRequest{QuestionRequest: q, Operation: "resume:" + legacy.ID})
	if err != nil || child.QueryID == "" {
		t.Fatal("legacy pending resume", err)
	}
}

func TestProtectedPendingInactiveAliasExpansionAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	q := protectedPendingQuestion(t, f)
	question := strings.TrimSpace(strings.Repeat("second ", 70))
	if len(question) > generationdecision.MaxQuestionBytes {
		t.Fatal("question must pass the model decision input bound")
	}
	f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, question))
	_, err := f.query.Plan(t.Context(), f.e, nlqexec.PlanRequest{QuestionRequest: q, Operation: "inactive-alias-expansion"})
	problem := nlqexec.GenerationProblem(err)
	if problem == nil || len(problem.Questions) != 1 || problem.Questions[0] != "[question withheld: redaction exceeds limit]" {
		t.Fatal("broader private masking rejected a bounded durable question", err)
	}
	retained := protectedPendingRecord(t, f, problem.QueryID)
	if !nlqexec.GenerationPendingValid(retained) {
		t.Fatal("redacted fallback was not durably valid")
	}
}
