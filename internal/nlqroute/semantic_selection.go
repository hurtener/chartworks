package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

const (
	semanticSelectionVersion = "catalog-selection-v1"
	maxSelectionReferences   = 128
	maxSelectionPasses       = 16
	maxSelectionTerms        = 16384
)

// SemanticSelection is detached interpretation evidence, never authority or a
// SQL conformance certificate. A nil selection denotes the legacy replay path.
type SemanticSelection struct {
	Version string          `json:"version"`
	Topics  []SelectedTopic `json:"topics"`
	Digest  string          `json:"digest"`
}

// SelectedTopic pins roots and rule facts to an admitted immutable publication.
// Descriptive associations in a context closure are not automatically rule facts.
type SelectedTopic struct {
	Topic            string                `json:"topic"`
	TopicVersion     string                `json:"topic_version"`
	PackDigest       string                `json:"pack_digest"`
	Roots            []SelectedRoot        `json:"roots"`
	References       []semantics.Reference `json:"references"`
	DependencyDigest string                `json:"dependency_digest"`
}

// SelectedRoot explains why a concept is part of the question. Candidate
// similarity alone is deliberately not a selection policy.
type SelectedRoot struct {
	Reference semantics.Reference `json:"reference"`
	Reason    string              `json:"reason"`
}

type semanticSelectionState struct {
	roots        map[semantics.Reference]string
	facts        []semantics.Reference
	dependencies []nlq.MetricDependency
}

type selectionTerm struct {
	topic int
	ref   semantics.Reference
}

func referenceLess(a, b semantics.Reference) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Dataset != b.Dataset {
		return a.Dataset < b.Dataset
	}
	if a.ID != b.ID {
		return a.ID < b.ID
	}
	return a.Revision < b.Revision
}

func selectionFailure(locale nlq.Language, reason string) *Clarification {
	prompt := "Choose exact reviewed metric and dimension references; the selected meanings must have one unambiguous connecting relationship."
	if locale == nlq.LanguageSpanish {
		prompt = "Elegí referencias exactas de métricas y dimensiones revisadas; sus significados deben tener una relación de conexión inequívoca."
	}
	return &Clarification{Reason: reason, Outcome: semantics.ClarificationConflicting, Prompt: prompt}
}

// catalogReference resolves stable coordinates only. Human-readable names are
// used by the separate bounded term selector, never to resolve a dependency.
func catalogReference(def topics.Definition, ref semantics.Reference) (any, []semantics.Reference, bool) {
	if !ref.Valid() {
		return nil, nil, false
	}
	filters := func(inputs []semantics.Reference, values []semantics.SemanticFilter) []semantics.Reference {
		out := append([]semantics.Reference(nil), inputs...)
		for _, f := range values {
			out = append(out, f.Field)
		}
		return out
	}
	switch ref.Kind {
	case semantics.KindMeasure:
		for _, v := range def.Measures {
			if v.ID == ref.ID {
				return v, filters([]semantics.Reference{v.Field}, v.Filters), true
			}
		}
	case semantics.KindKPI:
		for _, v := range def.KPIs {
			if v.ID == ref.ID {
				return v, filters(v.Inputs, v.Filters), true
			}
		}
	case semantics.KindDimension:
		for _, v := range def.Dimensions {
			if v.ID == ref.ID {
				return v, filters([]semantics.Reference{v.Field}, v.Filters), true
			}
		}
	case semantics.KindColumn:
		for _, d := range def.Datasets {
			if d.ID != ref.Dataset {
				continue
			}
			for _, v := range d.Columns {
				if v.ID == ref.ID {
					return v, []semantics.Reference{{Kind: semantics.KindDataset, ID: d.ID}}, true
				}
			}
		}
	case semantics.KindDataset:
		for _, d := range def.Datasets {
			if d.ID == ref.ID {
				return struct {
					ID     string         `json:"id"`
					Name   string         `json:"name"`
					Source topics.Binding `json:"source"`
				}{d.ID, d.Name, d.Source}, nil, true
			}
		}
	case semantics.KindJoin:
		for _, j := range def.Joins {
			if j.ID == ref.ID {
				return j, []semantics.Reference{j.Left, j.Right}, true
			}
		}
	case semantics.KindCanonicalEntity:
		for _, v := range def.CanonicalEntities {
			if v.ID == ref.ID && v.Revision == ref.Revision {
				return v, append([]semantics.Reference(nil), v.Keys...), true
			}
		}
	}
	return nil, nil, false
}

