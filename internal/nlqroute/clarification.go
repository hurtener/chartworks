package nlqroute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

// clarificationBindingReader is implemented by the existing live topic service.
// It exposes no credentials and never issues an executable plan.
type clarificationBindingReader interface {
	ClarificationBinding(context.Context, identity.Envelope, string, string) (readexec.Binding, error)
}

func (c *Clarification) Unwrap() error {
	if c.Outcome == semantics.ClarificationInvalid {
		return ErrInvalid
	}
	if c.Outcome == semantics.ClarificationConflicting {
		return store.ErrConflict
	}
	return nlq.ErrInsufficient
}

func clarificationFailure(locale nlq.Language, field, code string) *Clarification {
	message := "Refresh clarification for the current topic, policy, session and source context, then supply a typed answer."
	if locale == nlq.LanguageSpanish {
		message = "Actualizá la aclaración para el tema, la política, la sesión y el contexto actuales; después ingresá una respuesta tipada."
	}
	return &Clarification{Reason: code, Outcome: semantics.ClarificationInvalid, Errors: []semantics.ClarificationFieldError{{Field: field, Code: code, Message: message}}}
}

type clarificationTopicPin struct {
	Topic         string
	Version       string
	Digest        string
	Revision      int64
	RulesVersion  string
	RulesDigest   string
	RulesRevision int64
	Sources       []topics.Binding
	BindingDigest string
}

func clarificationContext(e identity.Envelope, admitted []admittedTopic) string {
	pins := make([]clarificationTopicPin, 0, len(admitted))
	for _, item := range admitted {
		pin := clarificationTopicPin{Topic: item.id, Version: item.publication.State.Version, Digest: item.publication.Digest, Revision: item.publication.State.Revision}
		if item.hasRules {
			pin.RulesVersion, pin.RulesDigest, pin.RulesRevision = item.rules.State.Version, item.rules.Digest, item.rules.State.Revision
		}
		for _, dataset := range item.publication.Definition.Datasets {
			pin.Sources = append(pin.Sources, dataset.Source)
		}
		sort.Slice(pin.Sources, func(i, j int) bool { return pin.Sources[i].Dataset < pin.Sources[j].Dataset })
		if item.binding != nil {
			pin.BindingDigest = readexec.Hash(*item.binding)
		}
		pins = append(pins, pin)
	}
	sort.Slice(pins, func(i, j int) bool { return pins[i].Topic < pins[j].Topic })
	return readexec.Hash(struct {
		Schema                 int
		Tenant, Actor, Session string
		Topics                 []clarificationTopicPin
	}{semantics.ClarificationSchemaVersion, e.Tenant(), e.User(), e.Session(), pins})
}

func scopedClarificationInputs(in RouteRequest, admitted []admittedTopic) (map[string]semantics.ClarificationInput, error) {
	inputs := map[string]semantics.ClarificationInput{}
	for _, item := range admitted {
		inputs[item.id] = semantics.ClarificationInput{Locale: string(in.Locale), Question: in.Question, References: append([]semantics.Reference(nil), in.References...)}
	}
	for _, answer := range semantics.CloneClarificationAnswers(in.Answers) {
		input, ok := inputs[answer.Topic]
		if !ok {
			return nil, clarificationFailure(in.Locale, "answers.topic", "foreign_answer")
		}
		input.Answers = append(input.Answers, answer)
		inputs[answer.Topic] = input
	}
	for _, choice := range in.Choices {
		owner, matches := "", 0
		for _, item := range admitted {
			for _, pattern := range item.rules.Definition.Patterns {
				if choice.Pattern != "" && choice.Pattern != pattern.ID {
					continue
				}
				for _, slot := range pattern.Slots {
					if slot.ID == choice.Slot {
						owner, matches = item.id, matches+1
					}
				}
			}
		}
		if matches != 1 {
			return nil, clarificationFailure(in.Locale, "choices", "foreign_or_ambiguous_choice")
		}
		input := inputs[owner]
		input.LegacyChoices = append(input.LegacyChoices, semantics.LegacyClarificationChoice{Pattern: choice.Pattern, Slot: choice.Slot, Value: choice.Value})
		inputs[owner] = input
	}
	return inputs, nil
}

