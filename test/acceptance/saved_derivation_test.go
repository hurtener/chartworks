package acceptance

import (
	"context"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/generationdecision"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestSavedDerivationReviewedOrigins(t *testing.T) {
	for _, mode := range []string{"ordinary_upgrade", "generation_resolution", "intent_review"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			var f *cw01Fixture
			var root nlqexec.PlanResult
			var err error
			if mode == "ordinary_upgrade" {
				f = newCW01Fixture(t)
				f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`))
				root, err = f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue", nlq.LanguageEnglish), Operation: "ordinary-parent"})
			} else if mode == "generation_resolution" {
				f = newCW01Fixture(t)
				question := f.question("Revenue", nlq.LanguageEnglish)
				f.model.mode.Store(decisionRawResponse(t, generationdecision.Clarify, "Which reviewed metric?"))
				_, err = f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: question, Operation: "derive-pending"})
				pending := nlqexec.GenerationProblem(err)
				if pending == nil {
					t.Fatal("missing generation origin", err)
				}
				question.GenerationQuery, question.GenerationContext = pending.QueryID, pending.AnswerContext
				question.References = []semantics.Reference{{Kind: semantics.KindMeasure, ID: f.published.Definition.Measures[0].ID}}
				f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) AS revenue FROM analytics.sales`))
				root, err = f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: question, Operation: "resume:" + pending.QueryID})
			} else {
				f = groupingContinuationFixture(t)
				old := retainedIntentReviewFixture(t, f, nlq.LanguageEnglish)
				question := f.question("Revenue named sales by Status", nlq.LanguageEnglish)
				question.Grouping = groupingState(f, "status")
				pending := f.preflight(t, question)
				f.model.mode.Store(phase18RawResponse(t, `SELECT active,sum(amount) AS revenue FROM analytics.sales GROUP BY active ORDER BY active`))
				root, err = f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: old.ID, IntentReview: &nlqexec.LegacyIntentReview{QueryID: pending.QueryID, AnswerContext: pending.Route.AnswerContext, Answers: []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("primero"))}}})
			}
			if err != nil {
				t.Fatal("reviewed root", err)
			}
			sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
			parent, err := f.f.db.ReadQuery(ctx, sc, root.QueryID)
			if err != nil {
				t.Fatal(err)
			}
			before := nlqexec.QueryLineageDigest(parent)
			saved := nlqexec.SavedQuestion{Durability: "session_bound", Query: root.QueryID, Context: parent.Context, Topics: []nlqexec.SavedTopic{{Topic: f.published.Definition.Topic, Version: f.published.Definition.Version, Digest: f.published.Digest}}}
			evidence, err := f.query.InspectSaved(ctx, f.e, saved)
			if err != nil {
				t.Fatal(err)
			}
			calls := decisionChatCount(f)
			plans := make([]nlqexec.SavedPlan, 3)
			errs := make([]error, 3)
			var wg sync.WaitGroup
			for i := range plans {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					plans[i], errs[i] = f.query.PrepareSaved(ctx, f.e, saved, evidence, "derive-copy", "en")
				}(i)
			}
			wg.Wait()
			for i, e := range errs {
				if e != nil || plans[i] != plans[0] {
					t.Fatal("concurrent saved copy", i, e)
				}
			}
			child, err := f.f.db.ReadQuery(ctx, sc, plans[0].Query)
			if err != nil {
				t.Fatal(err)
			}
			if child.GenerationResolution != nil || child.IntentReview != nil || child.SavedCopyParent != parent.ID || child.ParentDigest != before {
				t.Fatal("copy claimed direct submission or lost origin")
			}
			forged := child
			forged.ID = "forged-saved-copy"
			forged.Operation = "forged-saved-operation"
			forged.SQL = `SELECT count(*) FROM analytics.sales`
			forged.Created = time.Now().UTC()
			forged.Updated = forged.Created
			if err := f.f.db.CreateQuery(ctx, sc, forged); err == nil {
				t.Fatal("altered executable copy persisted")
			}
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			result, err := f.query.RunSaved(ctx, f.e, saved, evidence, plans[0], 100, 1<<20, false)
			if err != nil {
				t.Fatal("restarted derived execution", err)
			}
			if mode != "intent_review" {
				requireExplanationSum(t, nlqexec.RunResult{Execution: result.Execution}, "9007199254740998.625")
			} else {
				groupingSums(t, nlqexec.RunResult{Execution: result.Execution}, "5", "10")
			}
			if mode == "generation_resolution" {
				// Seed the permitted persisted terminal-correction shape. Replay
				// must establish actual native equivalence, not trust that shape.
				corrected, err := f.f.db.ReadQuery(ctx, sc, plans[0].Query)
				if err != nil {
					t.Fatal(err)
				}
				expectedRevision := corrected.Revision
				corrected.SQL += " "
				proof := *corrected.Analytical
				proof.Query = readexec.AnalyticalQueryDigest(corrected.SQL, corrected.Parameters)
				corrected.Analytical, corrected.Revision = &proof, expectedRevision+1
				if err := f.f.db.UpdateQuery(ctx, sc, corrected, expectedRevision); err != nil {
					t.Fatal("terminal correction fixture", err)
				}
			}
			replay, err := f.query.RunSaved(ctx, f.e, saved, evidence, plans[0], 100, 1<<20, false)
			if err != nil || !reflect.DeepEqual(result.Execution.Result, replay.Execution.Result) {
				t.Fatal("derived execution replay", err)
			}
			second, err := f.query.PrepareSaved(ctx, f.e, saved, evidence, "derive-second-copy", "en")
			if err != nil || second.Query == plans[0].Query {
				t.Fatal("independent saved operation", err)
			}
			if _, err = f.query.RunSaved(ctx, f.e, saved, evidence, second, 100, 1<<20, false); err != nil {
				t.Fatal("second saved execution", err)
			}
			nested := saved
			nested.Query = plans[0].Query
			nestedEvidence, err := f.query.InspectSaved(ctx, f.e, nested)
			if err != nil {
				t.Fatal("nested evidence", err)
			}
			nestedPlan, err := f.query.PrepareSaved(ctx, f.e, nested, nestedEvidence, "derive-nested-copy", "en")
			if err != nil {
				t.Fatal("nested prepare", err)
			}
			if _, err = f.query.RunSaved(ctx, f.e, nested, nestedEvidence, nestedPlan, 100, 1<<20, false); err != nil {
				t.Fatal("nested execution", err)
			}
			if mode == "ordinary_upgrade" {
				// Reapply the additive migration over existing ordinary copies,
				// whose persisted shape is the unchanged 074 creation contract.
				migration, err := os.ReadFile("../../internal/store/postgres/migrations/076_saved_query_derivation.sql")
				if err != nil {
					t.Fatal(err)
				}
				raw := support.Raw(t, f.f.dsn)
				tx, err := raw.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(context.Background())
				if _, err = tx.Exec(ctx, `DROP TRIGGER b_saved_derivation_payload ON chartworks.nlq_queries; DROP FUNCTION chartworks.guard_saved_derivation_payload(); ALTER TABLE chartworks.nlq_queries DROP CONSTRAINT nlq_saved_derivation_shape`); err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(ctx, string(migration)); err != nil {
					t.Fatal("076 upgrade over ordinary saved copies", err)
				}
				if err = tx.Rollback(ctx); err != nil {
					t.Fatal(err)
				}
			}
			unchanged, err := f.f.db.ReadQuery(ctx, sc, parent.ID)
			if err != nil || nlqexec.QueryLineageDigest(unchanged) != before || decisionChatCount(f) != calls {
				t.Fatal("derivation changed origin or invoked provider", err)
			}
			if mode == "generation_resolution" {
				altered, err := f.f.db.ReadQuery(ctx, sc, second.Query)
				if err != nil {
					t.Fatal(err)
				}
				expectedRevision := altered.Revision
				altered.SQL = "SELECT count(*) AS revenue FROM analytics.sales"
				proof := *altered.Analytical
				proof.Query = readexec.AnalyticalQueryDigest(altered.SQL, altered.Parameters)
				altered.Analytical, altered.Revision = &proof, expectedRevision+1
				if err := f.f.db.UpdateQuery(ctx, sc, altered, expectedRevision); err != nil {
					t.Fatal("safe but different SQL fixture", err)
				}
				if out, err := f.query.RunSaved(ctx, f.e, saved, evidence, second, 100, 1<<20, false); err == nil || out.Execution.Result != nil {
					t.Fatal("different result meaning reused terminal rows", err)
				}
			}
			rootOperation := "derive-root-later-run"
			if parent.GenerationResolution != nil {
				rootOperation = parent.Operation
			}
			if _, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: parent.ID, Operation: rootOperation}); err != nil {
				t.Fatal("legitimate later parent run", err)
			}
			if stale, err := f.query.RunSaved(ctx, f.e, saved, evidence, plans[0], 100, 1<<20, false); err == nil || stale.Execution.Result != nil {
				t.Fatal("stale exact parent served copied rows", err)
			}
			if decisionChatCount(f) != calls {
				t.Fatal("stale derivation invoked provider")
			}
		})
	}
}
