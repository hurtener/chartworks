package acceptance

import (
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

// This producer witness uses real authenticated feedback/native validation. It
// does not imply activation, current-route consumption or live learning quality.
func assertScopedLearningBase(t *testing.T, service *nlqexec.Service, actor identity.Envelope, topic, query, sql, policy string) nlqexec.ExampleRecord {
	t.Helper()
	if err := service.Feedback(t.Context(), actor, nlqexec.FeedbackRequest{QueryID: query, Verdict: "positive"}); err != nil {
		t.Fatal("scoped feedback", err)
	}
	examples, err := service.Examples(t.Context(), actor, topic, 8)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	var matched nlqexec.ExampleRecord
	for _, example := range examples {
		if example.SQL != sql {
			continue
		}
		found++
		matched = example
		if example.Origin.BindingPolicy != policy || example.ParameterSchema != nil || example.State != "candidate" || strings.Contains(example.Question, "2026") || strings.Contains(example.Question, "2025") {
			t.Fatal("scoped base lost reviewed custody or retained historical period")
		}
	}
	if found != 1 {
		t.Fatalf("expected one authenticated scoped base, found %d", found)
	}
	return matched
}

func activateScopedLearningBase(t *testing.T, service *nlqexec.Service, actor identity.Envelope, x nlqexec.ExampleRecord) nlqexec.ExampleRecord {
	t.Helper()
	active, err := service.ExampleState(t.Context(), actor, nlqexec.ExampleStateRequest{ExampleID: x.ID, ExpectedVersion: x.Version, State: "active", ReviewNote: "Reviewed value-free scoped population base"})
	if err != nil || active.State != "active" {
		t.Fatal("scoped activation", err)
	}
	return active
}

func assertScopedExampleUsage(t *testing.T, q nlqexec.QueryRecord, x nlqexec.ExampleRecord, schema int) {
	t.Helper()
	if q.Clarification == nil || q.Clarification.Binding.SchemaVersion != schema || q.ExampleSelection.Usage == nil || q.ExampleSelection.Eligibility == nil || q.ExampleSelection.Eligibility.CurrentScopedPolicy != x.Origin.BindingPolicy {
		t.Fatal("current scoped example custody missing")
	}
	for _, used := range q.ExampleSelection.Usage.Used {
		if used.ExampleID == x.ID {
			return
		}
	}
	t.Fatalf("scoped example not used: wanted=%s selected=%+v used=%+v omitted=%+v", x.ID, q.ExampleSelection.Selected, q.ExampleSelection.Usage.Used, q.ExampleSelection.Usage.Omitted)
}
