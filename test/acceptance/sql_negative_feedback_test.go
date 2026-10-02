package acceptance

import (
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"testing"
)

func TestSQLRecoveryNegativeFailureObservationAcceptance(t *testing.T) {
	f := newPhase18Fixture(t)
	query, _ := newPhase18Service(t, f)
	e := phase18Envelope(t, f, f.f.e.User(), "negative-review", true)
	f.model.mode.Store(phase18RawResponse(t, "SELECT sum(amount) AS revenue FROM analytics.sales"))
	p, err := query.Plan(t.Context(), e, nlqexec.PlanRequest{QuestionRequest: phase18Question(f, nlq.LanguageEnglish, f.pack.Topic)})
	if err != nil {
		t.Fatal(err)
	}
	metadata := support.Raw(t, f.f.dsn)
	before := f.model.requests.Load()
	attempts := count(t, metadata, "SELECT count(*) FROM chartworks.read_attempts")
	// Emulate an owned retained failed query from a pre-analytical policy.
	// It is deliberately not presented as a valid current plan or receipt.
	scope, _ := store.NewScope(e.Tenant(), e.User())
	historical, err := f.f.db.ReadQuery(t.Context(), scope, p.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	historical.ID = readexec.Hash([]string{p.QueryID, "failed-historical-sql"})[:32]
	historical.Operation = ""
	historical.SQL = "SELECT no_such_reviewed_column FROM analytics.sales"
	historical.Status = "failed"
	historical.AnalyticalVersion = 0
	historical.Analytical = nil
	if err = f.f.db.CreateQuery(t.Context(), scope, historical); err != nil {
		t.Fatal("historical failed fixture", err)
	}
	p.QueryID = historical.ID
	in := nlqexec.FeedbackRequest{QueryID: p.QueryID, Verdict: "negative", Note: "The query failed against the source; retain this observation"}
	for i := 0; i < 2; i++ {
		if err = query.Feedback(t.Context(), e, in); err != nil {
			t.Fatal("invalid source SQL erased negative feedback", err)
		}
	}
	if count(t, metadata, "SELECT count(*) FROM chartworks.nlq_feedback") != 1 || count(t, metadata, "SELECT count(*) FROM chartworks.nlq_examples") != 0 || count(t, metadata, "SELECT count(*) FROM chartworks.read_attempts") != attempts || f.model.requests.Load() != before {
		t.Fatal("negative observation gained example, inference or execution effect")
	}
	if err = query.Feedback(t.Context(), e, nlqexec.FeedbackRequest{QueryID: p.QueryID, Verdict: "positive"}); err == nil {
		t.Fatal("invalid positive learned")
	}
	if count(t, metadata, "SELECT count(*) FROM chartworks.nlq_feedback") != 1 {
		t.Fatal("invalid positive feedback retained as approved evidence")
	}
}
