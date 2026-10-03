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
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

// A private spelling may establish a reviewed clarification's applicability,
// but may not be retained in the question merely to replay that decision.
// This test crosses the actual SDK, HTTP handler, immutable PostgreSQL query
// records, source binding, and native execution; it is not a route-JSON test.
func TestProtectedApplicabilitySDKStoredReplayAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	definition := f.definition
	definition.Version = "protected-trigger-v2"
	customer := cw01Pattern(t, definition, "customer")
	customer.Policy.When = semantics.ClarificationWhen{AnyTerms: []string{"alias-secret-731"}}
	definition.Patterns = []semantics.ClarificationPattern{customer}
	f.publishRules(t, definition, 1)
	f.definition = definition
	client, server, _ := cw01HTTPClient(t, f)
	ctx := context.Background()
	question := f.question("Show alias-secret-731", nlq.LanguageEnglish)
	before := f.model.requests.Load()
	pending, err := client.PreflightNLQ(ctx, sdk.NLQPreflightRequest{QuestionRequest: question})
	if err != nil || pending.QueryID == "" || pending.Route.Clarification == nil || f.model.requests.Load() != before {
		protectedPreflightFailure(t, f, question, err)
	}
	if strings.Contains(pending.Route.Request.Question, "alias-secret-731") {
		t.Fatal("preflight retained private trigger")
	}
	// Resume exactly the canonical question the service retained. A fresh raw
	// question that merely redacts to this same string must not borrow this origin.
	question.Question = pending.Route.Request.Question
	question.AnswerContext = pending.Route.AnswerContext
	question.ClarificationQuery = pending.QueryID
	question.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("alias-secret-731"))}
	planned, err := client.PlanNLQ(ctx, sdk.NLQPlanRequest{QuestionRequest: question, Operation: "protected-private-trigger-plan"})
	if err != nil || planned.QueryID == "" || planned.Bindings == nil {
		t.Fatal("canonical protected applicability was lost before planning", err)
	}
	scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
	retained, err := f.f.db.ReadQuery(ctx, scope, planned.QueryID)
	if err != nil || strings.Contains(retained.Question, "alias-secret-731") || strings.Contains(retained.Route.Request.Question, "alias-secret-731") {
		t.Fatal("private question retained for replay", err)
	}
	if len(retained.Parameters) != 1 || retained.Parameters[0].Value != "cw-alpha-731" {
		t.Fatal("private customer oracle changed")
	}
	// Rebuild the service before consuming persisted evidence. No in-memory
	// producer seal survives this boundary.
	f.query, _ = newPhase18Service(t, f.phase17Fixture)
	client, server, _ = cw01HTTPClient(t, f)
	run, err := client.RunNLQ(ctx, sdk.NLQRunRequest{QueryID: planned.QueryID, Operation: "protected-private-trigger-run"})
	if err != nil {
		t.Fatal("stored protected applicability replay", err)
	}
	requireParameterIDs(t, run, "1")
	calls := f.model.requests.Load()
	attempts := count(t, support.Raw(t, f.f.dsn), `SELECT count(*) FROM chartworks.read_attempts`)
	again, err := client.RunNLQ(ctx, sdk.NLQRunRequest{QueryID: planned.QueryID, Operation: "protected-private-trigger-run"})
	if err != nil || f.model.requests.Load() != calls || count(t, support.Raw(t, f.f.dsn), `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
		t.Fatal("terminal replay issued new work", err)
	}
	requireParameterIDs(t, again, "1")
	oldRows, _ := json.Marshal(run.Execution.Result.Rows)
	newRows, _ := json.Marshal(again.Execution.Result.Rows)
	if string(oldRows) != string(newRows) {
		t.Fatal("terminal replay changed source rows")
	}
	assertProtectedStoredReceipt(t, f, "protected-private-trigger-run", run.Execution)
	// JSON-produced witness coordinates cannot be inserted as a new authority row.
	forged := retained
	forged.ID = "forged-protected-applicability"
	forged.Parent, forged.ParentDigest, forged.PlanOperation, forged.PlanRequestDigest, forged.Operation = "", "", "", "", ""
	forged.ParentRevision = 0
	wire, _ := json.Marshal(retained.Route)
	forged.Route.Applicability = nil
	if json.Unmarshal(wire, &forged.Route) != nil {
		t.Fatal("forged fixture")
	}
	forged.Route.Applicability.Query = forged.ID
	rowsBefore := count(t, support.Raw(t, f.f.dsn), `SELECT count(*) FROM chartworks.nlq_queries`)
	if err := f.f.db.CreateQuery(ctx, scope, forged); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("JSON witness crossed guarded INSERT", err)
	}
	if count(t, support.Raw(t, f.f.dsn), `SELECT count(*) FROM chartworks.nlq_queries`) != rowsBefore {
		t.Fatal("forged origin persisted")
	}
	for _, foreign := range []struct{ actor, session string }{{"foreign-reader", f.e.Session()}, {f.e.User(), "foreign-session"}} {
		e := phase18Envelope(t, f.phase17Fixture, foreign.actor, foreign.session, true)
		if _, err := f.f.db.ReadClarificationApplicability(ctx, e, planned.QueryID, "query.execute"); err == nil {
			t.Fatal("foreign origin read")
		}
	}
	// Fresh raw wording cannot borrow the old form just because it redacts to
	// the same marker. Only the exact original or canonical question may resume.
	changedQuestion := question
	changedQuestion.Question = "Show second"
	if _, err := client.PlanNLQ(ctx, sdk.NLQPlanRequest{QuestionRequest: changedQuestion}); err == nil {
		t.Fatal("same-marker raw question borrowed origin")
	}
	if f.model.requests.Load() != calls {
		t.Fatal("origin denial called a model")
	}

	saved := nlqexec.SavedQuestion{Durability: "session_bound", Context: f.context, Topics: []nlqexec.SavedTopic{{Topic: f.pack.Topic, Version: f.published.State.Version, Digest: f.published.Digest}}, Query: planned.QueryID}
	evidence, err := f.query.InspectSaved(ctx, f.e, saved)
	if err != nil {
		t.Fatal("saved origin inspection", err)
	}
	copy, err := f.query.PrepareSaved(ctx, f.e, saved, evidence, "protected-applicability-saved", "en")
	if err != nil || f.model.requests.Load() != calls {
		protectedSavedFailure(t, f, planned.QueryID, err)
	}
	copyRow, err := f.f.db.ReadQuery(ctx, scope, copy.Query)
	if err != nil || copyRow.Route.Applicability == nil || copyRow.Route.Applicability.Query != copy.Query || copyRow.Route.Applicability.PriorQuery != planned.QueryID {
		t.Fatal("copy origin identity", err)
	}
	savedRun, err := f.query.RunSaved(ctx, f.e, saved, evidence, copy, 100, 1<<20, false)
	if err != nil || savedRun.Execution.Result == nil || len(savedRun.Execution.Result.Rows) != 1 || f.model.requests.Load() != calls {
		t.Fatal("saved private oracle", err)
	}
	savedAgain, err := f.query.RunSaved(ctx, f.e, saved, evidence, copy, 100, 1<<20, false)
	if err != nil || savedAgain.Execution.Attempt.ID != savedRun.Execution.Attempt.ID || f.model.requests.Load() != calls {
		t.Fatal("saved zero-model replay", err)
	}

	corrected := f.answer(t, "customer", cw01Text("second"))
	child, err := client.RefineNLQ(ctx, sdk.NLQRefineRequest{QueryID: planned.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{corrected}}})
	if err != nil || child.QueryID == "" {
		t.Fatal("protected refinement", err)
	}
	childRow, err := f.f.db.ReadQuery(ctx, scope, child.QueryID)
	if err != nil || len(childRow.Parameters) != 1 || childRow.Parameters[0].Value != "cw-beta-731" {
		t.Fatal("refined parameter oracle", err)
	}
	childRun, err := client.RunNLQ(ctx, sdk.NLQRunRequest{QueryID: child.QueryID, Operation: "protected-private-child-run"})
	if err != nil {
		t.Fatal("refined run", err)
	}
	requireParameterIDs(t, childRun, "2")
	calls = f.model.requests.Load()
	childAttempts := count(t, support.Raw(t, f.f.dsn), `SELECT count(*) FROM chartworks.read_attempts`)
	childAgain, err := client.RunNLQ(ctx, sdk.NLQRunRequest{QueryID: child.QueryID, Operation: "protected-private-child-run"})
	if err != nil || f.model.requests.Load() != calls || count(t, support.Raw(t, f.f.dsn), `SELECT count(*) FROM chartworks.read_attempts`) != childAttempts {
		t.Fatal("refined zero-model replay", err)
	}
	requireParameterIDs(t, childAgain, "2")
	assertProtectedStoredReceipt(t, f, "protected-private-child-run", childRun.Execution)
	// Reading an owned origin cannot add missing current source-query grants.
	var limitedScopes []string
	for _, scope := range phase18Scopes(f.e.Tenant(), true) {
		if scope != "sources.query" && scope != "cw.source.query:*" {
			limitedScopes = append(limitedScopes, scope)
		}
	}
	claims := f.model.token.claims(f.e.Tenant(), f.e.User(), limitedScopes)
	claims["session"] = f.e.Session()
	limitedToken := f.model.token.sign(t, claims, nil)
	limitedClient, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return limitedToken, nil })
	if err != nil {
		t.Fatal(err)
	}
	_, err = limitedClient.RunNLQ(ctx, sdk.NLQRunRequest{QueryID: child.QueryID, Operation: "protected-private-child-run"})
	var denial *sdk.StatusError
	if !errors.As(err, &denial) || denial.Status != 403 && denial.Status != 404 || f.model.requests.Load() != calls {
		t.Fatal("origin reader granted source authority", err)
	}
	// A new current policy cannot consume an old private-only term witness.
	changedPolicy := definition
	changedPolicy.Version = "protected-trigger-v3"
	changedPolicy.Patterns[0].Policy.When.AnyTerms = []string{"different reviewed trigger"}
	f.publishRules(t, changedPolicy, 2)
	_, err = client.RunNLQ(ctx, sdk.NLQRunRequest{QueryID: child.QueryID, Operation: "protected-private-child-run"})
	if err == nil || f.model.requests.Load() != calls {
		t.Fatal("changed policy consumed stale protected origin", err)
	}

}

func TestProtectedApplicabilityNumericAndNullSDKAcceptance(t *testing.T) {
	for _, kind := range []string{"number", "null"} {
		t.Run(kind, func(t *testing.T) {
			f := newCW01Fixture(t)
			ctx := context.Background()
			definition := f.definition
			definition.Version = "protected-" + kind + "-v2"
			pattern := cw01Pattern(t, definition, "amount-default")
			pattern.Slots[0].Sensitivity = semantics.LiteralSensitive
			pattern.Policy.When.AnyTerms = []string{"20"}
			pattern.Slots[0].Default = &semantics.ClarificationValue{Number: &semantics.ClarificationNumberInput{Value: "20.000", Unit: "USD"}}
			text := "Show checked 20"
			if kind == "null" {
				customer := cw01Pattern(t, definition, "customer")
				customer.Policy.When.AnyTerms = []string{"alias-secret-731"}
				nullSlot := pattern.Slots[0]
				nullSlot.ID = "missing_amount"
				nullSlot.Default = &semantics.ClarificationValue{Null: true}
				nullSlot.Effect.Nulls = "only"
				customer.Slots = append(customer.Slots, nullSlot)
				customer.Targets = append(customer.Targets, nullSlot.Effect.Target)
				pattern = customer
				text = "Show alias-secret-731"
				if _, err := f.f.admin.Exec(ctx, `UPDATE analytics.sales SET amount=NULL WHERE id=1`); err != nil {
					t.Fatal(err)
				}
			}
			definition.Patterns = []semantics.ClarificationPattern{pattern}
			f.publishRules(t, definition, 1)
			f.definition = definition
			client, _, _ := cw01HTTPClient(t, f)
			question := f.question(text, nlq.LanguageEnglish)
			pending, err := client.PreflightNLQ(ctx, sdk.NLQPreflightRequest{QuestionRequest: question})
			if err != nil {
				protectedPreflightFailure(t, f, question, err)
			}
			question.Question = pending.Route.Request.Question
			question.AnswerContext = pending.Route.AnswerContext
			question.ClarificationQuery = pending.QueryID
			if kind == "null" {
				question.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("alias-secret-731"))}
			}
			plan, err := client.PlanNLQ(ctx, sdk.NLQPlanRequest{QuestionRequest: question})
			if err != nil {
				t.Fatal("typed plan", err)
			}
			scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
			retained, err := f.f.db.ReadQuery(ctx, scope, plan.QueryID)
			if err != nil || retained.Route.Applicability == nil {
				t.Fatal("retained typed origin", err)
			}
			if kind == "number" && (len(retained.Parameters) != 1 || retained.Parameters[0].Value != "20") {
				t.Fatal("exact numeric oracle")
			}
			if kind == "null" {
				nulls := 0
				for _, resolution := range retained.Route.Resolutions {
					if resolution.Null {
						nulls++
					}
				}
				if nulls != 1 || len(retained.Parameters) != 1 || retained.Parameters[0].Value != "cw-alpha-731" {
					t.Fatal("explicit NULL oracle")
				}
			}
			run, err := client.RunNLQ(ctx, sdk.NLQRunRequest{QueryID: plan.QueryID, Operation: "protected-" + kind + "-run"})
			if err != nil {
				t.Fatal("typed run", err)
			}
			requireParameterIDs(t, run, "1")
			calls := f.model.requests.Load()
			attempts := count(t, support.Raw(t, f.f.dsn), `SELECT count(*) FROM chartworks.read_attempts`)
			again, err := client.RunNLQ(ctx, sdk.NLQRunRequest{QueryID: plan.QueryID, Operation: "protected-" + kind + "-run"})
			if err != nil || f.model.requests.Load() != calls || count(t, support.Raw(t, f.f.dsn), `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("typed zero-model replay", err)
			}
			requireParameterIDs(t, again, "1")
			assertProtectedStoredReceipt(t, f, "protected-"+kind+"-run", run.Execution)
		})
	}
}

