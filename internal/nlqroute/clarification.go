package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

// ClarificationPins bind business evidence to the admitted request partition.
// They contain no bearer, signing material, source credential or authority grant.
type ClarificationPins struct {
	SchemaVersion int                      `json:"schema_version"`
	Tenant        string                   `json:"tenant"`
	Actor         string                   `json:"actor"`
	Session       string                   `json:"session"`
	Context       string                   `json:"context"`
	Sources       []ClarificationSourcePin `json:"sources"`
}

// ClarificationSourcePin is exact source continuity evidence, not access proof.
type ClarificationSourcePin struct {
	Topic    string `json:"topic"`
	Version  string `json:"version"`
	Digest   string `json:"digest"`
	Source   string `json:"source"`
	Context  string `json:"context"`
	Dataset  string `json:"dataset"`
	Revision int64  `json:"revision"`
}

// Recheck repeats current admission and typed resolution without provider or
// vector work. Refinement and execution use it with freshly verified authority.
func (s *Service) Recheck(ctx context.Context, e identity.Envelope, in RouteRequest) (RouteResult, error) {
	if ctx == nil || s == nil || s.topics == nil || s.rules == nil || s.assembler == nil {
		return RouteResult{}, ErrInvalid
	}
	if !e.Valid() {
		return RouteResult{}, access.ErrUnauthenticated
	}
	ids, err := normalizeRequest(in)
	if err != nil {
		return RouteResult{}, err
	}
	_, result, _, err := s.prepareClarifications(ctx, e, in, ids)
	return result, err
}

