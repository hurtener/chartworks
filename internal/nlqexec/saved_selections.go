package nlqexec

import (
	"slices"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

// SavedSelections retains explicit business routing choices, not SQL, prompts,
// source bindings or authority. The existing router resolves every choice
// against the pinned/current reviewed semantic definitions before generation.
type SavedSelections struct {
	Kinds        []string                   `json:"kinds,omitempty"`
	LimitPerKind int                        `json:"limit_per_kind,omitempty"`
	References   []semantics.Reference      `json:"references,omitempty"`
	Choices      []nlqroute.ChoiceSelection `json:"choices,omitempty"`
	Joins        []nlqroute.JoinChoice      `json:"joins,omitempty"`
	MetricIDs    []string                   `json:"metric_ids,omitempty"`
	Rerank       bool                       `json:"rerank,omitempty"`
}

func savedRouting(in SavedQuestion, language nlq.Language) QuestionRequest {
	q := QuestionRequest{Context: in.Context, Locale: language, Question: in.Question}
	for _, pin := range in.Topics {
		q.Topics = append(q.Topics, pin.Topic)
	}
	if in.Selections != nil {
		s := in.Selections
		q.Kinds, q.LimitPerKind = slices.Clone(s.Kinds), s.LimitPerKind
		q.References, q.Choices = slices.Clone(s.References), slices.Clone(s.Choices)
		q.Joins, q.MetricIDs, q.Rerank = slices.Clone(s.Joins), slices.Clone(s.MetricIDs), s.Rerank
	}
	return q
}

func savedSelectionsValid(in SavedQuestion) bool {
	if in.Durability == "session_bound" {
		// A session reference uses its original reviewed routing inputs. It
		// cannot reinterpret the referenced query through new selections.
		return in.Selections == nil
	}
	return nlqroute.ValidateRequest(savedRouting(in, nlq.LanguageEnglish).routeRequest()) == nil
}

func savedSelectionsMatch(q QueryRecord, in SavedQuestion) bool {
	if in.Durability != "replayable" {
		return true
	}
	r := q.Route.Request
	actual := SavedSelections{Kinds: r.Kinds, LimitPerKind: r.LimitPerKind, References: r.References, Choices: r.Choices,
		Joins: r.JoinChoices, MetricIDs: r.MetricIDs, Rerank: r.Rerank}
	var expected SavedSelections
	if in.Selections != nil {
		expected = *in.Selections
	}
	return len(r.Examples) == 0 && exec.Hash(actual) == exec.Hash(expected)
}

// ValidateSavedQuestion checks static shape using the ordinary router's bounds.
// Publication eligibility and signed resource checks remain in InspectSaved.
func ValidateSavedQuestion(in SavedQuestion) error {
	if !savedQuestionValid(in) {
		return ErrInvalid
	}
	return nil
}
