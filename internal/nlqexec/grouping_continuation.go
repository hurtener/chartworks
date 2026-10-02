package nlqexec

import (
	"context"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

// retainGrouping is reached only after authorized parent/current source reads,
// scalar replay and lineage checks. Neither old SQL nor its labels are parsed as
// analytical intent. A typed selection replaces the whole set; an empty set is
// a deliberate total. Absent input inherits an exactly verified parent grain.
func retainGrouping(ctx context.Context, old QueryRecord, parent admission, constraints []exec.BusinessConstraint, delta QuestionRequest, question *QuestionRequest) error {
	if ctx == nil || question == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := nlqroute.ValidateGrouping(delta.Grouping); err != nil {
		return err
	}
	if old.SQL == "" {
		// Pending forms have no analytical SQL proof yet. Their protected
		// logical request can still be replaced by a new complete question;
		// otherwise a stale explicit set would override its recognized words.
		selected := nlqroute.CloneGrouping(delta.Grouping)
		if selected == nil && delta.Question != "" && delta.Question != old.Question {
			var err error
			selected, err = groupingFromQuestion(ctx, parent, question, old.Route.Request.Grouping != nil)
			if err != nil {
				return err
			}
		}
		if selected == nil {
			selected = nlqroute.CloneGrouping(old.Route.Request.Grouping)
		}
		question.Grouping = selected
		return nil
	}
	prior, err := expectedAnalytical(ctx, old, parent, constraints)
	if err != nil {
		return err
	}
	var inherited *nlqroute.GroupingSelection
	if prior != nil && prior.Grain != nil {
		if old.Route.Request.Grouping != nil {
			// expectedAnalytical has just reconstructed this exact request and
			// proof. Preserve its dimension-to-grain mapping, not a cross-product
			// guessed from dimensions sharing a physical timestamp column.
			inherited = nlqroute.CloneGrouping(old.Route.Request.Grouping)
		} else {
			inherited, err = logicalGrouping(parent, prior.Grain)
			if err != nil {
				return err
			}
		}
	}
	selected := nlqroute.CloneGrouping(delta.Grouping)
	if selected == nil && delta.Question != "" && delta.Question != old.Question {
		selected, err = groupingFromQuestion(ctx, parent, question, inherited != nil)
		if err != nil {
			return err
		}
	}
	if selected == nil {
		selected = inherited
	}
	question.Grouping = nlqroute.CloneGrouping(selected)
	if selected == nil || inherited == nil {
		return nil
	}
	// The compatibility helper materializes old catalog roots for typed edits.
	// Remove the old group-role roots, not references explicitly supplied for this
	// edit or filter meanings (the router reintroduces interpreted filters).
	kept := make([]semantics.Reference, 0, len(question.References))
	for _, ref := range question.References {
		wasGroup, explicitNow := false, false
		for _, key := range inherited.Keys {
			wasGroup = wasGroup || ref.Kind == semantics.KindDimension && ref.ID == key.Dimension
		}
		for _, r := range delta.References {
			explicitNow = explicitNow || r == ref
		}
		if !wasGroup || explicitNow {
			kept = append(kept, ref)
		}
	}
	question.References = kept
	return nil
}

// This reuses the existing bounded recognizer with reviewed dimensions as local
// *candidates*. Those candidates are never installed as selected rule facts.
// Only its resolved grouping keys are sent to normal authorized routing.
func groupingFromQuestion(ctx context.Context, parent admission, q *QuestionRequest, requireResolved bool) (*nlqroute.GroupingSelection, error) {
	redactions := semantics.CloneClarificationResolutions(parent.route.Resolutions)
	// A still-pending scalar has not acquired a resolution/sensitivity record
	// yet. Its spelling is value data, not grouping syntax. Redact all submitted
	// scalar spellings for this local recognizer without changing stored answers
	// or promoting these temporary descriptors into resolved rule facts.
	for _, answer := range q.Answers {
		if answer.Value != nil && answer.Value.OptionID == "" {
			redactions = append(redactions, semantics.ClarificationResolution{Topic: answer.Topic, Pattern: answer.Pattern, Slot: answer.Slot, Sensitivity: semantics.LiteralSensitive})
		}
	}
	text := semantics.RedactClarificationText(q.Question, q.Answers, redactions)
	words := grainWords(text)
	switch strings.Join(words, " ") {
	case "total", "grand total", "overall total", "total general", "sin agrupar", "without grouping", "no grouping":
		return &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy, Keys: []nlqroute.GroupingKey{}}, nil
	}
	if len(parent.publications) == 0 || len(parent.publications) > 4 {
		return nil, exec.ErrBinding
	}
	candidate := parent
	candidate.route = parent.route
	candidate.route.Request = q.routeRequest()
	candidate.route.Request.Question = text
	candidate.route.Selection = &nlqroute.SemanticSelection{}
	for _, publication := range parent.publications {
		def := publication.Definition
		topic := nlqroute.SelectedTopic{Topic: def.Topic}
		// Every complete reviewed metric label protects an embedded by/per word,
		// not just the old metric's label. This does not select the new metric.
		if len(def.Dimensions)+len(def.Measures)+len(def.KPIs) > 1024 {
			return nil, exec.ErrLimit
		}
		for _, d := range def.Dimensions {
			topic.Roots = append(topic.Roots, nlqroute.SelectedRoot{Reference: semantics.Reference{Kind: semantics.KindDimension, ID: d.ID}, Reason: "catalog_term"})
		}
		for _, m := range def.Measures {
			topic.Roots = append(topic.Roots, nlqroute.SelectedRoot{Reference: semantics.Reference{Kind: semantics.KindMeasure, ID: m.ID}, Reason: "catalog_term"})
		}
		for _, k := range def.KPIs {
			topic.Roots = append(topic.Roots, nlqroute.SelectedRoot{Reference: semantics.Reference{Kind: semantics.KindKPI, ID: k.ID}, Reason: "catalog_term"})
		}
		candidate.route.Selection.Topics = append(candidate.route.Selection.Topics, topic)
	}
	recognized := false
	grain, err := compileAnalyticalGrainPolicy(ctx, candidate, exec.AnalyticalContract{}, true, &recognized)
	if err != nil {
		return nil, err
	}
	if grain == nil {
		if recognized && requireResolved {
			return nil, analyticalUnsupported("analytical_grain_unsupported")
		}
		return nil, nil
	}
	return logicalGrouping(candidate, grain)
}

