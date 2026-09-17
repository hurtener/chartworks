"""Apply inspected saved-query projection and nondisclosing export regressions."""
from pathlib import Path

pending = {}
def edit(path, before, after):
    text = pending.get(path, Path(path).read_text())
    if text.count(before) != 1:
        raise SystemExit(f"{path}: reviewed anchor changed")
    pending[path] = text.replace(before, after, 1)

# The shared scanner now reads the appended protected clarification column.
# This reader continues to omit result values and filter current actor/session
# and signed context reach in SQL, before returning protected query evidence.
edit("internal/store/postgres/saved_question.go",
     "validation_fixes,execution_fixes,revision,created_at,updated_at\n FROM chartworks.nlq_queries",
     "validation_fixes,execution_fixes,revision,created_at,updated_at,clarification\n FROM chartworks.nlq_queries")

path = "test/acceptance/cw01_consumers_test.go"
edit(path,
     '''		name, remove string
	}{
		{"missing-export-action", "topics.export"},
		{"missing-export-resource", "cw.topic.export:*"},''',
     '''		name, remove string
		wantError error
		wantStatus int
	}{
		{"missing-export-action", "topics.export", access.ErrForbidden, http.StatusForbidden},
		{"missing-export-resource", "cw.topic.export:*", access.ErrNotFound, http.StatusNotFound},''')
edit(path,
     '!errors.Is(err, access.ErrForbidden) || out.RuleDigest != "" || out.Definition.Topic != ""',
     '!errors.Is(err, denied.wantError) || out.RuleDigest != "" || out.Definition.Topic != ""')
edit(path,
     'status.Status != http.StatusForbidden || out.RuleDigest != "" || out.Definition.Topic != ""',
     'status.Status != denied.wantStatus || out.RuleDigest != "" || out.Definition.Topic != ""')
edit(path,
     't.Fatal("direct export did not enforce signed export reach")',
     't.Fatalf("direct export did not preserve the nondisclosing authority contract: %T %v", err, err)')
edit(path,
     't.Fatal("HTTP export did not enforce signed export reach")',
     't.Fatalf("HTTP export did not preserve the nondisclosing authority contract: %T %v", err, err)')

edit("test/acceptance/cw01_test.go",
     '\t\tt.Run("consumers", cw01ConsumerAcceptance)',
     '\t\tt.Run("consumers", cw01ConsumerAcceptance)\n\t\tt.Run("saved-query", TestSavedQuestionReplayable)\n\t\tt.Run("saved-selection-identity", TestSavedQuestionClarificationAndSelectionIdentity)\n\t\tt.Run("typed-saved-query", cw01TypedSavedQueryAcceptance)')

path = "test/acceptance/cw01_saved_test.go"
if Path(path).exists():
    raise SystemExit("new saved consumer regression already exists")
pending[path] = '''package acceptance

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/test/support"
)

// Retained query consumers must round-trip protected clarification evidence,
// not merely the public route projection or a previously formatted SQL string.
func cw01TypedSavedQueryAcceptance(t *testing.T) {
	t.Helper()
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
'''

for path, text in sorted(pending.items()):
    Path(path).write_text(text)
print("Applied saved query projection and nondisclosing export/consumer regressions")