func (s *Service) prepareClarifications(ctx context.Context, e identity.Envelope, in RouteRequest, ids []string) ([]admittedTopic, RouteResult, []nlq.PinnedMetric, error) {
	result := RouteResult{Outcome: nlq.StrategySingleTopic, Request: cloneRouteRequest(in), Topic: ids[0], Topics: append([]string(nil), ids...), Stages: []Stage{}}
	// Raw answers are never retained by a rejected or partially admitted route.
	result.Request.Answers, result.Request.Choices = nil, nil
	if len(ids) > 1 {
		result.Outcome = nlq.StrategyMultiTopic
	}
	admitted := make([]admittedTopic, 0, len(ids))
	started := time.Now()
	for _, id := range ids {
		contract, err := s.topics.Contract(ctx, e, id)
		if err != nil {
			return nil, RouteResult{}, nil, err
		}
		publication := contract.Publication
		if publication.State.Topic != id || publication.Definition.Topic != id || publication.Definition.Version != publication.State.Version || publication.State.Archived || !publication.State.Active || publication.State.Version == "" || !topics.DigestValid(publication.Digest) {
			return nil, RouteResult{}, nil, store.ErrConflict
		}
		item := admittedTopic{id: id, publication: publication}
		item.rules, item.hasRules, err = s.readRules(ctx, e, id, publication.State.Version, publication.Digest)
		if err != nil {
			return nil, RouteResult{}, nil, err
		}
		admitted = append(admitted, item)
	}
	// Admit every topic and its exact context before evaluating or revealing any
	// question. An early blocker cannot mask an inaccessible later topic.
	if !contextMatches(admitted, in.Context) {
		return nil, RouteResult{}, nil, readexec.ErrBinding
	}
	metrics, err := resolveMetrics(admitted, in.MetricIDs)
	if err != nil {
		return nil, RouteResult{}, nil, err
	}
	result.TopicVersions, result.RuleVersions = make([]string, len(admitted)), make([]string, len(admitted))
	pins := &ClarificationPins{SchemaVersion: semantics.ClarificationSchemaVersion, Tenant: e.Tenant(), Actor: e.User(), Session: e.Session(), Context: in.Context}
	for i, item := range admitted {
		result.TopicVersions[i] = item.publication.State.Version
		if item.hasRules {
			result.RuleVersions[i] = item.rules.State.Version
		}
		for _, dataset := range item.publication.Definition.Datasets {
			pins.Sources = append(pins.Sources, ClarificationSourcePin{Topic: item.id, Version: item.publication.State.Version, Digest: item.publication.Digest, Source: dataset.Source.Source, Context: dataset.Source.Context, Dataset: dataset.ID, Revision: dataset.Source.SourceRevision})
		}
	}
	sort.Slice(pins.Sources, func(i, j int) bool {
		a, b := pins.Sources[i], pins.Sources[j]
		if a.Topic != b.Topic {
			return a.Topic < b.Topic
		}
		return a.Dataset < b.Dataset
	})
	byTopic := map[string][]semantics.ClarificationAnswer{}
	for _, answer := range in.Answers {
		found := false
		for _, item := range admitted {
			if item.id == answer.Topic {
				found = true
				break
			}
		}
		if !found {
			return nil, RouteResult{}, nil, ErrInvalid
		}
		byTopic[answer.Topic] = append(byTopic[answer.Topic], answer)
	}
	// Legacy choices have no topic coordinate. Resolve them only when the exact
	// pattern/slot is unique across the complete admitted topic set.
	legacy := map[string][]semantics.LegacyClarificationChoice{}
	for _, choice := range in.Choices {
		owner, patternID, matches := "", "", 0
		for _, item := range admitted {
			if !item.hasRules {
				continue
			}
			for _, pattern := range item.rules.Definition.Patterns {
				if choice.Pattern != "" && choice.Pattern != pattern.ID {
					continue
				}
				for _, slot := range pattern.Slots {
					if slot.ID == choice.Slot {
						owner, patternID, matches = item.id, pattern.ID, matches+1
					}
				}
			}
		}
		if matches != 1 {
			return nil, RouteResult{}, nil, ErrInvalid
		}
		legacy[owner] = append(legacy[owner], semantics.LegacyClarificationChoice{Pattern: patternID, Slot: choice.Slot, Value: choice.Value})
	}
	evaluations := make([]semantics.ClarificationEvaluation, 0, len(admitted))
	for i := range admitted {
		item := &admitted[i]
		if !item.hasRules {
			if len(byTopic[item.id]) != 0 {
				return nil, RouteResult{}, nil, ErrInvalid
			}
			continue
		}
		subject, err := clarificationSubject(item.publication)
		if err != nil {
			return nil, RouteResult{}, nil, err
		}
		model, err := semantics.CompilePublishedRules(subject, item.rules.Definition)
		if err != nil {
			return nil, RouteResult{}, nil, err
		}
		if model.Digest() != item.rules.Digest {
			return nil, RouteResult{}, nil, store.ErrConflict
		}
		refs := append([]semantics.Reference(nil), in.References...)
		// Explicit metric selections participate in applicability without relying on
		// generated interpretations or storage order.
		for _, id := range in.MetricIDs {
			for _, m := range item.publication.Definition.Measures {
				if m.ID == id {
					refs = appendUniqueRef(refs, semantics.Reference{Kind: semantics.KindMeasure, ID: id})
				}
			}
			for _, m := range item.publication.Definition.KPIs {
				if m.ID == id {
					refs = appendUniqueRef(refs, semantics.Reference{Kind: semantics.KindKPI, ID: id})
				}
			}
		}
		evaluation := semantics.ResolveClarifications(model, semantics.ClarificationInput{Locale: string(in.Locale), Question: in.Question, References: refs, Answers: byTopic[item.id], LegacyChoices: legacy[item.id]})
		evaluations = append(evaluations, evaluation)
		result.ClarificationOutcomes = append(result.ClarificationOutcomes, evaluation.Slots...)
		result.Dispositions = append(result.Dispositions, evaluation.Dispositions...)
		result.Resolutions = append(result.Resolutions, evaluation.Resolutions...)
		if evaluation.Outcome == semantics.ClarificationInvalid || evaluation.Outcome == semantics.ClarificationConflicting || evaluation.Outcome == semantics.ClarificationMissing {
			continue
		}
		refs = evaluation.References
		if len(refs) == 0 {
			if len(item.publication.Definition.Datasets) == 0 {
				return nil, RouteResult{}, nil, store.ErrConflict
			}
			refs = []semantics.Reference{{Kind: semantics.KindDataset, ID: item.publication.Definition.Datasets[0].ID}}
		}
		checked, err := s.rules.Evaluate(ctx, e, item.id, rulesets.EvaluateRequest{References: refs})
		if err != nil {
			return nil, RouteResult{}, nil, err
		}
		if checked.TopicVersion != item.publication.State.Version || checked.PackDigest != item.publication.Digest || checked.RuleVersion != item.rules.State.Version || checked.RuleDigest != item.rules.Digest {
			return nil, RouteResult{}, nil, store.ErrConflict
		}
		state := &nlq.ConstraintState{Allowed: checked.Result.Allowed}
		for _, ref := range checked.Result.Required {
			state.Required = append(state.Required, nlq.MandatoryConstraint{ID: referenceID(ref), Kind: "required", Text: referenceText(ref)})
		}
		for _, ref := range checked.Result.Excluded {
			state.Excluded = append(state.Excluded, nlq.MandatoryConstraint{ID: referenceID(ref), Kind: "excluded", Text: referenceText(ref)})
		}
		// Every selected reference and every typed effect participates in the same
		// mandatory lane, including choices with no separate require_reference rule.
		for _, resolution := range evaluation.Resolutions {
			state.Required = append(state.Required, nlq.MandatoryConstraint{ID: "clarification:" + resolution.ID, Kind: "required", Text: resolutionContext(resolution)})
		}
		item.constraints = state
		if !state.Allowed {
			result.Clarification = &Clarification{Reason: "mandatory_constraint", Prompt: "Choose a metric or dimension that satisfies the published rules."}
		}
		for _, rule := range item.rules.Definition.Rules {
			if rule.Class == semantics.RuleAdvisoryContext && rule.Guidance != nil && rule.Guidance.Sensitivity == semantics.LiteralNonSensitive {
				item.advisory = append(item.advisory, nlq.OptionalItem{ID: rule.ID, Text: rule.Guidance.Text, Priority: rule.Priority, Source: "rules"})
			}
		}
	}
	// Typed failures have precedence over missing inputs and expose no partial
	// constraint. Independent blockers retain deterministic topic/policy ordering.
	outcome := semantics.ClarificationSatisfied
	for _, evaluation := range evaluations {
		switch evaluation.Outcome {
		case semantics.ClarificationInvalid:
			outcome = semantics.ClarificationInvalid
		case semantics.ClarificationConflicting:
			if outcome != semantics.ClarificationInvalid {
				outcome = semantics.ClarificationConflicting
			}
		case semantics.ClarificationMissing:
			if outcome == semantics.ClarificationSatisfied {
				outcome = semantics.ClarificationMissing
			}
		}
	}
	if outcome != semantics.ClarificationSatisfied {
		result.Clarification = &Clarification{Reason: string(outcome), Outcome: outcome, Questions: append([]semantics.ClarificationSlotOutcome(nil), result.ClarificationOutcomes...)}
		for _, evaluation := range evaluations {
			result.Clarification.Errors = append(result.Clarification.Errors, evaluation.Errors...)
		}
		for _, q := range result.ClarificationOutcomes {
			if q.Outcome == semantics.ClarificationMissing && q.Prompt != "" {
				result.Clarification.Pattern, result.Clarification.Slot, result.Clarification.Prompt = q.Pattern, q.Slot, q.Prompt
				for _, c := range q.Choices {
					result.Clarification.Choices = append(result.Clarification.Choices, ClarificationChoice{ID: c.ID, Label: c.Label})
				}
				break
			}
		}
		if outcome == semantics.ClarificationInvalid || outcome == semantics.ClarificationConflicting {
			result.Resolutions = nil
		}
	}
	result.Request.Answers = semantics.CanonicalClarificationAnswers(result.Resolutions)
	if len(result.ClarificationOutcomes) > 0 || len(result.Resolutions) > 0 {
		result.ClarificationPins = pins
	}
	if result.Clarification == nil && len(admitted) > 1 {
		result.Clarification = confirmJoins(admitted, in.JoinChoices)
	}
	result.Stages = append(result.Stages, Stage{Name: "admission", DurationMS: time.Since(started).Milliseconds()})
	if result.Clarification != nil {
		result.Outcome = nlq.StrategyClarify
		return admitted, result, metrics, nil
	}
	if len(result.Resolutions) > 0 {
		// Reserve a single fixed tier for the complete mandatory group before any
		// provider call. Retrieval cannot later downgrade this reservation.
		_, err = s.assembler.Assemble(ctx, nlq.ContextInput{Locale: in.Locale, Strategy: result.Outcome, Topic: result.Topic, TopicVersion: result.TopicVersions[0], Topics: topicRevisions(admitted), Question: in.Question, Constraints: mergeConstraints(admitted), Metrics: metrics}, nlq.TierHigh)
		if err != nil {
			return nil, RouteResult{}, nil, err
		}
	}
	return admitted, result, metrics, nil
}