func protectedPreflightFailure(t *testing.T, f *cw01Fixture, question nlqexec.QuestionRequest, cause error) {
	t.Helper()
	status, code := 0, ""
	var wire *sdk.StatusError
	if errors.As(cause, &wire) {
		status, code = wire.Status, wire.Code
	}
	route, routeErr := f.service.Route(t.Context(), f.e, nlqroute.RouteRequest{Topic: question.Topic, Context: question.Context, Locale: question.Locale, Question: question.Question, Kinds: question.Kinds, LimitPerKind: question.LimitPerKind})
	bound, bindErr := route.BindApplicabilityQuery(f.e, "diagnostic-query")
	valid := bound.ApplicabilityWriteValid(f.e.Tenant(), f.e.User(), f.e.Session(), "diagnostic-query")
	_, directErr := f.query.Preflight(t.Context(), f.e, nlqexec.PreflightRequest{QuestionRequest: question})
	t.Fatalf("SDK preflight failed: status=%d code=%s; route=%v bind=%v write_valid=%v direct=%T %v", status, code, routeErr, bindErr, valid, directErr, directErr)
}

// Ordinary Run's historical terminal projection intentionally omits the Attempt
// object. Verify its real immutable journal receipt through the authenticated
// seam already used by RunSaved, rather than inventing a new API contract.
func assertProtectedStoredReceipt(t *testing.T, f *cw01Fixture, operation string, first readexec.ExecutionReport) {
	t.Helper()
	stored, err := f.f.db.ReadSavedAttempt(t.Context(), f.e, operation)
	if err != nil || first.Attempt.ID == "" || stored.ID != first.Attempt.ID || stored.Manifest.Operation != operation || stored.Manifest.Session != f.e.Session() || stored.Manifest.Receipt.Context != f.context || stored.RemoteState != "stopped" || stored.Finished == nil || readexec.Hash(stored.Manifest) != readexec.Hash(first.Attempt.Manifest) {
		t.Fatal("terminal replay changed its authenticated source receipt", err)
	}
}

