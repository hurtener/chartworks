package acceptance

import (
	"testing"

	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/test/support"
)

// Rotation is isolated from publication/rule changes so a different failure
// cannot stand in for the current source/context custody fence.
func TestSQLRecoveryGroupedFactLearningSourceFences(t *testing.T) {
	f := newScopedRequalificationFixture(t, 5, false, false)
	ctx := t.Context()
	anchor := f.anchor(t, "2026", "PRIVATE_REQUAL_731")
	plan, err := f.query.Plan(ctx, f.actor, nlqexec.PlanRequest{QuestionRequest: anchor})
	if err != nil {
		t.Fatal(err)
	}
	run := nlqexec.RunRequest{QueryID: plan.QueryID, Operation: plan.QueryID + "-source-fence"}
	result, err := f.query.Run(ctx, f.actor, run)
	if err != nil {
		t.Fatal(err)
	}
	f.assertResult(t, result, "2026", "PRIVATE_REQUAL_731")
	candidate := assertScopedLearningBase(t, f.query, f.actor, f.pack.Topic, plan.QueryID, f.sql, nlqexec.ScopedGroupedFactExamplePolicy)
	bundle, err := f.query.ExportExamples(ctx, f.actor, nlqexec.ExampleExportRequest{Topic: f.pack.Topic, Limit: 8})
	if err != nil || len(bundle.Examples) != 1 {
		t.Fatal("current protected export", err)
	}
	// Establish a successful current import before rotation, without activation.
	if _, err = f.query.ImportExample(ctx, f.actor, nlqexec.ExampleImportRequest{Anchor: anchor, Example: bundle.Examples[0]}); err != nil {
		t.Fatal("current protected import", err)
	}
	source := f.pack.Datasets[0].Source
	rotated, err := f.f.s.Rotate(ctx, f.f.e, source.Source, source.SourceRevision)
	if err != nil || rotated.Revision == source.SourceRevision || rotated.ContextID == source.Context {
		t.Fatal("real source rotation", err)
	}
	metadata := support.Raw(t, f.f.dsn)
	calls, reads := f.model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	if _, err = f.query.ExampleState(ctx, f.actor, nlqexec.ExampleStateRequest{ExampleID: candidate.ID, ExpectedVersion: candidate.Version, State: "active", ReviewNote: "Review rotated fact source"}); err == nil {
		t.Fatal("rotated source activated historical base")
	}
	if _, err = f.query.ImportExample(ctx, f.actor, nlqexec.ExampleImportRequest{Anchor: anchor, Example: bundle.Examples[0]}); err == nil {
		t.Fatal("rotated source imported historical base")
	}
	if _, err = f.query.RequalifyExample(ctx, f.actor, nlqexec.ExampleRequalificationRequest{ExampleID: candidate.ID, ExpectedVersion: candidate.Version, Anchor: anchor}); err == nil {
		t.Fatal("rotated source qualified historical anchor")
	}
	if err = f.query.Feedback(ctx, f.actor, nlqexec.FeedbackRequest{QueryID: plan.QueryID, Verdict: "positive"}); err == nil {
		t.Fatal("rotated source learned historical query")
	}
	if _, err = f.query.Run(ctx, f.actor, run); err == nil {
		t.Fatal("rotated source exposed retained values")
	}
	if _, err = f.query.Plan(ctx, f.actor, nlqexec.PlanRequest{QuestionRequest: anchor}); err == nil {
		t.Fatal("rotated source selected old base")
	}
	if calls != f.model.requests.Load() || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) || count(t, metadata, `SELECT count(*) FROM chartworks.nlq_examples`) != 1 {
		t.Fatal("source denial performed model/row work or added evidence")
	}
}
