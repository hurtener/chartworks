package acceptance

import (
	"fmt"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
	"strings"
	"testing"
	"time"
)

func TestSQLRecoveryEligibleExampleRetrievalAcceptance(t *testing.T) {
	f := newPhase18Fixture(t)
	query, _ := newPhase18Service(t, f)
	actor := phase18Envelope(t, f, f.f.e.User(), "example-retrieval", true)
	sql := "SELECT sum(amount) AS revenue FROM analytics.sales"
	f.model.mode.Store(phase18RawResponse(t, sql))
	question := phase18Question(f, nlq.LanguageEnglish, f.pack.Topic)
	first, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: question})
	if err != nil {
		t.Fatal(err)
	}
	if err = query.Feedback(t.Context(), actor, nlqexec.FeedbackRequest{QueryID: first.QueryID, Verdict: "positive"}); err != nil {
		t.Fatal(err)
	}
	examples, err := query.Examples(t.Context(), actor, f.pack.Topic, 8)
	if err != nil || len(examples) != 1 {
		t.Fatal("base example", err)
	}
	current, err := query.ExampleState(t.Context(), actor, nlqexec.ExampleStateRequest{ExampleID: examples[0].ID, ExpectedVersion: examples[0].Version, State: "active", ReviewNote: "Synthetic reviewed exact revenue demonstration"})
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := store.NewScope(actor.Tenant(), actor.User())
	current, err = f.f.db.ReadExample(t.Context(), scope, current.ID)
	if err != nil {
		t.Fatal("protected reviewed example", err)
	}
	for _, kind := range []string{"candidate", "stale", "irrelevant"} {
		for i := 0; i < 65; i++ {
			x := current
			x.ID = readexec.Hash([]any{kind, i})[:32]
			x.Question = fmt.Sprintf("Approved historical analysis variant %s %d", kind, i)
			x.Digest = readexec.Hash([]string{kind, x.Question, x.SQL})
			x.State = "candidate"
			x.Version = 1
			x.ReviewedAt = nil
			x.ReviewedBy = ""
			x.ReviewNote = ""
			x.Weight = 100.0 / 101
			x.PositiveEvidence = 99
			x.NegativeEvidence = 0
			x.EvidenceCount = 99
			x.EvidenceOutcome = "positive"
			x.Uncertainty = 0.1
			x.Provenance = "synthetic-retained-review"
			x.Created = time.Now().UTC()
			x.Updated = x.Created
			if kind == "stale" {
				x.Origin.TopicVersion = "retained-old-topic"
				x.Question = "STALE_CONTEXT_CANARY " + x.Question
			}
			if x.ParameterSchema != nil {
				domain := "parameterized-example-v1"
				if x.ParameterSchema.Version == "example-parameters-v2" {
					domain = "parameterized-example-v2"
				}
				x.Digest = readexec.Hash([]any{domain, x.Topic, x.Question, x.SQL, x.ParameterSchema})
			}
			if !nlqexec.ExampleParametersValid(x) {
				t.Fatal("invalid synthetic example before storage")
			}
			x, err = f.f.db.UpsertExample(t.Context(), scope, x)
			if err != nil {
				t.Fatal("seed bounded historical cohort", kind, err, "valid", nlqexec.ExampleParametersValid(x), "sql_bytes", len(x.SQL), "origin", x.Origin.SchemaVersion, "schema", x.ParameterSchema != nil)
			}
			if kind != "candidate" {
				if _, err = f.f.db.SetExampleState(t.Context(), scope, nlqexec.ExampleStateRequest{ExampleID: x.ID, ExpectedVersion: x.Version, State: "active", ReviewNote: "Existing synthetic review receipt"}, "synthetic-reviewer"); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	// The legacy public list remains a review listing. It does not accidentally
	// become the generation search API or gain inferred current-origin authority.
	oldPage, err := f.f.db.ListExamples(t.Context(), scope, f.pack.Topic, 64)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range oldPage {
		if x.ID == current.ID {
			t.Fatal("fixture did not reproduce pre-limit starvation")
		}
	}
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.mu.Unlock()
	planned, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: question})
	if err != nil {
		t.Fatal("eligible retrieval plan", err)
	}
	saved, err := f.f.db.ReadQuery(t.Context(), scope, planned.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ExampleSelection.PolicyVersion != "current-eligible-fts-bayes-v2" || saved.ExampleSelection.Usage == nil {
		t.Fatal("missing actual retrieval policy/use evidence")
	}
	used := false
	for _, x := range saved.ExampleSelection.Usage.Used {
		used = used || x.ExampleID == current.ID
	}
	if !used {
		t.Fatal("relevant active example starved before rendered prompt", saved.ExampleSelection)
	}
	f.model.mu.Lock()
	wire := strings.Join(f.model.requestBodies[start:], "\n")
	f.model.mu.Unlock()
	if strings.Contains(wire, "STALE_CONTEXT_CANARY") {
		t.Fatal("stale rows reached model")
	}
	if _, err = query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: planned.QueryID + "-run"}); err != nil {
		t.Fatal("learned context weakened native execution", err)
	}
}
