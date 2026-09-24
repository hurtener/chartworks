package nlq

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// projectionIdentity matches the existing catalog-selection-v1 producer's JSON
// content IDs. These IDs recover topic ownership inside admitted mandatory
// context; they are not signatures, grants or executable semantic proof.
// Keep this encoding aligned with nlqroute.applySelectedContext. Route-level
// tests exercise that producer directly rather than manufacturing its IDs.
func projectionIdentity(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func projectionOwnerFailure() error {
	return &ValidationError{Code: CodeInvalidValue, Path: "projection.ownership"}
}

// independentlySelectedProjection prevents filter-only interpretations and
// required dependencies from masquerading as a complete output selection.
// Unknown/clarification-only root reasons conservatively retain the full topic.
func independentlySelectedProjection(text string) (bool, error) {
	var roots []struct {
		Reference struct {
			Kind string `json:"kind"`
		} `json:"reference"`
		Reason string `json:"reason"`
	}
	if json.Unmarshal([]byte(text), &roots) != nil || len(roots) == 0 || len(roots) > 128 {
		return false, projectionOwnerFailure()
	}
	for _, root := range roots {
		if root.Reason != "catalog_term" && root.Reason != "explicit_reference" && root.Reason != "explicit_metric" {
			continue
		}
		switch root.Reference.Kind {
		case "column", "dimension", "measure", "kpi":
			return true, nil
		}
	}
	return false, nil
}

// scopedPromptRelations extends rendering, not the stored authorization scope.
// Legacy unscoped selection markers cannot guess topic ownership and retain the
// old rendering. Once a scoped marker is present, incomplete mappings are errors.
func scopedPromptRelations(input ContextInput) ([]SourceRelation, bool, error) {
	if input.Constraints == nil || len(input.Relations) == 0 || (input.Strategy != StrategySingleTopic && input.Strategy != StrategyMultiTopic) {
		return input.Relations, false, nil
	}
	topics := []string{input.Topic}
	if input.Strategy == StrategyMultiTopic {
		topics = nil
		for _, topic := range input.Topics {
			topics = append(topics, topic.Topic)
		}
	}
	if len(topics) == 0 || len(topics) > 8 {
		return nil, false, projectionOwnerFailure()
	}
	known := map[string]bool{}
	selectionOwners := map[string]string{}
	for _, topic := range topics {
		if !validID(topic) || known[topic] {
			return nil, false, projectionOwnerFailure()
		}
		known[topic] = true
		selectionOwners["selection-"+projectionIdentity(topic)] = topic
	}
	selected := map[string]bool{}
	independent := map[string]bool{}
	unscoped := false
	for _, constraint := range input.Constraints.Required {
		if constraint.Kind != "selected_semantics" {
			continue
		}
		owner, found := selectionOwners[constraint.ID]
		if !found {
			unscoped = true
			continue
		}
		eligible, err := independentlySelectedProjection(constraint.Text)
		if err != nil {
			return nil, false, err
		}
		selected[owner], independent[owner] = true, eligible
	}
	if len(selected) == 0 {
		return input.Relations, false, nil
	}
	if unscoped {
		return nil, false, projectionOwnerFailure()
	}

	byTopic := map[string][]SourceRelation{}
	datasetOwners := map[string]map[string]bool{}
	physicalNames := map[string]string{}
	for _, relation := range input.Relations {
		if !known[relation.Topic] {
			return nil, false, projectionOwnerFailure()
		}
		if prior, exists := physicalNames[relation.Dataset]; exists && prior != relation.Name {
			return nil, false, projectionOwnerFailure()
		}
		physicalNames[relation.Dataset] = relation.Name
		if datasetOwners[relation.Dataset] == nil {
			datasetOwners[relation.Dataset] = map[string]bool{}
		}
		datasetOwners[relation.Dataset][relation.Topic] = true
		byTopic[relation.Topic] = append(byTopic[relation.Topic], relation)
	}

	dependencies := map[string][]MetricDependency{}
	opaque := map[string]bool{}
	total := 0
	appendDependencies := func(owner string, values []MetricDependency) error {
		if total+len(values) > 4096 {
			return &ValidationError{Code: CodeLimit, Path: "projection.dependencies"}
		}
		total += len(values)
		dependencies[owner] = append(dependencies[owner], values...)
		return nil
	}
	for _, metric := range input.Metrics {
		owner := ""
		for _, topic := range topics {
			if strings.HasPrefix(metric.ID, topic+":") && len(metric.ID) > len(topic)+1 {
				if owner != "" {
					return nil, false, projectionOwnerFailure()
				}
				owner = topic
			}
		}
		if owner == "" {
			return nil, false, projectionOwnerFailure()
		}
		if len(metric.Dependencies) == 0 {
			opaque[owner] = true
		}
		if err := appendDependencies(owner, metric.Dependencies); err != nil {
			return nil, false, err
		}
	}
	for _, constraint := range input.Constraints.Required {
		if constraint.Kind != "semantic_dependency" {
			continue
		}
		var dependency MetricDependency
		if json.Unmarshal([]byte(constraint.Text), &dependency) != nil || !validID(dependency.Kind) || !validID(dependency.ID) || !validText(dependency.Text, 16<<10) {
			return nil, false, &ValidationError{Code: CodeInvalidValue, Path: "projection.dependencies"}
		}
		owner := ""
		for _, topic := range topics {
			if constraint.ID == "semantic-"+projectionIdentity([]string{topic, dependency.Kind, dependency.ID}) {
				owner = topic
				break
			}
		}
		if owner == "" {
			return nil, false, projectionOwnerFailure()
		}
		if err := appendDependencies(owner, []MetricDependency{dependency}); err != nil {
			return nil, false, err
		}
	}

	type coordinate struct{ topic, dataset string }
	projected := map[coordinate]SourceRelation{}
	narrowed := map[string]bool{}
	for _, topic := range topics {
		if !selected[topic] {
			continue
		}
		if len(byTopic[topic]) == 0 {
			return nil, false, projectionOwnerFailure()
		}
		hasColumns := false
		for _, dependency := range dependencies[topic] {
			hasColumns = hasColumns || dependency.Kind == "column"
		}
		// A dataset-only selection or legacy opaque metric is not an exact
		// column closure. Preserve that topic instead of inventing one.
		if !hasColumns {
			continue
		}
		items, err := DependencyRelations(topic, byTopic[topic], dependencies[topic])
		if err != nil {
			return nil, false, err
		}
		if opaque[topic] || !independent[topic] {
			continue
		}
		narrowed[topic] = true
		for _, item := range items {
			projected[coordinate{topic, item.Dataset}] = item
		}
	}
	if len(narrowed) == 0 {
		return input.Relations, false, nil
	}
	var out []SourceRelation
	for _, relation := range input.Relations {
		// The multi-topic admission contract requires independently confirmed
		// relationships over shared datasets. Join choices are not serialized
		// in ContextInput. Retain every shared relation's own reviewed columns
		// so projection cannot orphan a confirmed join not in a metric closure.
		// Do not merge columns between topics or infer a join from this rule.
		if !narrowed[relation.Topic] || len(datasetOwners[relation.Dataset]) > 1 {
			item := relation
			item.Columns = append([]string(nil), relation.Columns...)
			out = append(out, item)
			continue
		}
		if item, found := projected[coordinate{relation.Topic, relation.Dataset}]; found {
			out = append(out, item)
		}
	}
	return out, true, nil
}