// prepareClarifications runs only after every topic has been admitted. It
// produces atomic invalid/conflict failures and groups independent blockers.
func (s *Service) prepareClarifications(ctx context.Context, e identity.Envelope, in RouteRequest, admitted []admittedTopic, result *RouteResult) error {
	inputs, err := scopedClarificationInputs(in, admitted)
	if err != nil {
		return err
	}
	order := make([]int, len(admitted))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return admitted[order[i]].id < admitted[order[j]].id })
	var missing bool
	for _, index := range order {
		item := &admitted[index]
		input := inputs[item.id]
		if !item.hasRules {
			if len(input.Answers)+len(input.LegacyChoices) > 0 {
				return clarificationFailure(in.Locale, "answers", "no_reviewed_policy")
			}
			continue
		}
		evaluation, err := rulesets.ResolvePublishedClarifications(item.publication, item.rules, input)
		if err != nil {
			return err
		}
		item.clarifications = &evaluation
		if evaluation.Outcome == semantics.ClarificationInvalid || evaluation.Outcome == semantics.ClarificationConflicting {
			return &Clarification{Reason: "clarification_" + string(evaluation.Outcome), Outcome: evaluation.Outcome, Questions: evaluation.Slots, Errors: evaluation.Errors}
		}
		missing = missing || evaluation.Outcome == semantics.ClarificationMissing
		needsBinding := false
		for _, state := range evaluation.Slots {
			if state.Effect != nil && (state.Outcome != semantics.ClarificationNotApplicable || state.Reason == "optional_omitted") {
				needsBinding = true
			}
		}
		if needsBinding {
			reader, ok := s.topics.(clarificationBindingReader)
			if !ok {
				return &readexec.BusinessConstraintError{Code: "unsupported_constraint_binding", Field: "source"}
			}
			for _, dataset := range item.publication.Definition.Datasets {
				binding, err := reader.ClarificationBinding(ctx, e, dataset.Source.Source, in.Context)
				if err != nil {
					return err
				}
				if !binding.Valid() || binding.Tenant != e.Tenant() || binding.Context != in.Context || binding.Source != dataset.Source.Source || binding.Revision != dataset.Source.SourceRevision {
					return readexec.ErrBinding
				}
				if item.binding != nil && readexec.Hash(*item.binding) != readexec.Hash(binding) {
					return readexec.ErrBinding
				}
				copy := binding.Clone()
				item.binding = &copy
			}
		}
	}
	result.AnswerContext = clarificationContext(e, admitted)
	if len(in.Answers) > 0 && in.AnswerContext == "" || in.AnswerContext != "" && in.AnswerContext != result.AnswerContext {
		return clarificationFailure(in.Locale, "answer_context", "stale_answer_context")
	}
	var resolutions []semantics.ClarificationResolution
	for _, index := range order {
		if admitted[index].clarifications != nil {
			resolutions = append(resolutions, admitted[index].clarifications.Resolutions...)
		}
	}
	// Classification is taken from the reviewed slot, including an unresolved
	// supplied value. Such raw input is never retained just to resume a form.
	redactions := semantics.CloneClarificationResolutions(resolutions)
	for _, answer := range in.Answers {
		for _, item := range admitted {
			if item.id != answer.Topic {
				continue
			}
			for _, pattern := range item.rules.Definition.Patterns {
				if pattern.ID != answer.Pattern {
					continue
				}
				for _, slot := range pattern.Slots {
					if slot.ID == answer.Slot && slot.Sensitivity == semantics.LiteralSensitive {
						redactions = append(redactions, semantics.ClarificationResolution{Topic: answer.Topic, Pattern: answer.Pattern, Slot: answer.Slot, Sensitivity: slot.Sensitivity})
					}
				}
			}
		}
	}
	safeQuestion := semantics.RedactClarificationText(in.Question, in.Answers, redactions)
	questionDigest := sha256.Sum256([]byte(safeQuestion))
	for _, index := range order {
		item := &admitted[index]
		if item.clarifications == nil {
			continue
		}
		for i := range item.clarifications.Resolutions {
			r := &item.clarifications.Resolutions[i]
			r.QuestionDigest = hex.EncodeToString(questionDigest[:])
			if r.Provenance != "reviewed_default" && r.Provenance != "typed_interpretation" {
				r.Provenance = "answer"
			}
			r.ID = semantics.ClarificationResolutionDigest(*r)
		}
		result.Resolutions = append(result.Resolutions, item.clarifications.Resolutions...)
		view := *item.clarifications
		view.Resolutions = nil
		result.Clarifications = append(result.Clarifications, view)
	}
	result.Request = cloneRouteRequest(in)
	result.Request.Question = safeQuestion
	for i := range result.Request.Examples {
		result.Request.Examples[i].Text = semantics.RedactClarificationText(result.Request.Examples[i].Text, in.Answers, redactions)
	}
	result.Request.Answers = semantics.CanonicalClarificationAnswers(result.Resolutions)
	result.Request.AnswerContext = result.AnswerContext
	result.Request.Choices = nil
	if len(in.Choices) > 0 {
		result.Warnings = append(result.Warnings, "legacy_reference_choices_normalized")
	}
	if len(result.Resolutions) == 0 && !missing {
		result.Request.AnswerContext = ""
	}
	if missing {
		clarification := &Clarification{Reason: "required_answers", Outcome: semantics.ClarificationMissing, Why: "Each required field resolves a reviewed business constraint. Dependent fields follow their prerequisites."}
		if in.Locale == nlq.LanguageSpanish {
			clarification.Why = "Cada campo obligatorio resuelve una condición de negocio revisada. Los campos dependientes siguen a sus requisitos."
		}
		for _, group := range result.Clarifications {
			clarification.Errors = append(clarification.Errors, group.Errors...)
			for _, question := range group.Slots {
				if question.Outcome != semantics.ClarificationMissing {
					continue
				}
				clarification.Questions = append(clarification.Questions, question)
				if clarification.Prompt == "" && question.Prompt != "" {
					clarification.Pattern, clarification.Slot, clarification.Prompt = question.Pattern, question.Slot, question.Prompt
					for _, choice := range question.Choices {
						clarification.Choices = append(clarification.Choices, ClarificationChoice{ID: choice.ID, Label: choice.Label})
					}
				}
			}
		}
		result.Clarification, result.Outcome = clarification, nlq.StrategyClarify
		return nil
	}
	for _, item := range admitted {
		if item.clarifications == nil {
			continue
		}
		for _, resolution := range item.clarifications.Resolutions {
			if resolution.Effect == nil {
				continue
			}
			if item.binding == nil {
				return readexec.ErrBinding
			}
			constraint, err := businessConstraint(item, resolution)
			if err != nil {
				return err
			}
			if err := readexec.ValidateBusinessConstraints(*item.binding, []readexec.BusinessConstraint{constraint}); err != nil {
				return err
			}
			result.business = append(result.business, constraint)
			digest := readexec.Hash(*item.binding)
			if result.SourceBindingDigest != "" && result.SourceBindingDigest != digest {
				return readexec.ErrBinding
			}
			result.SourceBindingDigest = digest
		}
	}
	return nil
}

