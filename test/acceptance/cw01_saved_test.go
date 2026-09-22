package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// Retained query consumers must round-trip protected clarification evidence,
// not merely the public route projection or a previously formatted SQL string.
func cw01TypedSavedQueryAcceptance(t *testing.T) {
	t.Helper()
	t.Run("reference-choice-replay", cw01SavedReferenceAcceptance)
	f := newCW01Fixture(t)
	ctx := context.Background()
	planned := f.plan(t, f.question("Show large sales", nlq.LanguageEnglish), "amount-required", cw01Number("20"))
	saved := nlqexec.SavedQuestion{Durability: "session_bound", Context: f.context,
		Topics: []nlqexec.SavedTopic{{Topic: f.pack.Topic, Version: f.published.State.Version, Digest: f.published.Digest}}, Query: planned.QueryID}
	beforeModels := f.model.requests.Load()
	attempts := func() int {
		var count int
		if err := support.Raw(t, f.f.dsn).QueryRow(ctx, `SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1`, f.e.Tenant()).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	beforeAttempts := attempts()
	evidence, err := f.query.InspectSaved(ctx, f.e, saved)
	if err != nil {
		t.Fatalf("typed saved inspection: %T %v", err, err)
	}
	copy, err := f.query.PrepareSaved(ctx, f.e, saved, evidence, "cw01-saved-typed", "en")
	if err != nil || copy.Query == "" || copy.Query == planned.QueryID || copy.BindingDigest == "" {
		t.Fatalf("typed saved preparation: %T %v", err, err)
	}
	if f.model.requests.Load() != beforeModels || attempts() != beforeAttempts {
		t.Fatal("retained typed preparation repeated generation or source execution")
	}
	result, err := f.query.RunSaved(ctx, f.e, saved, evidence, copy, 100, 1<<20, false)
	if err != nil || result.Execution.Result == nil || len(result.Execution.Result.Rows) != 1 || result.Execution.Attempt.ID == "" {
		t.Fatalf("typed saved execution lost its constraint or real receipt: %T %v", err, err)
	}
	raw, err := json.Marshal(result.Execution.Result.Rows)
	if err != nil || !strings.Contains(string(raw), "9007199254740993.125") {
		t.Fatal("typed saved result did not preserve the exact selected value")
	}
	if attempts() != beforeAttempts+1 || f.model.requests.Load() != beforeModels {
		t.Fatal("retained typed execution repeated planning or source attempts")
	}
	replayed, err := f.query.RunSaved(ctx, f.e, saved, evidence, copy, 100, 1<<20, false)
	if err != nil || replayed.Execution.Attempt.ID != result.Execution.Attempt.ID || attempts() != beforeAttempts+1 || f.model.requests.Load() != beforeModels {
		t.Fatalf("typed saved replay repeated work or discarded its receipt: %T %v", err, err)
	}
}

func cw01SavedReferenceAcceptance(t *testing.T) {
	t.Helper()
	f := newCW01Fixture(t)
	ctx := context.Background()
	saved := nlqexec.SavedQuestion{Durability: "replayable", Context: f.context, Question: "Show choose sales",
		Topics:     []nlqexec.SavedTopic{{Topic: f.pack.Topic, Version: f.published.State.Version, Digest: f.published.Digest}},
		Selections: &nlqexec.SavedSelections{Kinds: []string{"measure"}, LimitPerKind: 1, Choices: []nlqroute.ChoiceSelection{{Pattern: "metric", Slot: "metric", Value: "revenue-option"}}}}
	evidence, err := f.query.InspectSaved(ctx, f.e, saved)
	if err != nil {
		t.Fatal("saved reference inspection", err)
	}
	plan, err := f.query.PrepareSaved(ctx, f.e, saved, evidence, "cw01-saved-reference", "en")
	if err != nil || plan.Query == "" {
		t.Fatal("saved reference normalization did not reach a validated plan", err)
	}
	before := f.model.requests.Load()
	replayed, err := f.query.PrepareSaved(ctx, f.e, saved, evidence, "cw01-saved-reference", "en")
	if err != nil || replayed != plan || f.model.requests.Load() != before {
		t.Fatal("exact saved reference replay repeated planning", err)
	}
	changed := phase27Copy(t, saved)
	changed.Selections.Choices[0].Value = "amount-option"
	changedEvidence, err := f.query.InspectSaved(ctx, f.e, changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.query.PrepareSaved(ctx, f.e, changed, changedEvidence, "cw01-saved-reference", "en"); !errors.Is(err, store.ErrConflict) || f.model.requests.Load() != before {
		t.Fatal("saved reference replay accepted a changed reviewed option", err)
	}
	result, err := f.query.RunSaved(ctx, f.e, saved, evidence, plan, 100, 1<<20, false)
	if err != nil || result.Execution.Result == nil || len(result.Execution.Result.Rows) != 2 || result.Execution.Attempt.ID == "" {
		t.Fatal("saved reference lost the actual query result or receipt", err)
	}
}