// logicalGrouping maps only known reviewed identities and their exact fields.
// It never splits colon-bearing IDs or retains physical expressions as authority.
func logicalGrouping(a admission, grain *exec.AnalyticalGrain) (*nlqroute.GroupingSelection, error) {
	if grain == nil {
		return nil, nil
	}
	out := &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy, Keys: []nlqroute.GroupingKey{}}
	if len(grain.Columns)+len(grain.Buckets) == 0 {
		if grain.Policy != exec.AnalyticalGroupingPolicy || len(grain.Dimensions) != 0 {
			return nil, exec.ErrBinding
		}
		return out, nil
	}
	usedColumns, usedBuckets := map[string]bool{}, map[string]bool{}
	calendarOwners := map[string]map[string]bool{}
	calendarChoices := map[string]int{}
	for _, id := range grain.Dimensions {
		matches := 0
		for _, p := range a.publications {
			for _, d := range p.Definition.Dimensions {
				if p.Definition.Topic+":dimension:"+d.ID != id {
					continue
				}
				matches++
				var field semantics.Column
				fields := 0
				for _, dataset := range p.Definition.Datasets {
					if dataset.ID == d.Field.Dataset {
						for _, c := range dataset.Columns {
							if c.ID == d.Field.ID {
								field = c
								fields++
							}
						}
					}
				}
				if fields != 1 || field.SourceName == "" {
					return nil, exec.ErrBinding
				}
				matched := false
				for _, column := range grain.Columns {
					if column == field.SourceName && d.Role != semantics.DimensionTemporal && len(d.Filters) == 0 {
						out.Keys = append(out.Keys, nlqroute.GroupingKey{Topic: p.Definition.Topic, Dimension: d.ID})
						usedColumns[column] = true
						matched = true
					}
				}
				for _, bucket := range grain.Buckets {
					if bucket.Column == field.SourceName {
						dim := grainDimension{id: d.ID, field: d.Field, role: d.Role, filters: d.Filters, temporal: d.Temporal}
						reviewed, err := compileCalendarBucket(dim, field, bucket.Grain)
						if err != nil || reviewed != bucket {
							continue
						}
						out.Keys = append(out.Keys, nlqroute.GroupingKey{Topic: p.Definition.Topic, Dimension: d.ID, Grain: semantics.TimeGrain(bucket.Grain)})
						bucketID := exec.Hash(bucket)
						usedBuckets[bucketID] = true
						if calendarOwners[bucketID] == nil {
							calendarOwners[bucketID] = map[string]bool{}
						}
						calendarOwners[bucketID][id] = true
						calendarChoices[id]++
						matched = true
					}
				}
				if !matched {
					return nil, exec.ErrBinding
				}
			}
		}
		if matches != 1 {
			return nil, exec.ErrBinding
		}
	}
	// A flat legacy/natural proof may not encode which of two same-field
	// dimensions owned which of two compatible buckets. Do not invent all
	// pairings; the caller can provide an explicit logical set. Multiple buckets
	// on a single dimension and multiple labels for one bucket are unambiguous.
	for _, owners := range calendarOwners {
		if len(owners) > 1 {
			for id := range owners {
				if calendarChoices[id] > 1 {
					return nil, analyticalUnsupported("analytical_grain_unsupported")
				}
			}
		}
	}
	if len(usedColumns) != len(grain.Columns) || len(usedBuckets) != len(grain.Buckets) {
		return nil, exec.ErrBinding
	}
	if err := nlqroute.ValidateGrouping(out); err != nil {
		return nil, err
	}
	return nlqroute.CloneGrouping(out), nil
}