func businessConstraint(item admittedTopic, resolution semantics.ClarificationResolution) (readexec.BusinessConstraint, error) {
	effect := resolution.Effect
	if effect == nil {
		return readexec.BusinessConstraint{}, readexec.ErrBinding
	}
	dataset, column, err := rulesets.ClarificationColumn(item.publication.Definition, effect.Target)
	if err != nil {
		return readexec.BusinessConstraint{}, err
	}
	out := readexec.BusinessConstraint{Resolution: resolution.ID, Dataset: dataset, Column: column.SourceName, SourceRevision: item.binding.Revision, Kind: effect.Kind, Operator: effect.Operator, Nulls: effect.Nulls, Unit: effect.Unit, Precision: effect.Precision, Scale: effect.Scale, Bounds: effect.Bounds, TemporalType: effect.TemporalType, Calendar: effect.Calendar, TimeZone: effect.TimeZone, Value: resolution.Value, Upper: resolution.Upper, Null: resolution.Null}
	if effect.Target.Kind == semantics.KindMeasure {
		// A measure target denotes an aggregate threshold, not a silent row
		// filter on its input. The binding stage handles its reviewed aggregate.
		if effect.Kind != "number" {
			return out, &readexec.BusinessConstraintError{Code: "unsupported_aggregate_effect", Resolution: resolution.ID, Field: "target"}
		}
		for _, measure := range item.publication.Definition.Measures {
			if measure.ID == effect.Target.ID {
				if effect.Unit != measure.Unit {
					return out, &readexec.BusinessConstraintError{Code: "conflicting_unit", Resolution: resolution.ID, Field: "unit"}
				}
				out.Aggregation = string(measure.Aggregation)
			}
		}
	}
	if resolution.Time != nil {
		out.Grain = resolution.Time.Grain
		out.Value, out.Upper = resolution.Time.LocalStart, resolution.Time.LocalEnd
		if effect.TemporalType == "timestamptz" {
			out.Value, out.Upper = resolution.Time.StartUTC, resolution.Time.EndUTC
		}
	}
	return out, nil
}

func (r RouteResult) resolutionProof() string {
	return readexec.Hash([]any{r.AnswerContext, r.SourceBindingDigest, r.Resolutions, r.business})
}

// ResolvedBusinessConstraints is available only from the in-process sealed
// route whose mandatory context includes every resolution. Wire reconstruction
// cannot turn a supplied answer into execution authority.
func (r RouteResult) ResolvedBusinessConstraints() ([]readexec.BusinessConstraint, error) {
	if _, err := r.GenerationContext(); err != nil {
		return nil, err
	}
	if r.resolutionSeal == "" || r.resolutionSeal != r.resolutionProof() {
		return nil, readexec.ErrBinding
	}
	for _, resolution := range r.Resolutions {
		found := false
		if r.Context.Constraints != nil {
			for _, constraint := range r.Context.Constraints.Required {
				if constraint.ID == "clarification-"+resolution.ID {
					found = true
				}
			}
		}
		if !found {
			return nil, readexec.ErrBinding
		}
	}
	return append([]readexec.BusinessConstraint(nil), r.business...), nil
}