func clarificationSubject(p topics.Published) (semantics.RuleSubject, error) {
	d := p.Definition
	pack := semantics.TopicPack{SchemaVersion: d.SchemaVersion, Topic: d.Topic, Version: d.Version, Name: d.Name, Description: d.Description, Measures: d.Measures, Dimensions: d.Dimensions, KPIs: d.KPIs, Joins: d.Joins, CanonicalEntities: d.CanonicalEntities}
	for _, dataset := range d.Datasets {
		pack.Datasets = append(pack.Datasets, semantics.Dataset{ID: dataset.ID, Name: dataset.Name, Columns: append([]semantics.Column(nil), dataset.Columns...)})
	}
	return semantics.NewRuleSubject(pack, p.Digest)
}

// resolutionContext is deliberately value-free. The execution binder applies
// the canonical values; the model needs only the reviewed structural meaning.
// Even governed value dictionaries and display labels are excluded here.
func resolutionContext(r semantics.ClarificationResolution) string {
	type effectView struct {
		Kind     string              `json:"kind"`
		Target   semantics.Reference `json:"target"`
		Operator string              `json:"operator"`
		Nulls    string              `json:"nulls"`
		Unit     string              `json:"unit,omitempty"`
		Bounds   string              `json:"bounds,omitempty"`
		Calendar string              `json:"calendar,omitempty"`
		TimeZone string              `json:"time_zone,omitempty"`
		Grain    string              `json:"grain,omitempty"`
	}
	view := struct {
		ID        string               `json:"resolution"`
		Reference *semantics.Reference `json:"reference,omitempty"`
		Effect    *effectView          `json:"effect,omitempty"`
		Binding   string               `json:"binding"`
	}{ID: r.ID, Reference: r.Reference, Binding: "Server binds reviewed values before validation. Do not invent or duplicate these filters."}
	if r.Effect != nil {
		e := r.Effect
		view.Effect = &effectView{Kind: e.Kind, Target: e.Target, Operator: e.Operator, Nulls: e.Nulls, Unit: e.Unit, Bounds: e.Bounds, Calendar: e.Calendar, TimeZone: e.TimeZone}
		if r.Time != nil {
			view.Effect.Grain = r.Time.Grain
		}
	}
	raw, _ := json.Marshal(view)
	return string(raw)
}

// CloneAnswers detaches all nested typed values at request boundaries.
func CloneAnswers(in []semantics.ClarificationAnswer) []semantics.ClarificationAnswer {
	if in == nil {
		return nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	var out []semantics.ClarificationAnswer
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

// IsClarificationFailure identifies a typed terminal repair request.
func IsClarificationFailure(err error) bool { var c *Clarification; return errors.As(err, &c) }

// ConstraintBindingReceipt is content-free evidence of a semantically bound
// candidate and its ordinary validator-issued read receipt. It grants no access.
type ConstraintBindingReceipt struct {
	SchemaVersion int      `json:"schema_version"`
	Dialect       string   `json:"dialect"`
	ResolutionIDs []string `json:"resolution_ids"`
	SourceBinding string   `json:"source_binding"`
	Statement     string   `json:"statement"`
	Parameters    string   `json:"parameters"`
	ReadManifest  string   `json:"read_manifest"`
}
