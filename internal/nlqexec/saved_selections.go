package nlqexec

import (
	"slices"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
)

// SavedSelections retains explicit business routing choices, not SQL, prompts,
// source bindings or authority. The existing router resolves every choice
// against the pinned/current reviewed semantic definitions before generation.
type SavedSelections struct {
	InterpretationPolicy     string                             `json:"interpretation_policy,omitempty"`
	InterpretationAnchor     string                             `json:"interpretation_anchor,omitempty"`
	InterpretationSelections []nlqroute.InterpretationSelection `json:"interpretation_selections,omitempty"`
	InterpretationEdits      []nlqroute.InterpretationEdit      `json:"interpretation_edits,omitempty"`
	Templates                []rulesets.TemplateSelection       `json:"templates,omitempty"`
	Kinds                    []string                           `json:"kinds,omitempty"`
	LimitPerKind             int                                `json:"limit_per_kind,omitempty"`
	References               []semantics.Reference              `json:"references,omitempty"`
	OmittedRoots             []semantics.Reference              `json:"omitted_roots,omitempty"`
	Choices                  []nlqroute.ChoiceSelection         `json:"choices,omitempty"`
	Joins                    []nlqroute.JoinChoice              `json:"joins,omitempty"`
	MetricIDs                []string                           `json:"metric_ids,omitempty"`
	Rerank                   bool                               `json:"rerank,omitempty"`
}

func savedRouting(in SavedQuestion, language nlq.Language) QuestionRequest {
	q := QuestionRequest{Context: in.Context, Locale: language, Question: in.Question}
	for _, pin := range in.Topics {
		q.Topics = append(q.Topics, pin.Topic)
	}
	if in.Selections != nil {
		s := in.Selections
		q.InterpretationPolicy = s.InterpretationPolicy
		q.InterpretationAnchor = s.InterpretationAnchor
		q.InterpretationSelections = nlqroute.CloneInterpretationSelections(s.InterpretationSelections)
		q.InterpretationEdits = nlqroute.CloneInterpretationEdits(s.InterpretationEdits)
		q.Templates = slices.Clone(s.Templates)
		q.Kinds, q.LimitPerKind = slices.Clone(s.Kinds), s.LimitPerKind
		q.References, q.Choices = slices.Clone(s.References), slices.Clone(s.Choices)
		q.OmittedRoots = slices.Clone(s.OmittedRoots)
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
	actual := SavedSelections{InterpretationPolicy: r.InterpretationPolicy, InterpretationSelections: nlqroute.CloneInterpretationSelections(r.InterpretationSelections), InterpretationEdits: nlqroute.CloneInterpretationEdits(r.InterpretationEdits), Templates: q.Templates, Kinds: r.Kinds, LimitPerKind: r.LimitPerKind, References: r.References, OmittedRoots: r.OmittedRoots, Choices: r.Choices,
		Joins: r.JoinChoices, MetricIDs: r.MetricIDs, Rerank: r.Rerank}
	// An explicit saved anchor is pinned. Absence keeps the existing per-plan
	// server-anchor semantics; it must not compare an absent input to a
	// generated default or make an otherwise valid saved question unexecutable.
	if in.Selections != nil && in.Selections.InterpretationAnchor != "" {
		actual.InterpretationAnchor = r.InterpretationAnchor
	}
	var expected SavedSelections
	if in.Selections != nil {
		expected = *in.Selections
	}
	if len(actual.Choices) == 0 {
		// Current routing replaces legacy reference choices with pinned typed
		// answers. Compare that exact evidence rather than retaining raw choices
		// alongside answers or silently ignoring newly introduced constraints.
		if !savedCanonicalChoicesMatch(q, expected.Choices) {
			return false
		}
		actual.Choices = expected.Choices
	} else if len(r.Answers) != 0 || len(semantics.CanonicalClarificationAnswers(q.Route.Resolutions)) != 0 {
		return false
	}
	return len(r.Examples) == 0 && exec.Hash(actual) == exec.Hash(expected)
}

// savedCanonicalChoicesMatch compares retained business evidence only. It does
// not grant authority or replace current publication checks and read validation.
func savedCanonicalChoicesMatch(q QueryRecord, choices []nlqroute.ChoiceSelection) bool {
	answers := semantics.CanonicalClarificationAnswers(q.Route.Resolutions)
	if len(answers) != len(choices) || exec.Hash(answers) != exec.Hash(q.Route.Request.Answers) {
		return false
	}
	for _, resolution := range q.Route.Resolutions {
		if resolution.Provenance != "answer" && resolution.Provenance != "legacy_reference_choice" {
			continue
		}
		i := slices.Index(q.Topics, resolution.Topic)
		if i < 0 || i >= len(q.TopicVersions) || i >= len(q.RuleVersions) ||
			resolution.TopicVersion != q.TopicVersions[i] || resolution.RulesetVersion == "" || resolution.RulesetVersion != q.RuleVersions[i] ||
			resolution.SchemaVersion != semantics.ClarificationSchemaVersion || resolution.ID == "" || resolution.ID != semantics.ClarificationResolutionDigest(resolution) ||
			resolution.Reference == nil || !resolution.Reference.Valid() || resolution.Effect != nil || resolution.Time != nil || resolution.Null || resolution.Upper != "" {
			return false
		}
	}
	used := make([]bool, len(answers))
	for _, choice := range choices {
		match := -1
		for i, answer := range answers {
			if answer.Slot != choice.Slot || choice.Pattern != "" && answer.Pattern != choice.Pattern {
				continue
			}
			if answer.Value == nil || answer.Value.OptionID != choice.Value || match >= 0 {
				return false
			}
			match = i
		}
		if match < 0 || used[match] {
			return false
		}
		used[match] = true
	}
	return true
}

// ValidateSavedQuestion checks static shape using the ordinary router's bounds.
// Publication eligibility and signed resource checks remain in InspectSaved.
func ValidateSavedQuestion(in SavedQuestion) error {
	if !savedQuestionValid(in) {
		return ErrInvalid
	}
	return nil
}