func appendResolvedConstraints(state *nlq.ConstraintState, resolutions []semantics.ClarificationResolution, full bool) (*nlq.ConstraintState, error) {
	if len(resolutions) == 0 {
		return state, nil
	}
	if state == nil {
		state = &nlq.ConstraintState{Allowed: true}
	}
	for _, resolution := range resolutions {
		text, err := semantics.ProviderClarificationText(resolution)
		if full {
			text, err = semantics.ClarificationBudgetText(resolution)
		}
		if err != nil {
			return nil, err
		}
		state.Required = append(state.Required, nlq.MandatoryConstraint{ID: "clarification-" + resolution.ID, Kind: "clarification", Text: text})
	}
	return state, nil
}

func (s *Service) preflightClarificationBudget(ctx context.Context, in RouteRequest, admitted []admittedTopic, result RouteResult, metrics []nlq.PinnedMetric) (nlq.Tier, error) {
	constraints := mergeConstraints(admitted)
	if constraints != nil {
		kept := make([]nlq.MandatoryConstraint, 0, len(constraints.Required))
		for _, constraint := range constraints.Required {
			if constraint.Kind != "clarification" {
				kept = append(kept, constraint)
			}
		}
		constraints.Required = kept
	}
	constraints, err := appendResolvedConstraints(constraints, result.Resolutions, true)
	if err != nil {
		return "", err
	}
	input := nlq.ContextInput{Locale: in.Locale, Strategy: result.Outcome, Topic: admitted[0].id, TopicVersion: admitted[0].publication.State.Version, Topics: topicRevisions(admitted), Question: in.Question, Constraints: constraints, Metrics: metrics}
	var last error
	for _, tier := range []nlq.Tier{nlq.TierLow, nlq.TierMedium, nlq.TierHigh} {
		if _, err := s.assembler.Assemble(ctx, input, tier); err == nil {
			return tier, nil
		} else if !errors.Is(err, nlq.ErrInsufficient) {
			return "", err
		} else {
			last = err
		}
	}
	return "", last
}

func selectedClarificationMetrics(ids []string, resolutions []semantics.ClarificationResolution) []string {
	out := append([]string(nil), ids...)
	for _, resolution := range resolutions {
		if resolution.Reference == nil || resolution.Reference.Kind != semantics.KindMeasure && resolution.Reference.Kind != semantics.KindKPI {
			continue
		}
		found := false
		for _, id := range out {
			found = found || id == resolution.Reference.ID
		}
		if !found {
			out = append(out, resolution.Reference.ID)
		}
	}
	return out
}

// ReplayClarifications reevaluates stored canonical answers with current bearer
// reach and publication/source pins, without embedding, generation or execution.
func (s *Service) ReplayClarifications(ctx context.Context, e identity.Envelope, previous RouteResult) ([]readexec.BusinessConstraint, string, error) {
	in := cloneRouteRequest(previous.Request)
	ids, err := normalizeRequest(in)
	if err != nil {
		return nil, "", err
	}
	var admitted []admittedTopic
	for _, id := range ids {
		contract, err := s.topics.Contract(ctx, e, id)
		if err != nil {
			return nil, "", err
		}
		item := admittedTopic{id: id, publication: contract.Publication}
		item.rules, item.hasRules, err = s.readRules(ctx, e, id, contract.Publication.State.Version, contract.Publication.Digest)
		if err != nil {
			return nil, "", err
		}
		admitted = append(admitted, item)
	}
	if !contextMatches(admitted, in.Context) {
		return nil, "", readexec.ErrBinding
	}
	current := RouteResult{Outcome: previous.Outcome}
	if err := s.prepareClarifications(ctx, e, in, admitted, &current); err != nil {
		return nil, "", err
	}
	if current.Clarification != nil && previous.Clarification == nil {
		return nil, "", current.Clarification
	}
	old, _ := json.Marshal(previous.Resolutions)
	fresh, _ := json.Marshal(current.Resolutions)
	if string(old) != string(fresh) || previous.AnswerContext != current.AnswerContext || previous.SourceBindingDigest != current.SourceBindingDigest {
		return nil, "", clarificationFailure(in.Locale, "answers", "stale_answer_resolution")
	}
	return append([]readexec.BusinessConstraint(nil), current.business...), current.SourceBindingDigest, nil
}
