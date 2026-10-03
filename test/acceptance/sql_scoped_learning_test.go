package acceptance

import (
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

// This producer witness uses real authenticated feedback/native validation. It
// does not imply activation, current-route consumption or live learning quality.
func assertScopedLearningBase(t *testing.T, service *nlqexec.Service, actor identity.Envelope, topic, query, sql, policy string) {
	t.Helper()
	if err := service.Feedback(t.Context(), actor, nlqexec.FeedbackRequest{QueryID: query, Verdict: "positive"}); err != nil {
		t.Fatal("scoped feedback", err)
	}
	examples, err := service.Examples(t.Context(), actor, topic, 8)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, example := range examples {
		if example.SQL != sql {
			continue
		}
		found++
		if example.Origin.BindingPolicy != policy || example.ParameterSchema != nil || example.State != "candidate" || strings.Contains(example.Question, "2026") || strings.Contains(example.Question, "2025") {
			t.Fatal("scoped base lost reviewed custody or retained historical period")
		}
	}
	if found != 1 {
		t.Fatalf("expected one authenticated scoped base, found %d", found)
	}
}
