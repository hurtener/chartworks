package nlqexec

import (
	"testing"

	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestSavedCanonicalReferenceChoices(t *testing.T) {
	fixture := func() (QueryRecord, SavedQuestion) {
		ref := semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}
		resolution := semantics.ClarificationResolution{SchemaVersion: semantics.ClarificationSchemaVersion, Topic: "sales", TopicVersion: "v1", RulesetVersion: "rules-v1", Pattern: "metric", PatternVersion: "v1", Slot: "measure", Provenance: "answer", Reference: &ref, Value: "revenue-option"}
		resolution.ID = semantics.ClarificationResolutionDigest(resolution)
		q := QueryRecord{Topics: []string{"sales"}, TopicVersions: []string{"v1"}, RuleVersions: []string{"rules-v1"}}
		q.Route.Resolutions = []semantics.ClarificationResolution{resolution}
		q.Route.Request.Answers = semantics.CanonicalClarificationAnswers(q.Route.Resolutions)
		in := SavedQuestion{Durability: "replayable", Selections: &SavedSelections{Choices: []nlqroute.ChoiceSelection{{Pattern: "metric", Slot: "measure", Value: "revenue-option"}}}}
		return q, in
	}
	t.Run("exact-reviewed-reference", func(t *testing.T) {
		q, in := fixture()
		if !savedSelectionsMatch(q, in) {
			t.Fatal("normalized exact reference choice lost saved identity")
		}
		in.Selections.Choices[0].Pattern = ""
		if !savedSelectionsMatch(q, in) {
			t.Fatal("unambiguous legacy slot could not match its pinned reference")
		}
	})
	t.Run("retained-legacy-record", func(t *testing.T) {
		q, in := fixture()
		q.Route.Resolutions, q.Route.Request.Answers = nil, nil
		q.Route.Request.Choices = append([]nlqroute.ChoiceSelection(nil), in.Selections.Choices...)
		if !savedSelectionsMatch(q, in) {
			t.Fatal("unchanged retained legacy record lost exact identity")
		}
	})
	for _, name := range []string{"changed-option", "missing-resolution", "foreign-topic", "stale-topic", "stale-rule", "unsealed-value", "typed-scalar", "unexpected-answer", "duplicate-choice", "mixed-representation"} {
		t.Run(name, func(t *testing.T) {
			q, in := fixture()
			r := &q.Route.Resolutions[0]
			switch name {
			case "changed-option":
				in.Selections.Choices[0].Value = "another-option"
			case "missing-resolution":
				q.Route.Resolutions = nil
			case "foreign-topic":
				r.Topic = "another-topic"
			case "stale-topic":
				r.TopicVersion = "v2"
			case "stale-rule":
				r.RulesetVersion = "rules-v2"
			case "unsealed-value":
				r.Value = "another-option"
			case "typed-scalar":
				r.Reference = nil
				r.Effect = &semantics.ClarificationEffect{Kind: "number", Unit: "USD"}
				r.Value = "20"
			case "unexpected-answer":
				in.Selections.Choices = nil
			case "duplicate-choice":
				in.Selections.Choices = append(in.Selections.Choices, in.Selections.Choices[0])
			case "mixed-representation":
				q.Route.Request.Choices = in.Selections.Choices
			}
			if name != "missing-resolution" {
				if name != "unsealed-value" {
					r.ID = semantics.ClarificationResolutionDigest(*r)
				}
				q.Route.Request.Answers = semantics.CanonicalClarificationAnswers(q.Route.Resolutions)
			}
			if savedSelectionsMatch(q, in) {
				t.Fatal("saved selection accepted incompatible or unrequested clarification evidence")
			}
		})
	}
}