// Compatibility with existing SDK clients requires an exact original-text
// resubmission, while a different private phrase with the same visible redaction
// must never borrow that protected preflight.
func TestProtectedExactOriginalQuestionSDKAcceptance(t *testing.T) {
	for _, privateOnly := range []bool{false, true} {
		name := "public_trigger"
		if privateOnly {
			name = "private_trigger"
		}
		t.Run(name, func(t *testing.T) {
			f := newCW01Fixture(t)
			ctx := context.Background()
			if privateOnly {
				definition := f.definition
				definition.Version = "exact-original-v2"
				customer := cw01Pattern(t, definition, "customer")
				customer.Policy.When.AnyTerms = []string{"alias-secret-731"}
				definition.Patterns = []semantics.ClarificationPattern{customer}
				f.publishRules(t, definition, 1)
				f.definition = definition
			}
			client, _, _ := cw01HTTPClient(t, f)
			question := f.question("Show named sales for alias-secret-731", nlq.LanguageEnglish)
			pending, err := client.PreflightNLQ(ctx, sdk.NLQPreflightRequest{QuestionRequest: question})
			if err != nil || pending.Route.Clarification == nil {
				t.Fatal("preflight", err)
			}
			if strings.Contains(pending.Route.Request.Question, "alias-secret-731") {
				t.Fatal("preflight retained original secret")
			}
			question.AnswerContext = pending.Route.AnswerContext
			question.ClarificationQuery = pending.QueryID
			question.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("alias-secret-731"))}
			plan, err := client.PlanNLQ(ctx, sdk.NLQPlanRequest{QuestionRequest: question})
			if err != nil || plan.QueryID == "" {
				var status *sdk.StatusError
				reason := ""
				if errors.As(err, &status) && status.Clarification != nil {
					reason = status.Clarification.Reason
				}
				t.Fatalf("exact original private question failed to resume: reason=%s error=%v", reason, err)
			}
			before := f.model.requests.Load()
			question.Question = "Show named sales for second"
			if _, err := client.PlanNLQ(ctx, sdk.NLQPlanRequest{QuestionRequest: question}); err == nil || f.model.requests.Load() != before {
				t.Fatal("same-marker private question borrowed another origin", err)
			}
		})
	}
}