func addSelectedRoot(item *admittedTopic, ref semantics.Reference, reason string, omitted []semantics.Reference) (bool, error) {
	if _, _, ok := catalogReference(item.publication.Definition, ref); !ok {
		return false, ErrMetricContext
	}
	for _, other := range omitted {
		if other == ref {
			return false, ErrInvalid
		}
	}
	if _, exists := item.selection.roots[ref]; exists {
		return false, nil
	}
	if len(item.selection.roots) >= maxSelectionReferences {
		return false, nlq.ErrInsufficient
	}
	item.selection.roots[ref] = reason
	return true, nil
}

func initialSemanticSelection(ctx context.Context, in RouteRequest, admitted []admittedTopic, interpretation *Interpretation) error {
	if ctx == nil {
		return ErrInvalid
	}
	for i := range admitted {
		admitted[i].selection = &semanticSelectionState{roots: map[semantics.Reference]string{}}
	}
	resolve := func(ref semantics.Reference, reason string) error {
		var owners []selectionTerm
		for i := range admitted {
			if _, _, ok := catalogReference(admitted[i].publication.Definition, ref); ok {
				owners = append(owners, selectionTerm{topic: i, ref: ref})
			}
		}
		if len(owners) == 0 {
			return ErrInvalid
		}
		if len(owners) > 1 {
			same, err := sameSelectedConcept(ctx, admitted, owners)
			if err != nil {
				return err
			}
			if !same {
				return ErrInvalid
			}
		}
		for _, owner := range owners {
			if _, err := addSelectedRoot(&admitted[owner.topic], ref, reason, in.OmittedRoots); err != nil {
				return err
			}
		}
		return nil
	}
	for _, ref := range in.References {
		if err := resolve(ref, "explicit_reference"); err != nil {
			return err
		}
	}
	for _, id := range in.MetricIDs {
		owner, matches := -1, 0
		var ref semantics.Reference
		for i := range admitted {
			for _, kind := range []semantics.Kind{semantics.KindMeasure, semantics.KindKPI} {
				r := semantics.Reference{Kind: kind, ID: id}
				if _, _, ok := catalogReference(admitted[i].publication.Definition, r); ok {
					owner, ref, matches = i, r, matches+1
				}
			}
		}
		if matches != 1 {
			return ErrInvalid
		}
		if _, err := addSelectedRoot(&admitted[owner], ref, "explicit_metric", in.OmittedRoots); err != nil {
			return err
		}
	}
	for _, ref := range in.OmittedRoots {
		found := false
		for _, item := range admitted {
			_, _, ok := catalogReference(item.publication.Definition, ref)
			found = found || ok
		}
		if !found {
			return ErrInvalid
		}
	}
	if err := selectGrouping(ctx, in, admitted); err != nil {
		return err
	}
	if err := selectCatalogTerms(ctx, in, admitted); err != nil {
		return err
	}
	if interpretation != nil {
		add := func(topic, dimension, reason string) error {
			for i := range admitted {
				if admitted[i].id == topic {
					_, err := addSelectedRoot(&admitted[i], semantics.Reference{Kind: semantics.KindDimension, ID: dimension}, reason, in.OmittedRoots)
					return err
				}
			}
			return ErrInvalid
		}
		for _, v := range interpretation.Values {
			if err := add(v.Topic, v.Dimension, "interpreted_value"); err != nil {
				return err
			}
		}
		for _, v := range interpretation.Temporal {
			if err := add(v.Topic, v.Dimension, "interpreted_time"); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}

func selectCatalogTerms(ctx context.Context, in RouteRequest, admitted []admittedTopic) error {
	terms := map[string][]selectionTerm{}
	maxWords, entries, bytes := 0, 0, 0
	add := func(owner int, ref semantics.Reference, name string, aliases []string) error {
		for _, phrase := range append([]string{name}, aliases...) {
			n := normalizedPhrase(phrase)
			words := len(strings.Fields(n))
			if words == 0 {
				continue
			}
			entries++
			bytes += len(n)
			if entries > maxSelectionTerms || bytes > 1<<20 || words > maxSemanticDepth {
				return nlq.ErrInsufficient
			}
			if words > maxWords {
				maxWords = words
			}
			candidate := selectionTerm{owner, ref}
			duplicate := false
			for _, existing := range terms[n] {
				duplicate = duplicate || existing == candidate
			}
			if !duplicate {
				terms[n] = append(terms[n], candidate)
			}
		}
		return nil
	}
	var redactions []semantics.ClarificationResolution
	for i, item := range admitted {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, v := range item.publication.Definition.Measures {
			if err := add(i, semantics.Reference{Kind: semantics.KindMeasure, ID: v.ID}, v.Name, v.Aliases); err != nil {
				return err
			}
		}
		for _, v := range item.publication.Definition.KPIs {
			if err := add(i, semantics.Reference{Kind: semantics.KindKPI, ID: v.ID}, v.Name, v.Aliases); err != nil {
				return err
			}
		}
		for _, v := range item.publication.Definition.Dimensions {
			if in.Grouping != nil && !groupingContains(in.Grouping, item.id, v.ID) {
				continue
			}
			if err := add(i, semantics.Reference{Kind: semantics.KindDimension, ID: v.ID}, v.Name, v.Aliases); err != nil {
				return err
			}
		}
		for _, p := range item.rules.Definition.Patterns {
			for _, slot := range p.Slots {
				if slot.Sensitivity == semantics.LiteralSensitive {
					redactions = append(redactions, semantics.ClarificationResolution{Topic: item.id, Pattern: p.ID, Slot: slot.ID, Sensitivity: slot.Sensitivity})
				}
			}
		}
	}
	words := strings.Fields(normalizedPhrase(semantics.RedactClarificationText(in.Question, in.Answers, redactions)))
	for start := 0; start < len(words); start++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for width := min(maxWords, len(words)-start); width > 0; width-- {
			candidates := terms[strings.Join(words[start:start+width], " ")]
			if len(candidates) == 0 {
				continue
			}
			// Longest complete term wins. A suppressed/negated longer concept must
			// not accidentally select a shorter constituent name inside that span.
			negated := start > 0 && temporalNegator(words[start-1])
			if start > 1 && (words[start-1] == "by" || words[start-1] == "por") {
				negated = negated || temporalNegator(words[start-2])
			}
			if !negated {
				var eligible, explicit []selectionTerm
				for _, c := range candidates {
					omit := false
					for _, ref := range in.OmittedRoots {
						omit = omit || ref == c.ref
					}
					if omit {
						continue
					}
					eligible = append(eligible, c)
					if reason := admitted[c.topic].selection.roots[c.ref]; strings.HasPrefix(reason, "explicit_") {
						explicit = append(explicit, c)
					}
				}
				if len(eligible) > 1 && len(explicit) == 1 {
					eligible = explicit
				}
				if len(eligible) > 1 {
					same, err := sameSelectedConcept(ctx, admitted, eligible)
					if err != nil {
						return err
					}
					if !same {
						return selectionFailure(in.Locale, "ambiguous_semantic_root")
					}
				}
				for _, c := range eligible {
					if _, err := addSelectedRoot(&admitted[c.topic], c.ref, "catalog_term", in.OmittedRoots); err != nil {
						return err
					}
				}
			}
			start += width - 1
			break
		}
	}
	return nil
}

// Two explicitly combined topics may independently publish the exact same
// concept. Treat that as shared meaning only when typed coordinates, complete
// dependencies and physical source bindings match. Equal labels are not proof.
func sameSelectedConcept(ctx context.Context, admitted []admittedTopic, candidates []selectionTerm) (bool, error) {
	var digest string
	seenTopics := map[int]bool{}
	for _, c := range candidates {
		if c.ref != candidates[0].ref || seenTopics[c.topic] {
			return false, nil
		}
		seenTopics[c.topic] = true
		item := admitted[c.topic]
		item.selection = &semanticSelectionState{roots: map[semantics.Reference]string{c.ref: "catalog_term"}}
		if err := expandSelectedFacts(ctx, &item); err != nil {
			return false, err
		}
		current := readexec.Hash([]any{item.selection.facts, item.selection.dependencies})
		if digest != "" && digest != current {
			return false, nil
		}
		digest = current
	}
	return true, nil
}

// expandSelectedFacts follows actual expression/field/filter dependencies, not
// optional column-to-dimension descriptions. Only the former activate rules.
func expandSelectedFacts(ctx context.Context, item *admittedTopic) error {
	state, def := item.selection, item.publication.Definition
	seen, active := map[semantics.Reference]bool{}, map[semantics.Reference]bool{}
	datasets := map[string]bool{}
	var visit func(semantics.Reference, int) error
	visit = func(ref semantics.Reference, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if active[ref] || depth > maxSemanticDepth {
			return ErrMetricContext
		}
		if seen[ref] {
			return nil
		}
		_, inputs, ok := catalogReference(def, ref)
		if !ok {
			return ErrMetricContext
		}
		seen[ref], active[ref] = true, true
		defer delete(active, ref)
		if len(seen) > maxSelectionReferences {
			return nlq.ErrInsufficient
		}
		if ref.Kind == semantics.KindDataset {
			datasets[ref.ID] = true
		}
		for _, input := range inputs {
			if err := visit(input, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range selectedRoots(state) {
		if err := visit(root.Reference, 0); err != nil {
			return err
		}
	}
	joins, err := uniqueJoinSubgraph(def.Joins, datasets)
	if err != nil {
		return err
	}
	for _, j := range joins {
		if err := visit(semantics.Reference{Kind: semantics.KindJoin, ID: j.ID}, 0); err != nil {
			return err
		}
	}
	state.facts = make([]semantics.Reference, 0, len(seen))
	var promptRoots []semantics.Reference
	for ref := range seen {
		state.facts = append(state.facts, ref)
		if ref.Kind == semantics.KindMeasure || ref.Kind == semantics.KindKPI || ref.Kind == semantics.KindDimension || ref.Kind == semantics.KindColumn {
			promptRoots = append(promptRoots, ref)
		}
	}
	sort.Slice(state.facts, func(i, j int) bool { return referenceLess(state.facts[i], state.facts[j]) })
	sort.Slice(promptRoots, func(i, j int) bool { return referenceLess(promptRoots[i], promptRoots[j]) })
	state.dependencies = nil
	if len(promptRoots) > 0 {
		state.dependencies, err = semanticClosure(ctx, def, promptRoots)
		if err != nil {
			return err
		}
	}
	// Dataset/canonical identity belongs in required context, but selecting a
	// dataset is not permission to dump all of its columns or invent a metric.
	for _, ref := range state.facts {
		if ref.Kind != semantics.KindDataset && ref.Kind != semantics.KindCanonicalEntity {
			continue
		}
		value, _, _ := catalogReference(def, ref)
		raw, err := json.Marshal(value)
		if err != nil {
			return ErrMetricContext
		}
		state.dependencies = append(state.dependencies, nlq.MetricDependency{Kind: string(ref.Kind), ID: ref.ID, Text: string(raw)})
	}
	return nil
}

func selectedRoots(state *semanticSelectionState) []SelectedRoot {
	out := make([]SelectedRoot, 0, len(state.roots))
	for ref, reason := range state.roots {
		out = append(out, SelectedRoot{ref, reason})
	}
	sort.Slice(out, func(i, j int) bool { return referenceLess(out[i].Reference, out[j].Reference) })
	return out
}

func selectedRuleReferences(item admittedTopic) []semantics.Reference {
	refs := append([]semantics.Reference(nil), item.selection.facts...)
	if len(refs) == 0 && len(item.publication.Definition.Datasets) != 0 {
		ids := make([]string, 0, len(item.publication.Definition.Datasets))
		for _, d := range item.publication.Definition.Datasets {
			ids = append(ids, d.ID)
		}
		sort.Strings(ids)
		refs = append(refs, semantics.Reference{Kind: semantics.KindDataset, ID: ids[0]})
	}
	return refs
}

// resolveSemanticSelection keeps all intermediate work request-local. It is
// bounded by reference count and passes, and makes no learned-model calls.
func (s *Service) resolveSemanticSelection(ctx context.Context, e identity.Envelope, in RouteRequest, admitted []admittedTopic, result *RouteResult) error {
	if err := initialSemanticSelection(ctx, in, admitted, result.Interpretation); err != nil {
		return err
	}
	base := *result
	for pass := 0; pass < maxSelectionPasses; pass++ {
		changed := false
		for i := range admitted {
			item := &admitted[i]
			if err := expandSelectedFacts(ctx, item); err != nil {
				return err
			}
			if !item.hasRules {
				continue
			}
			evaluation, err := s.rules.Evaluate(ctx, e, item.id, rulesets.EvaluateRequest{References: selectedRuleReferences(*item), Template: item.template})
			if err != nil {
				return err
			}
			if evaluation.Topic != item.id || evaluation.TopicVersion != item.publication.State.Version || evaluation.PackDigest != item.publication.Digest || evaluation.RuleVersion != item.rules.State.Version || evaluation.RuleDigest != item.rules.Digest {
				return store.ErrConflict
			}
			for _, violation := range evaluation.Result.Violations {
				if violation.Kind != semantics.ViolationMissingRequired {
					return selectionFailure(in.Locale, "conflicting_semantic_rules")
				}
			}
			for _, ref := range evaluation.Result.Excluded {
				for _, fact := range item.selection.facts {
					if ref == fact {
						return selectionFailure(in.Locale, "conflicting_semantic_rules")
					}
				}
			}
			for _, ref := range evaluation.Result.Required {
				present := false
				for _, fact := range item.selection.facts {
					present = present || fact == ref
				}
				if present {
					continue
				}
				added, err := addSelectedRoot(item, ref, "required_rule", in.OmittedRoots)
				if err != nil {
					return selectionFailure(in.Locale, "conflicting_semantic_rules")
				}
				changed = changed || added
			}
			if !evaluation.Result.Allowed && !changed {
				return selectionFailure(in.Locale, "conflicting_semantic_rules")
			}
		}
		if changed {
			continue
		}
		current := base
		current.business = append([]readexec.BusinessConstraint(nil), base.business...)
		if err := s.prepareClarifications(ctx, e, in, admitted, &current); err != nil {
			return err
		}
		for _, resolution := range current.Resolutions {
			var ref *semantics.Reference
			if resolution.Reference != nil {
				ref = resolution.Reference
			} else if resolution.Effect != nil {
				ref = &resolution.Effect.Target
			}
			if ref == nil {
				continue
			}
			for i := range admitted {
				if admitted[i].id == resolution.Topic {
					added, err := addSelectedRoot(&admitted[i], *ref, "clarification", in.OmittedRoots)
					if err != nil {
						return selectionFailure(in.Locale, "conflicting_semantic_selection")
					}
					changed = changed || added
				}
			}
		}
		if changed {
			continue
		}
		current.Selection = selectionView(admitted)
		*result = current
		return nil
	}
	return nlq.ErrInsufficient
}

func selectionView(admitted []admittedTopic) *SemanticSelection {
	out := &SemanticSelection{Version: semanticSelectionVersion, Topics: make([]SelectedTopic, 0, len(admitted))}
	for _, item := range admitted {
		out.Topics = append(out.Topics, SelectedTopic{Topic: item.id, TopicVersion: item.publication.State.Version, PackDigest: item.publication.Digest, Roots: selectedRoots(item.selection), References: append([]semantics.Reference(nil), item.selection.facts...), DependencyDigest: readexec.Hash(item.selection.dependencies)})
	}
	out.Digest = readexec.Hash(out)
	return out
}

// applySelectedContext pins selected metrics using the existing metric contract.
// Additional grouping/filter/connecting dependencies go into the mandatory lane,
// never optional retrieval. Definitions already pinned by a metric are deduped.
func applySelectedContext(admitted []admittedTopic, omitted []semantics.Reference) ([]nlq.PinnedMetric, error) {
	var metrics []nlq.PinnedMetric
	for i := range admitted {
		item := &admitted[i]
		covered := map[string]bool{}
		metricIDs := map[string]bool{}
		for _, root := range selectedRoots(item.selection) {
			if root.Reference.Kind != semantics.KindMeasure && root.Reference.Kind != semantics.KindKPI {
				continue
			}
			// Metric labels use the existing topic:id namespace. Distinct typed
			// roots must never collapse into that same legacy key.
			if metricIDs[root.Reference.ID] {
				return nil, ErrInvalid
			}
			metricIDs[root.Reference.ID] = true
			value, _, ok := catalogReference(item.publication.Definition, root.Reference)
			if !ok {
				return nil, ErrMetricContext
			}
			metric := nlq.PinnedMetric{ID: item.id + ":" + root.Reference.ID}
			switch v := value.(type) {
			case semantics.Measure:
				metric.Text = v.Name + " (" + string(v.Aggregation) + ")"
			case semantics.KPI:
				metric.Text = v.Name + " = " + v.Expression
			default:
				return nil, ErrMetricContext
			}
			var err error
			metric.Dependencies, err = metricClosure(item.publication.Definition, []semantics.Reference{root.Reference})
			if err != nil {
				return nil, err
			}
			metrics = append(metrics, metric)
			for _, d := range metric.Dependencies {
				covered[d.Kind+"\x00"+d.ID] = true
			}
		}
		for _, ref := range omitted {
			if _, _, ok := catalogReference(item.publication.Definition, ref); !ok {
				continue
			}
			if item.constraints == nil {
				item.constraints = &nlq.ConstraintState{Allowed: true}
			}
			raw, err := json.Marshal(ref)
			if err != nil {
				return nil, err
			}
			item.constraints.Excluded = append(item.constraints.Excluded, nlq.MandatoryConstraint{ID: "omitted-" + readexec.Hash([]any{item.id, ref}), Kind: "omitted_semantic_root", Text: "Do not select this concept as an output metric or grouping. It may remain an ingredient of another selected metric: " + string(raw)})
		}
		if len(item.selection.roots) == 0 {
			continue
		}
		if item.constraints == nil {
			item.constraints = &nlq.ConstraintState{Allowed: true}
		}
		roots, err := json.Marshal(selectedRoots(item.selection))
		if err != nil {
			return nil, err
		}
		item.constraints.Required = append(item.constraints.Required, nlq.MandatoryConstraint{ID: "selection-" + readexec.Hash(item.id), Kind: "selected_semantics", Text: string(roots)})
		for _, d := range item.selection.dependencies {
			if covered[d.Kind+"\x00"+d.ID] {
				continue
			}
			raw, err := json.Marshal(d)
			if err != nil {
				return nil, err
			}
			item.constraints.Required = append(item.constraints.Required, nlq.MandatoryConstraint{ID: "semantic-" + readexec.Hash([]string{item.id, d.Kind, d.ID}), Kind: "semantic_dependency", Text: string(raw)})
		}
		if len(item.constraints.Required)+len(item.constraints.Excluded) > nlq.MaxConstraints {
			return nil, nlq.ErrInsufficient
		}
	}
	return metrics, nil
}

func semanticSelectionError(err error, locale nlq.Language) error {
	if errors.Is(err, ErrMetricContext) {
		return selectionFailure(locale, "ambiguous_semantic_relationship")
	}
	return err
}

// Drop omitted candidate demonstrations, but preserve untouched retrieval receipts.
func makeSelectionEvidence(hits []hitWithTopic, admitted []admittedTopic, omitted []semantics.Reference) []nlq.Evidence {
	if len(omitted) == 0 {
		return makeEvidence(hits)
	}
	kept := make([]hitWithTopic, 0, len(hits))
	for _, hit := range hits {
		remove := false
		if hit.hit.Kind == "measure" || hit.hit.Kind == "kpi" || hit.hit.Kind == "dimension" {
			for _, item := range admitted {
				if item.id == hit.topic {
					ref, _, err := catalogFacet(item.publication.Definition, hit.hit)
					if err == nil {
						for _, other := range omitted {
							remove = remove || ref == other
						}
					}
				}
			}
		}
		if !remove {
			kept = append(kept, hit)
		}
	}
	return makeEvidence(kept)
}

func selectionFailureRequest(in RouteRequest, admitted []admittedTopic) RouteRequest {
	out := cloneRouteRequest(in)
	var redactions []semantics.ClarificationResolution
	for _, item := range admitted {
		for _, p := range item.rules.Definition.Patterns {
			for _, slot := range p.Slots {
				if slot.Sensitivity == semantics.LiteralSensitive {
					redactions = append(redactions, semantics.ClarificationResolution{Topic: item.id, Pattern: p.ID, Slot: slot.ID, Sensitivity: slot.Sensitivity})
				}
			}
		}
	}
	out.Question = semantics.RedactClarificationText(in.Question, in.Answers, redactions)
	for i := range out.Examples {
		out.Examples[i].Text = semantics.RedactClarificationText(out.Examples[i].Text, in.Answers, redactions)
	}
	out.Answers = nil
	out.Choices = nil
	return out
}
