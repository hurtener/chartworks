package nlqexec

import (
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func savedSelectionFixture() SavedQuestion {
	return SavedQuestion{Durability: "replayable", Context: "warehouse", Question: "What is revenue?",
		Topics: []SavedTopic{{Topic: "sales", Version: "v1", Digest: strings.Repeat("a", 64)}}}
}

func TestSavedQuestionSelections(t *testing.T) {
	base := savedSelectionFixture()
	if err := ValidateSavedQuestion(base); err != nil {
		t.Fatal("legacy replayable question", err)
	}
	// Optional selectors do not change historical nil-selector wire digests.
	legacy := struct {
		Durability string       `json:"durability"`
		Context    string       `json:"context"`
		Topics     []SavedTopic `json:"topics"`
		Question   string       `json:"question,omitempty"`
		Query      string       `json:"query,omitempty"`
	}{base.Durability, base.Context, base.Topics, base.Question, base.Query}
	if exec.Hash(base) != exec.Hash(legacy) {
		t.Fatal("optional selections rewrote historical evidence identity")
	}
	for _, test := range []struct {
		name      string
		selection SavedSelections
	}{
		{"unknown-kind", SavedSelections{Kinds: []string{"sql"}}},
		{"duplicate-kind", SavedSelections{Kinds: []string{"measure", "measure"}}},
		{"negative-limit", SavedSelections{LimitPerKind: -1}},
		{"excessive-limit", SavedSelections{LimitPerKind: 11}},
		{"invalid-reference", SavedSelections{References: []semantics.Reference{{ID: "sales"}}}},
		{"duplicate-choice", SavedSelections{Choices: []nlqroute.ChoiceSelection{{Slot: "period", Value: "month"}, {Slot: "period", Value: "year"}}}},
		{"oversize-choice", SavedSelections{Choices: []nlqroute.ChoiceSelection{{Slot: "period", Value: strings.Repeat("x", 1025)}}}},
		{"single-topic-join", SavedSelections{Joins: []nlqroute.JoinChoice{{Topic: "sales", JoinID: "joined"}}}},
		{"duplicate-metric", SavedSelections{MetricIDs: []string{"revenue", "revenue"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			in := base
			in.Selections = &test.selection
			if ValidateSavedQuestion(in) == nil {
				t.Fatal("saved authoring bypassed the ordinary router's bounds")
			}
		})
	}
	valid := base
	valid.Selections = &SavedSelections{Kinds: []string{"measure"}, LimitPerKind: 1, MetricIDs: []string{"revenue"}}
	if ValidateSavedQuestion(valid) != nil || exec.Hash(valid) == exec.Hash(base) {
		t.Fatal("valid explicit choices were rejected or omitted from identity")
	}
	private := base
	private.Durability, private.Query, private.Question = "session_bound", "origin-query", ""
	if ValidateSavedQuestion(private) != nil {
		t.Fatal("exact session reference rejected")
	}
	private.Selections = &SavedSelections{}
	if ValidateSavedQuestion(private) == nil {
		t.Fatal("session-bound SQL could be reinterpreted with new routing inputs")
	}
}

func TestSavedQuestionSelectionReplay(t *testing.T) {
	in := savedSelectionFixture()
	in.Selections = &SavedSelections{Kinds: []string{"measure"}, LimitPerKind: 1}
	routing := savedRouting(in, nlq.LanguageEnglish)
	q := QueryRecord{SQL: "SELECT revenue FROM sales", Context: in.Context, Topics: []string{"sales"}, TopicVersions: []string{"v1"}}
	q.Route.Request = routing.routeRequest()
	if !savedRecordMatches(q, in) {
		t.Fatal("exact saved routing identity rejected")
	}
	changed := in
	changed.Selections = &SavedSelections{Kinds: []string{"measure"}, LimitPerKind: 2}
	if savedRecordMatches(q, changed) {
		t.Fatal("replayed operation adopted different retrieval semantics")
	}
	in.Selections.Kinds[0] = "dimension"
	if routing.Kinds[0] != "measure" {
		t.Fatal("routing retained mutable caller selection slices")
	}
	if savedRecordMatches(q, in) {
		t.Fatal("changed selected meaning reused an earlier plan")
	}
}