// Diagnose only closed admission predicates; never print the protected record.
func protectedSavedFailure(t *testing.T, f *cw01Fixture, id string, cause error) {
	t.Helper()
	parent, err := f.f.db.ReadSavedQuery(t.Context(), f.e, id, false)
	if err != nil {
		t.Fatal("saved parent diagnostic", err)
	}
	child := parent
	child.Route, err = f.service.WithApplicabilityReader(f.f.db).ReissueApplicabilityForCopy(t.Context(), f.e, id, parent.Route)
	if err != nil {
		t.Fatal("saved route diagnostic", err)
	}
	child.ID, child.Parent, child.SavedCopyParent, child.Operation = "saved-diagnostic", parent.ID, parent.ID, "saved-diagnostic-operation"
	child.Route, err = child.Route.BindApplicabilityQuery(f.e, child.ID)
	if err != nil {
		t.Fatal("saved bind diagnostic", err)
	}
	child.PlanOperation, child.PlanRequestDigest = "", ""
	child.IntentReview, child.GenerationResolution = nil, nil
	child.ParentRevision, child.ParentDigest = parent.Revision, nlqexec.QueryLineageDigest(parent)
	child.Status, child.Result, child.Revision, child.ExecutionFixes = "planned", nil, 1, 0
	t.Fatalf("saved origin reseal: %v; analytical=%t pending=%t review=%t submission=%t derivation=%t meaning=%t creation=%t write=%t ancestry=%t", cause, nlqexec.AnalyticalRecordValid(child), nlqexec.GenerationPendingValid(child), nlqexec.IntentReviewValid(child), nlqexec.PlanSubmissionValid(child), nlqexec.SavedDerivationValid(child), nlqexec.SavedDerivationMeaningEqual(child, parent), nlqexec.SavedDerivationCreationValid(child, parent), child.Route.ApplicabilityWriteValid(f.e.Tenant(), f.e.User(), f.e.Session(), child.ID), child.Route.Applicability.PriorDigest == readexec.Hash(parent.Route.Applicability))
}
